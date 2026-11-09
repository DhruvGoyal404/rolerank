#!/usr/bin/env bash
# Creates RoleRank's production parameters in SSM, once (MAINTENANCE.md, "Secrets"). Safe to run
# again: a parameter that exists is never overwritten (pass --rotate NAME to replace one).
#   - generated here, random, never shown: database and Valkey passwords, internal tokens, the
#     resume key-encryption key, and the backup key pair (age; the public half is not secret);
#   - asked for, without echo: the model API key, the production OAuth clients (callbacks
#     https://<domain>/api/auth/{google,github}/callback), the Cloudflare DNS token;
#   - plain config: the domain, the ACME and alarm addresses.
# Values travel in a 0600 file passed by file://, never in argv, logs or output.
#
#   bash scripts/prod-secrets.sh [--rotate NAME]
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/rolerank-lib.sh"
rotate=""
[ "${1:-}" = --rotate ] && rotate=${2:?--rotate needs a parameter name}

for c in aws jq openssl age-keygen; do command -v "$c" >/dev/null || die "$c is not installed (age: winget install FiloSottile.age)"; done
aws sts get-caller-identity >/dev/null 2>&1 || aws sso login --profile "$AWS_PROFILE" || die "aws sso login failed"

exists() { aws ssm get-parameter --name "$1" --query Parameter.Name --output text >/dev/null 2>&1; }
# put NAME TYPE: reads the value from stdin; skips a parameter that exists unless rotating it.
put() {
  local name=$1 type=$2 f="$WORK/param.json"
  if exists "$name" && [ "$rotate" != "${name##*/}" ]; then
    cat >/dev/null
    echo "kept     $name" >&2
    return
  fi
  (umask 077 && jq -Rs --arg n "$name" --arg t "$type" '{Name: $n, Type: $t, Value: (. | rtrimstr("\n")), Overwrite: true, Tier: "Standard"}' >"$f")
  aws_ ssm put-parameter --cli-input-json "file://$(winpath "$f")" >/dev/null
  rm -f "$f"
  echo "stored   $name ($type)" >&2
}
ask() { # ask NAME PROMPT [secret] [default]
  local v
  if exists "/rolerank/$1" && [ "$rotate" != "${1##*/}" ]; then echo "kept     /rolerank/$1" >&2; return; fi
  if [ -n "${3:-}" ]; then read -rsp "$2: " v; echo >&2; else read -rp "$2: " v; fi
  v=${v:-${4:-}}
  [ -n "$v" ] || die "$2 is required"
  local type=String
  [ -z "${3:-}" ] || type=SecureString
  printf '%s' "$v" | put "/rolerank/$1" "$type"
}

P=/rolerank/prod
for n in ROLERANK_PG_SUPERUSER_PASSWORD ROLERANK_VALKEY_PASSWORD ROLERANK_PW_MIGRATOR_APP ROLERANK_PW_MIGRATOR_CORPUS ROLERANK_PW_APP_RW \
  ROLERANK_PW_CORPUS_RW ROLERANK_PW_CORPUS_RO ROLERANK_EXTRACTOR_TOKEN ROLERANK_RANKER_TOKEN AIWORKER_TOKEN; do
  openssl rand -hex 24 | put "$P/$n" SecureString
done
openssl rand -base64 32 | put "$P/ROLERANK_KEK" SecureString
if ! exists "$P/age_identity" || [ "$rotate" = age_identity ]; then
  key=$(age-keygen 2>/dev/null)
  printf '%s\n' "$key" | grep -v '^#' | put "$P/age_identity" SecureString
  printf '%s\n' "$key" | sed -n 's/^# public key: //p' | put "$P/age_recipient" String
  unset key
else
  echo "kept     $P/age_identity (and age_recipient)" >&2
fi
ask prod/ROLERANK_DOMAIN "Domain [rolerank.dhruvgoyal.tech]" "" rolerank.dhruvgoyal.tech
ask prod/ACME_EMAIL "Let's Encrypt account email [bot@dhruvgoyal.tech]" "" bot@dhruvgoyal.tech
ask ops/alarm_email "Alarm and budget email"
ask prod/ANTHROPIC_API_KEY "Model API key (zero-retention org)" secret
ask prod/GOOGLE_CLIENT_ID "Google OAuth client ID (production client)"
ask prod/GOOGLE_CLIENT_SECRET "Google OAuth client secret" secret
ask prod/GITHUB_CLIENT_ID "GitHub OAuth app client ID (production app)"
ask prod/GITHUB_CLIENT_SECRET "GitHub OAuth app client secret" secret
ask ops/cloudflare-token "Cloudflare API token (Zone:Read, DNS:Edit on dhruvgoyal.tech)" secret
echo "done: every production parameter exists under /rolerank (values not shown)" >&2
