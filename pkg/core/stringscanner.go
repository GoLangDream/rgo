package core

import (
	"fmt"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/object"
)

func stringScannerCoerceString(value *object.EmeraldValue) (*object.EmeraldValue, string, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil {
		return nil, "", NewTypeError("no implicit conversion into String")
	}
	if value.Type == object.ValueString {
		return value, value.Data.(string), nil
	}
	if CallMethod == nil || !receiverHasCallableMethod(value, "to_str") {
		return nil, "", NewTypeError("no implicit conversion into String")
	}
	converted := CallMethod(value, "to_str")
	if converted != nil && converted.Type == object.ValueException {
		return nil, "", converted
	}
	if converted == nil || converted.Type != object.ValueString {
		return nil, "", NewTypeError("can't convert into String")
	}
	return converted, converted.Data.(string), nil
}

func stringScannerMatch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := stringScannerCheck(receiver, args...)
	if result == nil || result.Type == object.ValueNil || result.Type == object.ValueException {
		return result
	}
	return newInt(int64(len(result.Data.(string))))
}

func stringScannerSkip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := stringScannerScan(receiver, args...)
	if result == nil || result.Type == object.ValueNil || result.Type == object.ValueException {
		return result
	}
	return newInt(int64(len(result.Data.(string))))
}

func stringScannerScanFull(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 3 {
		return NewArgumentError("wrong number of arguments")
	}
	var result *object.EmeraldValue
	if args[1].IsTruthy() {
		result = stringScannerScan(receiver, args[0])
	} else {
		result = stringScannerCheck(receiver, args[0])
	}
	if result == nil || result.Type == object.ValueNil || result.Type == object.ValueException {
		return result
	}
	if args[2].IsTruthy() {
		return result
	}
	return newInt(int64(len(result.Data.(string))))
}

func stringScannerRestSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil {
		return newInt(0)
	}
	return newInt(int64(len(data.str)) - data.pos)
}

func stringScannerRestQ(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data != nil && data.pos < int64(len(data.str)) {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringScannerBeginningOfLine(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil || data.pos == 0 {
		return R.TrueVal
	}
	if data.pos <= int64(len(data.str)) && data.pos > 0 && data.str[data.pos-1] == '\n' {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringScannerSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil || data.lastMatches == nil {
		return R.NilVal
	}
	return newInt(int64(len(data.lastMatches)))
}

func stringScannerCharpos(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil {
		return newInt(0)
	}
	return newInt(int64(utf8.RuneCountInString(data.str[:data.pos])))
}

func stringScannerCaptures(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil || data.lastMatches == nil {
		return R.NilVal
	}
	values := make([]*object.EmeraldValue, 0, len(data.lastMatches)-1)
	for i := 1; i < len(data.lastMatches); i++ {
		values = append(values, stringScannerCapture(data, newInt(int64(i))))
	}
	return matrixArray(values)
}

func stringScannerUnscan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil || !data.canUnscan {
		return newRuntimeException(R.Classes["StringScanner::Error"], "unscan failed")
	}
	data.pos = data.previousPos
	data.canUnscan = false
	stringScannerClearMatch(data)
	return receiver
}

func stringScannerSetString(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil && receiver.Frozen {
		return NewFrozenError("can't modify frozen StringScanner")
	}
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	source, raw, errVal := stringScannerCoerceString(args[0])
	if errVal != nil {
		return errVal
	}
	data := stringScannerDataFrom(receiver)
	data.str = raw
	data.encoding = stringEncodingName(args[0])
	data.source = source
	data.pos = 0
	data.canUnscan = false
	stringScannerClearMatch(data)
	return args[0]
}

func stringScannerInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := stringScannerDataFrom(receiver)
	if data == nil {
		return rubyString("#<StringScanner>")
	}
	if data.pos >= int64(len(data.str)) {
		return rubyString(fmt.Sprintf("#<%s fin>", receiver.Class.Name))
	}
	rest := data.str[data.pos:]
	if len(rest) > 5 {
		rest = rest[:5] + "..."
	}
	if data.pos == 0 {
		return rubyString(fmt.Sprintf("#<%s 0/%d @ %q>", receiver.Class.Name, len(data.str), rest))
	}
	consumed := data.str[:data.pos]
	if len(consumed) > 5 {
		consumed = "..." + consumed[len(consumed)-5:]
	}
	return rubyString(fmt.Sprintf("#<%s %d/%d %q @ %q>", receiver.Class.Name, data.pos, len(data.str), consumed, rest))
}

func stringScannerMustCVersion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func stringScannerInitializeCopy(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || args[0].Class != receiver.Class {
		return NewTypeError("initialize_copy should take same class object")
	}
	source := stringScannerDataFrom(args[0])
	if source == nil {
		return receiver
	}
	copyData := *source
	copyData.lastMatches = append([]string(nil), source.lastMatches...)
	if source.lastNames != nil {
		copyData.lastNames = make(map[string]int, len(source.lastNames))
		for name, index := range source.lastNames {
			copyData.lastNames[name] = index
		}
	}
	receiver.Data = &copyData
	return receiver
}
