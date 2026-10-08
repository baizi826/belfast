package world

import (
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// buildWorldInfo 把 WorldRuntime 映射成 SC_33001.world（WORLDINFO）。
//
// 字段对照官方抓包 captures/req33000_rep33001_00.txt（world 段 1295 行、21 个子字段）：
// 官服发的是 map_id / round / task_finish_count / action_power 系列 / enter_map_id 这几组状态。
//
// ponytail: 只发上面这 9 个。WORLDINFO 的 8 个 required 全在这一组里，所以不会出现
// marshal 失败（SC_13001 当初就是漏了 required 被 reset）。未发的都是 optional/repeated：
// group_list（地图上的舰队位置）、task_list、item_list、goods_list、chapter_list、
// cd_list、buff_list、month_boss —— 它们需要地图级状态，WorldRuntime 还没有对应存储。
// 天花板：大世界界面能显示体力/轮次/当前地图，但地图上的舰队与任务列表是空的。
// 升级路径：给 WorldRuntime 补地图级舰队位置与任务进度后，在这里逐项填上。
func buildWorldInfo(runtime *orm.WorldRuntime) *protobuf.WORLDINFO {
	return &protobuf.WORLDINFO{
		MapId:                    proto.Uint32(runtime.MapID),
		Round:                    proto.Uint32(runtime.Round),
		TaskFinishCount:          proto.Uint32(runtime.TaskFinishCount),
		SubmarineState:           proto.Uint32(0),
		GroupList:                []*protobuf.GROUPINCHAPTER_P33{},
		TaskList:                 []*protobuf.TASK_INFO{},
		ItemList:                 []*protobuf.WORLD_ITEM_INFO{},
		GoodsList:                []*protobuf.GOODS_INFO_P33{},
		ActionPower:              proto.Uint32(runtime.ActionPower),
		ActionPowerExtra:         proto.Uint32(runtime.ActionPowerExtra),
		LastRecoverTimestamp:     proto.Uint32(runtime.LastRecoverTimestamp),
		ActionPowerFetchCount:    proto.Uint32(runtime.ActionPowerFetchCount),
		LastChangeGroupTimestamp: proto.Uint32(runtime.LastChangeGroupTimestamp),
		EnterMapId:               proto.Uint32(runtime.EnterMapID),
		CdList:                   []*protobuf.IDTIMEINFO{},
		BuffList:                 []*protobuf.BUFF_INFO{},
		ChapterList:              []*protobuf.WORLDMAPID{},
		MonthBoss:                []*protobuf.KVDATA{},
	}
}

// buildWorldCleanChapter 发 SC_33001.clean_chapter —— 已压制的海域列表。
//
// 存的是 world_chapter_random 的 id：官方那次抓包的 127 个值全部命中该表，
// world_chapter_template 表 0 命中。客户端拿去喂 NetUpdateWorldMapPressing。
func buildWorldCleanChapter(runtime *orm.WorldRuntime) []uint32 {
	if runtime == nil || len(runtime.ClearedChapters) == 0 {
		return []uint32{}
	}
	out := make([]uint32, len(runtime.ClearedChapters))
	copy(out, runtime.ClearedChapters)
	return out
}

func buildWorldCountInfo(runtime *orm.WorldRuntime) *protobuf.COUNTINFO {
	activateCount := uint32(0)
	if runtime.MapID > 0 {
		activateCount = 1
	}
	return &protobuf.COUNTINFO{
		StepCount:      proto.Uint32(0),
		TreasureCount:  proto.Uint32(0),
		TaskProgress:   proto.Uint32(runtime.Progress),
		ActivateCount:  proto.Uint32(activateCount),
		CollectionList: []uint32{},
	}
}

func worldBossStateToProto(boss *orm.WorldBossBossState) *protobuf.WORLDBOSS_INFO_P34 {
	if boss == nil {
		return &protobuf.WORLDBOSS_INFO_P34{
			Id:         proto.Uint32(0),
			TemplateId: proto.Uint32(0),
			Lv:         proto.Uint32(0),
			Hp:         proto.Uint32(0),
			Owner:      proto.Uint32(0),
			LastTime:   proto.Uint32(0),
			KillTime:   proto.Uint32(0),
			FightCount: proto.Uint32(0),
			RankCount:  proto.Uint32(0),
		}
	}
	return &protobuf.WORLDBOSS_INFO_P34{
		Id:         proto.Uint32(boss.ID),
		TemplateId: proto.Uint32(boss.TemplateID),
		Lv:         proto.Uint32(boss.Lv),
		Hp:         proto.Uint32(boss.Hp),
		Owner:      proto.Uint32(boss.Owner),
		LastTime:   proto.Uint32(boss.LastTime),
		KillTime:   proto.Uint32(boss.KillTime),
		FightCount: proto.Uint32(boss.FightCount),
		RankCount:  proto.Uint32(boss.RankCount),
	}
}
