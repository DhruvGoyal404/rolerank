#!/usr/bin/env bash
# Parses one of your own resumes through the real sandbox and Claude, and prints the draft
# profile. Keep the file in my-resumes/ (gitignored). Nothing about the resume is stored; the
# ledger keeps the call's cost in the local dev database. About $0.0003 a run.
#   bash scripts/parse-resume.sh my-resumes/resume.pdf
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(git rev-parse --show-toplevel)"
[ $# -eq 1 ] && [ -f "$1" ] || { echo "usage: scripts/parse-resume.sh <file.pdf|file.docx>"; exit 2; }
file=$(cygpath -w "$(realpath "$1")" 2>/dev/null || realpath "$1")
set -a
# shellcheck disable=SC1091
. ./.env
set +a
[ -n "${ANTHROPIC_API_KEY:-}" ] || { echo "ANTHROPIC_API_KEY is not set in .env"; exit 1; }
docker compose up -d --wait postgres >/dev/null
docker compose --profile tools run --rm migrate-app up >/dev/null
bash scripts/build-extractor.sh >/dev/null
# Built, not "go run", so the process we start is the server itself and the trap can stop it.
(cd worker && go build -o .bin/rolerank-extractor.exe ./cmd/extractor)
ROLERANK_EXTRACTOR_IMAGE=rolerank-extractor:dev ROLERANK_EXTRACTOR_ADDR=127.0.0.1:8092   worker/.bin/rolerank-extractor.exe 2>/dev/null &
extractor=$!
trap 'kill $extractor 2>/dev/null || true' EXIT
for _ in $(seq 90); do curl -fs http://127.0.0.1:8092/health >/dev/null 2>&1 && break; sleep 1; done
(cd api && npm run --silent build)
APP_DATABASE_URL="postgres://app_rw:${ROLERANK_PW_APP_RW}@127.0.0.1:55433/app" \
EXTRACTOR_URL=http://127.0.0.1:8092 EXTRACTOR_TOKEN="$ROLERANK_EXTRACTOR_TOKEN" \
  node api/dist/cli/parse-resume.js "$file"
