#!/bin/bash
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  printf 'Usage: %s <output.csv> [test-name-regex]\n' "$0" >&2
  exit 2
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=$1
FILTER=${2:-'^Test'}

mkdir -p "$(dirname "$OUT")"
printf 'test,status,duration_ms\n' > "$OUT"

set +e
grep -E "$FILTER" </dev/null >/dev/null 2>&1
grep_code=$?
set -e
if [ "$grep_code" -eq 2 ]; then
  printf 'Invalid test-name regex: %s\n' "$FILTER" >&2
  exit 2
fi

mapfile -t TESTS < <(cd "$ROOT" && go test ./pkg/vm -list . | grep -E '^Test' | grep -E "$FILTER" | sort)

for test_name in "${TESTS[@]}"; do
  start=$(date +%s%3N)
  set +e
  (cd "$ROOT" && scripts/safe_go_test.sh ./pkg/vm -run "^${test_name}$" -count=1 >/dev/null 2>&1)
  code=$?
  set -e
  end=$(date +%s%3N)
  duration=$((end - start))

  status=error
  if [ "$code" -eq 0 ]; then
    status=pass
  elif [ "$code" -eq 124 ]; then
    status=timeout
  elif [ "$code" -eq 137 ] || [ "$code" -eq 143 ]; then
    status=oom_or_killed
  fi

  printf '%s,%s,%s\n' "$test_name" "$status" "$duration" >> "$OUT"
done

printf 'Wrote %s (%d tests)\n' "$OUT" "${#TESTS[@]}"
