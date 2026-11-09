#!/usr/bin/env bash
# shellcheck disable=SC2016 # host scripts are single-quoted on purpose; local values are spliced in
# Takes RoleRank down so nothing bills while idle (docs/ARCHITECTURE.md, "Deployment design"):
#   1. writers stopped, then a fresh encrypted backup of both databases and a restore drill
#      from it that must reproduce every app table's row count; Caddy's certificates too.
#      Any failure stops here: nothing is removed, and the writers are started again;
#   2. the backups are verified in S3 and kept under rolerank/final/ with their counts, and
#      the live release's manifest is saved so up can redeploy it without a build;
#   3. the backup alarm's actions are disabled and DNS is parked at 192.0.2.1;
#   4. the host stack (EC2, EBS, Elastic IP) is destroyed; registries and backups stay;
#   5. an audit of everything of RoleRank's that can still bill ("STILL BILLING", exit 1).
# Safe to run again after a half-failed run.
#
#   ./rolerank-down.sh
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/scripts/rolerank-lib.sh"

preflight
cf_check
BUCKET=$(bucket)
ID=$(host_id)

if [ -n "$ID" ]; then
  log "1/5 Stop writers, back up, restore drill, Caddy data"
  out=$(ssm_run "$ID" '
set -euo pipefail
set -a; . /etc/rolerank/backup.env; set +a
compose() { '"$HOST_COMPOSE"' "$@"; }
restart_writers() { systemctl start rolerank-extractor.service; compose start api aiworker ingest >/dev/null 2>&1 || true; }
trap restart_writers ERR
# Nothing writes while the backup and its counts are taken, so the two agree exactly.
compose stop api aiworker ingest >/dev/null
systemctl stop rolerank-extractor.service
start=$(date +%s)
systemctl start rolerank-backup.service
journal=$(journalctl -u rolerank-backup.service --since "@$start" -o cat --no-pager)
stamp=$(sed -n "s/^pg-backup: uploaded s3:\/\/.*rolerank-\([0-9TZ]*\)-app\.dump\.age$/\1/p" <<<"$journal" | tail -n 1)
counts=$(sed -n "s/^counts: //p" <<<"$journal" | tail -n 1)
[ -n "$stamp" ] && [ -n "$counts" ] || { echo "the backup ran but reported no upload"; exit 1; }
echo "RESULT stamp=$stamp"
echo "RESULT counts=$counts"
/opt/rolerank/current/app/deploy/backup/pg-restore.sh "$stamp" --check "$counts"
vol='"$COMPOSE_PROJECT"'_caddy_data
dir=$(docker volume inspect -f "{{.Mountpoint}}" "$vol")
certs=$(find "$dir" -path "*/certificates/*" -name "*.crt" | wc -l)
if [ "$certs" -ge 1 ]; then
  tgz=$(mktemp /var/lib/rolerank/backup/caddy.XXXXXX.tgz)
  tar -C "$dir" -czf "$tgz" .
  sha=$(sha256sum "$tgz" | cut -d" " -f1)
  aws s3 cp "$tgz" "s3://$BUCKET/'"$NIGHTLY_PREFIX"'caddy-$stamp.tar.gz" --sse AES256 --metadata "sha256=$sha" --only-show-errors
  rm -f "$tgz"
  echo "RESULT caddy=caddy-$stamp.tar.gz"
fi
echo "RESULT release=$(base64 -w0 /opt/rolerank/state/current.json)"
') || die "backup or restore drill failed on the host: nothing was removed, writers restarted"
  printf '%s\n' "$out" | grep -E 'pg-backup|pg-restore|drill' >&2 || true
  stamp=$(result stamp "$out")
  counts=$(result counts "$out")
  caddy=$(result caddy "$out")
  if [ -z "$stamp" ] || [ -z "$counts" ]; then die "the host did not report the backup: nothing was removed"; fi

  log "2/5 Verify in S3 and keep under $FINAL_PREFIX"
  for name in "rolerank-$stamp-app.dump.age" "rolerank-$stamp-corpus.dump.age" ${caddy:+"$caddy"}; do
    want=$(aws_ s3api head-object --bucket "$BUCKET" --key "$NIGHTLY_PREFIX$name" --query Metadata.sha256 --output text)
    [ "${#want}" = 64 ] || die "$name has no recorded sha256: nothing was removed"
    aws_ s3 cp "s3://$BUCKET/$NIGHTLY_PREFIX$name" "s3://$BUCKET/$FINAL_PREFIX$name" --only-show-errors
    got=$(aws_ s3api head-object --bucket "$BUCKET" --key "$FINAL_PREFIX$name" --query Metadata.sha256 --output text)
    [ "$got" = "$want" ] || die "the copy of $name lost its checksum: nothing was removed"
    echo "kept $FINAL_PREFIX$name (sha256 ${want:0:12}...)" >&2
  done
  printf '%s\n' "$counts" >"$WORK/counts"
  aws_ s3 cp "$(winpath "$WORK/counts")" "s3://$BUCKET/${FINAL_PREFIX}rolerank-$stamp.counts" --only-show-errors
  result release "$out" | base64 -d >"$WORK/release.json"
  jq -e '.release and .images.api and .images.web and .images.worker and .images.sandbox' "$WORK/release.json" >/dev/null ||
    die "the host's release manifest is incomplete: nothing was removed"
  aws_ s3 cp "$(winpath "$WORK/release.json")" "s3://$BUCKET/$RELEASE_KEY" --only-show-errors
  echo "saved release $(jq -r .release "$WORK/release.json" | cut -c1-12); counts at backup: $counts" >&2
  # The newest few sets stay; older ones get delete markers (Object Lock keeps their versions
  # for the retention period, then the lifecycle rule removes them).
  aws_ s3api list-objects-v2 --bucket "$BUCKET" --prefix "$FINAL_PREFIX" --query 'Contents[].Key' --output text | tr '\t' '\n' | tr -d '\r' |
    sed -n 's/^.*rolerank-\([0-9TZ]*\)-app\.dump\.age$/\1/p' | sort | head -n "-$KEEP_FINAL" | while read -r old; do
    for k in "rolerank-$old-app.dump.age" "rolerank-$old-corpus.dump.age" "rolerank-$old.counts" "caddy-$old.tar.gz"; do
      aws s3 rm "s3://$BUCKET/$FINAL_PREFIX$k" --only-show-errors 2>/dev/null || true
    done
    echo "pruned backup set $old" >&2
  done
else
  log "1-2/5 No host running: nothing to back up (newest kept set: $(newest_stamp "$FINAL_PREFIX"))"
fi

log "3/5 Alarm actions off (the budget stays), DNS parked at $PARKED_IP"
alarm=$(backup_alarm)
if [ -n "$alarm" ] && [ "$alarm" != None ]; then
  aws_ cloudwatch disable-alarm-actions --alarm-names "$alarm"
  echo "actions disabled: $alarm" >&2
fi
cf_set_a "$PARKED_IP"

log "4/5 Remove the host (EC2, EBS, Elastic IP)"
stack_exists "$HOST_STACK" && cdk_ destroy "$HOST_STACK" --force
cdk_ deploy "$PERSISTENT" --exclusively

log "5/5 Audit"
audit || exit 1
printf '\n\033[1mRoleRank is DOWN.\033[0m Newest backup set: %s. Bring it back: ./rolerank-up.sh\n' "$(newest_stamp "$FINAL_PREFIX")" >&2
echo "took $(($(date +%s) - T0)) s" >&2
