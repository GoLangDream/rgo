# Language Timeout Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce the remaining bounded `vendor/ruby/spec/language` timeout set by clearing at least one target file or recording a concrete blocker and advancing another file.

**Architecture:** Treat `reports/spec-status/language.csv` as the dashboard source of truth, but reconcile it with the current dirty worktree before selecting new work. Investigate one timeout file at a time with bounded commands, add the smallest internal regression test that reproduces the root cause, then make the minimal parser/compiler/VM/core change needed for that reproducer.

**Tech Stack:** Go, Bash, RGo VM, ruby/spec, existing `scripts/spec_status.sh`, existing safe Go test scripts.

---

## File Structure

- Modify `reports/spec-status/language.csv`: regenerated dashboard after focused fixes.
- Modify `TODO.md`: update language gate counts and record any newly identified large blocker.
- Modify `pkg/parser/parser_test.go` and `pkg/parser/parser.go` only if investigation proves a syntax boundary or token consumption bug.
- Modify `pkg/compiler/compiler_test.go` and `pkg/compiler/compiler.go` only if investigation proves a bytecode lowering or jump patching bug.
- Modify `pkg/vm/executor_test.go` and VM implementation files under `pkg/vm/` only if investigation proves runtime execution, dispatch, block, rescue, or `super` behavior is the root cause.
- Modify files under `pkg/core/` only if investigation proves a missing Ruby core method/helper is directly responsible for the selected spec timeout.

## Task 1: Reconcile the Active Language Baseline

**Files:**
- Modify: `reports/spec-status/language.csv`
- Modify: `TODO.md`

- [ ] **Step 1: Confirm dirty worktree state**

Run:

```bash
git status --short
```

Expected: output includes the current uncommitted parser/compiler/VM test changes plus `TODO.md`. Do not revert or overwrite those changes.

- [ ] **Step 2: Build the current worktree binary**

Run:

```bash
go build -o rgo ./cmd/rgo
```

Expected: command exits 0 and produces or refreshes `./rgo`.

- [ ] **Step 3: Refresh the language dashboard with a bounded run**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected: command exits 0 and prints `Wrote reports/spec-status/language.csv (80 specs)`.

- [ ] **Step 4: Inspect active timeout rows**

Run:

```bash
grep ',timeout,' reports/spec-status/language.csv
```

Expected: timeout rows should match the current worktree state. If `next_spec.rb`, `or_spec.rb`, or `variables_spec.rb` still appear as failing after the refresh, do not start a new target; record the mismatch under the `Language spec gate` section in `TODO.md` and investigate the mismatch first.

- [ ] **Step 5: Update the language summary in `TODO.md`**

Edit the existing language gate summary so its pass/timeout/runtime counts match the refreshed `reports/spec-status/language.csv`. Keep existing per-file notes that are still true.

- [ ] **Step 6: Commit the reconciled baseline if only dashboard/docs changed**

Run:

```bash
git add reports/spec-status/language.csv TODO.md
git commit -m "test: refresh language spec baseline"
```

Expected: commit succeeds if only the dashboard and `TODO.md` changed in this task. If code changes from earlier work are still unstaged and required for the refreshed baseline, skip this commit and include the dashboard update in the later code commit instead.

## Task 2: Select and Reproduce the First Timeout Target

**Files:**
- Read: `reports/spec-status/language.csv`
- Read: one selected file under `vendor/ruby/spec/language/`
- Modify: `TODO.md` only if the selected target is too broad for this milestone

- [ ] **Step 1: List timeout files sorted by dashboard order**

Run:

```bash
grep ',timeout,' reports/spec-status/language.csv
```

Expected: output lists only the active timeout files.

- [ ] **Step 2: Select the first target**

Choose the first target in this priority order that still appears as `timeout`:

```text
vendor/ruby/spec/language/optional_assignments_spec.rb
vendor/ruby/spec/language/keyword_arguments_spec.rb
vendor/ruby/spec/language/method_spec.rb
vendor/ruby/spec/language/super_spec.rb
vendor/ruby/spec/language/predefined_spec.rb
vendor/ruby/spec/language/rescue_spec.rb
```

Expected: selected target is one file. This order favors assignment/argument surface area before deeper rescue unwinding.

- [ ] **Step 3: Run the selected target alone with a bounded timeout**

Run, replacing the path with the selected target from Step 2:

```bash
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: command exits 0 and `/tmp/rgo-selected-language-target.csv` contains one row. If the row is `pass`, skip to Task 6 and refresh the full dashboard. If the row is `timeout`, continue.

- [ ] **Step 4: Run the selected target through the CLI to capture visible output**

Run, replacing the path with the selected target from Step 2:

```bash
timeout --kill-after=2s 2s ./rgo test vendor/ruby/spec/language/optional_assignments_spec.rb
```

Expected: either partial example output before timeout, or no useful output before timeout. Save the exact command and observed behavior for the next test name and `TODO.md` note if needed.

- [ ] **Step 5: Read the selected spec file and identify the smallest likely hanging construct**

Use the selected spec file content to find the smallest construct near the last visible example or the earliest control-flow/argument construct in the file. Prefer a single Ruby snippet that can run through `runRuby` in `pkg/vm/executor_test.go`.

Expected: one concrete Ruby snippet is chosen for a focused Go regression test. If no small snippet can be isolated within 20 minutes, add a blocker note to `TODO.md` under `Language spec gate` and return to Step 2 with the next target file.

## Task 3: Add a Focused Failing Regression Test

**Files:**
- Modify: `pkg/vm/executor_test.go` for runtime hangs or wrong values
- Modify: `pkg/compiler/compiler_test.go` for compile-time panics or bad bytecode lowering
- Modify: `pkg/parser/parser_test.go` for parser hangs or wrong AST shape

- [ ] **Step 1: Choose the test file by failure layer**

Use this mapping:

```text
parser cannot finish or AST shape is wrong -> pkg/parser/parser_test.go
compiler errors or emits looping bytecode -> pkg/compiler/compiler_test.go
VM run hangs, panics, or returns wrong value -> pkg/vm/executor_test.go
```

Expected: one test file is selected. Do not add tests to multiple layers unless the first focused test proves the bug crosses that boundary.

- [ ] **Step 2: Add a timeout-guarded VM regression when the failure is a hang**

If `optional_assignments_spec.rb` is still the selected target and the investigation points at optional assignment runtime execution, add this concrete regression to `pkg/vm/executor_test.go` and adjust only the Ruby source if Task 2 identified a later, smaller construct in the same file:

```go
func TestOptionalAssignmentSpecReproducerTerminates(t *testing.T) {
	type result struct {
		value *object.EmeraldValue
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, _ := runRuby(t, `
a = false
a ||= 10
`)
		done <- result{value: value}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		assertIntResult(t, got.value, 10)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("optional assignment reproducer did not terminate")
	}
}
```

Expected: the test fails before implementation if Task 2 isolated a hanging optional-assignment runtime construct. If this exact local-variable snippet already passes, replace only the Ruby source and expected value with the concrete optional-assignment construct found in Task 2 before running Step 5. If `time` is not already imported in `pkg/vm/executor_test.go`, add it to the import block.

- [ ] **Step 3: Add a parser regression when the failure is syntax consumption**

If parser investigation is the root cause for `optional_assignments_spec.rb`, add this concrete regression to `pkg/parser/parser_test.go` and adjust only the Ruby source if Task 2 identified a later, smaller construct in the same file:

```go
func TestParseOptionalAssignmentAccessorOrEquals(t *testing.T) {
	program := parse(t, `
@a.b ||= 10
`)
	if len(program.Statements) == 0 {
		t.Fatal("expected at least one parsed statement")
	}
}
```

Expected: the test fails before implementation with the current parser error, hang guard, or wrong AST assertion.

- [ ] **Step 4: Add a compiler regression when the failure is bytecode lowering**

If compiler investigation is the root cause for `optional_assignments_spec.rb`, add this concrete regression to `pkg/compiler/compiler_test.go` and adjust only the Ruby source if Task 2 identified a later, smaller construct in the same file:

```go
func TestCompileOptionalAssignmentAccessorOrEquals(t *testing.T) {
	compile(t, `
@a.b ||= 10
`)
}
```

Expected: the test fails before implementation with the current compiler error or hangs only if run with an external `timeout` command.

- [ ] **Step 5: Run the focused test and verify it fails**

Run the matching command for the file selected in Step 1:

```bash
go test ./pkg/vm -run '^TestOptionalAssignmentSpecReproducerTerminates$' -count=1
go test ./pkg/parser -run '^TestParseOptionalAssignmentAccessorOrEquals$' -count=1
go test ./pkg/compiler -run '^TestCompileOptionalAssignmentAccessorOrEquals$' -count=1
```

Expected: exactly one command is run, with the test name updated to the concrete test added in Steps 2-4. It fails for the root-cause symptom observed in Task 2.

## Task 4: Implement the Minimal Root-Cause Fix

**Files:**
- Modify: the implementation file proven by Task 2 and Task 3
- Modify: the focused regression test from Task 3 only if the assertion needs to reflect Ruby-compatible behavior discovered during investigation

- [ ] **Step 1: Identify the nearest working pattern**

Search for the closest existing passing implementation before editing:

```bash
grep -R "compile.*Assignment\|OpSendSuper\|keyword\|rescue\|nextPatchPos\|redo" -n pkg/parser pkg/compiler pkg/vm pkg/core
```

Expected: identify an existing nearby pattern to copy or extend. If no nearby pattern exists, record the absence in the task notes before editing.

- [ ] **Step 2: Make one minimal implementation change**

Edit only the file identified by the failing test layer. Examples of acceptable minimal changes:

```text
parser root cause -> adjust token boundary or method-name acceptance for the selected syntax only
compiler root cause -> patch the missing jump/operand/lowering path for the selected AST node only
VM root cause -> stop the selected runaway execution path or fix the selected dispatch/control-flow result only
core root cause -> add the single missing method/helper behavior used by the selected reproducer only
```

Expected: no unrelated refactoring and no broad Ruby semantic rewrite.

- [ ] **Step 3: Run the focused regression test**

Run the same focused command from Task 3 Step 5.

Expected: focused regression test passes.

- [ ] **Step 4: Run the selected spec file with a bounded dashboard command**

Run, replacing the path with the selected target:

```bash
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: selected target is no longer `timeout`. `pass`, `runtime_error`, `compile_error`, or `nonzero_failures` are acceptable progress states if the timeout is eliminated.

- [ ] **Step 5: If the selected file remains timeout, stop after one fix attempt**

Append a blocker note under `Language spec gate` in `TODO.md` using the actual selected target. For `optional_assignments_spec.rb`, use this entry and replace the reproducer line with the concrete test name or Ruby snippet from Task 3:

```text
- [ ] `vendor/ruby/spec/language/optional_assignments_spec.rb` still times out after focused reproducer fix
  - Reproducer: `TestOptionalAssignmentSpecReproducerTerminates`
  - Observed command: `RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv`
  - Next investigation: continue from the next smallest hanging construct in the same spec or switch to the next timeout target.
```

Expected: the blocker note contains the actual selected spec path and test name.

## Task 5: Verify Package-Level Safety

**Files:**
- Read/modify only files touched by Task 4 if verification exposes a direct regression

- [ ] **Step 1: Run the relevant package tests through the safe wrapper**

Run the command matching the changed layer:

```bash
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/vm -run 'TestOptionalAssignmentSpecReproducerTerminates|TestNextWithValueInLambdaReturnsWithoutLooping' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/parser -run '^TestParseOptionalAssignmentAccessorOrEquals$' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/compiler -run '^TestCompileOptionalAssignmentAccessorOrEquals$' -count=1
```

Expected: run only the command for the changed package. It exits 0.

- [ ] **Step 2: Run existing nearby regression tests**

Run the command matching the touched area:

```bash
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/vm -run 'TestNext|TestBreak|TestRedo|TestRescue|TestSuper' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/parser -run 'Assignment|Method|Rescue|Super|Keyword' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/compiler -run 'Assignment|Method|Rescue|Super|Keyword|Next' -count=1
```

Expected: run only the command for the changed package. It exits 0 or exposes an existing unrelated failure. If it exposes an existing unrelated failure, record it in `TODO.md` and continue with the selected spec verification.

## Task 6: Refresh Dashboard and Documentation

**Files:**
- Modify: `reports/spec-status/language.csv`
- Modify: `TODO.md`
- Modify: `reports/spec-status/README.md` if the language baseline summary changes materially

- [ ] **Step 1: Refresh the full language dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected: command exits 0 and writes 80 spec rows.

- [ ] **Step 2: Count dashboard states**

Run:

```bash
cut -d, -f2 reports/spec-status/language.csv | sort | uniq -c
```

Expected: counts show at least one fewer `timeout` than the reconciled baseline, or show the selected blocker was recorded and another file advanced to a more precise failure class.

- [ ] **Step 3: Update `TODO.md` language section**

Edit `TODO.md` so the language summary includes the new pass/timeout/runtime/compile/nonzero counts, the selected file status, and the focused fix or blocker note.

- [ ] **Step 4: Update `reports/spec-status/README.md` if needed**

If the README language baseline still describes obsolete counts or obsolete timeout targets, update the language baseline block to match `reports/spec-status/language.csv`.

## Task 7: Final Verification and Commit

**Files:**
- Commit all implementation, focused tests, dashboard, and documentation changes from this plan

- [ ] **Step 1: Format Go code**

Run:

```bash
go fmt ./pkg/parser ./pkg/compiler ./pkg/vm ./pkg/core
```

Expected: command exits 0.

- [ ] **Step 2: Run focused test one final time**

Run the focused package test command used in Task 5 Step 1.

Expected: command exits 0.

- [ ] **Step 3: Run selected spec one final time**

Run, replacing the path with the selected target:

```bash
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: selected target is not `timeout`.

- [ ] **Step 4: Review the diff**

Run:

```bash
git diff --stat
git diff -- pkg/parser pkg/compiler pkg/vm pkg/core TODO.md reports/spec-status/language.csv reports/spec-status/README.md
```

Expected: diff contains only the selected fix, focused tests, dashboard update, and documentation update.

- [ ] **Step 5: Commit**

Run:

```bash
git add pkg/parser pkg/compiler pkg/vm pkg/core TODO.md reports/spec-status/language.csv reports/spec-status/README.md
git commit -m "feat: reduce language spec timeouts"
```

Expected: commit succeeds. If `reports/spec-status/README.md` or a package directory was unchanged, omit the unchanged path from `git add`.

## Self-Review Notes

- Spec coverage: the plan confirms the active timeout set, uses bounded per-file spec runs, adds focused tests, avoids broad rewrites, updates dashboard files, and records large blockers in `TODO.md`.
- Placeholder scan: no placeholder tokens remain; template snippets use the first target file and require concrete names before execution.
- Type consistency: package names, script paths, and dashboard paths match the current repository layout.
