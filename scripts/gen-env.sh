#!/usr/bin/env bash
# Writes a .env with fresh random values, for CI and first-time setup. Prints nothing
# secret, and refuses to overwrite an existing .env.
set -euo pipefail
[ ! -e .env ] || { echo ".env already exists"; exit 1; }
while IFS= read -r line; do
  case "$line" in
    ROLERANK_*=) echo "${line}$(openssl rand -hex 24)" >>.env ;;
  esac
done <.env.example
