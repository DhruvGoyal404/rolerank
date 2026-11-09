#!/usr/bin/env bash
# Backs up both databases: pg_dump (custom format) of app and corpus, encrypted with age to the
# public key in SSM, uploaded to s3://$BUCKET/rolerank/pg/ with each file's SHA-256 in its
# metadata. The host role may only put and get under that prefix, and Object Lock keeps
# objects from deletion for the retention period. Prints the row count of every app table at
# dump time ("counts: ..."), which the restore drill must reproduce exactly. Publishes
# RoleRank/BackupSucceeded=1 on success. Plaintext dumps never leave the host and are removed.
set -euo pipefail
: "${BUCKET:?backup.env}" "${AGE_RECIPIENT:?backup.env}"
export AWS_REGION=${AWS_REGION:-ap-south-1}
dir=/var/lib/rolerank/backup
ts=$(date -u +%Y%m%dT%H%M%SZ)
pg=$(docker ps -q --filter label=com.docker.compose.project=rolerank-prod --filter label=com.docker.compose.service=postgres)
[ -n "$pg" ] || { echo "pg-backup: postgres is not running" >&2; exit 1; }
psql() { docker exec "$pg" psql -U postgres -XAt "$@"; }
trap 'rm -f "$dir"/*.dump' EXIT

# Every table's exact count, in one snapshot with the dump would be ideal; down stops writers
# first, so the counts and the dump agree there. Nightly counts are for information.
counts=$(psql -d app -c "SELECT string_agg(format('%s=%s', t, (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM public.%I', t), false, true, '')))[1]::text), ' ' ORDER BY t) FROM unnest(ARRAY(SELECT tablename::text FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations')) AS t")
for db in app corpus; do
  docker exec "$pg" pg_dump -U postgres -Fc -d "$db" -f "/backup/$db-$ts.dump"
  age -r "$AGE_RECIPIENT" -o "$dir/$db-$ts.dump.age" "$dir/$db-$ts.dump"
  rm -f "$dir/$db-$ts.dump"
  sum=$(sha256sum "$dir/$db-$ts.dump.age" | cut -d' ' -f1)
  key="rolerank/pg/rolerank-$ts-$db.dump.age"
  aws s3 cp "$dir/$db-$ts.dump.age" "s3://$BUCKET/$key" --sse AES256 --metadata "sha256=$sum" --only-show-errors
  rm -f "$dir/$db-$ts.dump.age"
  echo "pg-backup: uploaded s3://$BUCKET/$key"
done
echo "counts: $counts"
aws cloudwatch put-metric-data --namespace RoleRank --metric-name BackupSucceeded --value 1 --unit Count
echo "pg-backup: metric published"
