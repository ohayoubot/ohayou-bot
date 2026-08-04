-- Small pieces of bot state that outlive a restart and belong to no user, such
-- as how far the drop plugin has read through the upload queue.
CREATE TABLE IF NOT EXISTS kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Work a plugin wants done later, kept so an eight-hour timer survives a
-- restart. One row per (plugin, kind, key), so scheduling the same thing twice
-- replaces it rather than queueing a duplicate.
CREATE TABLE IF NOT EXISTS tasks (
    plugin   TEXT    NOT NULL,
    kind     TEXT    NOT NULL,
    key      TEXT    NOT NULL,
    due      INTEGER NOT NULL,
    interval INTEGER NOT NULL DEFAULT 0,
    payload  TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (plugin, kind, key)
);

CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due);
