# Language Foundation Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce RGo language spec failures by fixing foundational block/yield/proc/lambda/control-flow behavior one focused gate at a time.

**Architecture:** Treat each failing Ruby spec file as a small gate, reduce one failing example to a Go regression test, then change parser/compiler/VM/core code minimally. Keep the current file layout and avoid unrelated cleanup while the working tree contains existing broad compatibility changes.

**Tech Stack:** Go, RGo bytecode VM, Ruby spec fixtures, shell spec gate scripts.

---

## File Structure

- Modify: `pkg/vm/executor_test.go` for focused end-to-end Ruby behavior tests.
- Modify: `pkg/vm/executor.go` for block, yield, control-flow, and frame semantics.
- Modify: `pkg/compiler/compiler.go` and `pkg/compiler/opcode.go` if bytecode generation or opcodes need adjustment.
- Modify: `pkg/parser/parser.go` and `pkg/parser/ast/node.go` only if a failure is a parse/AST problem.
- Modify: `pkg/core/init.go` only when the failing behavior is a Ruby core method needed by the spec.
- Modify: `TODO.md` only to record blockers that are too large for the current focused task.
- Read: `reports/spec-status/language-current.csv` for current gate status.

## Task 1: Refresh And Inspect Yield Gate

- [ ] **Step 1: Run the focused yield spec status**

Run:

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language/yield_spec.rb reports/spec-status/language-yield-current.csv
```

Expected: `language-yield-current.csv` reports `nonzero_failures` or `pass`.

- [ ] **Step 2: Inspect the yield spec failure log**

Run:

```bash
ls reports/spec-status/spec-logs | rg 'yield_spec'
```

Then open the matching log with `sed -n '1,220p'`.

Expected: identify the first concrete failing behavior.

- [ ] **Step 3: If yield already passes, move to block gate**

Run:

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language/block_spec.rb reports/spec-status/language-block-current.csv
```

Expected: `language-block-current.csv` reports the next active failure gate.

## Task 2: Add The First Failing Go Regression

- [ ] **Step 1: Locate existing VM helper patterns**

Run:

```bash
rg 'func Test.*Yield|func Test.*Block|runVM|Run' pkg/vm/executor_test.go
```

Expected: choose the existing helper style already used by VM end-to-end tests.

- [ ] **Step 2: Add one focused failing test**

Add exactly one test to `pkg/vm/executor_test.go` for the first failing yield/block behavior found in Task 1. Use the existing test helper style from the file. Keep the source snippet minimal Ruby code that demonstrates the expected Ruby behavior.

- [ ] **Step 3: Run the focused test and verify RED**

Run the exact focused test:

```bash
scripts/safe_go_test.sh ./pkg/vm -run TestNameAddedInStep2
```

Expected: FAIL because the current VM/compiler/parser behavior is missing or wrong.

## Task 3: Implement The Minimal Fix

- [ ] **Step 1: Identify owning layer**

Use the failure mode:

- Parser errors mean `pkg/parser/parser.go` or AST changes.
- Compile errors mean `pkg/compiler/compiler.go` or opcode changes.
- Runtime wrong result/errors mean `pkg/vm/executor.go` or `pkg/core/init.go`.

- [ ] **Step 2: Make the smallest production change**

Edit only the owning layer files needed for the failing behavior. Do not broaden the implementation to unrelated spec failures.

- [ ] **Step 3: Run the focused Go test and verify GREEN**

Run:

```bash
scripts/safe_go_test.sh ./pkg/vm -run TestNameAddedInStep2
```

Expected: PASS.

## Task 4: Verify Gate And Internal Suite

- [ ] **Step 1: Re-run the focused Ruby spec gate**

Run the same target from Task 1:

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language/yield_spec.rb reports/spec-status/language-yield-current.csv
```

Expected: fewer failures or `pass`. If the gate is still failing, repeat Tasks 2 and 3 for the next smallest failure.

- [ ] **Step 2: Run full internal Go tests**

Run:

```bash
scripts/safe_go_test.sh ./...
```

Expected: all Go package tests pass.

- [ ] **Step 3: Record blockers if needed**

If the next failure requires a broad redesign, add a concise entry to `TODO.md` under the current language gate section and continue with the next spec file.

## Task 5: Refresh Language Dashboard

- [ ] **Step 1: Refresh language status**

Run:

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language-current.csv
```

Expected: updated status for all language spec files.

- [ ] **Step 2: Summarize counts**

Run:

```bash
awk -F, 'NR>1 {count[$2]++; examples+=$3; failures+=$4} END {for (s in count) print s,count[s]; print "examples",examples; print "failures",failures; print "files",NR-1}' reports/spec-status/language-current.csv
```

Expected: pass count does not regress from the current baseline.
