#!/bin/bash
# Rewrite stored product image URLs from absolute to domain-free:
#   https://dupli1.com/product-images/<key>  →  /product-images/<key>
# so a domain move (dupli1.com → dupli2.com → …) no longer breaks every image.
# The edge serves /product-images/ on every host, so the browser resolves the
# path against whichever domain the page came from.
#
#   deploy/venus/rewrite-image-urls.sh            # dry run: count what would change
#   deploy/venus/rewrite-image-urls.sh --apply    # back up, then rewrite
#
# Scans every text, varchar, json/jsonb and text[] column in every database:
# the URLs live in product (products, product_variants) and also in snapshots
# such as order's order_items.image_url.
#
# --apply first writes a pg_dump -Fc of each database it is about to change to
# $DUPLI1_HOME/backups/image-urls-<timestamp>/ and checks it is readable; any
# failure stops before the database is touched. Each database is then rewritten
# in one transaction. Restore one with:
#   docker compose … exec -T postgres pg_restore -U postgres --clean --if-exists -d <db> < <db>.dump
#
# Set S3_PUBLIC_ENDPOINT to /product-images first (docker-compose.yml), or new
# uploads keep writing absolute URLs.
set -euo pipefail
MODE=${1:-}
case $MODE in ''|--apply) ;; *) echo "usage: $0 [--apply]" >&2; exit 2 ;; esac
HERE=$(cd "$(dirname "$0")" && pwd)
DUPLI1_HOME=${DUPLI1_HOME:-/opt/dupli1}
ENV_FILE=$DUPLI1_HOME/.env
DC=(docker compose -f "$HERE/docker-compose.yml" --env-file "$ENV_FILE")
# Any scheme and host in front of /product-images/. Quotes and spaces end the
# host so a match never spans two array elements or JSON strings.
PATTERN='https?://[^/"[:space:]]+/product-images/'
psql() { "${DC[@]}" exec -T postgres psql -X -q -v ON_ERROR_STOP=1 -U postgres "$@"; }

# Every column an image URL can sit in, as "schema|table|column|type".
COLUMNS_SQL="SELECT c.table_schema||'|'||c.table_name||'|'||c.column_name||'|'||format_type(a.atttypid, a.atttypmod)
FROM information_schema.columns c
JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
JOIN pg_attribute a ON a.attrelid = format('%I.%I', c.table_schema, c.table_name)::regclass AND a.attname = c.column_name
WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema')
  AND (c.data_type IN ('text', 'character varying', 'json', 'jsonb')
       OR (c.data_type = 'ARRAY' AND c.udt_name IN ('_text', '_varchar')))
ORDER BY 1"

# Prints "db|schema.table.column|rows" for every column holding a match.
scan() {
  local db=$1 col s t c typ n
  while IFS= read -r col; do
    [ -n "$col" ] || continue
    IFS='|' read -r s t c typ <<<"$col"
    # psql only substitutes :'p' in input it reads, not in -c.
    n=$(psql -At -d "$db" -v p="$PATTERN" <<<"SELECT count(*) FROM \"$s\".\"$t\" WHERE \"$c\"::text ~ :'p'")
    [ "$n" != 0 ] && echo "$db|$s.$t.$c|$n"
  done < <(psql -At -d "$db" -c "$COLUMNS_SQL")
  return 0
}

"${DC[@]}" up -d --wait postgres >/dev/null
DBS=$(psql -At -c "SELECT datname FROM pg_database WHERE NOT datistemplate AND datname <> 'postgres' ORDER BY 1")

echo "== absolute image URLs (pattern $PATTERN)"
FOUND=$(for db in $DBS; do scan "$db"; done)
if [ -z "$FOUND" ]; then echo "none — nothing to do"; exit 0; fi
echo "$FOUND" | awk -F'|' '{ printf "  %-12s %-45s %s rows\n", $1, $2, $3 }'
TARGETS=$(echo "$FOUND" | cut -d'|' -f1 | sort -u)

if [ "$MODE" != --apply ]; then
  echo "dry run — rerun with --apply to back up and rewrite: $(echo $TARGETS)"
  exit 0
fi

# ── Backup ──────────────────────────────────────────────────────────────────
BACKUP_DIR=$DUPLI1_HOME/backups/image-urls-$(date +%Y%m%d-%H%M%S)
mkdir -p "$BACKUP_DIR"
echo "== backup → $BACKUP_DIR"
for db in $TARGETS; do
  "${DC[@]}" exec -T postgres pg_dump -U postgres -Fc "$db" > "$BACKUP_DIR/$db.dump"
  # A dump pg_restore can't list is no backup; stop before touching anything.
  tables=$("${DC[@]}" exec -T postgres pg_restore -l < "$BACKUP_DIR/$db.dump" | grep -c ' TABLE DATA ') ||
    { echo "$db: backup unreadable, nothing changed" >&2; exit 1; }
  echo "  $db.dump  $(du -h "$BACKUP_DIR/$db.dump" | cut -f1)  ($tables tables)"
done
echo "$FOUND" > "$BACKUP_DIR/rewritten-columns.txt"

# ── Rewrite ─────────────────────────────────────────────────────────────────
# One DO block per database is one transaction: a failure leaves it as it was.
# Triggers off so updated_at-style triggers don't touch the rows.
echo "== rewrite"
for db in $TARGETS; do
  psql -d "$db" -v p="$PATTERN" <<'SQL'
SELECT set_config('rewrite.pattern', :'p', false) \g /dev/null
SET session_replication_role = replica;
DO $$
DECLARE r record; n bigint; p text := current_setting('rewrite.pattern');
BEGIN
  FOR r IN
    SELECT c.table_schema, c.table_name, c.column_name, format_type(a.atttypid, a.atttypmod) AS typ
    FROM information_schema.columns c
    JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
    JOIN pg_attribute a ON a.attrelid = format('%I.%I', c.table_schema, c.table_name)::regclass AND a.attname = c.column_name
    WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema')
      AND (c.data_type IN ('text', 'character varying', 'json', 'jsonb')
           OR (c.data_type = 'ARRAY' AND c.udt_name IN ('_text', '_varchar')))
  LOOP
    EXECUTE format('UPDATE %I.%I SET %I = regexp_replace(%I::text, $1, %L, %L)::%s WHERE %I::text ~ $1',
                   r.table_schema, r.table_name, r.column_name, r.column_name,
                   '/product-images/', 'g', r.typ, r.column_name)
      USING p;
    GET DIAGNOSTICS n = ROW_COUNT;
    IF n > 0 THEN
      RAISE NOTICE 'rewrote %.%.% (% rows)', r.table_schema, r.table_name, r.column_name, n;
    END IF;
  END LOOP;
END $$;
SQL
done

LEFT=$(for db in $TARGETS; do scan "$db"; done)
if [ -n "$LEFT" ]; then
  echo "ABSOLUTE URLS REMAIN:"; echo "$LEFT"; exit 1
fi
echo "OK — no absolute image URLs left. Backup: $BACKUP_DIR"
