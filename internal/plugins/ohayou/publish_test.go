package ohayou

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
	"github.com/ohayoubot/ohayou-bot/internal/web"
)

const publishSecret = "0123456789abcdef0123456789abcdef"

// publishedTable is one call the site received.
type publishedTable struct {
	Table string            `json:"table"`
	Rows  []json.RawMessage `json:"rows"`
}

// fakeSite records what the bot published, the way the worker would.
type fakeSite struct {
	server *httptest.Server

	mu   sync.Mutex
	got  []publishedTable
	fail bool
}

func newSite(t *testing.T) *fakeSite {
	t.Helper()
	s := &fakeSite{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body publishedTable
		_ = json.NewDecoder(r.Body).Decode(&body)

		s.mu.Lock()
		failing := s.fail
		if !failing {
			s.got = append(s.got, body)
		}
		s.mu.Unlock()

		if failing {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"published","rows":` + itoa(len(body.Rows)) + `}`))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func (s *fakeSite) calls() []publishedTable {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]publishedTable(nil), s.got...)
}

func (s *fakeSite) table(t *testing.T, name string) publishedTable {
	t.Helper()
	for _, call := range s.calls() {
		if call.Table == name {
			return call
		}
	}
	t.Fatalf("%s was never published; got %+v", name, s.calls())
	return publishedTable{}
}

// publishingGame is a game wired to a fake site, with the catalog seeded.
func publishingGame(t *testing.T) (*Plugin, *sqlite.DB, *fakeSite) {
	t.Helper()
	g, db := testGame(t)
	plotCatalog(t, db)

	site := newSite(t)
	g.feed = web.NewPublisher(site.server.URL, publishSecret).For("ohayou")
	return g, db, site
}

// player creates a user at the given visibility, with an acre and some cats.
func player(t *testing.T, db *sqlite.DB, nick, account string, v store.Visibility) {
	t.Helper()
	ctx := context.Background()
	if err := db.CreateUser(ctx, nick, 100); err != nil {
		t.Fatalf("create %s: %v", nick, err)
	}
	if account != "" {
		if err := db.SetAccount(ctx, nick, account); err != nil {
			t.Fatalf("account %s: %v", nick, err)
		}
	}
	if err := db.SetVisibility(ctx, nick, v); err != nil {
		t.Fatalf("visibility %s: %v", nick, err)
	}
}

// Everyone is on the map; only those who said so are named on it.
func TestEveryoneIsOnTheMapButOnlySomeAreNamed(t *testing.T) {
	g, db, site := publishingGame(t)

	player(t, db, "yes", "YesAcct", store.VisibilityPublic)
	player(t, db, "no", "NoAcct", store.VisibilityHidden)
	player(t, db, "unasked", "UnaskedAcct", store.VisibilityUnset)
	// Agreed, but never identified, so there is no identity to publish under.
	player(t, db, "nameless", "", store.VisibilityPublic)

	g.publish(context.Background())

	world := site.table(t, tablePlot)
	if len(world.Rows) != 4 {
		t.Fatalf("the map holds %d plots, want all four: %s", len(world.Rows), world.Rows)
	}

	var named int
	for _, raw := range world.Rows {
		body := string(raw)
		if strings.Contains(body, `"named":true`) {
			named++
			if !strings.Contains(body, "YesAcct") {
				t.Errorf("a named plot is not the consenting one: %s", body)
			}
			continue
		}
		// Everything about who this is must be absent, not merely blank.
		for _, absent := range []string{"NoAcct", "UnaskedAcct", "nameless", "no", "unasked"} {
			if strings.Contains(body, `"nick":"`+absent+`"`) {
				t.Errorf("an unnamed plot names %q: %s", absent, body)
			}
		}
	}
	if named != 1 {
		t.Errorf("%d named plots, want 1", named)
	}

	// The private tier is consent-gated outright.
	private := site.table(t, tablePlotPrivate)
	if len(private.Rows) != 1 {
		t.Fatalf("the private tier holds %d rows, want only the consenting one: %s",
			len(private.Rows), private.Rows)
	}
	if !strings.Contains(string(private.Rows[0]), "YesAcct") {
		t.Errorf("the private row is not the consenting player's: %s", private.Rows[0])
	}
}

// Withdrawing consent takes the name off the map and the private row with it,
// rather than merely stopping the refresh.
func TestOptingOutUnnamesThePlot(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	g.publish(ctx)
	if n := len(site.table(t, tablePlotPrivate).Rows); n != 1 {
		t.Fatalf("the private tier held %d rows to begin with", n)
	}

	if err := db.SetVisibility(ctx, "yes", store.VisibilityHidden); err != nil {
		t.Fatal(err)
	}
	g.publish(ctx)

	world := lastCall(site, tablePlot)
	if len(world.Rows) != 1 {
		t.Fatalf("the plot vanished instead of being unnamed: %s", world.Rows)
	}
	body := string(world.Rows[0])
	if strings.Contains(body, "YesAcct") || strings.Contains(body, `"nick":"yes"`) {
		t.Errorf("the plot still names its owner: %s", body)
	}
	if !strings.Contains(body, `"named":false`) {
		t.Errorf("the plot is still named: %s", body)
	}

	if private := lastCall(site, tablePlotPrivate); len(private.Rows) != 0 {
		t.Errorf("the private row survived opting out: %s", private.Rows)
	}
}

// A flag is a chosen picture, which is as identifying as a nick.
func TestAnUnnamedPlotFliesNoFlag(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "quiet", "QuietAcct", store.VisibilityUnset)
	if err := db.SetFlag(ctx, "quiet", "senordeer"); err != nil {
		t.Fatal(err)
	}

	g.publish(ctx)

	body := string(site.table(t, tablePlot).Rows[0])
	if strings.Contains(body, "senordeer") {
		t.Errorf("an unnamed plot published its flag: %s", body)
	}
}

func TestANamedPlotFliesItsFlag(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "loud", "LoudAcct", store.VisibilityPublic)
	if err := db.SetFlag(ctx, "loud", "senordeer"); err != nil {
		t.Fatal(err)
	}

	g.publish(ctx)

	body := string(site.table(t, tablePlot).Rows[0])
	if !strings.Contains(body, `"flag":"senordeer"`) {
		t.Errorf("a named plot lost its flag: %s", body)
	}
}

// An id that moved would shuffle somebody's plot across the map on a restart.
func TestAnonymousIDsAreStableAndUnguessable(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "quiet", "QuietAcct", store.VisibilityUnset)

	g.publish(ctx)
	first := string(site.table(t, tablePlot).Rows[0])

	// A fresh plugin over the same store: the salt comes back from the store
	// rather than being generated again.
	g2 := testGameOn(t, db)
	g2.feed = g.feed
	g2.publish(ctx)

	if second := lastCall(site, tablePlot); len(second.Rows) != 1 ||
		string(second.Rows[0]) != first {
		t.Errorf("the id moved across a restart:\n  %s\n  %s", first, second.Rows)
	}

	// It must not be the account, the nick, or anything derived from them
	// without the salt.
	for _, guess := range []string{"quiet", "QuietAcct"} {
		if strings.Contains(first, `"id":"`+guess+`"`) {
			t.Errorf("the id is just %q: %s", guess, first)
		}
	}
}

func TestAnonymousIDsDifferPerPlayer(t *testing.T) {
	g, db, site := publishingGame(t)
	player(t, db, "one", "OneAcct", store.VisibilityUnset)
	player(t, db, "two", "TwoAcct", store.VisibilityUnset)

	g.publish(context.Background())

	rows := site.table(t, tablePlot).Rows
	if len(rows) != 2 {
		t.Fatalf("%d plots", len(rows))
	}
	if string(rows[0]) == string(rows[1]) {
		t.Errorf("two players share a plot: %s", rows[0])
	}
}

// lastCall is the most recent publish of one table.
func lastCall(site *fakeSite, table string) publishedTable {
	var last publishedTable
	for _, call := range site.calls() {
		if call.Table == table {
			last = call
		}
	}
	return last
}

// A quiet channel should cost nothing: the projection is compared, not sent.
func TestAnUnchangedProjectionIsNotPublishedAgain(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	g.publish(ctx)
	first := len(site.calls())
	if first != 2 {
		t.Fatalf("%d calls for the first publish, want one per table", first)
	}

	g.publish(ctx)
	g.publish(ctx)
	if got := len(site.calls()); got != first {
		t.Errorf("%d calls after two idle rounds, want %d", got, first)
	}
}

func TestAChangedProjectionIsPublishedAgain(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	g.publish(ctx)
	before := len(site.calls())

	item, err := db.GetItem(ctx, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddItem(ctx, "yes", *item, 3); err != nil {
		t.Fatal(err)
	}
	g.publish(ctx)

	if got := len(site.calls()); got <= before {
		t.Errorf("%d calls after a purchase, want more than %d", got, before)
	}
}

// A publish that failed must be retried, not remembered as done.
func TestAFailedPublishIsRetried(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	site.mu.Lock()
	site.fail = true
	site.mu.Unlock()
	g.publish(ctx)
	if n := len(site.calls()); n != 0 {
		t.Fatalf("the failing site recorded %d calls", n)
	}

	site.mu.Lock()
	site.fail = false
	site.mu.Unlock()
	g.publish(ctx)

	if len(site.calls()) != 2 {
		t.Errorf("after recovery the projection was not re-sent: %+v", site.calls())
	}
}

// A bot nobody has played yet still says so, rather than saying nothing.
func TestAnEmptyWorldStillPublishes(t *testing.T) {
	g, _, site := publishingGame(t)

	g.publish(context.Background())

	got := site.table(t, tablePlot)
	if len(got.Rows) != 0 {
		t.Errorf("published %s, want an empty table", got.Rows)
	}
}

// A running quarry is the most useful thing the site can say, and it belongs
// only to the player it is running for.
func TestRunningActivitiesReachThePrivateTier(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	if err := g.tasks.After(ctx, taskMining, "yes", 4*time.Hour, "1"); err != nil {
		t.Fatal(err)
	}
	// Housekeeping, which is the bot's business and not a countdown.
	if err := g.tasks.After(ctx, taskPolice, "yes", time.Hour, ""); err != nil {
		t.Fatal(err)
	}
	g.publish(ctx)

	body := string(site.table(t, tablePlotPrivate).Rows[0])
	if !strings.Contains(body, taskMining) {
		t.Errorf("the private tier does not mention the quarry run: %s", body)
	}
	if strings.Contains(body, taskPolice) {
		t.Errorf("the private tier counts down a housekeeping task: %s", body)
	}

	if public := string(site.table(t, tablePlot).Rows[0]); strings.Contains(public, taskMining) {
		t.Errorf("the public tier leaked a running activity: %s", public)
	}
}

// With no site there is nothing to publish to, and nothing should try.
func TestNoFeedPublishesNothing(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)
	player(t, db, "yes", "YesAcct", store.VisibilityPublic)

	g.feed = nil
	g.startPublishing(context.Background())
	g.publish(context.Background()) // must not panic
}

// The fake site above agrees with the bot by construction. This one does not:
// it runs the real projection against the real worker, which is the only thing
// that proves the allowlist in ingest.js accepts what publicPlot produces.
//
//	cd web && pnpm db init ohayou && pnpm exec wrangler pages dev
//	OHAYOU_LIVE_SITE=http://localhost:8788 go test ./internal/plugins/ohayou/ -run Live -v
func TestLiveSiteAcceptsTheProjection(t *testing.T) {
	url := os.Getenv("OHAYOU_LIVE_SITE")
	if url == "" {
		t.Skip("set OHAYOU_LIVE_SITE to run this against a real worker")
	}

	ctx := context.Background()
	g, db := testGame(t)
	plotCatalog(t, db)
	player(t, db, "mallow", "Mallow", store.VisibilityPublic)
	player(t, db, "quiet", "QuietAcct", store.VisibilityUnset)

	item, err := db.GetItem(ctx, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddItem(ctx, "mallow", *item, 5); err != nil {
		t.Fatal(err)
	}
	if err := g.tasks.After(ctx, taskMining, "mallow", 4*time.Hour, "1"); err != nil {
		t.Fatal(err)
	}

	// The dev secret from web/.dev.vars.example.
	g.feed = web.NewPublisher(url, "local-development-only-not-a-real-secret").For("ohayou")

	public, private, err := g.projection(ctx)
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	for table, rows := range map[string]any{
		tablePlot:        public,
		tablePlotPrivate: private,
	} {
		result, err := g.feed.Publish(ctx, table, rows)
		if err != nil {
			t.Fatalf("the live site refused %s: %v", table, err)
		}
		want := map[string]int{tablePlot: 2, tablePlotPrivate: 1}[table]
		if !result.Published() || result.Rows != want {
			t.Errorf("%s: %+v", table, result)
		}
	}
}
