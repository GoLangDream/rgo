# RGo Language Gate Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `vendor/ruby/spec/language` pass as the first concrete milestone toward a more complete Ruby implementation.

**Architecture:** Use the existing spec dashboard as the control loop. Fix failures at the earliest responsible layer: lexer, parser, compiler, VM, then core runtime only when the language spec depends on runtime behavior. Keep each change tied to a focused regression and refresh `reports/spec-status/language.csv` after meaningful progress.

**Tech Stack:** Go, RGo lexer/parser/compiler/VM/core packages, vendored ruby/spec, existing `scripts/spec_status.sh` dashboard.

---

## Current Baseline

Generated on 2026-05-17 with:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Current status:

- pass: 25 files
- parse_error: 3 files
- runtime_error: 1 file
- nonzero_failures: 51 files
- timeout: 0 files
- compile_error: 0 files

Current parse/runtime blockers:

- `vendor/ruby/spec/language/precedence_spec.rb`: `Parse Error: line 456:45: no prefix parse function for . found`
- `vendor/ruby/spec/language/send_spec.rb`: bare method call arguments in `(specs.fooM3 1,2,3).should == [1,2,3]`
- `vendor/ruby/spec/language/variables_spec.rb`: multiple assignment and splat assignment parse errors
- `vendor/ruby/spec/language/or_spec.rb`: VM panic in `eval "break true or false"` matcher path

## File Map

- Modify `pkg/parser/parser_test.go`: focused parser regressions for parse-error files.
- Modify `pkg/parser/parser.go`: parser fixes for precedence modifiers, bare method-call arguments, multiple assignment, and splat assignment.
- Modify `pkg/compiler/compiler_test.go`: compile regressions only when a parser fix produces AST that needs compiler support.
- Modify `pkg/compiler/compiler.go`: compile fixes for new or corrected AST forms.
- Modify `pkg/vm/executor_test.go`: VM regressions for runtime panics and language behavior.
- Modify `pkg/vm/executor.go`: stack-safety and control-flow fixes.
- Modify `pkg/core/init.go`: only for minimal runtime methods or mspec matcher behavior needed by language specs.
- Modify `TODO.md`: record unrelated blockers discovered while pursuing one selected target.
- Modify `reports/spec-status/language.csv`: refreshed dashboard after target fixes.

## Task 1: Lock The Baseline

**Files:**
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Refresh the dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected:

```text
Wrote reports/spec-status/language.csv (80 specs)
```

- [ ] **Step 2: Confirm failure categories**

Run:

```bash
cut -d, -f2 reports/spec-status/language.csv | sort | uniq -c
```

Expected before implementation starts:

```text
     51 nonzero_failures
      3 parse_error
     25 pass
      1 runtime_error
      1 status
```

- [ ] **Step 3: Commit the refreshed baseline if changed**

Run:

```bash
git add reports/spec-status/language.csv
git commit -m "test: refresh language spec dashboard baseline"
```

Expected: a commit if the dashboard changed; if git reports nothing to commit, continue without a commit.

## Task 2: Clear `precedence_spec.rb` Parse Error

**Files:**
- Modify: `pkg/parser/parser_test.go`
- Modify: `pkg/parser/parser.go`
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Add focused parser regressions**

Append these tests to `pkg/parser/parser_test.go`:

```go
func TestParseAndOrBeforeModifierConditions(t *testing.T) {
	parse(t, `(1 if 2 and 3).should == 1`)
	parse(t, `(1 if 2 or 3).should == 1`)
	parse(t, `(1 unless false and true).should == 1`)
	parse(t, `(1 unless false or false).should == 1`)
}

func TestParseLoopModifiersWithAndOrConditions(t *testing.T) {
	parse(t, `(1 while true and false).should == nil`)
	parse(t, `(1 while false or false).should == nil`)
	parse(t, `((raise until true and false) rescue 10).should == 10`)
	parse(t, `(1 until false or true).should == nil`)
}
```

- [ ] **Step 2: Run the focused parser tests and verify they fail**

Run:

```bash
go test ./pkg/parser -run 'TestParseAndOrBeforeModifierConditions|TestParseLoopModifiersWithAndOrConditions' -count=1
```

Expected: FAIL with a parser error matching `no prefix parse function for . found` or an incorrect top-level statement count.

- [ ] **Step 3: Fix modifier parsing**

In `pkg/parser/parser.go`, update modifier parsing so postfix `if`, `unless`, `while`, and `until` consume the full low-precedence condition containing `and` or `or`, then stop before the outer chained call. The target functions are:

- `parseIfModifier`
- `parseUnlessModifier`
- `parseWhileModifier`
- `parseUntilModifier`

The condition parser must accept `and` and `or` as part of the modifier condition for inputs like:

```ruby
(1 if 2 and 3).should == 1
(1 while false or false).should == nil
```

It must still leave the closing `)` available for the grouped expression and the following `.should` chain.

- [ ] **Step 4: Verify focused parser tests pass**

Run:

```bash
go test ./pkg/parser -run 'TestParseAndOrBeforeModifierConditions|TestParseLoopModifiersWithAndOrConditions' -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify the target spec passes parsing**

Run:

```bash
./rgo test vendor/ruby/spec/language/precedence_spec.rb
```

Expected: no `Parse Error:`. The file may still report nonzero failures; that is acceptable for this task.

- [ ] **Step 6: Refresh dashboard and commit**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
git add pkg/parser/parser_test.go pkg/parser/parser.go reports/spec-status/language.csv
git commit -m "fix: parse language precedence modifier conditions"
```

Expected: `precedence_spec.rb` is no longer `parse_error`.

## Task 3: Clear `send_spec.rb` Bare Argument Parse Error

**Files:**
- Modify: `pkg/parser/parser_test.go`
- Modify: `pkg/parser/parser.go`
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Add focused parser regressions**

Append these tests to `pkg/parser/parser_test.go`:

```go
func TestParseParenthesizedDottedCallWithBareArguments(t *testing.T) {
	parse(t, `(specs.fooM3 1,2,3).should == [1,2,3]`)
}

func TestParseDottedCallWithBareSplatArgument(t *testing.T) {
	parse(t, `a = [1,2,3]
specs.fooM3(*a).should == [1,2,3]`)
}
```

- [ ] **Step 2: Run the focused parser tests and verify they fail**

Run:

```bash
go test ./pkg/parser -run 'TestParseParenthesizedDottedCallWithBareArguments|TestParseDottedCallWithBareSplatArgument' -count=1
```

Expected: FAIL for the parenthesized bare-argument call if the current parser has not been fixed.

- [ ] **Step 3: Fix dotted method-call argument parsing**

In `pkg/parser/parser.go`, update `parseMethodCall`, `parseOneCallArg`, and `parseOneCallArgStoppingAtRParen` so a dotted call can parse Ruby-style bare arguments without parentheses when the receiver expression is grouped:

```ruby
(specs.fooM3 1,2,3).should == [1,2,3]
```

The parser must stop argument collection at `)` so the outer `.should` chain remains outside the `fooM3` call.

- [ ] **Step 4: Verify focused parser tests pass**

Run:

```bash
go test ./pkg/parser -run 'TestParseParenthesizedDottedCallWithBareArguments|TestParseDottedCallWithBareSplatArgument' -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify target spec no longer parse-errors**

Run:

```bash
./rgo test vendor/ruby/spec/language/send_spec.rb
```

Expected: no `Parse Error:`.

- [ ] **Step 6: Refresh dashboard and commit**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
git add pkg/parser/parser_test.go pkg/parser/parser.go reports/spec-status/language.csv
git commit -m "fix: parse bare arguments for dotted sends"
```

Expected: `send_spec.rb` is no longer `parse_error`.

## Task 4: Clear `variables_spec.rb` Parse Error

**Files:**
- Modify: `pkg/parser/parser_test.go`
- Modify: `pkg/parser/parser.go`
- Modify: `pkg/compiler/compiler_test.go` if compile support is needed
- Modify: `pkg/compiler/compiler.go` if compile support is needed
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Add focused parser regressions**

Append these tests to `pkg/parser/parser_test.go`:

```go
func TestParseGroupedMultipleAssignmentReturnsRHS(t *testing.T) {
	parse(t, `ary = [1, 2]
x = (a, b = ary)
x.should equal(ary)`)
}

func TestParseGroupedSplatAssignment(t *testing.T) {
	parse(t, `ary = [1, 2]
(a = *ary).should == [1, 2]
a.should_not equal(ary)`)
}

func TestParseNestedMultipleAssignmentWithSplatRHS(t *testing.T) {
	parse(t, `x = mock("multi-assign")
(a, *b, (c, d) = 1, 2, 3, *x).should == [1, 2, 3, x]`)
}
```

- [ ] **Step 2: Run the focused parser tests and verify they fail**

Run:

```bash
go test ./pkg/parser -run 'TestParseGroupedMultipleAssignmentReturnsRHS|TestParseGroupedSplatAssignment|TestParseNestedMultipleAssignmentWithSplatRHS' -count=1
```

Expected: FAIL with parse errors involving `,`, `=`, or missing method name.

- [ ] **Step 3: Fix multiple-assignment parsing**

In `pkg/parser/parser.go`, update assignment parsing around these functions:

- `parseAssignExpression`
- `parseAssignmentValue`
- `parseSplatExpression`

The parser must accept grouped multiple assignment and grouped splat assignment forms:

```ruby
x = (a, b = ary)
(a = *ary).should == [1, 2]
(a, *b, (c, d) = 1, 2, 3, *x).should == [1, 2, 3, x]
```

If the AST already has a multiple-assignment node, reuse it. If it does not, add the smallest AST representation needed in `pkg/parser/ast/node.go` and keep compiler support in the same task.

- [ ] **Step 4: Add compile regressions only if a new AST form was introduced**

If Step 3 adds or changes an AST node, append this test to `pkg/compiler/compiler_test.go`:

```go
func TestCompileGroupedMultipleAssignmentForms(t *testing.T) {
	compile(t, `ary = [1, 2]
x = (a, b = ary)
(a = *ary)
x`)
}
```

Run:

```bash
go test ./pkg/compiler -run TestCompileGroupedMultipleAssignmentForms -count=1
```

Expected before compiler support: FAIL with a compile error for the new AST form. If no AST form changed, skip this compile test.

- [ ] **Step 5: Implement compiler support if needed**

In `pkg/compiler/compiler.go`, compile the grouped multiple-assignment form so it preserves Ruby's RHS return value where required and assigns local variables in left-to-right order.

- [ ] **Step 6: Verify focused parser and compiler tests pass**

Run:

```bash
go test ./pkg/parser -run 'TestParseGroupedMultipleAssignmentReturnsRHS|TestParseGroupedSplatAssignment|TestParseNestedMultipleAssignmentWithSplatRHS' -count=1
go test ./pkg/compiler -run TestCompileGroupedMultipleAssignmentForms -count=1
```

Expected: parser tests PASS. Compiler test PASS if it was added; if it was skipped, the compiler command may report no tests to run.

- [ ] **Step 7: Verify target spec no longer parse-errors**

Run:

```bash
./rgo test vendor/ruby/spec/language/variables_spec.rb
```

Expected: no `Parse Error:`.

- [ ] **Step 8: Refresh dashboard and commit**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
git add pkg/parser/parser_test.go pkg/parser/parser.go pkg/parser/ast/node.go pkg/compiler/compiler_test.go pkg/compiler/compiler.go reports/spec-status/language.csv
git commit -m "fix: parse language multiple assignment forms"
```

Expected: `variables_spec.rb` is no longer `parse_error`.

## Task 5: Fix `or_spec.rb` Runtime Panic

**Files:**
- Modify: `pkg/vm/executor_test.go`
- Modify: `pkg/vm/executor.go`
- Modify: `pkg/core/init.go` if matcher/eval error conversion is responsible
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Add a focused VM regression**

Append this test to `pkg/vm/executor_test.go`:

```go
func TestEvalBreakTrueOrFalseReturnsSyntaxErrorInsteadOfPanicking(t *testing.T) {
	_, out := runRuby(t, `begin
  eval "break true or false"
rescue SyntaxError => e
  puts e.class
end`)
	if out != "SyntaxError\n" {
		t.Fatalf("expected SyntaxError output, got %q", out)
	}
}
```

- [ ] **Step 2: Run the focused VM test and verify it fails**

Run:

```bash
go test ./pkg/vm -run TestEvalBreakTrueOrFalseReturnsSyntaxErrorInsteadOfPanicking -count=1
```

Expected: FAIL or panic with the current index-out-of-range stack trace.

- [ ] **Step 3: Fix eval/control-flow error handling**

Inspect:

- `pkg/vm/executor.go:Run`
- `pkg/vm/executor.go:evalSource`
- `pkg/core/init.go:methodEval`
- `pkg/core/init.go:evaluateRaiseErrorMatcher`

Make `eval "break true or false"` return a Ruby `SyntaxError` object/error path instead of underflowing the VM stack. Preserve existing passing behavior for:

```ruby
-> { break false || true }.call.should be_true
-> { eval "next true or false" }.should raise_error(SyntaxError, /void value expression/)
-> { eval "return true or false" }.should raise_error(SyntaxError, /void value expression/)
```

- [ ] **Step 4: Verify focused VM test passes**

Run:

```bash
go test ./pkg/vm -run TestEvalBreakTrueOrFalseReturnsSyntaxErrorInsteadOfPanicking -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify target spec passes**

Run:

```bash
./rgo test vendor/ruby/spec/language/or_spec.rb
```

Expected:

```text
16 examples, 0 failures
```

- [ ] **Step 6: Refresh dashboard and commit**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
git add pkg/vm/executor_test.go pkg/vm/executor.go pkg/core/init.go reports/spec-status/language.csv
git commit -m "fix: return syntax errors for invalid eval control flow"
```

Expected: `or_spec.rb` is `pass`.

## Task 6: Re-Sort Remaining Nonzero Failures

**Files:**
- Modify: `reports/spec-status/language.csv`
- Modify: `TODO.md` only for unrelated blockers

- [ ] **Step 1: Print remaining non-pass rows**

Run:

```bash
awk -F, 'NR==1 || $2!="pass" {print}' reports/spec-status/language.csv
```

Expected: no `parse_error`, no `runtime_error`, no `timeout`, no `compile_error`. Remaining rows should be `nonzero_failures`.

- [ ] **Step 2: Select the next target group**

Choose the group with the highest shared root cause from the remaining nonzero failures. Use this priority:

1. block/proc/lambda/yield/return/break/next/redo/retry
2. class/module/constants/class_variable/singleton_class/metaclass
3. keyword_arguments/method/super/delegation/send
4. regexp/back-references/character_classes/encoding/escapes/grouping/interpolation/modifiers
5. remaining isolated files

- [ ] **Step 3: Record unrelated discoveries**

If investigation reveals an unrelated blocker, add one bullet under the relevant section in `TODO.md`:

```markdown
- [ ] `path/to/spec.rb` brief problem summary discovered while clearing language gate; defer until its target group is selected.
```

Commit only if `TODO.md` changed:

```bash
git add TODO.md
git commit -m "docs: record deferred language spec blocker"
```

## Task 7: Clear One Nonzero-Failure Group At A Time

**Files:**
- Modify: the relevant package files for the selected group
- Modify: focused tests in `pkg/parser`, `pkg/compiler`, or `pkg/vm`
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Run the selected target spec**

For example, if the selected group is block/proc/lambda:

```bash
./rgo test vendor/ruby/spec/language/block_spec.rb
./rgo test vendor/ruby/spec/language/proc_spec.rb
./rgo test vendor/ruby/spec/language/lambda_spec.rb
```

Expected: visible assertion failures with examples and failure messages.

- [ ] **Step 2: Add the smallest focused regression**

Add a Go test in the package where the behavior belongs. Use `pkg/vm/executor_test.go` for end-to-end language behavior:

```go
func TestLanguageGateFocusedRegression(t *testing.T) {
	result, _ := runRuby(t, `1 + 1`)
	assertIntResult(t, result, 2)
}
```

Replace the Ruby source and assertion with the smallest failing expression copied from the selected spec failure.

- [ ] **Step 3: Verify the regression fails**

Run the focused test:

```bash
go test ./pkg/vm -run TestLanguageGateFocusedRegression -count=1
```

Expected: FAIL with the same behavior gap as the selected spec.

- [ ] **Step 4: Implement the smallest fix**

Make the behavior change in the earliest responsible layer:

- parser syntax: `pkg/parser/parser.go`
- bytecode generation or scope layout: `pkg/compiler/compiler.go`
- stack, frames, dispatch, blocks, exceptions: `pkg/vm/executor.go`
- minimal runtime object/method support: `pkg/core/init.go`

- [ ] **Step 5: Verify focused regression passes**

Run:

```bash
go test ./pkg/vm -run TestLanguageGateFocusedRegression -count=1
```

Expected: PASS.

- [ ] **Step 6: Verify selected target spec passes**

Run the selected spec file:

```bash
./rgo test vendor/ruby/spec/language/block_spec.rb
```

Expected: failure count decreases. If the file reaches `0 failures`, refresh the dashboard before moving to the next file.

- [ ] **Step 7: Refresh dashboard and commit**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
git add pkg/parser pkg/compiler pkg/vm pkg/core TODO.md reports/spec-status/language.csv
git commit -m "fix: advance language spec compatibility"
```

Expected: at least one language spec file improves status or failure count. Do not commit if there is no measurable improvement unless the commit only adds a failing test for the next task.

## Task 8: Final Language Gate Verification

**Files:**
- Modify: `reports/spec-status/language.csv`

- [ ] **Step 1: Run full Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Refresh full language dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected:

```text
Wrote reports/spec-status/language.csv (80 specs)
```

- [ ] **Step 3: Confirm all language specs pass**

Run:

```bash
awk -F, 'NR>1 && $2!="pass" {print}' reports/spec-status/language.csv
```

Expected: no output.

- [ ] **Step 4: Run touched-gate regression dashboards**

If parser/compiler/VM shared behavior changed, run at least:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/string reports/spec-status/string.csv
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/kernel reports/spec-status/kernel.csv
```

Expected: no regression from the previously cleared status documented in `TODO.md`.

- [ ] **Step 5: Commit final dashboard**

Run:

```bash
git add reports/spec-status/language.csv reports/spec-status/array.csv reports/spec-status/string.csv reports/spec-status/kernel.csv
git commit -m "test: clear language spec dashboard"
```

Expected: commit contains the final passing language dashboard and any refreshed touched-gate dashboards.
