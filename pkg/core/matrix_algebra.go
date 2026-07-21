package core

import "github.com/GoLangDream/rgo/pkg/object"

func matrixResultClass(receiver *object.EmeraldValue) *object.Class {
	if receiver != nil && receiver.Class != nil && classInheritsFrom(receiver.Class, R.Classes["Matrix"]) {
		return receiver.Class
	}
	return R.Classes["Matrix"]
}

func matrixElementCall(value *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := callMethodPropagatingException(value, method, args...)
	if result == nil {
		return NewTypeError("matrix element does not support " + method)
	}
	return result
}

func matrixElementZero(value *object.EmeraldValue) (bool, *object.EmeraldValue) {
	result := matrixRubyEqual(value, newInt(0))
	if result.Type == object.ValueException {
		return false, result
	}
	return result.IsTruthy(), nil
}

func matrixQuotient(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && left.Class == R.Classes["Integer"] {
		return matrixElementCall(left, "quo", right)
	}
	return matrixElementCall(left, "/", right)
}

func matrixMap(receiver *object.EmeraldValue, fn func(*object.EmeraldValue) *object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return NewTypeError("wrong argument type")
	}
	rows := make([][]*object.EmeraldValue, data.rowCount)
	for i := 0; i < data.rowCount; i++ {
		rows[i] = make([]*object.EmeraldValue, data.colCount)
		for j := 0; j < data.colCount; j++ {
			rows[i][j] = fn(data.objects[i][j])
			if rows[i][j] != nil && rows[i][j].Type == object.ValueException {
				return rows[i][j]
			}
		}
	}
	return matrixNewValue(matrixResultClass(receiver), rows, data.rowCount, data.colCount)
}

func matrixBinary(receiver *object.EmeraldValue, other *object.EmeraldValue, method string) *object.EmeraldValue {
	left := matrixDataFrom(receiver)
	right := matrixDataFrom(other)
	if right == nil {
		if other != nil && other.Class != nil && classInheritsFrom(other.Class, R.Classes["Numeric"]) {
			class := R.Classes["Matrix::ErrOperationNotDefined"]
			if class == nil {
				class = R.Classes["Exception"]
			}
			return newRuntimeException(class, "operation not defined")
		}
		return NewTypeError("wrong argument type")
	}
	if left.rowCount != right.rowCount || left.colCount != right.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	rows := make([][]*object.EmeraldValue, left.rowCount)
	for i := 0; i < left.rowCount; i++ {
		rows[i] = make([]*object.EmeraldValue, left.colCount)
		for j := 0; j < left.colCount; j++ {
			rows[i][j] = matrixElementCall(left.objects[i][j], method, right.objects[i][j])
			if rows[i][j].Type == object.ValueException {
				return rows[i][j]
			}
		}
	}
	return matrixNewValue(matrixResultClass(receiver), rows, left.rowCount, left.colCount)
}

func matrixAddGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixBinary(receiver, args[0], "+")
}

func matrixSubtractGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixBinary(receiver, args[0], "-")
}

func matrixDot(left []*object.EmeraldValue, right []*object.EmeraldValue) *object.EmeraldValue {
	sum := newInt(0)
	for i := range left {
		product := matrixElementCall(left[i], "*", right[i])
		if product.Type == object.ValueException {
			return product
		}
		sum = matrixElementCall(sum, "+", product)
		if sum.Type == object.ValueException {
			return sum
		}
	}
	return sum
}

func matrixMultiplyGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := matrixDataFrom(receiver)
	other := args[0]
	if right := matrixDataFrom(other); right != nil {
		if left.colCount != right.rowCount {
			return matrixDimensionError("dimension mismatch")
		}
		rows := make([][]*object.EmeraldValue, left.rowCount)
		for i := 0; i < left.rowCount; i++ {
			rows[i] = make([]*object.EmeraldValue, right.colCount)
			for j := 0; j < right.colCount; j++ {
				column := make([]*object.EmeraldValue, right.rowCount)
				for k := 0; k < right.rowCount; k++ {
					column[k] = right.objects[k][j]
				}
				rows[i][j] = matrixDot(left.objects[i], column)
				if rows[i][j].Type == object.ValueException {
					return rows[i][j]
				}
			}
		}
		return matrixNewValue(matrixResultClass(receiver), rows, left.rowCount, right.colCount)
	}
	if vector := vectorDataFrom(other); vector != nil {
		if left.colCount != len(vector.objects) {
			return matrixDimensionError("dimension mismatch")
		}
		values := make([]*object.EmeraldValue, left.rowCount)
		for i := 0; i < left.rowCount; i++ {
			values[i] = matrixDot(left.objects[i], vector.objects)
			if values[i].Type == object.ValueException {
				return values[i]
			}
		}
		return matrixVectorValue(values)
	}
	if other == nil || other.Class == nil || !classInheritsFrom(other.Class, R.Classes["Numeric"]) {
		return NewTypeError("wrong argument type")
	}
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return matrixElementCall(value, "*", other)
	})
}

func matrixDivideGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if matrixDataFrom(args[0]) != nil {
		inverse := matrixInverseGeneric(args[0])
		if inverse.Type == object.ValueException {
			return inverse
		}
		return matrixMultiplyGeneric(receiver, inverse)
	}
	other := args[0]
	if other == nil || other.Class == nil || !classInheritsFrom(other.Class, R.Classes["Numeric"]) {
		return NewTypeError("wrong argument type")
	}
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return matrixElementCall(value, "/", other)
	})
}

func matrixIdentityFor(receiver *object.EmeraldValue, size int) *object.EmeraldValue {
	rows := make([][]*object.EmeraldValue, size)
	for i := range rows {
		rows[i] = make([]*object.EmeraldValue, size)
		for j := range rows[i] {
			rows[i][j] = newInt(0)
		}
		rows[i][i] = newInt(1)
	}
	return matrixNewValue(matrixResultClass(receiver), rows, size, size)
}

func matrixPowerGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	if args[0] == nil || args[0].Type != object.ValueInteger {
		return matrixPowerReal(receiver, args[0])
	}
	power := args[0].Data.(int64)
	base := receiver
	if power < 0 {
		base = matrixInverseGeneric(receiver)
		if base.Type == object.ValueException {
			return base
		}
		power = -power
	}
	result := matrixIdentityFor(receiver, data.rowCount)
	for power > 0 {
		if power&1 == 1 {
			result = matrixMultiplyGeneric(result, base)
			if result.Type == object.ValueException {
				return result
			}
		}
		power >>= 1
		if power > 0 {
			base = matrixMultiplyGeneric(base, base)
			if base.Type == object.ValueException {
				return base
			}
		}
	}
	return result
}

func matrixDeterminantRows(rows [][]*object.EmeraldValue) *object.EmeraldValue {
	n := len(rows)
	if n == 0 {
		return newInt(1)
	}
	if n == 1 {
		return rows[0][0]
	}
	total := newInt(0)
	for column := 0; column < n; column++ {
		minor := make([][]*object.EmeraldValue, n-1)
		for i := 1; i < n; i++ {
			minor[i-1] = make([]*object.EmeraldValue, 0, n-1)
			for j := 0; j < n; j++ {
				if j != column {
					minor[i-1] = append(minor[i-1], rows[i][j])
				}
			}
		}
		term := matrixElementCall(rows[0][column], "*", matrixDeterminantRows(minor))
		if column&1 == 1 {
			term = matrixElementCall(newInt(0), "-", term)
		}
		total = matrixElementCall(total, "+", term)
		if total.Type == object.ValueException {
			return total
		}
	}
	return total
}

func matrixDeterminantGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	return matrixDeterminantRows(data.objects)
}

func matrixInverseGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	n := data.rowCount
	work := make([][]*object.EmeraldValue, n)
	for i := 0; i < n; i++ {
		work[i] = make([]*object.EmeraldValue, 2*n)
		copy(work[i], data.objects[i])
		for j := 0; j < n; j++ {
			work[i][n+j] = newInt(0)
		}
		work[i][n+i] = newInt(1)
	}
	for column := 0; column < n; column++ {
		pivot := column
		for pivot < n {
			zero, errVal := matrixElementZero(work[pivot][column])
			if errVal != nil {
				return errVal
			}
			if !zero {
				break
			}
			pivot++
		}
		if pivot == n {
			return newRuntimeException(R.Classes["Matrix::ErrNotRegular"], "not regular")
		}
		work[column], work[pivot] = work[pivot], work[column]
		pivotValue := work[column][column]
		for j := 0; j < 2*n; j++ {
			work[column][j] = matrixQuotient(work[column][j], pivotValue)
			if work[column][j].Type == object.ValueException {
				return work[column][j]
			}
		}
		for i := 0; i < n; i++ {
			if i == column {
				continue
			}
			factor := work[i][column]
			for j := 0; j < 2*n; j++ {
				product := matrixElementCall(factor, "*", work[column][j])
				work[i][j] = matrixElementCall(work[i][j], "-", product)
				if work[i][j].Type == object.ValueException {
					return work[i][j]
				}
			}
		}
	}
	rows := make([][]*object.EmeraldValue, n)
	for i := range rows {
		rows[i] = append([]*object.EmeraldValue(nil), work[i][n:]...)
	}
	return matrixNewValue(matrixResultClass(receiver), rows, n, n)
}

func matrixTraceGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	total := newInt(0)
	for i := 0; i < data.rowCount; i++ {
		total = matrixElementCall(total, "+", data.objects[i][i])
		if total.Type == object.ValueException {
			return total
		}
	}
	return total
}

func matrixRankGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	rows := make([][]*object.EmeraldValue, data.rowCount)
	for i := range rows {
		rows[i] = append([]*object.EmeraldValue(nil), data.objects[i]...)
	}
	rank, pivotRow := 0, 0
	for column := 0; column < data.colCount && pivotRow < data.rowCount; column++ {
		pivot := pivotRow
		for pivot < data.rowCount {
			zero, errVal := matrixElementZero(rows[pivot][column])
			if errVal != nil {
				return errVal
			}
			if !zero {
				break
			}
			pivot++
		}
		if pivot == data.rowCount {
			continue
		}
		rows[pivotRow], rows[pivot] = rows[pivot], rows[pivotRow]
		for i := pivotRow + 1; i < data.rowCount; i++ {
			factor := matrixQuotient(rows[i][column], rows[pivotRow][column])
			for j := column; j < data.colCount; j++ {
				product := matrixElementCall(factor, "*", rows[pivotRow][j])
				rows[i][j] = matrixElementCall(rows[i][j], "-", product)
			}
		}
		rank++
		pivotRow++
	}
	return newInt(int64(rank))
}

func matrixConjugateGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		if receiverHasCallableMethod(value, "conj") {
			return matrixElementCall(value, "conj")
		}
		return value
	})
}

func matrixRealGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return matrixElementCall(value, "real")
	})
}

func matrixImaginaryGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return matrixElementCall(value, "imaginary")
	})
}

func matrixRectangularGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixArray([]*object.EmeraldValue{
		matrixRealGeneric(receiver),
		matrixImaginaryGeneric(receiver),
	})
}

func matrixRealPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	complexClass := R.Classes["Complex"]
	for _, row := range data.objects {
		for _, value := range row {
			if complexClass != nil && value.Class != nil && classInheritsFrom(value.Class, complexClass) {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixRoundGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return matrixElementCall(value, "round", args...)
	})
}

func matrixCollectGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		values := make([]*object.EmeraldValue, 0, data.rowCount*data.colCount)
		for _, row := range data.objects {
			values = append(values, row...)
		}
		return newStaticEnumerator(values)
	}
	return matrixMap(receiver, func(value *object.EmeraldValue) *object.EmeraldValue {
		return CallBlockWithArgs(CurrentBlockValue(), value)
	})
}

func matrixRegularGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	determinant := matrixDeterminantGeneric(receiver)
	if determinant.Type == object.ValueException {
		return determinant
	}
	zero, errVal := matrixElementZero(determinant)
	if errVal != nil {
		return errVal
	}
	return boolValue(!zero)
}

func matrixSingularGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	regular := matrixRegularGeneric(receiver)
	if regular.Type == object.ValueException {
		return regular
	}
	return boolValue(!regular.IsTruthy())
}

func matrixAdjointValue(receiver *object.EmeraldValue) *object.EmeraldValue {
	return matrixTransposeGeneric(matrixConjugateGeneric(receiver))
}

func matrixNormalGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	adjoint := matrixAdjointValue(receiver)
	left := matrixMultiplyGeneric(receiver, adjoint)
	right := matrixMultiplyGeneric(adjoint, receiver)
	return matrixEqualGeneric(left, right)
}

func matrixOrthogonalGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	product := matrixMultiplyGeneric(matrixTransposeGeneric(receiver), receiver)
	return matrixEqualGeneric(product, matrixIdentityFor(receiver, data.rowCount))
}

func matrixUnitaryGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data.rowCount != data.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	product := matrixMultiplyGeneric(matrixAdjointValue(receiver), receiver)
	return matrixEqualGeneric(product, matrixIdentityFor(receiver, data.rowCount))
}

func matrixCoerceGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	other := args[0]
	if data.rowCount != data.colCount || other == nil || other.Class == nil || !classInheritsFrom(other.Class, R.Classes["Numeric"]) {
		return NewTypeError("can't coerce")
	}
	rows := make([][]*object.EmeraldValue, data.rowCount)
	for i := range rows {
		rows[i] = make([]*object.EmeraldValue, data.colCount)
		for j := range rows[i] {
			rows[i][j] = newInt(0)
		}
		rows[i][i] = other
	}
	left := matrixNewValue(matrixResultClass(receiver), rows, data.rowCount, data.colCount)
	return matrixArray([]*object.EmeraldValue{left, receiver})
}

func matrixHashGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	array := matrixToArray(receiver)
	hash := matrixElementCall(array, "hash")
	if hash.Type == object.ValueException {
		return hash
	}
	return hash
}
