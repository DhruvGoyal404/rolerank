#!/usr/bin/env bash
# Labelling for D5 and D6:
#   bash scripts/label.sh select 2026-p2     draw the items
#   bash scripts/label.sh d6                 label postings, then open the page it prints
#   bash scripts/label.sh d5                 label pairs
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"
set -a
# shellcheck disable=SC1091
. ./.env
set +a
export ROLERANK_CORPUS_URL="postgres://corpus_ro:${ROLERANK_PW_CORPUS_RO}@127.0.0.1:55433/corpus"
win=$(cygpath -w "$root" 2>/dev/null || echo "$root")
cd worker
if [ "$1" = select ]; then
  exec go run ./cmd/label select "$win" "$2"
fi
exec go run ./cmd/label serve "$win" "$1"
