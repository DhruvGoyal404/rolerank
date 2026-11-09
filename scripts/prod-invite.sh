#!/usr/bin/env bash
# shellcheck disable=SC2016 # the host script is single-quoted on purpose; it expands on the host
# Creates one production invite code while the site is up. The code is made on this PC and
# only its SHA-256 goes to the host (through SSM), so it never appears in SSM command history,
# logs or chat. The code is appended to .dev/prod-invites.txt (gitignored) and not printed.
#
#   bash scripts/prod-invite.sh [uses=1] [days=30]
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/rolerank-lib.sh"
uses=${1:-1}
days=${2:-30}
[[ "$uses" =~ ^[1-9][0-9]{0,2}$ && "$days" =~ ^[1-9][0-9]{0,2}$ ]] || die "uses and days are whole numbers from 1 to 999"
preflight
ID=$(host_id)
[ -n "$ID" ] || die "the site is down: run ./rolerank-up.sh first"

# Same shape as api/src/crypto.ts randomToken(16): 16 random bytes, base64url without padding.
code=$(openssl rand -base64 16 | tr '+/' '-_' | tr -d '=\r\n') # Windows openssl ends with CRLF
hash=$(printf '%s' "$code" | sha256sum | cut -d' ' -f1)
[[ "$hash" =~ ^[0-9a-f]{64}$ ]] || die "could not hash the code"

out=$(ssm_run "$ID" '
set -euo pipefail
pg=$('"$HOST_COMPOSE"' ps -q postgres)
docker exec "$pg" psql -U postgres -d app -XAt -v ON_ERROR_STOP=1 -c "INSERT INTO invites (code_sha256, uses_left, expires_at) VALUES (decode('"'$hash'"', '"'hex'"'), '"$uses"', now() + make_interval(days => '"$days"')) RETURNING '"'RESULT ok=1'"'"
' 120) || die "the host did not store the invite"
[ "$(result ok "$out")" = 1 ] || die "the host did not confirm the invite"

mkdir -p "$ROOT/.dev"
(umask 077 && printf '%s  uses=%s  expires=%s  %s\n' "$(date -u +%F)" "$uses" "$(date -u -d "+$days days" +%F)" "$code" >>"$ROOT/.dev/prod-invites.txt")
unset code
echo "invite stored (only its hash is on the host); the code is the last line of .dev/prod-invites.txt" >&2
