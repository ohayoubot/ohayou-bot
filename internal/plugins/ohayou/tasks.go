package ohayou

import (
	"context"
	"strconv"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

// The activities a user starts and then waits hours for. Each is keyed by the
// user, so a second start replaces the pending one rather than queueing a
// duplicate, and each sets the matching status flag for as long as it is
// outstanding.
const (
	taskMining   = "mining"
	taskPumping  = "pumping"
	taskBreeding = "breeding"
	// taskPolice weakens a guard the Ohayou Police put on a robbery victim.
	taskPolice = "police"
	// policeKey is where the guards themselves are kept between restarts.
	policeKey = "police"
)

// runs maps a task kind to the status flag it holds while it is outstanding.
// The names match, which is what lets startup reconcile one against the other.
var runs = []string{taskMining, taskPumping, taskBreeding}

// registerTasks claims the handlers. Every one is Fire: a quarry run that
// finished while the bot was down has still finished, and the user has still
// waited the eight hours.
func (g *Plugin) registerTasks(q *task.Queue) {
	q.Handle(taskMining, task.Fire, g.runMining)
	q.Handle(taskPumping, task.Fire, g.runPumping)
	q.Handle(taskBreeding, task.Fire, g.runBreeding)
	// Fire rather than Reschedule: a guard that went unwatched for hours has
	// still been weakening, and the handler ages it by however long that was.
	q.Handle(taskPolice, task.Fire, g.runPoliceDecay)
}

// startRun queues an activity and marks the user as busy with it.
func (g *Plugin) startRun(kind, username string, multiplier int, d time.Duration) error {
	if err := g.tasks.After(g.ctx(), kind, username, d, strconv.Itoa(multiplier)); err != nil {
		return err
	}
	g.setStatus(username, kind, true)
	return nil
}

// finish clears the status a run held. It runs whether or not the work
// succeeded, so a failure cannot leave a user unable to start again.
func (g *Plugin) finish(kind, username string) { g.setStatus(username, kind, false) }

// multiplier is what the user owned when the run started, carried in the
// payload so buying another quarry mid-run does not change the one in flight.
func multiplier(t store.Task) int {
	n, err := strconv.Atoi(t.Payload)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func (g *Plugin) runMining(ctx context.Context, t store.Task) error {
	defer g.finish(taskMining, t.Key)
	g.mine(t.Key, multiplier(t))
	return nil
}

func (g *Plugin) runPumping(ctx context.Context, t store.Task) error {
	defer g.finish(taskPumping, t.Key)
	g.pumpOil(t.Key, multiplier(t))
	return nil
}

func (g *Plugin) runBreeding(ctx context.Context, t store.Task) error {
	defer g.finish(taskBreeding, t.Key)
	g.breedCat(t.Key, multiplier(t))
	return nil
}

// reconcileRuns squares the status flags against what is actually queued. A
// flag with no task behind it would leave a user unable to start again, which
// is what the old blanket reset at startup was there to prevent; a task with no
// flag would let them start a second run over the top of the first.
func (g *Plugin) reconcileRuns(ctx context.Context) error {
	if err := g.store.ResetAllStatus(ctx); err != nil {
		return err
	}

	pending, err := g.tasks.Pending(ctx)
	if err != nil {
		return err
	}

	busy := 0
	for _, t := range pending {
		for _, kind := range runs {
			if t.Kind == kind {
				g.setStatus(t.Key, kind, true)
				busy++
			}
		}
	}
	if busy > 0 {
		g.log.Info("restored activities in progress", "count", busy)
	}
	return nil
}
