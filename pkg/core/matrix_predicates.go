package core

import "github.com/GoLangDream/rgo/pkg/object"

func matrixValueIsZero(value *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	result := matrixRubyEqual(value, newInt(0))
	if result != nil && result.Type == object.ValueException {
		return nil, result
	}
	return boolValue(result == R.TrueVal), nil
}

func matrixValueIsOne(value *object.EmeraldValue) bool {
	return matrixRubyEqual(value, newInt(1)) == R.TrueVal
}

func matrixEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	return boolValue(data == nil || data.rowCount == 0 || data.colCount == 0)
}

func matrixSquare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	return boolValue(data != nil && data.rowCount == data.colCount)
}

func matrixZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	for _, row := range matrixObjectRows(data) {
		for _, value := range row {
			zero, errVal := matrixValueIsZero(value)
			if errVal != nil {
				return errVal
			}
			if zero != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixRequireSquare(receiver *object.EmeraldValue) (*matrixData, *object.EmeraldValue) {
	data := matrixDataFrom(receiver)
	if data == nil || data.rowCount != data.colCount {
		return nil, matrixDimensionError("expected square matrix")
	}
	return data, nil
}

func matrixDiagonal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := 0; column < data.colCount; column++ {
			if row == column {
				continue
			}
			zero, elementErr := matrixValueIsZero(rows[row][column])
			if elementErr != nil {
				return elementErr
			}
			if zero != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixSymmetric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := row + 1; column < data.colCount; column++ {
			if equal := matrixRubyEqual(rows[row][column], rows[column][row]); equal != R.TrueVal {
				return equal
			}
		}
	}
	return R.TrueVal
}

func matrixHermitian(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := row; column < data.colCount; column++ {
			conjugate := rows[column][row]
			if receiverHasCallableMethod(conjugate, "conj") {
				conjugate = CallMethod(conjugate, "conj")
				if conjugate != nil && conjugate.Type == object.ValueException {
					return conjugate
				}
			}
			if equal := matrixRubyEqual(rows[row][column], conjugate); equal != R.TrueVal {
				return equal
			}
		}
	}
	return R.TrueVal
}

func matrixLowerTriangular(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := row + 1; column < data.colCount; column++ {
			zero, errVal := matrixValueIsZero(rows[row][column])
			if errVal != nil {
				return errVal
			}
			if zero != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixUpperTriangular(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		limit := row
		if limit > data.colCount {
			limit = data.colCount
		}
		for column := 0; column < limit; column++ {
			zero, errVal := matrixValueIsZero(rows[row][column])
			if errVal != nil {
				return errVal
			}
			if zero != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixIdentityPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := 0; column < data.colCount; column++ {
			if row == column {
				if !matrixValueIsOne(rows[row][column]) {
					return R.FalseVal
				}
			} else if zero, elementErr := matrixValueIsZero(rows[row][column]); elementErr != nil {
				return elementErr
			} else if zero != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func matrixPermutation(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	columnCounts := make([]int, data.colCount)
	for row := 0; row < data.rowCount; row++ {
		rowCount := 0
		for column := 0; column < data.colCount; column++ {
			if matrixValueIsOne(rows[row][column]) {
				rowCount++
				columnCounts[column]++
				continue
			}
			zero, elementErr := matrixValueIsZero(rows[row][column])
			if elementErr != nil {
				return elementErr
			}
			if zero != R.TrueVal {
				return R.FalseVal
			}
		}
		if rowCount != 1 {
			return R.FalseVal
		}
	}
	for _, count := range columnCounts {
		if count != 1 {
			return R.FalseVal
		}
	}
	return R.TrueVal
}

func matrixAntisymmetric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := matrixRequireSquare(receiver)
	if errVal != nil {
		return errVal
	}
	rows := matrixObjectRows(data)
	for row := 0; row < data.rowCount; row++ {
		for column := row; column < data.colCount; column++ {
			negated := CallMethod(rows[column][row], "-@")
			if negated != nil && negated.Type == object.ValueException {
				return negated
			}
			if equal := matrixRubyEqual(rows[row][column], negated); equal != R.TrueVal {
				return equal
			}
		}
	}
	return R.TrueVal
}
