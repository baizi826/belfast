package chapter

import (
	"errors"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

func ChapterBattleResultRequest(buffer *[]byte, client *connection.Client) (int, int, error) {
	var payload protobuf.CS_13106
	if err := proto.Unmarshal(*buffer, &payload); err != nil {
		return 0, 13105, err
	}
	state, err := orm.GetChapterState(client.Commander.CommanderID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			response := protobuf.SC_13105{
				MapUpdate:    []*protobuf.CHAPTERCELLINFO_P13{},
				AiList:       []*protobuf.CHAPTERCELLINFO_P13{},
				AddFlagList:  []uint32{},
				DelFlagList:  []uint32{},
				BuffList:     []uint32{},
				CellFlagList: []*protobuf.CELLFLAG{},
			}
			return client.SendMessage(13105, &response)
		}
		return 0, 13105, err
	}
	var current protobuf.CURRENTCHAPTERINFO
	if err := proto.Unmarshal(state.State, &current); err != nil {
		return 0, 13105, err
	}
	// 击杀 → 推进刷怪波次：清完一波补下一波；累计击杀到 boss_refresh 后 BOSS 节点出现。
	// 只把内容变化的格放进 map_update（与官服 13105 行为一致）。
	update := []*protobuf.CHAPTERCELLINFO_P13{}
	if template, err := loadChapterTemplate(current.GetId(), current.GetLoopFlag()); err == nil && template != nil {
		if grids, err := parseChapterGrids(template.Grids); err == nil {
			bossPlan := syncChapterBossPlan(client, current.GetId(), template)
			newKills := current.GetKillCount() + 1
			// ① 被击破的怪格 → 「击沉」态（保留 type/id + flag=disabled，客户端画残骸，点位可再刷新）
			if sunk := markChapterCellSunk(&current, chapterBattleExpedition(client)); sunk != nil {
				update = append(update, sunk)
			}
			// ② 立刻增援：照抄 ALAS spawn_data 表，第 newKills 次战斗该刷多少就刷多少
			//     （表用完后不再刷；BOSS 在 boss_refresh 那一步出现）
			step := chapterSpawnStepForBattle(template, newKills)
			update = append(update, applyChapterSpawnStep(&current, grids, template, step, bossPlan)...)
			current.KillCount = proto.Uint32(newKills)
			if stateBytes, err := proto.Marshal(&current); err == nil {
				state.State = stateBytes
				_ = orm.UpsertChapterState(state)
			}
		}
	}
	response := protobuf.SC_13105{
		MapUpdate:    update,
		AiList:       current.GetAiList(),
		AddFlagList:  []uint32{},
		DelFlagList:  []uint32{},
		BuffList:     current.GetBuffList(),
		CellFlagList: current.GetCellFlagList(),
	}
	return client.SendMessage(13105, &response)
}

func containsChapterCell(cells []*protobuf.CHAPTERCELLINFO_P13, cell *protobuf.CHAPTERCELLINFO_P13) bool {
	for _, candidate := range cells {
		if candidate.GetPos().GetRow() == cell.GetPos().GetRow() && candidate.GetPos().GetColumn() == cell.GetPos().GetColumn() {
			return true
		}
	}
	return false
}

// syncChapterBossPlan 判断本场是不是 BOSS 战（靠战斗会话的 stage_id = 正在打的节点远征 id），
// 是则记一次 BOSS 击破并按进度推进，返回当前的 BOSS 刷怪计划。
// 1-15 章：整图只有 1 个 BOSS，击破即整图结算；16 章：多 BOSS，按序列一个个刷。
func syncChapterBossPlan(client *connection.Client, chapterID uint32, template *chapterTemplate) chapterBossPlan {
	plan := chapterBossPlan{ChapterID: chapterID}
	if template != nil && template.Map >= 16 {
		plan.MultiPhase = true
	}
	if stats, err := loadChapterAutoStatistics(chapterID); err == nil && stats != nil {
		plan.IDs = stats.BossExpeditionID
	}
	progress, err := orm.GetChapterProgress(client.Commander.CommanderID, chapterID)
	if err != nil || progress == nil {
		progress = &orm.ChapterProgress{CommanderID: client.Commander.CommanderID, ChapterID: chapterID}
	}
	if session, err := orm.GetBattleSession(client.Commander.CommanderID); err == nil && session != nil &&
		isBossExpedition(plan.IDs, template, session.StageID) {
		progress.KillBossCount++
		step := uint32(100)
		if plan.MultiPhase {
			if step = template.ProgressBoss; step == 0 {
				step = 100
			}
		}
		if progress.Progress+step > 100 {
			progress.Progress = 100
		} else {
			progress.Progress += step
		}
		_ = orm.UpsertChapterProgress(progress)
	}
	plan.Phase = progress.KillBossCount
	return plan
}

func isBossExpedition(bossIDs []uint32, template *chapterTemplate, expeditionID uint32) bool {
	if expeditionID == 0 {
		return false
	}
	for _, id := range bossIDs {
		if id == expeditionID {
			return true
		}
	}
	if template != nil {
		for _, id := range template.BossExpeditionID {
			if id == expeditionID {
				return true
			}
		}
	}
	return false
}
