# RGo 100% Ruby Spec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring RGo from the current partial Ruby core/spec compatibility to a repeatable 100% pass target for the selected `vendor/ruby/spec` scope.

**Architecture:** Stop treating individual core methods as the main blocker; the current dominant blocker is parser/compiler/MSpec infrastructure hanging or failing before examples run. Build a spec harness that can classify parse/compile/runtime/spec failures, then clear language/MSpec blockers before broadening core library coverage.

**Tech Stack:** Go, RGo lexer/parser/compiler/VM, `cmd/rgo` CLI, `vendor/ruby/spec`, Go unit tests, `scripts/feature_test.sh`.

---

### Task 1: Establish A Spec Progress Dashboard

**Files:**
- Create: `scripts/spec_status.sh`
- Create: `reports/spec-status/README.md`
- Modify: `TODO.md`

- [ ] Create `scripts/spec_status.sh` that runs a spec directory with per-file timeout and emits CSV columns: `file,status,examples,failures,error_kind,duration_ms`.
- [ ] Use statuses: `pass`, `parse_error`, `compile_error`, `runtime_error`, `timeout`, `nonzero_failures`.
- [ ] Run: `scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv`.
- [ ] Record totals in `reports/spec-status/README.md`.
- [ ] Run: `go test ./...` and `scripts/feature_test.sh`.

### Task 2: Fix Parser/Compiler Timeout Class Before Adding More Core Methods

**Files:**
- Modify: `pkg/parser/parser.go`
- Modify: `pkg/parser/parser_test.go`
- Modify: `pkg/compiler/compiler.go`
- Modify: `pkg/vm/executor_test.go`

- [ ] Pick the first timeout from `reports/spec-status/array.csv`.
- [ ] Minimize it to a single Ruby snippet that hangs before first `describe` output.
- [ ] Add a parser or VM regression test for that exact snippet.
- [ ] Run the new test and confirm it fails or times out for the expected reason.
- [ ] Fix only that parser/compiler bug.
- [ ] Re-run the targeted spec file and the full Go suite.
- [ ] Repeat until timeout count for `core/array` is zero.

### Task 3: Complete MSpec DSL Compatibility Needed By Core Specs

**Files:**
- Modify: `pkg/core/init.go`
- Modify: `pkg/vm/executor.go`
- Modify: `cmd/rgo/main.go`
- Modify: `pkg/vm/executor_test.go`

- [ ] Implement missing hooks used in specs: `before`, `after`, `ScratchPad`, `mock`, `should_receive`, `should_not_receive`, `complain`, `raise_error` message/class matching.
- [ ] Add VM tests for each DSL primitive before implementation.
- [ ] Ensure `require_relative '../../spec_helper'` and `fixtures/classes` can be selectively loaded or safely stubbed.
- [ ] Re-run `scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv` after each DSL primitive.

### Task 4: Finish Array Core Semantics Against Full Specs

**Files:**
- Modify: `pkg/core/init.go`
- Modify: `pkg/vm/executor_test.go`

- [ ] For every `core/array/*_spec.rb` with `runtime_error` or `nonzero_failures`, add one minimized VM test for the first failing Ruby behavior.
- [ ] Fix the corresponding `array*` method in `pkg/core/init.go`.
- [ ] Re-run the individual spec file.
- [ ] Continue until all 102 `core/array/*_spec.rb` files are `pass`.

### Task 5: Generalize Enumerable Instead Of Duplicating Collection Logic

**Files:**
- Modify: `pkg/core/init.go`
- Consider creating: `pkg/core/enumerable.go`
- Modify: `pkg/vm/executor_test.go`

- [ ] Extract shared enumerable behavior used by Array and Hash: `map`, `select`, `reject`, `find`, `reduce`, `any?`, `all?`, `none?`, `one?`, `sort_by`, `min_by`, `max_by`.
- [ ] Preserve current Array tests while adding Hash tests for shared behavior.
- [ ] Re-run `go test ./...` and representative Array specs.

### Task 6: Expand Scope From Array To Core Classes

**Files:**
- Modify: `TODO.md`
- Modify: `reports/spec-status/README.md`
- Modify core files under `pkg/core/`

- [ ] Run status dashboard for `vendor/ruby/spec/core/string`.
- [ ] Clear parser/MSpec blockers first.
- [ ] Then complete String methods until string specs pass.
- [ ] Repeat for Hash, Integer, Float, Symbol, Range, Proc, Exception, Class, Module, Object.

### Task 7: Define The Final 100% Gate

**Files:**
- Create: `scripts/full_spec_gate.sh`
- Modify: `README.md` or project docs if needed

- [ ] Add a single command that runs: `go test ./...`, `scripts/feature_test.sh`, and all selected `vendor/ruby/spec` files with timeout classification.
- [ ] The command exits non-zero if any spec is not `pass`.
- [ ] Run this command before claiming 100%.

---

## Current Baseline

- Go suite: `go test ./...` passes.
- Feature smoke: `scripts/feature_test.sh` reports `170 passed, 0 failed out of 170 tests`.
- Array method bindings: 93 methods currently registered in `pkg/core/init.go`.
- Array spec files: 102 files under `vendor/ruby/spec/core/array`.
- Freshly verified passing Array spec files include: `assoc_spec.rb`, `deconstruct_spec.rb`, `fetch_spec.rb`, `at_spec.rb`, `append_spec.rb`, `to_a_spec.rb`, `empty_spec.rb`, `length_spec.rb`.

## Completion Definition

100% means the chosen spec scope has zero `timeout`, zero parse/compile/runtime errors, and zero failing examples under the automated gate. Internal Go tests and feature smoke must also pass.
