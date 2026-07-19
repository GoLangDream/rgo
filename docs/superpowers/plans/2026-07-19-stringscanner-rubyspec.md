# StringScanner RubySpec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Execute inline; do not dispatch subagents and do not run Git commands.

**Goal:** Make all 44 repository StringScanner specs pass with consistent byte offsets, regexp captures, scanner advancement, and reversible match state.

**Architecture:** Keep the public native methods registered in `pkg/core/init.go`, but move shared matching and state logic into focused `pkg/core/stringscanner.go` and `pkg/core/stringscanner_match.go` files. All scan/search methods consume one match-result structure, so advancement, captures, and failure clearing cannot diverge.

**Tech Stack:** Go 1.24, RGo native core methods, existing regexp translation layer, RubySpec/MSpec.

---

### Task 1: Shared match state and anchored operations

**Files:**
- Create: `pkg/core/stringscanner.go`
- Create: `pkg/core/stringscanner_match.go`
- Modify: `pkg/core/init.go:862-873`
- Modify: `pkg/core/init.go:52091-52755`
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write the failing state/anchored regression**

Add `TestStringScannerSharedAnchoredMatchState` using real Ruby behavior:

```go
func TestStringScannerSharedAnchoredMatchState(t *testing.T) {
    core.RegisterMspec()
    _, _ = runRuby(t, `require "strscan"
s = StringScanner.new("test string")
s.scan(/test/).should == "test"
s.pos.should == 4
s.matched.should == "test"
s.matched_size.should == 4
s.pre_match.should == ""
s.post_match.should == " string"
s.check(/ string/).should == " string"
s.pos.should == 4
s.scan(/missing/).should == nil
s.matched?.should == false`)
    if failures := core.GetSpecRunner().FailCount; failures != 0 {
        t.Fatalf("expected 0 failures, got %d", failures)
    }
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```sh
GOMAXPROCS=1 GOCACHE=/tmp/rgo-go-cache GOMODCACHE=/tmp/rgo-go-mod-cache GOFLAGS=-p=1 nice -n 10 go test ./pkg/vm -run '^TestStringScannerSharedAnchoredMatchState$' -count=1
```

Expected: FAIL because current match state loses exact spans or leaves stale data.

- [ ] **Step 3: Define exact scanner and match-result state**

Move the scanner state to `pkg/core/stringscanner.go`:

```go
type stringScannerSpan struct { start, end int }

type stringScannerData struct {
    source       *object.EmeraldValue
    content      string
    encoding     string
    offset       int
    previous     int
    canUnscan    bool
    matched      bool
    full         stringScannerSpan
    captures     []stringScannerSpan
    captureNames map[string]int
    fixedAnchor  bool
}

type stringScannerMatch struct {
    found    bool
    consumed stringScannerSpan
    full     stringScannerSpan
    captures []stringScannerSpan
    names    map[string]int
}
```

Use `start: -1, end: -1` for an unmatched capture. `source` retains the Ruby String object; `content` is refreshed after `string=` and `concat`.

- [ ] **Step 4: Implement one anchored matcher**

In `pkg/core/stringscanner_match.go`, add:

```go
func stringScannerAnchoredMatch(data *stringScannerData, pattern *object.EmeraldValue) (stringScannerMatch, *object.EmeraldValue)
func stringScannerApplyMatch(data *stringScannerData, match stringScannerMatch, advance bool)
func stringScannerClearRecordedMatch(data *stringScannerData)
func stringScannerSlice(data *stringScannerData, span stringScannerSpan) *object.EmeraldValue
```

`stringScannerAnchoredMatch` must translate the existing `RRegexp`, search only at `data.offset`, convert regexp-relative byte indices to source-absolute spans, and preserve unmatched captures as `{-1,-1}`. `stringScannerApplyMatch` records `previous` before advancing and enables `unscan` only for a successful advancing call.

- [ ] **Step 5: Route `scan`, `skip`, `check`, and `match?` through it**

Keep their Ruby return contracts exact:

```go
scan   => matched substring, advances
skip   => consumed byte count, advances
check  => matched substring, no advance
match? => matched byte count, no advance
```

On failure, return `nil`, clear recorded captures, preserve `offset`, and set `canUnscan` false.

- [ ] **Step 6: Verify the batch**

Run the focused Go test, build `rgo`, then run these files sequentially:

```sh
./rgo test vendor/ruby/spec/library/stringscanner/scan_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/skip_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/match_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/matched_spec.rb
```

Expected: focused Go test and all visible examples in the four files report zero failures.

### Task 2: Search operations and full return modes

**Files:**
- Modify: `pkg/core/stringscanner_match.go`
- Modify: `pkg/core/init.go:52383-52639`
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write the failing search/full regression**

Add `TestStringScannerSearchAndFullReturnModes`:

```go
func TestStringScannerSearchAndFullReturnModes(t *testing.T) {
    core.RegisterMspec()
    _, _ = runRuby(t, `require "strscan"
s = StringScanner.new("abc def")
s.check_until(/def/).should == "abc def"
s.pos.should == 0
s.exist?(/def/).should == 7
s.skip_until(/def/).should == 7
s.pos.should == 7
s.reset
s.scan_full(/abc/, true, true).should == "abc"
s.reset
s.scan_full(/abc/, true, false).should == 3
s.reset
s.search_full(/def/, false, true).should == "abc def"
s.pos.should == 0`)
    if failures := core.GetSpecRunner().FailCount; failures != 0 {
        t.Fatalf("expected 0 failures, got %d", failures)
    }
}
```

- [ ] **Step 2: Run it and verify RED**

Use the same single-worker Go command, replacing the `-run` expression with `^TestStringScannerSearchAndFullReturnModes$`. Expected: FAIL in current `scan_full`/search advancement or return-mode handling.

- [ ] **Step 3: Add forward-search matching**

Implement:

```go
func stringScannerSearchMatch(data *stringScannerData, pattern *object.EmeraldValue) (stringScannerMatch, *object.EmeraldValue)
```

The returned `consumed` span starts at the scanner offset and ends at the regexp match end; `full` spans only the regexp match. Captures remain relative to the regexp match and are converted to absolute source offsets.

- [ ] **Step 4: Centralize result selection**

Implement:

```go
func stringScannerMatchResult(data *stringScannerData, match stringScannerMatch, returnString bool) *object.EmeraldValue
```

When `returnString` is true, return the consumed substring. Otherwise return its byte length. Use it from `scan_full` and `search_full`; implement `scan_until`, `check_until`, `skip_until`, and `exist?` as fixed combinations of anchored/search, advance, and return mode.

- [ ] **Step 5: Verify search/full specs**

Run the focused test plus:

```sh
./rgo test vendor/ruby/spec/library/stringscanner/scan_full_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/scan_until_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/check_until_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/skip_until_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/search_full_spec.rb
./rgo test vendor/ruby/spec/library/stringscanner/exist_spec.rb
```

Expected: zero failures in the focused test and every file.

### Task 3: Captures, `unscan`, and invalidation

**Files:**
- Modify: `pkg/core/stringscanner.go`
- Modify: `pkg/core/stringscanner_match.go`
- Modify: `pkg/core/init.go:52287-52535`
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write the failing capture/rollback regression**

Add `TestStringScannerCapturesAndUnscanState`:

```go
func TestStringScannerCapturesAndUnscanState(t *testing.T) {
    core.RegisterMspec()
    _, _ = runRuby(t, `require "strscan"
s = StringScanner.new("abc123")
s.scan(/(?<letters>[a-z]+)(\d+)/).should == "abc123"
s[0].should == "abc123"
s[1].should == "abc"
s[2].should == "123"
s[:letters].should == "abc"
s.captures.should == ["abc", "123"]
s.values_at(0, 2).should == ["abc123", "123"]
s.unscan.should == s
s.pos.should == 0
-> { s.unscan }.should raise_error(StringScanner::Error)`)
    if failures := core.GetSpecRunner().FailCount; failures != 0 {
        t.Fatalf("expected 0 failures, got %d", failures)
    }
}
```

- [ ] **Step 2: Run it and verify RED**

Run the single-worker focused test. Expected: FAIL for named/unmatched capture spans or repeated `unscan` state.

- [ ] **Step 3: Make capture readers span-based**

Implement `stringScannerCapture` so integer `0` reads `full`, positive/negative indices address captures, symbols/strings resolve through `captureNames`, and unmatched/out-of-range entries return `nil`. Implement `captures` and `values_at` by repeatedly calling the same reader.

- [ ] **Step 4: Make rollback one-shot**

Implement `unscan` as:

```go
if !data.canUnscan { return newRuntimeException(R.Classes["StringScanner::Error"], "unscan failed") }
data.offset = data.previous
data.canUnscan = false
stringScannerClearRecordedMatch(data)
return receiver
```

`reset`, `terminate`, `string=`, and a failed match also disable rollback. `concat` refreshes content and clears the recorded match without changing a valid current offset.

- [ ] **Step 5: Verify capture/state specs**

Run the focused test plus `captures_spec.rb`, `values_at_spec.rb`, `element_reference_spec.rb`, `pre_match_spec.rb`, `post_match_spec.rb`, `unscan_spec.rb`, `reset_spec.rb`, `string_spec.rb`, and `concat_spec.rb`. Expected: zero failures.

### Task 4: Byte/character boundaries, duplication, and complete gate

**Files:**
- Modify: `pkg/core/stringscanner.go`
- Modify: `pkg/core/init.go:52140-52273`
- Modify: `pkg/core/init.go:52640-52755`
- Modify: `TODO.md`
- Test: `pkg/vm/executor_test.go`

- [ ] **Step 1: Write the failing multibyte/state-copy regression**

Add `TestStringScannerByteAndCharacterPositions`:

```go
func TestStringScannerByteAndCharacterPositions(t *testing.T) {
    core.RegisterMspec()
    _, _ = runRuby(t, `require "strscan"
s = StringScanner.new("あb")
s.getch.should == "あ"
s.pos.should == 3
s.charpos.should == 1
s.rest_size.should == 1
s.size.should == 4
copy = s.dup
copy.getch.should == "b"
s.pos.should == 3
s.inspect.should include("3/4")`)
    if failures := core.GetSpecRunner().FailCount; failures != 0 {
        t.Fatalf("expected 0 failures, got %d", failures)
    }
}
```

- [ ] **Step 2: Run it and verify RED**

Run the single-worker focused test. Expected: FAIL where current code treats byte counts as rune counts or shares mutable scanner state after `dup`.

- [ ] **Step 3: Implement byte-safe and character-safe helpers**

Add:

```go
func stringScannerByteSize(data *stringScannerData) int
func stringScannerCharacterPosition(data *stringScannerData) int
func stringScannerNextCharacterEnd(data *stringScannerData) int
func stringScannerCloneData(data *stringScannerData) *stringScannerData
```

`pos`, `size`, `rest_size`, scan lengths, and regexp spans use bytes. `charpos` counts decoded characters before `offset`. `getch` advances one encoded character; `get_byte` advances one byte. Clone all slices/maps in scanner match state while retaining the same Ruby source String reference.

- [ ] **Step 4: Normalize representation and mutation behavior**

Make `inspect` report class, byte offset, byte size, and remaining content without exposing Go types. Ensure `string` returns the original Ruby String object, `string=` replaces it and resets offset, and frozen receiver mutations raise `FrozenError` before changing state.

- [ ] **Step 5: Run focused regressions and build**

Run all four new StringScanner Go tests together, then:

```sh
GOMAXPROCS=1 GOCACHE=/tmp/rgo-go-cache GOMODCACHE=/tmp/rgo-go-mod-cache GOFLAGS=-p=1 nice -n 10 go build -o rgo ./cmd/rgo
```

Expected: Go tests pass and build exits 0.

- [ ] **Step 6: Run the complete StringScanner gate**

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 BUILD_BINARY=0 RGO_SPEC_TIMEOUT=20 nice -n 10 scripts/spec_status.sh vendor/ruby/spec/library/stringscanner /tmp/rgo-stringscanner-final.csv
awk -F, 'NR>1{status[$2]++; e+=$3; f+=$4} END{for(s in status) print s,status[s]; print "examples",e; print "failures",f}' /tmp/rgo-stringscanner-final.csv
```

Expected: 44 passing files, 249 examples, zero failures, timeouts, or runtime errors.

- [ ] **Step 7: Record verified totals**

Update `TODO.md` with the exact fresh status. If a visible behavior is correct but the MSpec runner reports a hidden count, document the smallest reproduction and continue per project rules. Do not stage or commit files.
