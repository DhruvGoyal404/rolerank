#!/usr/bin/env bash
# Runs cmd/rank (D3 pooling and scoring) against the local corpus, read-only.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. ./.env
set +a
export ROLERANK_CORPUS_URL="postgres://corpus_ro:${ROLERANK_PW_CORPUS_RO}@127.0.0.1:55433/corpus"
root=$(pwd)
win=$(cygpath -w "$root" 2>/dev/null || echo "$root")
cd worker
args=("$1")
for a in "${@:2}"; do args+=("$win/$a"); done
exec go run ./cmd/rank "${args[@]}"
