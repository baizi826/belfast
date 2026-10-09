package misc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/logger"
)

// Ship group types.
//
// SC_17001's `ship_info_list[].id` must be a *group_type* -- a key of the client's
// ship_data_group.get_id_list_by_group_type -- and NOT `owned_ships.ship_id / 10`.
// Verified against the official reply: 793/793 of its ids are group_types, while
// `ship_id / 10` agrees only for "prototype" ids. Refit ships break it, because one
// group_type covers several template ids:
//
//	ship_data_template.get_id_list_by_group_type
//	[10126] = { 101261, 101262, 101263, 101264, ..., 101994, 900431 }
//	                                     ^ 101264/10 = 10126 OK
//	                                              ^ 101994/10 = 10199 NOT a group_type
//
// The client resolves each entry through pg.ship_data_group[id]; an id it does not know
// makes ShipGroup.__index raise, and the login-time painting check aborts
// (LoginMediator:checkPaintingRes -> PaintingGroupConst:GetPaintingNameListInLogin ->
// CollectionProxy:getGroups), so the client never reaches the main screen.
//
// ships.group_type is populated from ship_group_types.json. That file is produced by
// tools/gen-ship-group-types.py from the mirrored Lua and refreshed with the Lua mirror,
// the same status as versions.json or client_resources.json -- the server never reads a
// captured packet here.
const shipGroupTypesFile = "ship_group_types.json"

var (
	shipGroupTypesOnce sync.Once
	shipGroupTypesMap  map[uint32]uint32
)

// ShipGroupTypes returns template_id -> group_type, or nil when the mapping file is
// absent (the caller then has no way to answer and must not invent one).
func ShipGroupTypes() map[uint32]uint32 {
	shipGroupTypesOnce.Do(func() {
		dir := belfastDataDir()
		if dir == "" {
			return
		}
		path := filepath.Join(dir, shipGroupTypesFile)
		body, err := os.ReadFile(path)
		if err != nil {
			logger.LogEvent("GameData", "ShipGroupTypes",
				fmt.Sprintf("cannot read %s: %v", path, err), logger.LOG_LEVEL_WARN)
			return
		}
		var doc struct {
			Entries map[string]uint32 `json:"entries"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			logger.LogEvent("GameData", "ShipGroupTypes",
				fmt.Sprintf("cannot parse %s: %v", path, err), logger.LOG_LEVEL_ERROR)
			return
		}
		m := make(map[uint32]uint32, len(doc.Entries))
		for k, v := range doc.Entries {
			var id uint32
			if _, err := fmt.Sscanf(k, "%d", &id); err != nil {
				continue
			}
			m[id] = v
		}
		if len(m) > 0 {
			shipGroupTypesMap = m
			logger.LogEvent("GameData", "ShipGroupTypes",
				fmt.Sprintf("loaded %d template->group_type mappings", len(m)), logger.LOG_LEVEL_INFO)
		}
	})
	return shipGroupTypesMap
}

// ShipGroupType resolves one template id. ok is false when the mapping is unknown, so
// callers can skip the ship instead of emitting an id the client cannot look up.
func ShipGroupType(templateID uint32) (uint32, bool) {
	m := ShipGroupTypes()
	if m == nil {
		return 0, false
	}
	g, ok := m[templateID]
	return g, ok
}

// BackfillShipGroupTypes writes the mapping into ships.group_type.
//
// Idempotent and cheap on repeat runs (it only touches rows whose value differs), so it
// is safe to call on every startup: a reseed wipes the column and this restores it
// without anyone remembering a manual step.
func BackfillShipGroupTypes(ctx context.Context) error {
	m := ShipGroupTypes()
	if len(m) == 0 {
		logger.LogEvent("GameData", "ShipGroupTypes",
			fmt.Sprintf("no %s; ships.group_type left untouched", shipGroupTypesFile), logger.LOG_LEVEL_WARN)
		return nil
	}

	// Build the same pairs the JSON holds, then apply with a single UPDATE ... FROM.
	ids := make([]int64, 0, len(m))
	groups := make([]int64, 0, len(m))
	for id, g := range m {
		ids = append(ids, int64(id))
		groups = append(groups, int64(g))
	}

	tag, err := db.DefaultStore.Pool.Exec(ctx, `
UPDATE ships AS s
SET group_type = v.group_type
FROM (
	SELECT unnest($1::bigint[]) AS template_id, unnest($2::bigint[]) AS group_type
) AS v
WHERE s.template_id = v.template_id
  AND (s.group_type IS DISTINCT FROM v.group_type)
`, ids, groups)
	if err != nil {
		return fmt.Errorf("backfill ships.group_type: %w", err)
	}
	if n := tag.RowsAffected(); n > 0 {
		logger.LogEvent("GameData", "ShipGroupTypes",
			fmt.Sprintf("backfilled ships.group_type for %d rows", n), logger.LOG_LEVEL_INFO)
	}

	// Anything the mapping does not cover stays NULL. Surface it: those ships would be
	// silently dropped from the collection response.
	var missing int64
	if err := db.DefaultStore.Pool.QueryRow(ctx,
		`SELECT count(*) FROM ships WHERE group_type IS NULL`).Scan(&missing); err != nil {
		return err
	}
	if missing > 0 {
		logger.LogEvent("GameData", "ShipGroupTypes",
			fmt.Sprintf("%d ships have no group_type (absent from %s) and will be skipped in SC_17001",
				missing, shipGroupTypesFile), logger.LOG_LEVEL_WARN)
	}
	return nil
}
