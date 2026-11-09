#!/usr/bin/env bash
# Starts all of RoleRank locally against the real local corpus, in one command:
#   Postgres and Valkey (compose), migrations, the extractor sandbox service, aiworker, the ranker
#   service (compose), the API and the built site. Prints the URL.
#   Stop it all with: bash scripts/dev-down.sh
# Reads the gitignored .env and prints no secret. Logs go to .dev/logs (gitignored).
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(git rev-parse --show-toplevel)"
export PATH="/c/nvm4w/nodejs:$PATH"
DEV="$(pwd)/.dev"
mkdir -p "$DEV/logs"

say() { printf '\n== %s\n' "$*"; }
port_busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
wait_http() { # url, seconds
  for _ in $(seq "$2"); do curl -fs "$1" >/dev/null 2>&1 && return 0; sleep 1; done
  return 1
}
# Starts a background process and records its Windows PID, so dev-down can stop it and its children.
start() { # name, command...
  local name=$1
  shift
  nohup "$@" >"$DEV/logs/$name.log" 2>&1 &
  local pid=$!
  local win
  win=$(cat "/proc/$pid/winpid" 2>/dev/null || echo "$pid")
  echo "$name $win" >>"$DEV/pids"
}

if [ -s "$DEV/pids" ]; then
  echo "RoleRank looks already started (.dev/pids exists). Run: bash scripts/dev-down.sh"
  exit 1
fi
for p in 5173 8080 8090 8093 8094; do
  if port_busy "$p"; then echo "Port $p is in use. Stop whatever holds it (or run scripts/dev-down.sh) and retry."; exit 1; fi
done

set -a
# shellcheck disable=SC1091
. ./.env
set +a
for v in ROLERANK_KEK ROLERANK_EXTRACTOR_TOKEN ANTHROPIC_API_KEY ROLERANK_RANKER_TOKEN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET GITHUB_CLIENT_ID GITHUB_CLIENT_SECRET; do
  [ -n "${!v:-}" ] || { echo "$v is missing from .env"; exit 1; }
done
# aiworker's internal token: generated once into .env, never printed.
if [ -z "${AIWORKER_TOKEN:-}" ]; then
  AIWORKER_TOKEN=$(openssl rand -hex 24)
  printf '\nAIWORKER_TOKEN=%s\n' "$AIWORKER_TOKEN" >>.env
fi

say "Docker"
if ! docker info >/dev/null 2>&1; then
  echo "Starting Docker Desktop..."
  "/c/Program Files/Docker/Docker/Docker Desktop.exe" >/dev/null 2>&1 &
  for _ in $(seq 120); do docker info >/dev/null 2>&1 && break; sleep 2; done
  docker info >/dev/null 2>&1 || { echo "Docker didn't start; open Docker Desktop and retry."; exit 1; }
fi

say "Postgres and Valkey"
docker compose up -d --wait postgres valkey
docker compose --profile tools run --rm migrate-app up >/dev/null
docker compose --profile tools run --rm migrate-corpus up >/dev/null
rows=$(docker compose exec -T postgres psql -U postgres -d corpus -Atc "SELECT count(*) FROM search_postings" 2>/dev/null || echo 0)
echo "Corpus: $rows searchable postings"

say "Builds (API, site, extractor)"
(cd api && npm run --silent build)
(cd web && npx vite build --logLevel error)
bash scripts/build-extractor.sh >/dev/null
(cd worker && go build -o .bin/rolerank-extractor.exe ./cmd/extractor)

say "Ranker service (first start compiles it in its container: a minute or two)"
bash scripts/models.sh >/dev/null
docker compose --profile ranker up -d ranker
echo "ranker compose" >>"$DEV/pids"

say "Extractor, aiworker, API, site"
APP_URL="postgres://app_rw:${ROLERANK_PW_APP_RW}@127.0.0.1:55433/app"
ROLERANK_EXTRACTOR_IMAGE=rolerank-extractor:dev ROLERANK_EXTRACTOR_ADDR=127.0.0.1:8090 \
  start extractor worker/.bin/rolerank-extractor.exe
APP_DATABASE_URL="$APP_URL" EXTRACTOR_URL=http://127.0.0.1:8090 EXTRACTOR_TOKEN="$ROLERANK_EXTRACTOR_TOKEN" \
  AIWORKER_TOKEN="$AIWORKER_TOKEN" AIWORKER_ADDR=127.0.0.1:8093 \
  start aiworker node api/dist/aiworker.js
NODE_ENV=development PUBLIC_ORIGIN=http://localhost:5173 APP_DATABASE_URL="$APP_URL" \
  VALKEY_URL="redis://:${ROLERANK_VALKEY_PASSWORD}@127.0.0.1:56380" \
  CORPUS_DATABASE_URL="postgres://corpus_ro:${ROLERANK_PW_CORPUS_RO}@127.0.0.1:55433/corpus" \
  AIWORKER_URL=http://127.0.0.1:8093 AIWORKER_TOKEN="$AIWORKER_TOKEN" RANKER_URL=http://127.0.0.1:8094 RANKER_TOKEN="$ROLERANK_RANKER_TOKEN" \
  start api node api/dist/index.js
# The built site under vite preview: the production CSP, with /api proxied to the API.
(cd web && start web npx vite preview --port 5173 --strictPort)

say "Waiting for health"
wait_http http://127.0.0.1:8090/health 60 || { echo "extractor didn't start: see $DEV/logs/extractor.log"; exit 1; }
wait_http http://127.0.0.1:8080/api/health 60 || { echo "API didn't start: see $DEV/logs/api.log"; exit 1; }
wait_http http://localhost:5173 60 || { echo "site didn't start: see $DEV/logs/web.log"; exit 1; }
wait_http http://127.0.0.1:8094/health 300 || { echo "ranker didn't start: docker compose logs ranker"; exit 1; }

cat <<EOF

+------------------------------------------------------------------+
|  RoleRank is running locally                                     |
|                                                                  |
|  Open:    http://localhost:5173                                  |
|  Corpus:  $rows searchable postings
|  Logs:    .dev/logs/                                             |
|                                                                  |
|  Stop everything:  bash scripts/dev-down.sh                      |
+------------------------------------------------------------------+
EOF
