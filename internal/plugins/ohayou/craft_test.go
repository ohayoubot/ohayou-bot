package ohayou

import (
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/plugins/ohayou/seed"
	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// TestCatalogConsistency guards against drift between the crafting recipes,
// the hard-coded balance constants, and the shipped item catalog.
func TestCatalogConsistency(t *testing.T) {
	items, err := seed.LoadItems("../../../data/items.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	byName := make(map[string]store.Item, len(items))
	for _, it := range items {
		byName[it.Name] = it
	}

	// Every recipe output and every item input must exist in the catalog.
	for _, r := range recipeList {
		if _, ok := byName[r.name]; !ok {
			t.Errorf("recipe %q has no matching catalog item", r.name)
		}
		for in := range r.items {
			if _, ok := byName[in]; !ok {
				t.Errorf("recipe %q input %q is not in the catalog", r.name, in)
			}
		}
	}

	// The cat cap constant must match the catalog acrelimit.
	if cat := byName["cat"]; cat.Acrelimit != catsPerAcre {
		t.Errorf("cat acrelimit %d != catsPerAcre %d", cat.Acrelimit, catsPerAcre)
	}

	// Buildings must carry a passive bonus; intermediates must not.
	buildings := map[string]bool{"workshop": true, "factory": true, "refinery": true}
	for name, isBuilding := range buildings {
		if byName[name].Add <= 0 {
			t.Errorf("building %q should have a positive add", name)
		}
		if isBuilding && byName[name].Purchase {
			t.Errorf("building %q should be built, not purchasable", name)
		}
	}
}

func TestMissingResources(t *testing.T) {
	rec := recipes["factory"]
	u := &store.User{
		Items:  map[string]int{"gear": 5},
		Quarry: store.Quarry{Metals: map[string]int{}},
	}
	got := missingResources(u, rec)
	// factory needs 20 gear (have 5 -> short 15), 10 circuit, 5 steelplate.
	for _, want := range []string{"15 gear", "10 circuit", "5 steelplate"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q not reported in %q", want, got)
		}
	}

	full := &store.User{
		Items:  map[string]int{"gear": 20, "circuit": 10, "steelplate": 5},
		Quarry: store.Quarry{Metals: map[string]int{}},
	}
	if got := missingResources(full, rec); got != "" {
		t.Errorf("expected nothing missing, got %q", got)
	}
}

func TestRecipeCost(t *testing.T) {
	got := recipeCost(recipes["gear"])
	if got != "2 iron" {
		t.Errorf("gear recipe cost = %q, want %q", got, "2 iron")
	}
}
