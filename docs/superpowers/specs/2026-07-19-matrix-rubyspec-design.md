# Matrix RubySpec Compatibility Design

## Goal

Make all specs under `vendor/ruby/spec/library/matrix` pass while preserving Ruby's generic numeric behavior for Integer, Float, Rational, Complex, and user-defined arithmetic objects.

## Data Model

Replace `[][]float64` and `[]float64` with copied `*object.EmeraldValue` collections. Matrix state records rows plus explicit row/column dimensions so `0×n` and `n×0` matrices remain distinguishable. Vector state stores Ruby values unchanged. All public constructors validate rectangular dimensions and preserve subclass receiver classes where Ruby requires it.

Arithmetic uses small helpers that dispatch `+`, `-`, `*`, `/`, `**`, unary negation, comparison, conjugation, real/imaginary access, and zero checks through the VM. Primitive Integer/Float fast paths are allowed only when their returned Ruby type matches normal dispatch. No operation silently converts Rational or Complex values to Float.

## Components

- Constructors: rows, columns, build, diagonal, scalar, identity/unit/I, row/column vectors, empty matrices, and protected `new` behavior.
- Shape and access: dimensions, `[]`, rows/columns/vectors, minors, iteration modes, find/index, conversion, inspect/string/hash/equality.
- Algebra: matrix/scalar/vector addition, subtraction, multiplication, division, power, transpose/conjugate, trace, determinant, inverse, rank, rounding, real/imaginary projections, and coercion.
- Predicates: square, rectangular, empty, diagonal, symmetric, Hermitian, triangular, regular/singular, identity/unit, zero, normal, orthogonal/unitary, antisymmetric, and permutation.
- Decompositions: LUP state (`l`, `u`, `p`, determinant, solve) and the eigenvalue decomposition surface covered by RubySpec.
- Vector operations: generic equality/enumeration, inner/cross product, normalization, and dimension errors.

The existing Matrix, Vector, Scalar, LUP, and eigenvalue class names and exception hierarchy remain public. Implementation is split into focused files: `matrix.go` for installation/data/constructors, `matrix_access.go`, `matrix_algebra.go`, and `matrix_decomposition.go`.

## Error Semantics

Dimension errors use Matrix/Vector exception classes expected by RubySpec. Invalid indices return nil or raise according to the individual method contract. Arithmetic errors from element methods propagate unchanged. Singular inverse/solve and invalid constructor shapes raise the corresponding Matrix errors rather than returning partial results.

## Verification Order

1. Value-preserving constructors, dimensions, accessors, and conversions.
2. Enumeration, equality, formatting, and predicates.
3. Generic algebra, scalar coercion, Rational/Complex preservation, determinant/rank/inverse.
4. Vector, LUP, and eigenvalue decomposition behavior.
5. Full 97-file Matrix directory gate, followed by targeted Core numeric regressions.

Every batch begins with a focused failing Go regression and runs sequentially with one Go worker, `nice`, and per-file timeouts. No Git staging or commit is performed.
