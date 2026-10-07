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

// ChapterAutoJob 是一件"已完成待领取"的周回。奖励发放需要 answer 包的指挥官经验
// 逻辑，所以这里只把记录交出去，由桥接层结算。
type ChapterAutoJob struct {
	ConfigID uint32
	CostTime uint32
}

// TakeFinishedChapterAutoCommissions 取出并删除已完成的周回（先删后发，避免重复领取）。
// want 为 0 表示"全部已完成的"。
func TakeFinishedChapterAutoCommissions(commanderID uint32, want uint32) ([]ChapterAutoJob, error) {
	now := uint32(time.Now().Unix())
	rows, err := orm.ListChapterAutoCommissions(commanderID)
	if err != nil {
		return nil, err
	}
	if want == 0 {
		want = uint32(len(rows))
	}
	jobs := make([]ChapterAutoJob, 0, want)
	ids := make([]int64, 0, want)
	for _, row := range rows {
		if uint32(len(jobs)) >= want {
			break
		}
		// 队列按 finishTime 升序，第一个还没到点就说明后面都没到。
		if row.FinishTime > now {
			break
		}
		jobs = append(jobs, ChapterAutoJob{ConfigID: row.ConfigID, CostTime: row.CostTime})
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := orm.DeleteChapterAutoCommissions(commanderID, ids); err != nil {
		return nil, err
	}
	return jobs, nil
}

// ChapterAutoBaseExp 每次周回给指挥官的固定经验（chapter_auto_statistics.base_class_exp）。
func ChapterAutoBaseExp(configID uint32) (uint32, error) {
	stats, err := loadChapterAutoStatistics(configID)
	if err != nil {
		return 0, err
	}
	if stats == nil {
		return 0, nil
	}
	return stats.BaseClassExp, nil
}

// HandleChapterAutoUseTicket answers CS_13016 —— 使用票券。
//
// 票券的模型（chapterautoticket.lua）已查清：type ∈ {MAIN=1, WORLD=2, TIME=3}，
// time 就是过期时间戳，num 是张数，同 (type, expireTime) 合并计数；
// 显示时映射到虚拟物品 68710/68711/68712。过期时间由 CreateByItem 从物品的
// getConfig("drop_arg") 推出，drop_arg 为空串时是永久票券（FOREVER_TIME = 0xFFFFFFFF）。
//
// 卡住的地方在数据侧：item_data_statistics 里没有 drop_arg 字段，
// item_virtual_data_statistics 里的 drop_arg 全部是空串。而官方 SC_13001 样本里
// 两条票券的 time 是 1791129600 / 1791388799 —— 都不是永久值，
// 说明它们的来源（活动发放？商店？）在我们手上的配置里没有对应记录。
//
// ponytail: 只回 result=0，客户端票券数不变。
// 天花板：买的/送的票券不会出现在周回界面。
// 升级路径：先确定票券真实物品 id（或改成"一律永久票券"），再在此扣物品并回 SC_13017。
func HandleChapterAutoUseTicket(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13016
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13017, err
	}
	response := protobuf.SC_13017{Result: proto.Uint32(0)}
	return client.SendMessage(13017, &response)
}
