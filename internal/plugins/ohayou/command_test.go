package ohayou

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/d1"
	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

// queueOf stands in for the site's command table.
func queueOf(t *testing.T, rows ...command) *d1.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SQL string `json:"sql"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		results := any(rows)
		if len(body.SQL) > 6 && body.SQL[:20] == "SELECT COALESCE(MAX(" {
			results = []map[string]int64{{"id": 0}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"result":  []map[string]any{{"results": results}},
		})
	}))
	t.Cleanup(server.Close)
	return d1.New(server.URL, "acct", "db", "token", 0)
}

func commandGame(t *testing.T, rows ...command) (*Plugin, *sqlite.DB) {
	t.Helper()
	g, db := testGame(t)
	plotCatalog(t, db)
	g.commands = queueOf(t, rows...)
	return g, db
}

// A request from the website reaches the store through the same methods irc
// does, so every guard there still stands.
func TestACommandChangesTheFlag(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t, command{ID: 1, Account: "MalAcct", Kind: "flag", Value: "senordeer"})
	player(t, db, "mallow", "MalAcct", store.VisibilityPublic)

	g.pollCommands(ctx)

	u, err := db.GetUser(ctx, "mallow")
	if err != nil {
		t.Fatal(err)
	}
	if u.Flag != "senordeer" {
		t.Errorf("flag = %q, want senordeer", u.Flag)
	}
}

func TestACommandChangesVisibility(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t, command{ID: 1, Account: "MalAcct", Kind: "territory", Value: "on"})
	player(t, db, "mallow", "MalAcct", store.VisibilityUnset)

	g.pollCommands(ctx)

	u, _ := db.GetUser(ctx, "mallow")
	if u.Web != store.VisibilityPublic {
		t.Errorf("visibility = %q, want public", u.Web)
	}
}

// The authority check the site cannot make: an account nobody has proved owns
// nothing to change.
func TestACommandForAnUnknownAccountIsDropped(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t, command{ID: 1, Account: "Stranger", Kind: "flag", Value: "senordeer"})
	player(t, db, "mallow", "MalAcct", store.VisibilityPublic)

	g.pollCommands(ctx)

	u, _ := db.GetUser(ctx, "mallow")
	if u.Flag != "" {
		t.Errorf("a stranger's request changed mallow's flag to %q", u.Flag)
	}
}

// The site's list of kinds and the bot's are the same list twice. One naming
// something the other does not must do nothing at all.
func TestACommandOfAnUnknownKindIsDropped(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t,
		command{ID: 1, Account: "MalAcct", Kind: "buy", Value: "cat"},
		command{ID: 2, Account: "MalAcct", Kind: "ohayous", Value: "100000"},
	)
	player(t, db, "mallow", "MalAcct", store.VisibilityPublic)

	before, _ := db.GetUser(ctx, "mallow")
	g.pollCommands(ctx)
	after, _ := db.GetUser(ctx, "mallow")

	if after.Ohayous != before.Ohayous || after.Items["cat"] != before.Items["cat"] {
		t.Errorf("a request nobody taught it moved something: %d -> %d ohayous",
			before.Ohayous, after.Ohayous)
	}
}

// A restart must not replay what has already been applied.
func TestTheCursorSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t, command{ID: 7, Account: "MalAcct", Kind: "flag", Value: "first"})
	player(t, db, "mallow", "MalAcct", store.VisibilityPublic)

	g.pollCommands(ctx)
	if got, err := g.kv.Get(ctx, commandCursor); err != nil || got != "7" {
		t.Fatalf("cursor = %q (%v), want 7", got, err)
	}

	// A fresh plugin over the same store resumes past what it applied.
	g2 := testGameOn(t, db)
	g2.commands = queueOf(t)
	g2.pollCommands(ctx)
	if got, _ := g2.kv.Get(ctx, commandCursor); got != "7" {
		t.Errorf("cursor moved to %q on restart", got)
	}
}

// A bot switched on against a queue with requests already in it should not act
// on ones made before it was listening.
func TestAFreshBotStartsAtTheEndOfTheQueue(t *testing.T) {
	ctx := context.Background()
	g, db := commandGame(t)
	player(t, db, "mallow", "MalAcct", store.VisibilityPublic)

	g.pollCommands(ctx)
	if got, err := g.kv.Get(ctx, commandCursor); err != nil {
		t.Fatalf("no cursor was written: %v", err)
	} else if got != "0" {
		t.Errorf("cursor = %q, want the end of an empty queue", got)
	}
}

// No database configured means no listening, not a panic every poll.
func TestNoQueueIsNotAnError(t *testing.T) {
	g, _ := testGame(t)
	g.commands = nil
	g.startCommands(context.Background())
	g.pollCommands(context.Background())
}
