#!/usr/bin/env bash
# Runs cmd/embed in the project's Linux container. Usage: scripts/embed.sh jobs|text [flags]
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(git rev-parse --show-toplevel)"
bash scripts/models.sh >/dev/null
exec docker compose --profile tools run --rm -T embed go run -tags onnx ./cmd/embed "$@"
