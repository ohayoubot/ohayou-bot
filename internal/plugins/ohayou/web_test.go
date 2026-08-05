package ohayou

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/store/sqlite"
)

// plotCatalog covers one of each thing a plot can hold: land, an animal that
// occupies it, industry that needs its own acre, a defensive animal, armor that
// takes no room, and a material that is not on the map at all.
func plotCatalog(t *testing.T, db *sqlite.DB) {
	t.Helper()
	items := []store.Item{
		{Name: "acre", Desc: "land", Price: 250, Purchase: true, Category: "land"},
		{Name: "cat", Desc: "a cat", Price: 12, Add: 1, Acrelimit: 20, Purchase: true, Category: "animals"},
		{Name: "dog", Desc: "a dog", Price: 500, Add: 3, Acrelimit: 1, Defense: 12, Purchase: true, Category: "animals"},
		{Name: "quarry", Desc: "mine", Price: 3000, Acrelimit: 1, NeedsAcre: true, Purchase: true, Category: "industry"},
		{Name: "helmet", Desc: "armor", Price: 50, Defense: 36, Purchase: true, Category: "armor", EquipCategory: "head"},
		{Name: "gear", Desc: "a gear", Category: "materials"},
	}
	if _, err := db.SeedItems(context.Background(), items); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// richUser is a player holding one of everything, so a projection that carries
// something along has something to carry.
func richUser() *store.User {
	return &store.User{
		Username:   "alice",
		Account:    "AliceAcct",
		Web:        store.VisibilityPublic,
		Ohayous:    4200,
		CumOhayous: 30000,
		Items: map[string]int{
			"acre": 6, "cat": 25, "dog": 2, "quarry": 1, "helmet": 1, "gear": 9,
		},
		Quarry:        store.Quarry{Metals: map[string]int{"iron": 40}},
		Equipped:      map[string]store.Item{"head": {Name: "helmet", Defense: 36}},
		Vault:         store.Vault{Installed: true, Level: 1, Ohayous: 9000},
		Fortune:       "a fine day",
		TimesOhayoued: 120,
		Probation:     time.Now().Add(time.Hour),
		StealSuccess:  3,
	}
}

func keysOf(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// The public row's field list is the boundary between the game and the
// internet. A field added to store.User must fail here rather than ship.
func TestPublicPlotCarriesOnlyWhatWasPromised(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	got := keysOf(t, g.publicPlot(richUser()))
	want := []string{"acres", "id", "land", "named", "nick", "rations", "wealth"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("public plot fields = %v, want %v", got, want)
	}
}

// !web says the public tier never shows a balance, a vault or defences. This is
// that promise, checked against the bytes rather than the field names.
// An unnamed plot is the same shape with the identifying half left out, so the
// site cannot tell one kind from the other by which fields arrived.
func TestAnonymousPlotIsTheSameShapeWithoutTheName(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	named := keysOf(t, g.publicPlot(richUser()))
	anonymous := keysOf(t, g.anonymousPlot(richUser(), "opaque"))
	if strings.Join(named, ",") != strings.Join(anonymous, ",") {
		t.Errorf("fields differ: named %v, anonymous %v", named, anonymous)
	}

	plot := g.anonymousPlot(richUser(), "opaque")
	if plot.Named {
		t.Error("an anonymous plot says it is named")
	}
	if plot.ID != "opaque" || plot.Nick != "" {
		t.Errorf("id = %q, nick = %q, want the opaque id and no nick", plot.ID, plot.Nick)
	}
	if len(plot.Land) != 0 {
		t.Errorf("land = %v, want nothing: a distinctive set of buildings names you", plot.Land)
	}
	// The scale is the point of being on the map at all.
	if plot.Acres != 6 || plot.Wealth != "industrialist" || plot.Rations != 120 {
		t.Errorf("the scale was lost: %+v", plot)
	}
}

// The account and nick are what an anonymous plot exists not to say.
func TestAnonymousPlotNamesNobody(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	raw, err := json.Marshal(g.anonymousPlot(richUser(), "opaque"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"alice", "AliceAcct", "cat", "quarry"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("an anonymous plot contains %q: %s", secret, raw)
		}
	}
}

func TestPublicPlotLeaksNoBalanceVaultOrDefence(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	raw, err := json.Marshal(g.publicPlot(richUser()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, secret := range []string{"4200", "9000", "dog", "helmet", "iron", "fine day"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("public plot contains %q: %s", secret, raw)
		}
	}
}

// The dog is left off the map, and the acres it sits on are not accounted for
// anywhere either: an observer must not be able to subtract their way to it.
func TestPublicPlotHidesDefensiveAnimals(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	plot := g.publicPlot(richUser())
	if _, ok := plot.Land["dog"]; ok {
		t.Error("the dog is on the public map")
	}
	if plot.Land["cat"] != 25 || plot.Land["quarry"] != 1 {
		t.Errorf("land = %v, want the cats and the quarry", plot.Land)
	}
	// helmet takes no room and gear is not land, so neither is on the map.
	if _, ok := plot.Land["helmet"]; ok {
		t.Error("armor is on the map")
	}
	if _, ok := plot.Land["gear"]; ok {
		t.Error("a material is on the map")
	}
}

func TestWealthIsABandNotABalance(t *testing.T) {
	for _, tc := range []struct {
		cumulative int
		want       string
	}{
		{0, "newcomer"},
		{499, "newcomer"},
		{500, "settler"},
		{9999, "landowner"},
		{50000, "magnate"},
		{1 << 40, "tycoon"},
	} {
		if got := wealth(tc.cumulative); got != tc.want {
			t.Errorf("wealth(%d) = %q, want %q", tc.cumulative, got, tc.want)
		}
	}
}

func TestPrivatePlotIsTheOwnersFullPicture(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	due := time.Now().Add(4 * time.Hour)
	got := g.privatePlot(richUser(), map[string]time.Time{taskMining: due})

	if got.Ohayous != 4200 || got.Cumulative != 30000 {
		t.Errorf("ohayous = %d, cumulative = %d", got.Ohayous, got.Cumulative)
	}
	if got.Vault == nil || got.Vault.Ohayous != 9000 || got.Vault.Level != 2 {
		t.Errorf("vault = %+v, want level 2 holding 9000", got.Vault)
	}
	if got.Equipped["head"] != "helmet" || got.Defense == 0 {
		t.Errorf("equipped = %v, defense = %d", got.Equipped, got.Defense)
	}
	if got.Metals["iron"] != 40 {
		t.Errorf("metals = %v, want 40 iron", got.Metals)
	}
	if len(got.Running) != 1 || got.Running[0].Kind != taskMining || got.Running[0].Due != due.Unix() {
		t.Errorf("running = %+v, want one mining run due %d", got.Running, due.Unix())
	}
	if got.Probation == 0 {
		t.Error("probation still in force but not reported")
	}
}

// Probation that has already expired is not something to count down to.
func TestPrivatePlotDropsSpentProbation(t *testing.T) {
	g, db := testGame(t)
	plotCatalog(t, db)

	u := richUser()
	u.Probation = time.Now().Add(-time.Hour)
	if got := g.privatePlot(u, nil); got.Probation != 0 {
		t.Errorf("probation = %d, want 0 once it has run out", got.Probation)
	}
}

func TestPublishableNeedsConsentAndAnAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		visible store.Visibility
		account string
		want    bool
	}{
		{"public with an account", store.VisibilityPublic, "AliceAcct", true},
		{"public but never identified", store.VisibilityPublic, "", false},
		{"never asked", store.VisibilityUnset, "AliceAcct", false},
		{"opted out", store.VisibilityHidden, "AliceAcct", false},
	} {
		u := &store.User{Web: tc.visible, Account: tc.account}
		if got := publishable(u); got != tc.want {
			t.Errorf("%s: publishable = %v, want %v", tc.name, got, tc.want)
		}
	}
}
