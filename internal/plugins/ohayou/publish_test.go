package ohayou

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
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
	Mode  string            `json:"mode"`
	Keep  int               `json:"keep"`
	Rows  []json.RawMessage `json:"rows"`
}

// fakeSite records what the bot published, the way the worker would: a replace
// swaps the table, an append adds to it and trims the oldest away.
type fakeSite struct {
	server *httptest.Server

	mu   sync.Mutex
	got  []publishedTable
	held map[string]map[int64]json.RawMessage
	fail bool
	// short answers with a table one row shorter than it is, which is what the
	// two ends drifting apart looks like from here.
	short bool
}

func newSite(t *testing.T) *fakeSite {
	t.Helper()
	s := &fakeSite{held: map[string]map[int64]json.RawMessage{}}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body publishedTable
		_ = json.NewDecoder(r.Body).Decode(&body)

		s.mu.Lock()
		failing := s.fail
		total := 0
		if !failing {
			s.got = append(s.got, body)
			total = s.apply(body)
			if s.short {
				total--
			}
		}
		s.mu.Unlock()

		if failing {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":"error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"status":"published","rows":%d,"total":%d}`, len(body.Rows), total)))
	}))
	t.Cleanup(s.server.Close)
	return s
}

// apply writes the rows the way ingest.js would and returns what the table
// holds afterwards. Called with the lock held.
func (s *fakeSite) apply(body publishedTable) int {
	if body.Mode != "append" || s.held[body.Table] == nil {
		s.held[body.Table] = map[int64]json.RawMessage{}
	}
	rows := s.held[body.Table]
	for i, raw := range body.Rows {
		rows[rowID(raw, int64(i))] = raw
	}

	// The trim, newest ids kept.
	if body.Mode == "append" && body.Keep > 0 && len(rows) > body.Keep {
		ids := make([]int64, 0, len(rows))
		for id := range rows {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(a, b int) bool { return ids[a] > ids[b] })
		for _, id := range ids[body.Keep:] {
			delete(rows, id)
		}
	}
	return len(rows)
}

// rowID is a row's id, or fallback for a table that has none.
func rowID(raw json.RawMessage, fallback int64) int64 {
	var row struct {
		ID *int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &row); err == nil && row.ID != nil {
		return *row.ID
	}
	return fallback
}

// holds is how many rows of a table the site is left with.
func (s *fakeSite) holds(table string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.held[table])
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

// Everyone is on the map. A plot carries its owner's nick unless they opted
// out, or the bot never learned an account to publish it under.
func TestEveryoneIsOnTheMapAndOnlyTheOptedOutAreAnonymous(t *testing.T) {
	g, db, site := publishingGame(t)

	player(t, db, "yes", "YesAcct", store.VisibilityPublic)
	player(t, db, "no", "NoAcct", store.VisibilityHidden)
	// Never asked, which publishes: opting out is a thing you do.
	player(t, db, "unasked", "UnaskedAcct", store.VisibilityUnset)
	// Willing, but never identified, so there is no identity to publish under.
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
			continue
		}
		// Everything about who this is must be absent, not merely blank.
		if !strings.Contains(body, `"nick":""`) {
			t.Errorf("an anonymous plot carries a nick: %s", body)
		}
		for _, absent := range []string{"NoAcct", "no"} {
			if strings.Contains(body, `"nick":"`+absent+`"`) {
				t.Errorf("an anonymous plot names %q: %s", absent, body)
			}
		}
	}
	if named != 3 {
		t.Errorf("%d named plots, want everybody but the one who opted out", named)
	}

	// The private tier needs an account as well: it is matched against a
	// session rather than displayed, and a nick is not an identity.
	private := site.table(t, tablePlotPrivate)
	if len(private.Rows) != 2 {
		t.Fatalf("the private tier holds %d rows, want the two with accounts: %s",
			len(private.Rows), private.Rows)
	}
	for _, raw := range private.Rows {
		body := string(raw)
		if strings.Contains(body, "NoAcct") {
			t.Errorf("the opted out player has a private row: %s", body)
		}
		if strings.Contains(body, `"nick":"nameless"`) {
			t.Errorf("a player with no account has a private row: %s", body)
		}
	}
}

// A nick is a display name. It must never become the key a session is resolved
// against, or holding the nick would be holding the plot.
func TestANamedPlotWithNoAccountIsNotKeyedOnItsNick(t *testing.T) {
	g, db, site := publishingGame(t)
	player(t, db, "nameless", "", store.VisibilityUnset)

	g.publish(context.Background())

	body := string(site.table(t, tablePlot).Rows[0])
	if !strings.Contains(body, `"nick":"nameless"`) {
		t.Fatalf("the plot lost its holder's name: %s", body)
	}
	if strings.Contains(body, `"id":"nameless"`) {
		t.Errorf("the id is the nick, which anybody can take: %s", body)
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
func TestAnAnonymousPlotFliesNoFlag(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "quiet", "QuietAcct", store.VisibilityHidden)
	if err := db.SetFlag(ctx, "quiet", "senordeer"); err != nil {
		t.Fatal(err)
	}

	g.publish(ctx)

	body := string(site.table(t, tablePlot).Rows[0])
	if strings.Contains(body, "senordeer") {
		t.Errorf("an anonymous plot published its flag: %s", body)
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
	player(t, db, "quiet", "QuietAcct", store.VisibilityHidden)

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
	player(t, db, "one", "OneAcct", store.VisibilityHidden)
	player(t, db, "two", "TwoAcct", store.VisibilityHidden)

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
	if first != publishedTables {
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

	if len(site.calls()) != publishedTables {
		t.Errorf("after recovery the projection was not re-sent: %+v", site.calls())
	}
}

// event writes one line of the chronicle and returns the game's view of it.
func chronicled(t *testing.T, db *sqlite.DB, kind, actor, subject string) {
	t.Helper()
	e := store.Event{TS: time.Now(), Kind: kind, Actor: actor, Subject: subject}
	if err := db.RecordEvent(context.Background(), e, eventLog); err != nil {
		t.Fatalf("record: %v", err)
	}
}

// Adding a line to the chronicle should cost a line, not the whole feed. This
// is most of what the site is asked to write, so it is most of the budget.
func TestANewEntryIsAppendedRatherThanTheWholeChronicle(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "mallow", "MallowAcct", store.VisibilityPublic)
	chronicled(t, db, eventCat, "mallow", "")

	g.publish(ctx)
	if first := lastCall(site, tableEvent); first.Mode != "" || len(first.Rows) != 1 {
		t.Fatalf("the first publish was %q with %d rows, want the whole feed",
			first.Mode, len(first.Rows))
	}

	chronicled(t, db, eventCat, "mallow", "")
	g.publish(ctx)

	got := lastCall(site, tableEvent)
	if got.Mode != "append" || len(got.Rows) != 1 {
		t.Errorf("sent %q with %d rows, want one appended", got.Mode, len(got.Rows))
	}
	if got.Keep != eventFeed {
		t.Errorf("asked the site to keep %d, want %d", got.Keep, eventFeed)
	}
	if held := site.holds(tableEvent); held != 2 {
		t.Errorf("the site holds %d entries, want both", held)
	}
}

// Withdrawing a name rewrites every entry that carried it, which an append
// cannot do: the whole feed has to go again.
func TestWithdrawingANameRepublishesTheWholeChronicle(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "mallow", "MallowAcct", store.VisibilityPublic)
	chronicled(t, db, eventCat, "mallow", "")

	g.publish(ctx)
	if err := db.SetVisibility(ctx, "mallow", store.VisibilityHidden); err != nil {
		t.Fatal(err)
	}
	g.publish(ctx)

	got := lastCall(site, tableEvent)
	if got.Mode != "" {
		t.Errorf("sent %q, want the whole feed", got.Mode)
	}
	if len(got.Rows) != 1 || strings.Contains(string(got.Rows[0]), "mallow") {
		t.Errorf("the republished entry still names them: %s", got.Rows)
	}
}

// If the site does not hold what we think it holds, stop appending to it.
func TestASiteHoldingSomethingElseIsSentTheWholeChronicle(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "mallow", "MallowAcct", store.VisibilityPublic)
	chronicled(t, db, eventCat, "mallow", "")

	site.mu.Lock()
	site.short = true
	site.mu.Unlock()
	g.publish(ctx)

	chronicled(t, db, eventCat, "mallow", "")
	g.publish(ctx)

	if got := lastCall(site, tableEvent); got.Mode != "" || len(got.Rows) != 2 {
		t.Errorf("sent %q with %d rows, want the whole feed", got.Mode, len(got.Rows))
	}
}

// A quiet chronicle is not sent again, appended or otherwise.
func TestAnUnchangedChronicleIsNotSentAgain(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "mallow", "MallowAcct", store.VisibilityPublic)
	chronicled(t, db, eventCat, "mallow", "")

	g.publish(ctx)
	before := len(site.calls())
	g.publish(ctx)
	g.publish(ctx)

	if got := len(site.calls()); got != before {
		t.Errorf("%d calls after two idle rounds, want %d", got, before)
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
