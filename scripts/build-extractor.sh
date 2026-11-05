#!/usr/bin/env bash
# Builds the resume extractor's sandbox image for local development and tests.
# Production images are built in the cloud from git archive HEAD (P8).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
docker build -q --label rolerank.managed=true -t rolerank-extractor:dev extractor
