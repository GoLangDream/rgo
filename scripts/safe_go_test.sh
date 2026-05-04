#!/bin/bash
set -euo pipefail

TIMEOUT_SECONDS=${RGO_GO_TEST_TIMEOUT:-60}
MEMORY_KB=${RGO_TEST_MEMORY_KB:-}

args=("$@")
has_package_parallelism=0

for arg in "${args[@]}"; do
  case "$arg" in
    -args)
      break
      ;;
    -p|-p=*)
      has_package_parallelism=1
      break
      ;;
  esac
done

if [ "$has_package_parallelism" -eq 0 ]; then
  args=(-p 1 "${args[@]}")
fi

run_go_test() {
  export GOMAXPROCS=${GOMAXPROCS:-1}
  if [ -n "$MEMORY_KB" ]; then
    ulimit -v "$MEMORY_KB"
  fi
  exec go test "$@"
}

export -f run_go_test
export MEMORY_KB
timeout "$TIMEOUT_SECONDS" bash -c 'run_go_test "$@"' bash "${args[@]}"
