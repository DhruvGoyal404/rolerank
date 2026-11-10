#!/usr/bin/env bash
# Tests rolerank-up.sh's host placement (place_host in rolerank-lib.sh) offline: a fake deploy
# refuses capacity where told to, and the order, the fallback and the stops are checked.
#   bash scripts/place-host.test.sh
set -euo pipefail
# shellcheck source=scripts/rolerank-lib.sh
. "$(dirname "$0")/rolerank-lib.sh"

cleared=0
clear_failed_host() { cleared=$((cleared + 1)); } # no AWS here: count the clean-ups instead
NO_CAPACITY=""                                    # "az/type" pairs the fake refuses
OTHER_FAILURE=""
tried=()
fake_deploy() { # AZ TYPE LOG
  tried+=("$1/$2")
  if [ "$OTHER_FAILURE" = "$1/$2" ]; then echo "AccessDenied: not allowed" >"$3"; return 1; fi
  if [[ " $NO_CAPACITY " == *" $1/$2 "* ]]; then echo "Instance: InsufficientInstanceCapacity" >"$3"; return 1; fi
  echo "deployed" >"$3"
}
fail() { echo "FAIL: $*" >&2; exit 1; }
case_() { # NAME, then expects HOST_AZ HOST_TYPE TRIED... set by the caller
  tried=() cleared=0 HOST_AZ="" HOST_TYPE=""
  place_host fake_deploy 2>/dev/null
}

# Capacity in the first AZ: one try, nothing to clear.
NO_CAPACITY="" case_
[ "$HOST_AZ/$HOST_TYPE" = "ap-south-1a/m7g.xlarge" ] && [ "${#tried[@]}" = 1 ] && [ "$cleared" = 0 ] || fail "first AZ: ${tried[*]}"

# The owner's failure: no m7g.xlarge in 1a. The same type moves to 1b, and the failed stack is cleared.
NO_CAPACITY="ap-south-1a/m7g.xlarge" case_
[ "$HOST_AZ/$HOST_TYPE" = "ap-south-1b/m7g.xlarge" ] && [ "${tried[*]}" = "ap-south-1a/m7g.xlarge ap-south-1b/m7g.xlarge" ] && [ "$cleared" = 1 ] ||
  fail "1a refused: ${tried[*]} cleared=$cleared"

# No m7g.xlarge anywhere: every AZ is tried before the fallback type, which starts again at 1a.
NO_CAPACITY="ap-south-1a/m7g.xlarge ap-south-1b/m7g.xlarge ap-south-1c/m7g.xlarge" case_
[ "$HOST_AZ/$HOST_TYPE" = "ap-south-1a/m6g.xlarge" ] && [ "${#tried[@]}" = 4 ] && [ "$cleared" = 3 ] || fail "m7g gone: ${tried[*]}"

# Only r7g.xlarge in 1c is left.
NO_CAPACITY="$(for t in m7g.xlarge m6g.xlarge; do for z in a b c; do printf 'ap-south-1%s/%s ' "$z" "$t"; done; done)ap-south-1a/r7g.xlarge ap-south-1b/r7g.xlarge" case_
[ "$HOST_AZ/$HOST_TYPE" = "ap-south-1c/r7g.xlarge" ] && [ "${#tried[@]}" = 9 ] || fail "last resort: ${tried[*]}"

# Nothing anywhere: it stops with an error after all nine, and doesn't loop.
if (NO_CAPACITY="$(for t in "${HOST_TYPES[@]}"; do for z in "${HOST_AZS[@]}"; do printf '%s/%s ' "$z" "$t"; done; done)" case_) 2>/dev/null; then
  fail "no capacity anywhere should stop"
fi

# A failure that isn't about capacity stops at once: retrying elsewhere would only hide it.
if (OTHER_FAILURE="ap-south-1a/m7g.xlarge" case_) 2>/dev/null; then fail "a non-capacity failure should stop"; fi

# The box's prices: the instance plus the disk and public IPv4.
[ "$(host_price m7g.xlarge)" = "0.127" ] && [ "$(host_price m6g.xlarge)" = "0.111" ] && [ "$(host_price r7g.xlarge)" = "0.160" ] ||
  fail "prices: $(host_price m7g.xlarge) $(host_price m6g.xlarge) $(host_price r7g.xlarge)"

echo "place-host: 7 cases passed"
