package orm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ggmolly/belfast/internal/db"
)

func TestConfigEntryListAndGet(t *testing.T) {
	initCommanderItemTestDB(t)
	clearTable(t, &ConfigEntry{})

	entries := []ConfigEntry{
		{Category: "alpha", Key: "a", Data: json.RawMessage(`"one"`)},
		{Category: "alpha", Key: "b", Data: json.RawMessage(`"two"`)},
		{Category: "beta", Key: "a", Data: json.RawMessage(`"three"`)},
	}
	for i := range entries {
		if _, err := db.DefaultStore.Pool.Exec(context.Background(), `INSERT INTO config_entries (category, key, data) VALUES ($1, $2, $3)`, entries[i].Category, entries[i].Key, entries[i].Data); err != nil {
			t.Fatalf("seed config entry: %v", err)
		}
	}
	list, err := ListConfigEntries("alpha")
	if err != nil {
		t.Fatalf("list config entries: %v", err)
	}
	if len(list) != 2 || list[0].Key != "a" || list[1].Key != "b" {
		t.Fatalf("unexpected list order")
	}
	entry, err := GetConfigEntry("beta", "a")
	if err != nil {
		t.Fatalf("get config entry: %v", err)
	}
	if string(entry.Data) != `"three"` {
		t.Fatalf("unexpected entry data")
	}
}

// 这是 ShareCfg/transform_data_template.json 里 15501 行（舰体改良I）的**原样**数据：
// ship_id / edit_trans / condition_id 是空 Lua 表，库里存成 `{}`，而 Go 侧它们是 slice。
//
// 曾经的后果不是“报个错”而是“一个回复都没有”：RemouldShip 里 GetTransformDataTemplate
// 返回 error ⇒ handler 返回 error ⇒ 服务端关连接 ⇒ 客户端只能跳“网络异常”。
// 该表 1253 行里 ship_id 的空表有 1209 行、edit_trans 有 1245 行，所以改造功能整体不可用。
func TestDecodeConfigTreatsEmptyLuaTableAsAbsent(t *testing.T) {
	const shipRemouldRow = `{"id": 15501, "icon": "hp_1", "name": "舰体改良I", "ship_id": {},
		"skin_id": 0, "use_gold": 400, "use_item": [[[18001, 2]]], "use_ship": 0, "max_level": 1,
		"edit_trans": {}, "star_limit": 2, "level_limit": 1, "condition_id": {}}`

	var config TransformDataTemplate
	if err := json.Unmarshal([]byte(shipRemouldRow), &config); err == nil {
		t.Fatalf("原样数据在 `{}=slice` 上本来就该报错；这条断言是防这个前提被悄悄改掉")
	}
	if err := DecodeConfig([]byte(shipRemouldRow), &config); err != nil {
		t.Fatalf("DecodeConfig 应当容忍空 Lua 表: %v", err)
	}
	if config.ID != 15501 || config.UseGold != 400 || config.LevelLimit != 1 || config.StarLimit != 2 {
		t.Fatalf("标量字段解错了: %+v", config)
	}
	if len(config.ShipID) != 0 || len(config.EditTrans) != 0 || len(config.ConditionID) != 0 {
		t.Fatalf("空 Lua 表应当解成空 slice: %+v", config)
	}
	if len(config.UseItem) != 1 || config.UseItem[0][0][0] != 18001 || config.UseItem[0][0][1] != 2 {
		t.Fatalf("非空的 use_item 不该被动: %+v", config.UseItem)
	}

	// 非对象条目（数组 / 标量）照旧解，不能被新逻辑误伤。
	var list []uint32
	if err := DecodeConfig([]byte(`[1,2,3]`), &list); err != nil || len(list) != 3 {
		t.Fatalf("数组条目应当照常解: %v %v", list, err)
	}

	// 只改 slice 字段：非 slice 字段拿到 `{}` 时的行为必须和原来一模一样，
	// 否则 pointer-to-struct 字段会被悄悄变成 nil（下一处 nil 解引用）。
	type mixedFields struct {
		Inner *TransformDataTemplate `json:"inner"`
		IDs   []uint32               `json:"ids"`
	}
	var mixed mixedFields
	if err := DecodeConfig([]byte(`{"inner": {"use_gold": 7}, "ids": {}}`), &mixed); err != nil {
		t.Fatalf("混合形状应当解成功: %v", err)
	}
	if mixed.Inner == nil || mixed.Inner.UseGold != 7 {
		t.Fatalf("非 slice 字段不该被动: %+v", mixed.Inner)
	}
	if len(mixed.IDs) != 0 {
		t.Fatalf("slice 字段的空表应解成空: %+v", mixed.IDs)
	}
}
