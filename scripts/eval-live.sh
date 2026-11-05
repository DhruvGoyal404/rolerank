#!/usr/bin/env bash
# Live D1 and D7 against the real model and the real extractor sandbox (EVALS.md, tier T1).
# Needs ANTHROPIC_API_KEY in .env. Spend is capped by the ledger: about $0.30 a run.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. ./.env
set +a
[ -n "${ANTHROPIC_API_KEY:-}" ] || { echo "ANTHROPIC_API_KEY is not set in .env"; exit 1; }
# shellcheck disable=SC1091
. scripts/test-env.sh >/dev/null
bash scripts/build-extractor.sh >/dev/null
export EXTRACTOR_URL=http://127.0.0.1:8091 EXTRACTOR_TOKEN="$ROLERANK_EXTRACTOR_TOKEN"
(cd worker && ROLERANK_EXTRACTOR_IMAGE=rolerank-extractor:dev ROLERANK_EXTRACTOR_ADDR=127.0.0.1:8091 go run ./cmd/extractor) &
extractor=$!
trap 'kill $extractor 2>/dev/null || true' EXIT
for _ in $(seq 60); do curl -fs http://127.0.0.1:8091/health >/dev/null 2>&1 && break; sleep 1; done
cd api && npx vitest run --config vitest.eval.config.ts
