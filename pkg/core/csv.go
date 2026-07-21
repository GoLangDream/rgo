package core

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type csvState struct {
	source  string
	target  *object.EmeraldValue
	colSep  string
	liberal bool
}

func installCSVClass(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing, ok := objectClass.Constants["CSV"]; ok && existing != nil && existing.Type == object.ValueClass {
		return
	}
	csvClass := object.NewClass("CSV")
	csvClass.SuperClass = objectClass
	R.Classes["CSV"] = csvClass
	csvClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: csvClassNew, Arity: -1})
	csvClass.DefineClassMethod("parse", &object.Method{Name: "parse", Fn: csvClassParse, Arity: -1})
	csvClass.DefineClassMethod("generate_line", &object.Method{Name: "generate_line", Fn: csvClassGenerateLine, Arity: -1})
	csvClass.DefineClassMethod("generate_row", &object.Method{Name: "generate_row", Fn: csvClassGenerateLine, Arity: -1})
	csvClass.DefineClassMethod("generate", &object.Method{Name: "generate", Fn: csvClassGenerate, Arity: -1})
	csvClass.DefineClassMethod("read", &object.Method{Name: "read", Fn: csvClassRead, Arity: -1})
	csvClass.DefineClassMethod("readlines", &object.Method{Name: "readlines", Fn: csvClassRead, Arity: -1})
	csvClass.DefineMethod("readlines", &object.Method{Name: "readlines", Fn: csvReadlines, Arity: 0})
	csvClass.DefineMethod("liberal_parsing?", &object.Method{Name: "liberal_parsing?", Fn: csvLiberalParsing, Arity: 0})
	csvClass.DefineMethod("add_row", &object.Method{Name: "add_row", Fn: csvAddRow, Arity: 1})
	csvClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: csvAddRow, Arity: 1})

	malformed := object.NewClass("CSV::MalformedCSVError")
	malformed.SuperClass = R.Classes["StandardError"]
	R.Classes["CSV::MalformedCSVError"] = malformed
	csvClass.DefineConstant("MalformedCSVError", &object.EmeraldValue{Type: object.ValueClass, Data: malformed, Class: R.Classes["Class"]})

	for _, name := range []string{"Cell", "StreamBuf", "IOBuf", "StringReader", "IOReader", "BasicWriter", "Writer"} {
		klass := object.NewClass("CSV::" + name)
		klass.SuperClass = objectClass
		klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: csvClassNew, Arity: -1})
		if name == "Writer" {
			klass.DefineMethod("add_row", &object.Method{Name: "add_row", Fn: csvAddRow, Arity: 1})
			klass.DefineMethod("<<", &object.Method{Name: "<<", Fn: csvAddRow, Arity: 1})
		}
		R.Classes["CSV::"+name] = klass
		csvClass.DefineConstant(name, &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]})
	}

	value := &object.EmeraldValue{Type: object.ValueClass, Data: csvClass, Class: R.Classes["Class"]}
	objectClass.DefineConstant("CSV", value)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "CSV", value)
}

func csvOptions(args []*object.EmeraldValue) (string, bool, *object.EmeraldValue) {
	sep := ","
	liberal := false
	if len(args) == 0 {
		return sep, liberal, nil
	}
	opts := args[len(args)-1]
	if opts == nil || opts.Type != object.ValueHash {
		return sep, liberal, nil
	}
	values := valueToHashMap(opts)
	if value, ok := hashLookup(values, rubySymbol("col_sep")); ok && value != nil {
		switch value.Type {
		case object.ValueString:
			sep = stringRawValue(value)
		case object.ValueInteger:
			sep = string(rune(value.Data.(int64)))
		default:
			return "", false, NewTypeError("col_sep must be String")
		}
		if len([]rune(sep)) != 1 {
			return "", false, NewArgumentError("col_sep must be 1-character String")
		}
	}
	if value, ok := hashLookup(values, rubySymbol("liberal_parsing")); ok && value != nil {
		liberal = value.IsTruthy()
	}
	return sep, liberal, nil
}

func csvClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	source := args[0]
	if source == nil || source.Type != object.ValueString {
		return NewTypeError("no implicit conversion into String")
	}
	sep, liberal, errVal := csvOptions(args[1:])
	if errVal != nil {
		return errVal
	}
	class, _ := receiver.Data.(*object.Class)
	if class == nil {
		class = R.Classes["CSV"]
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &csvState{source: stringRawValue(source), colSep: sep, liberal: liberal}, Class: class}
}

func csvClassParse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueString {
		return NewArgumentError("wrong number of arguments")
	}
	sep, liberal, errVal := csvOptions(args[1:])
	if errVal != nil {
		return errVal
	}
	rows, parseErr := csvParseRows(stringRawValue(args[0]), sep, liberal)
	if parseErr != nil {
		return parseErr
	}
	return csvRowsValue(rows)
}

func csvReadlines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	state, _ := receiver.Data.(*csvState)
	if state == nil {
		return csvRowsValue(nil)
	}
	rows, errVal := csvParseRows(state.source, state.colSep, state.liberal)
	if errVal != nil {
		return errVal
	}
	return csvRowsValue(rows)
}

func csvLiberalParsing(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	state, _ := receiver.Data.(*csvState)
	return boolValue(state != nil && state.liberal)
}

func csvClassGenerateLine(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueArray {
		return NewArgumentError("wrong number of arguments")
	}
	sep, _, errVal := csvOptions(args[1:])
	if errVal != nil {
		return errVal
	}
	line, errVal := csvGenerateLine(args[0], sep)
	if errVal != nil {
		return errVal
	}
	return rubyString(line)
}

func csvGenerateLine(row *object.EmeraldValue, sep string) (string, *object.EmeraldValue) {
	values := row.Data.([]*object.EmeraldValue)
	fields := make([]string, len(values))
	for i, value := range values {
		if value == nil || value.Type == object.ValueNil {
			continue
		}
		converted := value
		if value.Type != object.ValueString {
			if CallMethod == nil {
				return "", NewTypeError("cannot convert CSV field to String")
			}
			converted = CallMethod(value, "to_s")
		}
		if converted == nil || converted.Type == object.ValueException {
			return "", converted
		}
		if converted.Type != object.ValueString {
			return "", NewTypeError("to_s must return String")
		}
		field := stringRawValue(converted)
		if strings.Contains(field, sep) || strings.ContainsAny(field, "\"\r\n") {
			field = `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
		}
		fields[i] = field
	}
	return strings.Join(fields, sep) + "\n", nil
}

func csvClassGenerate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	sep, _, errVal := csvOptions(args)
	if errVal != nil {
		return errVal
	}
	var target *object.EmeraldValue
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueString {
		target = args[0]
	} else {
		target = rubyString("")
	}
	writerClass := R.Classes["CSV::Writer"]
	writer := &object.EmeraldValue{Type: object.ValueObject, Data: &csvState{target: target, colSep: sep}, Class: writerClass}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CurrentBlockValue() != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), writer)
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return target
}

func csvAddRow(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || args[0].Type != object.ValueArray {
		return NewArgumentError("wrong number of arguments")
	}
	state, _ := receiver.Data.(*csvState)
	if state == nil || state.target == nil || state.target.Type != object.ValueString {
		return NewTypeError("CSV writer has no String target")
	}
	line, errVal := csvGenerateLine(args[0], state.colSep)
	if errVal != nil {
		return errVal
	}
	state.target.Data = stringRawValue(state.target) + line
	return receiver
}

func csvClassRead(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueString {
		return NewArgumentError("wrong number of arguments")
	}
	content, err := os.ReadFile(stringRawValue(args[0]))
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	parseArgs := append([]*object.EmeraldValue{rubyString(string(content))}, args[1:]...)
	return csvClassParse(receiver, parseArgs...)
}

func csvRowsValue(rows [][]*object.EmeraldValue) *object.EmeraldValue {
	values := make([]*object.EmeraldValue, len(rows))
	for i, row := range rows {
		values[i] = &object.EmeraldValue{Type: object.ValueArray, Data: row, Class: R.Classes["Array"]}
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func csvParseRows(source, sep string, liberal bool) ([][]*object.EmeraldValue, *object.EmeraldValue) {
	if source == "" {
		return nil, nil
	}
	delimiter := sep[0]
	rows := make([][]*object.EmeraldValue, 0)
	row := make([]*object.EmeraldValue, 0)
	var field strings.Builder
	inQuotes := false
	afterQuote := false
	quoted := false
	atFieldStart := true
	appendField := func() {
		if field.Len() == 0 && !quoted {
			row = append(row, R.NilVal)
		} else {
			row = append(row, rubyString(field.String()))
		}
		field.Reset()
		quoted = false
		afterQuote = false
		atFieldStart = true
	}
	appendRow := func() {
		if len(row) == 0 && field.Len() == 0 && !quoted {
			rows = append(rows, []*object.EmeraldValue{})
		} else {
			appendField()
			rows = append(rows, row)
		}
		row = make([]*object.EmeraldValue, 0)
		field.Reset()
		quoted = false
		afterQuote = false
		atFieldStart = true
	}
	malformed := func(message string) ([][]*object.EmeraldValue, *object.EmeraldValue) {
		return nil, newRuntimeException(R.Classes["CSV::MalformedCSVError"], message)
	}
	for i := 0; i < len(source); i++ {
		char := source[i]
		if inQuotes {
			if char == '"' {
				if i+1 < len(source) && source[i+1] == '"' {
					field.WriteByte('"')
					i++
				} else {
					inQuotes = false
					afterQuote = true
				}
			} else {
				field.WriteByte(char)
			}
			continue
		}
		if afterQuote {
			switch char {
			case delimiter:
				appendField()
			case '\n':
				appendRow()
			case '\r':
				if i+1 < len(source) && source[i+1] == '\n' {
					i++
				}
				appendRow()
			default:
				if !liberal {
					return malformed("Any value after quoted field isn't allowed")
				}
				field.WriteByte(char)
				afterQuote = false
				atFieldStart = false
			}
			continue
		}
		switch char {
		case delimiter:
			appendField()
		case '\n':
			appendRow()
		case '\r':
			if i+1 < len(source) && source[i+1] == '\n' {
				i++
			}
			appendRow()
		case '"':
			if atFieldStart {
				inQuotes = true
				quoted = true
			} else if liberal {
				field.WriteByte(char)
			} else {
				return malformed("Illegal quoting")
			}
		default:
			field.WriteByte(char)
			atFieldStart = false
		}
	}
	if inQuotes && !liberal {
		return malformed("Unclosed quoted field")
	}
	if source[len(source)-1] != '\n' && source[len(source)-1] != '\r' {
		appendRow()
	}
	return rows, nil
}
