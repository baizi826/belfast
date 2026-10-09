package orm

import (
	"context"
	"errors"

	"github.com/ggmolly/belfast/internal/db"
	"github.com/jackc/pgx/v5"
)

const summaryLove200Threshold = 20000

type PlayerSummaryStats struct {
	RegisterDate    uint32
	ChapterID       uint32
	MarryNumber     uint32
	MedalNumber     uint32
	FurnitureNumber uint32
	FurnitureWorth  uint32
	CharacterID     uint32
	FirstLadyID     uint32
	FirstLadyName   string
	FirstLadyTime   uint32
	FirstOnline     uint32
	WorldMaxTask    uint32
	CollectNum      uint32
	Combat          uint32
	ShipNumTotal    uint32
	ShipNum120      uint32
	ShipNum125      uint32
	Love200Num      uint32
	SkinNum         uint32
	SkinShipNum     uint32
}

func GetPlayerSummaryStats(commanderID uint32) (*PlayerSummaryStats, error) {
	ctx := context.Background()
	stats := &PlayerSummaryStats{}

	var registerDate int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COALESCE(
	EXTRACT(EPOCH FROM accounts.created_at)::bigint,
	EXTRACT(EPOCH FROM commanders.last_login)::bigint,
	EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
)
FROM commanders
LEFT JOIN accounts ON accounts.commander_id = commanders.commander_id
WHERE commanders.commander_id = $1
`, int64(commanderID)).Scan(&registerDate); err != nil {
		return nil, err
	}
	stats.RegisterDate = uint32(registerDate)
	stats.FirstOnline = stats.RegisterDate

	var chapterID int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COALESCE(MAX(chapter_id), 0)
FROM chapter_progress
WHERE commander_id = $1
`, int64(commanderID)).Scan(&chapterID); err != nil {
		return nil, err
	}
	if chapterID < 101 {
		chapterID = 101
	}
	stats.ChapterID = uint32(chapterID)

	var medalCount int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM commander_medal_displays
WHERE commander_id = $1
`, int64(commanderID)).Scan(&medalCount); err != nil {
		return nil, err
	}
	stats.MedalNumber = uint32(medalCount)

	var furnitureCount int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COALESCE(SUM(count), 0)
FROM commander_furnitures
WHERE commander_id = $1
`, int64(commanderID)).Scan(&furnitureCount); err != nil {
		return nil, err
	}
	stats.FurnitureNumber = uint32(furnitureCount)

	var shipNumTotal, shipNum120, shipNum125, marryNumber, love200Num, collectNum int64
	// collect_num counts distinct group_types, not `ship_id / 10`: a refit shares its
	// prototype's group, so /10 double-counts it as a separate collection entry.
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COUNT(*) AS ship_num_total,
	COUNT(*) FILTER (WHERE max_level >= 120) AS ship_num_120,
	COUNT(*) FILTER (WHERE max_level >= 125) AS ship_num_125,
	COUNT(*) FILTER (WHERE propose = TRUE) AS marry_number,
	COUNT(*) FILTER (WHERE intimacy >= $2) AS love200_num,
	COUNT(DISTINCT ships.group_type) AS collect_num
FROM owned_ships
INNER JOIN ships ON owned_ships.ship_id = ships.template_id
WHERE owner_id = $1
  AND deleted_at IS NULL
  AND ships.group_type IS NOT NULL
`, int64(commanderID), int64(summaryLove200Threshold)).Scan(&shipNumTotal, &shipNum120, &shipNum125, &marryNumber, &love200Num, &collectNum); err != nil {
		return nil, err
	}
	stats.ShipNumTotal = uint32(shipNumTotal)
	stats.ShipNum120 = uint32(shipNum120)
	stats.ShipNum125 = uint32(shipNum125)
	stats.MarryNumber = uint32(marryNumber)
	stats.Love200Num = uint32(love200Num)
	stats.CollectNum = uint32(collectNum)

	var characterID int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COALESCE((
	SELECT ship_id
	FROM owned_ships
	WHERE owner_id = $1
	  AND deleted_at IS NULL
	  AND is_secretary = TRUE
	ORDER BY COALESCE(secretary_position, 999), id
	LIMIT 1
), (
	SELECT ship_id
	FROM owned_ships
	WHERE owner_id = $1
	  AND deleted_at IS NULL
	ORDER BY id
	LIMIT 1
), 100001)
`, int64(commanderID)).Scan(&characterID); err != nil {
		return nil, err
	}
	stats.CharacterID = uint32(characterID)

	var firstLadyID int64
	var firstLadyName string
	var firstLadyTime int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT ship_id,
	COALESCE(NULLIF(custom_name, ''), ships.name, ''),
	EXTRACT(EPOCH FROM create_time)::bigint
FROM owned_ships
LEFT JOIN ships ON ships.template_id = owned_ships.ship_id
WHERE owner_id = $1
	AND deleted_at IS NULL
	AND propose = TRUE
ORDER BY create_time ASC, id ASC
LIMIT 1
`, int64(commanderID)).Scan(&firstLadyID, &firstLadyName, &firstLadyTime); err == nil {
		stats.FirstLadyID = uint32(firstLadyID)
		stats.FirstLadyName = firstLadyName
		stats.FirstLadyTime = uint32(firstLadyTime)
	} else if !errors.Is(err, pgx.ErrNoRows) && !db.IsNotFound(err) {
		return nil, err
	}

	var skinNum int64
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM owned_skins
WHERE commander_id = $1
  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
`, int64(commanderID)).Scan(&skinNum); err != nil {
		return nil, err
	}
	stats.SkinNum = uint32(skinNum)

	var skinShipNum int64
	// skins.ship_group lives in the same number space as group_type (verified against the
	// client tables), so matching it against `ship_id / 10` silently dropped every skin
	// belonging to a refit ship.
	if err := db.DefaultStore.Pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT owned_ships.id)
FROM owned_ships
INNER JOIN ships ON ships.template_id = owned_ships.ship_id
JOIN skins ON skins.ship_group = ships.group_type
JOIN owned_skins ON owned_skins.commander_id = owned_ships.owner_id
      AND owned_skins.skin_id = skins.id
      AND (owned_skins.expires_at IS NULL OR owned_skins.expires_at > CURRENT_TIMESTAMP)
WHERE owned_ships.owner_id = $1
  AND owned_ships.deleted_at IS NULL
  AND ships.group_type IS NOT NULL
`, int64(commanderID)).Scan(&skinShipNum); err != nil {
		return nil, err
	}
	stats.SkinShipNum = uint32(skinShipNum)

	return stats, nil
}
