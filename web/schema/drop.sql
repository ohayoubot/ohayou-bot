-- Image uploads. The bot mints a signed grant in irc, the browser redeems it
-- for a session, and every write is made by the worker: the bot's D1 token
-- stays read-only.

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
