#!/usr/bin/env bash
# shellcheck disable=SC2016 # host scripts are single-quoted on purpose; local values are spliced in
# Brings RoleRank up from fully down (docs/ARCHITECTURE.md, "Deployment design"):
#   1. both stacks deployed (the host stack: EC2 m7g.xlarge, EBS, Elastic IP);
#   2. the host bootstraps (Docker, gVisor, AWS CLI: deploy/host/bootstrap.sh);
#   3. Caddy's certificates restored before Caddy first starts (no new Let's Encrypt certificate);
#   4. the release saved at the last down redeployed from ECR by digest; with --build, or with
#      nothing saved, main is built in the cloud by the GitHub Actions "Deploy" workflow;
#   5. the newest backup set restored, tombstones re-applied, counts checked against those
#      recorded at down; only then DNS points at the host; a fresh backup; the alarm back on;
#   6. the ranker warmed with a real search, so the first user's matches aren't a cold start;
#   7. checks: health by IP and by domain, Google and GitHub authorize pages, one real LLM call
#      through the spend ledger (api/src/cli/llm-check.ts),
#      the warm search's latency;
#   8. the box: URL, fallback, cost per hour, "run rolerank-down.sh when done".
# Safe to run again after a half-failed run.
#
#   ./rolerank-up.sh [--build] [--first]
#   --build   rebuild main instead of redeploying the saved release
#   --first   allow an empty start when no backup exists yet (the very first up)
set -euo pipefail
BUILD=0 FIRST=0
for a in "$@"; do
  case "$a" in
    --build) BUILD=1 ;;
    --first) FIRST=1 ;;
    *) echo "usage: $0 [--build] [--first]" >&2; exit 2 ;;
  esac
done
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/scripts/rolerank-lib.sh"

preflight
cf_check
ami=$(jq -r '.context["rolerank:hostAmi"] // ""' "$ROOT/infra/cdk.json")
[[ "$ami" =~ ^ami-[0-9a-f]{8,17}$ ]] ||
  die "pin the host AMI first: infra/cdk.json \"rolerank:hostAmi\" (Ubuntu 24.04 arm64; MAINTENANCE.md, \"The host's AMI\")"
main_sha=$(git -C "$ROOT" ls-remote origin refs/heads/main | cut -f1)
[ "$(git -C "$ROOT" rev-parse HEAD)" = "$main_sha" ] ||
  echo "note: local HEAD differs from origin/main; a build deploys origin/main" >&2

log "1/8 Stacks (persistent, then the host)"
cdk_ deploy "$PERSISTENT" "$HOST_STACK"
BUCKET=$(bucket)
ID=$(stack_output "$HOST_STACK" InstanceId)
IP=$(stack_output "$HOST_STACK" PublicIp)
echo "host ${ID:0:5}..., IP $IP" >&2
STAMP=$(newest_stamp "$FINAL_PREFIX")
SEED=""
if [ -z "$STAMP" ]; then
  [ "$FIRST" = 1 ] || die "no backup in s3://$BUCKET/$FINAL_PREFIX: refusing an empty site (pass --first for the very first up)"
  # The first up: no users yet, and the corpus comes from the seed scripts/seed-corpus.sh uploaded.
  SEED=$(aws_ s3api list-objects-v2 --bucket "$BUCKET" --prefix "$SEED_PREFIX" --query 'Contents[].Key' --output text | tr '\t' '\n' | tr -d '\r' |
    sed -n 's/^.*rolerank-\([0-9TZ]*\)-corpus\.dump\.age$/\1/p' | sort | tail -n 1)
  [ -n "$SEED" ] || die "no backup and no corpus seed: run bash scripts/seed-corpus.sh first"
  echo "no backup yet (--first): empty app database, corpus from the seed of $SEED" >&2
fi
CADDY=""
[ -n "$STAMP" ] && aws s3api head-object --bucket "$BUCKET" --key "${FINAL_PREFIX}caddy-$STAMP.tar.gz" >/dev/null 2>&1 && CADDY="caddy-$STAMP.tar.gz"
# Without saved certificates Let's Encrypt must reach the host by name, so DNS moves first.
[ -n "$CADDY" ] || cf_set_a "$IP"

log "2/8 Wait for the host to finish bootstrapping"
for _ in $(seq 120); do
  [ "$(aws ssm describe-instance-information --filters "Key=InstanceIds,Values=$ID" --query 'InstanceInformationList[0].PingStatus' --output text 2>/dev/null)" = Online ] && break
  sleep 5
done
ssm_run "$ID" 'cloud-init status --wait >/dev/null || true; cloud-init status --long | grep -E "^(status|errors)"; cloud-init status | grep -q "status: done"' 1800 ||
  die "the host bootstrap did not finish (see /var/log/cloud-init-output.log through an SSM session)"

# The host may read only the nightly prefix: the set to restore is copied there.
for name in ${STAMP:+"rolerank-$STAMP-app.dump.age" "rolerank-$STAMP-corpus.dump.age"} ${CADDY:+"$CADDY"}; do
  aws_ s3 cp "s3://$BUCKET/$FINAL_PREFIX$name" "s3://$BUCKET/$NIGHTLY_PREFIX$name" --only-show-errors
done
[ -z "$SEED" ] || aws_ s3 cp "s3://$BUCKET/${SEED_PREFIX}rolerank-$SEED-corpus.dump.age" "s3://$BUCKET/${NIGHTLY_PREFIX}rolerank-$SEED-corpus.dump.age" --only-show-errors

log "3/8 Restore Caddy's certificates (before Caddy starts)"
if [ -n "$CADDY" ]; then
  sha=$(aws_ s3api head-object --bucket "$BUCKET" --key "$NIGHTLY_PREFIX$CADDY" --query Metadata.sha256 --output text)
  ssm_run "$ID" '
set -euo pipefail
vol='"$COMPOSE_PROJECT"'_caddy_data
if docker volume inspect "$vol" >/dev/null 2>&1 && [ -n "$(ls -A "$(docker volume inspect -f "{{.Mountpoint}}" "$vol")")" ]; then
  echo "caddy_data already restored"; exit 0
fi
docker volume inspect "$vol" >/dev/null 2>&1 ||
  docker volume create --label com.docker.compose.project='"$COMPOSE_PROJECT"' --label com.docker.compose.volume=caddy_data --label rolerank.managed=true "$vol" >/dev/null
tgz=$(mktemp /var/lib/rolerank/backup/caddy.XXXXXX.tgz)
aws s3 cp "s3://'"$BUCKET/$NIGHTLY_PREFIX$CADDY"'" "$tgz" --only-show-errors
[ "$(sha256sum "$tgz" | cut -d" " -f1)" = '"$sha"' ] || { echo "Caddy backup checksum mismatch"; exit 1; }
tar -C "$(docker volume inspect -f "{{.Mountpoint}}" "$vol")" -xzf "$tgz"
rm -f "$tgz"
echo "caddy_data restored"
' || die "Caddy data restore failed"
else
  echo "no saved certificates: Caddy requests a new one" >&2
fi

log "4/8 Release"
current=$(ssm_run "$ID" 'jq -r .release /opt/rolerank/state/current.json 2>/dev/null || true' 2>/dev/null || true)
saved=""
if [ "$BUILD" = 0 ] && aws s3 cp "s3://$BUCKET/$RELEASE_KEY" "$(winpath "$WORK/release.json")" --only-show-errors 2>/dev/null; then
  saved=$(jq -r .release "$WORK/release.json")
  for k in api web worker sandbox; do
    ref=$(jq -r ".images.$k" "$WORK/release.json")
    repo=${ref#*/}
    aws ecr describe-images --repository-name "${repo%@*}" --image-ids "imageDigest=${ref#*@}" >/dev/null 2>&1 ||
      { echo "image ${repo%@*} of the saved release is gone from ECR: building instead" >&2; saved=""; break; }
  done
fi
if [ -n "$saved" ]; then
  ahead=$(git -C "$ROOT" rev-list --count "$saved..origin/main" 2>/dev/null || echo "?")
  [ "$ahead" = 0 ] || printf '\033[33mmain is %s commits ahead of the saved release; run with --build to rebuild\033[0m\n' "$ahead" >&2
  if [ "$current" = "$saved" ]; then
    echo "release ${saved:0:12} already live" >&2
  else
    echo "deploying the saved release ${saved:0:12} by digest (no build)" >&2
    jq -n --arg m "$(base64 -w0 "$WORK/release.json")" '{Manifest: [$m]}' >"$WORK/deploy-params.json"
    ssm_send "$ID" RoleRank-Deploy "$WORK/deploy-params.json" | grep -E "^deploy: " >&2 ||
      die "deploy of the saved release failed (the host rolled back by itself; see above)"
  fi
elif [ "$current" = "$main_sha" ]; then
  echo "release ${main_sha:0:12} already live" >&2
else
  echo "building main ${main_sha:0:12} in the cloud (GitHub Actions: Deploy)" >&2
  since=$(date -u +%FT%TZ)
  gh workflow run Deploy --ref main -R "$REPO" >/dev/null
  run=""
  for _ in $(seq 60); do
    run=$(gh run list -R "$REPO" -w Deploy -e workflow_dispatch -L 5 --json databaseId,createdAt --jq "[.[] | select(.createdAt >= \"$since\")][0].databaseId // empty")
    [ -n "$run" ] && break
    sleep 5
  done
  [ -n "$run" ] || die "the Deploy run did not start"
  echo "Deploy run https://github.com/$REPO/actions/runs/$run" >&2
  gh run watch "$run" -R "$REPO" --exit-status --interval 20 >/dev/null || die "Deploy failed: https://github.com/$REPO/actions/runs/$run"
fi

log "5/8 Restore the databases, then DNS; fresh backup; alarm actions on"
if [ -n "$STAMP" ]; then
  want=$(aws s3 cp "s3://$BUCKET/${FINAL_PREFIX}rolerank-$STAMP.counts" - 2>/dev/null | tr -d '\r' || true)
  out=$(ssm_run "$ID" '
set -euo pipefail
set -a; . /etc/rolerank/backup.env; set +a
compose() { '"$HOST_COMPOSE"' "$@"; }
marker=/opt/rolerank/state/restored-from
if [ -s "$marker" ]; then # restored already: this host'"'"'s data is live now, never restore over it
  echo "already restored from $(cat "$marker")"
else
  compose stop api aiworker ingest ranker >/dev/null
  systemctl stop rolerank-extractor.service
  /opt/rolerank/current/app/deploy/backup/pg-restore.sh '"$STAMP"' --into-live | tee /opt/rolerank/state/restore.log
  compose run --rm migrate-app up >/dev/null
  compose run --rm migrate-corpus up >/dev/null
  compose up -d --wait --wait-timeout 600 api aiworker ingest ranker >/dev/null
  systemctl start rolerank-extractor.service
  echo '"$STAMP"' >"$marker"
fi
echo "RESULT counts=$(sed -n "s/^restored-counts: //p" /opt/rolerank/state/restore.log | tail -n 1)"
') || die "database restore failed (DNS not moved)"
  got=$(result counts "$out")
  echo "restored: $got" >&2
  # Equal unless a tombstone removed a user the backup still held: then fewer, never more.
  [ -z "$want" ] || [ "$got" = "$want" ] || {
    printf '\033[33mcounts differ from those recorded at down:\n  down:     %s\n  restored: %s\033[0m\n' "$want" "$got" >&2
    [ "$(grep -o 'users=[0-9]*' <<<"$got" | cut -d= -f2)" -le "$(grep -o 'users=[0-9]*' <<<"$want" | cut -d= -f2)" ] ||
      die "more users after restore than at down: stop and look (DNS not moved)"
  }
fi
if [ -n "$SEED" ]; then
  out=$(ssm_run "$ID" '
set -euo pipefail
set -a; . /etc/rolerank/backup.env; set +a
compose() { '"$HOST_COMPOSE"' "$@"; }
marker=/opt/rolerank/state/seeded-from
if [ -s "$marker" ]; then
  echo "already seeded from $(cat "$marker")"
else
  compose stop ranker ingest >/dev/null
  /opt/rolerank/current/app/deploy/backup/pg-restore.sh '"$SEED"' --corpus-only | tee /opt/rolerank/state/seed.log
  compose run --rm migrate-corpus up >/dev/null
  compose up -d --wait --wait-timeout 600 ranker ingest >/dev/null
  echo '"$SEED"' >"$marker"
fi
sed -n "s/^restored-corpus-\(postings\|seconds\): /RESULT \1=/p" /opt/rolerank/state/seed.log
') || die "corpus seed restore failed (DNS not moved)"
  echo "corpus seeded: $(result postings "$out") postings, restore and reindex took $(result seconds "$out") s" >&2
fi
cf_set_a "$IP"
ssm_run "$ID" 'start=$(date +%s); systemctl start rolerank-backup.service; journalctl -u rolerank-backup.service --since "@$start" -o cat --no-pager | grep -E "uploaded|metric"' >&2 ||
  die "the fresh backup failed"
alarm=$(backup_alarm)
aws_ cloudwatch enable-alarm-actions --alarm-names "$alarm"
echo "actions enabled: $alarm" >&2

log "6/8 Warm the ranker (the embedding model and the corpus pages)"
# A synthetic profile, never a user's. Two searches: the first pays the cold start, the second
# is what users see.
warm=$(ssm_run "$ID" '
set -euo pipefail
'"$HOST_COMPOSE"' exec -T api node --input-type=module -e "
const body = JSON.stringify({ skills: [\"Python\", \"SQL\", \"Docker\"], titles: \"Backend Engineer\", level: \"entry\", country: \"IN\", remote: true, text: \"Backend engineer: Python, SQL, REST APIs and Docker.\", limit: 50 });
const go = async () => { const t = performance.now(); const r = await fetch(\"http://ranker:8094/search\", { method: \"POST\", headers: { authorization: \"Bearer \" + process.env.RANKER_TOKEN, \"content-type\": \"application/json\" }, body }); const n = (await r.json()).hits.length; return [r.status, Math.round(performance.now() - t), n]; };
const [s1, cold] = await go(); const [s2, ms, n] = await go();
console.log(\"RESULT warm=\" + [s1, s2, cold, ms, n].join(\" \"));
"') || die "the ranker warm-up failed"
read -r s1 s2 cold ms hits <<<"$(result warm "$warm")"
[ "$s1" = 200 ] && [ "$s2" = 200 ] || die "the ranker answered $s1/$s2 to the warm-up"
echo "ranker: cold search ${cold} ms, warm ${ms} ms, $hits postings" >&2

log "7/8 Checks"
fail=0
check() { if "$@"; then printf '  ok    %s\n' "$CHECK" >&2; else printf '  \033[31mFAIL\033[0m  %s\n' "$CHECK" >&2; fail=1; fi; }
CHECK="health on the IP $IP with the domain's certificate"
check retry curl -fsS --max-time 15 --resolve "$DOMAIN:443:$IP" "https://$DOMAIN/api/health" -o "$(winpath "$WORK/body")"
CHECK="health on https://$DOMAIN (DNS -> $IP, valid certificate)"
domain_ok() {
  for _ in $(seq 60); do
    [ "$(curl -fsS --max-time 10 -o "$(winpath "$WORK/body")" -w '%{remote_ip}' "https://$DOMAIN/api/health" 2>/dev/null)" = "$IP" ] && return 0
    sleep 10
  done
  return 1
}
check domain_ok
CHECK="Google OAuth: authorize redirect carries our client, PKCE and nonce"
google_ok() {
  local loc
  loc=$(curl -fsS --max-time 15 -o "$(winpath "$WORK/body")" -w '%{redirect_url}' --resolve "$DOMAIN:443:$IP" "https://$DOMAIN/api/auth/google/start")
  [[ "$loc" == https://accounts.google.com/* && "$loc" == *client_id=* && "$loc" == *code_challenge_method=S256* && "$loc" == *nonce=* ]] || return 1
  curl -fsS --max-time 15 -o /dev/null "$loc" # Google answers 200 for a registered client, 400 for an unknown one
}
check google_ok
CHECK="GitHub OAuth: the authorize page names our app"
github_ok() {
  local loc page
  loc=$(curl -fsS --max-time 15 -o "$(winpath "$WORK/body")" -w '%{redirect_url}' --resolve "$DOMAIN:443:$IP" "https://$DOMAIN/api/auth/github/start")
  [[ "$loc" == https://github.com/login/oauth/authorize?* ]] || return 1
  page=$(curl -fsS --max-time 15 -L "$loc") || return 1
  grep -qi 'continue to' <<<"$page"
}
check github_ok
CHECK="one real LLM call through the spend ledger (ops scope, its own daily cap; no user data)"
check ssm_run "$ID" ''"$HOST_COMPOSE"' exec -T aiworker node dist/cli/llm-check.js'
CHECK="warm search under 200 ms on the host (${ms} ms)"
check test "$ms" -lt 200
CHECK="backup and embedding timers active, extractor running"
check ssm_run "$ID" 'systemctl is-active rolerank-backup.timer rolerank-embed.timer rolerank-extractor.service'
[ "$fail" = 0 ] || die "some checks failed (above); the site is up: fix it, or run ./rolerank-down.sh"

log "8/8 Up"
secs=$(($(date +%s) - T0))
printf '\n\033[1m+------------------------------------------------------------------+\n' >&2
printf '| RoleRank is UP                                                   |\n' >&2
printf '|   URL:       https://%-44s|\n' "$DOMAIN" >&2
printf '|   fallback:  add to your hosts file: %-28s|\n' "$IP $DOMAIN" >&2
printf '|   cost:      ~$%s/hour (~Rs %s/hour) while up                 |\n' "$PRICE_HOUR_USD" "$(awk "BEGIN{printf \"%.1f\", $PRICE_HOUR_USD*$USD_INR}")" >&2
printf '|   took:      %-52s|\n' "$((secs / 60)) min $((secs % 60)) s" >&2
printf '|   when done: run ./rolerank-down.sh                              |\n' >&2
printf '+------------------------------------------------------------------+\033[0m\n' >&2
