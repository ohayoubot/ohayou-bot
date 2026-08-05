-- Remove bad arts.
--
-- Generated from the seed.sql diff; do not hand-edit.
--   pnpm db purge deerkins                # local
--   pnpm db purge deerkins remote --yes   # production
--   pnpm db purge deerkins preview --yes  # preview

DELETE FROM deer WHERE deer IN (
-- 'bad art 1',
-- 'badart2
);

UPDATE deer SET creator = 'Anonydeer' WHERE deer IN (
-- 'badname',
-- 'badname2
);
