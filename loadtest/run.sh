#!/bin/bash
# GopherWallet Load Test Runner
# Usage: ./loadtest/run.sh [hey|k6]

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
METHOD="${1:-hey}"

echo "=== GopherWallet Load Test ==="
echo "Target: $BASE_URL"
echo ""

# Wait for service to be ready
echo "Waiting for service..."
for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/health" > /dev/null 2>&1; then
    echo "Service is ready!"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Service not ready after 30s"
    exit 1
  fi
  sleep 1
done

echo ""

# Create test accounts
echo "Creating test accounts..."
ACCT1=$(curl -sf -X POST "$BASE_URL/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"loadtest-a","balance":999999999,"currency":"THB"}')
ACCT2=$(curl -sf -X POST "$BASE_URL/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"loadtest-b","balance":999999999,"currency":"THB"}')

ID1=$(echo "$ACCT1" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
ID2=$(echo "$ACCT2" | grep -o '"id":[0-9]*' | grep -o '[0-9]*')

echo "Account A: ID=$ID1"
echo "Account B: ID=$ID2"
echo ""

if [ "$METHOD" = "k6" ]; then
  echo "=== Running k6 load test ==="
  k6 run --env BASE_URL="$BASE_URL" loadtest/k6_transfer.js
else
  echo "=== Running load test ==="
  echo ""

  echo "--- Test 1: Health endpoint (baseline) ---"
  hey -n 1000 -c 50 "$BASE_URL/health"
  echo ""

  echo "--- Test 2: Transfer - 500 requests, 20 concurrent ---"
  go run ./loadtest/cmd "$ID1" "$ID2" 500 20
  echo ""

  echo "--- Test 3: Transfer - 1000 requests, 50 concurrent (stress) ---"
  go run ./loadtest/cmd "$ID1" "$ID2" 1000 50
  echo ""

  echo "--- Test 4: Get Account (read performance) ---"
  hey -n 2000 -c 100 "$BASE_URL/api/v1/accounts/$ID1"
fi

echo ""
echo "=== Load test complete ==="
echo "Check Grafana at http://localhost:3000 (admin/admin) for dashboards"
echo "Check Prometheus at http://localhost:9090 for raw metrics"
