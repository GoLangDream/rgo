# RGo Language Gate Completion Design

## Goal

Make `vendor/ruby/spec/language` pass as the first concrete milestone toward a more complete Ruby implementation.

This milestone is intentionally limited to the Ruby language spec directory. It does not attempt to make all of `vendor/ruby/spec` pass in one step.

## Current Context

RGo already has several cleared compatibility gates, including core array, string, integer, kernel, and time areas documented in `TODO.md` and `reports/spec-status`.

The language gate remains a foundation risk because parser, compiler, VM, and control-flow semantics affect every later core and stdlib gate. Clearing language first should reduce repeated fixes in downstream specs.

## Scope

The milestone covers:

- Refreshing `reports/spec-status/language.csv`.
- Prioritizing failures in this order: timeout, parse error, compile error, runtime error, nonzero failures.
- Fixing one target spec file at a time.
- Adding focused Go regression tests or focused ruby/spec reproductions before behavior changes.
- Preserving already-cleared gates where practical.
- Recording unrelated discovered blockers in `TODO.md` instead of expanding the current task.

The milestone excludes:

- Clearing every `vendor/ruby/spec/core`, `library`, `optional`, or Rails compatibility dashboard.
- Large runtime rewrites unless they are required by a language spec blocker.
- Replacing the current spec-status scripts.
- Perfect CRuby parity outside behavior exercised by the language specs.

## Workflow

1. Refresh the language dashboard with:

   ```bash
   RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
   ```

2. Select the highest-priority failing file from the refreshed dashboard.

3. Reproduce the failure in the smallest useful form:

   - Add or update a focused Go test when the failure maps cleanly to lexer, parser, compiler, VM, or core behavior.
   - Use the focused spec command when the behavior is better verified through ruby/spec.

4. Implement the smallest compatible fix in the responsible layer.

5. Verify the focused target passes.

6. Refresh `language.csv` after each meaningful fix or group of tightly related fixes.

7. Repeat until all `vendor/ruby/spec/language` files pass.

## Architecture Notes

Language failures should be assigned to the earliest responsible layer:

- Lexer fixes for tokenization and literal boundaries.
- Parser fixes for AST structure and Ruby syntax forms.
- Compiler fixes for bytecode generation, scope management, and jump layout.
- VM fixes for stack discipline, control flow, method dispatch, block execution, exception flow, and frame state.
- Core fixes only when the language spec depends on a minimal runtime object or method.

When a failure crosses layers, the implementation should still keep the public behavior tested at the outermost layer and add lower-level regression tests only where they clarify the defect.

## Error Handling

Timeouts are treated as correctness failures, not just performance problems. The first response to a timeout is to identify the loop or scheduling condition that prevents progress.

Unexpected side issues discovered during investigation should be added to `TODO.md` if they are not required for the selected language target. This follows the project rule to avoid losing progress to broad incidental debugging.

## Verification

Completion requires:

```bash
go test ./...
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

The final `language.csv` must show no timeout, parse error, compile error, runtime error, or nonzero failure files.

Before declaring the milestone complete, run at least targeted checks for previously cleared gates touched by the changes. If the edits touch shared parser, compiler, VM, or core behavior, run the relevant full dashboards where practical.

## Next Milestone

After this milestone passes, choose the next gate from refreshed dashboards. Likely candidates are remaining concurrency/thread-related specs, core directories with high failure density, or Rails ActiveSupport compatibility slices.
