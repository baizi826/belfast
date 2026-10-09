package answer

import "testing"

// 回归点：按 15501（舰体改良I）改造绫波时服务端回 result=1，客户端显示
// "突破舰船失败：无效操作"。
//
// 该行的 ship_id 是空表 —— ShareCfg/transform_data_template.json 1253 行里有 1209 行都是空表：
// 它们是"通用改造项目"（不限定船），语义是保持本体模板，不是"没有匹配的船"。
func TestFindRemouldTargetTreatsEmptyShipIDListAsAnyShip(t *testing.T) {
	const ayanami = uint32(301054)

	target, ok := findRemouldTarget(nil, ayanami)
	if !ok || target != ayanami {
		t.Fatalf("空 ship_id 应当放行且保持本体模板，得到 (%d, %v)", target, ok)
	}

	// 空表在 Go 侧也可能解成空切片而不是 nil，两种都要放行。
	if target, ok := findRemouldTarget([][]uint32{}, ayanami); !ok || target != ayanami {
		t.Fatalf("空切片同上，得到 (%d, %v)", target, ok)
	}

	// 限定船的条目照旧：命中则换模板。
	target, ok = findRemouldTarget([][]uint32{{203024, 203124}}, 203024)
	if !ok || target != 203124 {
		t.Fatalf("限定条目应当换成目标模板，得到 (%d, %v)", target, ok)
	}

	// 限定船的条目对不上的船仍然要拒。
	if _, ok := findRemouldTarget([][]uint32{{203024, 203124}}, ayanami); ok {
		t.Fatalf("不匹配的船不该放行")
	}
}
