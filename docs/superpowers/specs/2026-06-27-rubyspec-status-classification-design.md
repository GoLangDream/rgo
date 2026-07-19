# RubySpec Status Classification Design

## Goal

Make the full RubySpec progress report trustworthy before implementing broad compatibility fixes. The current full report shows 3809 files, 2391 pass, and 1418 non-pass, but some differences from older focused reports indicate that several statuses need better classification before they should drive runtime work.

This phase does not aim to make every RubySpec pass. It aims to turn the full report into an actionable backlog that separates true Ruby behavior gaps from harness, guard, timeout, and reporting artifacts.

## Scope

This slice focuses on the RubySpec harness and reporting path:

- `scripts/spec_status.sh`
- `scripts/full_spec_gate.sh`
- `reports/spec-status/` outputs
- optional small helper scripts under `scripts/` if they keep the classification logic simple
- `TODO.md` updates for confirmed blockers

Runtime changes in `pkg/core`, `pkg/vm`, parser, compiler, or CLI are out of scope unless a reporting bug cannot be diagnosed without a tiny focused probe. If runtime bugs are discovered, record them in `TODO.md` and keep this phase moving.

## Current Baseline

Latest full report: `reports/spec-status/ruby-spec-full.csv`, generated on 2026-06-27.

- Total files: 3809
- Pass: 2391
- Non-pass: 1418
- Examples: 30201
- Failures: 3999
- Status counts: 699 `nonzero_failures`, 680 `zero_examples`, 34 `runtime_error`, 5 `timeout`

Older focused reports exist for `language` and `core/array`, but the full report must be the source of truth. Focused reports can be used to identify drift, not to override the latest full gate.

## Classification Model

Keep the existing high-level `status` column for compatibility, and add or derive a more specific reason classification.

Primary statuses remain:

- `pass`
- `nonzero_failures`
- `zero_examples`
- `runtime_error`
- `parse_error`
- `compile_error`
- `timeout`
- `oom_or_killed`

New reason categories should distinguish:

- `guarded_out`: examples intentionally skipped by `ruby_version_is`, `platform_is`, feature guards, or similar RubySpec guards.
- `mspec_not_expanded`: zero examples caused by missing or broken MSpec DSL support, shared examples, or helper loading.
- `load_error`: required helper, fixture, or library failed before examples ran.
- `runtime_exception`: RGo raised an unhandled runtime error while executing examples.
- `assertion_failures`: examples ran and failed normally.
- `hang`: process exceeded timeout.
- `infrastructure_error`: invalid environment, memory limit, script failure, or report-generation problem.

The classifier should prefer explicit evidence from logs over guessing from file path. Unknown cases should remain `unknown` rather than being forced into a misleading bucket.

## Reporting Changes

The report should keep the current CSV fields and add fields only if this can be done without breaking existing scripts. If adding columns is risky, create a second derived report such as `reports/spec-status/ruby-spec-full-classified.csv`.

Recommended derived columns:

- `file`
- `status`
- `reason`
- `examples`
- `failures`
- `top_dir`
- `sub_dir`
- `duration_ms`
- `log`

The summary should include:

- counts by `status`
- counts by `reason`
- counts by top directory and reason
- timeout list
- largest failure clusters by subdirectory
- zero-example breakdown

## Data Flow

1. `full_spec_gate.sh --ruby-only` runs `spec_status.sh` over `vendor/ruby/spec`.
2. `spec_status.sh` stores logs for non-pass files.
3. A classifier reads the CSV and logs.
4. The classifier writes a classified CSV and a Markdown summary.
5. `TODO.md` records only confirmed runtime or harness blockers that need later implementation work.

## Error Handling

Classification must be conservative:

- Missing logs produce `reason=unknown` rather than failing the whole summary.
- Malformed rows produce `reason=infrastructure_error` and are counted separately.
- Timeouts remain visible even if a partial summary line exists.
- `zero_examples` is not automatically considered success or failure until classified.

## Testing

Use script-level tests before changing report behavior:

- Extend `scripts/spec_status_test.sh` or add a focused classifier test script.
- Cover pass, nonzero failures, runtime error, timeout, zero examples from guard, zero examples from MSpec/load failure, and missing-log cases.
- Run `scripts/spec_status_test.sh`.
- Run the classifier against the existing `ruby-spec-full.csv` and logs.
- Run a small real target such as `vendor/ruby/spec/language/it_parameter_spec.rb` or one `core/array/pack` file to confirm logs still line up with rows.

Full RubySpec reruns are useful but not required for every edit because they are expensive. A fresh full run is required before claiming a new global progress number.

## Completion Criteria

This phase is complete when:

- The full RubySpec report has an accompanying classified report.
- `zero_examples` is split into actionable reason categories where logs contain enough evidence.
- Timeout and runtime-error files are listed explicitly.
- The summary identifies the next implementation backlog by true runtime capability gaps, not raw status alone.
- Script tests for the classifier/reporting behavior pass.
- Any discovered runtime bugs are recorded in `TODO.md` rather than fixed inside this phase.

## Out Of Scope

- Making all RubySpec files pass.
- Implementing socket, net/http, integer bignum, string encoding, IO, or module semantics.
- Rails gate work.
- Large refactors of `pkg/core/init.go` or `pkg/vm/executor.go`.
