# RGo Core Array Spec Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every file under `vendor/ruby/spec/core/array` pass under `rgo test`.

**Architecture:** Use the existing `rgo test` path as the compatibility gate, keep the dashboard authoritative, and drive each fix from a minimized Go regression test. Fix parse/compile/runtime blockers before expanding Array semantics.

**Tech Stack:** Go, RGo lexer/parser/compiler/VM/core runtime, `cmd/rgo`, `vendor/ruby/spec`, shell dashboard scripts.

---

### Task 1: Refresh The Array Dashboard Against Latest ruby/spec

**Files:**
- Modify: `reports/spec-status/array.csv`
- Modify: `reports/spec-status/README.md`
- Modify: `TODO.md`

- [ ] **Step 1: Run the latest dashboard scan**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv
```

Expected output:

```text
Wrote reports/spec-status/array.csv (129 specs)
```

- [ ] **Step 2: Calculate totals**

Run:

```bash
awk -F, 'NR>1 {count[$2]++; examples+=$3; failures+=$4} END {for (s in count) print s,count[s]; print "examples",examples; print "failures",failures}' reports/spec-status/array.csv
```

Expected current baseline:

```text
pass 37
timeout 1
runtime_error 1
parse_error 90
examples 350
failures 0
```

- [ ] **Step 3: Update the report text**

Set `reports/spec-status/README.md` to record:

```markdown
# Spec Status Reports

Generated with:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv
```

## core/array Baseline

- ruby/spec revision: `9b3f5ffd6`
- Total files scanned: 129
- Passing files: 37
- Timeout files: 1
- Parse error files: 90
- Runtime error files: 1
- Nonzero failure files: 0
- Examples observed in completed files: 350
- Failures observed in completed files: 0

## Interpretation

Most non-passing files still fail before meaningful examples execute. Parser/MSpec compatibility is the dominant blocker, followed by one runtime panic class and one timeout.

## First Runtime Error Target

- `vendor/ruby/spec/core/array/delete_at_spec.rb`

## First Parse Error Target

- `vendor/ruby/spec/core/array/any_spec.rb`
```

- [ ] **Step 4: Update TODO**

Add this entry under the latest known issues section in `TODO.md`:

```markdown
- [ ] Refresh Array spec gate to latest ruby/spec `9b3f5ffd6`
  - Current `RGO_SPEC_TIMEOUT=1` baseline: 37 pass, 90 parse_error, 1 runtime_error, 1 timeout out of 129 files.
  - First runtime target: `vendor/ruby/spec/core/array/delete_at_spec.rb`.
  - First parser target: `vendor/ruby/spec/core/array/any_spec.rb`.
```

- [ ] **Step 5: Verify dashboard script**

Run:

```bash
scripts/spec_status_test.sh
```

Expected output:

```text
spec_status_test: PASS
```

### Task 2: Stop `Array#delete_at` From Panicking On `to_int` Specs

**Files:**
- Modify: `pkg/core/init.go`
- Modify: `pkg/core/core_test.go`

- [ ] **Step 1: Add a failing core regression test**

Add this test near the Array method tests in `pkg/core/core_test.go`:

```go
func TestArrayDeleteAtNonIntegerDoesNotPanic(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2))
	arg := &object.EmeraldValue{Type: object.ValueObject, Data: "not-int", Class: R.Classes["Object"]}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("delete_at panicked for non-integer argument: %v", recovered)
		}
	}()

	result := callMethod(t, arr, "delete_at", arg)
	assertNil(t, result)
}
```

- [ ] **Step 2: Run the focused test and confirm failure**

Run:

```bash
go test ./pkg/core -run TestArrayDeleteAtNonIntegerDoesNotPanic -count=1
```

Expected before the fix:

```text
FAIL
delete_at panicked for non-integer argument
```

- [ ] **Step 3: Add a checked integer conversion helper**

Add this helper in `pkg/core/init.go` near the Array helpers:

```go
func valueToArrayIndex(value *object.EmeraldValue) (int, bool) {
	if value == nil || value.Type != object.ValueInteger {
		return 0, false
	}
	idx, ok := value.Data.(int64)
	if !ok {
		return 0, false
	}
	return int(idx), true
}
```

- [ ] **Step 4: Use the helper in `arrayDeleteAt`**

Replace the direct assertion:

```go
idx := int(args[0].Data.(int64))
```

with:

```go
idx, ok := valueToArrayIndex(args[0])
if !ok {
	return R.NilVal
}
```

- [ ] **Step 5: Verify focused tests**

Run:

```bash
go test ./pkg/core -run 'TestArrayDeleteAt|TestArrayDeleteAtNonIntegerDoesNotPanic' -count=1
```

Expected:

```text
ok  	github.com/GoLangDream/rgo/pkg/core
```

- [ ] **Step 6: Re-run the target ruby/spec**

Run:

```bash
timeout 10 ./rgo test vendor/ruby/spec/core/array/delete_at_spec.rb
```

Expected after this safety fix: the process must not print a Go panic stack. It may still report Ruby-level failures until `mock#to_int`, frozen arrays, and `raise_error` behavior are completed.

### Task 3: Parse Chained Predicate Matchers Used By `any_spec.rb`

**Files:**
- Modify: `pkg/parser/parser_test.go`
- Modify: `pkg/parser/parser.go`

- [ ] **Step 1: Add a parser regression for chained zero-arg predicate calls**

Add this test to `pkg/parser/parser_test.go`:

```go
func TestParseChainedPredicateMethodCall(t *testing.T) {
	input := "empty_array.should_not.any?"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}
```

- [ ] **Step 2: Run the focused parser test and confirm failure**

Run:

```bash
go test ./pkg/parser -run TestParseChainedPredicateMethodCall -count=1
```

Expected before the fix:

```text
FAIL
parser errors
```

- [ ] **Step 3: Fix parser handling for method-call chaining**

Update `parseExpression`/method-call parsing so an expression returned by one `.` method call can immediately become the receiver for another `.` method call. The minimal accepted input for this task is:

```ruby
empty_array.should_not.any?
```

- [ ] **Step 4: Verify focused parser tests**

Run:

```bash
go test ./pkg/parser -run 'TestParseChainedPredicateMethodCall|TestMethodCall' -count=1
```

Expected:

```text
ok  	github.com/GoLangDream/rgo/pkg/parser
```

- [ ] **Step 5: Re-run the target spec**

Run:

```bash
timeout 10 ./rgo test vendor/ruby/spec/core/array/any_spec.rb
```

Expected after this parser fix: the original `no prefix parse function for . found` errors disappear. Remaining parse or runtime failures become the next minimized task.

### Task 4: Add A Strict Array Gate Script

**Files:**
- Create: `scripts/array_spec_gate.sh`
- Modify: `TODO.md`

- [ ] **Step 1: Create the gate script**

Create `scripts/array_spec_gate.sh`:

```bash
#!/bin/bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/reports/spec-status/array.csv"

(cd "$ROOT" && go test ./...)
(cd "$ROOT" && scripts/feature_test.sh)
(cd "$ROOT" && RGO_SPEC_TIMEOUT="${RGO_SPEC_TIMEOUT:-5}" scripts/spec_status.sh vendor/ruby/spec/core/array "$OUT")

non_pass=$(awk -F, 'NR>1 && $2!="pass" {count++} END {print count+0}' "$OUT")
if [ "$non_pass" -ne 0 ]; then
  awk -F, 'NR>1 && $2!="pass" {print $2 " " $1}' "$OUT" >&2
  exit 1
fi
```

- [ ] **Step 2: Make it executable**

Run:

```bash
chmod +x scripts/array_spec_gate.sh
```

- [ ] **Step 3: Run the gate and confirm it fails for known non-pass specs**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/array_spec_gate.sh
```

Expected current result:

```text
exit status 1
```

The stderr list must include `parse_error vendor/ruby/spec/core/array/any_spec.rb` until Task 3 is completed.

### Task 5: Process The Next Dashboard Failure

**Files:**
- Modify: package-specific Go files under `pkg/`
- Modify: package-specific Go tests under `pkg/`
- Modify: `reports/spec-status/array.csv`
- Modify: `reports/spec-status/README.md`
- Modify: `TODO.md`

- [ ] **Step 1: Select the next failing file**

Run:

```bash
awk -F, 'NR>1 && $2!="pass" {print $1 "," $2 "," $5; exit}' reports/spec-status/array.csv > /tmp/rgo-next-array-failure.txt
cat /tmp/rgo-next-array-failure.txt
```

Expected current output before Tasks 2 and 3 are complete:

```text
vendor/ruby/spec/core/array/any_spec.rb,parse_error,parse_error
```

- [ ] **Step 2: Reproduce that one file**

Run:

```bash
spec=$(cut -d, -f1 /tmp/rgo-next-array-failure.txt)
timeout 10 ./rgo test "$spec"
```

Expected: the command reproduces the status selected in Step 1.

- [ ] **Step 3: Minimize and add one Go regression**

For a parser failure, add the smallest Ruby snippet to `pkg/parser/parser_test.go`.

For a compiler failure, add the smallest Ruby snippet to `pkg/compiler/compiler_test.go`.

For a runtime or semantic failure, add the smallest Ruby snippet to `pkg/vm/executor_test.go` or the smallest direct method test to `pkg/core/core_test.go`.

- [ ] **Step 4: Implement the minimized fix**

Change the smallest relevant production file:

- parser syntax: `pkg/parser/parser.go`
- bytecode generation: `pkg/compiler/compiler.go`
- VM execution: `pkg/vm/executor.go`
- core method/MSpec behavior: `pkg/core/init.go`

- [ ] **Step 5: Verify the focused fix**

Run the focused Go test added in Step 3.

- [ ] **Step 6: Verify the target spec**

Run:

```bash
spec=$(cut -d, -f1 /tmp/rgo-next-array-failure.txt)
timeout 10 ./rgo test "$spec"
```

Expected: the selected file either passes or advances to a later, distinct failure. If it advances, record the new failure in `TODO.md` and continue with the next minimized test.

- [ ] **Step 7: Refresh the dashboard**

Run:

```bash
RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv
```

- [ ] **Step 8: Stop condition**

Run:

```bash
awk -F, 'NR>1 && $2!="pass" {count++} END {print count+0}' reports/spec-status/array.csv
```

Expected when this milestone is ready for final verification:

```text
0
```

If the command prints a nonzero number, repeat Task 5 from Step 1 using the newly selected concrete file recorded in `/tmp/rgo-next-array-failure.txt`.

### Task 6: Final Verification And Documentation

**Files:**
- Modify: `reports/spec-status/README.md`
- Modify: `TODO.md`

- [ ] **Step 1: Run the strict gate**

Run:

```bash
scripts/array_spec_gate.sh
```

Expected:

```text
Wrote reports/spec-status/array.csv (129 specs)
```

and exit code `0`.

- [ ] **Step 2: Confirm all rows pass**

Run:

```bash
awk -F, 'NR>1 {count[$2]++} END {for (s in count) print s,count[s]}' reports/spec-status/array.csv
```

Expected:

```text
pass 129
```

- [ ] **Step 3: Update report**

Set `reports/spec-status/README.md` to say:

```markdown
## core/array Final Gate

- ruby/spec revision: `9b3f5ffd6`
- Total files scanned: 129
- Passing files: 129
- Non-passing files: 0
```

- [ ] **Step 4: Update TODO**

Mark the Array gate item complete:

```markdown
- [x] `vendor/ruby/spec/core/array` gate passes 129/129 files at ruby/spec `9b3f5ffd6`.
```

- [ ] **Step 5: Commit the milestone**

Run:

```bash
git add pkg cmd scripts reports TODO.md docs/superpowers/plans/2026-05-03-rgo-core-array-spec-gate.md
git commit -m "feat: pass ruby core array specs"
```
