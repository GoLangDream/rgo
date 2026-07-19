# Matrix RubySpec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the float-only Matrix shim with value-preserving Matrix, Vector, Scalar, LUP, and eigenvalue behavior sufficient for all 97 repository Matrix specs.

**Architecture:** Move Matrix code out of `pkg/core/init.go` into focused files. Store Ruby values unchanged and route element arithmetic through VM method dispatch; keep numeric fast paths only where they preserve Ruby result types. Build access, algebra, predicates, and decompositions in independently verified batches.

**Tech Stack:** Go 1.24, RGo object model/VM dispatch, RubySpec.

---

### Task 1: Value-preserving model, constructors, and access

**Files:**
- Create: `pkg/core/matrix.go`
- Create: `pkg/core/matrix_access.go`
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestMatrixPreservesRubyValuesAndShapes` covering Rational/Complex elements, `rows`, `columns`, `build`, `diagonal`, `scalar`, identity/empty constructors, row/column sizes, `[]`, minor, vectors, and `to_a` copy isolation.
- [ ] Run the focused test with one worker; expect failures from missing constructors and Float conversion.
- [ ] Replace the old structs with:

```go
type matrixData struct {
	rows     [][]*object.EmeraldValue
	rowCount int
	colCount int
}
type vectorData struct { values []*object.EmeraldValue }
```

Copy only row slices, never element values. Add `matrixNewValue(class, rows, rowCount, colCount)` and validate equal row widths.
- [ ] Register exact constructor methods `[]`, `rows`, `columns`, `build`, `diagonal`, `scalar`, `identity`, `unit`, `I`, `zero`, `empty`, `row_vector`, and `column_vector`. Block constructors invoke `CallBlockWithArgs` using integer indices.
- [ ] Implement `row_size`, `column_size`, `[]`, `row`, `column`, `rows`, `columns`, `row_vectors`, `column_vectors`, `minor`, and `to_a`, preserving negative-index and range contracts from their specs.
- [ ] Run constructor/access RubySpec files as one sequential batch; expect zero failures.

### Task 2: Enumeration, representation, equality, and predicates

**Files:**
- Modify: `pkg/core/matrix_access.go`
- Create: `pkg/core/matrix_predicates.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestMatrixEnumerationFormattingAndPredicates` covering `each`/`each_with_index` modes, `find_index`, inspect/string/hash, value/eql equality, and square/empty/zero/diagonal/symmetric/Hermitian/triangular/identity/permutation predicates.
- [ ] Run it and confirm failures for the unregistered interfaces.
- [ ] Implement Ruby truth/equality helpers through dispatch:

```go
func matrixCall(value *object.EmeraldValue, name string, args ...*object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	result := CallMethod(value, name, args...)
	if result != nil && result.Type == object.ValueException { return nil, result }
	return result, nil
}
```

Use `zero?`, `==`, `eql?`, `conj`, `real`, and `imaginary` rather than Float conversion.
- [ ] Implement enumerator materialization for `:all`, `:diagonal`, `:off_diagonal`, `:lower`, `:strict_lower`, `:upper`, and `:strict_upper`; `each_with_index` yields value,row,column.
- [ ] Implement stable `to_s`, `inspect`, `hash`, `==`, `eql?`, and all predicate aliases listed in the design. Composite predicates reuse transpose/conjugate/equality helpers.
- [ ] Run all access/enumeration/representation/predicate spec files; expect zero failures.

### Task 3: Generic Matrix algebra and Scalar coercion

**Files:**
- Create: `pkg/core/matrix_algebra.go`
- Modify: `pkg/core/matrix.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestMatrixAlgebraPreservesRationalAndComplex` covering `+`, `-`, matrix/vector/scalar `*`, `/`, integer `**`, transpose/conjugate, trace/determinant/rank/inverse, round, real/imaginary, and Scalar left-hand coercion.
- [ ] Run it and confirm missing-method failures.
- [ ] Implement element helpers `matrixAdd`, `matrixSub`, `matrixMul`, `matrixDiv`, `matrixPow`, and `matrixNegate` by `CallMethod`; initialize sums with Ruby integer zero and propagate exceptions unchanged.
- [ ] Add shape-checked addition/subtraction, standard matrix product, matrix-vector product, scalar product/division, exponentiation by squaring, transpose/conjugate transpose, trace, and projections.
- [ ] Implement determinant and inverse using value-preserving elimination. Pivot selection uses `zero?`; division remains Ruby division. Rank uses the same elimination without converting elements to Float.
- [ ] Define `Matrix::Scalar` with `value`, `+`, `-`, `*`, `/`, `**`, and `coerce`; install Matrix dimension/singularity exception classes and raise them at every shape/singular boundary.
- [ ] Run algebra and Scalar spec groups sequentially; expect zero failures.

### Task 4: Vector and LUP behavior

**Files:**
- Modify: `pkg/core/matrix.go`
- Modify: `pkg/core/matrix_algebra.go`
- Create: `pkg/core/matrix_decomposition.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestVectorAndMatrixLUPGenericArithmetic` covering Vector equality/each2/inner/cross/normalize and LUP `l`, `u`, `p`, determinant, solve, and `to_a` reconstruction.
- [ ] Run it and confirm failures outside the existing float-only `to_a` path.
- [ ] Implement Vector methods over Ruby values and raise `Vector::ErrDimensionMismatch` for mismatched operations. Normalize divides each component by `sqrt(inner_product(self))` through Ruby dispatch.
- [ ] Implement generic partial-pivot LUP state containing copied Ruby-value L/U matrices and permutation indices. Add readers, determinant parity, forward/back substitution, and Matrix/Vector right-hand solve.
- [ ] Run all Vector and LUP spec files; expect zero failures.

### Task 5: Eigenvalue surface and complete gate

**Files:**
- Modify: `pkg/core/matrix_decomposition.go`
- Modify: `TODO.md`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestMatrixEigenvalueDecompositionCoveredCases` for `[[1,2],[2,1]]`, `[[14,16],[-6,-6]]`, and rotation `[[1,1],[-1,1]]`, checking real/complex eigenvalues, exact fixture eigenvectors, eigenvalue/eigenvector matrices, and `to_a` reconstruction.
- [ ] Implement analytic 2×2 eigenvalues/eigenvectors, including Complex conjugate roots for negative discriminants, and deterministic ordering matching the fixtures. For larger square matrices use a strictly iteration-bounded real QR fallback; the covered 5×5 constructor must return without hanging. Preserve Matrix/Vector types and expose `v`, `d`, `v_inv`, `eigenvalues`, `eigenvectors`, `eigenvalue_matrix`, `eigenvector_matrix`, and `to_a`.
- [ ] Run all six eigenvalue files; expect zero failures.
- [ ] Run all Matrix-focused Go regressions, build with one worker, then execute:

```sh
env BUILD_BINARY=0 RGO_SPEC_TIMEOUT=30 GOMAXPROCS=1 GOFLAGS=-p=1 nice -n 10 timeout 300s scripts/spec_status.sh vendor/ruby/spec/library/matrix /tmp/rgo-matrix-final.csv
```

Expected: 97 files, zero failures, no runtime errors or timeouts; version-guard-only files may remain `zero_examples`.
- [ ] Run Rational, Complex, Array, and Enumerable focused regressions to verify shared Ruby-value dispatch. Record exact totals in `TODO.md`; do not stage or commit files.
