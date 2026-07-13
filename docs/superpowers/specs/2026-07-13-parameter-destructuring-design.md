# Ruby Parameter Destructuring Design

## Goal

Implement Ruby-compatible nested positional parameter destructuring for methods, arrow lambdas, `lambda`/`proc` blocks, and ordinary blocks, including nested groups, leading/trailing parameters, and `*rest` captures.

## Architecture

The parser will preserve destructuring syntax as a tree instead of discarding parenthesized parameters. AST parameter entries remain the top-level physical arguments used for arity, while an optional pattern tree describes how one physical argument expands into named local variables.

The compiler will define every named leaf as a local and translate AST patterns into runtime-only `object.ParameterPattern` metadata on `object.Function`. The VM will bind ordinary arguments first, then recursively destructure patterned slots before executing the body. This keeps parsing, local allocation, and runtime coercion separate and lets methods and all block/lambda forms share one binder.

## Data Model

`ast.ParameterPattern` and `object.ParameterPattern` each contain:

- ordered child entries;
- an optional leaf name;
- an optional rest child and its insertion index;
- nesting through child patterns.

Each function stores patterns aligned with `Function.Params`. A nil entry means ordinary binding. A patterned entry consumes exactly one top-level positional argument for arity purposes.

## Binding Semantics

- An Array is destructured directly.
- A non-Array is converted with `to_ary` when available; nil from `to_ary` means a single-element sequence containing the original value; a non-Array result raises `TypeError`.
- Without `to_ary`, the value is treated as a single-element sequence.
- Missing entries bind nil.
- Extra entries are ignored unless captured by `*rest`.
- `*rest` captures the middle segment after reserving trailing children.
- Nested child patterns recursively apply the same rules.
- Anonymous rest captures values without defining a local.
- Lambda arity counts each top-level destructuring pattern as one argument; nested leaves do not affect arity.

## Parser Scope

The same recursive parameter-pattern parser will be used by:

- `def m((a, *b))`;
- `-> ((a, *b)) { ... }`;
- `lambda { |(a, *b)| ... }` and proc/block forms.

It will retain existing duplicate-name validation across all leaves and preserve ordinary optional, keyword, rest, and block parameters outside the pattern.

## Error Handling

Malformed or unclosed patterns remain parser errors. Multiple rest entries in one pattern are rejected. Runtime coercion errors propagate as Ruby exceptions through the normal call path.

## Verification

TDD coverage will include parser tree shape, simple/nested method binding, lambda arity, pre/rest/post binding, scalar and Array inputs, `to_ary`, missing values, and anonymous rest. Completion evidence for this slice is:

- focused parser/compiler/VM regressions pass;
- `vendor/ruby/spec/language/method_spec.rb` passes;
- `vendor/ruby/spec/language/lambda_spec.rb` passes;
- the refreshed language dashboard shows no regressions in previously passing files.

## Scope Boundaries

This change does not alter keyword destructuring, pattern matching, or multiple assignment outside parameter lists. Those continue through their existing implementations.
