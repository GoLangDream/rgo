# Language Timeout Reduction Design

## Goal

Reduce the remaining timeout set in `vendor/ruby/spec/language` without broad VM rewrites or unsafe test runs. The current working baseline is 74 passing files, 6 timeouts, no parse errors, no compile errors, no runtime errors, and no nonzero failures out of 80 language spec files.

## Scope

Target only the current timeout files:

- `vendor/ruby/spec/language/keyword_arguments_spec.rb`
- `vendor/ruby/spec/language/method_spec.rb`
- `vendor/ruby/spec/language/optional_assignments_spec.rb`
- `vendor/ruby/spec/language/predefined_spec.rb`
- `vendor/ruby/spec/language/rescue_spec.rb`
- `vendor/ruby/spec/language/super_spec.rb`

The first implementation round should clear at least one timeout file if a minimal fix is available. If the first investigated file exposes a large semantic gap or runaway execution, record the blocker in `TODO.md` and move to the next smallest target.

## Approach

Use the existing spec dashboard as the source of truth, then inspect individual timeout files with bounded commands. Prefer small, isolated fixes that move one spec file from timeout to pass or to a more precise failure class.

The recommended order is:

1. Refresh or inspect `reports/spec-status/language.csv` to confirm the active timeout set.
2. Run individual timeout files with `RGO_SPEC_TIMEOUT=1` or the existing safe scripts so failures remain bounded.
3. Pick the smallest reproducible case from the file output or by adding a focused internal Go test.
4. Fix only the required parser/compiler/VM/core behavior for that reproducer.
5. Re-run the focused test and the affected spec file.
6. Update `TODO.md` and `reports/spec-status/language.csv` with the new state.

## Boundaries

Do not attempt a general Ruby control-flow rewrite in this milestone. `redo`, `retry`, rescue unwinding, keyword argument fidelity, and `super` forwarding may each require separate designs if the bounded investigation shows they are the real blocker.

Do not run broad unbounded `go test ./...` or full spec commands. Use safe wrappers, package-level commands, or per-file spec runs with explicit timeouts.

Follow the project rule for newly discovered bugs: test enough to identify what works and what does not, record large or unrelated blockers in `TODO.md`, then continue with another viable target instead of getting stuck.

## Components

The likely edit points are:

- `pkg/parser`: only if a timeout hides parser recovery or ambiguous syntax consumption.
- `pkg/compiler`: for jump patching, block/lambda control-flow compilation, keyword/rest argument lowering, or `super` opcode emission.
- `pkg/vm`: for bounded execution behavior, method/block invocation semantics, rescue unwinding, or `super` dispatch.
- `pkg/core`: only for missing core helpers directly exercised by the selected language spec.
- `TODO.md` and `reports/spec-status/language.csv`: to keep the dashboard and known-blocker list accurate.

## Testing

Each change needs one focused regression test when practical, usually in `pkg/parser`, `pkg/compiler`, or `pkg/vm`, plus a bounded single-file spec run for the affected language spec.

Completion for this milestone requires evidence that at least one target file is no longer a timeout, or that a blocker was recorded and another target was advanced. Broad language dashboard refresh is preferred when safe, but the single-file spec result is sufficient for a small implementation round.

## Risks

Timeouts may be caused by deep runtime semantics rather than parser/compiler gaps. The main risk is overfitting a fix that passes one spec while breaking block, method, or rescue control flow elsewhere. Focused Go regression tests and bounded spec re-runs should catch the most likely regressions without triggering the known OOM failure mode.
