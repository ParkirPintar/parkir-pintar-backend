#!/usr/bin/env bash
# Run all k6 load test scenarios
# Usage: ./run-k6.sh [base_url]

set -euo pipefail

BASE_URL="${1:-http://localhost:8000}"
DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> Target: $BASE_URL"
echo ""

run() {
  local name="$1"
  local file="$2"
  echo "==> Running: $name"
  k6 run --env BASE_URL="$BASE_URL" "$DIR/$file"
  echo ""
}

# 1. Smoke test first
run "Smoke Test" "smoke-test.js"

# 2. Full load test (all 14 scenarios)
run "Load Test (all scenarios)" "load-test.js"

# 3. War-booking stress test
run "War Booking Stress" "war-booking.js"

echo "==> All k6 tests completed"
