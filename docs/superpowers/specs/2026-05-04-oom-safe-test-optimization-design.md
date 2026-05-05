# OOM-Safe Test Optimization Design

## Goal

Prevent Codex or manual test runs from exhausting system memory while preserving a path to identify the VM or spec case that causes unbounded execution or allocation.

The immediate failure mode was a system OOM event where Go's `pkg/vm` test binary (`vm.test`) consumed tens of GB of memory and caused the surrounding tmux scope to be killed. The fix should therefore add guardrails before attempting deeper VM behavior changes.

## Non-Goals

- Do not change VM execution semantics in the first step.
- Do not mask known Ruby spec timeouts as passing.
- Do not make normal local test runs depend on external services or privileged system configuration.

## Approach

Use a two-layer strategy:

1. Add safe test wrappers that bound wall-clock time, Go package parallelism, and optional process memory.
2. Add targeted diagnostics that run VM tests and Ruby specs one at a time, producing machine-readable status so the next debugging pass can focus on a single reproducer.

This keeps the system stable first, then narrows root-cause investigation without relying on broad `go test ./...` runs.

## Components

### Safe Go Test Wrapper

Add `scripts/safe_go_test.sh` as the preferred entry point for broad Go tests.

Default behavior:

- `GOMAXPROCS=1` unless already set.
- `go test -p 1` unless caller explicitly passes another package parallelism option.
- A wall-clock timeout, controlled by `RGO_GO_TEST_TIMEOUT`, with a conservative default.
- Optional virtual-memory cap via `RGO_TEST_MEMORY_KB`; if unset, do not apply `ulimit`.

The wrapper should pass through all user arguments to `go test`, so existing commands can migrate with minimal friction.

### Gate Script Integration

Update `scripts/array_spec_gate.sh` to use the safe Go test wrapper instead of bare `go test ./...`.

Keep the existing spec gate behavior intact:

- `scripts/feature_test.sh` still runs after Go tests.
- `scripts/spec_status.sh` still runs one Ruby spec file at a time.
- Non-pass spec files still fail the gate.

### VM Test Diagnostic Runner

Add `scripts/vm_test_status.sh` to enumerate `pkg/vm` Go tests and run each one individually.

For each test, record:

- test name
- status: `pass`, `timeout`, `error`, or `oom_or_killed`
- duration in milliseconds

The output should be CSV so it can be sorted, checked into reports if useful, or compared across runs. Each individual test run should use the safe wrapper with strict timeout and optional memory cap.

### Spec Status Hardening

Keep `scripts/spec_status.sh` as the Ruby spec dashboard runner. If needed during implementation, thread the same optional memory cap into each `rgo test` subprocess. Do not change the CSV schema unless necessary.

## Data Flow

Broad validation flow:

1. Developer or Codex runs `scripts/array_spec_gate.sh`.
2. The gate runs `scripts/safe_go_test.sh ./...` instead of `go test ./...`.
3. The gate runs feature tests and Ruby spec status as before.
4. Any timeout or non-pass status is reported without exhausting system memory.

Diagnostic flow:

1. Developer runs `scripts/vm_test_status.sh reports/vm-test-status.csv`.
2. The script lists `pkg/vm` tests.
3. Each test runs in isolation with bounded time and optional memory.
4. The CSV identifies the first failing, timing out, or killed test for deeper debugging.

## Error Handling

- Timeout exit code should be classified distinctly from ordinary test failure.
- Signal-based termination such as `137` should be classified as `oom_or_killed`.
- Temporary output files should be removed after each run.
- The scripts should fail fast on invalid arguments, but diagnostic scripts should continue after individual test failures.

## Testing

Add shell-level checks where practical:

- Verify `safe_go_test.sh` passes through arguments and runs a small package.
- Verify `vm_test_status.sh` produces a CSV header and at least one row for `pkg/vm`.
- Re-run the existing `scripts/spec_status_test.sh`.
- Run the updated gate with a short timeout only if it is safe under the new wrapper.

## Rollout

Use the safe wrapper for all broad test commands during Codex sessions. If the VM diagnostic runner identifies a specific reproducer, record it in `TODO.md` and debug that case separately under the project's existing rule for newly found bugs.
