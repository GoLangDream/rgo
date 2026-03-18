# Fix Parser + Run Ruby Specs

## TL;DR
> **Summary**: Fix multi-line Hash/Array parsing, then run Ruby specs to measure progress.
> **Deliverables**: Working multi-line literals, spec baseline report
> **Effort**: Short
> **Parallel**: NO - sequential
> **Critical Path**: Fix parser → build → run specs

## Context
### Original Request
用户要求完成所有 TODO 任务，执行 ruby spec，不要中断。

### Current State
- 39/84 plan tasks complete
- Multi-line Hash/Array literals fail to parse (NEWLINE tokens not skipped)
- Spec baseline: Array 60%, String 60%, Hash 10%, Integer 30%, Float 10%

## Work Objectives
### Core Objective
Fix parser bugs and run Ruby specs to measure current coverage.

### Must NOT Have
- ❌ Do NOT ask user questions
- ❌ Do NOT stop between tasks
- ❌ Do NOT add unrelated features

## TODOs

- [ ] 1. Fix parseArrayLiteral and parseHashLiteral to skip NEWLINE tokens

  **What to do**:
  Edit `pkg/parser/parser.go`. Replace `parseArrayLiteral` (lines ~514-545) and `parseHashLiteral` (lines ~547-573) with versions that skip NEWLINE tokens after opening delimiter, after commas, and before closing delimiter.

  **Exact replacement for parseArrayLiteral**:
  ```go
  func (p *Parser) parseArrayLiteral() ast.Expression {
  	arr := &ast.ArrayLiteral{
  		Token:    p.curToken,
  		Elements: []ast.Expression{},
  	}
  	for p.peekTokenIs(lexer.NEWLINE) {
  		p.nextToken()
  	}
  	if p.peekTokenIs(lexer.RBRACKET) {
  		p.nextToken()
  		return arr
  	}
  	p.nextToken()
  	element := p.parseExpression(LOWEST)
  	if element != nil {
  		arr.Elements = append(arr.Elements, element)
  	}
  	for p.peekTokenIs(lexer.COMMA) {
  		p.nextToken()
  		for p.peekTokenIs(lexer.NEWLINE) {
  			p.nextToken()
  		}
  		p.nextToken()
  		element := p.parseExpression(LOWEST)
  		if element != nil {
  			arr.Elements = append(arr.Elements, element)
  		}
  	}
  	for p.peekTokenIs(lexer.NEWLINE) {
  		p.nextToken()
  	}
  	if !p.expectPeek(lexer.RBRACKET) {
  		return nil
  	}
  	return arr
  }
  ```

  **Exact replacement for parseHashLiteral**:
  ```go
  func (p *Parser) parseHashLiteral() ast.Expression {
  	hash := &ast.HashLiteral{
  		Token: p.curToken,
  		Pairs: make(map[ast.Expression]ast.Expression),
  		Order: []ast.Expression{},
  	}
  	for p.peekTokenIs(lexer.NEWLINE) {
  		p.nextToken()
  	}
  	if p.peekTokenIs(lexer.RBRACE) {
  		p.nextToken()
  		return hash
  	}
  	p.nextToken()
  	p.parseHashPair(hash)
  	for p.peekTokenIs(lexer.COMMA) {
  		p.nextToken()
  		for p.peekTokenIs(lexer.NEWLINE) {
  			p.nextToken()
  		}
  		p.nextToken()
  		p.parseHashPair(hash)
  	}
  	for p.peekTokenIs(lexer.NEWLINE) {
  		p.nextToken()
  	}
  	if !p.expectPeek(lexer.RBRACE) {
  		return nil
  	}
  	return hash
  }
  ```

  **Recommended Agent Profile**:
  - Category: `quick`
  - Skills: []

  **Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [2] | Blocked By: []

  **References**:
  - File: `pkg/parser/parser.go` lines 514-573

  **Acceptance Criteria**:
  - [ ] `go build ./cmd/rgo` succeeds
  - [ ] `go test ./pkg/parser -count=1` passes
  - [ ] Multi-line hash parses: `h = {\n  "a" => 1\n}` → `h["a"]` returns 1
  - [ ] Multi-line array parses: `arr = [\n  1,\n  2\n]` → `arr[0]` returns 1

  **QA Scenarios**:
  ```
  Scenario: Multi-line hash
    Tool: Bash
    Steps:
      printf 'h = {\n  "a" => 1,\n  "b" => 2\n}\nputs h["a"]' > /tmp/t.rb
      ./rgo run /tmp/t.rb
    Expected: 1
    Evidence: .sisyphus/evidence/task-1-multiline-hash.txt

  Scenario: Multi-line array
    Tool: Bash
    Steps:
      printf 'arr = [\n  1,\n  2,\n  3\n]\nputs arr[0]' > /tmp/t.rb
      ./rgo run /tmp/t.rb
    Expected: 1
    Evidence: .sisyphus/evidence/task-1-multiline-array.txt
  ```

  **Commit**: YES | Message: `fix(parser): support multi-line Hash and Array literals` | Files: [pkg/parser/parser.go]

---

- [ ] 2. Run Ruby spec baseline and record results

  **What to do**:
  Run a sample of Ruby specs for each core class and record pass/fail counts. Save results to `.sisyphus/evidence/spec-run-YYYYMMDD.txt`.

  Run these spec directories (10 specs each, timeout 5s per spec):
  - `vendor/ruby/spec/core/array/`
  - `vendor/ruby/spec/core/string/`
  - `vendor/ruby/spec/core/hash/`
  - `vendor/ruby/spec/core/integer/`
  - `vendor/ruby/spec/core/float/`

  For each spec file: run `./rgo run <spec>`, check if output contains `✓` (pass) or parse/runtime errors (fail).

  **Recommended Agent Profile**:
  - Category: `quick`
  - Skills: []

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: [] | Blocked By: [1]

  **References**:
  - Spec dir: `vendor/ruby/spec/core/`
  - Binary: `./rgo run`

  **Acceptance Criteria**:
  - [ ] Results file saved to `.sisyphus/evidence/spec-run-YYYYMMDD.txt`
  - [ ] All 5 classes tested
  - [ ] Pass rate recorded per class

  **QA Scenarios**:
  ```
  Scenario: Spec results recorded
    Tool: Bash
    Steps:
      ls .sisyphus/evidence/spec-run-*.txt
    Expected: File exists with results
    Evidence: .sisyphus/evidence/task-2-spec-run.txt
  ```

  **Commit**: YES | Message: `chore: record Ruby spec baseline results` | Files: [.sisyphus/evidence/]

## Final Verification Wave

- [ ] F1. Verify parser fix works end-to-end
  Run: `./rgo run vendor/ruby/spec/core/hash/store_spec.rb` — should show more passing tests than before.

## Commit Strategy
- Task 1: `fix(parser): support multi-line Hash and Array literals`
- Task 2: `chore: record Ruby spec baseline results`

## Success Criteria
- [ ] Multi-line Hash/Array literals parse correctly
- [ ] Spec baseline recorded with per-class pass rates
