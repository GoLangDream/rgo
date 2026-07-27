#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

usage() {
  cat <<EOF
Usage: $0 [--ruby-only|--rails-only]

Run Ruby specs under vendor/ruby/spec and/or Rails specs under vendor/rails/rails
with conservative defaults for timeout/CPU/memory.

Environment variables:
  RGO_SPEC_TIMEOUT            per-file timeout for rgo test (default: 30)
  RGO_TEST_MEMORY_KB          per-file memory cap for rgo test (soft/hard KB, optional)
  RGO_SPEC_CPU_SECONDS        per-file CPU cap for rgo test (optional)
  RGO_SPEC_LOG_DIR            directory to store non-pass spec logs (optional)
  RGO_RUBY_SPEC_TARGET        ruby-spec root to scan (default: vendor/ruby/spec)
  RGO_INCLUDE_OPTIONAL_CAPI   run MRI C-extension ABI specs when set to 1 (default: unsupported_capi)

  RGO_GO_TEST_TIMEOUT         timeout for each go test invocation (default: 60)
  RGO_GO_TEST_CPU_SECONDS     CPU cap for go test via safe_go_test.sh (optional)

  RGO_RAILS_TEST_TIMEOUT      timeout for each Rails task (default: 1200)
  RGO_RAILS_TEST_CPU_SECONDS  CPU cap for Rails task (optional)
  RAILS_PARALLEL_WORKERS      ActiveSupport test worker count (default: 1)
  RGO_RAILS_TASKS             custom space-separated Rails rake tasks
  RGO_FULL_SPEC_CONTINUE_ON_FAIL keep running when one task fails (default: 1)
  RAILS_BUNDLE_PRECHECK       run bundle dependency precheck by default: 1 (set 0 to skip)
EOF
}

MODE="both"
if [ "$#" -gt 1 ]; then
  usage >&2
  exit 2
elif [ "$#" -eq 1 ]; then
  case "$1" in
    --ruby-only)
      MODE="ruby"
      ;;
    --rails-only)
      MODE="rails"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
fi

RUBY_SPEC_TARGET=${RGO_RUBY_SPEC_TARGET:-"$ROOT/vendor/ruby/spec"}
RAILS_ROOT=${RGO_RAILS_ROOT:-"$ROOT/vendor/rails/rails"}
RAILS_TEST_MEMORY_KB="${RGO_RAILS_TEST_MEMORY_KB:-}"
RAILS_TEST_CPU_SECONDS="${RGO_RAILS_TEST_CPU_SECONDS:-}"
export RGO_SPEC_LOG_DIR
if [ -n "${RGO_SPEC_LOG_DIR:-}" ] && [ "${MODE}" != "rails" ]; then
  mkdir -p "$RGO_SPEC_LOG_DIR"
fi
export RAILS_ROOT
export RAILS_TEST_MEMORY_KB
export RAILS_TEST_CPU_SECONDS
export RAILS_PARALLEL_WORKERS
export RAILS_TEST_EXECUTABLE

if [ "${RGO_FULL_SPEC_CONTINUE_ON_FAIL:-1}" != "0" ]; then
  CONTINUE_ON_FAIL=1
else
  CONTINUE_ON_FAIL=0
fi

mkdir -p "$ROOT/reports/spec-status"
RUBY_REPORT="$ROOT/reports/spec-status/ruby-spec-full.csv"
RAILS_REPORT="$ROOT/reports/spec-status/rails-spec-full.csv"
RAILS_LOG_DIR="$ROOT/reports/spec-status/rails-task-logs"
mkdir -p "$RAILS_LOG_DIR"

run_rails_task() {
  local task=$1
  local log_file=$2
  local start
  local end
  local duration
  local code=0

  start=$(date +%s%3N)
  set +e
  (
    cd "$RAILS_ROOT"
    export PARALLEL_WORKERS="${RAILS_PARALLEL_WORKERS:-1}"
    export RAILS_TEST_EXECUTABLE="${RAILS_TEST_EXECUTABLE:-bin/test}"
    if [ -n "${RAILS_TEST_MEMORY_KB:-}" ]; then
      ulimit -v "${RAILS_TEST_MEMORY_KB}" || exit 125
    fi
    if [ -n "${RAILS_TEST_CPU_SECONDS:-}" ]; then
      ulimit -t "${RAILS_TEST_CPU_SECONDS}" || exit 125
    fi
    bundle exec rake "$task" >"$log_file" 2>&1
  )
  code=$?
  set -e
  end=$(date +%s%3N)
  duration=$((end - start))

  echo "$task,$code,$duration,$log_file"
}
export -f run_rails_task

if [ "$MODE" != "rails" ]; then
  echo "[full-spec-gate] running ruby specs from: $RUBY_SPEC_TARGET"
  if [ ! -e "$RUBY_SPEC_TARGET" ]; then
    echo "ruby spec target not found: $RUBY_SPEC_TARGET" >&2
    exit 2
  fi
  if [ -f "$RUBY_SPEC_TARGET" ] && [ "${RUBY_SPEC_TARGET##*.}" != "rb" ]; then
    echo "ruby spec target must be a directory or .rb file: $RUBY_SPEC_TARGET" >&2
    exit 2
  fi
  RGO_SPEC_TIMEOUT="${RGO_SPEC_TIMEOUT:-5}" \
  RGO_TEST_MEMORY_KB="${RGO_TEST_MEMORY_KB:-2000000}" \
  RGO_SPEC_CPU_SECONDS="${RGO_SPEC_CPU_SECONDS:-}" \
  RGO_SPEC_LOG_DIR="${RGO_SPEC_LOG_DIR:-$ROOT/reports/spec-status/spec-logs}" \
  "$ROOT/scripts/spec_status.sh" "$RUBY_SPEC_TARGET" "$RUBY_REPORT"
  echo "[full-spec-gate] ruby spec report: $RUBY_REPORT"
fi

if [ "$MODE" != "ruby" ]; then
  echo "[full-spec-gate] running rails specs from: $RAILS_ROOT"
  if [ ! -d "$RAILS_ROOT" ]; then
    echo "rails spec target not found: $RAILS_ROOT" >&2
    printf 'framework,status,duration_ms,exit_code,log\n' > "$RAILS_REPORT"
    printf 'rails,target_missing,0,2,\n' >> "$RAILS_REPORT"
    echo "[full-spec-gate] rails spec report: $RAILS_REPORT"
    if [ "$CONTINUE_ON_FAIL" -eq 0 ]; then
      exit 2
    fi
    exit 0
  fi
  if [ ! -f "$RAILS_ROOT/Gemfile" ] && [ ! -f "$RAILS_ROOT/Rakefile" ]; then
    echo "rails spec target missing Gemfile and Rakefile: $RAILS_ROOT" >&2
    exit 2
  fi

  if [ -n "${RGO_RAILS_TASKS:-}" ]; then
    read -r -a rails_tasks <<< "$RGO_RAILS_TASKS"
  else
    rails_tasks=(
      "activesupport:test"
      "actionpack:test"
      "actionview:test"
      "activemodel:test"
      "activejob:test"
      "actionmailer:test"
      "actionmailbox:test"
      "actiontext:test"
      "actioncable:test"
      "activestorage:test"
      "activerecord:test:sqlite3:test"
      "railties:test"
    )
  fi

  bundle_check_log="$RAILS_LOG_DIR/bundle_check.log"
  if ! (cd "$RAILS_ROOT" && bundle check > "$bundle_check_log" 2>&1); then
    if [ "${RAILS_BUNDLE_PRECHECK:-1}" != "0" ]; then
      echo "Rails bundle dependencies are not installed. Run 'bundle install' in $RAILS_ROOT." >&2
      printf 'framework,status,duration_ms,exit_code,log\n' > "$RAILS_REPORT"
      for task in "${rails_tasks[@]}"; do
        printf '%s,bundle_missing,0,1,%s\n' "$task" "$bundle_check_log" >> "$RAILS_REPORT"
      done
      if [ "$CONTINUE_ON_FAIL" -eq 0 ]; then
        exit 1
      fi
      echo "[full-spec-gate] rails spec report: $RAILS_REPORT"
      exit 0
    fi
  fi

  if [ -n "${RAILS_PARALLEL_WORKERS:-}" ] && [[ ! "$RAILS_PARALLEL_WORKERS" =~ ^[0-9]+$ ]]; then
    echo "Invalid RAILS_PARALLEL_WORKERS: $RAILS_PARALLEL_WORKERS" >&2
    exit 2
  fi

  printf 'framework,status,duration_ms,exit_code,log\n' > "$RAILS_REPORT"
  for task in "${rails_tasks[@]}"; do
    safe_task_name="${task//:/_}"
    safe_task_name="${safe_task_name//\//_}"
    log_file="$RAILS_LOG_DIR/${safe_task_name}.log"
    set +e
    result=$(timeout "${RGO_RAILS_TEST_TIMEOUT:-1200}" bash -c 'run_rails_task "$1" "$2"' bash "$task" "$log_file")
    code=$?
    set -e

    if [ -z "$result" ]; then
      result="$task,0,0,$log_file"
    fi

    status_code=$(echo "$result" | awk -F, '{print $2}')
    duration_ms=$(echo "$result" | awk -F, '{print $3}')
    task_name=$(echo "$result" | awk -F, '{print $1}')

    if [ -z "$status_code" ]; then
      status_code=$code
    fi
    if [ -z "$duration_ms" ]; then
      duration_ms=0
    fi
    if [ "$code" -eq 0 ] && [ "$status_code" -eq 0 ]; then
      status="pass"
    elif [ "$code" -eq 124 ]; then
      status="timeout"
    elif [ "$status_code" -eq 125 ]; then
      status="invalid_limit"
    elif [ "$status_code" -eq 137 ] || [ "$status_code" -eq 143 ]; then
      status="oom_or_killed"
    elif [ "$code" -ne 0 ]; then
      status="failure"
    else
      status="failure"
    fi

    printf '%s,%s,%s,%s,%s\n' "$task_name" "$status" "$duration_ms" "$status_code" "$log_file" >> "$RAILS_REPORT"

    if [ "$status" != "pass" ] && [ "$CONTINUE_ON_FAIL" -eq 0 ]; then
      echo "task failed: $task_name" >&2
      exit "$status_code"
    fi
  done
  echo "[full-spec-gate] rails spec report: $RAILS_REPORT"
fi
