#!/usr/bin/env bash
# Runs the ingest command against the compose services, for example:
#   bash scripts/ingest.sh status
#   bash scripts/ingest.sh boards greenhouse data/greenhouse.txt
#   bash scripts/ingest.sh run
# Safe to stop with Ctrl-C at any moment, and to run again. Reads .env; prints no values.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. ./.env
set +a
export ROLERANK_CORPUS_URL="postgres://corpus_rw:${ROLERANK_PW_CORPUS_RW}@127.0.0.1:55433/corpus"
export ROLERANK_VALKEY_URL="redis://:${ROLERANK_VALKEY_PASSWORD}@127.0.0.1:56380"
cd worker
exec go run ./cmd/ingest "$@"
