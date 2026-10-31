#!/bin/sh
# First-boot provisioning, run by the Postgres image from docker-entrypoint-initdb.d.
set -eu
render() {
  sed -e "s/__P__//g" -e "s/__APP_DB__/app/g" -e "s/__CORPUS_DB__/corpus/g" \
    -e "s/__PW_MIGRATOR_APP__/$ROLERANK_PW_MIGRATOR_APP/g" \
    -e "s/__PW_MIGRATOR_CORPUS__/$ROLERANK_PW_MIGRATOR_CORPUS/g" \
    -e "s/__PW_APP_RW__/$ROLERANK_PW_APP_RW/g" \
    -e "s/__PW_CORPUS_RW__/$ROLERANK_PW_CORPUS_RW/g" \
    -e "s/__PW_CORPUS_RO__/$ROLERANK_PW_CORPUS_RO/g" "$1"
}
dir=/opt/rolerank/provision
render "$dir/cluster.sql.tmpl" | psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres -q
render "$dir/app.sql.tmpl" | psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d app -q
render "$dir/corpus.sql.tmpl" | psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d corpus -q
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres -q \
  -c "REVOKE CONNECT ON DATABASE postgres FROM PUBLIC" \
  -c "REVOKE CONNECT ON DATABASE template1 FROM PUBLIC"
