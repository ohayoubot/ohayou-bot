-- Remove bad arts.
--
-- Generated from the seed.sql diff; do not hand-edit.
--   pnpm run db:purge          # local
--   pnpm run db:purge:remote   # production
--   pnpm run db:purge:preview  # preview

DELETE FROM deer WHERE deer IN (
-- 'bad art 1',
-- 'badart2
);

UPDATE deer SET creator = 'Anonydeer' WHERE deer IN (
-- 'badname',
-- 'badname2
);
