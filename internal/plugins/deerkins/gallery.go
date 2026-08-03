package deerkins

import (
	"context"
	"errors"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/d1"
)

var errNoDeer = errors.New("deerkins: no such deer")

// gallery is the read side of the art the web app writes.
type gallery struct {
	db *d1.Client
}

func newGallery(base, account, database, token string, timeout time.Duration) *gallery {
	return &gallery{db: d1.New(base, account, database, token, timeout)}
}

// row is one piece of art
type row struct {
	Deer     string `json:"deer"`
	Creator  string `json:"creator"`
	Date     string `json:"date"`
	Kinskode string `json:"kinskode"`
}

func (g *gallery) byName(ctx context.Context, name string) (*row, error) {
	return g.one(ctx, "SELECT deer, creator, date, kinskode FROM deer WHERE deer = ?1 LIMIT 1", name)
}

func (g *gallery) random(ctx context.Context) (*row, error) {
	return g.one(ctx, "SELECT deer, creator, date, kinskode FROM deer ORDER BY RANDOM() LIMIT 1")
}

func (g *gallery) latest(ctx context.Context) (*row, error) {
	return g.one(ctx, "SELECT deer, creator, date, kinskode FROM deer ORDER BY date DESC, id DESC LIMIT 1")
}

func (g *gallery) count(ctx context.Context) (int, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := g.db.Query(ctx, "SELECT COUNT(*) AS n FROM deer", nil, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

func (g *gallery) one(ctx context.Context, sql string, params ...string) (*row, error) {
	var rows []row
	if err := g.db.Query(ctx, sql, params, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNoDeer
	}
	return &rows[0], nil
}
