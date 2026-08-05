#!/usr/bin/env bash
#
# Build ohayoubot and install it as a systemd service under /opt/ohayoubot.
#
# Rerunning will rebuild and replace binary, data files, and systemd unit, but
# NEVER the existing conf.json or sqlite db.
#
# Usage: sudo deploy/install.sh [--yes]
#
#   --yes   restart a running bot without asking
set -euo pipefail

PREFIX=/opt/ohayoubot
SVC_USER=ohayoubot
UNIT=/etc/systemd/system/ohayoubot.service
BACKUPS_KEEP=10
ASSUME_YES=no

for arg in "$@"; do
	case "$arg" in
	-y | --yes) ASSUME_YES=yes ;;
	-h | --help)
		sed -n '3,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $arg" >&2
		exit 1
		;;
	esac
done

if [[ $EUID -ne 0 ]]; then
	echo "This installer must run as root: sudo $0" >&2
	exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# take a snapshot of the current db, if one exists. if not does nothing
backup_db() {
	local db="$1" dir="$2" dest line

	[[ -f "$db" ]] || return 0

	install -d -o "$SVC_USER" -g "$SVC_USER" -m 0700 "$dir"
	dest="$dir/ohayoubot-$(date +%Y%m%d-%H%M%S).db"
	echo ">> Backing up database to $dest..."
	if command -v sqlite3 >/dev/null; then
		sqlite3 "$db" ".backup '$dest'"
	else
		cp -p "$db" "$dest"
	fi
	chown "$SVC_USER":"$SVC_USER" "$dest"
	chmod 0600 "$dest"

	# Prune oldest-first down to the newest $BACKUPS_KEEP snapshots.
	local backups=()
	while IFS= read -r line; do
		backups+=("$line")
	done < <(ls -1 "$dir"/ohayoubot-*.db 2>/dev/null | sort)
	local excess=$(( ${#backups[@]} - BACKUPS_KEEP ))
	if (( excess > 0 )); then
		echo "   Pruning $excess old backup(s), keeping the newest $BACKUPS_KEEP."
		rm -f -- "${backups[@]:0:excess}"
	fi
}

apply_migrations() {
	local db="$1" dir="$2" f id

	[[ -d "$dir" ]] || return 0
	# only ordered migrations (001_..., 002_...) are applied & in numeric order.
	shopt -s nullglob
	local files=("$dir"/[0-9][0-9][0-9]_*.sql)
	shopt -u nullglob
	[[ ${#files[@]} -gt 0 ]] || return 0

	if [[ ! -f "$db" ]]; then
		echo ">> No database at $db yet; migrations will apply on a later run."
		return 0
	fi
	if ! command -v sqlite3 >/dev/null; then
		echo "!! sqlite3 not found on PATH; skipping ${#files[@]} pending migration(s)." >&2
		echo "   Install sqlite3 and re-run '$0' to apply them." >&2
		return 0
	fi

	sqlite3 "$db" "CREATE TABLE IF NOT EXISTS schema_migrations (
		id TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	);"

	while IFS= read -r f; do
		id="$(basename "$f")"
		if [[ -n "$(sqlite3 "$db" "SELECT 1 FROM schema_migrations WHERE id='$id' LIMIT 1;")" ]]; then
			continue
		fi
		echo ">> Applying migration $id..."
		if ! sqlite3 -bail "$db" <<-SQL
			BEGIN;
			$(cat "$f")
			INSERT INTO schema_migrations(id) VALUES('$id');
			COMMIT;
		SQL
		then
			echo "!! Migration $id failed; rolled back. Fix it and re-run '$0'." >&2
			exit 1
		fi
	done < <(printf '%s\n' "${files[@]}" | sort)

	echo "   Database migrations are up to date."
}

command -v go >/dev/null || { echo "go toolchain not found on PATH" >&2; exit 1; }
nologin_path="$(command -v nologin || echo /usr/sbin/nologin)"

echo ">> Building binary (pure Go, no CGO)..."
( cd "$repo_root" && CGO_ENABLED=0 go build -o ohayoubot ./cmd/ohayoubot )

echo ">> Ensuring system user '$SVC_USER'..."
if ! id -u "$SVC_USER" >/dev/null 2>&1; then
	useradd --system --home-dir "$PREFIX" --shell "$nologin_path" "$SVC_USER"
fi

echo ">> Installing files to $PREFIX..."
install -d -o "$SVC_USER" -g "$SVC_USER" "$PREFIX" "$PREFIX/data"
install -o "$SVC_USER" -g "$SVC_USER" -m 0755 "$repo_root/ohayoubot" "$PREFIX/ohayoubot"
install -o "$SVC_USER" -g "$SVC_USER" -m 0644 "$repo_root"/data/* "$PREFIX/data/"

if [[ ! -f "$PREFIX/conf.json" ]]; then
	install -o "$SVC_USER" -g "$SVC_USER" -m 0600 "$repo_root/conf-example.json" "$PREFIX/conf.json"
	echo "   Wrote $PREFIX/conf.json from the example. Edit it before starting."
else
	echo "   Kept existing $PREFIX/conf.json."
fi

echo ">> Installing systemd unit..."
install -m 0644 "$repo_root/deploy/ohayoubot.service" "$UNIT"
systemctl daemon-reload
systemctl enable ohayoubot.service

# Load the config the way the service will, and refuse to go further if it will
# not come up. Without this a bad config is only found after the restart, as a
# systemd restart loop.
check_config() {
	local prefix="$1"
	local env_file="$prefix/ohayoubot.env"

	if [[ ! -f "$prefix/conf.json" ]]; then
		echo ">> No conf.json yet; skipping the config check."
		return 0
	fi

	echo ">> Checking the config..."
	# In a subshell, so the credentials do not outlive the check.
	(
		set -a
		# shellcheck disable=SC1090
		[[ -f "$env_file" ]] && . "$env_file"
		set +a
		cd "$prefix" && ./ohayoubot -check -config "$prefix/conf.json"
	) || {
		echo "!! The config is not usable. Nothing was restarted." >&2
		echo "   Fix $prefix/conf.json (or $env_file) and re-run '$0'." >&2
		exit 1
	}
}

# Asks before taking a running bot down. The migrations need it stopped, so this
# comes before the stop rather than before the start: answering no leaves it
# running on the old build with the new one installed but not loaded.
confirm_restart() {
	local reply

	[[ "$ASSUME_YES" == yes ]] && return 0
	if [[ ! -t 0 ]]; then
		echo ">> Not a terminal, so nothing was restarted. Re-run with --yes."
		return 1
	fi

	read -r -p "Stop ohayoubot, back up the database, apply migrations and restart? [y/N] " reply
	[[ "$reply" == [yY] || "$reply" == [yY][eE][sS] ]]
}

# Resolve the sqlite path from conf.json (defaulting to ohayoubot.db), relative
# to the service WorkingDirectory ($PREFIX) unless it is already absolute.
db_path="$(sed -n 's/.*"database"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$PREFIX/conf.json" 2>/dev/null | head -n1)"
db_path="${db_path:-ohayoubot.db}"
[[ "$db_path" = /* ]] || db_path="$PREFIX/$db_path"

# Apply pending DB migrations with the service stopped for exclusive write
# access, then restore whatever run state it was in. A stopped bot re-seeds the
# catalog (picking up new prices/items) on its next start.
check_config "$PREFIX"

was_active=no
if systemctl is-active --quiet ohayoubot.service; then
	if ! confirm_restart; then
		echo
		echo "Done. The new build is installed; the bot is still on the old one."
		echo "  Apply it with: sudo $0 --yes"
		exit 0
	fi
	was_active=yes
	echo ">> Stopping ohayoubot to back up the database and apply migrations..."
	systemctl stop ohayoubot.service
fi

backup_db "$db_path" "$PREFIX/backups"
apply_migrations "$db_path" "$repo_root/deploy/migrations"

if [[ "$was_active" == yes ]]; then
	systemctl start ohayoubot.service
	echo "   Restarted ohayoubot on the new build; catalog re-seeded."
else
	echo "   Start the bot when ready with: systemctl start ohayoubot"
fi

echo
echo "Done."
echo "  1. Edit the config:   sudoedit $PREFIX/conf.json"
echo "  2. Start the bot:     systemctl start ohayoubot"
echo "  3. Watch the logs:    journalctl -u ohayoubot -f"
