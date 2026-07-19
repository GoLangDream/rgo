# StringScanner RubySpec Design

## Goal

Make all 44 specs under `vendor/ruby/spec/library/stringscanner` pass while preserving byte offsets, character offsets, regexp captures, and reversible scanner state.

Current baseline: 28 passing files, 16 failing files, 249 examples, 52 failures.

## Approach

Use one shared matching engine instead of fixing each public method separately. Every regexp or literal operation will produce a match result containing byte start/end offsets and capture spans. Public methods will decide only whether to advance, what value to return, and whether to preserve the previous match.

Rejected alternatives:

- Per-method patches: smaller initially, but duplicate offset/capture rules and risk inconsistent behavior.
- Delegating all behavior to Ruby code: reduces Go code but does not fit the existing native StringScanner state and regexp integration.

## State Model

Extend `stringScannerData` with:

- source string and encoding;
- current byte offset;
- previous byte offset for `unscan`;
- last full-match byte span;
- capture byte spans, including unmatched captures;
- named-capture mapping;
- a flag indicating whether the last operation produced a match.

Character positions are derived from the source encoding and byte offset. Scanning remains byte-based so binary and multibyte strings use the same advancement rules.

## Matching Flow

Introduce one internal operation that accepts:

- regexp or literal pattern;
- anchored versus forward search;
- advance versus non-advance;
- return mode: matched text, consumed length, or boolean-like result.

It translates the regexp through the existing shared regexp layer, executes against the unconsumed source, records exact spans, and updates scanner state atomically. A failed match clears match data but does not move the scanner.

`scan`, `skip`, `check`, `match?`, `scan_full`, `search_full`, `scan_until`, `check_until`, `skip_until`, and `exist?` reuse this operation.

## Public Behavior

- `matched`, `matched_size`, captures, `[]`, `values_at`, `pre_match`, and `post_match` read the recorded spans.
- `unscan` restores the previous byte offset only after a successful advancing operation; otherwise it raises `StringScanner::Error`.
- `reset`, `terminate`, string replacement, and concatenation invalidate match state consistently.
- `pos`, `charpos`, `size`, `rest_size`, `rest`, `peek`, `get_byte`, and `getch` distinguish byte counts from character counts.
- `dup`, `inspect`, and `string` preserve Ruby object identity/frozen behavior required by the specs.

## Errors and Boundaries

- Reject non-Regexp patterns where required with `TypeError`.
- Validate integer/coercible lengths and positions through existing conversion helpers.
- Preserve source encoding on returned substrings.
- Treat end-of-string and empty matches without advancing past the buffer.
- Never loop on zero-width matches.

## Verification

Use TDD in batches:

1. shared match state and anchored operations;
2. search/full operations and return modes;
3. captures, `unscan`, and state invalidation;
4. byte/character offsets, duplication, and representation.

Run focused Go regressions first, then all 44 StringScanner files sequentially with `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `nice -n 10`, and bounded per-file timeouts. No Git staging or commit is performed.
