# Language Foundation Gate Design

## Goal

Advance RGo's Ruby language compatibility by reducing the remaining failures in `reports/spec-status/language-current.csv`, starting with the foundational block/yield/proc/lambda/break/rescue/method/variable semantics that affect many later core and Rails specs.

## Scope

This slice targets the Ruby language spec gate, not Rails execution or broad core-library completion. The first development pass focuses on one failing spec file at a time and uses focused Go regression tests before production changes.

Initial priority:

1. `vendor/ruby/spec/language/yield_spec.rb`
2. `vendor/ruby/spec/language/block_spec.rb`
3. `vendor/ruby/spec/language/proc_spec.rb`
4. `vendor/ruby/spec/language/lambda_spec.rb`
5. `vendor/ruby/spec/language/break_spec.rb`
6. `vendor/ruby/spec/language/rescue_spec.rb`
7. `vendor/ruby/spec/language/method_spec.rb` and `def_spec.rb`
8. `vendor/ruby/spec/language/variables_spec.rb`

## Approach

Each spec file is treated as a gate:

1. Run the spec to capture the current failure.
2. Reduce the failure to a focused Go test in the narrowest package that owns the behavior.
3. Implement the smallest parser/compiler/VM/core change needed to pass that test.
4. Re-run the focused Go test, the target spec, and the full Go test suite.
5. Record blockers in `TODO.md` if a failure expands beyond the current slice.

## Architecture Notes

Language semantics are split across:

- `pkg/parser/parser.go` and `pkg/parser/ast/node.go` for syntax and AST shape.
- `pkg/compiler/compiler.go` and `pkg/compiler/opcode.go` for bytecode generation.
- `pkg/vm/executor.go` for runtime control flow, block invocation, method frames, and exception handling.
- `pkg/core/init.go` for Ruby core object behavior used by specs.

Changes should preserve the existing large-file structure for this slice. Refactoring is allowed only when it directly supports the current failing behavior and is covered by tests.

## Testing

Primary verification:

- Focused Go regression tests, usually under `pkg/vm/executor_test.go`, `pkg/parser/parser_test.go`, or `pkg/compiler/compiler_test.go`.
- Focused Ruby spec execution through `scripts/spec_status.sh`.
- Full internal verification with `scripts/safe_go_test.sh ./...`.

The current baseline already has passing Go tests. Any new production behavior must be preceded by a failing Go test.

## Out Of Scope

- Rails bundle installation and Rails test execution.
- Full Ruby spec completion.
- Large-scale extraction of `pkg/core/init.go` or `pkg/vm/executor.go`.
- Cleaning unrelated temporary files unless they block verification.
