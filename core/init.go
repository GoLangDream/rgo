package core

import (
	"fmt"

	"github.com/GoLangDream/rgo/vm/object"
)

type BuiltinMethod func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue

type Runtime struct {
	Classes map[string]*object.Class

	TrueVal  *object.EmeraldValue
	FalseVal *object.EmeraldValue
	NilVal   *object.EmeraldValue

	Main *object.EmeraldValue
}

var R *Runtime

func Init() {
	R = &Runtime{
		Classes: make(map[string]*object.Class),
	}

	R.TrueVal = &object.EmeraldValue{
		Type:  object.ValueBool,
		Data:  true,
		Class: nil,
	}

	R.FalseVal = &object.EmeraldValue{
		Type:  object.ValueBool,
		Data:  false,
		Class: nil,
	}

	R.NilVal = &object.EmeraldValue{
		Type:  object.ValueNil,
		Data:  nil,
		Class: nil,
	}

	R.createClasses()
	R.defineMethods()
}

func (rt *Runtime) createClasses() {
	basicObject := object.NewClass("BasicObject")
	objectClass := object.NewClass("Object")
	objectClass.SuperClass = basicObject
	moduleClass := object.NewClass("Module")
	moduleClass.SuperClass = objectClass
	classClass := object.NewClass("Class")
	classClass.SuperClass = moduleClass

	trueClass := object.NewClass("TrueClass")
	trueClass.SuperClass = objectClass
	falseClass := object.NewClass("FalseClass")
	falseClass.SuperClass = objectClass
	nilClass := object.NewClass("NilClass")
	nilClass.SuperClass = objectClass

	integerClass := object.NewClass("Integer")
	integerClass.SuperClass = objectClass
	floatClass := object.NewClass("Float")
	floatClass.SuperClass = objectClass
	stringClass := object.NewClass("String")
	stringClass.SuperClass = objectClass
	arrayClass := object.NewClass("Array")
	arrayClass.SuperClass = objectClass
	hashClass := object.NewClass("Hash")
	hashClass.SuperClass = objectClass
	symbolClass := object.NewClass("Symbol")
	symbolClass.SuperClass = objectClass
	regexpClass := object.NewClass("Regexp")
	regexpClass.SuperClass = objectClass
	rangeClass := object.NewClass("Range")
	rangeClass.SuperClass = objectClass

	R.TrueVal.Class = trueClass
	R.FalseVal.Class = falseClass
	R.NilVal.Class = nilClass

	R.Classes["BasicObject"] = basicObject
	R.Classes["Object"] = objectClass
	R.Classes["Module"] = moduleClass
	R.Classes["Class"] = classClass
	R.Classes["TrueClass"] = trueClass
	R.Classes["FalseClass"] = falseClass
	R.Classes["NilClass"] = nilClass
	R.Classes["Integer"] = integerClass
	R.Classes["Float"] = floatClass
	R.Classes["String"] = stringClass
	R.Classes["Array"] = arrayClass
	R.Classes["Hash"] = hashClass
	R.Classes["Symbol"] = symbolClass
	R.Classes["Regexp"] = regexpClass
	R.Classes["Range"] = rangeClass
}

func (rt *Runtime) defineMethods() {
	objectClass := R.Classes["Object"]
	objectClass.DefineMethod("class", &object.Method{Name: "class", Fn: methodClass, Arity: 0})
	objectClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: methodToS, Arity: 0})
	objectClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: methodInspect, Arity: 0})
	objectClass.DefineMethod("nil?", &object.Method{Name: "nil?", Fn: methodIsNil, Arity: 0})
	objectClass.DefineMethod("equal?", &object.Method{Name: "equal?", Fn: methodEqual, Arity: 1})
	objectClass.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: methodEql, Arity: 1})
	objectClass.DefineMethod("respond_to?", &object.Method{Name: "respond_to?", Fn: methodRespondTo, Arity: 1})
	objectClass.DefineMethod("send", &object.Method{Name: "send", Fn: methodSend, Arity: 1})
	objectClass.DefineMethod("is_a?", &object.Method{Name: "is_a?", Fn: methodIsA, Arity: 1})

	integerClass := R.Classes["Integer"]
	integerClass.DefineMethod("+", &object.Method{Name: "+", Fn: intAdd, Arity: 1})
	integerClass.DefineMethod("-", &object.Method{Name: "-", Fn: intSub, Arity: 1})
	integerClass.DefineMethod("*", &object.Method{Name: "*", Fn: intMul, Arity: 1})
	integerClass.DefineMethod("/", &object.Method{Name: "/", Fn: intDiv, Arity: 1})
	integerClass.DefineMethod("%", &object.Method{Name: "%", Fn: intMod, Arity: 1})
	integerClass.DefineMethod("**", &object.Method{Name: "**", Fn: intPow, Arity: 1})
	integerClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: intToS, Arity: 0})
	integerClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: intSucc, Arity: 0})
	integerClass.DefineMethod("pred", &object.Method{Name: "pred", Fn: intPred, Arity: 0})
	integerClass.DefineMethod("chr", &object.Method{Name: "chr", Fn: intChr, Arity: 0})
	integerClass.DefineMethod("odd", &object.Method{Name: "odd", Fn: intOdd, Arity: 0})
	integerClass.DefineMethod("even", &object.Method{Name: "even", Fn: intEven, Arity: 0})
	integerClass.DefineMethod("zero", &object.Method{Name: "zero", Fn: intZero, Arity: 0})
	integerClass.DefineMethod("abs", &object.Method{Name: "abs", Fn: intAbs, Arity: 0})
	integerClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: intToF, Arity: 0})

	floatClass := R.Classes["Float"]
	floatClass.DefineMethod("+", &object.Method{Name: "+", Fn: floatAdd, Arity: 1})
	floatClass.DefineMethod("-", &object.Method{Name: "-", Fn: floatSub, Arity: 1})
	floatClass.DefineMethod("*", &object.Method{Name: "*", Fn: floatMul, Arity: 1})
	floatClass.DefineMethod("/", &object.Method{Name: "/", Fn: floatDiv, Arity: 1})
	floatClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: floatToS, Arity: 0})
	floatClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: floatToI, Arity: 0})

	stringClass := R.Classes["String"]
	stringClass.DefineMethod("+", &object.Method{Name: "+", Fn: stringAdd, Arity: 1})
	stringClass.DefineMethod("*", &object.Method{Name: "*", Fn: stringMul, Arity: 1})
	stringClass.DefineMethod("length", &object.Method{Name: "length", Fn: stringLength, Arity: 0})
	stringClass.DefineMethod("size", &object.Method{Name: "size", Fn: stringLength, Arity: 0})
	stringClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: stringEmpty, Arity: 0})
	stringClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: stringToS, Arity: 0})
	stringClass.DefineMethod("upcase", &object.Method{Name: "upcase", Fn: stringUpcase, Arity: 0})
	stringClass.DefineMethod("downcase", &object.Method{Name: "downcase", Fn: stringDowncase, Arity: 0})
	stringClass.DefineMethod("strip", &object.Method{Name: "strip", Fn: stringStrip, Arity: 0})
	stringClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: stringIndex, Arity: 1})
	stringClass.DefineMethod("capitalize", &object.Method{Name: "capitalize", Fn: stringCapitalize, Arity: 0})
	stringClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: stringInclude, Arity: 1})
	stringClass.DefineMethod("start_with?", &object.Method{Name: "start_with?", Fn: stringStartWith, Arity: 1})
	stringClass.DefineMethod("end_with?", &object.Method{Name: "end_with?", Fn: stringEndWith, Arity: 1})
	stringClass.DefineMethod("reverse", &object.Method{Name: "reverse", Fn: stringReverse, Arity: 0})
	stringClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: stringToI, Arity: 0})

	arrayClass := R.Classes["Array"]
	arrayClass.DefineMethod("length", &object.Method{Name: "length", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("size", &object.Method{Name: "size", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("first", &object.Method{Name: "first", Fn: arrayFirst, Arity: 0})
	arrayClass.DefineMethod("last", &object.Method{Name: "last", Fn: arrayLast, Arity: 0})
	arrayClass.DefineMethod("push", &object.Method{Name: "push", Fn: arrayPush, Arity: 1})
	arrayClass.DefineMethod("pop", &object.Method{Name: "pop", Fn: arrayPop, Arity: 0})
	arrayClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: arrayEmpty, Arity: 0})
	arrayClass.DefineMethod("join", &object.Method{Name: "join", Fn: arrayJoin, Arity: 0})
	arrayClass.DefineMethod("reverse", &object.Method{Name: "reverse", Fn: arrayReverse, Arity: 0})
	arrayClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: arrayIndex, Arity: 1})

	hashClass := R.Classes["Hash"]
	hashClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: hashIndex, Arity: 1})
	hashClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: hashIndexSet, Arity: 2})
	hashClass.DefineMethod("keys", &object.Method{Name: "keys", Fn: hashKeys, Arity: 0})
	hashClass.DefineMethod("values", &object.Method{Name: "values", Fn: hashValues, Arity: 0})
	hashClass.DefineMethod("length", &object.Method{Name: "length", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("size", &object.Method{Name: "size", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: hashEmpty, Arity: 0})

	objectClass.DefineMethod("puts", &object.Method{Name: "puts", Fn: builtinPuts, Arity: -1})
	objectClass.DefineMethod("print", &object.Method{Name: "print", Fn: builtinPrint, Arity: -1})
	objectClass.DefineMethod("p", &object.Method{Name: "p", Fn: builtinP, Arity: -1})
	objectClass.DefineMethod("gets", &object.Method{Name: "gets", Fn: builtinGets, Arity: 0})

	R.Main = &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(objectClass),
		Class: objectClass,
	}
}

func methodClass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueClass,
		Data:  receiver.Class,
		Class: R.Classes["Class"],
	}
}

func methodToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Inspect(),
		Class: R.Classes["String"],
	}
}

func methodInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Inspect(),
		Class: R.Classes["String"],
	}
}

func methodIsNil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueNil {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver == args[0] {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodEql(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Equals(args[0]) {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodRespondTo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	methodName, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	_, ok = receiver.Class.GetMethod(methodName)
	if ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodSend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	methodName, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	method, ok := receiver.Class.GetMethod(methodName)
	if !ok {
		return R.NilVal
	}
	if fn, ok := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return fn(receiver, args[1:]...)
	}
	return R.NilVal
}

func methodIsA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if args[0].Type != object.ValueClass {
		return R.FalseVal
	}
	targetClass := args[0].Data.(*object.Class)
	currentClass := receiver.Class
	for currentClass != nil {
		if currentClass == targetClass {
			return R.TrueVal
		}
		currentClass = currentClass.SuperClass
	}
	return R.FalseVal
}

func intAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l + r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) + r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l - r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) - r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l * r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) * r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intDiv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l / r, Class: R.Classes["Integer"]}
	case float64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) / r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intMod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l % r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intPow(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if r < 0 {
			return &object.EmeraldValue{Type: object.ValueFloat, Data: 1.0 / powInt(l, -int(r)), Class: R.Classes["Float"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: powInt(l, int(r)), Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: mathPow(float64(l), r), Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func powInt(base int64, exp int) int64 {
	result := int64(1)
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func mathPow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func intToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  fmt.Sprintf("%d", receiver.Data.(int64)),
		Class: R.Classes["String"],
	}
}

func intSucc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  v + 1,
		Class: R.Classes["Integer"],
	}
}

func intPred(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  v - 1,
		Class: R.Classes["Integer"],
	}
}

func intChr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  string(rune(receiver.Data.(int64))),
		Class: R.Classes["String"],
	}
}

func intOdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64)%2 == 1 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intEven(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64)%2 == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intAbs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	if v < 0 {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: -v, Class: R.Classes["Integer"]}
	}
	return receiver
}

func intToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  float64(receiver.Data.(int64)),
		Class: R.Classes["Float"],
	}
}

func floatAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l + float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l + r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l - float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l - r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l * float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l * r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatDiv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l / float64(r), Class: R.Classes["Float"]}
	case float64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l / r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  fmt.Sprintf("%g", receiver.Data.(float64)),
		Class: R.Classes["String"],
	}
}

func floatToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(receiver.Data.(float64)),
		Class: R.Classes["Integer"],
	}
}

func stringAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	r, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Data.(string) + r,
		Class: R.Classes["String"],
	}
}

func stringMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	n, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	s := receiver.Data.(string)
	result := ""
	for i := int64(0); i < n; i++ {
		result += s
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(receiver.Data.(string))),
		Class: R.Classes["Integer"],
	}
}

func stringEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(receiver.Data.(string)) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func stringUpcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringDowncase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringStrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				result += " "
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	if len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	switch idx := args[0].Data.(type) {
	case int64:
		if idx < 0 {
			idx = int64(len(s)) + idx
		}
		if idx < 0 || idx >= int64(len(s)) {
			return R.NilVal
		}
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  string(s[idx]),
			Class: R.Classes["String"],
		}
	}
	return R.NilVal
}

func arrayLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(arr)),
		Class: R.Classes["Integer"],
	}
}

func arrayFirst(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) > 0 {
		return arr[0]
	}
	return R.NilVal
}

func arrayLast(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) > 0 {
		return arr[len(arr)-1]
	}
	return R.NilVal
}

func arrayPush(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := append(arr, args[0])
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayPop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	return arr[len(arr)-1]
}

func arrayEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func arrayJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := ""
	for i, v := range arr {
		result += v.Inspect()
		if i < len(arr)-1 {
			result += ", "
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func arrayReverse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, len(arr))
	for i, v := range arr {
		newArr[len(arr)-1-i] = v
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	switch idx := args[0].Data.(type) {
	case int64:
		if idx < 0 {
			idx = int64(len(arr)) + idx
		}
		if idx < 0 || idx >= int64(len(arr)) {
			return R.NilVal
		}
		return arr[idx]
	}
	return R.NilVal
}

func hashIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	if val, ok := h[args[0]]; ok {
		return val
	}
	return R.NilVal
}

func hashIndexSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	h[args[0]] = args[1]
	return args[1]
}

func hashKeys(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	keys := make([]*object.EmeraldValue, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  keys,
		Class: R.Classes["Array"],
	}
}

func hashValues(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	values := make([]*object.EmeraldValue, 0, len(h))
	for _, v := range h {
		values = append(values, v)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  values,
		Class: R.Classes["Array"],
	}
}

func hashLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(h)),
		Class: R.Classes["Integer"],
	}
}

func hashEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	if len(h) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func builtinPuts(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Println(arg.Inspect())
	}
	return R.NilVal
}

func builtinPrint(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Print(arg.Inspect())
	}
	return R.NilVal
}

func builtinP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Printf("%#v\n", arg.Inspect())
	}
	return R.NilVal
}

func builtinGets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var input string
	fmt.Scanln(&input)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  input,
		Class: R.Classes["String"],
	}
}

func stringCapitalize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := string(s[0] - 32)
	if len(s) > 1 {
		result += s[1:]
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	substr, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(s) == 0 && len(substr) == 0 {
		return R.TrueVal
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func stringStartWith(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	prefix, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(prefix) > len(s) {
		return R.FalseVal
	}
	if s[:len(prefix)] == prefix {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringEndWith(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	suffix, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(suffix) > len(s) {
		return R.FalseVal
	}
	if s[len(s)-len(suffix):] == suffix {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringReverse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		result += string(s[i])
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int64(c-'0')
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}
