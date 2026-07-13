# Token Efficiency Rules Design

## Goal

Reduce token use in every project task without weakening correctness, verification, safety, or required reporting.

## Design

Extend the root `AGENTS.md` token section with concise, enforceable rules covering:

- concise answers and progress updates;
- reuse of existing context and summaries;
- targeted file discovery, bounded reads, and filtered command output;
- batched independent read-only checks and avoidance of duplicate tool calls;
- minimal relevant tests before broader verification;
- plans, documentation, web research, skills, and subagents only when required or materially useful;
- reporting deltas instead of repeating unchanged context;
- stopping investigation when sufficient evidence exists;
- correctness, safety, user requirements, and necessary verification always taking priority over token savings.

Rules will be grouped by workflow stage so agents can apply them directly. They will avoid arbitrary numeric limits that could cause incomplete work.

## Verification

Run `git diff --check -- AGENTS.md` and inspect the final diff for duplication, contradictions, and vague language.
