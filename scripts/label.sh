#!/usr/bin/env bash
# Labelling for D5 and D6:
#   bash scripts/label.sh select 2026-p2     draw the items
#   bash scripts/label.sh d6                 label postings, then open the page it prints
#   bash scripts/label.sh d5                 label pairs
#   bash scripts/label.sh recheck d5 evals/labels/d5-recheck-ids.txt <seed>   draw a second pass
#   bash scripts/label.sh d5-recheck         label it, at least a day after the first pass
#   bash scripts/label.sh agree d5           score the finished second pass
#   bash scripts/label.sh recheck d6 evals/labels/d6-recheck-ids.txt <seed> full   same, with full text
#   bash scripts/label.sh redo d5 evals/labels/d5-redo-ids.txt <seed>   draw a redo, with full text
#   bash scripts/label.sh d5-redo            label it, after the recheck
#   bash scripts/label.sh redo d5 evals/labels/d5-redo-2-ids.txt <seed> redo-2   the other pairs
#   bash scripts/label.sh d5-redo-2          label them
#   bash scripts/label.sh d5-audit           audit the model's labels: answer, then see the model's
#   bash scripts/label.sh audit d5-audit     score the finished audit against the model
#   bash scripts/label.sh d5-merged-audit    the 10-item audit of the merged-pair top-up
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
case "$1" in
  select) exec go run ./cmd/label select "$win" "$2" ;;
  recheck) exec go run ./cmd/label recheck "$win" "$2" "$win/$3" "$4" ${5:+"$5"} ;;
  redo) exec go run ./cmd/label redo "$win" "$2" "$win/$3" "$4" ${5:+"$5"} ;;
  agree) exec go run ./cmd/label agree "$win" "$2" ;;
  audit) exec go run ./cmd/label audit "$win" "$2" ;;
esac
exec go run ./cmd/label serve "$win" "$1"
