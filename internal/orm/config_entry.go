package orm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/db/gen"
)

type ConfigEntry struct {
	ID       uint64          `json:"id"`
	Category string          `json:"category"`
	Key      string          `json:"key"`
	Data     json.RawMessage `json:"data"`
}

// alternateCategory returns the other prefix the same table may be stored under.
//
// The data mirror (belfast-data / AzurLaneLuaScripts) puts the same table under either
// ShareCfg/ or sharecfgdata/ depending on the file, while handlers hard-code one of the
// two paths, so half the referenced categories read as "not found" (2026-09-30: 59 of
// 251 referenced categories were empty in the DB purely because of this).
func alternateCategory(category string) string {
	switch {
	case strings.HasPrefix(category, "ShareCfg/"):
		return "sharecfgdata/" + strings.TrimPrefix(category, "ShareCfg/")
	case strings.HasPrefix(category, "sharecfgdata/"):
		return "ShareCfg/" + strings.TrimPrefix(category, "sharecfgdata/")
	}
	return ""
}

func ListConfigEntries(category string) ([]ConfigEntry, error) {
	if db.DefaultStore == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	ctx := context.Background()
	rows, err := db.DefaultStore.Queries.ListConfigEntriesByCategory(ctx, category)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if alt := alternateCategory(category); alt != "" {
			rows, err = db.DefaultStore.Queries.ListConfigEntriesByCategory(ctx, alt)
			if err != nil {
				return nil, err
			}
		}
	}
	entries := make([]ConfigEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, ConfigEntry{ID: uint64(r.ID), Category: r.Category, Key: r.Key, Data: r.Data})
	}
	return entries, nil
}

func GetConfigEntry(category string, key string) (*ConfigEntry, error) {
	if db.DefaultStore == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	ctx := context.Background()
	row, err := db.DefaultStore.Queries.GetConfigEntry(ctx, gen.GetConfigEntryParams{Category: category, Key: key})
	err = db.MapNotFound(err)
	if err != nil && db.IsNotFound(err) {
		if alt := alternateCategory(category); alt != "" {
			altRow, altErr := db.DefaultStore.Queries.GetConfigEntry(ctx, gen.GetConfigEntryParams{Category: alt, Key: key})
			if db.MapNotFound(altErr) == nil {
				return &ConfigEntry{ID: uint64(altRow.ID), Category: altRow.Category, Key: altRow.Key, Data: altRow.Data}, nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	entry := ConfigEntry{ID: uint64(row.ID), Category: row.Category, Key: row.Key, Data: row.Data}
	return &entry, nil
}

// DecodeConfig unmarshals a config entry into v, reading the game's empty Lua tables as empty.
//
// The config is Lua tables dumped to JSON, and an *empty* table has no type information left: it
// reaches the DB as `{}`, not `[]`. `{}` fields are everywhere (bullet_template alone has 55k of
// them) and `json.Unmarshal` refuses `{}` for any slice field, so one empty field takes a whole
// template down. Two features already died on exactly this:
//
//	ShareCfg/equip_data_template.json     destory_item={}  ⇒ 分解装备(14008) 通用失败
//	ShareCfg/transform_data_template.json ship_id={}       ⇒ 舰船改造(12011) 处理器返回 error
//
// The second was worse than a wrong answer: the error closed the connection, so the client got no
// reply at all and reported a network failure for a request the server had understood fine.
//
// Only fields the target declares as **slices** are rewritten (`{}` ⇒ `[]`). Every other field is
// decoded from the original bytes, so struct, pointer-to-struct and map fields keep behaving
// exactly as before - in particular a pointer field still gets a non-nil pointer for `{}`.
func DecodeConfig(raw json.RawMessage, v any) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		// Not a JSON object (array/scalar entry): nothing to patch, decode as before.
		return json.Unmarshal(raw, v)
	}
	lists := sliceFieldNames(v)
	patched := false
	for key, value := range fields {
		if !lists[key] || !bytes.Equal(bytes.TrimSpace(value), []byte("{}")) {
			continue
		}
		fields[key] = json.RawMessage("[]")
		patched = true
	}
	if !patched {
		return json.Unmarshal(raw, v)
	}
	rebuilt, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(rebuilt, v)
}

// sliceFieldNames returns the JSON names of the struct's slice-typed fields.
func sliceFieldNames(v any) map[string]bool {
	names := make(map[string]bool)
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return names
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.Slice {
			continue
		}
		name := field.Tag.Get("json")
		if comma := strings.IndexByte(name, ','); comma >= 0 {
			name = name[:comma]
		}
		if name == "" {
			name = field.Name
		}
		names[name] = true
	}
	return names
}

func UpsertConfigEntry(category string, key string, data json.RawMessage) error {
	if db.DefaultStore == nil {
		return fmt.Errorf("database is not initialized")
	}
	ctx := context.Background()
	return db.DefaultStore.Queries.UpsertConfigEntry(ctx, gen.UpsertConfigEntryParams{Category: category, Key: key, Data: data})
}
