package orm

import (
	"encoding/json"
	"fmt"
	"strings"
)

const shipBreakoutCategory = "sharecfgdata/ship_data_breakout.json"

// ShipBreakoutItems 容忍 [][]uint32 与 {} 两种形态。
// 9.7 的 sharecfgdata/ship_data_breakout.json 里 use_item 有的行是空对象（Lua 空表被
// tools/lua2json.py 写成 {}），直接解进 [][]uint32 会报
// "cannot unmarshal object into Go struct field ShipBreakoutConfig.use_item of type [][]uint32"，
// 调用方把 error 往上抛 ⇒ 整条连接被 reset（实测 SC_12025 一进突破就掉线）。
type ShipBreakoutItems [][]uint32

func (s *ShipBreakoutItems) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed[0] != '[' {
		*s = nil // {} / null / 其它非数组形态一律当空
		return nil
	}
	var v [][]uint32
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return err
	}
	*s = v
	return nil
}

type ShipBreakoutConfig struct {
	ID           uint32            `json:"id"`
	BreakoutID   uint32            `json:"breakout_id"`
	PreID        uint32            `json:"pre_id"`
	Level        uint32            `json:"level"`
	UseGold      uint32            `json:"use_gold"`
	UseItem      ShipBreakoutItems `json:"use_item"`
	UseChar      uint32            `json:"use_char"`
	UseCharNum   uint32            `json:"use_char_num"`
	WeaponIDs    []uint32          `json:"weapon_ids"`
	BreakoutView string            `json:"breakout_view"`
}

func GetShipBreakoutConfig(templateID uint32) (*ShipBreakoutConfig, error) {
	return GetShipBreakoutConfigTx(nil, templateID)
}

func GetShipBreakoutConfigTx(_ any, templateID uint32) (*ShipBreakoutConfig, error) {
	entry, err := GetConfigEntry(shipBreakoutCategory, fmt.Sprintf("%d", templateID))
	if err != nil {
		return nil, err
	}
	var config ShipBreakoutConfig
	if err := json.Unmarshal(entry.Data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
