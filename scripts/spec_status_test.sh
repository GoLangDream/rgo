#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/rgo_spec_status_test_XXXXXX)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/specs"
OUT="$WORK/spec_status.csv"

cat >"$WORK/specs/pass_spec.rb" <<'RUBY'
expect(1).should_not(2)
RUBY

cat >"$WORK/specs/parse_error_spec.rb" <<'RUBY'
def
RUBY

mkfifo "$WORK/specs/timeout_spec.rb"

cat >"$WORK/specs/zero_examples_spec.rb" <<'RUBY'
value = 1
RUBY

RGO_SPEC_TIMEOUT=1 "$ROOT/scripts/spec_status.sh" "$WORK/specs" "$OUT" >/dev/null

grep -q '^file,status,examples,failures,error_kind,duration_ms$' "$OUT"
grep -q "$WORK/specs/parse_error_spec.rb,parse_error,0,0,parse_error," "$OUT"
grep -q "$WORK/specs/pass_spec.rb,pass,1,0,," "$OUT"
grep -q "$WORK/specs/timeout_spec.rb,timeout,0,0,timeout," "$OUT"
grep -q "$WORK/specs/zero_examples_spec.rb,zero_examples,0,0,zero_examples," "$OUT"

RGO_TEST_MEMORY_KB=1048576 RGO_SPEC_TIMEOUT=1 "$ROOT/scripts/spec_status.sh" "$WORK/specs/pass_spec.rb" "$WORK/mem_status.csv" >/dev/null
grep -q 'pass_spec.rb,pass,1,0,,' "$WORK/mem_status.csv"

RGO_TEST_MEMORY_KB=invalid RGO_SPEC_TIMEOUT=1 "$ROOT/scripts/spec_status.sh" "$WORK/specs/pass_spec.rb" "$WORK/invalid_mem_status.csv" >/dev/null
grep -q 'pass_spec.rb,runtime_error,0,0,runtime_error,' "$WORK/invalid_mem_status.csv"
! grep -q 'pass_spec.rb,pass,1,0,,' "$WORK/invalid_mem_status.csv"

echo "spec_status_test: PASS"
