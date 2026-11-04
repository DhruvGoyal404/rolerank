#!/usr/bin/env bash
# Search latency bench on a synthetic corpus in its own *_test database.
#   scripts/bench.sh load 500000
#   scripts/bench.sh [-users 20 -queries 2000 -writes 50] run
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
# shellcheck disable=SC1091
. scripts/test-env.sh >/dev/null
cd worker
exec go run ./cmd/bench "$@"
