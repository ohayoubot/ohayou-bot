# web

The ohayou bot's Cloudflare Pages project: the world map at `/ohayou/`, the day
book at `/ohayou/lately`, the deerkins gallery at `/deerkins/` and the drop
uploader at `/drop/`. Every command below runs from this directory.

## Names

| Thing | Name | Holds |
| --- | --- | --- |
| Pages project | `ohayou-web` | |
| D1 | `ohayou-site` | sessions, grants, the gallery, drop's queue |
| D1 | `ohayou-game` | the world projection, the chronicle, the request queue |
| R2 | `hemera` | uploaded images |

Each D1 has a `-preview` twin. R2 cannot be renamed and image links already said
in channels point at `hemera`, so it keeps that name.

The `pages.dev` subdomain does not follow a project rename, so deployment urls
still read `deerkins.pages.dev`. The custom domain is what is used.

Bindings come from `wrangler.toml` and are applied when a deployment carrying
them is built. The dashboard shows what the last build declared.

To move a database: `wrangler d1 export` the old, `wrangler d1 create` the new,
`wrangler d1 execute --file` the dump into it, put the id in `wrangler.toml`.
The export carries explicit primary keys and `sqlite_sequence`, so drop's queue
ids survive and the bot's cursor stays valid.

## The shell

`public/shell.js` is the header every page wears, and the only place a session
is read. It redeems whatever grant is in the url fragment, so a `!web` link
signs you in on whichever page you open it, then fills in the tabs from
`plugins.json` and the account menu from the session. A page calls
`shell({ area, current })` and gets the session back; `area` is what colours the
page's chrome.

Only a fragment shaped like a grant is redeemed. The gallery puts an art name in
the fragment too, and that one is left alone.

`GET /api/session` is what every page loads, which makes it where a session's
expiry is slid forward. See `lib/session.js` for the month it lasts and the
third of it that has to be spent before a read is worth a write.

## Requirements

Node 18 or newer and a Cloudflare account.

## Local

```sh
pnpm install
pnpm db init ohayou-site   # create the schema
pnpm db init ohayou-game
pnpm db seed ohayou-site   # load seed.sql
pnpm run dev               # http://localhost:8788/
pnpm test
pnpm lint                  # biome; `pnpm lint:fix` writes the fixes
```

Schemas live under `schema/` and are applied with `pnpm db`:

```sh
pnpm db <init|seed|purge|reset> <ohayou-site|ohayou-game> [local|remote|preview] [--yes]
```

`local` is the default; anything else is a real database and needs `--yes`. One
database may hold several plugins' tables, so a schema is several files:
`init ohayou-site` applies `site.sql`, `deerkins.sql` and `drop.sql` in turn.
`init` is idempotent; `reset` drops the game's tables so `init` can rebuild them.

`seed.sql` contains the original 1600+ deerkins/artbutt works from yore.

## Deploy

Create the databases once and put the ids they print into `wrangler.toml`.
Production is bound at the top level, preview under
`[[env.preview.d1_databases]]`, so preview deployments never touch production
data:

```sh
pnpm exec wrangler d1 create ohayou-site
pnpm exec wrangler d1 create ohayou-site-preview
pnpm exec wrangler d1 create ohayou-game
pnpm exec wrangler d1 create ohayou-game-preview
```

If the wrangler token is expired, run `pnpm exec wrangler login`. Then:

```sh
pnpm db init ohayou-site remote --yes
pnpm db init ohayou-game remote --yes
pnpm db seed ohayou-site remote --yes  # might take a few minutes
pnpm run deploy
openssl rand -hex 32 | pnpm exec wrangler pages secret put IP_SALT --project-name ohayou-web
pnpm run deploy
```

- The first `deploy` is what creates the Pages project.
- The second `deploy` applies the `IP_SALT` env variable

`IP_SALT` salts the hashed client IPs used for rate limiting. This means they
are *anonymized*. You can inspect the code yourself!

## Preview deployments

Cloudflare's GitHub integration builds a preview deployment for every push to a
non-production branch on the host repo (NOT from forks).

Give the preview databases their schema, otherwise preview builds serve a
working site backed by empty tables:

```sh
pnpm db init ohayou-site preview --yes
pnpm db init ohayou-game preview --yes
pnpm db seed ohayou-site preview --yes  # optional, for real data to click around
```

Re-run `init` against production *and* preview whenever `schema/` changes. A
table added for production only fails at runtime on a branch build, not at
deploy.

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

`POST /api/ingest` is how the bot publishes the game's projection, and the only
way game data reaches the site. The bot holds no D1 credential: it signs a json
body with `OHAYOU_WEB_SECRET` and this end makes the write.

`lib/site/ingest.js` holds the allowlist of what each plugin may write and the
shape of a row. A field the bot starts sending is refused until it is added
there. Treat a change to that list as a change to what is public.

A publish replaces its table outright, so a player who withdrew consent is
absent rather than stale. The chronicle is the exception: `mode: "append"` adds
the entries the site does not have and trims to the newest `keep`, because
rewriting two hundred rows to add one is most of a free plan's daily write
budget. Only the tables in `APPENDABLE` take it, and the bot asks for it only
when every entry the site holds is one it would have sent again — withdrawing a
name rewrites past entries, and that goes as a replace. The answer carries
`total`, the rows the table holds afterwards; when that is not what the bot
expects, it sends the whole feed next round.

The projection's shape changes with the game's. `CREATE TABLE IF NOT EXISTS`
cannot add a column, so rebuild it:

```sh
pnpm db reset ohayou-game remote --yes
pnpm db init ohayou-game remote --yes
```

Nothing is lost; the bot republishes within a couple of minutes.

## The webchat

The landing page opens a webchat in a frame on click; nothing is loaded until
then. `IRC_WEBCHAT` in `wrangler.toml` is the url, and must be:

- `https`, or the frame is blocked as mixed content;
- on a host named in `frame-src` in `public/_headers`.

`tools/site.test.mjs` checks both. Changing the host means changing that
`frame-src` line.

## The day book

`/ohayou/lately` is the chronicle the bot publishes: what happened on the land,
newest first. `?nick=<name>` narrows it to one holder, which is what a deed page
links to. The front page shows the newest six, and a deed page the newest eight
for that holder.

The bot decides who is named before it publishes, so an entry arrives here with
`actor` and `subject` already empty for anyone whose plot carries no name. The
filter is passed to the api rather than applied in the page, so a name that was
withheld cannot be found by asking for it.

`public/ohayou/chronicle.js` turns an entry into a sentence, and is shared by
the browser and the worker the way `plot.js` is. The bot has its own copy of
those sentences for irc. Neither is authoritative: an entry whose kind this end
has no words for is dropped rather than guessed at, so a kind the bot learns
first costs a line, not a page.

## Player pages

`/ohayou/p/<nick>` is one player's plot; `/ohayou/api/card/<nick>` is the same
plot as an svg, used as the link preview image. Both are rendered by a function
so the meta tags are in the html a crawler is handed.

Only a plot whose owner named it has a page. `IRC_CHANNEL` and `IRC_NETWORK` in
`wrangler.toml` are what the card tells a visitor to do next.

The page is html a crawler can read without running anything; the one script on
it is `public/player.js`, which fills in the header's account menu and nothing
else. `tools/card.test.mjs` holds it to that.

## Drop

`/drop/` trades a signed link from the irc bot for a cookie, uploads images to
the `hemera` bucket, and queues a line for the bot to say in a channel.

The bot signs those links with `OHAYOU_WEB_SECRET`, and this end verifies them
with the same value:

```sh
openssl rand -hex 32 | pnpm exec wrangler pages secret put OHAYOU_WEB_SECRET --project-name ohayou-web
```

Give the bot the same string. Both sides key on its utf-8 bytes, not the hex it
looks like. Preview needs its own, set from the dashboard, like `IP_SALT`.

`PUBLIC_IMAGE_BASE` is the hostname the bucket is served from. R2 custom domains
send no `X-Content-Type-Options`, so add a transform rule there setting
`nosniff`. Uploaded bytes are typed from their magic numbers and served from a
hostname the cookie is not scoped to; that header is the third layer.

## Checks after deploying

Under project settings, confirm `IP_SALT` and `OHAYOU_WEB_SECRET` are set for
Production as well as Preview, and that the bindings are:

| Binding | Production | Preview |
| --- | --- | --- |
| `DB` | `ohayou-site` | `ohayou-site-preview` |
| `GAME` | `ohayou-game` | `ohayou-game-preview` |
| `UPLOADS` | `hemera` | `hemera-preview` |

If Preview shows a production name, a branch build can write production data.

Per-deployment preview URLs return 404 for `/deerkins/api/*`. The production
domain serves it. Test against the production domain.
