package game

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

func testGame(t *testing.T) (*Game, *sqlite.DB) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}
	g := &Game{
		store:      db,
		log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		est:        time.UTC,
		baseCtx:    context.Background(),
		identified: map[string]bool{},
	}
	return g, db
}

// A negative deposit or withdraw must be rejected outright, or "!deposit
// -1000000" would mint ohayous via a negative vault transfer.
func TestDepositWithdrawRejectNonPositive(t *testing.T) {
	g, _ := testGame(t)
	u := &store.User{Username: "alice", Vault: store.Vault{Installed: true}}
	for _, amt := range []int{0, -1, -1000000} {
		if msg := g.deposit(u, amt); !strings.Contains(msg, "positive") {
			t.Errorf("deposit(%d) = %q, want a rejection", amt, msg)
		}
		if msg := g.withdraw(u, amt); !strings.Contains(msg, "positive") {
			t.Errorf("withdraw(%d) = %q, want a rejection", amt, msg)
		}
	}
}

// Depositing then withdrawing the same amount is balance-neutral, and the
// vault's once-per-day lock still applies (a second op the same day is denied).
func TestDepositWithdrawRoundTrip(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 500); err != nil {
		t.Fatal(err)
	}
	if err := db.InstallVault(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "alice")

	if msg := g.deposit(u, 100); !strings.Contains(msg, "deposited 100") {
		t.Fatalf("deposit: %q", msg)
	}
	u, _ = db.GetUser(ctx, "alice")
	if u.Ohayous != 400 || u.Vault.Ohayous != 100 {
		t.Fatalf("after deposit: ohayous=%d vault=%d, want 400/100", u.Ohayous, u.Vault.Ohayous)
	}
	// Same day: the vault is locked.
	if msg := g.withdraw(u, 100); !strings.Contains(msg, "once today") &&
		!strings.Contains(msg, "security") {
		t.Fatalf("expected once-per-day lock, got %q", msg)
	}
}

// buyCatalog seeds the minimal item set the buy tests need.
func buyCatalog(t *testing.T, db *sqlite.DB) {
	t.Helper()
	items := []store.Item{
		{Name: "acre", Desc: "land", Price: 250, Purchase: true, Category: "land"},
		{Name: "cat", Desc: "a cat", Price: 12, Add: 1, Acrelimit: 20, Purchase: true, Category: "animals"},
		{Name: "quarry", Desc: "mine", Price: 3000, Acrelimit: 1, NeedsAcre: true, Purchase: true, Category: "industry"},
		{Name: "oilwell", Desc: "well", Price: 1200, Acrelimit: 1, NeedsAcre: true, Purchase: true, Category: "industry"},
	}
	if _, err := db.SeedItems(context.Background(), items); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// A quantity so large that price*amt overflows int64 must be rejected, not
// silently wrapped negative into a balance-minting "purchase".
func TestBuyRejectsOverflowQuantity(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	buyCatalog(t, db)
	if err := db.CreateUser(ctx, "alice", 300); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "alice")

	msg := g.buy(u, "acre", 40000000000000000) // 4e16 * 250 overflows int64
	if !strings.Contains(msg, "afford") {
		t.Fatalf("buy(huge) = %q, want an affordability rejection", msg)
	}
	after, _ := db.GetUser(ctx, "alice")
	if after.Ohayous != 300 {
		t.Fatalf("balance changed to %d after a rejected buy, want 300", after.Ohayous)
	}
	if after.Items["acre"] != 1 { // unchanged from the starting acre
		t.Fatalf("acre count changed to %d on a rejected buy, want 1", after.Items["acre"])
	}
}

// freeAcre must sum the acres consumed across all acre-limited item types. With
// a full plot (cats on one acre, a quarry on the other) there is no room for a
// second own-acre item, even though no single type fills the whole plot alone.
func TestFreeAcreSumsAcrossItemTypes(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	buyCatalog(t, db)
	if err := db.CreateUser(ctx, "bob", 100000); err != nil {
		t.Fatal(err)
	}
	// Two acres, fully occupied: 20 cats (1 acre) + 1 quarry (1 acre).
	cat, _ := db.GetItem(ctx, "cat")
	acre, _ := db.GetItem(ctx, "acre")
	quarry, _ := db.GetItem(ctx, "quarry")
	if err := db.AddItem(ctx, "bob", *acre, 1); err != nil { // now 2 acres
		t.Fatal(err)
	}
	if err := db.AddItem(ctx, "bob", *cat, 20); err != nil {
		t.Fatal(err)
	}
	if err := db.AddItem(ctx, "bob", *quarry, 1); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "bob")

	if g.freeAcre(u, 1) {
		t.Fatal("freeAcre reported room on a full 2-acre plot")
	}
	if msg := g.buy(u, "oilwell", 1); !strings.Contains(msg, "empty acre") {
		t.Fatalf("buy oilwell = %q, want an empty-acre rejection", msg)
	}
}

// Many buys fired off the same stale snapshot must not overspend. Every
// goroutine sees the full 100-ohayou balance and passes its own affordability
// check, but the atomic debit caps real spend at the balance: no negative
// balance, and cats granted always match ohayous actually spent.
func TestBuyConcurrentDoesNotOverspend(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	buyCatalog(t, db)
	if err := db.CreateUser(ctx, "alice", 100); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "alice") // one snapshot shared by all workers

	const workers = 20
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.buy(u, "cat", 1) // 12 ohayous each; only 8 fit in 100
		}()
	}
	wg.Wait()

	after, _ := db.GetUser(ctx, "alice")
	if after.Ohayous < 0 {
		t.Fatalf("balance went negative: %d", after.Ohayous)
	}
	if spent := 100 - after.Ohayous; spent != after.Items["cat"]*12 {
		t.Fatalf("spent %d ohayous but hold %d cats (want spend == cats*12)", spent, after.Items["cat"])
	}
	if after.Items["cat"] != 8 { // floor(100/12)
		t.Fatalf("cats = %d, want 8", after.Items["cat"])
	}
}

// cum_ohayous must grow by the ration earned, not by the whole balance added
// to the cumulative total every greeting.
func TestOhayouCumulativeTracksGainOnly(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()

	// Seed a returning player (CreateUser sets cum == starting balance), then
	// clear "last" so another greeting is allowed today.
	if err := db.CreateUser(ctx, "alice", 12); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetLast(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	before, _ := db.GetUser(ctx, "alice")

	g.Ohayou("alice")
	after, _ := db.GetUser(ctx, "alice")

	gain := after.Ohayous - before.Ohayous
	cumDelta := after.CumOhayous - before.CumOhayous
	if gain < 0 || gain > 6 {
		t.Fatalf("ration gain = %d, expected 0..6", gain)
	}
	if cumDelta != gain {
		t.Errorf("cum grew by %d, want %d (the ration only)", cumDelta, gain)
	}
}
