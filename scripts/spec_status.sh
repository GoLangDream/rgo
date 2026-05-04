#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  printf 'Usage: %s <spec-dir-or-file> <output.csv>\n' "$0" >&2
  exit 2
fi

TARGET=$1
OUT=$2
TIMEOUT_SECONDS=${RGO_SPEC_TIMEOUT:-5}
MEMORY_KB=${RGO_TEST_MEMORY_KB:-}
ROOT=$(cd "$(dirname "$0")/.." && pwd)

run_rgo_test() {
  local spec=$1
  local tmp=$2
  if [ -n "$MEMORY_KB" ]; then
    ulimit -v "$MEMORY_KB" || exit 125
  fi
  exec "$ROOT/rgo" test "$spec" >"$tmp" 2>&1
}
export -f run_rgo_test
export ROOT MEMORY_KB

TMPDIR=$(mktemp -d /tmp/rgo_spec_status_XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

if [ ! -x "$ROOT/rgo" ]; then
  (cd "$ROOT" && go build -o rgo ./cmd/rgo)
fi

mkdir -p "$(dirname "$OUT")"
printf 'file,status,examples,failures,error_kind,duration_ms\n' > "$OUT"

if [ -d "$TARGET" ]; then
  mapfile -t FILES < <(find "$TARGET" -name '*_spec.rb' | sort)
else
  FILES=("$TARGET")
fi

for spec in "${FILES[@]}"; do
  start=$(date +%s%3N)
  tmp=$(mktemp "$TMPDIR/spec_XXXXXX")
  set +e
  timeout --kill-after=2s "$TIMEOUT_SECONDS" bash -c 'run_rgo_test "$1" "$2"' bash "$spec" "$tmp"
  code=$?
  set -e
  end=$(date +%s%3N)
  duration=$((end - start))

  status=runtime_error
  error_kind=runtime_error
  examples=0
  failures=0

  if [ "$code" -eq 124 ] || [ "$code" -eq 137 ]; then
    status=timeout
    error_kind=timeout
  elif grep -q '^Parse Error:' "$tmp"; then
    status=parse_error
    error_kind=parse_error
  elif grep -q '^Compile Error:' "$tmp"; then
    status=compile_error
    error_kind=compile_error
  else
    summary=$(grep -E '^[0-9]+ examples, [0-9]+ failures$' "$tmp" | tail -n 1 || true)
    if [ -n "$summary" ]; then
      examples=${summary%% examples,*}
      failures=${summary#*, }
      failures=${failures%% failures}
      if [ "$examples" = "0" ] && [ "$failures" = "0" ] && [ "$code" -eq 0 ]; then
        status=zero_examples
        error_kind=zero_examples
      elif [ "$failures" = "0" ] && [ "$code" -eq 0 ]; then
        status=pass
        error_kind=
      else
        status=nonzero_failures
        error_kind=nonzero_failures
      fi
    elif grep -q '^Runtime Error:' "$tmp"; then
      status=runtime_error
      error_kind=runtime_error
    fi
  fi

  printf '%s,%s,%s,%s,%s,%s\n' "$spec" "$status" "$examples" "$failures" "$error_kind" "$duration" >> "$OUT"
  rm -f "$tmp"
done

printf 'Wrote %s (%d specs)\n' "$OUT" "${#FILES[@]}"
