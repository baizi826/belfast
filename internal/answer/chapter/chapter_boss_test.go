package chapter

import (
	"testing"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// 官服抓包（2-4，boss_refresh=3）：进图无 BOSS，第 1/2 杀只刷小怪，第 3 杀那一步才刷 BOSS。
func TestChapterBossAppearsAtBossRefresh(t *testing.T) {
	template := &chapterTemplate{
		BossRefresh:      3,
		Num2:             12,
		EnemyRefresh:     []uint32{2, 2, 1, 2},
		EliteRefresh:     []uint32{0},
		BoxRefresh:       []uint32{0},
		AiRefresh:        []uint32{0},
		BossExpeditionID: []uint32{204000},
	}
	if got := chapterBossThreshold(template); got != 3 {
		t.Fatalf("boss 阈值应为 boss_refresh=3，得到 %d", got)
	}
	if chapterSpawnStepForBattle(template, 2).Boss {
		t.Fatalf("第 2 杀不该出 BOSS")
	}
	if !chapterSpawnStepForBattle(template, 3).Boss {
		t.Fatalf("第 3 杀应出 BOSS")
	}
	// 进图（battle 0）不下发 BOSS 格
	grids := []chapterGrid{
		{Row: 3, Column: 7, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 4, Column: 3, Walkable: true, Attachment: chapterAttachEnemy},
	}
	for _, cell := range buildChapterCellsAtProgress(grids, template, 0) {
		if cell.GetItemType() == chapterAttachBoss {
			t.Fatalf("进图不该有 BOSS 格: %+v", cell)
		}
	}
}

// 11-4 模板里有 6 个 boss 候选格，官服同一时刻只亮一个（换点位后换一个）。
func TestChapterOnlyOneBossCellActive(t *testing.T) {
	template := &chapterTemplate{
		BossRefresh:      6,
		Num2:             45,
		EnemyRefresh:     []uint32{0, 0, 2, 1, 1},
		BossExpeditionID: []uint32{1104000},
	}
	grids := []chapterGrid{
		{Row: 8, Column: 10, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 8, Column: 1, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 5, Column: 1, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 1, Column: 8, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 1, Column: 2, Walkable: true, Attachment: chapterAttachBoss},
		{Row: 1, Column: 1, Walkable: true, Attachment: chapterAttachBoss},
	}
	current := &protobuf.CURRENTCHAPTERINFO{Id: proto.Uint32(1104), CellList: buildChapterCellsAtProgress(grids, template, 0)}
	first := applyChapterBoss(current, grids, template, chapterBossPlan{ChapterID: 1104})
	if first == nil || first.GetItemType() != chapterAttachBoss {
		t.Fatalf("应刷出一个 BOSS: %+v", first)
	}
	if active := countActiveChapterBoss(current); active != 1 {
		t.Fatalf("同时活跃的 BOSS 格应为 1，得到 %d", active)
	}
	if again := applyChapterBoss(current, grids, template, chapterBossPlan{ChapterID: 1104}); again != nil {
		t.Fatalf("同一个 BOSS 已在场时不该重复下发")
	}
}

func countActiveChapterBoss(current *protobuf.CURRENTCHAPTERINFO) int {
	active := 0
	for _, cell := range current.GetCellList() {
		if cell.GetItemType() == chapterAttachBoss && cell.GetItemFlag() == chapterCellActive {
			active++
		}
	}
	return active
}

// 16 章才是多 BOSS：按 boss_expedition_id 序列一个个刷（16-1 有 4 个）。
func TestChapterBossPlanMultiPhase(t *testing.T) {
	plan := chapterBossPlan{ChapterID: 1601, IDs: []uint32{160021, 160002, 160003, 160005}, MultiPhase: true}
	for phase, want := range []uint32{160021, 160002, 160003, 160005} {
		plan.Phase = uint32(phase)
		got, ok := plan.next(0)
		if !ok || got != want {
			t.Fatalf("phase=%d: got=%d ok=%v want=%d", phase, got, ok, want)
		}
	}
	plan.Phase = 4
	if _, ok := plan.next(0); ok {
		t.Fatalf("4 个 BOSS 都击破后不应再刷")
	}
}

// 1-15 章整图只有 1 个 BOSS：击破一次后不再刷（多条候选 id 只是同一个 BOSS 的随机刷新点位）。
func TestChapterBossPlanSinglePhase(t *testing.T) {
	plan := chapterBossPlan{ChapterID: 1104, IDs: []uint32{1104000, 1104050, 1104060, 1104080, 1104090}}
	got, ok := plan.next(0)
	if !ok || got != 1104000 {
		t.Fatalf("首个 BOSS 应取候选第一个: got=%d ok=%v", got, ok)
	}
	plan.Phase = 1
	if _, ok := plan.next(0); ok {
		t.Fatalf("11-4 击破 1 个 BOSS 后不应再刷")
	}
	// 随机点位取向：同一阶段稳定（重建/重进地图 BOSS 不乱跳）
	cells := []chapterPos{{Row: 8, Column: 10}, {Row: 1, Column: 1}, {Row: 5, Column: 1}}
	fresh := chapterBossPlan{ChapterID: 1104}
	first, ok1 := fresh.pickCell(cells)
	again, ok2 := fresh.pickCell(cells)
	if !ok1 || !ok2 || first != again {
		t.Fatalf("同一阶段 BOSS 点位应稳定: %v/%v", first, again)
	}
	if _, ok := (chapterBossPlan{ChapterID: 1104, Phase: 1}).pickCell(cells); !ok {
		t.Fatalf("击破后仍应能取到下一个点位")
	}
}
