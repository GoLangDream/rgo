# Ruby Parameter Destructuring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve nested Ruby parameter patterns from parsing through compilation and bind them correctly for methods, lambdas, procs, and blocks.

**Architecture:** Add parallel AST/runtime parameter-pattern trees. The compiler defines leaf locals and serializes patterns onto `object.Function`; the VM recursively expands one physical positional argument into those locals after normal argument binding.

**Tech Stack:** Go 1.26, RGo parser/compiler/VM, RubySpec.

## Global Constraints

- Standard library only; add no dependencies.
- Use TDD: every production change follows a focused failing test.
- Run all commands serially with `nice -n 10`, `GOMAXPROCS=1`, and `GOFLAGS='-p=1'`.
- Preserve ordinary argument, keyword, rest, block, and lambda arity behavior.
- Record unrelated failures in `TODO.md` and continue.

---

### Task 1: Parse Parameter Pattern Trees

**Files:**
- Modify: `pkg/parser/ast/node.go`
- Modify: `pkg/parser/parser.go`
- Test: `pkg/parser/parser_test.go`

**Interfaces:**
- Produces: `ast.ParameterPattern` and pattern slices aligned with top-level positional parameters on method/proc/block AST nodes.
- Consumes: lexer tokens already emitted for `(`, `)`, `,`, `*`, and identifiers.

- [ ] **Step 1: Write failing parser shape tests**

Add tests for `def m((a, *b, c)); end`, `-> ((a, (b, *c))) {}`, and `proc { |(a, *b)| }`. Assert ordered children, rest index/name, nesting, and one physical parameter.

- [ ] **Step 2: Verify RED**

Run:

```sh
nice -n 10 env GOMAXPROCS=1 GOFLAGS='-p=1' GOCACHE=/tmp/rgo-go-build-cache GOMODCACHE=/tmp/rgo-go-mod-cache go test ./pkg/parser -run ParameterDestructuring -count=1
```

Expected: compile failure because `ParameterPattern` metadata does not exist.

- [ ] **Step 3: Implement the AST and recursive parser**

Add:

```go
type ParameterPattern struct {
    Name      *Identifier
    Children  []*ParameterPattern
    Rest      *ParameterPattern
    RestIndex int
}
```

Add `ParamPatterns []*ParameterPattern` aligned with `Params`. Replace the current parenthesis-skipping branches with one recursive parser shared by def, lambda, and block parameter parsing. Keep a hidden physical parameter identifier per top-level pattern and validate duplicate named leaves.

- [ ] **Step 4: Verify GREEN**

Run the focused parser command and require PASS.

### Task 2: Compile Pattern Metadata and Leaf Locals

**Files:**
- Modify: `pkg/object/value.go`
- Modify: `pkg/compiler/compiler.go`
- Test: `pkg/compiler/compiler_test.go`

**Interfaces:**
- Consumes: `ast.ParameterPattern` slices from Task 1.
- Produces: `object.ParameterPattern`, `Function.ParamPatterns`, and local slots for every named leaf.

- [ ] **Step 1: Write a failing compiler metadata test**

Compile `def m((a, *b, c)); [a, b, c]; end` and assert the function has one physical parameter, a three-part runtime pattern, and local indices for `a`, `b`, and `c`.

- [ ] **Step 2: Verify RED**

Run:

```sh
nice -n 10 env GOMAXPROCS=1 GOFLAGS='-p=1' GOCACHE=/tmp/rgo-go-build-cache GOMODCACHE=/tmp/rgo-go-mod-cache go test ./pkg/compiler -run ParameterDestructuring -count=1
```

Expected: compile failure because runtime pattern metadata is absent.

- [ ] **Step 3: Implement serialization and local definition**

Add an object-layer tree mirroring the AST tree, with leaf `Name string`. During method and block/lambda compilation, recursively define named leaves, translate each AST tree, and store it in `Function.ParamPatterns` aligned with `Function.Params`.

- [ ] **Step 4: Verify GREEN**

Run the focused compiler command and require PASS.

### Task 3: Bind Patterns in the VM

**Files:**
- Modify: `pkg/vm/executor.go`
- Test: `pkg/vm/executor_test.go`

**Interfaces:**
- Consumes: `Function.ParamPatterns`, `Function.LocalNames`, and already-bound physical argument slots.
- Produces: recursively bound Ruby locals before function instructions execute.

- [ ] **Step 1: Write failing runtime tests**

Cover method and lambda forms for scalar/Array input, nested patterns, missing values, `*rest` with trailing children, anonymous rest, `to_ary` success, nil fallback, invalid conversion, and lambda arity.

- [ ] **Step 2: Verify RED**

Run:

```sh
nice -n 10 env GOMAXPROCS=1 GOFLAGS='-p=1' GOCACHE=/tmp/rgo-go-build-cache GOMODCACHE=/tmp/rgo-go-mod-cache go test ./pkg/vm -run ParameterDestructuring -count=1
```

Expected: assertions fail because leaf locals are nil or interpreted as missing method calls.

- [ ] **Step 3: Implement recursive binding**

Add helpers with these contracts:

```go
func (vm *VM) bindParameterPatterns(fn *object.Function, bp int) *object.EmeraldValue
func (vm *VM) destructureParameterValue(value *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue)
func (vm *VM) bindParameterPattern(fn *object.Function, pattern *object.ParameterPattern, value *object.EmeraldValue, bp int) *object.EmeraldValue
```

Call the binder after ordinary/rest/keyword/block slots are populated and before bytecode execution. Bind missing entries to nil, reserve trailing children before rest capture, and propagate coercion exceptions.

- [ ] **Step 4: Verify GREEN and regression scope**

Run focused VM tests, then focused parser/compiler tests, all serially.

### Task 4: RubySpec Verification and Dashboard Refresh

**Files:**
- Modify: `reports/spec-status/language-current.csv`
- Modify: `TODO.md`

**Interfaces:**
- Consumes: completed parser/compiler/VM behavior.
- Produces: authoritative method/lambda status and updated remaining backlog.

- [ ] **Step 1: Build once**

```sh
nice -n 10 env GOMAXPROCS=1 GOFLAGS='-p=1' GOCACHE=/tmp/rgo-go-build-cache GOMODCACHE=/tmp/rgo-go-mod-cache go build -o rgo ./cmd/rgo
```

- [ ] **Step 2: Run focused RubySpecs serially**

Run `method_spec.rb`, then `lambda_spec.rb`, using the project memory and timeout limits. Require zero failures or diagnose each remaining distinct root cause with a new focused TDD cycle.

- [ ] **Step 3: Refresh language dashboard**

Run the entire language directory serially and update `TODO.md` with exact pass/failure totals and any unrelated regressions.

- [ ] **Step 4: Continue the global completion plan**

Select the next highest shared failure cluster from the refreshed report; do not treat this slice as completion of the full RubySpec goal.
