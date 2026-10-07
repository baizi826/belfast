package chapter

import (
	"math"
	"time"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// 客户端 model/vo/chapterauto/chapterautoticket.lua: TYPE = { MAIN = 1, WORLD = 2, TIME = 3 }
const (
	chapterAutoTypeMain  = 1
	chapterAutoTypeWorld = 2

	// ponytail: 三个上限都没有官服样本可对照，取"够用且能防滥用"的值。
	// 天花板：官方可能有不同的并发/批量上限，客户端会以 result != 0 表现为"无法开始"。
	chapterAutoMaxQueue = 50
	chapterAutoMaxBatch = 10
)

// BuildChapterAutoBattleList 把 DB 里的进行中周回转成协议结构。
// SC_13001 / SC_13013 / SC_13019 三处共用同一份编码（客户端读的是同一个 page 列表）。
func BuildChapterAutoBattleList(commanderID uint32) ([]*protobuf.CHAPTER_AUTO_BATTLE, error) {
	rows, err := orm.ListChapterAutoCommissions(commanderID)
	if err != nil {
		return nil, err
	}
	out := make([]*protobuf.CHAPTER_AUTO_BATTLE, 0, len(rows))
	for _, row := range rows {
		out = append(out, &protobuf.CHAPTER_AUTO_BATTLE{
			Type:       proto.Uint32(row.Type),
			Id:         proto.Uint32(row.ConfigID),
			Time:       proto.Uint32(row.FinishTime),
			TicketTime: proto.Uint32(row.TicketTime),
			Seconds:    proto.Uint32(row.CostTime),
		})
	}
	return out, nil
}

// computeChapterAutoCost 把基础耗时套上配置修正参数，与客户端 GetFixTime 同式：
//
//	floor(baseTime * time_rate) + time_correction
//
// baseTime 为 0（配置缺失）或 stats 为 nil 时返回 0，调用方按"无法开始"处理。
func computeChapterAutoCost(baseTime uint32, stats *chapterAutoStatistics) uint32 {
	if baseTime == 0 || stats == nil {
		return 0
	}
	rate := stats.TimeRate
	if rate <= 0 {
		rate = 1
	}
	return uint32(math.Floor(float64(baseTime)*rate)) + stats.TimeCorrection
}

// chapterAutoCost 单次周回的耗时（秒）。
//
// 客户端只做显示修正：chapterautoproxy.lua:350
//
//	GetFixTime(type, id, seconds) = floor(seconds * cfg.time_rate) + cfg.time_correction
//
// 所以服务端必须给"原始秒数"。这里取 chapter_template[id].Time 作为基础值，
// 修正参数与客户端同源（chapter_auto_statistics），保证两边看到的时长一致。
//
// ponytail: 官方原始基础耗时未经采样验证（2-4 的模板 time = 100s）。
// 天花板：若官服基础值不同，玩家看到完成时刻偏快/偏慢，但队列内部比例仍然正确。
// 升级路径：抓一次官服 SC_13013 的 chapter_auto_battle_list[].seconds 即可校准。
func chapterAutoCost(configID uint32) (uint32, error) {
	stats, err := loadChapterAutoStatistics(configID)
	if err != nil {
		return 0, err
	}
	base := uint32(0)
	if template, err := loadChapterTemplate(configID, 0); err == nil && template != nil {
		base = template.Time
	}
	return computeChapterAutoCost(base, stats), nil
}

// startChapterAutoJobs 追加 num 个串行周回，返回创建后的完整队列。
// 队列语义来自客户端 ChapterAutoProxy.SortCommissionList / IsAllCommissionFinish：
// 按 finishTime 升序，一次只推进一个，所以第 i 个的完成时刻接在队尾之后。
func startChapterAutoJobs(client *connection.Client, chapterType, configID, num, ticketNum uint32) ([]*protobuf.CHAPTER_AUTO_BATTLE, uint32, error) {
	if chapterType != chapterAutoTypeMain {
		// 2 = 大世界，配置表是 world_auto_statistics，字段语义不同，另行实现。
		return nil, 1, nil
	}
	if num == 0 || num > chapterAutoMaxBatch {
		return nil, 1, nil
	}
	if _, err := loadChapterAutoStatistics(configID); err != nil {
		return nil, 0, err
	}
	cost, err := chapterAutoCost(configID)
	if err != nil {
		return nil, 0, err
	}
	if cost == 0 {
		// 配置里没有这个关卡（或 time 为 0）→ 不能凭空造一个耗时。
		return nil, 1, nil
	}
	existing, err := orm.ListChapterAutoCommissions(client.Commander.CommanderID)
	if err != nil {
		return nil, 0, err
	}
	if len(existing)+int(num) > chapterAutoMaxQueue {
		return nil, 1, nil
	}
	// ponytail: ticket_num 未接入票券系统，一律按"不用票券"入库（ticket_time = 0）。
	// 天花板：玩家用票券不会缩短队列。升级路径：把票券落到 commander_items 后在此兑换。
	_ = ticketNum
	cursor := uint32(time.Now().Unix())
	for _, job := range existing {
		if job.FinishTime > cursor {
			cursor = job.FinishTime
		}
	}
	for i := uint32(0); i < num; i++ {
		cursor += cost
		if _, err := orm.InsertChapterAutoCommission(client.Commander.CommanderID, chapterType, configID, cursor, 0, cost); err != nil {
			return nil, 0, err
		}
	}
	list, err := BuildChapterAutoBattleList(client.Commander.CommanderID)
	if err != nil {
		return nil, 0, err
	}
	return list, 0, nil
}

// HandleChapterAutoStart answers CS_13012 —— 开始自动作战（周回）。
func HandleChapterAutoStart(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13012
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13013, err
	}
	list, result, err := startChapterAutoJobs(client, payload.GetType(), payload.GetId(), payload.GetNum(), payload.GetTicketNum())
	if err != nil {
		return 0, 13013, err
	}
	response := protobuf.SC_13013{
		Result:                proto.Uint32(result),
		ChapterAutoBattleList: list,
	}
	return client.SendMessage(13013, &response)
}

// HandleChapterAutoBatch answers CS_13018 —— 按地图列表批量开始。
func HandleChapterAutoBatch(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13018
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13019, err
	}
	for _, mapID := range payload.GetMapIdList() {
		if _, result, err := startChapterAutoJobs(client, chapterAutoTypeMain, mapID, 1, 0); err != nil {
			return 0, 13019, err
		} else if result != 0 {
			// 一个地图失败就整体失败，和客户端"批量开始"的语义一致（它只读 result）。
			response := protobuf.SC_13019{Result: proto.Uint32(result)}
			return client.SendMessage(13019, &response)
		}
	}
	list, err := BuildChapterAutoBattleList(client.Commander.CommanderID)
	if err != nil {
		return 0, 13019, err
	}
	response := protobuf.SC_13019{
		Result:                proto.Uint32(0),
		ChapterAutoBattleList: list,
	}
	return client.SendMessage(13019, &response)
}

// HandleChapterAutoClaim answers CS_13014 —— 领取已完成的周回。
//
// ponytail: 奖励结算（经验 / 掉落 / 票券）尚未接入，class_exp 与 drop_list 一律为空。
// 天花板：玩家领到的是"完成记录被清掉"，没有实际收益。
// 升级路径：接 battle_session 的掉落与指挥官经验后，在这里按 row.ConfigID 累计发放。
func HandleChapterAutoClaim(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13014
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13015, err
	}
	now := uint32(time.Now().Unix())
	rows, err := orm.ListChapterAutoCommissions(client.Commander.CommanderID)
	if err != nil {
		return 0, 13015, err
	}
	want := payload.GetNum()
	if want == 0 {
		want = uint32(len(rows))
	}
	ids := make([]int64, 0, want)
	seconds := uint32(0)
	for _, row := range rows {
		if uint32(len(ids)) >= want {
			break
		}
		// 队列按 finishTime 升序，第一个还没到点就说明后面都没到。
		if row.FinishTime > now {
			break
		}
		ids = append(ids, row.ID)
		seconds += row.CostTime
	}
	if len(ids) == 0 {
		response := protobuf.SC_13015{
			Result:                proto.Uint32(1),
			DropList:              []*protobuf.DROPINFO{},
			ChapterAutoTicketList: []*protobuf.CHAPTER_AUTO_TICKET{},
			Oil:                   proto.Uint32(0),
			Seconds:               proto.Uint32(0),
			WorldAp:               proto.Uint32(0),
			ClassExp:              proto.Uint32(0),
		}
		return client.SendMessage(13015, &response)
	}
	if err := orm.DeleteChapterAutoCommissions(client.Commander.CommanderID, ids); err != nil {
		return 0, 13015, err
	}
	response := protobuf.SC_13015{
		Result:                proto.Uint32(0),
		DropList:              []*protobuf.DROPINFO{},
		ChapterAutoTicketList: []*protobuf.CHAPTER_AUTO_TICKET{},
		Oil:                   proto.Uint32(0),
		Seconds:               proto.Uint32(seconds),
		WorldAp:               proto.Uint32(0),
		ClassExp:              proto.Uint32(0),
	}
	return client.SendMessage(13015, &response)
}

// HandleChapterAutoUseTicket answers CS_13016 —— 使用票券。
//
// ponytail: 票券是背包物品（chapterautoticket.lua CreateByItem 从 item 的 drop_arg
// 算过期时间），接入需要先确定票券的物品 id 与 drop_arg 语义。
// 天花板：这里只回 result=0，客户端的票券数不会增加。
// 升级路径：确定 68710/68711/68712 对应的真实物品后，在此扣物品并回 SC_13017。
func HandleChapterAutoUseTicket(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13016
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13017, err
	}
	response := protobuf.SC_13017{Result: proto.Uint32(0)}
	return client.SendMessage(13017, &response)
}
