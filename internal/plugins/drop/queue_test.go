package drop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeQueue answers the D1 http api with whatever rows a test sets.
type fakeQueue struct {
	mu     sync.Mutex
	rows   []map[string]any
	newest int64
	status int
	sql    []string
}

func (f *fakeQueue) start(t *testing.T) *queue {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SQL    string   `json:"sql"`
			Params []string `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		f.mu.Lock()
		f.sql = append(f.sql, body.SQL)
		status, rows := f.status, f.rows
		if strings.Contains(body.SQL, "MAX(id)") {
			rows = []map[string]any{{"id": f.newest}}
		} else if len(body.Params) > 0 {
			rows = after(rows, body.Params[0])
		}
		f.mu.Unlock()

		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []map[string]any{{"results": rows}},
		})
	}))
	t.Cleanup(srv.Close)
	return newQueue(srv.URL, "acct", "db", "token", 5*time.Second)
}

// after mimics "WHERE id > ?" so the fake behaves like the real table.
func after(rows []map[string]any, cursor string) []map[string]any {
	from, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return rows
	}
	var out []map[string]any
	for _, row := range rows {
		id, _ := strconv.ParseInt(fmt.Sprint(row["id"]), 10, 64)
		if id > from {
			out = append(out, row)
		}
	}
	return out
}

func (f *fakeQueue) set(rows ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
}

func (f *fakeQueue) statements() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sql...)
}

func TestQueueSinceReadsRows(t *testing.T) {
	fake := &fakeQueue{}
	q := fake.start(t)
	fake.set(
		map[string]any{"id": 7, "nick": "mallow", "channel": "#chan", "key": "abc.png"},
		map[string]any{"id": 8, "nick": "svaj", "channel": "#Other", "key": "def.gif"},
	)

	rows, err := q.since(context.Background(), 6, 20)
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].ID != 7 || rows[0].Nick != "mallow" || rows[0].Channel != "#chan" || rows[0].Key != "abc.png" {
		t.Errorf("first row = %+v", rows[0])
	}
}

// The cursor is bound as text, so the comparison must be cast rather than left
// to affinity: without it "10" sorts before "9".
func TestQueueSinceCastsTheCursor(t *testing.T) {
	fake := &fakeQueue{}
	q := fake.start(t)

	if _, err := q.since(context.Background(), 9, 20); err != nil {
		t.Fatalf("since: %v", err)
	}
	sql := fake.statements()[0]
	if !strings.Contains(sql, "CAST(?1 AS INTEGER)") {
		t.Errorf("query does not cast the cursor: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY id") {
		t.Errorf("query is not ordered: %s", sql)
	}
}

func TestQueueNewest(t *testing.T) {
	fake := &fakeQueue{newest: 42}
	q := fake.start(t)

	got, err := q.newest(context.Background())
	if err != nil {
		t.Fatalf("newest: %v", err)
	}
	if got != 42 {
		t.Errorf("newest = %d, want 42", got)
	}
}

func TestQueueNewestOnAnEmptyTable(t *testing.T) {
	fake := &fakeQueue{newest: 0}
	q := fake.start(t)

	if got, err := q.newest(context.Background()); err != nil || got != 0 {
		t.Errorf("newest = %d, %v; want 0", got, err)
	}
}

func TestQueueSurfacesErrors(t *testing.T) {
	fake := &fakeQueue{status: http.StatusUnauthorized}
	q := fake.start(t)

	if _, err := q.since(context.Background(), 0, 20); err == nil {
		t.Error("a 401 from d1 was not reported")
	}
}
