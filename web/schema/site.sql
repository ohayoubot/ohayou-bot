-- Tables belonging to the site rather than to any one plugin: the session a
-- link from irc is redeemed for, and the record of which links have been spent.
--
-- These live in the same database as the gallery's and drop's, which is named
-- for the gallery for historical reasons. See the naming note in README.md.

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
