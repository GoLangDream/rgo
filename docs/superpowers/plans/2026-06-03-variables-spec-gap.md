# Variables Spec Gap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce or eliminate the remaining 7 failures in `vendor/ruby/spec/language/variables_spec.rb`.

**Architecture:** The current failures are concentrated in multiple assignment coercion error paths and one dynamic constant assignment syntax check. Implement focused VM/compiler behavior where the current code already performs multiassign coercion, and avoid broader parser rewrites unless the failing test proves they are necessary.

**Tech Stack:** Go VM/compiler/parser, RGo mspec shim, Ruby language specs.

---

### Task 1: Multiassign Coercion TypeError Paths

**Files:**
- Modify: `pkg/vm/executor.go`
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write failing focused tests**

Add VM tests covering:
- `a, b, c = x` where `x.to_ary` returns `1` must raise `TypeError`.
- `a, *b = 1, *x` where `x.to_a` returns `1` must raise `TypeError`.
- `a, (b, c), d = 1, x, 3, 4` where `x.to_ary` returns non-Array must raise `TypeError`.

- [ ] **Step 2: Verify focused tests fail**

Run:
```bash
scripts/safe_go_test.sh ./pkg/vm -run 'TestMultipleAssignment.*Coercion'
```

- [ ] **Step 3: Implement minimal VM/compiler fix**

Use the existing multiassign helper paths. When coercion calls return a non-Array and non-nil value, propagate a Ruby `TypeError` value so `raise_error(TypeError)` can catch it.

- [ ] **Step 4: Verify focused tests pass**

Run:
```bash
scripts/safe_go_test.sh ./pkg/vm -run 'TestMultipleAssignment.*Coercion'
```

### Task 2: Dynamic Non-ASCII Constant Assignment SyntaxError

**Files:**
- Modify: `pkg/vm/executor.go` or parser/lexer files if evidence points there
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write failing focused test**

Add a VM/mspec test for:
```ruby
-> { eval("def test\n  ἍBB = 1\nend") }.should raise_error(SyntaxError, /dynamic constant assignment/)
```

- [ ] **Step 2: Verify focused test fails**

Run:
```bash
scripts/safe_go_test.sh ./pkg/vm -run 'TestDynamicNonASCIIConstantAssignmentRaisesSyntaxError'
```

- [ ] **Step 3: Implement minimal syntax validation**

If parser already produces a constant assignment node, make dynamic validation reject it inside method scope. If lexer/parser fails to classify the identifier as a constant, record the parser gap in `TODO.md` and continue with Task 1 verification.

### Task 3: Spec and Regression Verification

**Files:**
- Update: `reports/spec-status/language-variables-current.csv`
- Update: `reports/spec-status/language-current.csv`
- Update: `TODO.md`

- [ ] **Step 1: Run all Go tests**

```bash
scripts/safe_go_test.sh ./...
```

- [ ] **Step 2: Refresh target spec**

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language/variables_spec.rb reports/spec-status/language-variables-current.csv
```

- [ ] **Step 3: Refresh language dashboard**

```bash
RGO_SPEC_TIMEOUT=5 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language-current.csv
```

- [ ] **Step 4: Update TODO**

If `variables_spec.rb` reaches 0 failures, mark the existing variables TODO item complete. Otherwise, update it with exact remaining cases.
