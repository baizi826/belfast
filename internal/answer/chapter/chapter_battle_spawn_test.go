package chapter

import (
	"testing"

	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

// 刷怪表照 ALAS campaign_2_4.py 的 spawn_data：
// [{'battle':0,'enemy':2},{'battle':1,'enemy':2},{'battle':2,'enemy':1},{'battle':3,'enemy':2,'boss':1}]
// 与官服抓包逐杀一致；表用完后不再刷怪。
func TestChapterSpawnTableMatchesAlasAndCapture(t *testing.T) {
	template := &chapterTemplate{
		EnemyRefresh:     []uint32{2, 2, 1, 2},
		EliteRefresh:     []uint32{0},
		BoxRefresh:       []uint32{0},
		AiRefresh:        []uint32{0},
		BossRefresh:      3,
		BossExpeditionID: []uint32{204000},
		ExpeditionWeight: [][]any{{204010, 20, 0}, {204011, 20, 0}, {204012, 20, 0}},
	}
	want := []chapterSpawnStep{
		{Enemies: 2},
		{Enemies: 2},
		{Enemies: 1},
		{Enemies: 2, Boss: true},
	}
	table := chapterSpawnTable(template)
	if len(table) != len(want) {
		t.Fatalf("刷怪表长度应为 %d，得到 %d: %+v", len(want), len(table), table)
	}
	for index, step := range table {
		if step != want[index] {
			t.Fatalf("battle %d: 得到 %+v，期望 %+v", index, step, want[index])
		}
	}
	if step := chapterSpawnStepForBattle(template, 4); step != (chapterSpawnStep{}) {
		t.Fatalf("第 4 杀不应再刷怪: %+v", step)
	}
}

// 击破的格子变「击沉」态；增援只落在没刷过（或已击沉）的点位上，不当场占用刚击沉的点位。
func TestChapterSpawnPlacesCellsOnFreeGrids(t *testing.T) {
	template := &chapterTemplate{
		EnemyRefresh:     []uint32{2, 2, 1, 2},
		BossRefresh:      3,
		BossExpeditionID: []uint32{204000},
		ExpeditionWeight: [][]any{{204010, 20, 0}, {204011, 20, 0}, {204012, 20, 0}},
	}
	grids := []chapterGrid{
		{Row: 4, Column: 3, Walkable: true, Attachment: chapterAttachEnemy},
		{Row: 5, Column: 3, Walkable: true, Attachment: chapterAttachEnemy},
		{Row: 3, Column: 5, Walkable: true, Attachment: chapterAttachEnemy},
		{Row: 4, Column: 6, Walkable: true, Attachment: chapterAttachEnemy},
		{Row: 3, Column: 6, Walkable: true, Attachment: chapterAttachEnemy},
		{Row: 3, Column: 7, Walkable: true, Attachment: chapterAttachBoss},
	}
	cell := func(row, column, attachment, id uint32) *protobuf.CHAPTERCELLINFO_P13 {
		return &protobuf.CHAPTERCELLINFO_P13{
			Pos:      buildPos(chapterPos{Row: row, Column: column}),
			ItemType: proto.Uint32(attachment),
			ItemId:   proto.Uint32(id),
			ItemFlag: proto.Uint32(chapterCellActive),
		}
	}
	current := &protobuf.CURRENTCHAPTERINFO{
		Id:       proto.Uint32(204),
		CellList: []*protobuf.CHAPTERCELLINFO_P13{cell(4, 3, chapterAttachEnemy, 204010), cell(5, 3, chapterAttachEnemy, 204011), cell(3, 7, 0, 0)},
	}

	sunk := markChapterCellSunk(current, 204010)
	if sunk == nil || sunk.GetItemFlag() != chapterCellDisabled || sunk.GetItemType() != chapterAttachEnemy {
		t.Fatalf("被击破的怪格应切成击沉态: %+v", sunk)
	}
	changed := applyChapterSpawnStep(current, grids, template, chapterSpawnStepForBattle(template, 1), chapterBossPlan{ChapterID: 204})
	if len(changed) != 2 {
		t.Fatalf("第 1 杀应补 2 只增援，实际 %d", len(changed))
	}
	for _, spawned := range changed {
		if spawned.GetPos().GetRow() == 4 && spawned.GetPos().GetColumn() == 3 {
			t.Fatalf("增援不该刷在刚击沉的点位上")
		}
	}

	// 第 3 杀那一步出 BOSS（boss_refresh = 3）
	bossStep := chapterSpawnStepForBattle(template, 3)
	if !bossStep.Boss {
		t.Fatalf("第 3 杀应是出 BOSS 的那一步: %+v", bossStep)
	}
	found := false
	for _, spawned := range applyChapterSpawnStep(current, grids, template, bossStep, chapterBossPlan{ChapterID: 204}) {
		if spawned.GetItemType() == chapterAttachBoss && spawned.GetItemId() == 204000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("第 3 杀应刷出 BOSS")
	}
}
