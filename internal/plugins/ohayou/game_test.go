package ohayou

import (
	"testing"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func TestStealFineAndAmount(t *testing.T) {
	u := &store.User{Ohayous: 1000}
	if got := stealFine(u); got != 5+160 {
		t.Errorf("stealFine = %d, want %d", got, 165)
	}
	if got := stealAmount(u); got != 70 {
		t.Errorf("stealAmount = %d, want 70", got)
	}

	poor := &store.User{Ohayous: 0}
	if got := stealFine(poor); got != 5 {
		t.Errorf("stealFine(0) = %d, want 5 (the minimum)", got)
	}
}

func TestUserDefense(t *testing.T) {
	u := &store.User{Equipped: map[string]store.Item{
		"head": {Name: "helmet", Defense: 9},
		"body": {Name: "vest", Defense: 18},
	}}
	if got := userDefense(u); got != 27 {
		t.Errorf("userDefense = %d, want 27", got)
	}
	if got := userDefense(&store.User{Equipped: map[string]store.Item{}}); got != 0 {
		t.Errorf("userDefense(none) = %d, want 0", got)
	}
}

func TestVaultCap(t *testing.T) {
	cases := map[int]int{0: 1000, 1: 10000, 2: 100000}
	for level, want := range cases {
		if got := vaultCap(level); got != want {
			t.Errorf("vaultCap(%d) = %d, want %d", level, got, want)
		}
	}
}

func TestRandNumBounds(t *testing.T) {
	for i := 0; i < 1000; i++ {
		if n := randNum(0, 6); n < 0 || n > 6 {
			t.Fatalf("randNum(0,6) = %d, out of range", n)
		}
	}
	if n := randNum(5, 5); n != 5 {
		t.Errorf("randNum(5,5) = %d, want 5", n)
	}
	if n := randNum(9, 3); n != 9 {
		t.Errorf("randNum(9,3) = %d, want 9 (min when max<=min)", n)
	}
}

func TestAmountsDescAndNames(t *testing.T) {
	counts := map[string]int{"a": 3, "b": 1, "c": 3, "d": 0}
	amounts := amountsDesc(counts)
	want := []int{3, 1, 0}
	if len(amounts) != len(want) {
		t.Fatalf("amountsDesc = %v, want %v", amounts, want)
	}
	for i := range want {
		if amounts[i] != want[i] {
			t.Fatalf("amountsDesc = %v, want %v", amounts, want)
		}
	}

	names := namesWithAmount(counts, 3)
	if len(names) != 2 {
		t.Errorf("namesWithAmount(3) = %v, want 2 entries", names)
	}
}

func TestFormatItemLine(t *testing.T) {
	acre := store.Item{Name: "quarry", Desc: "a mine", Price: 500, Acrelimit: 1}
	if got := formatItemLine(acre); got != "quarry: a mine - Price: 500 ohayous. Limited to 1 per acre." {
		t.Errorf("acre-limited line = %q", got)
	}
	plain := store.Item{Name: "cat", Desc: "a cat", Price: 12}
	if got := formatItemLine(plain); got != "cat - 12 ohayous - a cat" {
		t.Errorf("plain line = %q", got)
	}
	part := store.Item{Name: "gear", Desc: "a mechanical gear"}
	if got := formatItemLine(part); got != "gear - not for sale, must be crafted - a mechanical gear" {
		t.Errorf("craftable line = %q", got)
	}
	building := store.Item{Name: "workshop", Desc: "a workshop", Acrelimit: 5}
	want := "workshop: a workshop - Not for sale, must be crafted. Limited to 5 per acre."
	if got := formatItemLine(building); got != want {
		t.Errorf("craftable acre-limited line = %q", got)
	}
}
