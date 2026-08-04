// Package task runs work a plugin wants done later, including after a restart.
package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// tick is how often the runner looks for work that has come due. The tasks it
// carries are measured in minutes and hours, so a second of slack costs
// nothing and keeps the loop simple.
const tick = time.Second

// Catchup says what to do with a task that came due while the bot was down.
type Catchup int

const (
	// Fire runs it at the next tick. A quarry run that finished overnight
	// should pay out, however late.
	Fire Catchup = iota
	// Reschedule pushes it out by the interval it was given, for work that is
	// only meaningful from now on.
	Reschedule
	// Drop forgets it. A sixty-second window to answer a question is not worth
	// reopening hours later.
	Drop
)

// Handler does the work. Returning an error logs it; the task is gone either
// way, so a handler that wants a retry schedules one.
type Handler func(ctx context.Context, t store.Task) error

type handler struct {
	fn      Handler
	catchup Catchup
}

// Goer is the part of the bot the runner needs: a goroutine it will drain on
// shutdown.
type Goer interface {
	Go(fn func())
}

// Runner owns the clock. One runs for the whole bot; each plugin gets a Queue
// onto it.
type Runner struct {
	store store.TaskStore
	go_   Goer
	log   *slog.Logger

	now func() time.Time

	mu       sync.RWMutex
	handlers map[string]handler // "plugin\x00kind"
}

func NewRunner(st store.TaskStore, g Goer, log *slog.Logger) *Runner {
	return &Runner{
		store:    st,
		go_:      g,
		log:      log,
		now:      time.Now,
		handlers: map[string]handler{},
	}
}

// For returns the handle a plugin schedules through. Its tasks are scoped to
// the plugin, so two plugins cannot fire each other's work.
func (r *Runner) For(plugin string) *Queue { return &Queue{runner: r, plugin: plugin} }

// Start applies the catch-up rules to whatever the last run left behind, then
// watches for work coming due until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) error {
	if err := r.catchUp(ctx); err != nil {
		return err
	}
	r.go_.Go(func() { r.loop(ctx) })
	return nil
}

// catchUp decides what to do with everything already overdue. A task whose
// plugin is no longer registered is left where it is: turning a plugin off for
// a day should not throw away what it had queued.
func (r *Runner) catchUp(ctx context.Context) error {
	overdue, err := r.store.DueTasks(ctx, r.now())
	if err != nil {
		return err
	}

	var fired, moved, dropped, orphaned int
	for _, t := range overdue {
		h, ok := r.handler(t)
		if !ok {
			orphaned++
			continue
		}
		switch h.catchup {
		case Reschedule:
			t.Due = r.now().Add(t.Interval)
			if err := r.store.SaveTask(ctx, t); err != nil {
				return err
			}
			moved++
		case Drop:
			if err := r.store.DeleteTask(ctx, t.Plugin, t.Kind, t.Key); err != nil {
				return err
			}
			dropped++
		default:
			fired++
		}
	}

	if len(overdue) > 0 {
		r.log.Info("catching up on tasks",
			"fire", fired, "reschedule", moved, "drop", dropped, "orphaned", orphaned)
	}
	return nil
}

func (r *Runner) loop(ctx context.Context) {
	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		r.fireDue(ctx)
	}
}

// fireDue runs everything that has come due. A task is deleted before its
// handler runs, so work that panics or hangs cannot be picked up again on the
// next tick and run forever.
func (r *Runner) fireDue(ctx context.Context) {
	due, err := r.store.DueTasks(ctx, r.now())
	if err != nil {
		r.log.Warn("reading due tasks", "err", err)
		return
	}

	for _, t := range due {
		h, ok := r.handler(t)
		if !ok {
			continue
		}
		if err := r.store.DeleteTask(ctx, t.Plugin, t.Kind, t.Key); err != nil {
			r.log.Error("clearing a task", "plugin", t.Plugin, "kind", t.Kind, "key", t.Key, "err", err)
			continue
		}
		r.go_.Go(func() {
			if err := h.fn(ctx, t); err != nil {
				r.log.Error("running a task",
					"plugin", t.Plugin, "kind", t.Kind, "key", t.Key, "err", err)
			}
		})
	}
}

func (r *Runner) handler(t store.Task) (handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[t.Plugin+"\x00"+t.Kind]
	return h, ok
}

// Queue is one plugin's handle on the runner.
type Queue struct {
	runner *Runner
	plugin string
}

// Handle registers what runs a kind of task, and what to do with one that came
// due while the bot was down. It belongs in Register, before Start: a task
// whose kind nobody claims is left in the store rather than run.
func (q *Queue) Handle(kind string, catchup Catchup, fn Handler) {
	q.runner.mu.Lock()
	defer q.runner.mu.Unlock()
	q.runner.handlers[q.plugin+"\x00"+kind] = handler{fn: fn, catchup: catchup}
}

// After schedules work for d from now, replacing anything already queued under
// the same kind and key.
func (q *Queue) After(ctx context.Context, kind, key string, d time.Duration, payload string) error {
	if kind == "" || key == "" {
		return fmt.Errorf("task: kind and key are required")
	}
	return q.runner.store.SaveTask(ctx, store.Task{
		Plugin:   q.plugin,
		Kind:     kind,
		Key:      key,
		Due:      q.runner.now().Add(d),
		Interval: d,
		Payload:  payload,
	})
}

// Pending is everything the plugin has queued, due or not, for reconciling its
// own state at startup against what is still outstanding.
func (q *Queue) Pending(ctx context.Context) ([]store.Task, error) {
	return q.runner.store.PluginTasks(ctx, q.plugin)
}

// Cancel drops queued work, and is not an error when there was none.
func (q *Queue) Cancel(ctx context.Context, kind, key string) error {
	return q.runner.store.DeleteTask(ctx, q.plugin, kind, key)
}
