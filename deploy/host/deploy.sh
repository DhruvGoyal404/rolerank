#!/usr/bin/env bash
# Deploys one RoleRank release on the host. Run as root by the RoleRank-Deploy SSM document
# (infra/lib/stacks.ts) with the release manifest, base64:
#   {"release": "<commit>", "images": {"api": "...@sha256:...", "web": ..., "worker": ..., "sandbox": ...}}
# Pulls every image by digest, takes deploy/ and db/ and the extractor binary from the worker
# image, renders the secrets from SSM (never printed), migrates (forward-only, so the previous
# release still works), starts everything, smoke tests, and rolls back to the previous release
# on any failure. Safe to run again with the same manifest.
set -euo pipefail
export AWS_REGION=${AWS_REGION:-ap-south-1}
export AWS_DEFAULT_REGION=$AWS_REGION
PARAMS=/rolerank/prod/
ROOT=/opt/rolerank
log() { printf 'deploy: %s\n' "$*"; }
die() { printf 'deploy: FAILED: %s\n' "$*" >&2; exit 1; }

manifest=$(printf '%s' "${1:?usage: deploy.sh <base64 manifest>}" | base64 -d) || die "manifest is not base64"
ref='^[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/rolerank/[a-z-]+@sha256:[0-9a-f]{64}$'
jq -e --arg re "$ref" '(.release | test("^[0-9a-f]{40}$")) and ([.images.api, .images.web, .images.worker, .images.sandbox] | all(type == "string" and test($re)))' \
  <<<"$manifest" >/dev/null || die "manifest is malformed"
release=$(jq -r .release <<<"$manifest")
img() { jq -r ".images.$1" <<<"$manifest"; }
dir=$ROOT/releases/$release

log "release ${release:0:12}: pulling images by digest"
registry=$(img api | cut -d/ -f1)
aws ecr get-login-password | docker login --username AWS --password-stdin "$registry" >/dev/null
for k in api web worker sandbox; do docker pull -q "$(img "$k")" >/dev/null; done

if [ ! -f "$dir/.complete" ]; then
  rm -rf "$dir" && install -d -m 0755 "$dir"
  cid=$(docker create "$(img worker)")
  docker cp "$cid:/app" "$dir/app" && docker cp "$cid:/usr/local/bin/extractor" "$dir/extractor"
  docker rm "$cid" >/dev/null
  printf 'API_IMAGE=%s\nWEB_IMAGE=%s\nWORKER_IMAGE=%s\nSANDBOX_IMAGE=%s\n' "$(img api)" "$(img web)" "$(img worker)" "$(img sandbox)" >"$dir/release.env"
  printf '%s\n' "$manifest" >"$dir/manifest.json"
  touch "$dir/.complete"
fi

log "rendering secrets from SSM $PARAMS (values never printed)"
umask 077
# Parameters named like environment variables become prod.env lines; lower-case ones are config.
aws ssm get-parameters-by-path --path "$PARAMS" --recursive --with-decryption --output json |
  jq -r '.Parameters[] | (.Name | split("/") | last) as $n | select($n | test("^[A-Z][A-Z0-9_]*$")) | "\($n)=\(.Value)"' >/etc/rolerank/prod.env.new
param() { aws ssm get-parameter --name "$PARAMS$1" --with-decryption --query Parameter.Value --output text; }
for v in ROLERANK_DOMAIN ACME_EMAIL ROLERANK_KEK ROLERANK_EXTRACTOR_TOKEN ROLERANK_RANKER_TOKEN AIWORKER_TOKEN ANTHROPIC_API_KEY \
  GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET GITHUB_CLIENT_ID GITHUB_CLIENT_SECRET ROLERANK_PG_SUPERUSER_PASSWORD ROLERANK_VALKEY_PASSWORD \
  ROLERANK_PW_MIGRATOR_APP ROLERANK_PW_MIGRATOR_CORPUS ROLERANK_PW_APP_RW ROLERANK_PW_CORPUS_RW ROLERANK_PW_CORPUS_RO; do
  grep -q "^$v=." /etc/rolerank/prod.env.new || die "SSM $PARAMS$v is missing (scripts/prod-secrets.sh)"
done
mv -f /etc/rolerank/prod.env.new /etc/rolerank/prod.env
token=$(sed -n 's/^ROLERANK_EXTRACTOR_TOKEN=//p' /etc/rolerank/prod.env)
printf 'ROLERANK_EXTRACTOR_TOKEN=%s\nROLERANK_EXTRACTOR_IMAGE=%s\nROLERANK_SANDBOX_RUNTIME=runsc\nROLERANK_EXTRACTOR_ADDR=172.17.0.1:8090\n' \
  "$token" "$(img sandbox)" >/etc/rolerank/extractor.env
printf 'BUCKET=%s\nAGE_RECIPIENT=%s\n' "$(param backup_bucket)" "$(param age_recipient)" >/etc/rolerank/backup.env
umask 022

previous=$(readlink -f "$ROOT/current" 2>/dev/null || true)
switch() { ln -sfn "$1" "$ROOT/current.new" && mv -Tf "$ROOT/current.new" "$ROOT/current"; }
compose() { docker compose -f "$ROOT/current/app/deploy/compose.prod.yaml" --env-file /etc/rolerank/prod.env --env-file "$ROOT/current/release.env" "$@"; }

start() {
  install -m 0644 "$ROOT"/current/app/deploy/systemd/*.service "$ROOT"/current/app/deploy/systemd/*.timer /etc/systemd/system/
  systemctl daemon-reload
  compose up -d --wait --wait-timeout 300 postgres valkey
  compose run --rm migrate-app up >/dev/null
  compose run --rm migrate-corpus up >/dev/null
  compose up -d --wait --wait-timeout 600 --remove-orphans caddy api aiworker ranker ingest
  systemctl restart rolerank-extractor.service
  # Started with enable --now and checked active, never just enabled (CLAUDE.md, Backups).
  systemctl enable --now rolerank-extractor.service rolerank-backup.timer rolerank-embed.timer >/dev/null 2>&1
  for u in rolerank-extractor.service rolerank-backup.timer rolerank-embed.timer; do
    systemctl is-active --quiet "$u" || { log "$u is not active"; return 1; }
  done
}
# fetch URL inside the API container (the internal network has no published ports).
fetch_ok() { compose exec -T api node -e "fetch(process.argv[1]).then(r=>process.exit(r.ok?0:1),()=>process.exit(1))" "$1"; }
# answers URL: any HTTP status at all (aiworker has no health route; unauthenticated calls get 401).
answers() { compose exec -T api node -e "fetch(process.argv[1]).then(()=>process.exit(0),()=>process.exit(1))" "$1"; }
smoke() {
  for _ in $(seq 30); do curl -fsS -o /dev/null http://172.17.0.1:8090/health && break; sleep 2; done
  curl -fsS -o /dev/null http://172.17.0.1:8090/health || { log "smoke: extractor"; return 1; }
  fetch_ok http://127.0.0.1:8080/api/health || { log "smoke: api"; return 1; }
  for _ in $(seq 60); do fetch_ok http://ranker:8094/health && break; sleep 2; done
  fetch_ok http://ranker:8094/health || { log "smoke: ranker"; return 1; }
  answers http://aiworker:8093/ || { log "smoke: aiworker"; return 1; }
  log "smoke: extractor, api, ranker and aiworker answer"
}

switch "$dir"
if start && smoke; then
  cp "$dir/manifest.json" "$ROOT/state/current.json"
  log "release ${release:0:12} is live"
  # Old releases: keep the live one and the previous one.
  find "$ROOT/releases" -mindepth 1 -maxdepth 1 -type d ! -path "$dir" ! -path "${previous:-/nonexistent}" -exec rm -rf {} +
  exit 0
fi
if [ -n "$previous" ] && [ "$previous" != "$dir" ]; then
  log "rolling back to $(basename "$previous" | cut -c1-12)"
  switch "$previous"
  if ! { start && smoke; }; then log "rollback did not pass its smoke test either"; fi
fi
die "release ${release:0:12} failed its smoke test"
