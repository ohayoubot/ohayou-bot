# web

The bot's Cloudflare Pages project: the deerkins gallery at `/deerkins/` and the
drop uploader at `/drop/`. Every command below runs from this directory.

## Requirements

Node 18 or newer and a Cloudflare account.

## Local

```sh
pnpm install
pnpm db init deerkins  # create the schema
pnpm db seed deerkins  # load seed.sql
pnpm run dev           # http://localhost:8788/deerkins/
pnpm test
pnpm lint              # biome; `pnpm lint:fix` writes the fixes
```

Each database has one schema under `schema/`, applied with `pnpm db`:

```sh
pnpm db <init|seed|purge> <database> [local|remote|preview] [--yes]
```

`local` is the default. Anything else is a real database and needs `--yes`.

`seed.sql` contains the original 1600+ deerkins/artbutt works from yore.

## Deploy

Create the two databases once and put the ids they print into `wrangler.toml`.
Production is bound at the top level; `deerkins-preview` is bound under
`[[env.preview.d1_databases]]` so preview deployments never touch production
data:

```sh
pnpm exec wrangler d1 create deerkins
pnpm exec wrangler d1 create deerkins-preview
```

If wrangler token is expired run:

```sh
pnpm exec wrangler login
```

Then:

```sh
pnpm db init deerkins remote --yes
pnpm db seed deerkins remote --yes  # might take a few minutes
pnpm run deploy
openssl rand -hex 32 | pnpm exec wrangler pages secret put IP_SALT --project-name deerkins
pnpm run deploy
```

- The first `deploy` is what creates the Pages project.
- The second `deploy` applies the `IP_SALT` env variable

`IP_SALT` salts the hashed client IPs used for rate limiting. This means they
are *anonymized*. You can inspect the code yourself!

## Preview deployments

Cloudflare's GitHub integration builds a preview deployment for every push to a
non-production branch on the host repo (NOT from forks).

Give the preview database its schema, otherwise preview builds serve a working
site backed by empty tables:

```sh
pnpm db init deerkins preview --yes
pnpm db seed deerkins preview --yes  # optional, for real data to click around
```

`init` is idempotent, so re-run it against production *and* preview whenever
`schema/` changes. A table added for production only fails at runtime on a
branch build, not at deploy.

`wrangler pages secret put` writes to Production only. Preview needs its own
`IP_SALT`, set in the dashboard under the project, Settings, Variables and
secrets, Preview. Use a different value than production so the two environments
cannot produce matching IP hashes.

Two dashboard settings worth turning on, neither of which lives in this repo:

- Settings, General, *Enable access policy*. Preview URLs are public by default
  and stay reachable forever once created. This limits them to account members.
  It does not cover the `pages.dev` domain or the custom domain.
- Settings, Builds and deployments, preview branch control. Defaults to every
  non-production branch. Narrow it to the branch prefixes you actually use, or
  set it to None to build only on merge.

Prefix a commit message with `[CI Skip]` or `[CF-Pages-Skip]` to skip one build.

## Domain

Add the domain in the Cloudflare dashboard under Workers & Pages, the deerkins
project, Custom domains. Cloudflare writes the DNS record itself if the zone is
already in the same account. Adding a zone to Cloudflare does not attach it to
the project. This is a separate step.

The project serves everything. The app is the `deerkins` subdirectory of
`public`, so it ends up at `<domain>/deerkins/`.

## Ingest

`POST /api/ingest` is how the bot publishes a projection, and the only way game
data reaches the site. The bot still holds no D1 credential: it signs a json
body with `UPLOAD_HMAC_SECRET` and this end makes the write.

`lib/site/ingest.js` holds the allowlist of what each plugin may write and the
exact shape of a row. A field the bot starts sending is refused until it is
added there, on purpose. That list is the boundary between the game and the
internet; treat a change to it as a change to what is public.

The game has its own database so a mistake publishing territories cannot reach
the art or the live sessions:

```sh
pnpm exec wrangler d1 create ohayou
pnpm exec wrangler d1 create ohayou-preview
```

Put the ids they print into `wrangler.toml`, then apply the schema to both:

```sh
pnpm db init ohayou remote --yes
pnpm db init ohayou preview --yes
```

## Drop

`/drop/` trades a signed link from the irc bot for a cookie, uploads images to
the `hemera` bucket, and queues a line for the bot to say in a channel.

The bot signs those links, so both ends need the same secret:

```sh
openssl rand -hex 32 | pnpm exec wrangler pages secret put UPLOAD_HMAC_SECRET --project-name deerkins
```

Give the bot the same value as `OHAYOU_UPLOAD_SECRET`. It keys on the utf-8
bytes of the string, not on the hex it decodes to. Preview needs its own,
dashboard-only, like `IP_SALT`.

`PUBLIC_IMAGE_BASE` is the hostname the bucket is served from. R2 custom domains
send no `X-Content-Type-Options`, so add a transform rule there setting
`nosniff`. Uploaded bytes are typed from their magic numbers and served from a
hostname the cookie is not scoped to; that header is the third layer.

## Checks after deploying

Under project settings, confirm `IP_SALT` and `UPLOAD_HMAC_SECRET` are listed
under Production and not only Preview, that the Production `DB` and `UPLOADS`
bindings point at `deerkins` and `hemera`, and that the Preview ones point at
`deerkins-preview` and `hemera-preview`. If Preview still shows the production
names, a branch build can write to production data.

Per-deployment preview URLs return 404 for `/deerkins/api/*`. The production
domain serves it. Test against the production domain.
