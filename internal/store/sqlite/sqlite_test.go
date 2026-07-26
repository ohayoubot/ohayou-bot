package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	return db
}

func seedCatalog(t *testing.T, db *DB) {
	t.Helper()
	items := []store.Item{
		{Name: "acre", Desc: "land", Price: 10, Purchase: true, Category: "land", Acrelimit: 0},
		{Name: "cat", Desc: "a cat", Price: 12, Add: 1, Purchase: true, Category: "animal"},
		{Name: "quarry", Desc: "mine", Price: 500, Purchase: true, Category: "industry", NeedsAcre: true},
		{Name: "oilbarrel", Desc: "crude oil", Price: 0, Category: "resources"},
		{Name: "helmet", Desc: "armor", Price: 50, Defense: 9, Purchase: true, Category: "armor", EquipCategory: "head"},
		{Name: "fertilizer", Desc: "boosts cats", Price: 30, Multiplies: "cat", Multiply: 2, Purchase: true, Category: "boost"},
	}
	n, err := db.SeedItems(context.Background(), items)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n != len(items) {
		t.Fatalf("seeded %d, want %d", n, len(items))
	}
	// Re-seeding syncs the catalog: it rewrites every item rather than skipping
	// which is what lets edited prices take effect.
	if n2, err := db.SeedItems(context.Background(), items); err != nil || n2 != len(items) {
		t.Fatalf("re-seed = (%d,%v), want (%d,nil)", n2, err, len(items))
	}
}

func TestSeedItemsUpdatesPrices(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)

	// A user buys a cat at the original price, then the catalog price changes.
	if _, err := db.SeedItems(ctx, []store.Item{
		{Name: "cat", Desc: "a cat", Price: 99, Add: 1, Purchase: true, Category: "animal"},
	}); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	it, err := db.GetItem(ctx, "cat")
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if it.Price != 99 {
		t.Errorf("cat price = %d, want 99", it.Price)
	}
}

func TestCreateAndGetUser(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)

	if _, err := db.GetUser(ctx, "nobody"); err != store.ErrNotFound {
		t.Fatalf("GetUser(missing) = %v, want ErrNotFound", err)
	}

	if err := db.CreateUser(ctx, "alice", 12); err != nil {
		t.Fatalf("create: %v", err)
	}
	u, err := db.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if u.Ohayous != 12 || u.CumOhayous != 12 || u.TimesOhayoued != 1 {
		t.Errorf("unexpected new user: %+v", u)
	}
	if u.Items["acre"] != 1 {
		t.Errorf("acre = %d, want 1", u.Items["acre"])
	}
	if u.Items == nil || u.Equipped == nil || u.Status == nil || u.Quarry.Metals == nil {
		t.Error("maps must be non-nil after load")
	}
}

func TestAddItemAndMultiplier(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "bob", 1000)

	cat, _ := db.GetItem(ctx, "cat")
	if err := db.AddItem(ctx, "bob", *cat, 3); err != nil {
		t.Fatalf("addItem: %v", err)
	}
	fert, _ := db.GetItem(ctx, "fertilizer")
	if err := db.AddItem(ctx, "bob", *fert, 1); err != nil {
		t.Fatalf("addItem fert: %v", err)
	}

	u, _ := db.GetUser(ctx, "bob")
	if u.Items["cat"] != 3 {
		t.Errorf("cat = %d, want 3", u.Items["cat"])
	}
	if u.Items["fertilizer"] != 1 {
		t.Errorf("fertilizer = %d, want 1", u.Items["fertilizer"])
	}
	if u.ItemMultiply["cat"] != 2 {
		t.Errorf("cat multiplier = %d, want 2", u.ItemMultiply["cat"])
	}
	// 1000 - (12*3) - (30*1) = 934
	if u.Ohayous != 934 {
		t.Errorf("ohayous = %d, want 934", u.Ohayous)
	}
}

func TestQuarryPurchase(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "carol", 4000)

	quarry, _ := db.GetItem(ctx, "quarry")
	if err := db.AddItem(ctx, "carol", *quarry, 1); err != nil {
		t.Fatalf("buy quarry: %v", err)
	}

	u, _ := db.GetUser(ctx, "carol")
	// Quarries are ordinary items. Buying one does not consume an acre.
	if u.Items["acre"] != 1 {
		t.Errorf("acre = %d, want 1 (unchanged)", u.Items["acre"])
	}
	if u.Items["quarry"] != 1 {
		t.Errorf("quarry count = %d, want 1", u.Items["quarry"])
	}
	if u.Ohayous != 4000-quarry.Price {
		t.Errorf("ohayous = %d, want %d", u.Ohayous, 4000-quarry.Price)
	}
}

func TestBuildConsumesResources(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "ivan", 1000)

	// Give ivan raw resources: 30 iron (metals) and 4 oil barrels (items).
	if err := db.AddMetals(ctx, "ivan", map[string]int{"iron": 30}); err != nil {
		t.Fatalf("add metals: %v", err)
	}
	oil, _ := db.GetItem(ctx, "oilbarrel")
	if err := db.AddItem(ctx, "ivan", *oil, 4); err != nil {
		t.Fatalf("add oil: %v", err)
	}

	// Craft 3 gears (2 iron each) then a workshop (10 gears... only have 3, so
	// exercise a valid partial: build 1 gear, assert consumption).
	if err := db.Build(ctx, "ivan", map[string]int{"iron": 2}, nil, 0, "gear", 1); err != nil {
		t.Fatalf("build gear: %v", err)
	}
	// Build plastic from 2 oil barrels plus a 100-ohayou fee.
	if err := db.Build(ctx, "ivan", nil, map[string]int{"oilbarrel": 2}, 100, "plastic", 1); err != nil {
		t.Fatalf("build plastic: %v", err)
	}

	u, _ := db.GetUser(ctx, "ivan")
	if u.Quarry.Metals["iron"] != 28 {
		t.Errorf("iron = %d, want 28", u.Quarry.Metals["iron"])
	}
	if u.Items["gear"] != 1 {
		t.Errorf("gear = %d, want 1", u.Items["gear"])
	}
	if u.Items["oilbarrel"] != 2 {
		t.Errorf("oilbarrel = %d, want 2", u.Items["oilbarrel"])
	}
	if u.Items["plastic"] != 1 {
		t.Errorf("plastic = %d, want 1", u.Items["plastic"])
	}
	if u.Ohayous != 900 {
		t.Errorf("ohayous = %d, want 900", u.Ohayous)
	}
}

func TestIncVaultLevel(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "jane", 100)

	if err := db.InstallVault(ctx, "jane"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := db.IncVaultLevel(ctx, "jane"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if err := db.IncVaultLevel(ctx, "jane"); err != nil {
		t.Fatalf("upgrade 2: %v", err)
	}
	u, _ := db.GetUser(ctx, "jane")
	if u.Vault.Level != 2 {
		t.Errorf("vault level = %d, want 2", u.Vault.Level)
	}
}

func TestEquipUnequip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "dave", 1000)

	helmet, _ := db.GetItem(ctx, "helmet")
	if err := db.Equip(ctx, "dave", *helmet); err != nil {
		t.Fatalf("equip: %v", err)
	}
	u, _ := db.GetUser(ctx, "dave")
	if got := u.Equipped["head"]; got.Name != "helmet" || got.Defense != 9 {
		t.Errorf("equipped head = %+v, want helmet/9", got)
	}
	if err := db.Unequip(ctx, "dave", "head"); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	u, _ = db.GetUser(ctx, "dave")
	if _, ok := u.Equipped["head"]; ok {
		t.Error("head slot should be empty after unequip")
	}
}

func TestSuccessSteal(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "thief", 100)
	mustCreate(t, db, "victim", 200)
	cat, _ := db.GetItem(ctx, "cat")
	if err := db.AddItem(ctx, "victim", *cat, 2); err != nil {
		t.Fatalf("give victim cats: %v", err)
	}

	if err := db.SaveSuccessSteal(ctx, "thief", "victim", 1, 25); err != nil {
		t.Fatalf("steal: %v", err)
	}
	th, _ := db.GetUser(ctx, "thief")
	vi, _ := db.GetUser(ctx, "victim")
	if th.Ohayous != 125 || th.Items["cat"] != 1 || th.StealSuccess != 1 || th.StolenOhayous != 25 {
		t.Errorf("thief after steal: %+v", th)
	}
	// victim: 200 - (12*2 spent) = 176, then -25 stolen = 151; cats 2-1=1
	if vi.Ohayous != 151 || vi.Items["cat"] != 1 || vi.StolenFrom != 1 || vi.OhayousStolen != 25 {
		t.Errorf("victim after steal: ohayous=%d cats=%d stolenFrom=%d ohStolen=%d",
			vi.Ohayous, vi.Items["cat"], vi.StolenFrom, vi.OhayousStolen)
	}
}

func TestFailStealAndProbation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "eve", 100)
	pro := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	if err := db.SaveFailSteal(ctx, "eve", 20, pro); err != nil {
		t.Fatalf("failsteal: %v", err)
	}
	u, _ := db.GetUser(ctx, "eve")
	if u.Ohayous != 80 || u.StealFail != 1 || u.ProbationCount != 1 {
		t.Errorf("eve after fail: %+v", u)
	}
	if !u.Probation.Equal(pro) {
		t.Errorf("probation = %v, want %v", u.Probation, pro)
	}
}

func TestVaultTransferAndStatus(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "frank", 500)

	if err := db.InstallVault(ctx, "frank"); err != nil {
		t.Fatalf("install vault: %v", err)
	}
	now := time.Now().Truncate(time.Second)
	day := now.Truncate(24 * time.Hour)
	if err := db.VaultTransfer(ctx, "frank", -100, 100, now, day); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	u, _ := db.GetUser(ctx, "frank")
	if !u.Vault.Installed || u.Vault.Ohayous != 100 || u.Ohayous != 400 {
		t.Errorf("vault state: installed=%v vaultOh=%d ohayous=%d", u.Vault.Installed, u.Vault.Ohayous, u.Ohayous)
	}

	if err := db.SetStatus(ctx, "frank", "mining", true); err != nil {
		t.Fatalf("set status: %v", err)
	}
	u, _ = db.GetUser(ctx, "frank")
	if !u.Status["mining"] {
		t.Error("mining status should be true")
	}
	if err := db.ResetAllStatus(ctx); err != nil {
		t.Fatalf("reset status: %v", err)
	}
	u, _ = db.GetUser(ctx, "frank")
	if u.Status["mining"] {
		t.Error("mining status should be cleared after ResetAllStatus")
	}
}

func TestMetalsAndCategories(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "grace", 100)

	if err := db.AddMetals(ctx, "grace", map[string]int{"iron": 5, "gold": 1}); err != nil {
		t.Fatalf("addmetals: %v", err)
	}
	if err := db.AddMetals(ctx, "grace", map[string]int{"iron": 3}); err != nil {
		t.Fatalf("addmetals 2: %v", err)
	}
	u, _ := db.GetUser(ctx, "grace")
	if u.Quarry.Metals["iron"] != 8 || u.Quarry.Metals["gold"] != 1 {
		t.Errorf("metals = %+v, want iron:8 gold:1", u.Quarry.Metals)
	}

	cats, err := db.Categories(ctx)
	if err != nil {
		t.Fatalf("categories: %v", err)
	}
	if len(cats) == 0 {
		t.Error("expected some categories")
	}

	armor, err := db.ItemsByCategory(ctx, "armor")
	if err != nil {
		t.Fatalf("bycategory: %v", err)
	}
	if len(armor) != 1 || armor[0].Name != "helmet" {
		t.Errorf("armor category = %+v", armor)
	}
}

func TestTop(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "low", 5)
	mustCreate(t, db, "high", 500)
	mustCreate(t, db, "mid", 50)

	top, err := db.Top(ctx, 2)
	if err != nil {
		t.Fatalf("top: %v", err)
	}
	if len(top) != 2 || top[0].Username != "high" || top[1].Username != "mid" {
		t.Errorf("top = %+v", top)
	}
}

func TestAddItemGuardsBalance(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "al", 10) // can't afford a 12-ohayou cat

	cat, _ := db.GetItem(ctx, "cat")
	if err := db.AddItem(ctx, "al", *cat, 1); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("AddItem over budget = %v, want ErrInsufficient", err)
	}
	u, _ := db.GetUser(ctx, "al")
	if u.Ohayous != 10 || u.Items["cat"] != 0 {
		t.Fatalf("state changed on a guarded-out buy: ohayous=%d cats=%d", u.Ohayous, u.Items["cat"])
	}
}

func TestBuildRollsBackOnShortfall(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "bo", 1000)
	if err := db.AddMetals(ctx, "bo", map[string]int{"iron": 2}); err != nil {
		t.Fatal(err)
	}

	// Needs 5 iron, has 2. every must roll back. no iron spent, no ohayous spent, no output
	err := db.Build(ctx, "bo", map[string]int{"iron": 5}, nil, 100, "steelplate", 1)
	if !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("Build short on iron = %v, want ErrInsufficient", err)
	}
	u, _ := db.GetUser(ctx, "bo")
	if u.Quarry.Metals["iron"] != 2 || u.Ohayous != 1000 || u.Items["steelplate"] != 0 {
		t.Fatalf("build not rolled back: iron=%d ohayous=%d steelplate=%d",
			u.Quarry.Metals["iron"], u.Ohayous, u.Items["steelplate"])
	}
}

func TestVaultTransferGuards(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "vi", 500)
	if err := db.InstallVault(ctx, "vi"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	day := now.Truncate(24 * time.Hour)

	if err := db.VaultTransfer(ctx, "vi", -100, 100, now, day); err != nil {
		t.Fatalf("first deposit: %v", err)
	}
	// Second op the same day is locked out atomically (vault_last >= day).
	if err := db.VaultTransfer(ctx, "vi", -50, 50, now, day); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("same-day second op = %v, want ErrInsufficient", err)
	}
	// and an overdraw on a fresh day is refused (vault only holds 100).
	tomorrow := day.Add(24 * time.Hour)
	if err := db.VaultTransfer(ctx, "vi", 200, -200, tomorrow, tomorrow); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("overdraw = %v, want ErrInsufficient", err)
	}
	u, _ := db.GetUser(ctx, "vi")
	if u.Ohayous != 400 || u.Vault.Ohayous != 100 {
		t.Fatalf("balances moved on guarded-out transfers: ohayous=%d vault=%d", u.Ohayous, u.Vault.Ohayous)
	}
}

func TestSaveOhayouOncePerDay(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "od", 12)
	if err := db.ResetLast(ctx, "od"); err != nil { // allow a fresh collect
		t.Fatal(err)
	}
	now := time.Now()
	day := now.Truncate(24 * time.Hour)

	if err := db.SaveOhayou(ctx, "od", 18, 6, now, day); err != nil {
		t.Fatalf("first collect: %v", err)
	}
	// a second greeting the same day is guarded out (last is now today).
	if err := db.SaveOhayou(ctx, "od", 24, 6, now, day); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("second same-day collect = %v, want ErrInsufficient", err)
	}
	u, _ := db.GetUser(ctx, "od")
	if u.Ohayous != 18 || u.TimesOhayoued != 2 { // 1 from create, 1 from the single collect
		t.Fatalf("double collect leaked through: ohayous=%d times=%d", u.Ohayous, u.TimesOhayoued)
	}
}

func TestSuccessStealConservesValue(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "rob", 100)
	mustCreate(t, db, "mark", 10) // only 10 on hand

	// a stale snapshot said 25 was takeable. the guard refuses to overdraw the
	// victim, and the whole tx rolls back so the thief gains nothing
	if err := db.SaveSuccessSteal(ctx, "rob", "mark", 0, 25); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("overdrawing steal = %v, want ErrInsufficient", err)
	}
	th, _ := db.GetUser(ctx, "rob")
	vi, _ := db.GetUser(ctx, "mark")
	if th.Ohayous != 100 || th.StealSuccess != 0 || vi.Ohayous != 10 {
		t.Fatalf("value not conserved: thief=%d (success %d) victim=%d",
			th.Ohayous, th.StealSuccess, vi.Ohayous)
	}
}

func TestRemoveCatGuardsCount(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	seedCatalog(t, db)
	mustCreate(t, db, "alice", 0)

	// nobody owns a cat yet, so there's nothing for a dog to kill
	if err := db.RemoveCat(ctx, "alice", 1); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("RemoveCat with no cats = %v, want ErrInsufficient", err)
	}

	if err := db.AddCat(ctx, "alice", 2); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveCat(ctx, "alice", 1); err != nil {
		t.Fatalf("RemoveCat: %v", err)
	}
	if err := db.RemoveCat(ctx, "alice", 5); !errors.Is(err, store.ErrInsufficient) {
		t.Fatalf("RemoveCat(5) of 1 = %v, want ErrInsufficient", err)
	}
	u, _ := db.GetUser(ctx, "alice")
	if u.Items["cat"] != 1 {
		t.Errorf("cats = %d, want 1", u.Items["cat"])
	}
}

func mustCreate(t *testing.T, db *DB, nick string, ohayous int) {
	t.Helper()
	if err := db.CreateUser(context.Background(), nick, ohayous); err != nil {
		t.Fatalf("create %s: %v", nick, err)
	}
}
