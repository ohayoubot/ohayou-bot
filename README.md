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
2. Build and run:
   ```sh
   go build ./cmd/ohayoubot
   ./ohayoubot -config conf.json -data data
   ```

On first run the item catalog in `data/items.json` is seeded into the database.
Running `sudo deploy/install.sh` will update prices/descriptions/etc if run
against an already seeded database.

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
