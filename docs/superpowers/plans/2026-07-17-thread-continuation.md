# Ruby Thread Continuation Implementation Plan

> **For agentic workers:** Execute inline with TDD; project rules prohibit unrequested subagents and Git operations.

**Goal:** Implement resumable cooperative Ruby Thread execution without OS-thread concurrency or block replay.

**Architecture:** VM owns channel-gated Go coroutines plus opaque execution-context snapshots; core owns Ruby-visible thread state and scheduler policy. Suspension swaps to the caller context and parks the coroutine; resumption restores the thread context before unblocking it.

**Tech Stack:** Go VM/interpreter, RubySpec, Go testing.

## Global Constraints

- Never run Git commands unless the user explicitly requests one.
- Run CPU-heavy work serially with `GOMAXPROCS=1`, Go `-p=1`, `nice -n 10`, and timeouts.
- Preserve Ruby behavior; never re-run a Thread block to imitate resumption.

---

### Task 1: Continuation boundary

**Files:** `pkg/vm/executor.go`, `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [x] Add a failing test where code before `Thread.stop` executes once and code after it executes only after wakeup.
- [x] Add VM-owned continuation state and core suspend/resume hooks.
- [x] Detach frames, stack, rescue, ensure, catch and pending-control state at suspension.
- [x] Restore the snapshot and continue from the saved instruction.
- [x] Pass the focused Go test and `stop_spec.rb`/`wakeup_spec.rb`.

### Task 2: Scheduler and status

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [x] Add failing state-transition tests for `pass`, `run`, `wakeup`, `join`, and `status`.
- [x] Drive only runnable continuations and preserve sleeping continuations.
- [x] Pass `run_spec.rb`, `wakeup_spec.rb`, `status_spec.rb`, and the non-interrupt portions of `backtrace_spec.rb`.

### Task 3: Cancellation and injected exceptions

**Files:** `pkg/vm/executor.go`, `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [x] Add failing tests proving `kill/terminate/raise` resume at a safe point and execute `ensure` exactly once.
- [x] Inject pending cancellation/exception through the existing VM exception-unwind path.
- [x] Pass `kill_spec.rb`, `terminate_spec.rb`, and `raise_spec.rb`.

### Task 4: Shared blocking operations and regression gate

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`, `TODO.md`

- [ ] Route sleep, mutex, condition-variable, queue and flock blocking markers through the same continuation path. Sleep, mutex and flock are complete; condition-variable and queue remain follow-up work.
- [x] Run focused Go tests, all remaining Thread specs, then the Thread directory dashboard serially.
- [x] Record only genuinely distinct remaining failures in `TODO.md`; completion requires all Thread specs green without replaying blocks.
