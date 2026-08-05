-- The deerkins gallery: pixel art drawn on the site and painted into irc.

CREATE TABLE IF NOT EXISTS deer (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  date     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now')),
  creator  TEXT NOT NULL DEFAULT 'n/a',
  deer     TEXT NOT NULL,
  kinskode TEXT NOT NULL,
  irccode  TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS deer_name ON deer (deer);
CREATE INDEX IF NOT EXISTS deer_date ON deer (date DESC);

-- rows older than 24h are pruned on write, so this stays small.
CREATE TABLE IF NOT EXISTS save_log (
  ip_hash TEXT NOT NULL,
  ts      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS save_log_ip_ts ON save_log (ip_hash, ts);
CREATE INDEX IF NOT EXISTS save_log_ts ON save_log (ts);
