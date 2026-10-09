package answer

import (
	"context"

	"github.com/ggmolly/belfast/internal/connection"
	"github.com/ggmolly/belfast/internal/db"
	"github.com/ggmolly/belfast/internal/orm"
	"github.com/ggmolly/belfast/internal/protobuf"
	"google.golang.org/protobuf/proto"
)

type shipCollectionStat struct {
	GroupID     uint32 `gorm:"column:group_id"`
	MaxStar     uint32 `gorm:"column:max_star"`
	MaxIntimacy uint32 `gorm:"column:max_intimacy"`
	MaxLevel    uint32 `gorm:"column:max_level"`
	MarryFlag   uint32 `gorm:"column:marry_flag"`
	HeartFlag   uint32 `gorm:"column:heart_flag"`
	HeartCount  uint32 `gorm:"column:heart_count"`
}

func listCollectionShipStats(commanderID uint32) ([]shipCollectionStat, error) {
	// The collection is keyed by group_type, NOT by `ship_id / 10`. One group_type covers
	// several template ids (a refit shares its prototype's group), and the client resolves
	// every entry through pg.ship_data_group[id] -- an unknown id aborts the login-time
	// painting check. ships.group_type is filled from the mirrored Lua mapping; rows
	// without one are skipped rather than emitted as a group the client cannot look up.
	resultRows, err := db.DefaultStore.Pool.Query(context.Background(), `
SELECT
stats.group_id,
stats.max_star,
stats.max_intimacy,
stats.max_level,
stats.marry_flag,
(SELECT COUNT(*) FROM likes WHERE group_id = stats.group_id AND liker_id = $1) AS heart_flag,
(SELECT COUNT(*) FROM likes WHERE group_id = stats.group_id) AS heart_count
FROM (
	SELECT
		ships.group_type AS group_id,
		MAX(ships.star) AS max_star,
		MAX(intimacy) AS max_intimacy,
		MAX(level) AS max_level,
		MAX(CASE WHEN propose THEN 1 ELSE 0 END) AS marry_flag
	FROM owned_ships
	INNER JOIN ships ON owned_ships.ship_id = ships.template_id
	WHERE owner_id = $2
	  AND ships.group_type IS NOT NULL
	GROUP BY ships.group_type
) AS stats
`, int64(commanderID), int64(commanderID))
	if err != nil {
		return nil, err
	}
	defer resultRows.Close()

	rows := make([]shipCollectionStat, 0)
	for resultRows.Next() {
		var row shipCollectionStat
		if err := resultRows.Scan(&row.GroupID, &row.MaxStar, &row.MaxIntimacy, &row.MaxLevel, &row.MarryFlag, &row.HeartFlag, &row.HeartCount); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := resultRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func getCollectionShipStat(commanderID uint32, groupID uint32) (*shipCollectionStat, bool, error) {
	row := shipCollectionStat{}
	err := db.DefaultStore.Pool.QueryRow(context.Background(), `
SELECT
	ships.group_type AS group_id,
	MAX(ships.star) AS max_star,
	MAX(intimacy) AS max_intimacy,
	MAX(level) AS max_level,
	MAX(CASE WHEN propose THEN 1 ELSE 0 END) AS marry_flag,
	(SELECT COUNT(*) FROM likes WHERE group_id = $2 AND liker_id = $1) AS heart_flag,
	(SELECT COUNT(*) FROM likes WHERE group_id = $2) AS heart_count
FROM owned_ships
INNER JOIN ships ON owned_ships.ship_id = ships.template_id
WHERE owner_id = $1 AND ships.group_type = $2
GROUP BY ships.group_type
`, int64(commanderID), int64(groupID)).Scan(
		&row.GroupID,
		&row.MaxStar,
		&row.MaxIntimacy,
		&row.MaxLevel,
		&row.MarryFlag,
		&row.HeartFlag,
		&row.HeartCount,
	)
	err = db.MapNotFound(err)
	if err != nil {
		if db.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &row, true, nil
}

func buildShipStatisticsInfo(row shipCollectionStat) *protobuf.SHIP_STATISTICS_INFO {
	return &protobuf.SHIP_STATISTICS_INFO{
		Id:          proto.Uint32(row.GroupID),
		Star:        proto.Uint32(row.MaxStar),
		HeartFlag:   proto.Uint32(row.HeartFlag),
		HeartCount:  proto.Uint32(row.HeartCount),
		MarryFlag:   proto.Uint32(row.MarryFlag),
		IntimacyMax: proto.Uint32(row.MaxIntimacy),
		LvMax:       proto.Uint32(row.MaxLevel),
	}
}

func sendCollectionShipGroupUpdate(client *connection.Client, groupID uint32) (bool, error) {
	row, ok, err := getCollectionShipStat(client.Commander.CommanderID, groupID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	response := protobuf.SC_17004{ShipInfo: buildShipStatisticsInfo(*row)}
	if _, _, err := client.SendMessage(17004, &response); err != nil {
		return false, err
	}
	return true, nil
}

func buildAchievementInfo(row orm.CommanderTrophyProgress) *protobuf.ACHIEVEMENT_INFO {
	return &protobuf.ACHIEVEMENT_INFO{
		Id:        proto.Uint32(row.TrophyID),
		Progress:  proto.Uint32(row.Progress),
		Timestamp: proto.Uint32(row.Timestamp),
	}
}

func sendTrophyProgressUpdate(client *connection.Client, rows ...orm.CommanderTrophyProgress) (bool, error) {
	if len(rows) == 0 {
		return false, nil
	}

	seen := make(map[uint32]struct{}, len(rows))
	updates := make([]*protobuf.ACHIEVEMENT_INFO, 0, len(rows))
	for i := range rows {
		if rows[i].TrophyID == 0 {
			continue
		}
		if _, ok := seen[rows[i].TrophyID]; ok {
			continue
		}
		seen[rows[i].TrophyID] = struct{}{}
		updates = append(updates, buildAchievementInfo(rows[i]))
	}
	if len(updates) == 0 {
		return false, nil
	}

	response := protobuf.SC_17002{ProgressList: updates}
	if _, _, err := client.SendMessage(17002, &response); err != nil {
		return false, err
	}
	return true, nil
}
