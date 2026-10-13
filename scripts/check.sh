#!/bin/bash
# Runs the checks of the CI "check" job locally, read from
# .github/workflows/ci.yml, so local runs and CI cannot drift apart.
# Usage: scripts/check.sh [-list]
set -e
cd "$(dirname "$0")/.."
exec go run ./internal/tools/cilocal "$@"
