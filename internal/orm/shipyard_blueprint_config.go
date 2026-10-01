package orm

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ggmolly/belfast/internal/db"
)

const (
	shipDataBlueprintCategory         = "ShareCfg/ship_data_blueprint.json"
	shipStrengthenBlueprintCategory   = "ShareCfg/ship_strengthen_blueprint.json"
	shipyardGameSetCategory           = "ShareCfg/gameset.json"
	shipDataBlueprintTaskCategory     = "ShareCfg/task_data_template.json"
	shipDataBlueprintTaskCategoryLC   = "sharecfgdata/task_data_template.json"
	itemStatisticsCategoryPrimary     = "sharecfgdata/item_data_statistics.json"
	itemStatisticsCategoryShipyardAlt = "ShareCfg/item_data_statistics.json"
)

// BlueprintIDList 容忍 []uint32 与 {} 两种形态（与 ShipBreakoutItems / TechnologyRows 同因同治）。
// 9.7 的 ShareCfg/ship_data_blueprint.json 里 fate_strengthen（14 行）与 gain_item_id（24 行）
// 是空对象（Lua 空表被 tools/lua2json.py 写成 {}），直接解进 []uint32 会报
// "cannot unmarshal object into Go struct field ShipDataBlueprintConfig.*"，
// handler 把 error 往上抛 ⇒ client.CloseWithError reset 整条连接。
type BlueprintIDList []uint32

func (s *BlueprintIDList) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed[0] != '[' {
		*s = nil // {} / null / 其它非数组形态一律当空
		return nil
	}
	var v []uint32
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return err
	}
	*s = v
	return nil
}

type ShipDataBlueprintConfig struct {
	ID                      uint32          `json:"id"`
	BlueprintVersion        uint32          `json:"blueprint_version"`
	UnlockTaskOpenCondition []uint32        `json:"unlock_task_open_condition"`
	UnlockTask              [][]uint32      `json:"unlock_task"`
	StrengthenEffect        []uint32        `json:"strengthen_effect"`
	FateStrengthen          BlueprintIDList `json:"fate_strengthen"`
	StrengthenItem          uint32          `json:"strengthen_item"`
	GainItemID              BlueprintIDList `json:"gain_item_id"`
	IsPursuing              uint32          `json:"is_pursuing"`
	Price                   uint32          `json:"price"`
}

func (c *ShipDataBlueprintConfig) ShipTemplateID() uint32 {
	return c.ID*10 + 1
}

type ShipStrengthenBlueprintConfig struct {
	ID      uint32 `json:"id"`
	LV      uint32 `json:"lv"`
	NeedExp uint32 `json:"need_exp"`
	NeedLV  uint32 `json:"need_lv"`
}

type ShipyardTaskTemplateConfig struct {
	ID        uint32 `json:"id"`
	TargetNum uint32 `json:"target_num"`
}

func GetShipDataBlueprintConfig(id uint32) (*ShipDataBlueprintConfig, error) {
	entry, err := GetConfigEntry(shipDataBlueprintCategory, strconv.FormatUint(uint64(id), 10))
	if err != nil {
		return nil, err
	}
	var cfg ShipDataBlueprintConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetShipStrengthenBlueprintConfig(id uint32) (*ShipStrengthenBlueprintConfig, error) {
	entry, err := GetConfigEntry(shipStrengthenBlueprintCategory, strconv.FormatUint(uint64(id), 10))
	if err != nil {
		return nil, err
	}
	var cfg ShipStrengthenBlueprintConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetShipyardTaskTemplateConfig(taskID uint32) (*ShipyardTaskTemplateConfig, error) {
	key := strconv.FormatUint(uint64(taskID), 10)
	entry, err := GetConfigEntry(shipDataBlueprintTaskCategory, key)
	if err != nil {
		if !db.IsNotFound(err) {
			return nil, err
		}
		entry, err = GetConfigEntry(shipDataBlueprintTaskCategoryLC, key)
		if err != nil {
			return nil, err
		}
	}
	var cfg ShipyardTaskTemplateConfig
	if err := json.Unmarshal(entry.Data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func GetTechnologyCatchupItem(version uint32) (uint32, uint32, error) {
	entry, err := GetConfigEntry(shipyardGameSetCategory, "technology_catchup_itemid")
	if err != nil {
		return 0, 0, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(entry.Data, &root); err != nil {
		return 0, 0, err
	}
	description, ok := root["description"]
	if !ok {
		return 0, 0, fmt.Errorf("gameset technology_catchup_itemid missing description")
	}
	var matrix [][]uint32
	if err := json.Unmarshal(description, &matrix); err != nil {
		return 0, 0, err
	}
	if version == 0 || int(version) > len(matrix) || len(matrix[version-1]) < 2 {
		return 0, 0, db.ErrNotFound
	}
	return matrix[version-1][0], matrix[version-1][1], nil
}

func GetShipyardPursueDiscounts(ur bool) ([]uint32, error) {
	key := "blueprint_pursue_discount_ssr"
	if ur {
		key = "blueprint_pursue_discount_ur"
	}
	entry, err := GetConfigEntry(shipyardGameSetCategory, key)
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(entry.Data, &root); err != nil {
		return nil, err
	}
	description, ok := root["description"]
	if !ok {
		return nil, fmt.Errorf("gameset %s missing description", key)
	}
	var anyValue any
	if err := json.Unmarshal(description, &anyValue); err != nil {
		return nil, err
	}
	values := collectUintValues(anyValue, nil)
	if len(values) == 0 {
		return nil, db.ErrNotFound
	}
	return values, nil
}

func LoadItemUsageExp(itemID uint32) (uint32, error) {
	key := strconv.FormatUint(uint64(itemID), 10)
	entry, err := GetConfigEntry(itemStatisticsCategoryPrimary, key)
	if err != nil {
		if !db.IsNotFound(err) {
			return 0, err
		}
		entry, err = GetConfigEntry(itemStatisticsCategoryShipyardAlt, key)
		if err != nil {
			return 0, err
		}
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(entry.Data, &root); err != nil {
		return 0, err
	}
	usageArg, ok := root["usage_arg"]
	if !ok {
		return 0, db.ErrNotFound
	}
	var list []uint32
	if err := json.Unmarshal(usageArg, &list); err == nil && len(list) > 0 && list[0] > 0 {
		return list[0], nil
	}
	var value uint32
	if err := json.Unmarshal(usageArg, &value); err == nil && value > 0 {
		return value, nil
	}
	var text string
	if err := json.Unmarshal(usageArg, &text); err == nil {
		parsed, err := strconv.ParseUint(text, 10, 32)
		if err == nil && parsed > 0 {
			return uint32(parsed), nil
		}
	}
	return 0, fmt.Errorf("unsupported item usage_arg for %d", itemID)
}

func collectUintValues(value any, out []uint32) []uint32 {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 {
			out = append(out, uint32(typed))
		}
	case string:
		parsed, err := strconv.ParseUint(typed, 10, 32)
		if err == nil {
			out = append(out, uint32(parsed))
		}
	case []any:
		for _, nested := range typed {
			out = collectUintValues(nested, out)
		}
	case map[string]any:
		for _, nested := range typed {
			out = collectUintValues(nested, out)
		}
	}
	return out
}
