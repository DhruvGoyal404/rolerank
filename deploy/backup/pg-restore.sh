#!/usr/bin/env bash
# Restores a backup set made by pg-backup.sh. Three modes:
#   pg-restore.sh <stamp> --check "<counts>"   the drill: restore into scratch databases and
#                                              compare every app table's count with <counts>
#   pg-restore.sh <stamp> --into-live          replace the live app and corpus databases, then
#                                              re-apply deletion tombstones
#   pg-restore.sh <stamp> --corpus-only        replace only the live corpus (the first up, from
#                                              the seed scripts/seed-corpus.sh uploaded)
# <stamp> names s3://$BUCKET/rolerank/pg/rolerank-<stamp>-{app,corpus}.dump.age. Each file's
# SHA-256 is checked against its metadata before decryption. The age identity is read from SSM
# (SecureString) into a 0600 file that is removed on exit. Prints "drill-counts: ..." or
# "restored-counts: ...".
set -euo pipefail
: "${BUCKET:?backup.env}"
export AWS_REGION=${AWS_REGION:-ap-south-1}
stamp=${1:?usage: pg-restore.sh <stamp> --check <counts> | --into-live | --corpus-only}
mode=${2:?--check, --into-live or --corpus-only}
dbs=(app corpus)
[ "$mode" != --corpus-only ] || dbs=(corpus)
dir=/var/lib/rolerank/backup
pg=$(docker ps -q --filter label=com.docker.compose.project=rolerank-prod --filter label=com.docker.compose.service=postgres)
[ -n "$pg" ] || { echo "pg-restore: postgres is not running" >&2; exit 1; }
psql() { docker exec "$pg" psql -U postgres -XAt -v ON_ERROR_STOP=1 "$@"; }
umask 077
identity=$(mktemp "$dir/identity.XXXXXX")
trap 'rm -f "$identity" "$dir"/restore-*.dump "$dir"/restore-*.age' EXIT
aws ssm get-parameter --name /rolerank/prod/age_identity --with-decryption --query Parameter.Value --output text >"$identity"

for db in "${dbs[@]}"; do
  key="rolerank/pg/rolerank-$stamp-$db.dump.age"
  aws s3 cp "s3://$BUCKET/$key" "$dir/restore-$db.age" --only-show-errors
  want=$(aws s3api head-object --bucket "$BUCKET" --key "$key" --query Metadata.sha256 --output text)
  [ "$(sha256sum "$dir/restore-$db.age" | cut -d' ' -f1)" = "$want" ] || { echo "pg-restore: $key checksum mismatch" >&2; exit 1; }
  age -d -i "$identity" -o "$dir/restore-$db.dump" "$dir/restore-$db.age"
  chmod 0644 "$dir/restore-$db.dump" # read by the postgres user inside the container
done

counts_of() {
  psql -d "$1" -c "SELECT string_agg(format('%s=%s', t, (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM public.%I', t), false, true, '')))[1]::text), ' ' ORDER BY t) FROM unnest(ARRAY(SELECT tablename::text FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations')) AS t"
}

case "$mode" in
  --check)
    expected=${3:?--check needs the counts recorded at backup time}
    for db in app corpus; do
      psql -d postgres -c "DROP DATABASE IF EXISTS drill_$db WITH (FORCE)" -c "CREATE DATABASE drill_$db" >/dev/null
      docker exec "$pg" pg_restore -U postgres -j 4 --no-owner --no-privileges -d "drill_$db" "/backup/restore-$db.dump"
    done
    got=$(counts_of drill_app)
    corpus=$(psql -d drill_corpus -c "SELECT count(*) FROM postings")
    psql -d postgres -c "DROP DATABASE drill_app WITH (FORCE)" -c "DROP DATABASE drill_corpus WITH (FORCE)" >/dev/null
    echo "drill-counts: $got"
    echo "drill-corpus-postings: $corpus"
    [ "$got" = "$expected" ] || { echo "pg-restore: drill counts differ from the backup's: $expected" >&2; exit 1; }
    [ "$corpus" -gt 0 ] || { echo "pg-restore: the corpus restored empty" >&2; exit 1; }
    echo "pg-restore: drill passed"
    ;;
  --into-live)
    for db in app corpus; do
      # Roles and databases exist from first boot (db/provision); objects come back with owners.
      docker exec "$pg" pg_restore -U postgres -j 4 --clean --if-exists -d "$db" "/backup/restore-$db.dump"
    done
    # A deleted user must not come back from a backup (CLAUDE.md, Privacy).
    psql -d app -c "DELETE FROM users WHERE id IN (SELECT user_id FROM deletion_tombstones)" \
      -c "DELETE FROM llm_ledger WHERE scope IN (SELECT user_id::text FROM deletion_tombstones)" >/dev/null
    psql -d corpus -c "ANALYZE" >/dev/null
    echo "restored-counts: $(counts_of app)"
    ;;
  --corpus-only)
    started=$(date +%s)
    docker exec "$pg" pg_restore -U postgres -j 4 --clean --if-exists -d corpus /backup/restore-corpus.dump
    # Fresh planner statistics: the restored indexes are only used well once the planner knows the data.
    psql -d corpus -c "ANALYZE" >/dev/null
    echo "restored-corpus-postings: $(psql -d corpus -c "SELECT count(*) FROM postings")"
    echo "restored-corpus-seconds: $(($(date +%s) - started))"
    ;;
  *) echo "pg-restore: unknown mode $mode" >&2; exit 2 ;;
esac
