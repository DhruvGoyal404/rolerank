#!/usr/bin/env bash
# Tests which final backup sets rolerank-down.sh prunes (stale_finals, prune_finals in
# rolerank-lib.sh) offline: S3 is replaced by a list of stamps.
#   bash scripts/prune.test.sh
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/rolerank-lib.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
stale() { printf '%s\n' "$@" | stale_finals | paste -sd' ' -; }

[ "$KEEP_FINAL" = 2 ] || fail "KEEP_FINAL is $KEEP_FINAL"
# Fewer than or exactly two sets: nothing goes, so the newest can never be pruned.
[ "$(stale)" = "" ] || fail "empty"
[ "$(stale 20261110T132112Z)" = "" ] || fail "one set"
[ "$(stale 20261109T164419Z 20261110T132112Z)" = "" ] || fail "two sets"
# Five sets in any order: the three oldest go, the two newest stay.
[ "$(stale 20261110T132112Z 20261109T115914Z 20261109T164419Z 20261109T135346Z 20261109T121858Z)" = \
  "20261109T115914Z 20261109T121858Z 20261109T135346Z" ] || fail "five sets"
# A set listed twice counts once.
[ "$(stale 20261109T164419Z 20261110T132112Z 20261110T132112Z)" = "" ] || fail "duplicate"

# prune_finals deletes exactly the stale sets' four files, never the one just kept.
BUCKET=test-bucket
SETS="20261108T100000Z 20261109T164419Z 20261110T132112Z"
removed=()
final_stamps() { tr ' ' '\n' <<<"$SETS"; }
aws_() { [ "$1 $2" = "s3 rm" ] && removed+=("${3##*/}"); }
prune_finals 20261110T132112Z 2>/dev/null
[ "${removed[*]}" = "rolerank-20261108T100000Z-app.dump.age rolerank-20261108T100000Z-corpus.dump.age rolerank-20261108T100000Z.counts caddy-20261108T100000Z.tar.gz" ] ||
  fail "pruned ${removed[*]}"
# Even if the set just verified were the oldest (a clock gone wrong), it is never pruned.
SETS="20261101T000000Z 20261109T164419Z 20261110T132112Z" removed=()
prune_finals 20261101T000000Z 2>/dev/null
[ "${#removed[@]}" = 0 ] || fail "pruned the set just kept: ${removed[*]}"

echo "prune: 7 cases passed"
