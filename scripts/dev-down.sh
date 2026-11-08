#!/usr/bin/env bash
# Stops everything scripts/dev-up.sh started: the extractor, aiworker, API and site, the ranker
# container, and Postgres and Valkey. Data stays in the compose volumes. Touches only this
# project's processes and containers. Safe to run twice.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(git rev-parse --show-toplevel)"
DEV="$(pwd)/.dev"

# Git Bash's own PIDs aren't the Windows programs it started, so stop by port instead: whatever
# listens on RoleRank's local ports, and only if it is one of ours (node or the extractor).
stopped=$(powershell -NoProfile -Command '
  $names = @("node", "rolerank-extractor")
  foreach ($port in 5173, 8080, 8090, 8093) {
    foreach ($c in @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)) {
      $p = Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue
      if ($p -and $names -contains $p.ProcessName) { Stop-Process -Id $p.Id -Force; "port-$port-$($p.ProcessName)" }
    }
  }
  exit 0' | tr -d '\r')
if [ -n "$stopped" ]; then
  for s in $stopped; do echo "stopped ${s//-/ }"; done
else
  echo "No RoleRank processes were running."
fi
rm -f "$DEV/pids"

if docker info >/dev/null 2>&1; then
  docker compose --profile ranker stop ranker >/dev/null 2>&1 && echo "stopped ranker" || true
  docker compose --profile ranker rm -f ranker >/dev/null 2>&1 || true
  docker compose stop postgres valkey >/dev/null 2>&1 && echo "stopped postgres and valkey" || true
fi
echo "RoleRank is stopped. Start again with: bash scripts/dev-up.sh"
