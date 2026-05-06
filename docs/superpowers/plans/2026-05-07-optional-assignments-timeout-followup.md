# Optional Assignments Timeout Follow-up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Continue reducing the `vendor/ruby/spec/language/optional_assignments_spec.rb` timeout by isolating the next smallest blocker after the `super()` parser fixes.

**Architecture:** Use bounded commands to gather evidence before changing code. Add one focused RED/GREEN regression for the proven failure layer, make one minimal parser/compiler/VM/core fix, then rerun the selected spec; if it still times out, record the narrowed blocker in `TODO.md` and stop.

**Tech Stack:** Go, Bash, RGo VM, ruby/spec, `scripts/spec_status.sh`, `scripts/safe_go_test.sh`.

---

## File Structure

- Modify `reports/spec-status/language.csv`: refreshed language dashboard after bounded runs.
- Modify `TODO.md`: language dashboard summary and the selected `optional_assignments_spec.rb` blocker.
- Modify `pkg/parser/parser.go` and `pkg/parser/parser_test.go` only if the next reproducer proves a parser hang or AST boundary bug.
- Modify `pkg/compiler/compiler.go` and `pkg/compiler/compiler_test.go` only if parsing terminates but bytecode compilation fails or emits a looping instruction sequence.
- Modify `pkg/vm/executor_test.go` and implementation files under `pkg/vm/` only if parsing and compilation terminate but execution hangs or returns the wrong Ruby value.
- Modify files under `pkg/core/` only if the narrowed reproducer directly depends on a missing core method behavior.

## Task 1: Refresh Baseline and Confirm Selected Timeout

**Files:**
- Modify: `reports/spec-status/language.csv`
- Modify: `TODO.md` only if counts differ from the refreshed CSV

- [ ] **Step 1: Confirm clean starting state**

Run:

```bash
git status --short
```

Expected: no output, or only `reports/spec-status/language.csv` if a previous refresh changed volatile durations. Do not proceed if unrelated files are dirty.

- [ ] **Step 2: Build the current binary**

Run:

```bash
go build -o rgo ./cmd/rgo
```

Expected: exit 0.

- [ ] **Step 3: Refresh the language dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected: exit 0 and output `Wrote reports/spec-status/language.csv (80 specs)`.

- [ ] **Step 4: Confirm dashboard counts**

Run:

```bash
cut -d, -f2 reports/spec-status/language.csv | sort | uniq -c
```

Expected: counts include `74 pass`, `6 timeout`, and one `status` header row.

- [ ] **Step 5: Confirm the selected timeout row**

Run:

```bash
grep '^vendor/ruby/spec/language/optional_assignments_spec.rb,' reports/spec-status/language.csv
```

Expected: row status is `timeout`.

- [ ] **Step 6: Update `TODO.md` if aggregate counts changed**

If Step 4 differs from `TODO.md`, update the language dashboard summary and the blocker note. If only duration values changed, do not update `TODO.md` for durations.

## Task 2: Isolate the Next Hanging Construct

**Files:**
- Read: `vendor/ruby/spec/language/optional_assignments_spec.rb`
- Modify: `TODO.md` only if no focused reproducer is found within this task

- [ ] **Step 1: Reproduce the selected spec timeout**

Run:

```bash
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: exit 0 and `/tmp/rgo-selected-language-target.csv` reports `timeout`.

- [ ] **Step 2: Capture process-layer evidence with SIGQUIT**

Run:

```bash
timeout -s QUIT --kill-after=2s 2s ./rgo test vendor/ruby/spec/language/optional_assignments_spec.rb
```

Expected: exit 124 and a Go stack trace. Record whether the deepest repeating stack is in `pkg/parser`, `pkg/compiler`, `pkg/vm`, or `pkg/core`.

- [ ] **Step 3: Probe the `Class.new(Array)` index/super construct**

Run:

```bash
timeout --kill-after=2s 2s ./rgo run <(printf 'ary = Class.new(Array) do\n  def [](x, y)\n    super(x + 3 * y)\n  end\n  def []=(x, y, value)\n    super(x + 3 * y, value)\n  end\nend.new\nary[0, 0] = 1\nary[1, 0] = 1\nary[2, 0] = nil\nary[3, 0] = 1\nary[4, 0] = 1\nary[5, 0] = 1\nary[6, 0] = nil\nfoo = [0, 2]\nary[foo.pop, foo.pop] ||= 2\nary[2, 0]\n')
```

Expected: command exits within 2 seconds. If it times out, use this exact Ruby source as the reproducer.

- [ ] **Step 4: Probe the splatted index optional assignment construct**

Run:

```bash
timeout --kill-after=2s 2s ./rgo run <(printf 'class Box\n  def [](k)\n    @hash ||= {}\n    @hash[k]\n  end\n  def []=(k, v)\n    @hash ||= {}\n    @hash[k] = v\n    7\n  end\nend\nb = Box.new\nb[*[:m]] ||= 10\nb[:m]\n')
```

Expected: command exits within 2 seconds. If it times out, use this exact Ruby source as the reproducer.

- [ ] **Step 5: Probe nested splatted index optional assignment**

Run:

```bash
timeout --kill-after=2s 2s ./rgo run <(printf 'class Box\n  def [](k)\n    @hash ||= {}\n    @hash[k]\n  end\n  def []=(k, v)\n    @hash ||= {}\n    @hash[k] = v\n    7\n  end\nend\nb = Box.new\nb[*[*[:k]]] ||= 20\nb[:k]\n')
```

Expected: command exits within 2 seconds. If it times out, use this exact Ruby source as the reproducer.

- [ ] **Step 6: Probe accessor `&&=` construct**

Run:

```bash
timeout --kill-after=2s 2s ./rgo run <(printf 'klass = Class.new do\n  attr_accessor :b\nend\na = klass.new\na.b = 10\na.b &&= 20\na.b\n')
```

Expected: command exits within 2 seconds. If it times out, use this exact Ruby source as the reproducer.

- [ ] **Step 7: If no probe times out, record the narrowed blocker**

Append this note under `Language timeout reduction blocker（2026-05-06）` in `TODO.md`:

```markdown
  - 2026-05-07 follow-up probes: `Class.new(Array)` super index assignment, splatted index `||=`, nested splatted index `||=`, and accessor `&&=` snippets all terminated under 2s; full `optional_assignments_spec.rb` still times out. Next investigation should use the SIGQUIT stack from the full spec command to choose the next layer-specific reproducer.
```

Expected: no code fix is attempted if no minimal reproducer is found.

## Task 3: Add the Focused Failing Regression

**Files:**
- Modify: one of `pkg/parser/parser_test.go`, `pkg/compiler/compiler_test.go`, or `pkg/vm/executor_test.go`

- [ ] **Step 1: Choose the failure layer**

Use Task 2 evidence:

```text
SIGQUIT stack in parser or a parser-only probe hangs -> pkg/parser/parser_test.go
parser terminates but compile probe hangs/errors -> pkg/compiler/compiler_test.go
parser and compiler terminate but ./rgo run hangs/wrong value -> pkg/vm/executor_test.go
runtime reaches a missing method in core -> pkg/vm/executor_test.go first, then core implementation if proven
```

Expected: choose exactly one test file.

- [ ] **Step 2: Add a VM timeout regression for runtime hangs**

If the reproducer is one of the Ruby snippets from Task 2 and the failure layer is VM/runtime, add this test to `pkg/vm/executor_test.go`:

```go
func TestOptionalAssignmentFollowupReproducerTerminates(t *testing.T) {
	type result struct {
		value *object.EmeraldValue
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, _ := runRuby(t, `class Box
  def [](k)
    @hash ||= {}
    @hash[k]
  end
  def []=(k, v)
    @hash ||= {}
    @hash[k] = v
    7
  end
end
b = Box.new
b[*[:m]] ||= 10
b[:m]
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
		t.Fatal("optional assignment follow-up reproducer did not terminate")
	}
}
```

Expected: if this exact snippet is not the failing reproducer, replace the Ruby source and final assertion with the exact Task 2 snippet before running Step 5. The test name stays `TestOptionalAssignmentFollowupReproducerTerminates`.

- [ ] **Step 3: Add a parser timeout regression for parser hangs**

If Task 2 shows a parser hang, add this test to `pkg/parser/parser_test.go`:

```go
func TestParseOptionalAssignmentFollowupReproducerTerminates(t *testing.T) {
	result := make(chan []string, 1)
	go func() {
		l := lexer.New(`class Box
  def [](k)
    @hash ||= {}
    @hash[k]
  end
  def []=(k, v)
    @hash ||= {}
    @hash[k] = v
    7
  end
end
b = Box.new
b[*[:m]] ||= 10
b[:m]
`)
		p := New(l)
		p.ParseProgram()
		result <- p.Errors()
	}()

	select {
	case errors := <-result:
		if len(errors) > 0 {
			t.Fatalf("parse errors: %v", errors)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("optional assignment follow-up parser reproducer did not terminate")
	}
}
```

Expected: if this exact snippet is not the parser reproducer, replace only the Ruby source with the exact Task 2 parser-hanging snippet before running Step 5.

- [ ] **Step 4: Add a compiler regression for compile-layer failures**

If Task 2 shows parser termination but compiler failure, add this test to `pkg/compiler/compiler_test.go`:

```go
func TestCompileOptionalAssignmentFollowupReproducer(t *testing.T) {
	compile(t, `class Box
  def [](k)
    @hash ||= {}
    @hash[k]
  end
  def []=(k, v)
    @hash ||= {}
    @hash[k] = v
    7
  end
end
b = Box.new
b[*[:m]] ||= 10
b[:m]
`)
}
```

Expected: if this exact snippet is not the compiler reproducer, replace only the Ruby source with the exact Task 2 compiler-failing snippet before running Step 5.

- [ ] **Step 5: Run the focused regression and verify RED**

Run exactly one matching command:

```bash
go test ./pkg/vm -run '^TestOptionalAssignmentFollowupReproducerTerminates$' -count=1
go test ./pkg/parser -run '^TestParseOptionalAssignmentFollowupReproducerTerminates$' -count=1
go test ./pkg/compiler -run '^TestCompileOptionalAssignmentFollowupReproducer$' -count=1
```

Expected: exactly one command is run. It fails for the expected timeout, parse error, compile error, or wrong value from the reproducer.

## Task 4: Implement One Minimal Root-Cause Fix

**Files:**
- Modify: the implementation file matching the proven failure layer
- Modify: `TODO.md` if the full selected spec still times out after the fix

- [ ] **Step 1: Find nearby working patterns**

Run:

```bash
grep -R "\|\|=\|&&=\|IndexAssign\|Splat\|OpSendSuper\|CallBlock" -n pkg/parser pkg/compiler pkg/vm pkg/core
```

Expected: identify the nearest existing implementation path for the failing construct.

- [ ] **Step 2: Make one minimal implementation change**

Edit only the proven failure layer from Task 3. The acceptable change is one of:

```text
parser: consume the missing delimiter or construct boundary for the reproducer without changing unrelated precedence rules
compiler: emit or patch the missing instruction sequence for the reproducer without changing unrelated AST lowering
VM: stop the runaway execution path or correct the one wrong dispatch/assignment result for the reproducer
core: add the one missing method behavior directly exercised by the reproducer
```

Expected: no broad refactor and no second unrelated fix.

- [ ] **Step 3: Run focused GREEN verification**

Run the same focused command from Task 3 Step 5.

Expected: focused regression passes.

- [ ] **Step 4: Run the selected spec status**

Run:

```bash
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: command exits 0. If the row is still `timeout`, continue to Step 5.

- [ ] **Step 5: Record blocker if selected spec still times out**

If Step 4 remains `timeout`, append this note under `Language timeout reduction blocker（2026-05-06）`, replacing the test name only if the Task 3 test used parser/compiler instead of VM:

```markdown
  - 2026-05-07 follow-up: fixed `TestOptionalAssignmentFollowupReproducerTerminates`, but `RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv` still reports timeout. Stop after this focused fix; next round should continue from the full-spec SIGQUIT stack.
```

Expected: no further implementation attempts in this milestone.

## Task 5: Verify Package Safety and Refresh Documentation

**Files:**
- Modify: `reports/spec-status/language.csv`
- Modify: `TODO.md`

- [ ] **Step 1: Run package tests for the changed layer**

Run exactly one relevant command:

```bash
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/vm -run 'TestOptionalAssignmentFollowupReproducerTerminates|TestNextWithValueInLambdaReturnsWithoutLooping' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/parser -run 'TestParseOptionalAssignmentFollowupReproducerTerminates|TestParseSuperWithEmptyParenthesesTerminates|TestParseSuperWithParenthesizedArgumentsTerminates' -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/compiler -run 'TestCompileOptionalAssignmentFollowupReproducer|TestCompileKeywordLiteralMethodNameAfterDot' -count=1
```

Expected: the command for the changed package exits 0.

- [ ] **Step 2: Run broader changed package test**

Run exactly one relevant command:

```bash
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/vm -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/parser -count=1
RGO_GO_TEST_TIMEOUT=30 scripts/safe_go_test.sh ./pkg/compiler -count=1
```

Expected: the command for the changed package exits 0.

- [ ] **Step 3: Refresh full language dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv
```

Expected: exit 0 and 80 specs written.

- [ ] **Step 4: Update `TODO.md`**

Ensure `TODO.md` records:

```markdown
  - 2026-05-07 follow-up: focused reproducer `TestOptionalAssignmentFollowupReproducerTerminates` now passes.
```

If Task 3 used the parser test, use `TestParseOptionalAssignmentFollowupReproducerTerminates` instead. If Task 3 used the compiler test, use `TestCompileOptionalAssignmentFollowupReproducer` instead. If `optional_assignments_spec.rb` still times out, keep the blocker marked open and include the selected spec command from Task 4 Step 4.

## Task 6: Final Verification and Commit

**Files:**
- Commit all implementation, focused tests, dashboard, and documentation updates from this plan

- [ ] **Step 1: Format Go code**

Run:

```bash
go fmt ./pkg/parser ./pkg/compiler ./pkg/vm ./pkg/core
```

Expected: exit 0.

- [ ] **Step 2: Run final bounded verification**

Run:

```bash
RGO_GO_TEST_TIMEOUT=60 scripts/safe_go_test.sh ./pkg/parser ./pkg/compiler ./pkg/vm -count=1
RGO_SPEC_TIMEOUT=2 scripts/spec_status.sh vendor/ruby/spec/language/optional_assignments_spec.rb /tmp/rgo-selected-language-target.csv
```

Expected: package tests exit 0. The selected spec either advances from timeout or remains timeout with `TODO.md` documenting the latest focused blocker.

- [ ] **Step 3: Review diff**

Run:

```bash
git diff --stat
git diff -- pkg/parser pkg/compiler pkg/vm pkg/core TODO.md reports/spec-status/language.csv
```

Expected: diff includes only the focused fix/test, dashboard update, and TODO update.

- [ ] **Step 4: Commit**

Run:

```bash
git add pkg/parser pkg/compiler pkg/vm pkg/core TODO.md reports/spec-status/language.csv
git commit -m "fix: narrow optional assignment timeout"
```

Expected: commit succeeds. Omit any unchanged package path from `git add`.

## Self-Review Notes

- Spec coverage: tasks refresh dashboard, reproduce selected timeout, isolate one construct, use RED/GREEN, run bounded verification, update TODO, and stop after one focused fix if the full spec still times out.
- Placeholder scan: the plan contains no TBD markers or placeholder tokens; the TODO update step lists concrete test names for VM, parser, and compiler paths.
- Type consistency: test names and command paths are consistent across tasks.
