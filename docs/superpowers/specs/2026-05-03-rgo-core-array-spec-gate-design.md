# RGo Core Array Spec Gate Design

## Goal

Bring `vendor/ruby/spec/core/array` to a repeatable 100% pass rate under RGo, while preserving the existing Go test suite and feature smoke tests. This is the first milestone on the path to full Ruby and Rails compatibility.

## Current Baseline

- `vendor/ruby/spec` is present and updated to `origin/master` at `9b3f5ffd6`.
- `vendor/rails/rails` is present and updated to `origin/main` at `bf67001`.
- `go test ./...` passes.
- `scripts/feature_test.sh` reports `170 passed, 0 failed out of 170 tests`.
- Latest fast scan for `vendor/ruby/spec/core/array` with `RGO_SPEC_TIMEOUT=1`:
  - `37` passing files
  - `90` parse errors
  - `1` runtime error
  - `1` timeout
  - `350` observed examples
  - `0` observed example failures

## Scope

This milestone covers only the Array slice of ruby/spec plus the harness needed to measure it reliably. It does not attempt to run Rails specs yet. Rails requires Bundler, Minitest, ActiveSupport, RubyGems, and many standard libraries, so it becomes a later gate after language, core, and stdlib compatibility are much stronger.

In scope:

- Stable per-file spec status classification for `vendor/ruby/spec/core/array`.
- Parser and compiler fixes needed for Array specs to load and execute.
- MSpec DSL compatibility needed by Array specs.
- VM panic prevention and runtime correctness fixes found by Array specs.
- Array core method behavior needed by the specs.
- A final automated gate that fails if any Array spec is not passing.

Out of scope for this milestone:

- Rails specs.
- Full ruby/spec beyond `core/array`.
- Performance claims against CRuby.
- Rewriting standard libraries.
- Large architecture rewrites unrelated to Array spec execution.

## Architecture

Use ruby/spec as the external behavioral source of truth, but drive each fix through a minimized Go regression test first. The spec runner remains a thin CLI layer in `cmd/rgo`; Ruby semantics belong in lexer/parser/compiler/VM/core packages.

The work order is intentionally front-loaded toward infrastructure:

1. Keep the status dashboard accurate.
2. Eliminate parse errors and timeouts before adding many Array methods.
3. Implement missing MSpec primitives only as needed by Array specs.
4. Fix runtime crashes before semantic failures.
5. Finish Array methods after specs can reliably execute.

This avoids confusing a language parse failure with a missing library method.

## Components

### Spec Status Dashboard

`scripts/spec_status.sh` scans a spec file or directory, runs each file with a timeout, and writes CSV rows with:

- `file`
- `status`
- `examples`
- `failures`
- `error_kind`
- `duration_ms`

The statuses are:

- `pass`
- `parse_error`
- `compile_error`
- `runtime_error`
- `timeout`
- `nonzero_failures`

Reports live under `reports/spec-status/`.

### CLI Spec Runner

`cmd/rgo` provides `rgo test <file.rb>`. It expands selected `require_relative` shared examples, parses the combined file, compiles it, runs it in the VM, and prints an examples/failures summary.

The runner should stay small. If a behavior is Ruby semantics, implement it in the core runtime or VM, not as CLI-specific string rewriting.

### Parser And Compiler

Parser/compiler changes should target Ruby syntax that appears in Array specs, including block forms, method names, symbol arguments, class construction forms, and spec-helper idioms.

Every parser hang or parse error class gets a minimized test in `pkg/parser` or `pkg/compiler` before implementation.

### MSpec Compatibility

MSpec support lives in `pkg/core/init.go` today. This milestone only implements the subset used by Array specs. Required behavior should be added with small VM tests rather than broad stubs that hide real failures.

### Array Core

Array methods should use shared helpers where practical for conversion, equality, block calls, range handling, and nil behavior. Avoid adding near-duplicate loops when an existing helper can express the behavior clearly.

### VM Runtime Safety

No Ruby input should panic the Go process. Runtime mismatches should become Ruby-level errors or spec failures. Existing panics discovered through specs must be converted into deterministic runtime behavior and backed by regression tests.

## Data Flow

1. `scripts/spec_status.sh` selects one spec file.
2. `rgo test` reads and expands supported shared requires.
3. Lexer/parser produce an AST or a parse error.
4. Compiler produces bytecode or a compile error.
5. VM executes bytecode against initialized core classes and MSpec helpers.
6. MSpec records example counts and failures.
7. The dashboard classifies the result.

## Error Handling

- Parser errors should include actionable messages and must not loop indefinitely.
- Compiler errors should be returned, not panics.
- VM/core panics from bad Ruby input are bugs and must be replaced with checked conversions or Ruby-level errors.
- The dashboard treats process timeouts, parser errors, compiler errors, runtime errors, and failing examples as distinct statuses.

## Testing Strategy

Each fix follows this pattern:

1. Pick the first failing spec from the refreshed dashboard.
2. Minimize the failure into a small Ruby snippet.
3. Add a focused Go regression test in the relevant package.
4. Confirm the test fails for the expected reason.
5. Implement the smallest runtime/compiler/parser/core fix.
6. Run the focused Go test.
7. Run the target ruby/spec file.
8. Run `go test ./...` and `scripts/feature_test.sh`.
9. Refresh `reports/spec-status/array.csv`.

The milestone is complete only when:

- `go test ./...` passes.
- `scripts/feature_test.sh` passes.
- `scripts/spec_status.sh vendor/ruby/spec/core/array reports/spec-status/array.csv` reports every file as `pass`.
- `reports/spec-status/README.md` records `129/129` passing files for the current ruby/spec checkout.

## Performance Position

This milestone must avoid obvious performance regressions but does not claim CRuby parity or 10x speed. Performance gates come after broad language and stdlib correctness because current failures are dominated by compatibility and execution coverage.

## Follow-On Milestones

After Array is green:

1. Expand to Ruby language specs.
2. Expand through high-impact core classes: String, Hash, Integer, Enumerable, Module/Class, Exception, Proc, IO/File.
3. Implement required standard libraries in Go-native packages.
4. Add RubyGems, Bundler, Minitest, and ActiveSupport gates.
5. Run Rails components in order: ActiveSupport, ActiveModel, ActionView/ActionPack, ActiveRecord, Railties.
6. Add CRuby comparison benchmarks and optimize after correctness gates are stable.
