# ohayoubot

## Setup

1. Copy `conf-example.json` to `conf.json` and edit it. Important fields:
   - `server`, `port`, `nick`, `user`, `nickPw` (optional NickServ password)
   - `tls` - connect over TLS (default port becomes `6697` when set)
   - `sasl` - SASL authentication. Used automatically when configured: set
     `login` and `password` for the `PLAIN` mechanism, or `"mechanism":
     "EXTERNAL"` for TLS client-certificate auth. Add `"enabled": false` to
     force it off, or `"enabled": true` to force it on. Prefer running `PLAIN`
     over `tls` so the password isn't sent in the clear.
   - `vhost` - HostServ vhost gate. On Rizon the bot logs in, activates its
     vhost (`command` sent to `service`, default `HostServ ON`), and waits for
     the server's host-hidden confirmation (numeric `396`) **before joining any
     channels**, so its real hostname is never exposed. Set `"enabled":
     true`/`false` to override the auto-detect, and `timeout` (ms) to for the
     wait to join. If it elapses the bot logs an error and stays out of the channels
     rather than leaking the host.

     Not really the same concern for SASL, which will not send the HostServ
     message. It simply checks the vhost is as expected.
   - `channels` - each entry may include a join key: `"#chan key"`
   - `commandPrefix` - e.g. `"!"`
   - `floodProtect` / `floodDelay` - `floodDelay` is the minimum milliseconds
     between outbound messages
   - `admins` - map of `nick` -> `host` allowed to run admin commands
   - `database` - path to the sqlite file (default `ohayoubot.db`)
   - `web` - `url` is the website's front door, which `!web` mints links
     against. See [Web](#web).
   - `cloudflare` - `accountId`, `databaseId` and `apiToken` for the D1 database
     the plugins that need one share. A plugin may override any of these in its
     own block once it wants a database of its own.
   - `plugins` - one block per plugin, keyed by its name. A block for a plugin
     that does not exist is an error at startup rather than silence later.
     - `deerkins` - the pixel art gallery. Off unless it has a database to read.
       See [Deerkins](#deerkins).
     - `drop` - image uploads. Off unless `OHAYOU_WEB_SECRET` is set and `url`
       is filled in. See [Drop](#drop).
     - `youtube` - names the videos linked in a channel. Needs no credentials,
       so it is **on** unless you set `"enabled": false`. See
       [YouTube previews](#youtube-previews).
     - `ohayou` - the daily economy game. On unless you set `"enabled": false`.
       `dataDir` is where `items.json` and `fortunes.txt` live (default
       `data`), and `timezone` is the calendar a daily ration runs on (default
       `America/New_York`). See [Ohayou](#ohayou).
2. Build and run:
   ```sh
   go build ./cmd/ohayoubot
   ./ohayoubot -config conf.json
   ```

On first run the item catalog in `data/items.json` is seeded into the database.
Running `sudo deploy/install.sh` will update prices/descriptions/etc if run
against an already seeded database.

## Ohayou

The game the bot is named after. `!ohayou` once a day collects a ration; spend
them on land, animals, industry and thievery. `!help` lists its topics, and
`!commands` everything it answers to.

It is a plugin like any other: turn it off with `"enabled": false` in its block
and the bot keeps its other commands, needing no `data/` directory at all.

## Deerkins

`!deerme` paints art from the deerkins gallery into the channel. The drawings
live in a Cloudflare D1 database that the web app in `web/` writes. The bot just
reads from it.

```
!deerme                     the deer named "deer"
!deerme senordeer           by name
!deerme random              one at random, credited
!deerme latest              the newest one, credited
!deerme iu|senordeer        stack modifiers before a pipe
!deerme x|senordeer         let it pick the modifiers
!deerme help                how to deer, plus how long until the next one
!deerme help modifiers      list the modifiers
!prevdeer                   what walked the earth last
```

Modifiers are `i` invert, `m` mirror, `n` unitinu, `d` divide, `r` reverse, `u`
upsidedown, `s` square, `f` flip, `t` transpose, and `x` for a random pile of
the rest.

### Access

The bot talks to the D1 HTTP API, so it needs three things, which go in the
top-level `cloudflare` block that drop reads too:

- `accountId` - Cloudflare account id, on the right of the dashboard overview.
- `databaseId` - from `npx wrangler d1 info deerkins`, or the `database_id`
  already in the gallery's `wrangler.toml`.
- an API token with **Account → D1 → Read** and nothing else. Create it under
  My Profile → API Tokens → Create Token → Create Custom Token, scoped to the
  one account. `Read` is enough because the bot only runs `SELECT`s; a token
  that cannot write cannot deface the gallery if the bot is ever compromised.

Keep the token out of the config file by exporting `OHAYOU_CF_API_TOKEN`, which
overrides `cloudflare.apiToken`. Under systemd that means an `EnvironmentFile`:

```sh
install -m 600 /dev/null /opt/ohayoubot/deerkins.env
echo 'OHAYOU_CF_API_TOKEN=...' > /opt/ohayoubot/deerkins.env
chown ohayoubot:ohayoubot /opt/ohayoubot/deerkins.env
```

`deploy/ohayoubot.service` reads that file if it exists. Check it works with:

```sh
curl -sS -H "Authorization: Bearer $OHAYOU_CF_API_TOKEN" \
  -H 'content-type: application/json' \
  -d '{"sql":"SELECT COUNT(*) AS n FROM deer","params":[]}' \
  "https://api.cloudflare.com/client/v4/accounts/$ACCOUNT/d1/database/$DATABASE/query"
```

Requests are also subject to the bot-wide `ignoreList` and `floodDelay`.

## Drop

`!upload` PMs an identified user a one-shot link to the upload site. What they
drop there is announced in a channel the two of you share.

`OHAYOU_WEB_SECRET` signs the links. It belongs to the site rather than to this
plugin, so it is the bot's rather than drop's: see [Web](#web).

In the `drop` block, `url` is the site and `imageBase` is where the bucket is
served. `imageBase` must match the site's `PUBLIC_IMAGE_BASE` or every link the
bot announces is dead. `accountId`, `databaseId` and `apiToken` default to the
shared `cloudflare` block; set them once the upload tables move to their own
database.

## YouTube previews

When someone links a video the bot says what it is:

```
<someone> have you seen https://youtu.be/dQw4w9WgXcQ yet
<ohayoubot> YouTube: Rick Astley - Never Gonna Give You Up (Rick Astley)
```

It reads youtube's public [oembed][oembed] endpoint, which needs no account,
API key or quota, so there is nothing to set up. It is on by default; turn it
off with `"enabled": false` in the `youtube` block.

[oembed]: https://oembed.com/

`watch?v=`, `youtu.be`, `/shorts/`, `/live/`, `/embed/` and the `m.`, `music.`
and `youtube-nocookie.com` spellings are all recognized.

- `maxLinks` - how many videos one message may name (default `2`), so pasting a
  playlist costs a line or two and no more.
- `cooldown` - seconds a channel waits between previews (default `10`).
- `repeat` - seconds before the same video is worth naming again in the same
  channel (default `600`), so a link going round doesn't get announced each
  time.
- `requestTimeout` - ms for one lookup (default `8000`).
- `ignoreChannels` - channels to stay quiet in.

Titles are flattened to one line and trimmed to fit an IRC message, and the
plugin only ever sees lines that aren't commands, from nicks that aren't on the
`ignoreList`.

## Running as a service

```sh
sudo deploy/install.sh
```

This builds the binary, creates a locked-down `ohayoubot` system user, installs
everything under `/opt/ohayoubot`, and enables `deploy/ohayoubot.service`. It is
safe to re-run to deploy an update.

If the service is running it is stopped for the duration so the database can be
backed up and any pending migrations applied with exclusive access, then started
again. Before migrations run, a timestamped copy of the sqlite db is written to
`/opt/ohayoubot/backups/`; the newest 10 are kept and older ones pruned.

### Database migrations

Some catalog changes can't be expressed by re-seeding `items.json` alone. For
example renaming an item, which leaves the old row (and any inventories holding
it).

Those go in `deploy/migrations/` as numbered SQL files (`001_*.sql`,
`002_*.sql`, ...). On each `install.sh` run they are applied in numeric order.
Applied files are recorded in a `schema_migrations` table in the sqlite db so
re-runs do nothing else.

Requires the `sqlite3` cli.

Then:

```sh
sudo vim /opt/ohayoubot/conf.json   # fill in server, nick, SASL, admins, etc.
systemctl start ohayoubot
journalctl -u ohayoubot -f          # follow the logs
```

To install by hand instead, copy the binary, `data/`, and `conf.json` into
`/opt/ohayoubot`, then `cp deploy/ohayoubot.service /etc/systemd/system/` and
`systemctl daemon-reload && systemctl enable --now ohayoubot`. A hand install
skips the migration runner, so apply anything in `deploy/migrations/` yourself
(e.g. `sqlite3 /opt/ohayoubot/ohayoubot.db < deploy/migrations/001_*.sql`).

## Web

`web/` is the Cloudflare Pages project the bot's plugins are read from and, for
drop, write through: the gallery at `/deerkins/` and the uploader at `/drop/`.
It was its own repository until it grew past the one plugin it was named after.
See `web/README.md` for its databases, secrets and deploys.

`!web` PMs an identified user a one-shot link that signs them in to everything
on the site they can use: the link carries the scopes of the enabled plugins
that asked for one, so there is no link per plugin. `web.url` is the site's
front door and `OHAYOU_WEB_SECRET` signs the links.

The secret has no field in `conf.json` on purpose, so it cannot be committed by
accident. The site holds the same value as `UPLOAD_HMAC_SECRET`; both sides key
on the string's bytes, so it is used as written and not decoded from hex.
`openssl rand -hex 32`.

The Pages project builds with its root directory set to `web`, and keeps the
name `deerkins`: Pages does not rename a project in place, and the name is not
visible behind the custom domain.

## Dev

```sh
go test ./...
go vet ./...
gofmt -l .
```

```sh
cd web && pnpm install && pnpm test && pnpm lint
```
