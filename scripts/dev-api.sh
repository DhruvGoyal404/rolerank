#!/usr/bin/env bash
# Starts the built API locally against the compose services. Reads the gitignored .env
# and prints no values. Run `npm run build` in api/ first.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. ./.env
set +a
export NODE_ENV=development
export PUBLIC_ORIGIN=http://localhost:5173
export APP_DATABASE_URL="postgres://app_rw:${ROLERANK_PW_APP_RW}@127.0.0.1:55433/app"
export VALKEY_URL="redis://:${ROLERANK_VALKEY_PASSWORD}@127.0.0.1:56380"
: "${GOOGLE_CLIENT_ID:?add GOOGLE_CLIENT_ID to .env}"
: "${GOOGLE_CLIENT_SECRET:?add GOOGLE_CLIENT_SECRET to .env}"
: "${GITHUB_CLIENT_ID:?add GITHUB_CLIENT_ID to .env}"
: "${GITHUB_CLIENT_SECRET:?add GITHUB_CLIENT_SECRET to .env}"
exec node api/dist/index.js
