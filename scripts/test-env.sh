#!/usr/bin/env bash
# Source this to point the integration tests at the local compose services.
# Values come from the gitignored .env and are never printed.
set -a
# shellcheck disable=SC1091
. "$(git rev-parse --show-toplevel)/.env"
set +a
export ROLERANK_TEST_ADMIN_URL="postgres://postgres:${ROLERANK_PG_SUPERUSER_PASSWORD}@127.0.0.1:55433/postgres"
export ROLERANK_TEST_VALKEY_URL="redis://:${ROLERANK_VALKEY_PASSWORD}@127.0.0.1:56380"
