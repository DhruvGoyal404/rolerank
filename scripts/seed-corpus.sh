#!/usr/bin/env bash
# Seeds production's corpus from the local one, for the first up (owner's decision, 2026-11-09).
# The corpus holds postings only, never user data (CLAUDE.md, "Database"); this script dumps the
# `corpus` database alone, checks that it holds no `app` table, encrypts the dump with the
# production backup key (age, public half from SSM), and uploads it with its SHA-256 to
# s3://<backup bucket>/rolerank/seed/. rolerank-up.sh restores the newest seed when there is no
# backup yet. Reads the local database only; prints no secret.
#
#   bash scripts/seed-corpus.sh
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/rolerank-lib.sh"
for c in docker age; do command -v "$c" >/dev/null || die "$c is not installed (age: winget install FiloSottile.age)"; done
aws sts get-caller-identity >/dev/null 2>&1 || aws sso login --profile "$AWS_PROFILE" || die "aws sso login failed"
stack_exists "$PERSISTENT" || die "deploy $PERSISTENT first (rolerank-up.sh does it): the seed goes in its bucket"
BUCKET=$(bucket)
cd "$ROOT"

log "1/3 Dump the local corpus"
psql() { docker compose exec -T postgres psql -U postgres -XAt -v ON_ERROR_STOP=1 "$@"; }
[ "$(psql -d corpus -c "SELECT count(*) FROM pg_tables WHERE tablename IN ('users', 'identities', 'resumes', 'profiles')")" = 0 ] ||
  die "the corpus database holds an app table: refusing to upload it"
postings=$(psql -d corpus -c "SELECT count(*) FROM postings")
embedded=$(psql -d corpus -c "SELECT count(*) FROM search_postings WHERE embedding IS NOT NULL")
stamp=$(date -u +%Y%m%dT%H%M%SZ)
t=$(date +%s)
docker compose exec -T postgres pg_dump -U postgres -Fc -d corpus -f /tmp/seed.dump
docker compose cp postgres:/tmp/seed.dump "$WORK/seed.dump" >/dev/null
docker compose exec -T postgres rm -f /tmp/seed.dump
echo "corpus: $postings postings, $embedded searchable; dump $(du -h "$WORK/seed.dump" | cut -f1) in $(($(date +%s) - t)) s" >&2

log "2/3 Encrypt with the production backup key"
recipient=$(param /rolerank/prod/age_recipient) || die "no /rolerank/prod/age_recipient: run scripts/prod-secrets.sh"
age -r "$recipient" -o "$WORK/seed.dump.age" "$WORK/seed.dump"
rm -f "$WORK/seed.dump"
sum=$(sha256sum "$WORK/seed.dump.age" | cut -d' ' -f1)

log "3/3 Upload"
key="rolerank/seed/rolerank-$stamp-corpus.dump.age"
aws_ s3 cp "$(winpath "$WORK/seed.dump.age")" "s3://$BUCKET/$key" --sse AES256 --metadata "sha256=$sum,postings=$postings" --only-show-errors
[ "$(aws_ s3api head-object --bucket "$BUCKET" --key "$key" --query Metadata.sha256 --output text)" = "$sum" ] || die "the upload lost its checksum"
echo "seeded s3://<backup bucket>/$key ($postings postings, sha256 ${sum:0:12}...)" >&2
