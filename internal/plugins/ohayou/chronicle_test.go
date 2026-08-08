package ohayou

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func TestPhraseCoversEveryKindItRecords(t *testing.T) {
	kinds := []string{
		eventSettle, eventLand, eventBuild, eventStrike, eventSteal,
		eventCaught, eventCat, eventFlag, eventDouble,
	}
	for _, kind := range kinds {
		e := store.Event{Kind: kind, Actor: "alice", Subject: "bob", Detail: map[string]string{
			"acres": "4", "thing": "refinery", "metal": "gold",
			"took": "a purse of ohayous", "deer": "artbutt",
		}}
		if phrase(e) == "" {
			t.Errorf("%s has no words", kind)
		}
	}
	if phrase(store.Event{Kind: "invented-later"}) != "" {
		t.Error("a kind this bot does not know got a sentence")
	}
}

// A world event has no actor, and must not be attributed to one.
func TestAWorldEventNamesNobody(t *testing.T) {
	got := phrase(store.Event{Kind: eventDouble})
	if strings.Contains(got, "somebody") {
		t.Errorf("%q attributes the distributor to a player", got)
	}
}

func TestAnActorlessPlayerEventReadsAsSomebody(t *testing.T) {
	got := phrase(store.Event{Kind: eventBuild, Detail: map[string]string{"thing": "factory"}})
	if !strings.HasPrefix(got, "somebody ") {
		t.Errorf("%q, want an unnamed builder", got)
	}
}

// The whole point of a band: a haul must never be a number.
func TestABandIsNeverAnAmount(t *testing.T) {
	for _, n := range []int{0, 1, 24, 25, 99, 100, 499, 500, 1999, 2000, 999999} {
		if got := band(n); strings.ContainsAny(got, "0123456789") {
			t.Errorf("band(%d) = %q, which carries a number", n, got)
		}
	}
	if band(10) == band(10000) {
		t.Error("every haul is the same band")
	}
}

func TestTookNamesWhatWasTaken(t *testing.T) {
	cases := []struct {
		cat, ohy int
		want     string
	}{
		{0, 300, "a purse of ohayous"},
		{1, 0, "a cat"},
		{1, 300, "a cat and a purse of ohayous"},
	}
	for _, c := range cases {
		if got := took(c.cat, c.ohy); got != c.want {
			t.Errorf("took(%d, %d) = %q, want %q", c.cat, c.ohy, got, c.want)
		}
	}
}

func TestAgoReadsInOneUnit(t *testing.T) {
	cases := []struct {
		since time.Duration
		want  string
	}{
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := ago(c.since); got != c.want {
			t.Errorf("ago(%s) = %q, want %q", c.since, got, c.want)
		}
	}
}

func TestPackFillsLinesAndStopsAtTheLimit(t *testing.T) {
	now := time.Now()
	var events []store.Event
	for i := 0; i < 200; i++ {
		events = append(events, store.Event{
			ID: int64(i), TS: now, Kind: eventCat, Actor: "alice",
		})
	}

	lines := pack(events, now, 3, 200)
	if len(lines) != 3 {
		t.Fatalf("%d lines, want the 3 asked for", len(lines))
	}
	for _, line := range lines {
		if len(line) > 200 {
			t.Errorf("a line ran to %d characters: %q", len(line), line)
		}
	}
}

// Nothing this bot has words for still has to answer.
func TestPackAlwaysSaysSomething(t *testing.T) {
	lines := pack([]store.Event{{Kind: "invented-later"}}, time.Now(), 3, 200)
	if len(lines) != 1 || lines[0] == "" {
		t.Errorf("pack over unreadable events gave %q", lines)
	}
}

// The feed must say as much about a player as the map does, and no more.
func TestTheFeedLeavesAHiddenPlayerUnnamed(t *testing.T) {
	g, _ := testGame(t)
	vis := map[string]store.Visibility{
		"alice": store.VisibilityPublic,
		"bob":   store.VisibilityHidden,
	}
	events := []store.Event{
		{ID: 3, Kind: eventSteal, Actor: "alice", Subject: "bob"},
		{ID: 2, Kind: eventBuild, Actor: "bob", Detail: map[string]string{"thing": "factory"}},
		{ID: 1, Kind: eventCat, Actor: "alice"},
	}

	got := g.chronicle(events, vis)
	if len(got) != 3 {
		t.Fatalf("%d events published, want all three", len(got))
	}
	if got[0].Actor != "alice" || got[0].Subject != "" {
		t.Errorf("the robbery names the hidden victim: %+v", got[0])
	}
	if got[1].Actor != "" {
		t.Errorf("the hidden builder is named: %+v", got[1])
	}
	if got[2].Actor != "alice" {
		t.Errorf("a public player lost their name: %+v", got[2])
	}
}

// A visibility nobody has set is not hidden: opting out is a thing you do.
func TestTheFeedNamesAPlayerNobodyAsked(t *testing.T) {
	g, _ := testGame(t)
	got := g.chronicle(
		[]store.Event{{ID: 1, Kind: eventCat, Actor: "alice"}},
		map[string]store.Visibility{"alice": store.VisibilityUnset},
	)
	if got[0].Actor != "alice" {
		t.Errorf("an unasked player was anonymised: %+v", got[0])
	}
}

func TestTheFeedDropsKindsItCannotRender(t *testing.T) {
	g, _ := testGame(t)
	got := g.chronicle(
		[]store.Event{{ID: 2, Kind: "invented-later", Actor: "alice"}},
		map[string]store.Visibility{},
	)
	if len(got) != 0 {
		t.Errorf("published an event with no words: %+v", got)
	}
}

// Detail is never nil, or it marshals to null and the column refuses it.
func TestAPublishedDetailIsAlwaysAnObject(t *testing.T) {
	g, _ := testGame(t)
	got := g.chronicle([]store.Event{{ID: 1, Kind: eventCat, Actor: "alice"}}, nil)
	if got[0].Detail == nil {
		t.Error("an event with no detail published a nil one")
	}
}

func TestTheFeedIsCappedAtWhatTheSiteTakes(t *testing.T) {
	g, _ := testGame(t)
	var events []store.Event
	for i := 0; i < eventFeed+50; i++ {
		events = append(events, store.Event{ID: int64(i), Kind: eventCat, Actor: "alice"})
	}
	if got := len(g.chronicle(events, nil)); got != eventFeed {
		t.Errorf("%d events published, want the %d cap", got, eventFeed)
	}
}

// The log is bounded, or a busy channel grows the database without limit.
func TestTheLogIsTrimmedToItsKeep(t *testing.T) {
	ctx := context.Background()
	_, db := testGame(t)

	for i := 0; i < 25; i++ {
		e := store.Event{TS: time.Now(), Kind: eventCat, Actor: "alice"}
		if err := db.RecordEvent(ctx, e, 10); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := db.RecentEvents(ctx, 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 10 {
		t.Errorf("%d rows kept, want the 10 asked for", len(got))
	}
	// Newest first, and the newest is the last one written.
	if len(got) > 1 && got[0].ID < got[1].ID {
		t.Errorf("the log came back oldest first: %d then %d", got[0].ID, got[1].ID)
	}
}

func TestEventsAboutReachesBothEndsOfARobbery(t *testing.T) {
	ctx := context.Background()
	_, db := testGame(t)

	write := func(kind, actor, subject string) {
		e := store.Event{TS: time.Now(), Kind: kind, Actor: actor, Subject: subject}
		if err := db.RecordEvent(ctx, e, eventLog); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	write(eventSteal, "alice", "bob")
	write(eventCat, "carol", "")

	for _, nick := range []string{"alice", "bob"} {
		got, err := db.EventsAbout(ctx, nick, 10)
		if err != nil {
			t.Fatalf("read %s: %v", nick, err)
		}
		if len(got) != 1 || got[0].Kind != eventSteal {
			t.Errorf("%s has %+v, want the robbery", nick, got)
		}
	}

	got, _ := db.EventsAbout(ctx, "carol", 10)
	if len(got) != 1 || got[0].Kind != eventCat {
		t.Errorf("carol has %+v, want her own line only", got)
	}
}

// End to end: a robbery that landed reaches the site under the right names,
// with the haul as a band.
func TestARobberyReachesTheFeed(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "alice", "AliceAcct", store.VisibilityPublic)
	player(t, db, "bob", "BobAcct", store.VisibilityPublic)

	// Inside what player() gave bob, or the store's guard refuses the theft.
	g.successSteal("alice", "bob", 0, 60)
	g.publish(ctx)

	body := string(site.table(t, tableEvent).Rows[0])
	for _, want := range []string{`"kind":"steal"`, `"actor":"alice"`, `"subject":"bob"`, "a pocketful"} {
		if !strings.Contains(body, want) {
			t.Errorf("the published robbery is missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "60") {
		t.Errorf("the published robbery carries the amount taken: %s", body)
	}
}

// The one that matters: an opted-out thief is not named by the feed, even
// though the channel heard it.
func TestAHiddenThiefIsNotNamedByTheFeed(t *testing.T) {
	ctx := context.Background()
	g, db, site := publishingGame(t)
	player(t, db, "alice", "AliceAcct", store.VisibilityHidden)
	player(t, db, "bob", "BobAcct", store.VisibilityPublic)

	g.successSteal("alice", "bob", 0, 60)
	g.publish(ctx)

	body := string(site.table(t, tableEvent).Rows[0])
	if strings.Contains(body, "alice") {
		t.Errorf("the feed named a player who is off the map: %s", body)
	}
	if !strings.Contains(body, `"subject":"bob"`) {
		t.Errorf("the victim lost their name too: %s", body)
	}
}

func TestADetailSurvivesARoundTrip(t *testing.T) {
	ctx := context.Background()
	_, db := testGame(t)

	want := map[string]string{"thing": "refinery"}
	if err := db.RecordEvent(ctx, store.Event{
		TS: time.Now(), Kind: eventBuild, Actor: "alice", Detail: want,
	}, eventLog); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := db.RecentEvents(ctx, 1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got[0].Detail["thing"] != "refinery" {
		t.Errorf("detail came back as %+v", got[0].Detail)
	}
}
