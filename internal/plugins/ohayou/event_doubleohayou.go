package ohayou

import (
	"context"
	"errors"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
	"github.com/ohayoubot/ohayou-bot/internal/task"
)

// The distributor "malfunctions" every so often, doubling rations for a few
// minutes. Both halves are queued rather than slept through, so a restart
// during the window still repairs it.
const (
	taskDoubleOn  = "double-on"
	taskDoubleOff = "double-off"
	// doubleKey names the one distributor there is.
	doubleKey = "distributor"
	// doubleFlag remembers a window that was open when the bot went down, so
	// the rations it promised stay doubled until it is repaired.
	doubleFlag = "double"
)

func nextDoubleDelay() time.Duration {
	return time.Duration(randNum(43200, 129600)) * time.Second // 12-36 hours
}

func doubleWindow() time.Duration {
	return time.Duration(randNum(2, 10)) * time.Minute
}

func (g *Plugin) registerDoubleOhayou(q *task.Queue) {
	// A malfunction nobody was around for is not worth announcing late, so the
	// next one is scheduled afresh instead.
	q.Handle(taskDoubleOn, task.Reschedule, g.startDouble)
	// The repair always happens, however late: a channel told the distributor
	// is broken must be told it was fixed.
	q.Handle(taskDoubleOff, task.Fire, g.endDouble)
}

func (g *Plugin) startDouble(ctx context.Context, t store.Task) error {
	active := doubleWindow()
	g.log.Info("event: double ohayou on", "duration", active)

	g.setDouble(true)
	if err := g.kv.Set(ctx, doubleFlag, "on"); err != nil {
		g.log.Warn("saving the double window", "err", err)
	}
	for _, c := range g.bot.Channels() {
		g.say(c, "ERROR: Ohayou distributor is malfunctioning.")
	}
	return g.tasks.After(ctx, taskDoubleOff, doubleKey, active, "")
}

func (g *Plugin) endDouble(ctx context.Context, t store.Task) error {
	g.setDouble(false)
	if err := g.kv.Delete(ctx, doubleFlag); err != nil {
		g.log.Warn("clearing the double window", "err", err)
	}
	g.log.Info("event: double ohayou off")
	for _, c := range g.bot.Channels() {
		g.say(c, "Technicians have fixed the ohayou distributor. "+
			"It should be working as normal now.")
	}
	return g.tasks.After(ctx, taskDoubleOn, doubleKey, nextDoubleDelay(), "")
}

// resumeDoubleOhayou restores a window that was open when the bot went down,
// and makes sure a malfunction is queued when none is.
func (g *Plugin) resumeDoubleOhayou(ctx context.Context) error {
	switch _, err := g.kv.Get(ctx, doubleFlag); {
	case err == nil:
		g.log.Info("event: double ohayou still on from the last run")
		g.setDouble(true)
	case !errors.Is(err, store.ErrNotFound):
		return err
	}

	pending, err := g.tasks.Pending(ctx)
	if err != nil {
		return err
	}
	for _, t := range pending {
		if t.Kind == taskDoubleOn || t.Kind == taskDoubleOff {
			return nil
		}
	}
	return g.tasks.After(ctx, taskDoubleOn, doubleKey, nextDoubleDelay(), "")
}
