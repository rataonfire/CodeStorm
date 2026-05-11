#!/bin/sh
set -e

REPORT_TXT=/tests/test_report.txt
REPORT_JSON=/tests/test_report.json
rm -f "$REPORT_TXT" "$REPORT_JSON"

total=0
passed=0
failed=0
skipped=0
results="[]"

add_result() {
  name="$1"
  status="$2"
  message="$3"
  total=$((total + 1))
  if [ "$status" = "passed" ]; then
    passed=$((passed + 1))
  elif [ "$status" = "failed" ]; then
    failed=$((failed + 1))
  else
    skipped=$((skipped + 1))
  fi
  results=$(echo "$results" | jq --arg name "$name" --arg status "$status" --arg message "$message" '. + [{name: $name, status: $status, message: $message}]')
}

wait_for_service() {
  name="$1"
  url="$2"
  retries=60
  echo "Waiting for $name..."
  for i in $(seq 1 "$retries"); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      echo "$name ready"
      return 0
    fi
    sleep 2
    echo "retrying $name... ($i)"
  done
  return 1
}

run_check() {
  name="$1"
  shift
  echo "Running test: $name"
  if sh -c "$*" >/tmp/test_output 2>&1; then
    echo "PASS: $name" | tee -a "$REPORT_TXT"
    add_result "$name" "passed" "$(cat /tmp/test_output | tr '\n' ' ' | sed 's/  */ /g')"
    return 0
  else
    echo "FAIL: $name" | tee -a "$REPORT_TXT"
    cat /tmp/test_output | tee -a "$REPORT_TXT"
    add_result "$name" "failed" "$(cat /tmp/test_output | tr '\n' ' ' | sed 's/  */ /g')"
    return 1
  fi
}

wait_for_service "matcher readiness endpoint" "http://matcher:8081/readyz"
if [ $? -ne 0 ]; then
  add_result "matcher readyz" "failed" "matcher readiness endpoint did not become available"
  exit 1
fi

wait_for_service "api gateway transaction endpoint" "http://api:8000/api/transactions"
if [ $? -ne 0 ]; then
  add_result "api gateway" "failed" "api gateway endpoint did not become available"
  exit 1
fi

run_check "matcher health endpoint" "curl -sSf http://matcher:8081/healthz >/dev/null"
run_check "matcher readiness endpoint" "curl -sSf http://matcher:8081/readyz >/dev/null"
run_check "api transactions endpoint" "curl -sSf -H 'Accept: application/json' http://api:8000/api/transactions | jq -e 'has(\"data\") and (.data | type == \"array\")' >/dev/null"
run_check "api incidents endpoint" "curl -sSf -H 'Accept: application/json' http://api:8000/api/incidents | jq -e 'type == \"array\"' >/dev/null"
run_check "api metrics endpoint" "curl -sSf -H 'Accept: application/json' http://api:8000/api/metrics/mismatch-per-minute | jq -e 'type == \"array\"' >/dev/null"

# Optional detail lookup if transactions exist
TRANSACTION_IDS=$(curl -sSf -H 'Accept: application/json' http://api:8000/api/transactions | jq -r '.data[].transaction_id' | head -n 1)
if [ -n "$TRANSACTION_IDS" ]; then
  run_check "api transaction details endpoint" "curl -sSf -H 'Accept: application/json' http://api:8000/api/transactions/$TRANSACTION_IDS | jq -e 'has(\"summary\") and has(\"details\")' >/dev/null"
else
  echo "SKIP: api transaction details endpoint (no transactions returned)" | tee -a "$REPORT_TXT"
  add_result "api transaction details endpoint" "skipped" "no transactions available"
fi

cat > "$REPORT_JSON" <<EOF
{
  "summary": {
    "total": $total,
    "passed": $passed,
    "failed": $failed,
    "skipped": $skipped
  },
  "results": $results
}
EOF

echo ""
echo "Integration test summary"
echo "------------------------"
echo "Total: $total"
echo "Passed: $passed"
echo "Failed: $failed"
echo "Skipped: $skipped"
echo ""
echo "Full report saved to $REPORT_TXT and $REPORT_JSON"
if [ "$failed" -ne 0 ]; then
  exit 1
fi

echo "All connectivity and endpoint checks passed."
