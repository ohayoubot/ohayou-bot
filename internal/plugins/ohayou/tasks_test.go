package ohayou

import (
	"context"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// startedRun queues an activity the way !use does and returns the user.
func startedRun(t *testing.T, g *Plugin, kind, nick string, d time.Duration) {
	t.Helper()
	if err := g.startRun(kind, nick, 2, d); err != nil {
		t.Fatalf("startRun: %v", err)
	}
}

func statusOf(t *testing.T, g *Plugin, nick, action string) bool {
	t.Helper()
	u, err := g.store.GetUser(context.Background(), nick)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	return u.Status[action]
}

func TestStartingARunQueuesItAndMarksTheUserBusy(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}

	startedRun(t, g, taskMining, "alice", miningTime)

	if !statusOf(t, g, "alice", taskMining) {
		t.Error("the user is not marked as mining")
	}
	pending, err := g.tasks.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Kind != taskMining || pending[0].Key != "alice" {
		t.Fatalf("pending = %+v, want one mining task for alice", pending)
	}
	if got := pending[0].Payload; got != "2" {
		t.Errorf("payload = %q, want the quarry count at the time it started", got)
	}
}

// An eight-hour run must not be lost to a deploy.
func TestARunSurvivesARestart(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}
	startedRun(t, g, taskMining, "alice", miningTime)

	// A second plugin over the same store stands in for the process restarting.
	fresh, _ := testGameOn(t, db)
	if err := fresh.reconcileRuns(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if !statusOf(t, fresh, "alice", taskMining) {
		t.Error("the run was forgotten across a restart")
	}
	pending, _ := fresh.tasks.Pending(ctx)
	if len(pending) != 1 {
		t.Errorf("%d tasks after a restart, want the run still queued", len(pending))
	}
}

// A flag left behind with no task would leave a user unable to ever start
// again, which is what the old blanket reset at startup guarded against.
func TestReconcileClearsAFlagWithNoTask(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}
	g.setStatus("alice", taskMining, true)

	if err := g.reconcileRuns(ctx); err != nil {
		t.Fatal(err)
	}
	if statusOf(t, g, "alice", taskMining) {
		t.Error("a stuck flag survived startup")
	}
}

func TestStartingTwiceReplacesRatherThanDuplicates(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}

	startedRun(t, g, taskMining, "alice", miningTime)
	startedRun(t, g, taskMining, "alice", miningTime)

	pending, _ := g.tasks.Pending(ctx)
	if len(pending) != 1 {
		t.Errorf("%d tasks, want the second start to replace the first", len(pending))
	}
}

// Finishing pays out and frees the user to start again.
func TestFinishingAMiningRunClearsTheFlag(t *testing.T) {
	g, db := testGame(t)
	ctx := context.Background()
	if err := db.CreateUser(ctx, "alice", 0); err != nil {
		t.Fatal(err)
	}
	startedRun(t, g, taskMining, "alice", miningTime)

	if err := g.runMining(ctx, store.Task{Kind: taskMining, Key: "alice", Payload: "2"}); err != nil {
		t.Fatalf("runMining: %v", err)
	}
	if statusOf(t, g, "alice", taskMining) {
		t.Error("the flag survived the run finishing")
	}

	u, _ := g.store.GetUser(ctx, "alice")
	if len(u.Quarry.Metals) == 0 {
		t.Error("a finished mining run paid out nothing")
	}
}

func TestMultiplierFallsBackToOne(t *testing.T) {
	for _, payload := range []string{"", "rubbish", "0", "-3"} {
		if got := multiplier(store.Task{Payload: payload}); got != 1 {
			t.Errorf("multiplier(%q) = %d, want 1", payload, got)
		}
	}
	if got := multiplier(store.Task{Payload: "7"}); got != 7 {
		t.Errorf("multiplier(7) = %d", got)
	}
}
