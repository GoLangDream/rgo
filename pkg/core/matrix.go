package core

import (
	"fmt"

	"github.com/GoLangDream/rgo/pkg/object"
)

func matrixReceiverClass(receiver *object.EmeraldValue) *object.Class {
	if receiver != nil && receiver.Type == object.ValueClass {
		if class, ok := receiver.Data.(*object.Class); ok && classInheritsFrom(class, R.Classes["Matrix"]) {
			return class
		}
	}
	return R.Classes["Matrix"]
}

func matrixArray(values []*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func matrixDimensionError(message string) *object.EmeraldValue {
	class := R.Classes["Matrix::ErrDimensionMismatch"]
	if class == nil {
		class = R.Classes["Exception"]
	}
	return newRuntimeException(class, message)
}

func matrixNewValue(class *object.Class, rows [][]*object.EmeraldValue, rowCount, colCount int) *object.EmeraldValue {
	objects := make([][]*object.EmeraldValue, len(rows))
	legacy := make([][]float64, len(rows))
	for i, row := range rows {
		objects[i] = append([]*object.EmeraldValue(nil), row...)
		legacy[i] = make([]float64, len(row))
		for j, value := range row {
			if number, ok := matrixNumber(value); ok {
				legacy[i][j] = number
			}
		}
	}
	if class == nil {
		class = R.Classes["Matrix"]
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &matrixData{rows: legacy, objects: objects, rowCount: rowCount, colCount: colCount}, Class: class}
}

func matrixVectorValue(values []*object.EmeraldValue) *object.EmeraldValue {
	objects := append([]*object.EmeraldValue(nil), values...)
	legacy := make([]float64, len(values))
	for i, value := range values {
		if number, ok := matrixNumber(value); ok {
			legacy[i] = number
		}
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &vectorData{values: legacy, objects: objects}, Class: R.Classes["Vector"]}
}

func matrixSequence(value *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if data := vectorDataFrom(value); data != nil {
		return append([]*object.EmeraldValue(nil), data.objects...), nil
	}
	if value != nil && value.Type == object.ValueArray {
		return value.Data.([]*object.EmeraldValue), nil
	}
	if CallMethod != nil && value != nil && receiverHasCallableMethod(value, "to_ary") {
		converted := CallMethod(value, "to_ary")
		if converted != nil && converted.Type == object.ValueException {
			return nil, converted
		}
		if converted != nil && converted.Type == object.ValueArray {
			return converted.Data.([]*object.EmeraldValue), nil
		}
	}
	return nil, NewTypeError("wrong argument type")
}

func matrixClassRowsGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	rows := make([][]*object.EmeraldValue, len(args))
	width := -1
	for i, argument := range args {
		row, errVal := matrixSequence(argument)
		if errVal != nil {
			return errVal
		}
		if width < 0 {
			width = len(row)
		} else if len(row) != width {
			return matrixDimensionError("row size differs")
		}
		rows[i] = append([]*object.EmeraldValue(nil), row...)
	}
	if width < 0 {
		width = 0
	}
	return matrixNewValue(matrixReceiverClass(receiver), rows, len(rows), width)
}

func matrixClassRowsFromArray(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))
	}
	outer, errVal := matrixSequence(args[0])
	if errVal != nil {
		return errVal
	}
	copyRows := len(args) < 2 || args[1].IsTruthy()
	rows := make([][]*object.EmeraldValue, len(outer))
	sources := make([]*object.EmeraldValue, len(outer))
	width := -1
	for i, rowValue := range outer {
		row, rowErr := matrixSequence(rowValue)
		if rowErr != nil {
			return rowErr
		}
		if width < 0 {
			width = len(row)
		} else if len(row) != width {
			return matrixDimensionError("row size differs")
		}
		if copyRows {
			rows[i] = append([]*object.EmeraldValue(nil), row...)
		} else {
			rows[i] = row
			sources[i] = rowValue
		}
	}
	if width < 0 {
		width = 0
	}
	result := matrixNewValue(matrixReceiverClass(receiver), rows, len(rows), width)
	if !copyRows {
		result.Data.(*matrixData).objects = rows
		result.Data.(*matrixData).rowSources = sources
	}
	return result
}

func matrixClassColumns(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	columns, errVal := matrixSequence(args[0])
	if errVal != nil {
		return errVal
	}
	columnValues := make([][]*object.EmeraldValue, len(columns))
	height := -1
	for i, columnValue := range columns {
		column, columnErr := matrixSequence(columnValue)
		if columnErr != nil {
			return columnErr
		}
		if height < 0 {
			height = len(column)
		} else if len(column) != height {
			return matrixDimensionError("column size differs")
		}
		columnValues[i] = column
	}
	if height < 0 {
		height = 0
	}
	rows := make([][]*object.EmeraldValue, height)
	for row := 0; row < height; row++ {
		rows[row] = make([]*object.EmeraldValue, len(columns))
		for column := range columns {
			rows[row][column] = columnValues[column][row]
		}
	}
	return matrixNewValue(matrixReceiverClass(receiver), rows, height, len(columns))
}

func matrixIntegerArgument(value *object.EmeraldValue) (int, *object.EmeraldValue) {
	integer, errVal := opensslIntegerArgument(value)
	if errVal != nil {
		return 0, errVal
	}
	if integer < 0 || uint64(integer) > uint64(^uint(0)>>1) {
		return 0, NewArgumentError("invalid dimension")
	}
	return int(integer), nil
}

func matrixClassBuild(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))
	}
	rowCount, errVal := matrixIntegerArgument(args[0])
	if errVal != nil {
		return errVal
	}
	colCount := rowCount
	if len(args) == 2 {
		colCount, errVal = matrixIntegerArgument(args[1])
		if errVal != nil {
			return errVal
		}
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		data := &enumeratorData{}
		data.eachFunc = func() *object.EmeraldValue {
			if CurrentBlockValue == nil || CallBlockWithArgs == nil {
				return R.NilVal
			}
			rows := make([][]*object.EmeraldValue, rowCount)
			for row := 0; row < rowCount; row++ {
				rows[row] = make([]*object.EmeraldValue, colCount)
				for column := 0; column < colCount; column++ {
					value := CallBlockWithArgs(CurrentBlockValue(), newInt(int64(row)), newInt(int64(column)))
					if value != nil && value.Type == object.ValueException {
						return value
					}
					rows[row][column] = value
				}
			}
			return matrixNewValue(matrixReceiverClass(receiver), rows, rowCount, colCount)
		}
		return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["Enumerator"]}
	}
	rows := make([][]*object.EmeraldValue, rowCount)
	for row := 0; row < rowCount; row++ {
		rows[row] = make([]*object.EmeraldValue, colCount)
		for column := 0; column < colCount; column++ {
			value := CallBlockWithArgs(CurrentBlockValue(), newInt(int64(row)), newInt(int64(column)))
			if value != nil && value.Type == object.ValueException {
				return value
			}
			rows[row][column] = value
		}
	}
	return matrixNewValue(matrixReceiverClass(receiver), rows, rowCount, colCount)
}

func matrixClassDiagonal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	size := len(args)
	rows := make([][]*object.EmeraldValue, size)
	for i := 0; i < size; i++ {
		rows[i] = make([]*object.EmeraldValue, size)
		for j := 0; j < size; j++ {
			rows[i][j] = newInt(0)
		}
		rows[i][i] = args[i]
	}
	return matrixNewValue(matrixReceiverClass(receiver), rows, size, size)
}

func matrixClassScalar(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	size, errVal := matrixIntegerArgument(args[0])
	if errVal != nil {
		return errVal
	}
	values := make([]*object.EmeraldValue, size)
	for i := range values {
		values[i] = args[1]
	}
	return matrixClassDiagonal(receiver, values...)
}

func matrixClassIdentity(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixClassScalar(receiver, args[0], newInt(1))
}

func matrixClassZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))
	}
	rows, errVal := matrixIntegerArgument(args[0])
	if errVal != nil {
		return errVal
	}
	columns := rows
	if len(args) == 2 {
		columns, errVal = matrixIntegerArgument(args[1])
		if errVal != nil {
			return errVal
		}
	}
	values := make([][]*object.EmeraldValue, rows)
	for i := range values {
		values[i] = make([]*object.EmeraldValue, columns)
		for j := range values[i] {
			values[i][j] = newInt(0)
		}
	}
	return matrixNewValue(matrixReceiverClass(receiver), values, rows, columns)
}

func matrixClassEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..2)", len(args)))
	}
	rows, columns := 0, 0
	var errVal *object.EmeraldValue
	if len(args) >= 1 {
		rows, errVal = matrixIntegerArgument(args[0])
		if errVal != nil {
			return errVal
		}
	}
	if len(args) == 2 {
		columns, errVal = matrixIntegerArgument(args[1])
		if errVal != nil {
			return errVal
		}
	}
	if rows > 0 && columns > 0 {
		return NewArgumentError("one size must be 0")
	}
	values := make([][]*object.EmeraldValue, rows)
	for i := range values {
		values[i] = []*object.EmeraldValue{}
	}
	return matrixNewValue(matrixReceiverClass(receiver), values, rows, columns)
}

func matrixClassRowVector(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	row, errVal := matrixSequence(args[0])
	if errVal != nil {
		return errVal
	}
	return matrixNewValue(matrixReceiverClass(receiver), [][]*object.EmeraldValue{row}, 1, len(row))
}

func matrixClassColumnVector(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	column, errVal := matrixSequence(args[0])
	if errVal != nil {
		return errVal
	}
	rows := make([][]*object.EmeraldValue, len(column))
	for i, value := range column {
		rows[i] = []*object.EmeraldValue{value}
	}
	return matrixNewValue(matrixReceiverClass(receiver), rows, len(rows), 1)
}

func vectorClassValuesGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixVectorValue(args)
}
