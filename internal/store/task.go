package store

import (
	"context"
	"time"
)

// Task is work a plugin wants done later. It outlives a restart, so a timer
// measured in hours is not lost to a deploy.
type Task struct {
	// Plugin owns the task; Kind says which of its handlers runs it.
	Plugin string
	Kind   string
	// Key makes a task replaceable and cancellable without a scan. One user's
	// quarry run is one key, so starting a second cannot queue a duplicate.
	Key string
	// Due is when the work should happen.
	Due time.Time
	// Interval is how long the task was originally scheduled for, so one that
	// came due while the bot was down can be pushed out by the same again.
	Interval time.Duration
	// Payload is whatever the handler needs, opaque to the store.
	Payload string
}

// TaskStore is the part of a Store the scheduler uses.
type TaskStore interface {
	// SaveTask writes t, replacing any task with the same plugin, kind and key.
	SaveTask(ctx context.Context, t Task) error
	// DeleteTask removes one, and is not an error when it was already gone.
	DeleteTask(ctx context.Context, plugin, kind, key string) error
	// DueTasks returns the tasks due at or before at, soonest first.
	DueTasks(ctx context.Context, at time.Time) ([]Task, error)
	// PluginTasks returns everything queued for one plugin, due or not, so it
	// can reconcile its own state against what is still outstanding.
	PluginTasks(ctx context.Context, plugin string) ([]Task, error)
}
