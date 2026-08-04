package task

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// memStore is a task store in a map, keyed the way the table is.
type memStore struct {
	mu    sync.Mutex
	tasks map[string]store.Task
	err   error
}

func newMemStore() *memStore { return &memStore{tasks: map[string]store.Task{}} }

func key(plugin, kind, k string) string { return plugin + "/" + kind + "/" + k }

func (m *memStore) SaveTask(ctx context.Context, t store.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.tasks[key(t.Plugin, t.Kind, t.Key)] = t
	return nil
}

func (m *memStore) DeleteTask(ctx context.Context, plugin, kind, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, key(plugin, kind, k))
	return nil
}

func (m *memStore) DueTasks(ctx context.Context, at time.Time) ([]store.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var out []store.Task
	for _, t := range m.tasks {
		if !t.Due.After(at) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) PluginTasks(ctx context.Context, plugin string) ([]store.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var out []store.Task
	for _, t := range m.tasks {
		if t.Plugin == plugin {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tasks)
}

func (m *memStore) get(plugin, kind, k string) (store.Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[key(plugin, kind, k)]
	return t, ok
}

// inline runs goroutines where they are, so tests do not race the runner.
type inline struct{}

func (inline) Go(fn func()) { fn() }

func newTestRunner(m *memStore, now time.Time) *Runner {
	r := NewRunner(m, inline{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.now = func() time.Time { return now }
	return r
}

func TestAfterWritesATask(t *testing.T) {
	m := newMemStore()
	now := time.Unix(1000, 0)
	q := newTestRunner(m, now).For("ohayou")

	if err := q.After(context.Background(), "mine", "alice", time.Hour, "3"); err != nil {
		t.Fatal(err)
	}
	got, ok := m.get("ohayou", "mine", "alice")
	if !ok {
		t.Fatal("nothing was written")
	}
	if !got.Due.Equal(now.Add(time.Hour)) {
		t.Errorf("Due = %v, want an hour out", got.Due)
	}
	if got.Interval != time.Hour || got.Payload != "3" {
		t.Errorf("task = %+v", got)
	}
}

// Starting the same work twice must replace it, not queue a second one.
func TestAfterReplacesTheSameKey(t *testing.T) {
	m := newMemStore()
	q := newTestRunner(m, time.Unix(1000, 0)).For("ohayou")
	ctx := context.Background()

	q.After(ctx, "mine", "alice", time.Hour, "first")
	q.After(ctx, "mine", "alice", 2*time.Hour, "second")

	if n := m.len(); n != 1 {
		t.Errorf("%d tasks, want the second to replace the first", n)
	}
	if got, _ := m.get("ohayou", "mine", "alice"); got.Payload != "second" {
		t.Errorf("payload = %q, want the later one", got.Payload)
	}
}

func TestKeysAndKindsAreSeparate(t *testing.T) {
	m := newMemStore()
	q := newTestRunner(m, time.Unix(1000, 0)).For("ohayou")
	ctx := context.Background()

	q.After(ctx, "mine", "alice", time.Hour, "")
	q.After(ctx, "mine", "bob", time.Hour, "")
	q.After(ctx, "pump", "alice", time.Hour, "")

	if n := m.len(); n != 3 {
		t.Errorf("%d tasks, want one per user and kind", n)
	}
}

func TestPluginsCannotSeeEachOther(t *testing.T) {
	m := newMemStore()
	r := newTestRunner(m, time.Unix(1000, 0))
	ctx := context.Background()

	r.For("one").After(ctx, "work", "k", time.Hour, "")
	r.For("two").After(ctx, "work", "k", time.Hour, "")

	if n := m.len(); n != 2 {
		t.Errorf("%d tasks, want one plugin's key not to displace another's", n)
	}
}

func TestAfterRefusesAnUnkeyedTask(t *testing.T) {
	q := newTestRunner(newMemStore(), time.Unix(1000, 0)).For("p")
	if err := q.After(context.Background(), "", "k", time.Hour, ""); err == nil {
		t.Error("After accepted a task with no kind")
	}
	if err := q.After(context.Background(), "kind", "", time.Hour, ""); err == nil {
		t.Error("After accepted a task with no key")
	}
}

func TestCancel(t *testing.T) {
	m := newMemStore()
	q := newTestRunner(m, time.Unix(1000, 0)).For("ohayou")
	ctx := context.Background()

	q.After(ctx, "mine", "alice", time.Hour, "")
	if err := q.Cancel(ctx, "mine", "alice"); err != nil {
		t.Fatal(err)
	}
	if m.len() != 0 {
		t.Error("the task survived a cancel")
	}
	if err := q.Cancel(ctx, "mine", "alice"); err != nil {
		t.Errorf("cancelling nothing was an error: %v", err)
	}
}

func TestDueWorkRuns(t *testing.T) {
	m := newMemStore()
	now := time.Unix(1000, 0)
	r := newTestRunner(m, now)

	var got store.Task
	r.For("ohayou").Handle("mine", Fire, func(ctx context.Context, t store.Task) error {
		got = t
		return nil
	})
	r.For("ohayou").After(context.Background(), "mine", "alice", -time.Second, "3")

	r.fireDue(context.Background())

	if got.Key != "alice" || got.Payload != "3" {
		t.Errorf("handler saw %+v", got)
	}
	if m.len() != 0 {
		t.Error("a task that ran is still queued")
	}
}

func TestWorkNotYetDueIsLeftAlone(t *testing.T) {
	m := newMemStore()
	r := newTestRunner(m, time.Unix(1000, 0))

	ran := false
	r.For("ohayou").Handle("mine", Fire, func(ctx context.Context, t store.Task) error {
		ran = true
		return nil
	})
	r.For("ohayou").After(context.Background(), "mine", "alice", time.Hour, "")

	r.fireDue(context.Background())

	if ran {
		t.Error("work ran an hour early")
	}
	if m.len() != 1 {
		t.Error("work that had not come due was cleared")
	}
}

// A handler that fails does not leave the task to run again forever.
func TestAFailedHandlerStillClearsTheTask(t *testing.T) {
	m := newMemStore()
	r := newTestRunner(m, time.Unix(1000, 0))

	r.For("ohayou").Handle("mine", Fire, func(ctx context.Context, t store.Task) error {
		return errors.New("nope")
	})
	r.For("ohayou").After(context.Background(), "mine", "alice", -time.Second, "")

	r.fireDue(context.Background())

	if m.len() != 0 {
		t.Error("a task whose handler failed is still queued")
	}
}

// A task whose kind nobody claims stays put: a plugin turned off for a day
// should not have its queued work thrown away.
func TestOrphanedWorkIsKept(t *testing.T) {
	m := newMemStore()
	r := newTestRunner(m, time.Unix(1000, 0))
	r.For("ohayou").After(context.Background(), "mine", "alice", -time.Second, "")

	r.fireDue(context.Background())
	if m.len() != 1 {
		t.Error("work belonging to an unregistered plugin was cleared")
	}

	if err := r.catchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.len() != 1 {
		t.Error("catch-up threw away work belonging to an unregistered plugin")
	}
}

func TestCatchupFireLeavesItToRun(t *testing.T) {
	m := newMemStore()
	now := time.Unix(10000, 0)
	r := newTestRunner(m, now)

	r.For("ohayou").Handle("mine", Fire, func(ctx context.Context, t store.Task) error { return nil })
	// Queued eight hours ago and due an hour ago: the bot was down for it.
	m.SaveTask(context.Background(), store.Task{
		Plugin: "ohayou", Kind: "mine", Key: "alice",
		Due: now.Add(-time.Hour), Interval: 8 * time.Hour,
	})

	if err := r.catchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, ok := m.get("ohayou", "mine", "alice")
	if !ok {
		t.Fatal("the task was dropped")
	}
	if got.Due.After(now) {
		t.Errorf("Due = %v, want it left in the past so it fires at once", got.Due)
	}
}

func TestCatchupRescheduleMovesItOut(t *testing.T) {
	m := newMemStore()
	now := time.Unix(10000, 0)
	r := newTestRunner(m, now)

	r.For("ohayou").Handle("event", Reschedule, func(ctx context.Context, t store.Task) error { return nil })
	m.SaveTask(context.Background(), store.Task{
		Plugin: "ohayou", Kind: "event", Key: "cat",
		Due: now.Add(-time.Hour), Interval: 12 * time.Hour,
	})

	if err := r.catchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := m.get("ohayou", "event", "cat")
	if want := now.Add(12 * time.Hour); !got.Due.Equal(want) {
		t.Errorf("Due = %v, want %v", got.Due, want)
	}
}

func TestCatchupDropForgetsIt(t *testing.T) {
	m := newMemStore()
	now := time.Unix(10000, 0)
	r := newTestRunner(m, now)

	r.For("ohayou").Handle("report", Drop, func(ctx context.Context, t store.Task) error { return nil })
	m.SaveTask(context.Background(), store.Task{
		Plugin: "ohayou", Kind: "report", Key: "alice",
		Due: now.Add(-time.Hour), Interval: time.Minute,
	})

	if err := r.catchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.len() != 0 {
		t.Error("a stale window was kept")
	}
}

func TestCatchupLeavesFutureWorkAlone(t *testing.T) {
	m := newMemStore()
	now := time.Unix(10000, 0)
	r := newTestRunner(m, now)

	r.For("ohayou").Handle("mine", Drop, func(ctx context.Context, t store.Task) error { return nil })
	r.For("ohayou").After(context.Background(), "mine", "alice", time.Hour, "")

	if err := r.catchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.len() != 1 {
		t.Error("catch-up touched work that was not overdue")
	}
}

func TestCatchupReportsAStoreFailure(t *testing.T) {
	m := newMemStore()
	m.err = errors.New("database is closed")
	r := newTestRunner(m, time.Unix(1000, 0))

	if err := r.catchUp(context.Background()); err == nil {
		t.Error("catch-up hid a store failure")
	}
}
