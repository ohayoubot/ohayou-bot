-- The ohayou game's projection.
--
-- The bot's sqlite is where the game actually lives; this is a view of it,
-- published over /api/ingest and never written by anything else. Nothing here
-- is authoritative: it can be rebuilt at any time by asking the bot to publish
-- again, and no game rule is decided from these rows.
--
-- Deliberately denormalised for display. Mirroring the game's own tables would
-- put the shape of its rules in a place that cannot enforce them, and a page
-- able to recompute a rule will eventually disagree with the bot about one.

-- What anyone may see. The columns are exactly what the bot's publicPlot
-- promises: no balance, no vault, no defences. See internal/plugins/ohayou/web.go.
CREATE TABLE IF NOT EXISTS plot (
  account  TEXT PRIMARY KEY,
  nick     TEXT NOT NULL,
  acres    INTEGER NOT NULL DEFAULT 0,
  -- land is the json object of what occupies those acres, by item name.
  land     TEXT NOT NULL DEFAULT '{}',
  -- wealth is a band, not a number: it ranks players without telling a thief
  -- what is worth taking today.
  wealth   TEXT NOT NULL DEFAULT '',
  rations  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS plot_rations ON plot (rations DESC);

-- What one player may see about themselves, served only against a session
-- holding the matching account. There is no listing endpoint for this table and
-- no route that takes an account as a parameter.
CREATE TABLE IF NOT EXISTS plot_private (
  account    TEXT PRIMARY KEY,
  nick       TEXT NOT NULL,
  ohayous    INTEGER NOT NULL DEFAULT 0,
  cumulative INTEGER NOT NULL DEFAULT 0,
  items      TEXT NOT NULL DEFAULT '{}',
  metals     TEXT NOT NULL DEFAULT '{}',
  equipped   TEXT NOT NULL DEFAULT '{}',
  defense    INTEGER NOT NULL DEFAULT 0,
  -- vault is the json object, or null when there is no vault installed.
  vault      TEXT,
  -- probation and the runs are unix seconds, so a page can count down to them.
  probation  INTEGER NOT NULL DEFAULT 0,
  fortune    TEXT NOT NULL DEFAULT '',
  running    TEXT NOT NULL DEFAULT '[]'
);

-- One row per table the bot publishes, holding the last generation accepted.
-- A publish carrying a generation this side has already seen is refused, so a
-- replayed request cannot put yesterday's territories back.
CREATE TABLE IF NOT EXISTS publish (
  plugin     TEXT NOT NULL,
  table_name TEXT NOT NULL,
  generation INTEGER NOT NULL,
  rows       INTEGER NOT NULL DEFAULT 0,
  updated    INTEGER NOT NULL,
  PRIMARY KEY (plugin, table_name)
);
