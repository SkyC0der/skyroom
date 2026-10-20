#!/bin/sh
# Produces a temporary logical backup and verifies that PostgreSQL can parse
# its archive and that Skyroom's critical durable tables are present.
set -eu

if [ -z "${DATABASE_URL:-}" ]; then
	echo "DATABASE_URL is required" >&2
	exit 2
fi
command -v pg_dump >/dev/null 2>&1 || { echo "pg_dump is required" >&2; exit 2; }
command -v pg_restore >/dev/null 2>&1 || { echo "pg_restore is required" >&2; exit 2; }

backup_dir="$(mktemp -d)"
trap 'rm -rf "$backup_dir"' EXIT HUP INT TERM
archive="$backup_dir/skyroom.dump"
contents="$backup_dir/contents.txt"

pg_dump --dbname="$DATABASE_URL" --format=custom --no-owner --no-privileges --file="$archive"
pg_restore --list "$archive" >"$contents"

for table in users sessions communities members rooms messages outbox_events read_cursors; do
	if ! grep -Eq "TABLE( DATA)? public $table " "$contents"; then
		echo "backup verification failed: $table is missing" >&2
		exit 1
	fi
done

size="$(wc -c <"$archive" | tr -d ' ')"
echo "Skyroom backup archive verified ($size bytes; critical tables present)"
