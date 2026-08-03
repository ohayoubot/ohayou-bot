package drop

import (
	"context"
	"strconv"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/d1"
)

// queue is the read side of the upload table the worker appends to. The bot
// never writes: its token carries D1:Read and nothing else.
type queue struct {
	db *d1.Client
}

func newQueue(base, account, database, token string, timeout time.Duration) *queue {
	return &queue{db: d1.New(base, account, database, token, timeout)}
}

// upload is one queued announcement.
type upload struct {
	ID      int64  `json:"id"`
	Nick    string `json:"nick"`
	Channel string `json:"channel"`
	Key     string `json:"key"`
}

// since returns uploads newer than the cursor, oldest first.
//
// The cursor is bound as text and cast, rather than left to sqlite's affinity
// rules, so the comparison is an integer one whatever the client sent.
func (q *queue) since(ctx context.Context, cursor int64, limit int) ([]upload, error) {
	var rows []upload
	err := q.db.Query(ctx,
		"SELECT id, nick, channel, key FROM upload WHERE id > CAST(?1 AS INTEGER) ORDER BY id LIMIT CAST(?2 AS INTEGER)",
		[]string{itoa(cursor), itoa(int64(limit))}, &rows)
	return rows, err
}

// newest is where a bot with no cursor starts: at the end, so switching the
// plugin on does not replay every upload ever made into the channels.
func (q *queue) newest(ctx context.Context) (int64, error) {
	var rows []struct {
		ID int64 `json:"id"`
	}
	if err := q.db.Query(ctx, "SELECT COALESCE(MAX(id), 0) AS id FROM upload", nil, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].ID, nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
