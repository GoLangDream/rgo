# OOM-Safe Test Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add bounded test runners so broad Codex/manual test runs cannot exhaust system memory, and add diagnostics to identify the `pkg/vm` test that triggers runaway execution.

**Architecture:** Introduce small shell scripts under `scripts/` that wrap existing commands without changing VM semantics. Broad gates use the safe wrapper; a separate diagnostic script runs VM tests one at a time and emits CSV status.

**Tech Stack:** Bash, Go test, GNU `timeout`, existing RGo scripts.

---

## File Structure

- Create `scripts/safe_go_test.sh`: safe `go test` wrapper with default serial package execution, wall-clock timeout, optional virtual-memory cap, and argument passthrough.
- Create `scripts/safe_go_test_test.sh`: shell-level tests for wrapper defaults and passthrough behavior.
- Modify `scripts/array_spec_gate.sh`: replace bare `go test ./...` with `scripts/safe_go_test.sh ./...`.
- Create `scripts/vm_test_status.sh`: enumerate `pkg/vm` tests, run each in isolation through the safe wrapper, emit CSV.
- Create `scripts/vm_test_status_test.sh`: shell-level smoke test for CSV generation and at least one VM test row.
- Modify `scripts/spec_status.sh`: add optional `RGO_TEST_MEMORY_KB` cap around each `rgo test` subprocess while preserving CSV schema.
- Modify `TODO.md`: add any reproducer discovered during diagnostic verification if a timeout/OOM is observed.

## Task 1: Add Safe Go Test Wrapper

**Files:**
- Create: `scripts/safe_go_test.sh`
- Create: `scripts/safe_go_test_test.sh`

- [ ] **Step 1: Write the wrapper test script**

Create `scripts/safe_go_test_test.sh`:

```bash
#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/rgo_safe_go_test_XXXXXX)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/example"

cat > "$WORK/example/go.mod" <<'GO'
module example

go 1.22
GO

cat > "$WORK/example/example_test.go" <<'GO'
package example

import "testing"

func TestPass(t *testing.T) {}

func TestSkipTarget(t *testing.T) {
    t.Fatal("this test should not run")
}
GO

cd "$WORK/example"

RGO_GO_TEST_TIMEOUT=5 "$ROOT/scripts/safe_go_test.sh" -run '^TestPass$' . >/tmp/rgo_safe_go_test_out 2>&1
grep -q 'ok[[:space:]]\+example' /tmp/rgo_safe_go_test_out

echo "safe_go_test_test: PASS"
```

- [ ] **Step 2: Run the wrapper test to verify it fails before implementation**

Run: `bash scripts/safe_go_test_test.sh`

Expected: FAIL with `No such file or directory` for `scripts/safe_go_test.sh`.

- [ ] **Step 3: Implement the safe wrapper**

Create `scripts/safe_go_test.sh`:

```bash
#!/bin/bash
set -euo pipefail

TIMEOUT_SECONDS=${RGO_GO_TEST_TIMEOUT:-60}
MEMORY_KB=${RGO_TEST_MEMORY_KB:-}

args=("$@")
has_package_parallelism=0

for arg in "${args[@]}"; do
  case "$arg" in
    -p|-p=*)
      has_package_parallelism=1
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
```

- [ ] **Step 4: Run shell syntax check**

Run: `bash -n scripts/safe_go_test.sh scripts/safe_go_test_test.sh`

Expected: no output.

- [ ] **Step 5: Make scripts executable**

Run: `chmod +x scripts/safe_go_test.sh scripts/safe_go_test_test.sh`

Expected: no output.

- [ ] **Step 6: Run the wrapper test**

Run: `bash scripts/safe_go_test_test.sh`

Expected: `safe_go_test_test: PASS`.

- [ ] **Step 7: Run a real package through the wrapper**

Run: `RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/lexer`

Expected: `ok` for `github.com/GoLangDream/rgo/pkg/lexer`.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
git add scripts/safe_go_test.sh scripts/safe_go_test_test.sh
git commit -m "test: add safe go test wrapper"
```

Expected: commit succeeds.

## Task 2: Integrate Safe Wrapper Into Array Gate

**Files:**
- Modify: `scripts/array_spec_gate.sh:7`

- [ ] **Step 1: Write the expected gate command change**

No separate test file is needed; this task changes one gate command and verifies by running the existing script with safe limits.

- [ ] **Step 2: Modify the gate**

Change `scripts/array_spec_gate.sh` line 7 from:

```bash
(cd "$ROOT" && go test ./...)
```

to:

```bash
(cd "$ROOT" && scripts/safe_go_test.sh ./...)
```

- [ ] **Step 3: Run shell syntax checks**

Run: `bash -n scripts/array_spec_gate.sh scripts/safe_go_test.sh`

Expected: no output.

- [ ] **Step 4: Run a bounded gate smoke check**

Run: `RGO_GO_TEST_TIMEOUT=30 RGO_SPEC_TIMEOUT=1 scripts/array_spec_gate.sh`

Expected: either PASS, or a bounded failure that exits without OOM. If it fails because an existing test/spec times out, record the command and status in `TODO.md` instead of fixing VM behavior in this task.

- [ ] **Step 5: Commit Task 2**

Run:

```bash
git add scripts/array_spec_gate.sh TODO.md
git commit -m "test: run array gate through safe go wrapper"
```

Expected: commit succeeds. If `TODO.md` was unchanged, omit it from `git add`.

## Task 3: Add VM Test Diagnostic Runner

**Files:**
- Create: `scripts/vm_test_status.sh`
- Create: `scripts/vm_test_status_test.sh`

- [ ] **Step 1: Write the diagnostic runner smoke test**

Create `scripts/vm_test_status_test.sh`:

```bash
#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d /tmp/rgo_vm_test_status_XXXXXX)
trap 'rm -rf "$WORK"' EXIT

OUT="$WORK/vm-test-status.csv"

RGO_GO_TEST_TIMEOUT=10 "$ROOT/scripts/vm_test_status.sh" "$OUT" '^TestIntegerAddition$' >/tmp/rgo_vm_test_status_out 2>&1

grep -q '^test,status,duration_ms$' "$OUT"
grep -q '^TestIntegerAddition,pass,' "$OUT"

echo "vm_test_status_test: PASS"
```

- [ ] **Step 2: Run the diagnostic test to verify it fails before implementation**

Run: `bash scripts/vm_test_status_test.sh`

Expected: FAIL with `No such file or directory` for `scripts/vm_test_status.sh`.

- [ ] **Step 3: Implement the diagnostic runner**

Create `scripts/vm_test_status.sh`:

```bash
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

mapfile -t TESTS < <(cd "$ROOT" && go test ./pkg/vm -list . | grep -E "$FILTER" | sort)

for test_name in "${TESTS[@]}"; do
  start=$(date +%s%3N)
  tmp=$(mktemp /tmp/rgo_vm_test_status_XXXXXX)
  set +e
  (cd "$ROOT" && scripts/safe_go_test.sh ./pkg/vm -run "^${test_name}$" -count=1 >"$tmp" 2>&1)
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
  rm -f "$tmp"
done

printf 'Wrote %s (%d tests)\n' "$OUT" "${#TESTS[@]}"
```

- [ ] **Step 4: Make scripts executable**

Run: `chmod +x scripts/vm_test_status.sh scripts/vm_test_status_test.sh`

Expected: no output.

- [ ] **Step 5: Run the diagnostic smoke test**

Run: `bash scripts/vm_test_status_test.sh`

Expected: `vm_test_status_test: PASS`.

- [ ] **Step 6: Run a bounded diagnostic sample**

Run: `RGO_GO_TEST_TIMEOUT=10 scripts/vm_test_status.sh reports/vm-test-status.csv '^TestInteger(Addition|Subtraction)$'`

Expected: `reports/vm-test-status.csv` contains two `pass` rows.

- [ ] **Step 7: Commit Task 3**

Run:

```bash
git add scripts/vm_test_status.sh scripts/vm_test_status_test.sh reports/vm-test-status.csv
git commit -m "test: add vm test status runner"
```

Expected: commit succeeds.

## Task 4: Add Optional Memory Cap To Spec Status

**Files:**
- Modify: `scripts/spec_status.sh:11-31`
- Modify: `scripts/spec_status_test.sh`

- [ ] **Step 1: Extend the existing spec status test**

Add this line after line 40 in `scripts/spec_status_test.sh`:

```bash
RGO_TEST_MEMORY_KB=1048576 RGO_SPEC_TIMEOUT=1 "$ROOT/scripts/spec_status.sh" "$WORK/specs/pass_spec.rb" "$WORK/mem_status.csv" >/dev/null
```

Add this assertion after the existing header assertion:

```bash
grep -q 'pass_spec.rb,pass,1,0,,' "$WORK/mem_status.csv"
```

- [ ] **Step 2: Run the test before implementation**

Run: `bash scripts/spec_status_test.sh`

Expected: It may still pass if the current script ignores `RGO_TEST_MEMORY_KB`; that is acceptable because this is a compatibility-preserving hardening change.

- [ ] **Step 3: Add memory-cap variables and helper**

In `scripts/spec_status.sh`, after line 11, add:

```bash
MEMORY_KB=${RGO_TEST_MEMORY_KB:-}
```

After the `ROOT=...` line, add:

```bash
run_rgo_test() {
  local spec=$1
  local tmp=$2
  if [ -n "$MEMORY_KB" ]; then
    ulimit -v "$MEMORY_KB"
  fi
  exec "$ROOT/rgo" test "$spec" >"$tmp" 2>&1
}
export -f run_rgo_test
export ROOT MEMORY_KB
```

- [ ] **Step 4: Use the helper for each spec subprocess**

Replace line 31 in `scripts/spec_status.sh`:

```bash
timeout "$TIMEOUT_SECONDS" "$ROOT/rgo" test "$spec" >"$tmp" 2>&1
```

with:

```bash
timeout "$TIMEOUT_SECONDS" bash -c 'run_rgo_test "$1" "$2"' bash "$spec" "$tmp"
```

- [ ] **Step 5: Run syntax and status tests**

Run: `bash -n scripts/spec_status.sh scripts/spec_status_test.sh && bash scripts/spec_status_test.sh`

Expected: `spec_status_test: PASS`.

- [ ] **Step 6: Commit Task 4**

Run:

```bash
git add scripts/spec_status.sh scripts/spec_status_test.sh
git commit -m "test: support memory cap in spec status"
```

Expected: commit succeeds.

## Task 5: Run Safe Diagnostics And Record Findings

**Files:**
- Modify: `TODO.md` only if diagnostics find timeout/killed/error rows
- Create/Modify: `reports/vm-test-status.csv`

- [ ] **Step 1: Run all script tests**

Run: `bash scripts/safe_go_test_test.sh && bash scripts/vm_test_status_test.sh && bash scripts/spec_status_test.sh`

Expected: all three scripts print `PASS`.

- [ ] **Step 2: Run bounded VM diagnostic**

Run: `RGO_GO_TEST_TIMEOUT=15 RGO_TEST_MEMORY_KB=4194304 scripts/vm_test_status.sh reports/vm-test-status.csv`

Expected: CSV is written. Some rows may be `timeout`, `error`, or `oom_or_killed`; the process must not OOM the system.

- [ ] **Step 3: Inspect diagnostic results**

Run: `grep -E ',(timeout|error|oom_or_killed),' reports/vm-test-status.csv || true`

Expected: prints problematic rows or no output.

- [ ] **Step 4: Record any problematic VM tests**

If Step 3 prints rows, append this section to `TODO.md` under the Codex/Go test OOM entry:

```markdown
  - `scripts/vm_test_status.sh` bounded diagnostic found these non-pass VM tests:
    - `<test-name>`: `<status>` with `RGO_GO_TEST_TIMEOUT=15 RGO_TEST_MEMORY_KB=4194304`.
```

Replace `<test-name>` and `<status>` with the actual CSV values. If Step 3 prints no rows, do not modify `TODO.md`.

- [ ] **Step 5: Run safe broad Go tests**

Run: `RGO_GO_TEST_TIMEOUT=60 RGO_TEST_MEMORY_KB=4194304 scripts/safe_go_test.sh ./...`

Expected: PASS or bounded failure. It must not OOM the system.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
git add reports/vm-test-status.csv TODO.md
git commit -m "test: record vm diagnostic baseline"
```

Expected: commit succeeds. If `TODO.md` was unchanged, omit it from `git add`.

## Self-Review

- Spec coverage: safe wrapper covered by Task 1, gate integration by Task 2, VM diagnostic runner by Task 3, spec memory hardening by Task 4, diagnostic recording by Task 5.
- Placeholder scan: no `TBD` or unresolved implementation placeholders remain; angle-bracket placeholders appear only in a conditional TODO template that instructs replacement with actual values.
- Type/name consistency: script names match the design: `safe_go_test.sh`, `vm_test_status.sh`, `spec_status.sh`, and `array_spec_gate.sh`.
