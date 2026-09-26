#!/bin/bash
# Load an AWS backup (layout from the 2026-09-26 migration backup) into the
# VENUS stack: Postgres databases, product images (SeaweedFS S3), and CloudFront → local
# image URL rewrite. Verifies row counts and object counts; exits non-zero on
# any mismatch.
#
#   deploy/venus/import-backup.sh <backup-dir> [--replace]
#
# --replace drops and recreates the databases and re-copies all images (used at
# cutover with a fresh dump). Stop the app services first; this script only
# starts postgres and s3.
set -euo pipefail
BACKUP=$(realpath "${1:?backup dir}")
REPLACE=${2:-}
HERE=$(cd "$(dirname "$0")" && pwd)
ENV_FILE=${DUPLI1_HOME:-/opt/dupli1}/.env
DC=(docker compose -f "$HERE/docker-compose.yml" --env-file "$ENV_FILE")
DUMPS=$BACKUP/db/dupli1-rds
S3DIR=$BACKUP/s3/dupli1-product-images-20260712132109100000000003
OLD_IMAGE_BASE=https://d180wegphwork2.cloudfront.net
NEW_IMAGE_BASE=$(set -a; . "$ENV_FILE"; echo "${PUBLIC_ORIGIN:-https://dupli1.com}")/product-images

COUNT_SQL="SELECT n.nspname||'.'||c.relname||'|'||(xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM %I.%I', n.nspname, c.relname), false, true, '')))[1]::text FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p') AND n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' ORDER BY 1;"
psql() { "${DC[@]}" exec -T postgres psql -X -q -v ON_ERROR_STOP=1 -U postgres "$@"; }

"${DC[@]}" up -d --wait postgres s3

# ── Postgres ────────────────────────────────────────────────────────────────
echo "== postgres"
# The services connect as schick, the RDS master user, so ownership in the
# dumps restores as-is. Password comes from .env via the container env.
P=$(set -a; . "$ENV_FILE"; echo "$DB_APP_PASSWORD") \
  "${DC[@]}" exec -T -e P postgres sh -c 'psql -X -q -v ON_ERROR_STOP=1 -U postgres -v pw="$P" -f -' <<'SQL'
SELECT 'CREATE ROLE schick LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'schick') \gexec
ALTER ROLE schick PASSWORD :'pw';
SQL

fail=0
for db in $(cut -d'|' -f1 "$DUMPS/databases.txt"); do
  [ "$db" = postgres ] && continue
  exists=$(psql -At -c "SELECT 1 FROM pg_database WHERE datname = '$db'")
  if [ -n "$exists" ]; then
    [ "$REPLACE" = --replace ] || { echo "$db exists; pass --replace to overwrite"; exit 1; }
    psql -c "DROP DATABASE \"$db\" WITH (FORCE)"
  fi
  psql -c "CREATE DATABASE \"$db\" OWNER schick"
  # --no-acl: the RDS grants reference rds_* roles that don't exist here.
  "${DC[@]}" exec -T postgres pg_restore -U postgres --no-acl --exit-on-error -d "$db" < "$DUMPS/$db.dump"
  psql -At -d "$db" -c "$COUNT_SQL" > "/tmp/import-$db.counts"
  if diff -q "$DUMPS/$db.counts" "/tmp/import-$db.counts" >/dev/null; then
    echo "$db: restored, row counts match ($(wc -l < "$DUMPS/$db.counts") tables)"
  else
    echo "$db: ROW COUNTS DIFFER"; diff "$DUMPS/$db.counts" "/tmp/import-$db.counts" || true; fail=1
  fi
  rm -f "/tmp/import-$db.counts"

  # Image URLs are stored absolute. Rewrite every text-like column; triggers
  # off so updated_at-style triggers don't touch the rows.
  psql -d "$db" -v old="$OLD_IMAGE_BASE" -v new="$NEW_IMAGE_BASE" <<'SQL'
SELECT set_config('rewrite.old', :'old', false), set_config('rewrite.new', :'new', false) \g /dev/null
SET session_replication_role = replica;
DO $$
DECLARE r record; n bigint; total bigint := 0;
        o text := current_setting('rewrite.old'); w text := current_setting('rewrite.new');
BEGIN
  FOR r IN
    SELECT c.table_schema, c.table_name, c.column_name, format_type(a.atttypid, a.atttypmod) AS typ
    FROM information_schema.columns c
    JOIN pg_attribute a ON a.attrelid = format('%I.%I', c.table_schema, c.table_name)::regclass AND a.attname = c.column_name
    JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
    WHERE c.table_schema NOT IN ('pg_catalog', 'information_schema')
      AND (c.data_type IN ('text', 'character varying', 'json', 'jsonb')
           OR (c.data_type = 'ARRAY' AND c.udt_name IN ('_text', '_varchar')))
  LOOP
    EXECUTE format('UPDATE %I.%I SET %I = replace(%I::text, $1, $2)::%s WHERE strpos(%I::text, $1) > 0',
                   r.table_schema, r.table_name, r.column_name, r.column_name, r.typ, r.column_name)
      USING o, w;
    GET DIAGNOSTICS n = ROW_COUNT;
    IF n > 0 THEN
      RAISE NOTICE 'rewrote image URLs: %.%.% (% rows)', r.table_schema, r.table_name, r.column_name, n;
      total := total + n;
    END IF;
  END LOOP;
END $$;
SQL
done
left=$(for db in $(cut -d'|' -f1 "$DUMPS/databases.txt"); do "${DC[@]}" exec -T postgres pg_dump -U postgres -a "$db"; done | grep -c "$OLD_IMAGE_BASE" || true)
echo "CloudFront URLs remaining in any database: $left"; [ "$left" = 0 ] || fail=1

# ── S3 (SeaweedFS) ──────────────────────────────────────────────────────────
echo "== s3"
# boto3 on the stack network, so each object keeps the Content-Type (and
# Cache-Control) it had in S3 — keys have no extensions to guess from.
S3_SECRET_KEY=$(set -a; . "$ENV_FILE"; echo "$S3_SECRET_KEY") \
docker run --rm -i --network dupli1_default -e S3_SECRET_KEY \
  -v "$S3DIR:/src:ro" -v "$BACKUP/s3/object-metadata.tsv:/meta.tsv:ro" \
  python:3.13-slim sh -c 'pip -q install --root-user-action=ignore boto3 >/dev/null 2>&1 && python -' <<'EOF'
import os, boto3
from concurrent.futures import ThreadPoolExecutor
s3 = boto3.client("s3", endpoint_url="http://s3:8333", region_name="us-east-1",
                  aws_access_key_id="dupli1", aws_secret_access_key=os.environ["S3_SECRET_KEY"])
bucket = "product-images"
if bucket not in [b["Name"] for b in s3.list_buckets()["Buckets"]]:
    s3.create_bucket(Bucket=bucket)
rows = [l.rstrip("\n").split("\t") for l in open("/meta.tsv")]
def put(row):
    key, ctype, cache = row
    extra = {"ContentType": ctype}
    if cache != "None":
        extra["CacheControl"] = cache
    with open(f"/src/{key}", "rb") as f:
        s3.put_object(Bucket=bucket, Key=key, Body=f, **extra)
with ThreadPoolExecutor(8) as ex:
    list(ex.map(put, rows))
n = size = 0
for page in s3.get_paginator("list_objects_v2").paginate(Bucket=bucket):
    for o in page.get("Contents", []):
        n += 1; size += o["Size"]
print(f"uploaded {len(rows)}; bucket now: {n} objects / {size} bytes")
EOF
python3 - "$BACKUP" <<'EOF'
import json, sys
o = json.load(open(f"{sys.argv[1]}/s3/source-listing.json"))["Contents"]
print(f"source: {len(o)} objects / {sum(x['Size'] for x in o)} bytes")
EOF

[ $fail = 0 ] && echo "IMPORT OK" || { echo "IMPORT HAD PROBLEMS (see above)"; exit 1; }
