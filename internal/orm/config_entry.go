package orm

import (
	"context"
	"encoding/json"
	"fmt"
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

func UpsertConfigEntry(category string, key string, data json.RawMessage) error {
	if db.DefaultStore == nil {
		return fmt.Errorf("database is not initialized")
	}
	ctx := context.Background()
	return db.DefaultStore.Queries.UpsertConfigEntry(ctx, gen.UpsertConfigEntryParams{Category: category, Key: key, Data: data})
}
