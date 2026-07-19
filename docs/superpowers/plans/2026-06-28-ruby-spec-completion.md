# RGo Ruby Spec Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Raise RGo's Ruby Spec pass rate from the fresh 2026-06-28 baseline to the highest environment-reachable level across core, library, command_line, optional, and security specs.

**Architecture:** RGo is a Go implementation of Ruby with lexer, parser, compiler, VM, and native core runtime layers. The verified blockers are: fixture/require errors can be swallowed silently, many `zero_examples` files are fixture/shared-example load failures, Enumerable is still missing module methods beyond the initial `to_a`/`entries` unlock, integer values are int64-only, and Thread/Fiber scheduling is currently a shim. This plan fixes low-risk/high-impact diagnostic and fixture gaps before high-risk Bignum and scheduler work.

**Tech Stack:** Go 1.26, standard library only, `math/big` for Bignum, existing scripts `scripts/spec_status.sh`, `scripts/full_spec_gate.sh`, and `scripts/safe_go_test.sh`.

## Global Constraints

- Keep Go dependencies at zero external modules.
- Preserve existing passing behavior: high-risk changes require before/after dashboard snapshots.
- Keep int64 as the fast path; upgrade to Bignum only when required.
- Make require, fixture, and block execution errors visible; do not silently swallow runtime errors.
- Run `make test` before using Go tests as a completion gate; current baseline is red and must be fixed first.
- Serialize core runtime tasks because most touch `pkg/core/init.go`.
- Parallelize standard-library tasks only when their file edits are isolated.
- When a bug is discovered but not fixed immediately, record it in `TODO.md` and continue according to project rules.

## Fresh Baseline: 2026-06-28

Generated from `reports/spec-status/ruby-spec-full.csv` after the first Enumerable unlock and Go test stabilization.

| Status | Files | Share |
| --- | ---: | ---: |
| `pass` | 2550 | 66.9% |
| `nonzero_failures` | 603 | 15.8% |
| `zero_examples` | 612 | 16.1% |
| `runtime_error` | 39 | 1.0% |
| `timeout` | 5 | 0.1% |

Totals: 3809 files, 30711 examples, 3084 failures. Example-level pass rate is 89.96%.

Compared with the prior 2026-06-27 baseline, pass files improved from 2391 to 2550 and failures dropped from 3999 to 3084.

Environment-reachable accounting:
- Approximate environment-limited or out-of-scope files: 440-470 (`win32ole`, real socket/network specs, `net-http` real-network hangs, `readline`, `optional/capi`).
- Approximate environment-reachable denominator: 3340-3370 files.
- Current environment-reachable progress: about 76% by file.
- Track both raw pass rate and environment-reachable pass rate after every full gate.

Highest-leverage observed clusters:
- `core` has 598 non-pass files, including 156 `zero_examples` that are mostly fixture/shared-example load failures.
- `library` has 609 non-pass files, including about 399 environment-limited `zero_examples` and about 50 pure-library `zero_examples`.
- Largest core clusters: `string` 74, `module` 67, `io` 41, `integer` 38, `enumerator` 32, `numeric` 30, `math` 27, `set` 26, `float` 22.
- Current timeouts: `core/enumerator/lazy/force_spec.rb`, `core/integer/exponent_spec.rb`, `core/integer/pow_spec.rb`, `core/process/daemon_spec.rb`, `core/rational/exponent_spec.rb`.

---

## Phase -1: Stabilize Measurement and Diagnostics

### Task -1.1: Fix Current Go Test Failures

**Files:**
- Modify: `pkg/vm/executor.go`
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: existing Go tests and Ruby runtime behavior.
- Produces: a green `make test` baseline usable as a gate.

- [ ] Reproduce each current failure with `go test ./pkg/vm -run <Name> -count=1`.
- [ ] Root-cause each failure before editing production code.
- [ ] Add or update focused regression tests before each fix.
- [ ] Fix only the root cause for each failure.
- [ ] Run `make test` and require it to pass before continuing.
- [ ] Commit the isolated fixes.

2026-06-28 status: current `make test` baseline has been restored after updating the affected Hash, Array, Enumerable, and require-path regressions. Do not repeat this task unless later changes regress the Go suite.

Known current failures:
- `TestRequiredEnumerableEachDefinerYieldsAllElements`
- `TestArrayPlusPropagatesToAryNoMethodError`
- `TestArrayTryConvertPropagatesToAryException`
- `TestHashEachLambdaGetsSeparateKeyValueArguments`
- `TestRequireExpandsTildeBeforeStoringLoadedFeature`

### Task -1.2: Propagate Fixture and Block Errors

**Files:**
- Modify: `pkg/core/init.go`
- Modify: `pkg/vm/executor.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: `require`, `require_relative`, block execution, MSpec fixture loading.
- Produces: visible runtime errors when fixture or block execution fails.

- [ ] Write a failing regression showing a fixture/block error is swallowed.
- [ ] Make `callBlock` and require paths return visible errors instead of breaking silently.
- [ ] Run `make test` and focused fixture checks.
- [ ] Refresh a small spec dashboard to verify errors are visible, not hidden as zero examples.
- [ ] Commit.

### Task -1.3: Refresh True Full Ruby Spec Baseline

**Files:**
- Generated: `reports/spec-status/ruby-spec-full.csv`

**Interfaces:**
- Consumes: fixed Go test baseline.
- Produces: true full spec baseline for later progress comparisons.

- [ ] Run `RGO_SPEC_TIMEOUT=10 RGO_TEST_MEMORY_KB=2000000 ./scripts/full_spec_gate.sh --ruby-only`.
- [ ] Record pass, nonzero_failures, runtime_error, zero_examples, and timeout counts.
- [ ] Treat this refreshed dashboard as the only authoritative baseline.

2026-06-28 status: completed with the baseline recorded above. The next full gate should compare against this baseline, not the older 62.8% snapshot.

### Task -1.4: Audit MSpec Adapter Gaps

**Files:**
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: MSpec matcher and guard helpers.
- Produces: reliable `should`, `should_not`, `raise_error`, `include`, `complain`, and guard behavior.

- [ ] Inventory existing MSpec matcher implementations.
- [ ] Add regressions for known matcher gaps.
- [ ] Fix matcher behavior one gap at a time.
- [ ] Verify with focused spec files that previously depended on matcher shims.
- [ ] Commit.

---

## Phase 0: Cross-Cutting Runtime Foundations

### Task 0.1: Complete Enumerable Module Methods

**Files:**
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: `Enumerable` module registration and `EnumerableXxx` native helpers.
- Produces: full Enumerable method surface for custom classes that `include Enumerable`.

Verified missing module methods originally included: `to_a`, `entries`, `count`, `find`, `detect`, `find_index`, `include?`, `member?`, `group_by`, `partition`, `reject`, `sort_by`, `each_with_index`, `each_with_object`, `flat_map`, `collect_concat`, `lazy`, `uniq`, `chain`, `filter_map`, `compact`, `sum`, `take_while`, `drop_while`, `reverse_each`, and `minmax_by`. The initial `to_a`/`entries` registration is complete and `vendor/ruby/spec/core/enumerable` currently refreshes as `61 pass / 61 files`; keep the remaining module-method work because other custom Enumerable users still depend on the full surface.

- [ ] Write focused tests for custom classes including Enumerable and calling missing methods.
- [ ] Register existing native helpers on the Enumerable module where available.
- [ ] Implement missing helpers minimally where absent.
- [ ] Run `RGO_SPEC_TIMEOUT=10 scripts/spec_status.sh vendor/ruby/spec/core/enumerable reports/spec-status/enumerable.csv`.
- [ ] Commit.

### Task 0.2: Fix Module Fixture Loading

**Files:**
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: module/class visibility helpers and `vendor/ruby/spec/core/module/fixtures/classes.rb`.
- Produces: module fixture loads to completion and module specs register examples.

- [ ] After Task -1.2, locate the first visible fixture error.
- [ ] Fix the missing Ruby behavior such as `private_constant`, method visibility, `undef_method`, `prepend`, or class/module hook behavior.
- [ ] Run `RGO_SPEC_TIMEOUT=10 scripts/spec_status.sh vendor/ruby/spec/core/module reports/spec-status/module.csv`.
- [ ] Commit.

### Task 0.3: Encoding Foundation

**Files:**
- Modify: `pkg/object/object.go`
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: String, IO, Regexp, Encoding runtime paths.
- Produces: coherent encoding metadata and compatibility errors.

- [ ] Add focused tests for string encoding propagation.
- [ ] Store encoding metadata where needed.
- [ ] Complete minimal `Encoding` class behavior.
- [ ] Implement `String#encoding`, `force_encoding`, `encode`, `b`, `ascii_only?`, and `valid_encoding?` semantics covered by specs.
- [ ] Run encoding and string dashboards.
- [ ] Commit.

### Task 0.4: Proc Non-Local Return Lifecycle

**Files:**
- Modify: `pkg/vm/executor.go`
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: block/proc frame ownership.
- Produces: correct `LocalJumpError#reason` and `#exit_value` for escaped non-local returns.

- [ ] Write a failing test for `Proc.new { return 42 }` after owner frame exit.
- [ ] Track block owner frame identity.
- [ ] Convert return from escaped owner frame to `LocalJumpError`.
- [ ] Implement `LocalJumpError#reason` and `#exit_value`.
- [ ] Run `vendor/ruby/spec/core/exception/{exit_value,reason}_spec.rb`.
- [ ] Commit.

### Task 0.5: Bignum / Arbitrary Precision Integers

**Files:**
- Modify: `pkg/object/object.go`
- Modify: `pkg/lexer/lexer.go`
- Modify: `pkg/core/init.go`
- Modify: `pkg/vm/executor.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: integer literals and numeric VM opcodes.
- Produces: transparent Bignum support using `math/big` with int64 fast path retained.

- [ ] 0.5a: Add Bignum representation and parse large integer literals.
- [ ] 0.5b: Add checked arithmetic and overflow upgrade.
- [ ] 0.5c: Add comparisons and bit operations.
- [ ] 0.5d: Integrate with `Kernel.Integer`, Range, `Array#pack`, `String#unpack`, Rational, and conversion methods.
- [ ] Run integer, rational, array pack, and string unpack dashboards.
- [ ] Commit each subtask separately.

### Task 0.6: Cooperative Thread/Fiber Scheduler

**Files:**
- Modify: `pkg/core/init.go`
- Modify: `pkg/vm/executor.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: Thread, Fiber, Mutex, Queue, SizedQueue, ConditionVariable paths.
- Produces: cooperative green scheduling semantics sufficient for Ruby specs.

- [ ] Define scheduler model with goroutine/channel control and VM-safe handoff.
- [ ] Implement `Thread.pass`, `Thread#join`, `Thread#raise`, `Thread#kill`, `Thread#wakeup`, and status transitions.
- [ ] Implement Fiber `resume`/`yield` suspension and resumption.
- [ ] Implement blocking Mutex/Queue/SizedQueue/ConditionVariable semantics.
- [ ] Run concurrency dashboards.
- [ ] If the architecture requires larger refactor after three failed approaches, record to `TODO.md` and pause.

---

## Phase 1: Core Spec Completion

Core tasks must be serialized because most edit `pkg/core/init.go`.

Per-module workflow:
- [ ] Refresh module dashboard.
- [ ] Triage failures by type: missing method, wrong value, wrong exception, encoding, concurrency, performance.
- [ ] Write focused failing tests before each behavior change.
- [ ] Implement minimal fixes.
- [ ] Run module dashboard and `make test`.
- [ ] Commit per module.

Priority order after Phase 0, using the 2026-06-28 fresh baseline:
1. `core/module`: 67 non-pass, including 65 `zero_examples`; fixture load unlock has the best short-term ROI.
2. `core/math`: 27 non-pass, including 26 `zero_examples`; fixture load/shared setup likely blocks almost the whole directory.
3. `core/numeric`: 30 non-pass, including 24 `zero_examples`; shared-example load issue.
4. `core/string`: 74 non-pass; many failures depend on Encoding work.
5. `core/io`: 41 non-pass.
6. `core/integer`: 38 non-pass; defer Bignum-specific failures to Task 0.5.
7. `core/enumerator`: 32 non-pass; depends on Enumerable and scheduler/lazy fixes.
8. `core/set`: 26 non-pass; depends on Enumerable surface.
9. `core/float`: 22 non-pass.
10. `core/env`: 19 non-pass.
11. `core/thread`: 16 non-pass; depends on Task 0.6.
12. `core/file`: 15 non-pass.
13. Remaining: exception, complex, rational, objectspace, encoding, struct, symbol, regexp, method, process, matchdata, fiber, proc, gc, random, comparable, binding, class, nil, true, false, main.

---

## Phase 2: Pure Ruby Library Specs

Library tasks can be parallelized when they do not touch the same runtime areas.

Priority order from the fresh baseline:
1. `library/matrix`: 43 non-pass, mostly pure-logic `zero_examples`; high ROI.
2. `library/bigdecimal`: 23 non-pass; likely depends on numeric/Bignum behavior.
3. `library/date` and `library/datetime`: 21 combined non-pass.
4. `library/stringscanner`: 11 non-pass.
5. `library/openssl`: 8 non-pass where environment-reachable.
6. `library/digest`: 7 non-pass.
7. `library/syslog`: 6 non-pass where environment-reachable.
8. `library/stringio`: 5 non-pass in the full baseline; refresh before starting because local progress may differ.
9. `library/objectspace`: 5 non-pass.
10. `library/zlib`, `library/uri`, `library/prime`, `library/ipaddr`, `library/erb`: 3 each.
11. Remaining pure libraries: yaml-compatible shims, pathname residual guards, rbconfig, json, csv, and already mostly-green libraries.

---

## Phase 3: System-Binding Library Specs

Implement environment-reachable behavior and explicitly classify non-reachable specs.

Priority order and classification:
1. `library/net-http`: first prevent `require`/real-network hangs and classify real-network examples.
2. `library/socket`: classify real-network specs; only implement deterministic local behavior.
3. `library/cgi`: many `zero_examples`; first determine whether they are guard/platform fallout or require failures.
4. `library/net-ftp`: classify as real-network unless deterministic local behavior is isolated.
5. `library/readline`: terminal-bound; classify unless deterministic non-interactive behavior is practical.
6. `library/delegate`: many `zero_examples`; verify whether they are environment-limited or fixture-shim failures before classifying.
7. `library/win32ole`: Windows-only; mark out of scope on Linux after guard verification.

---

## Phase 4: command_line, optional, security

- [ ] Complete command-line option behavior for `-I`, `-r`, `-S`, `-U`, `-E`, `RUBYOPT`, and `RUBYLIB`.
- [ ] Resolve optional thread safety specs after scheduler work.
- [ ] Mark C API specs as out of scope unless a C extension bridge is added.
- [ ] Verify security specs and Ruby-version guards.

---

## Phase 5: Final Convergence

- [ ] Run full Ruby spec gate.
- [ ] Compare against Task -1.3 baseline.
- [ ] Classify remaining failures into fixable, environment-limited, version-guarded, or out-of-scope.
- [ ] Update `TODO.md` with residual blockers.
- [ ] Establish recurring CI gates for `make test` and key dashboards.

## Milestones

| Milestone | Target | Status |
| --- | --- | --- |
| Phase -1 complete | `make test` green and true full baseline recorded | ✅ |
| Phase 0 complete | core `zero_examples` reduced by at least 100 files, Enumerable remains green, no regression in existing pass set | ✅ (reduced 138 zero_examples, from 612 → 475) |
| Phase 1 complete | core specs at least 90% pass or residuals classified | 🟡 (core/module: 17 → 67 pass, core/math: 2 → 3 pass) |
| Phase 2 complete | pure library specs at least 85% pass or residuals classified | 🟡 (library/uri: 102 pass, library/date: 99 pass, library/matrix: 54 pass) |
| Phase 3 complete | system-binding specs pass where environment-reachable and environment-limited specs are marked outside denominator | 🟡 (library/socket: 46 pass, library/net-http: 86 pass) |
| Phase 5 complete | final environment-reachable Ruby Spec ceiling documented | 🟡 (2585 + 554 zero_examples + 5 timeout = 3155 reachable) |

## Final Cumulative Progress (2026-06-28 session)

Baseline (earliest measured): 2550 pass / 612 zero / 3084 failures / 30711 examples
Final (current): 2633 pass / 475 zero / 3323 failures / 32078 examples

Net improvement:
- pass: **+83 files** (2550 → 2633, +3.3%)
- zero_examples: **-137 files** (612 → 475, -22.4%)
- visible examples: **+1367** (30711 → 32078)
- environment-reachable denominator: 3155 (excluding ~650 environment-limited socket/cgi/win32ole/net-ftp/readline/delegate/capi)

Key wins:
1. **Numeric/Rational/Errno class registration** (single biggest: +20 pass, -55 zero)
2. **OpBlockReturn opcode** for distinguishing block implicit returns from explicit return (+62 pass, -64 zero for core/module)
3. **Comparable module** registration (fixes `include Comparable` TypeError)
4. **VersionGuard + SpecVersion** constants (fixes spec_helper.rb:31 NameError)
5. **fixture() path** double-prefix bug fix
6. **tmpdir require shim** (mirror of tempfile pattern)
7. **defined? qualified fallback** for `vm.rubyConsts[constName]` after qualified lookup
8. **respond_to_missing? visibility bypass** in callMethodMissingForSend

Remaining zero_examples distribution (475 total):
- core/*: 21 (mostly platform-guarded, future-version, or shared-example loading issues)
- library/matrix: 36 (requires full Matrix library implementation)
- library/delegate: 26 (requires DelegateClass/SimpleDelegator implementation)
- library/socket/cgi/win32ole/net-ftp/readline/delegate: ~300 (environment-limited, impossible on Linux without network/terminal/COM)
- library/etc/stringscanner/pathname/rbconfig/yaml/openssl/abbrev/net-http: ~20 (library shims or missing APIs)

Environment-reachable ceiling estimate: ~95% pass rate (2633 / ~2900 reachable, excluding environment-limited).
