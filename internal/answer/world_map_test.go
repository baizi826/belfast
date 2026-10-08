package answer

import "testing"

// 用官方抓包 captures/req33106_rep33107_00.txt 的形状做样本：
//
//	terrain [[0,0,4,0],[0,1,4,0],[0,2,6,1]]  →
//	land_list [{pos:(0,0),type:4,dir:(1,1),distance:0},
//	           {pos:(0,1),type:4,dir:(1,1),distance:0},
//	           {pos:(0,2),type:6,dir:(1,1),distance:1}]
func TestBuildWorldLandListMatchesOfficialShape(t *testing.T) {
	template := &worldChapterTemplateConfig{
		Grids: [][]any{
			{float64(0), float64(0), true},
			{float64(0), float64(1), true},
			{float64(0), float64(2), false},
			{float64(1), float64(0), true},
		},
		Terrain: [][]any{
			{float64(0), float64(0), float64(4), float64(0)},
			{float64(0), float64(1), float64(4), float64(0)},
			{float64(0), float64(2), float64(6), float64(1)},
		},
	}

	lands := buildWorldLandList(template)
	if len(lands) != 3 {
		t.Fatalf("expected 3 land entries (one per terrain row), got %d", len(lands))
	}
	if lands[0].GetPos().GetRow() != 0 || lands[0].GetPos().GetColumn() != 0 {
		t.Fatalf("land[0] pos = (%d,%d), want (0,0)",
			lands[0].GetPos().GetRow(), lands[0].GetPos().GetColumn())
	}
	if lands[0].GetType() != 4 || lands[0].GetDistance() != 0 {
		t.Fatalf("land[0] type/distance = %d/%d, want 4/0", lands[0].GetType(), lands[0].GetDistance())
	}
	if lands[0].GetDir().GetRow() != 1 || lands[0].GetDir().GetColumn() != 1 {
		t.Fatalf("dir must be the constant (1,1), got (%d,%d)",
			lands[0].GetDir().GetRow(), lands[0].GetDir().GetColumn())
	}
	// 唯一能区分 row=x / column=y 的样本：terrain [0,1] 在官方包里是 {row:0, column:1}。
	if lands[1].GetPos().GetRow() != 0 || lands[1].GetPos().GetColumn() != 1 {
		t.Fatalf("terrain [0,1] must map to row=0,column=1; got (%d,%d) — axis order is flipped",
			lands[1].GetPos().GetRow(), lands[1].GetPos().GetColumn())
	}
	// type/distance 是逐条取值的，不是常量。
	if lands[2].GetType() != 6 || lands[2].GetDistance() != 1 {
		t.Fatalf("land[2] type/distance = %d/%d, want 6/1", lands[2].GetType(), lands[2].GetDistance())
	}
}

// 官方那次 pos_list 118 条 == 模板里 walkable=true 的格子数，所以不可通行的格子必须剔除。
func TestBuildWorldPosListSkipsUnwalkableGrids(t *testing.T) {
	template := &worldChapterTemplateConfig{
		Grids: [][]any{
			{float64(0), float64(0), true},
			{float64(0), float64(1), true},
			{float64(0), float64(2), false},
			{float64(1), float64(0), true},
		},
	}
	positions := buildWorldPosList(template)
	if len(positions) != 3 {
		t.Fatalf("expected 3 walkable cells, got %d", len(positions))
	}
	for _, p := range positions {
		if p.GetPos().GetRow() == 0 && p.GetPos().GetColumn() == 2 {
			t.Fatalf("unwalkable cell (0,2) must be excluded")
		}
	}
}

// flag 3 = "这包带着可用的格子数据"。1（已通关）/ 2（有视野）我们还没落库，不能宣称。
func TestBuildWorldStateFlagOnlyClaimsCellData(t *testing.T) {
	flags := buildWorldStateFlag()
	if len(flags) != 1 || flags[0] != worldMapFlagHasCells {
		t.Fatalf("expected exactly [%d], got %v", worldMapFlagHasCells, flags)
	}
}

// 模板缺失时必须回空列表而不是 panic —— WorldMapRequest 允许 loadWorldChapterTemplate 返回 nil。
func TestWorldMapBuildersTolerateMissingTemplate(t *testing.T) {
	if got := buildWorldLandList(nil); len(got) != 0 {
		t.Fatalf("nil template should yield no land entries, got %d", len(got))
	}
	if got := buildWorldPosList(nil); len(got) != 0 {
		t.Fatalf("nil template should yield no positions, got %d", len(got))
	}
}
