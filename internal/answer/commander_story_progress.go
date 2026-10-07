package answer

import (
	"time"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

func CommanderStoryProgress(buffer *[]byte, client *connection.Client) (int, int, error) {
	// 9.7 客户端把 oil / time_acc / extra_time_max 加成了 required 字段：不设值 ⇒ proto.Marshal 报
	// "required field ... not set" ⇒ 整条连接被 reset。这三个都是挂机作战的计数器，先给 0。
	response := protobuf.SC_13001{
		Oil:          proto.Uint32(0),
		TimeAcc:      proto.Uint32(0),
		ExtraTimeMax: proto.Uint32(0),
	}
	state, err := orm.GetOrCreateRemasterState(client.Commander.CommanderID)
	if err != nil {
		return 0, 13001, err
	}
	progress, err := orm.ListChapterProgress(client.Commander.CommanderID)
	if err != nil {
		return 0, 13001, err
	}
	if orm.ApplyRemasterDailyReset(state, time.Now()) {
		if err := orm.SaveRemasterState(state); err != nil {
			return 0, 13001, err
		}
	}
	chapterList := make([]*protobuf.CHAPTERINFO, 0, len(progress))
	for _, entry := range progress {
		killBossCount := entry.KillBossCount
		killEnemyCount := entry.KillEnemyCount
		takeBoxCount := entry.TakeBoxCount
		if entry.PassCount >= 3 {
			template, err := loadChapterTemplate(entry.ChapterID, 0)
			if err != nil {
				return 0, 13001, err
			}
			if template != nil {
				if template.Num1 > 0 && killBossCount < template.Num1 {
					killBossCount = template.Num1
				}
				if template.Num2 > 0 && killEnemyCount < template.Num2 {
					killEnemyCount = template.Num2
				}
				if template.Num3 > 0 && takeBoxCount < template.Num3 {
					takeBoxCount = template.Num3
				}
			}
		}
		chapterList = append(chapterList, &protobuf.CHAPTERINFO{
			Id:               proto.Uint32(entry.ChapterID),
			Progress:         proto.Uint32(entry.Progress),
			KillBossCount:    proto.Uint32(killBossCount),
			KillEnemyCount:   proto.Uint32(killEnemyCount),
			TakeBoxCount:     proto.Uint32(takeBoxCount),
			DefeatCount:      proto.Uint32(entry.DefeatCount),
			TodayDefeatCount: proto.Uint32(entry.TodayDefeatCount),
			PassCount:        proto.Uint32(entry.PassCount),
		})
	}
	response.ChapterList = chapterList
	// 进行中的自动作战（周回）队列。客户端 ChapterAutoProxy 靠这份列表画进度、
	// 判断"已完成待领取"，缺失时界面恒为空。同一份编码也用于 SC_13013/13019。
	autoList, err := BuildChapterAutoBattleList(client.Commander.CommanderID)
	if err != nil {
		return 0, 13001, err
	}
	response.ChapterAutoBattleList = autoList
	response.ReactChapter = &protobuf.REACTCHAPTER_INFO{
		Count:           proto.Uint32(state.TicketCount),
		ActiveTimestamp: proto.Uint32(uint32(state.LastDailyResetAt.Unix())),
		ActiveId:        proto.Uint32(state.ActiveChapterID),
		DailyCount:      proto.Uint32(state.DailyCount),
	}
	return client.SendMessage(13001, &response)
}
