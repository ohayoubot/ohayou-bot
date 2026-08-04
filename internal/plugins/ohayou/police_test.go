package ohayou

import (
	"context"
	"testing"
	"time"
)

func guarded(r *policeRegistry, user string) guard {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[user]
}

func TestReserveIsOnlyGrantedOnce(t *testing.T) {
	r := newPoliceRegistry()
	if !r.reserve("alice") {
		t.Fatal("the first reservation was refused")
	}
	if r.reserve("alice") {
		t.Error("a second reservation was granted")
	}
}

func TestBonusReportsTheGuard(t *testing.T) {
	r := newPoliceRegistry()
	r.set("alice", guard{Ohayou: 30, Cat: 20})

	ohayou, cat, ok := r.bonus("alice")
	if !ok || ohayou != 30 || cat != 20 {
		t.Errorf("bonus = %d, %d, %v", ohayou, cat, ok)
	}
	if _, _, ok := r.bonus("bob"); ok {
		t.Error("an unguarded user came back protected")
	}
}

func TestDecayTakesOneStepAnHour(t *testing.T) {
	r := newPoliceRegistry()
	now := time.Unix(100000, 0)
	r.set("alice", guard{Ohayou: 40, Cat: 20, DecOhayou: 10, DecCat: 5, Since: now})

	if !r.decay("alice", now.Add(time.Hour), time.Hour) {
		t.Fatal("the guard ended after one hour")
	}
	if g := guarded(r, "alice"); g.Ohayou != 30 || g.Cat != 15 {
		t.Errorf("guard = %+v, want one step off", g)
	}
}

// A bot that was down for hours must not hand those hours back.
func TestDecayCatchesUpOnMissedHours(t *testing.T) {
	r := newPoliceRegistry()
	now := time.Unix(100000, 0)
	r.set("alice", guard{Ohayou: 40, Cat: 20, DecOhayou: 10, DecCat: 5, Since: now})

	if !r.decay("alice", now.Add(3*time.Hour), time.Hour) {
		t.Fatal("the guard ended too early")
	}
	if g := guarded(r, "alice"); g.Ohayou != 10 || g.Cat != 5 {
		t.Errorf("guard = %+v, want three steps off", g)
	}
}

// Down longer than the protection would have lasted ends it, rather than
// resuming it as though no time had passed.
func TestADowntimeLongerThanTheGuardEndsIt(t *testing.T) {
	r := newPoliceRegistry()
	now := time.Unix(100000, 0)
	r.set("alice", guard{Ohayou: 40, Cat: 20, DecOhayou: 10, DecCat: 5, Since: now})

	if r.decay("alice", now.Add(9*time.Hour), time.Hour) {
		t.Error("a guard survived a downtime longer than itself")
	}
	if _, _, ok := r.bonus("alice"); ok {
		t.Error("the guard is still on the books")
	}
}

func TestDecayAlwaysTakesAtLeastOneStep(t *testing.T) {
	r := newPoliceRegistry()
	now := time.Unix(100000, 0)
	r.set("alice", guard{Ohayou: 40, Cat: 20, DecOhayou: 10, DecCat: 5, Since: now})

	// Fired early, before a whole period has passed.
	if !r.decay("alice", now.Add(time.Minute), time.Hour) {
		t.Fatal("the guard ended")
	}
	if g := guarded(r, "alice"); g.Ohayou != 30 {
		t.Errorf("Ohayou = %d, want one step off even so", g.Ohayou)
	}
}

func TestDecayOnNobodyIsNotAPanic(t *testing.T) {
	r := newPoliceRegistry()
	if r.decay("nobody", time.Now(), time.Hour) {
		t.Error("decaying a guard that does not exist reported one")
	}
}

func TestDumpAndRestore(t *testing.T) {
	r := newPoliceRegistry()
	now := time.Unix(100000, 0).UTC()
	r.set("alice", guard{Ohayou: 40, Cat: 20, DecOhayou: 10, DecCat: 5, Since: now})

	raw, err := r.dump()
	if err != nil {
		t.Fatal(err)
	}

	fresh := newPoliceRegistry()
	if err := fresh.restore(raw); err != nil {
		t.Fatal(err)
	}
	got := guarded(fresh, "alice")
	if got.Ohayou != 40 || got.DecOhayou != 10 || !got.Since.Equal(now) {
		t.Errorf("restored %+v", got)
	}
}

func TestRestoreRejectsRubbish(t *testing.T) {
	if err := newPoliceRegistry().restore("not json"); err == nil {
		t.Error("restore accepted something it did not write")
	}
}

// The whole path: a guard and its decay both outlive the process.
func TestGuardSurvivesARestart(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()

	g.protect("alice", 0)
	if _, _, ok := g.police.bonus("alice"); !ok {
		t.Fatal("protect did not put a guard on")
	}

	fresh := testGameOn(t, db)
	if err := fresh.resumePolice(ctx); err != nil {
		t.Fatalf("resume: %v", err)
	}

	if _, _, ok := fresh.police.bonus("alice"); !ok {
		t.Error("the guard was lost across a restart")
	}
	pending, err := fresh.tasks.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var decays int
	for _, task := range pending {
		if task.Kind == taskPolice && task.Key == "alice" {
			decays++
		}
	}
	if decays != 1 {
		t.Errorf("%d decays queued, want exactly one", decays)
	}
}

// A guard whose decay task went missing would otherwise last forever.
func TestResumeQueuesADecayForAnUnwatchedGuard(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()

	g.police.set("alice", guard{Ohayou: 40, DecOhayou: 10, Since: time.Now()})
	g.savePolice(ctx)

	fresh := testGameOn(t, db)
	if err := fresh.resumePolice(ctx); err != nil {
		t.Fatal(err)
	}

	pending, _ := fresh.tasks.Pending(ctx)
	if len(pending) != 1 || pending[0].Kind != taskPolice {
		t.Errorf("pending = %+v, want a decay queued for the unwatched guard", pending)
	}
}
