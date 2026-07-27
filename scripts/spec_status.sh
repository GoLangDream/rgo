#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  printf 'Usage: %s <spec-dir-or-file> <output.csv>\n' "$0" >&2
  exit 2
fi

TARGET=$1
OUT=$2
TIMEOUT_SECONDS=${RGO_SPEC_TIMEOUT:-30}
MEMORY_KB=${RGO_TEST_MEMORY_KB:-}
CPU_SECONDS=${RGO_SPEC_CPU_SECONDS:-}
INCLUDE_OPTIONAL_CAPI=${RGO_INCLUDE_OPTIONAL_CAPI:-0}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
LOG_DIR=${RGO_SPEC_LOG_DIR:-}

if [ -n "${LOG_DIR}" ]; then
  mkdir -p "$LOG_DIR"
fi

run_rgo_test() {
  local spec=$1
  local tmp=$2
  if [ -n "$MEMORY_KB" ]; then
    case "$MEMORY_KB" in
      '' | *[!0-9]*)
        echo "Invalid RGO_TEST_MEMORY_KB: $MEMORY_KB" >&2
        return 125
        ;;
    esac
    ulimit -v "$MEMORY_KB" || return 125
  fi
  if [ -n "$CPU_SECONDS" ]; then
    case "$CPU_SECONDS" in
      '' | *[!0-9]*)
        echo "Invalid RGO_SPEC_CPU_SECONDS: $CPU_SECONDS" >&2
        return 125
        ;;
    esac
    ulimit -t "$CPU_SECONDS" || return 125
  fi
  exec "$ROOT/rgo" test "$spec" >"$tmp" 2>&1
}
export -f run_rgo_test
export ROOT MEMORY_KB

TMPDIR=$(mktemp -d /tmp/rgo_spec_status_XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

BUILD_BINARY="${BUILD_BINARY:-1}"
if [ "$BUILD_BINARY" = "1" ]; then
  (cd "$ROOT" && GOCACHE="${GOCACHE:-/tmp/rgo-go-build-cache}" GOMODCACHE="${GOMODCACHE:-/tmp/rgo-go-mod-cache}" go build -o rgo ./cmd/rgo)
fi

mkdir -p "$(dirname "$OUT")"
printf 'file,status,examples,failures,error_kind,duration_ms\n' > "$OUT"

if [ -d "$TARGET" ]; then
  mapfile -t FILES < <(find "$TARGET" \( -name '*_spec.rb' -o -name '*_test.rb' \) | sort)
else
  FILES=("$TARGET")
fi

for spec in "${FILES[@]}"; do
  if [[ "$spec" == */optional/capi/* ]] && [ "$INCLUDE_OPTIONAL_CAPI" != "1" ]; then
    printf '%s,unsupported_capi,0,0,unsupported_capi,0\n' "$spec" >> "$OUT"
    continue
  fi
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
  saved_log=

  if [ "$code" -eq 124 ]; then
    status=timeout
    error_kind=timeout
  elif [ "$code" -eq 137 ] || [ "$code" -eq 143 ]; then
    status=oom_or_killed
    error_kind=oom_or_killed
  elif grep -a -q '^Parse Error:' "$tmp"; then
    status=parse_error
    error_kind=parse_error
  elif grep -a -q '^Compile Error:' "$tmp"; then
    status=compile_error
    error_kind=compile_error
  elif grep -a -q '^Runtime Error:' "$tmp"; then
    status=runtime_error
    error_kind=runtime_error
  else
    summary=$(grep -a -E '^[0-9]+ examples, [0-9]+ failures$' "$tmp" | tail -n 1 || true)
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
    elif grep -a -q '^Runtime Error:' "$tmp"; then
      status=runtime_error
      error_kind=runtime_error
    fi
  fi

  if [ "$LOG_DIR" != "" ] && [ "$status" != "pass" ]; then
    rel=${spec#"$ROOT/"}
    log_path="$LOG_DIR/${rel//\//_}.log"
    cp "$tmp" "$log_path"
    saved_log=$log_path
  fi

  printf '%s,%s,%s,%s,%s,%s\n' "$spec" "$status" "$examples" "$failures" "$error_kind" "$duration" >> "$OUT"
  if [ "$status" != "pass" ] && [ "$LOG_DIR" != "" ]; then
    printf '%s failure log: %s\n' "$spec" "$saved_log"
  fi
  rm -f "$tmp"
done

printf 'Wrote %s (%d specs)\n' "$OUT" "${#FILES[@]}"
