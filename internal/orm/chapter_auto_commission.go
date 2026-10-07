package orm

import (
	"context"
	"time"

	"github.com/ggmolly/belfast/internal/db"
)

// ChapterAutoCommission is one running "auto battle" (周回) job.
//
// Field semantics come from the client's model/vo/chapterauto/chapterautocommission.lua:
//
//	Ctor: type, id = configId, finishTime = time, ticketTime = ticket_time, costTime = seconds
//	GetStartTime() = finishTime - costTime
//	IsFinished()   = finishTime <= now
//	UsedTicket()   = ticketTime > 0
//
// All timestamps are absolute server seconds; the client never computes finishTime itself.
type ChapterAutoCommission struct {
	ID          int64
	CommanderID uint32
	Type        uint32
	ConfigID    uint32
	FinishTime  uint32
	TicketTime  uint32
	CostTime    uint32
	CreatedAt   time.Time
}

func ListChapterAutoCommissions(commanderID uint32) ([]ChapterAutoCommission, error) {
	ctx := context.Background()
	rows, err := db.DefaultStore.Pool.Query(ctx, `
SELECT id, commander_id, type, config_id, finish_time, ticket_time, cost_time, created_at
FROM chapter_auto_commissions
WHERE commander_id = $1
ORDER BY finish_time ASC, id ASC
`, int64(commanderID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChapterAutoCommission{}
	for rows.Next() {
		var c ChapterAutoCommission
		if err := rows.Scan(&c.ID, &c.CommanderID, &c.Type, &c.ConfigID, &c.FinishTime, &c.TicketTime, &c.CostTime, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertChapterAutoCommission appends one job and returns its new id.
func InsertChapterAutoCommission(commanderID, chapterType, configID, finishTime, ticketTime, costTime uint32) (int64, error) {
	ctx := context.Background()
	var id int64
	err := db.DefaultStore.Pool.QueryRow(ctx, `
INSERT INTO chapter_auto_commissions (
  commander_id, type, config_id, finish_time, ticket_time, cost_time
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id
`, int64(commanderID), int64(chapterType), int64(configID), int64(finishTime), int64(ticketTime), int64(costTime)).Scan(&id)
	return id, err
}

// DeleteChapterAutoCommissions removes jobs by id, scoped to the owner so a client
// cannot delete another commander's rows by guessing ids.
func DeleteChapterAutoCommissions(commanderID uint32, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	ctx := context.Background()
	_, err := db.DefaultStore.Pool.Exec(ctx, `
DELETE FROM chapter_auto_commissions
WHERE commander_id = $1 AND id = ANY($2)
`, int64(commanderID), ids)
	return err
}

// CountChapterAutoCommissions reports how many jobs are running, used to bound the queue.
func CountChapterAutoCommissions(commanderID uint32) (int, error) {
	ctx := context.Background()
	var count int
	err := db.DefaultStore.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chapter_auto_commissions WHERE commander_id = $1`,
		int64(commanderID)).Scan(&count)
	return count, err
}
