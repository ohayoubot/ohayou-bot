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
   - `deerkins` - the pixel art gallery. Off unless `accountId`, `databaseId`
     and a token are all present. See [Deerkins](#deerkins).
   - `drop` - image uploads. Off unless `OHAYOU_DROP_SECRET` is set and `url`
     is filled in. See [Drop](#drop).
2. Build and run:
   ```sh
   go build ./cmd/ohayoubot
   ./ohayoubot -config conf.json -data data
   ```

On first run the item catalog in `data/items.json` is seeded into the database.
Running `sudo deploy/install.sh` will update prices/descriptions/etc if run
against an already seeded database.

## Deerkins

`!deerme` paints art from the [deerkins](https://github.com/ohayoubot/deerkins)
gallery into the channel. The drawings live in a Cloudflare D1 database that the
web app writes. The bot just reads from it.

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

The bot talks to the D1 HTTP API, so it needs three things:

- `accountId` - Cloudflare account id, on the right of the dashboard overview.
- `databaseId` - from `npx wrangler d1 info deerkins`, or the `database_id`
  already in the gallery's `wrangler.toml`.
- an API token with **Account → D1 → Read** and nothing else. Create it under
  My Profile → API Tokens → Create Token → Create Custom Token, scoped to the
  one account. `Read` is enough because the bot only runs `SELECT`s; a token
  that cannot write cannot deface the gallery if the bot is ever compromised.

Keep the token out of the config file by exporting `DEERKINS_API_TOKEN`, which
overrides `apiToken`. Under systemd that means an `EnvironmentFile`:

```sh
install -m 600 /dev/null /opt/ohayoubot/deerkins.env
echo 'DEERKINS_API_TOKEN=...' > /opt/ohayoubot/deerkins.env
chown ohayoubot:ohayoubot /opt/ohayoubot/deerkins.env
```

`deploy/ohayoubot.service` reads that file if it exists. Check it works with:

```sh
curl -sS -H "Authorization: Bearer $DEERKINS_API_TOKEN" \
  -H 'content-type: application/json' \
  -d '{"sql":"SELECT COUNT(*) AS n FROM deer","params":[]}' \
  "https://api.cloudflare.com/client/v4/accounts/$ACCOUNT/d1/database/$DATABASE/query"
```

Requests are also subject to the bot-wide `ignoreList` and `floodDelay`.

## Drop

`!upload` PMs an identified user a one-shot link to the upload site. What they
drop there is announced in a channel the two of you share.

Two environment variables, neither in `conf.json`:

- `OHAYOU_DROP_SECRET` signs the links. The site holds the same value as
  `DROP_HMAC_SECRET`. Both sides key on the string's bytes, so it is used
  as written and not decoded from hex. `openssl rand -hex 32`.
- `OHAYOU_DROP_TOKEN` is optional; without it the deerkins **D1 Read** token is
  used, which is right while both read one database.

In the `drop` block, `url` is the site and `imageBase` is where the bucket is
served. `imageBase` must match the site's `PUBLIC_IMAGE_BASE` or every link the
bot announces is dead. `accountId` and `databaseId` default to the deerkins
block; set them once the upload tables move to their own database.

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

## Dev

```sh
go test ./...
go vet ./...
gofmt -l .
```
