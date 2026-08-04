package ohayou

import (
	"context"
	"strings"
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func TestDogDefenseCapped(t *testing.T) {
	cases := map[int]int{0: 0, 1: 12, 3: 36, 10: 36}
	for dogs, want := range cases {
		u := &store.User{Items: map[string]int{"dog": dogs}}
		if got := dogDefense(u); got != want {
			t.Errorf("dogDefense(%d dogs) = %d, want %d", dogs, got, want)
		}
	}
}

func TestUserDefenseIncludesDogs(t *testing.T) {
	u := &store.User{
		Equipped: map[string]store.Item{"head": {Name: "helmet", Defense: 9}},
		Items:    map[string]int{"dog": 1},
	}
	if got := armorDefense(u); got != 9 {
		t.Errorf("armorDefense = %d, want 9", got)
	}
	if got := userDefense(u); got != 21 {
		t.Errorf("userDefense = %d, want 21 (9 armor + 12 dog)", got)
	}
}

func TestDogAttacksCat(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()

	if g.dogAttacksCat(&store.User{Username: "alice", Items: map[string]int{"cat": 5}}) {
		t.Error("a catless kennel attacked")
	}
	if g.dogAttacksCat(&store.User{Username: "alice", Items: map[string]int{"dog": 5}}) {
		t.Error("a dogless yard attacked")
	}

	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}
	const cats = 200
	if err := db.AddCat(ctx, "alice", cats); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "alice")
	u.Items["dog"] = 5 // the capped 25% chance, to land some kills

	var kills int
	for i := 0; i < cats; i++ {
		if g.dogAttacksCat(u) {
			kills++
		}
	}
	if kills == 0 {
		t.Fatal("no kills in 200 rolls at a 25% chance")
	}

	after, _ := db.GetUser(ctx, "alice")
	if after.Items["cat"] != cats-kills {
		t.Errorf("cats = %d after %d kills, want %d", after.Items["cat"], kills, cats-kills)
	}
	if u.Items["cat"] != after.Items["cat"] {
		t.Errorf("snapshot has %d cats, store has %d", u.Items["cat"], after.Items["cat"])
	}
}

func TestDogAttacksCatStopsAtZero(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "bob", 0); err != nil {
		t.Fatal(err)
	}
	if err := db.AddCat(ctx, "bob", 1); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "bob")
	u.Items["dog"] = 5

	for i := 0; i < 200; i++ {
		g.dogAttacksCat(u)
	}
	after, _ := db.GetUser(ctx, "bob")
	if after.Items["cat"] < 0 {
		t.Fatalf("cats went negative: %d", after.Items["cat"])
	}
}

func TestWalkDogOncePerDayAndPays(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}
	u, _ := db.GetUser(ctx, "alice")
	u.Items["dog"] = 1

	first := g.walkDog(u, "dog")
	if strings.Contains(first, "already") {
		t.Fatalf("first walk was refused: %q", first)
	}

	after, _ := db.GetUser(ctx, "alice")
	var dug int
	for _, amt := range after.Quarry.Metals {
		dug += amt
	}
	if strings.Contains(first, "digs up") != (dug > 0) {
		t.Errorf("walk said %q but the pile holds %d metals", first, dug)
	}

	after.Items["dog"] = 1
	if second := g.walkDog(after, "dog"); !strings.Contains(second, "already") {
		t.Errorf("second walk of the day = %q, want a refusal", second)
	}
}
