package core

import (
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

func matrixObjectRows(data *matrixData) [][]*object.EmeraldValue {
	if data == nil {
		return nil
	}
	rows := data.objects
	if len(data.rowSources) == len(rows) {
		rows = append([][]*object.EmeraldValue(nil), rows...)
		for i, source := range data.rowSources {
			if source != nil && source.Type == object.ValueArray {
				rows[i] = source.Data.([]*object.EmeraldValue)
			}
		}
	}
	return rows
}

func matrixRowSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return newInt(0)
	}
	return newInt(int64(data.rowCount))
}

func matrixColumnSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return newInt(0)
	}
	return newInt(int64(data.colCount))
}

func matrixElementReference(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if len(args) == 1 && args[0] != nil && args[0].Type == object.ValueArray {
		args = args[0].Data.([]*object.EmeraldValue)
	}
	if data == nil || len(args) != 2 || args[0].Type != object.ValueInteger || args[1].Type != object.ValueInteger {
		return R.NilVal
	}
	row, column := int(args[0].Data.(int64)), int(args[1].Data.(int64))
	if row < 0 {
		row += data.rowCount
	}
	if column < 0 {
		column += data.colCount
	}
	rows := matrixObjectRows(data)
	if row < 0 || row >= data.rowCount || column < 0 || column >= data.colCount || row >= len(rows) || column >= len(rows[row]) {
		return R.NilVal
	}
	return rows[row][column]
}

func matrixRowGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil || len(args) != 1 || args[0].Type != object.ValueInteger {
		return R.NilVal
	}
	index := int(args[0].Data.(int64))
	if index < 0 {
		index += data.rowCount
	}
	rows := matrixObjectRows(data)
	if index < 0 || index >= data.rowCount || index >= len(rows) {
		return R.NilVal
	}
	values := append([]*object.EmeraldValue(nil), rows[index]...)
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for _, value := range values {
			if result := CallBlockWithArgs(CurrentBlockValue(), value); result != nil && result.Type == object.ValueException {
				return result
			}
		}
		return receiver
	}
	return matrixVectorValue(values)
}

func matrixColumnGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil || len(args) != 1 || args[0].Type != object.ValueInteger {
		return R.NilVal
	}
	index := int(args[0].Data.(int64))
	if index < 0 {
		index += data.colCount
	}
	if index < 0 || index >= data.colCount {
		return R.NilVal
	}
	rows := matrixObjectRows(data)
	values := make([]*object.EmeraldValue, data.rowCount)
	for row := 0; row < data.rowCount; row++ {
		if row >= len(rows) || index >= len(rows[row]) {
			return R.NilVal
		}
		values[row] = rows[row][index]
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for _, value := range values {
			if result := CallBlockWithArgs(CurrentBlockValue(), value); result != nil && result.Type == object.ValueException {
				return result
			}
		}
		return receiver
	}
	return matrixVectorValue(values)
}

func matrixRubyEqual(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left == right {
		return R.TrueVal
	}
	if CallMethod == nil || left == nil {
		return R.FalseVal
	}
	result := CallMethod(left, "==", right)
	if result == nil {
		return R.FalseVal
	}
	return result
}

func matrixEqualGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := matrixDataFrom(receiver)
	if len(args) != 1 {
		return R.FalseVal
	}
	right := matrixDataFrom(args[0])
	if left == nil || right == nil || left.rowCount != right.rowCount || left.colCount != right.colCount {
		return R.FalseVal
	}
	leftRows, rightRows := matrixObjectRows(left), matrixObjectRows(right)
	for row := 0; row < left.rowCount; row++ {
		for column := 0; column < left.colCount; column++ {
			equal := matrixRubyEqual(leftRows[row][column], rightRows[row][column])
			if equal != R.TrueVal {
				return equal
			}
		}
	}
	return R.TrueVal
}

func matrixEqlGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := matrixDataFrom(receiver)
	if len(args) != 1 {
		return R.FalseVal
	}
	right := matrixDataFrom(args[0])
	if left == nil || right == nil || left.rowCount != right.rowCount || left.colCount != right.colCount {
		return R.FalseVal
	}
	leftRows, rightRows := matrixObjectRows(left), matrixObjectRows(right)
	for row := 0; row < left.rowCount; row++ {
		for column := 0; column < left.colCount; column++ {
			if CallMethod == nil || CallMethod(leftRows[row][column], "eql?", rightRows[row][column]) != R.TrueVal {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func vectorEqualGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	left := vectorDataFrom(receiver)
	if left == nil || len(args) != 1 {
		return R.FalseVal
	}
	right := vectorDataFrom(args[0])
	if right == nil || len(left.objects) != len(right.objects) {
		return R.FalseVal
	}
	for i := range left.objects {
		if equal := matrixRubyEqual(left.objects[i], right.objects[i]); equal != R.TrueVal {
			return equal
		}
	}
	return R.TrueVal
}

func vectorToArray(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := vectorDataFrom(receiver)
	if data == nil {
		return matrixArray(nil)
	}
	return matrixArray(append([]*object.EmeraldValue(nil), data.objects...))
}

func matrixToArray(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return matrixArray(nil)
	}
	rows := matrixObjectRows(data)
	result := make([]*object.EmeraldValue, data.rowCount)
	for i := 0; i < data.rowCount; i++ {
		result[i] = matrixArray(append([]*object.EmeraldValue(nil), rows[i]...))
	}
	return matrixArray(result)
}

func matrixRowVectors(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	rows := matrixObjectRows(data)
	values := make([]*object.EmeraldValue, data.rowCount)
	for i := range values {
		values[i] = matrixVectorValue(rows[i])
	}
	return matrixArray(values)
}

func matrixColumnVectors(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	values := make([]*object.EmeraldValue, data.colCount)
	for column := 0; column < data.colCount; column++ {
		values[column] = matrixColumnGeneric(receiver, newInt(int64(column)))
	}
	return matrixArray(values)
}

func matrixRowsEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixRowVectors(receiver)
}

func matrixColumnsEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return matrixColumnVectors(receiver)
}

func matrixMinorBounds(value *object.EmeraldValue, size int) (int, int, bool) {
	if value == nil || value.Type != object.ValueRange {
		return 0, 0, false
	}
	rangeValue, _ := value.Data.(*object.RRange)
	if rangeValue == nil || rangeValue.StartMissing || rangeValue.EndMissing {
		return 0, 0, false
	}
	start, end := int(rangeValue.Start), int(rangeValue.End)
	if start < 0 {
		start += size
	}
	if end < 0 {
		end += size
	}
	if rangeValue.Exclusive {
		end--
	}
	if start < 0 || start >= size || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end - start + 1, true
}

func matrixMinor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return R.NilVal
	}
	rowStart, rowCount, colStart, colCount := 0, 0, 0, 0
	if len(args) == 2 {
		var ok bool
		rowStart, rowCount, ok = matrixMinorBounds(args[0], data.rowCount)
		if !ok {
			return R.NilVal
		}
		colStart, colCount, ok = matrixMinorBounds(args[1], data.colCount)
		if !ok {
			return R.NilVal
		}
	} else if len(args) == 4 {
		values := make([]int, 4)
		for i, argument := range args {
			if argument == nil || argument.Type != object.ValueInteger {
				return NewTypeError("no implicit conversion into Integer")
			}
			values[i] = int(argument.Data.(int64))
		}
		rowStart, rowCount, colStart, colCount = values[0], values[1], values[2], values[3]
		if rowStart < 0 {
			rowStart += data.rowCount
		}
		if colStart < 0 {
			colStart += data.colCount
		}
		if rowCount < 0 || colCount < 0 || rowStart < 0 || colStart < 0 || rowStart > data.rowCount || colStart > data.colCount {
			return R.NilVal
		}
		if rowStart+rowCount > data.rowCount {
			rowCount = data.rowCount - rowStart
		}
		if colStart+colCount > data.colCount {
			colCount = data.colCount - colStart
		}
	} else {
		return NewArgumentError("wrong number of arguments")
	}
	rows := matrixObjectRows(data)
	result := make([][]*object.EmeraldValue, rowCount)
	for row := 0; row < rowCount; row++ {
		result[row] = append([]*object.EmeraldValue(nil), rows[rowStart+row][colStart:colStart+colCount]...)
	}
	return matrixNewValue(receiver.Class, result, rowCount, colCount)
}

type matrixSelectedEntry struct {
	value       *object.EmeraldValue
	row, column int
}

func matrixSelectedEntries(receiver *object.EmeraldValue, args []*object.EmeraldValue) ([]matrixSelectedEntry, *object.EmeraldValue) {
	selector := "all"
	if len(args) > 1 {
		return nil, NewArgumentError("wrong number of arguments")
	}
	if len(args) == 1 {
		if args[0] == nil || args[0].Type != object.ValueSymbol {
			return nil, NewArgumentError("invalid selector")
		}
		selector = specName(args[0])
	}
	valid := map[string]bool{"all": true, "diagonal": true, "off_diagonal": true, "lower": true, "strict_lower": true, "upper": true, "strict_upper": true}
	if !valid[selector] {
		return nil, NewArgumentError("invalid selector")
	}
	data := matrixDataFrom(receiver)
	rows := matrixObjectRows(data)
	entries := []matrixSelectedEntry{}
	for row := 0; row < data.rowCount; row++ {
		for column := 0; column < data.colCount; column++ {
			include := selector == "all" ||
				(selector == "diagonal" && row == column) ||
				(selector == "off_diagonal" && row != column) ||
				(selector == "lower" && row >= column) ||
				(selector == "strict_lower" && row > column) ||
				(selector == "upper" && row <= column) ||
				(selector == "strict_upper" && row < column)
			if include {
				entries = append(entries, matrixSelectedEntry{value: rows[row][column], row: row, column: column})
			}
		}
	}
	return entries, nil
}

func matrixEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	entries, errVal := matrixSelectedEntries(receiver, args)
	if errVal != nil {
		return errVal
	}
	values := make([]*object.EmeraldValue, len(entries))
	for i, entry := range entries {
		values[i] = entry.value
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return newStaticEnumerator(values)
	}
	for _, value := range values {
		if result := CallBlockWithArgs(CurrentBlockValue(), value); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func matrixEachWithIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	entries, errVal := matrixSelectedEntries(receiver, args)
	if errVal != nil {
		return errVal
	}
	values := make([]*object.EmeraldValue, len(entries))
	for i, entry := range entries {
		values[i] = matrixArray([]*object.EmeraldValue{entry.value, newInt(int64(entry.row)), newInt(int64(entry.column))})
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return newStaticEnumerator(values)
	}
	for _, entry := range entries {
		if result := CallBlockWithArgs(CurrentBlockValue(), entry.value, newInt(int64(entry.row)), newInt(int64(entry.column))); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func matrixFindIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	selectorArgs := []*object.EmeraldValue{}
	var target *object.EmeraldValue
	useTarget := false
	if len(args) == 1 && args[0].Type == object.ValueSymbol {
		selectorArgs = args
	} else if len(args) == 1 {
		target, useTarget = args[0], true
	} else if len(args) == 2 {
		target, useTarget = args[0], true
		selectorArgs = args[1:]
	} else if len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	entries, errVal := matrixSelectedEntries(receiver, selectorArgs)
	if errVal != nil {
		return errVal
	}
	if !useTarget && (BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil) {
		values := make([]*object.EmeraldValue, len(entries))
		for i, entry := range entries {
			values[i] = entry.value
		}
		return newStaticEnumerator(values)
	}
	for _, entry := range entries {
		matched := false
		if useTarget {
			matched = matrixRubyEqual(entry.value, target) == R.TrueVal
		} else {
			result := CallBlockWithArgs(CurrentBlockValue(), entry.value)
			if result != nil && result.Type == object.ValueException {
				return result
			}
			matched = result != nil && result.IsTruthy()
		}
		if matched {
			return matrixArray([]*object.EmeraldValue{newInt(int64(entry.row)), newInt(int64(entry.column))})
		}
	}
	return R.NilVal
}

func matrixInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	if data == nil {
		return rubyString("Matrix[]")
	}
	name := "Matrix"
	if receiver.Class != nil && receiver.Class.Name != "" {
		name = receiver.Class.Name
	}
	if data.rowCount == 0 || data.colCount == 0 {
		return rubyString(name + ".empty(" + fmtInt(int64(data.rowCount)) + ", " + fmtInt(int64(data.colCount)) + ")")
	}
	rows := matrixObjectRows(data)
	parts := make([]string, data.rowCount)
	for row := 0; row < data.rowCount; row++ {
		items := make([]string, data.colCount)
		for column := 0; column < data.colCount; column++ {
			inspected := CallMethod(rows[row][column], "inspect")
			if inspected != nil && inspected.Type == object.ValueString {
				items[column] = stringRawValue(inspected)
			} else {
				items[column] = rows[row][column].Inspect()
			}
		}
		parts[row] = "[" + strings.Join(items, ", ") + "]"
	}
	return rubyString(name + "[" + strings.Join(parts, ", ") + "]")
}

func fmtInt(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buffer := [24]byte{}
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}

func matrixTransposeGeneric(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := matrixDataFrom(receiver)
	rows := matrixObjectRows(data)
	transposed := make([][]*object.EmeraldValue, data.colCount)
	for column := 0; column < data.colCount; column++ {
		transposed[column] = make([]*object.EmeraldValue, data.rowCount)
		for row := 0; row < data.rowCount; row++ {
			transposed[column][row] = rows[row][column]
		}
	}
	return matrixNewValue(receiver.Class, transposed, data.colCount, data.rowCount)
}
