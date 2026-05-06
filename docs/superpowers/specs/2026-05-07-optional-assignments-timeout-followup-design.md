# Optional Assignments Timeout Follow-up Design

## Goal

Continue reducing the `vendor/ruby/spec/language` timeout set by focusing on the remaining `optional_assignments_spec.rb` timeout. The previous round fixed parser hangs for `super()` and parenthesized `super(args)`, but the full selected spec still times out.

## Scope

This milestone targets only `vendor/ruby/spec/language/optional_assignments_spec.rb`. It should not attempt broad keyword argument, rescue, or full `super` runtime semantics unless a focused reproducer proves one of those is the immediate blocker for this file.

The current language dashboard should be refreshed and kept consistent with `TODO.md` before implementation work begins. Existing bounded test wrappers must be used for broad or potentially hanging commands.

## Approach

Start from the known blocker in `TODO.md`: `optional_assignments_spec.rb` still times out after the `super()` and `super(args)` parser fixes. Re-run the selected file with a bounded command, then isolate the next smallest hanging construct from the spec.

The investigation should prioritize the areas most likely to contain the next blocker:

- `Class.new(Array)` examples that use `super(...)` in `[]` or `[]=` methods.
- Accessor optional assignment forms such as `@a.b ||= value` and `@a.b &&= value`.
- Index optional assignment forms such as `obj[key] ||= value` and splatted index arguments.
- Inline rescue around optional assignment only if the above areas do not reproduce the hang.

Each isolated bug should follow test-first implementation: add one focused Go regression test, verify it fails for the expected reason, apply the smallest parser/compiler/VM/core fix, then rerun the focused test and the bounded selected spec status command.

## Boundaries

Stop after one focused fix attempt if `optional_assignments_spec.rb` still times out. Record the newly narrowed blocker in `TODO.md` with the exact command and reproducer instead of continuing into a broad debugging session.

Do not run unbounded `go test ./...` or full unbounded spec commands. Use `scripts/safe_go_test.sh`, `scripts/spec_status.sh`, and explicit shell `timeout` where needed.

Do not refactor parser/compiler/VM structure unless required by the minimal reproducer. Keep changes local to the identified failure layer.

## Components

Likely edit points are:

- `reports/spec-status/language.csv`: refreshed dashboard rows.
- `TODO.md`: selected blocker and language dashboard summary.
- `pkg/parser/parser.go` and `pkg/parser/parser_test.go`: only if another syntax/token-consumption hang is proven.
- `pkg/compiler/compiler.go` and `pkg/compiler/compiler_test.go`: only if bytecode lowering creates a loop or bad instruction sequence.
- `pkg/vm` tests and implementation: only if parsing/compilation terminates and runtime execution hangs or returns the wrong result.
- `pkg/core`: only if a directly exercised core method behavior is missing.

## Testing

The minimum verification set is:

- One focused RED/GREEN Go regression test for the new reproducer.
- The relevant package test through `scripts/safe_go_test.sh`.
- `RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv`.
- A refreshed `RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv` when updating dashboard documentation.

Completion means the selected spec advances from timeout to pass or a more precise failure class, or a concrete next blocker is documented after one focused fix attempt.

## Risks

The remaining timeout may be caused by runtime semantics rather than another parser hang. Optional assignment combines receiver evaluation, setter return values, index assignment, `super`, and inline rescue, so over-broad fixes could regress already passing language specs. The plan reduces this risk by isolating one snippet, adding a focused regression, and stopping after one fix attempt if the full spec still times out.
