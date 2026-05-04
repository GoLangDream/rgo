#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/rgo_vm_test_status_XXXXXX)
trap 'rm -rf "$WORK"' EXIT

OUT="$WORK/vm-test-status.csv"

RGO_GO_TEST_TIMEOUT=10 "$ROOT/scripts/vm_test_status.sh" "$OUT" '^TestIntegerAddition$' >"$WORK/out" 2>&1

grep -q '^test,status,duration_ms$' "$OUT"
grep -q '^TestIntegerAddition,pass,' "$OUT"

echo "vm_test_status_test: PASS"
