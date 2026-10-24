#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go run ./cmd/demo -base "${BASE_URL:-http://localhost:8080}" -load-only -requests "${REQUESTS:-104}"
