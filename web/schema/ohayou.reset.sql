-- Drops the projection so schema/ohayou.sql can rebuild it.
--
-- Safe in a way the other databases are not: nothing here is authoritative.
-- The bot republishes within a couple of minutes, so the cost of running this
-- is a gap on the world map, not lost data.
--
--   pnpm db reset ohayou && pnpm db init ohayou
--
-- Needed because CREATE TABLE IF NOT EXISTS cannot add a column to a table that
-- already exists, and a projection's shape changes whenever the game's does.

DROP TABLE IF EXISTS plot;
DROP TABLE IF EXISTS plot_private;

-- publish goes too: with the tables gone, the generations that filled them mean
-- nothing, and keeping them would make the next publish look stale.
DROP TABLE IF EXISTS publish;
