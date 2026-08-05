-- deerkins cloudflare D1 schema.
-- Apply with: pppm run db:init (local) / pnpm run db:init:remote

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

-- Image uploads. The bot mints a signed grant in irc, the browser redeems it
-- here for a cookie, and every write below is made by the worker: the bot's D1
-- token stays read-only.

-- A redeemed grant. The PRIMARY KEY conflict is the replay check, so one
-- statement decides it. Rows are pruned once past exp.
CREATE TABLE IF NOT EXISTS grant_used (
  jti TEXT PRIMARY KEY,
  exp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS grant_used_exp ON grant_used (exp);

-- One session for the whole site. id_hash is the sha-256 of the cookie value,
-- so this table is not a set of live sessions. channels is the json array
-- copied from the grant and bounds where the holder may post; scopes is the
-- bitmask copied from it and bounds what they may do. The browser can widen
-- neither.
--
-- upload_session was this table before sessions were shared. It is not migrated
-- and not dropped here: its rows last twelve hours, so it drains on its own.
-- Drop it by hand once it has.
CREATE TABLE IF NOT EXISTS session (
  id_hash  TEXT PRIMARY KEY,
  account  TEXT NOT NULL,
  nick     TEXT NOT NULL,
  channels TEXT NOT NULL,
  scopes   INTEGER NOT NULL DEFAULT 0,
  created  INTEGER NOT NULL,
  expires  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS session_expires ON session (expires);
CREATE INDEX IF NOT EXISTS session_account ON session (account);

-- The queue the bot polls, append-only. It tracks its own cursor and never
-- writes back, so there is no delivered flag. key is the r2 object key.
CREATE TABLE IF NOT EXISTS upload (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  ts      INTEGER NOT NULL,
  account TEXT NOT NULL,
  nick    TEXT NOT NULL,
  channel TEXT NOT NULL,
  key     TEXT NOT NULL,
  mime    TEXT NOT NULL,
  bytes   INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS upload_key ON upload (key);

-- Rate limiting, per account rather than per ip: a grant already proves who
-- this is. Pruned on write like save_log.
CREATE TABLE IF NOT EXISTS upload_log (
  account TEXT NOT NULL,
  ts      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS upload_log_account_ts ON upload_log (account, ts);
CREATE INDEX IF NOT EXISTS upload_log_ts ON upload_log (ts);
