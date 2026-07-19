package core

import (
	"math"

	"github.com/GoLangDream/rgo/pkg/object"
)

func vectorDimensionError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["Vector::ErrDimensionMismatch"], message)
}

func vectorInnerProduct(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := vectorDataFrom(receiver)
	right := vectorDataFrom(args[0])
	if right == nil {
		return NewTypeError("wrong argument type")
	}
	if len(left.objects) != len(right.objects) {
		return vectorDimensionError("dimension mismatch")
	}
	total := newInt(0)
	for i := range left.objects {
		value := right.objects[i]
		if receiverHasCallableMethod(value, "conj") && value.Class == R.Classes["Complex"] {
			value = matrixElementCall(value, "conj")
		}
		product := matrixElementCall(left.objects[i], "*", value)
		total = matrixElementCall(total, "+", product)
		if total.Type == object.ValueException {
			return total
		}
	}
	return total
}

func vectorCrossProduct(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := vectorDataFrom(receiver)
	right := vectorDataFrom(args[0])
	if right == nil {
		return NewTypeError("wrong argument type")
	}
	if len(left.objects) != 3 || len(right.objects) != 3 {
		return vectorDimensionError("dimension mismatch")
	}
	values := make([]*object.EmeraldValue, 3)
	pairs := [][4]int{{1, 2, 2, 1}, {2, 0, 0, 2}, {0, 1, 1, 0}}
	for i, pair := range pairs {
		first := matrixElementCall(left.objects[pair[0]], "*", right.objects[pair[1]])
		second := matrixElementCall(left.objects[pair[2]], "*", right.objects[pair[3]])
		values[i] = matrixElementCall(first, "-", second)
		if values[i].Type == object.ValueException {
			return values[i]
		}
	}
	return matrixVectorValue(values)
}

func vectorNormalize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := vectorDataFrom(receiver)
	inner := vectorInnerProduct(receiver, receiver)
	number, ok := matrixNumber(inner)
	if !ok || number <= 0 || len(data.objects) == 0 {
		return newRuntimeException(R.Classes["Vector::ZeroVectorError"], "zero vector")
	}
	norm := &object.EmeraldValue{Type: object.ValueFloat, Data: math.Sqrt(number), Class: R.Classes["Float"]}
	values := make([]*object.EmeraldValue, len(data.objects))
	for i, value := range data.objects {
		values[i] = matrixElementCall(value, "/", norm)
		if values[i].Type == object.ValueException {
			return values[i]
		}
	}
	return matrixVectorValue(values)
}

func matrixLUPDecomposeGeneric(data *matrixData) (*object.EmeraldValue, *object.EmeraldValue, *object.EmeraldValue, *object.EmeraldValue) {
	m, n := data.rowCount, data.colCount
	uRows := make([][]*object.EmeraldValue, m)
	lRows := make([][]*object.EmeraldValue, m)
	pRows := make([][]*object.EmeraldValue, m)
	for i := 0; i < m; i++ {
		uRows[i] = append([]*object.EmeraldValue(nil), data.objects[i]...)
		lRows[i] = make([]*object.EmeraldValue, m)
		pRows[i] = make([]*object.EmeraldValue, m)
		for j := 0; j < m; j++ {
			lRows[i][j] = newInt(0)
			pRows[i][j] = newInt(0)
		}
		lRows[i][i] = newInt(1)
		pRows[i][i] = newInt(1)
	}
	limit := m
	if n < limit {
		limit = n
	}
	for column := 0; column < limit; column++ {
		pivot := column
		for pivot < m {
			zero, errVal := matrixElementZero(uRows[pivot][column])
			if errVal != nil {
				return nil, nil, nil, errVal
			}
			if !zero {
				break
			}
			pivot++
		}
		if pivot == m {
			continue
		}
		if pivot != column {
			uRows[column], uRows[pivot] = uRows[pivot], uRows[column]
			pRows[column], pRows[pivot] = pRows[pivot], pRows[column]
			for j := 0; j < column; j++ {
				lRows[column][j], lRows[pivot][j] = lRows[pivot][j], lRows[column][j]
			}
		}
		for row := column + 1; row < m; row++ {
			factor := matrixQuotient(uRows[row][column], uRows[column][column])
			if factor.Type == object.ValueException {
				return nil, nil, nil, factor
			}
			lRows[row][column] = factor
			for j := column; j < n; j++ {
				product := matrixElementCall(factor, "*", uRows[column][j])
				uRows[row][j] = matrixElementCall(uRows[row][j], "-", product)
				if uRows[row][j].Type == object.ValueException {
					return nil, nil, nil, uRows[row][j]
				}
			}
		}
	}
	return matrixNewValue(R.Classes["Matrix"], lRows, m, m),
		matrixNewValue(R.Classes["Matrix"], uRows, m, n),
		matrixNewValue(R.Classes["Matrix"], pRows, m, m), nil
}

func matrixLUPClassNewGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	data := matrixDataFrom(args[0])
	if data == nil {
		return NewTypeError("wrong argument type")
	}
	l, u, p, errVal := matrixLUPDecomposeGeneric(data)
	if errVal != nil {
		return errVal
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &matrixLUPData{matrix: data, l: l, u: u, p: p}, Class: R.Classes["Matrix::LUPDecomposition"]}
}

func matrixLUPFromMatrix(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixLUPClassNewGeneric(nil, receiver)
}

func matrixLUPToArrayGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*matrixLUPData)
	return matrixArray([]*object.EmeraldValue{data.l, data.u, data.p})
}

func matrixLUPL(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixLUPData).l
}

func matrixLUPU(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixLUPData).u
}

func matrixLUPP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver.Data.(*matrixLUPData).p
}

func matrixLUPDeterminant(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := receiver.Data.(*matrixLUPData)
	matrix := matrixNewValue(R.Classes["Matrix"], data.matrix.objects, data.matrix.rowCount, data.matrix.colCount)
	return matrixDeterminantGeneric(matrix)
}

func matrixLUPSolve(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := receiver.Data.(*matrixLUPData)
	if data.matrix.rowCount != data.matrix.colCount {
		return matrixDimensionError("dimension mismatch")
	}
	matrix := matrixNewValue(R.Classes["Matrix"], data.matrix.objects, data.matrix.rowCount, data.matrix.colCount)
	inverse := matrixInverseGeneric(matrix)
	if inverse.Type == object.ValueException {
		return inverse
	}
	if right := matrixDataFrom(args[0]); right != nil {
		if right.rowCount != data.matrix.rowCount {
			return matrixDimensionError("dimension mismatch")
		}
		return matrixMultiplyGeneric(inverse, args[0])
	}
	if right := vectorDataFrom(args[0]); right != nil {
		if len(right.objects) != data.matrix.rowCount {
			return matrixDimensionError("dimension mismatch")
		}
		return matrixMultiplyGeneric(inverse, args[0])
	}
	return NewTypeError("wrong argument type")
}
