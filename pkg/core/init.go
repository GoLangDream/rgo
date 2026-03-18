package core

import (
	"fmt"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type BuiltinMethod func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue

// CallBlock is set by the VM at startup so core methods can invoke blocks.
var CallBlock func(args ...*object.EmeraldValue) *object.EmeraldValue

func isTruthy(val *object.EmeraldValue) bool {
	if val == nil || val == R.NilVal || val == R.FalseVal {
		return false
	}
	return true
}

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
	RegisterMspec()
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
	procClass := object.NewClass("Proc")
	procClass.SuperClass = objectClass

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
	R.Classes["Proc"] = procClass
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
	integerClass.DefineMethod("==", &object.Method{Name: "==", Fn: intEqual, Arity: 1})
	integerClass.DefineMethod("===", &object.Method{Name: "===", Fn: intEqual, Arity: 1})
	integerClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: intToS, Arity: 0})
	integerClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: intSucc, Arity: 0})
	integerClass.DefineMethod("pred", &object.Method{Name: "pred", Fn: intPred, Arity: 0})
	integerClass.DefineMethod("chr", &object.Method{Name: "chr", Fn: intChr, Arity: 0})
	integerClass.DefineMethod("odd?", &object.Method{Name: "odd?", Fn: intOdd, Arity: 0})
	integerClass.DefineMethod("even?", &object.Method{Name: "even?", Fn: intEven, Arity: 0})
	integerClass.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: intZero, Arity: 0})
	integerClass.DefineMethod("abs", &object.Method{Name: "abs", Fn: intAbs, Arity: 0})
	integerClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: intToF, Arity: 0})
	integerClass.DefineMethod("times", &object.Method{Name: "times", Fn: intTimes, Arity: 0})
	integerClass.DefineMethod("upto", &object.Method{Name: "upto", Fn: intUpto, Arity: 1})
	integerClass.DefineMethod("downto", &object.Method{Name: "downto", Fn: intDownto, Arity: 1})
	integerClass.DefineMethod("gcd", &object.Method{Name: "gcd", Fn: intGcd, Arity: 1})
	integerClass.DefineMethod("lcm", &object.Method{Name: "lcm", Fn: intLcm, Arity: 1})
	integerClass.DefineMethod("divmod", &object.Method{Name: "divmod", Fn: intDivmod, Arity: 1})

	// Bitwise operators
	integerClass.DefineMethod("&", &object.Method{Name: "&", Fn: intBitAnd, Arity: 1})
	integerClass.DefineMethod("|", &object.Method{Name: "|", Fn: intBitOr, Arity: 1})
	integerClass.DefineMethod("^", &object.Method{Name: "^", Fn: intBitXor, Arity: 1})
	integerClass.DefineMethod("~", &object.Method{Name: "~", Fn: intBitNot, Arity: 0})
	integerClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: intLeftShift, Arity: 1})
	integerClass.DefineMethod(">>", &object.Method{Name: ">>", Fn: intRightShift, Arity: 1})

	// Comparison operators
	integerClass.DefineMethod("<", &object.Method{Name: "<", Fn: intLessThan, Arity: 1})
	integerClass.DefineMethod(">", &object.Method{Name: ">", Fn: intGreaterThan, Arity: 1})
	integerClass.DefineMethod("<=", &object.Method{Name: "<=", Fn: intLessThanOrEqual, Arity: 1})
	integerClass.DefineMethod(">=", &object.Method{Name: ">=", Fn: intGreaterThanOrEqual, Arity: 1})
	integerClass.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: intCompare, Arity: 1})

	symbolClass := R.Classes["Symbol"]
	symbolClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: symbolToS, Arity: 0})
	symbolClass.DefineMethod("to_sym", &object.Method{Name: "to_sym", Fn: symbolToSym, Arity: 0})
	symbolClass.DefineMethod("length", &object.Method{Name: "length", Fn: symbolLength, Arity: 0})
	symbolClass.DefineMethod("size", &object.Method{Name: "size", Fn: symbolLength, Arity: 0})

	floatClass := R.Classes["Float"]
	floatClass.DefineMethod("+", &object.Method{Name: "+", Fn: floatAdd, Arity: 1})
	floatClass.DefineMethod("-", &object.Method{Name: "-", Fn: floatSub, Arity: 1})
	floatClass.DefineMethod("*", &object.Method{Name: "*", Fn: floatMul, Arity: 1})
	floatClass.DefineMethod("/", &object.Method{Name: "/", Fn: floatDiv, Arity: 1})
	floatClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: floatToS, Arity: 0})
	floatClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: floatToI, Arity: 0})
	floatClass.DefineMethod("floor", &object.Method{Name: "floor", Fn: floatFloor, Arity: 0})
	floatClass.DefineMethod("ceil", &object.Method{Name: "ceil", Fn: floatCeil, Arity: 0})
	floatClass.DefineMethod("round", &object.Method{Name: "round", Fn: floatRound, Arity: 0})
	floatClass.DefineMethod("abs", &object.Method{Name: "abs", Fn: floatAbs, Arity: 0})
	floatClass.DefineMethod("<", &object.Method{Name: "<", Fn: floatLessThan, Arity: 1})
	floatClass.DefineMethod(">", &object.Method{Name: ">", Fn: floatGreaterThan, Arity: 1})
	floatClass.DefineMethod("<=", &object.Method{Name: "<=", Fn: floatLessThanOrEqual, Arity: 1})
	floatClass.DefineMethod(">=", &object.Method{Name: ">=", Fn: floatGreaterThanOrEqual, Arity: 1})
	floatClass.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: floatCompare, Arity: 1})

	rangeClass := R.Classes["Range"]
	rangeClass.DefineMethod("each", &object.Method{Name: "each", Fn: rangeEach, Arity: 0})
	rangeClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: rangeToA, Arity: 0})

	regexpClass := R.Classes["Regexp"]
	regexpClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: regexpToS, Arity: 0})

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
	stringClass.DefineMethod("count", &object.Method{Name: "count", Fn: stringCount, Arity: 0})
	stringClass.DefineMethod("size", &object.Method{Name: "size", Fn: stringCountChars, Arity: 0})
	stringClass.DefineMethod("bytes", &object.Method{Name: "bytes", Fn: stringBytes, Arity: 0})
	stringClass.DefineMethod("chars", &object.Method{Name: "chars", Fn: stringChars, Arity: 0})
	stringClass.DefineMethod("find", &object.Method{Name: "find", Fn: stringFind, Arity: 1})
	stringClass.DefineMethod("slice", &object.Method{Name: "slice", Fn: stringSlice, Arity: 1})
	stringClass.DefineMethod("to_sym", &object.Method{Name: "to_sym", Fn: stringToSym, Arity: 0})
	stringClass.DefineMethod("ljust", &object.Method{Name: "ljust", Fn: stringLjust, Arity: 1})
	stringClass.DefineMethod("rjust", &object.Method{Name: "rjust", Fn: stringRjust, Arity: 1})
	stringClass.DefineMethod("center", &object.Method{Name: "center", Fn: stringCenter, Arity: 1})
	stringClass.DefineMethod("gsub", &object.Method{Name: "gsub", Fn: stringGsub, Arity: 2})
	stringClass.DefineMethod("sub", &object.Method{Name: "sub", Fn: stringSub, Arity: 2})
	stringClass.DefineMethod("split", &object.Method{Name: "split", Fn: stringSplit, Arity: 1})
	stringClass.DefineMethod("lines", &object.Method{Name: "lines", Fn: stringLines, Arity: 0})
	stringClass.DefineMethod("chomp", &object.Method{Name: "chomp", Fn: stringChomp, Arity: 0})
	stringClass.DefineMethod("chop", &object.Method{Name: "chop", Fn: stringChop, Arity: 0})
	stringClass.DefineMethod("strip!", &object.Method{Name: "strip!", Fn: stringStripBang, Arity: 0})
	stringClass.DefineMethod("upcase!", &object.Method{Name: "upcase!", Fn: stringUpcaseBang, Arity: 0})
	stringClass.DefineMethod("downcase!", &object.Method{Name: "downcase!", Fn: stringDowncaseBang, Arity: 0})
	stringClass.DefineMethod("reverse!", &object.Method{Name: "reverse!", Fn: stringReverseBang, Arity: 0})
	stringClass.DefineMethod("concat", &object.Method{Name: "concat", Fn: stringConcat, Arity: 1})
	stringClass.DefineMethod("index", &object.Method{Name: "index", Fn: stringIndexOf, Arity: 1})
	stringClass.DefineMethod("rindex", &object.Method{Name: "rindex", Fn: stringRIndexOf, Arity: 1})
	stringClass.DefineMethod("ord", &object.Method{Name: "ord", Fn: stringOrd, Arity: 0})
	stringClass.DefineMethod("+@", &object.Method{Name: "+@", Fn: stringUplus, Arity: 0})
	stringClass.DefineMethod("-@", &object.Method{Name: "-@", Fn: stringUminus, Arity: 0})
	stringClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: stringSucc, Arity: 0})
	stringClass.DefineMethod("next", &object.Method{Name: "next", Fn: stringSucc, Arity: 0})
	stringClass.DefineMethod("lstrip", &object.Method{Name: "lstrip", Fn: stringLstrip, Arity: 0})
	stringClass.DefineMethod("rstrip", &object.Method{Name: "rstrip", Fn: stringRstrip, Arity: 0})
	stringClass.DefineMethod("lstrip!", &object.Method{Name: "lstrip!", Fn: stringLstripBang, Arity: 0})
	stringClass.DefineMethod("rstrip!", &object.Method{Name: "rstrip!", Fn: stringRstripBang, Arity: 0})
	stringClass.DefineMethod("strip!", &object.Method{Name: "strip!", Fn: stringStripBang, Arity: 0})
	stringClass.DefineMethod("replace", &object.Method{Name: "replace", Fn: stringReplace, Arity: 1})
	stringClass.DefineMethod("insert", &object.Method{Name: "insert", Fn: stringInsert, Arity: 2})
	stringClass.DefineMethod("swapcase", &object.Method{Name: "swapcase", Fn: stringSwapcase, Arity: 0})
	stringClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: stringDelete, Arity: 1})
	stringClass.DefineMethod("squeeze", &object.Method{Name: "squeeze", Fn: stringSqueeze, Arity: 0})
	stringClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: stringToF, Arity: 0})
	stringClass.DefineMethod("hex", &object.Method{Name: "hex", Fn: stringHex, Arity: 0})
	stringClass.DefineMethod("oct", &object.Method{Name: "oct", Fn: stringOct, Arity: 0})
	stringClass.DefineMethod("unpack", &object.Method{Name: "unpack", Fn: stringUnpack, Arity: 1})

	arrayClass := R.Classes["Array"]
	arrayClass.DefineMethod("length", &object.Method{Name: "length", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("size", &object.Method{Name: "size", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("first", &object.Method{Name: "first", Fn: arrayFirst, Arity: 0})
	arrayClass.DefineMethod("last", &object.Method{Name: "last", Fn: arrayLast, Arity: 0})
	arrayClass.DefineMethod("push", &object.Method{Name: "push", Fn: arrayPush, Arity: 1})
	arrayClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: arrayPush, Arity: 1})
	arrayClass.DefineMethod("pop", &object.Method{Name: "pop", Fn: arrayPop, Arity: 0})
	arrayClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: arrayEmpty, Arity: 0})
	arrayClass.DefineMethod("join", &object.Method{Name: "join", Fn: arrayJoin, Arity: 0})
	arrayClass.DefineMethod("reverse", &object.Method{Name: "reverse", Fn: arrayReverse, Arity: 0})
	arrayClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: arrayIndex, Arity: 1})
	arrayClass.DefineMethod("each", &object.Method{Name: "each", Fn: arrayEach, Arity: 0})
	arrayClass.DefineMethod("map", &object.Method{Name: "map", Fn: arrayMap, Arity: 0})
	arrayClass.DefineMethod("select", &object.Method{Name: "select", Fn: arraySelect, Arity: 0})
	arrayClass.DefineMethod("find", &object.Method{Name: "find", Fn: arrayFind, Arity: 0})
	arrayClass.DefineMethod("concat", &object.Method{Name: "concat", Fn: arrayConcat, Arity: 1})
	arrayClass.DefineMethod("delete_at", &object.Method{Name: "delete_at", Fn: arrayDeleteAt, Arity: 1})
	arrayClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: arrayShift, Arity: 0})
	arrayClass.DefineMethod("unshift", &object.Method{Name: "unshift", Fn: arrayUnshift, Arity: 1})
	arrayClass.DefineMethod("sample", &object.Method{Name: "sample", Fn: arraySample, Arity: 0})
	arrayClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: arrayClear, Arity: 0})
	arrayClass.DefineMethod("include", &object.Method{Name: "include", Fn: arrayInclude, Arity: 1})
	arrayClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: arrayIndexSet, Arity: 2})
	arrayClass.DefineMethod("count", &object.Method{Name: "count", Fn: arrayCount, Arity: 0})
	arrayClass.DefineMethod("index", &object.Method{Name: "index", Fn: arrayIndexOf, Arity: 1})
	arrayClass.DefineMethod("rindex", &object.Method{Name: "rindex", Fn: arrayRIndexOf, Arity: 1})
	arrayClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: arrayDelete, Arity: 1})
	arrayClass.DefineMethod("compact", &object.Method{Name: "compact", Fn: arrayCompact, Arity: 0})
	arrayClass.DefineMethod("flatten", &object.Method{Name: "flatten", Fn: arrayFlatten, Arity: 0})
	arrayClass.DefineMethod("uniq", &object.Method{Name: "uniq", Fn: arrayUniq, Arity: 0})
	arrayClass.DefineMethod("sort", &object.Method{Name: "sort", Fn: arraySort, Arity: 0})
	arrayClass.DefineMethod("+", &object.Method{Name: "+", Fn: arrayPlus, Arity: 1})
	arrayClass.DefineMethod("-", &object.Method{Name: "-", Fn: arrayMinus, Arity: 1})
	arrayClass.DefineMethod("&", &object.Method{Name: "&", Fn: arrayIntersection, Arity: 1})
	arrayClass.DefineMethod("|", &object.Method{Name: "|", Fn: arrayUnion, Arity: 1})
	arrayClass.DefineMethod("take", &object.Method{Name: "take", Fn: arrayTake, Arity: 1})
	arrayClass.DefineMethod("drop", &object.Method{Name: "drop", Fn: arrayDrop, Arity: 1})
	arrayClass.DefineMethod("any?", &object.Method{Name: "any?", Fn: arrayAny, Arity: 0})
	arrayClass.DefineMethod("all?", &object.Method{Name: "all?", Fn: arrayAll, Arity: 0})
	arrayClass.DefineMethod("none?", &object.Method{Name: "none?", Fn: arrayNone, Arity: 0})
	arrayClass.DefineMethod("one?", &object.Method{Name: "one?", Fn: arrayOne, Arity: 0})
	arrayClass.DefineMethod("sum", &object.Method{Name: "sum", Fn: arraySum, Arity: 0})
	arrayClass.DefineMethod("max", &object.Method{Name: "max", Fn: arrayMax, Arity: 0})
	arrayClass.DefineMethod("min", &object.Method{Name: "min", Fn: arrayMin, Arity: 0})
	arrayClass.DefineMethod("insert", &object.Method{Name: "insert", Fn: arrayInsert, Arity: 2})
	arrayClass.DefineMethod("slice", &object.Method{Name: "slice", Fn: arraySlice, Arity: 1})
	arrayClass.DefineMethod("values_at", &object.Method{Name: "values_at", Fn: arrayValuesAt, Arity: -1})
	arrayClass.DefineMethod("zip", &object.Method{Name: "zip", Fn: arrayZip, Arity: 1})
	arrayClass.DefineMethod("each_index", &object.Method{Name: "each_index", Fn: arrayEachIndex, Arity: 0})
	arrayClass.DefineMethod("each_with_index", &object.Method{Name: "each_with_index", Fn: arrayEachWithIndex, Arity: 0})
	arrayClass.DefineMethod("rotate", &object.Method{Name: "rotate", Fn: arrayRotate, Arity: 0})
	arrayClass.DefineMethod("shuffle", &object.Method{Name: "shuffle", Fn: arrayShuffle, Arity: 0})
	arrayClass.DefineMethod("fetch", &object.Method{Name: "fetch", Fn: arrayFetch, Arity: 1})
	arrayClass.DefineMethod("reject", &object.Method{Name: "reject", Fn: arrayReject, Arity: 0})

	hashClass := R.Classes["Hash"]
	hashClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: hashIndex, Arity: 1})
	hashClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: hashIndexSet, Arity: 2})
	hashClass.DefineMethod("keys", &object.Method{Name: "keys", Fn: hashKeys, Arity: 0})
	hashClass.DefineMethod("values", &object.Method{Name: "values", Fn: hashValues, Arity: 0})
	hashClass.DefineMethod("length", &object.Method{Name: "length", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("size", &object.Method{Name: "size", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: hashEmpty, Arity: 0})
	hashClass.DefineMethod("each", &object.Method{Name: "each", Fn: hashEach, Arity: 0})
	hashClass.DefineMethod("each_key", &object.Method{Name: "each_key", Fn: hashEachKey, Arity: 0})
	hashClass.DefineMethod("each_value", &object.Method{Name: "each_value", Fn: hashEachValue, Arity: 0})
	hashClass.DefineMethod("key?", &object.Method{Name: "key?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("has_key?", &object.Method{Name: "has_key?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("fetch", &object.Method{Name: "fetch", Fn: hashFetch, Arity: 1})
	hashClass.DefineMethod("merge", &object.Method{Name: "merge", Fn: hashMerge, Arity: 1})
	hashClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: hashDelete, Arity: 1})
	hashClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: hashClear, Arity: 0})
	hashClass.DefineMethod("has_value?", &object.Method{Name: "has_value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineMethod("value?", &object.Method{Name: "value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineMethod("dig", &object.Method{Name: "dig", Fn: hashDig, Arity: 1})
	hashClass.DefineMethod("merge!", &object.Method{Name: "merge!", Fn: hashMergeBang, Arity: 1})
	hashClass.DefineMethod("update", &object.Method{Name: "update", Fn: hashMergeBang, Arity: 1})
	hashClass.DefineMethod("invert", &object.Method{Name: "invert", Fn: hashInvert, Arity: 0})
	hashClass.DefineMethod("each_pair", &object.Method{Name: "each_pair", Fn: hashEach, Arity: 0})
	hashClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: hashDelete, Arity: 1})
	hashClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: hashClear, Arity: 0})
	hashClass.DefineMethod("has_value?", &object.Method{Name: "has_value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineMethod("merge", &object.Method{Name: "merge", Fn: hashMerge, Arity: 1})
	hashClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: hashToA, Arity: 0})
	hashClass.DefineMethod("select", &object.Method{Name: "select", Fn: hashSelect, Arity: 0})
	hashClass.DefineMethod("reject", &object.Method{Name: "reject", Fn: hashReject, Arity: 0})
	hashClass.DefineMethod("transform_keys", &object.Method{Name: "transform_keys", Fn: hashTransformKeys, Arity: 0})
	hashClass.DefineMethod("transform_values", &object.Method{Name: "transform_values", Fn: hashTransformValues, Arity: 0})
	hashClass.DefineMethod("assoc", &object.Method{Name: "assoc", Fn: hashAssoc, Arity: 1})
	hashClass.DefineMethod("rassoc", &object.Method{Name: "rassoc", Fn: hashRassoc, Arity: 1})
	hashClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: hashShift, Arity: 0})
	hashClass.DefineMethod("replace", &object.Method{Name: "replace", Fn: hashReplace, Arity: 1})

	procClass := R.Classes["Proc"]
	procClass.DefineMethod("call", &object.Method{Name: "call", Fn: procCall, Arity: -1})
	procClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: procCall, Arity: -1})
	procClass.DefineMethod("arity", &object.Method{Name: "arity", Fn: procArity, Arity: 0})
	procClass.DefineMethod("lambda?", &object.Method{Name: "lambda?", Fn: procIsLambda, Arity: 0})

	objectClass.DefineMethod("puts", &object.Method{Name: "puts", Fn: builtinPuts, Arity: -1})
	objectClass.DefineMethod("print", &object.Method{Name: "print", Fn: builtinPrint, Arity: -1})
	objectClass.DefineMethod("p", &object.Method{Name: "p", Fn: builtinP, Arity: -1})
	objectClass.DefineMethod("gets", &object.Method{Name: "gets", Fn: builtinGets, Arity: 0})
	objectClass.DefineMethod("loop", &object.Method{Name: "loop", Fn: builtinLoop, Arity: 0})
	objectClass.DefineMethod("exit", &object.Method{Name: "exit", Fn: builtinExit, Arity: 0})
	objectClass.DefineMethod("sleep", &object.Method{Name: "sleep", Fn: builtinSleep, Arity: 1})
	objectClass.DefineMethod("rand", &object.Method{Name: "rand", Fn: builtinRand, Arity: 0})
	objectClass.DefineMethod("srand", &object.Method{Name: "srand", Fn: builtinSrand, Arity: 1})
	objectClass.DefineMethod("raise", &object.Method{Name: "raise", Fn: builtinRaise, Arity: 1})
	objectClass.DefineMethod("fail", &object.Method{Name: "fail", Fn: builtinRaise, Arity: 1})
	objectClass.DefineMethod("abort", &object.Method{Name: "abort", Fn: builtinAbort, Arity: 0})
	objectClass.DefineMethod("should", &object.Method{Name: "should", Arity: 0, Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return &object.EmeraldValue{Type: object.ValueObject, Data: r, Class: R.Classes["Expectation"]}
	}})
	objectClass.DefineMethod("should_not", &object.Method{Name: "should_not", Arity: 0, Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return &object.EmeraldValue{Type: object.ValueObject, Data: r, Class: R.Classes["Expectation"]}
	}})

	moduleClass := R.Classes["Module"]
	moduleClass.DefineMethod("include", &object.Method{Name: "include", Fn: moduleInclude, Arity: -1})
	moduleClass.DefineMethod("extend", &object.Method{Name: "extend", Fn: moduleExtend, Arity: -1})
	moduleClass.DefineMethod("prepend", &object.Method{Name: "prepend", Fn: modulePrepend, Arity: -1})

	classClass := R.Classes["Class"]
	classClass.DefineMethod("include", &object.Method{Name: "include", Fn: classInclude, Arity: -1})
	classClass.DefineMethod("extend", &object.Method{Name: "extend", Fn: classExtend, Arity: -1})
	classClass.DefineMethod("prepend", &object.Method{Name: "prepend", Fn: classPrepend, Arity: -1})

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

func intGcd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	a := receiver.Data.(int64)
	b := args[0].Data.(int64)
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b > 0 {
		a, b = b, a%b
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  a,
		Class: R.Classes["Integer"],
	}
}

func intLcm(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	a := receiver.Data.(int64)
	b := args[0].Data.(int64)
	gcd := a
	tmp := b
	for tmp > 0 {
		gcd, tmp = tmp, gcd%tmp
	}
	lcm := (a * b) / gcd
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  lcm,
		Class: R.Classes["Integer"],
	}
}

func intDivmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	a := receiver.Data.(int64)
	b := args[0].Data.(int64)
	quotient := a / b
	remainder := a % b
	result := make([]*object.EmeraldValue, 2)
	result[0] = &object.EmeraldValue{Type: object.ValueInteger, Data: quotient, Class: R.Classes["Integer"]}
	result[1] = &object.EmeraldValue{Type: object.ValueInteger, Data: remainder, Class: R.Classes["Integer"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}

func intBitAnd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l & r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitOr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l | r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitXor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l ^ r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitNot(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: ^v, Class: R.Classes["Integer"]}
}

func intLeftShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l << r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intRightShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l >> r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intLessThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < r {
			return R.TrueVal
		}
	case float64:
		if float64(l) < r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intGreaterThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l > r {
			return R.TrueVal
		}
	case float64:
		if float64(l) > r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intLessThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l <= r {
			return R.TrueVal
		}
	case float64:
		if float64(l) <= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intGreaterThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l >= r {
			return R.TrueVal
		}
	case float64:
		if float64(l) >= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	case float64:
		if float64(l) < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if float64(l) > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l == r {
			return R.TrueVal
		}
	case float64:
		if float64(l) == r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intTimes(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n := receiver.Data.(int64)
	for i := int64(0); i < n; i++ {
		fmt.Println(i)
	}
	return receiver
}

func intUpto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	start := receiver.Data.(int64)
	end := args[0].Data.(int64)
	for i := start; i <= end; i++ {
		fmt.Println(i)
	}
	return receiver
}

func intDownto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	start := receiver.Data.(int64)
	end := args[0].Data.(int64)
	for i := start; i >= end; i-- {
		fmt.Println(i)
	}
	return receiver
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

func floatFloor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatCeil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if f > 0 {
		f = f + 1
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatRound(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if f > 0 {
		f = f + 0.5
	} else {
		f = f - 0.5
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatAbs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if f < 0 {
		f = -f
	}
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  f,
		Class: R.Classes["Float"],
	}
}

func floatLessThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < float64(r) {
			return R.TrueVal
		}
	case float64:
		if l < r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatGreaterThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l > float64(r) {
			return R.TrueVal
		}
	case float64:
		if l > r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatLessThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l <= float64(r) {
			return R.TrueVal
		}
	case float64:
		if l <= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatGreaterThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l >= float64(r) {
			return R.TrueVal
		}
	case float64:
		if l >= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < float64(r) {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > float64(r) {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	case float64:
		if l < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}
	return R.NilVal
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

func stringFind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	substr, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	idx := strings.Index(s, substr)
	if idx < 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(idx),
		Class: R.Classes["Integer"],
	}
}

func stringSlice(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return R.NilVal
	}

	start := 0
	if args[0].Type == object.ValueInteger {
		start = int(args[0].Data.(int64))
	}

	length := len(s)
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		length = int(args[1].Data.(int64))
	}

	if start < 0 {
		start = len(s) + start
	}
	if start < 0 {
		start = 0
	}
	if start > len(s) {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "",
			Class: R.Classes["String"],
		}
	}

	if length > len(s)-start {
		length = len(s) - start
	}

	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  s[start : start+length],
		Class: R.Classes["String"],
	}
}

func stringToSym(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  s,
		Class: R.Classes["Symbol"],
	}
}

func arrayEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for _, elem := range arr {
		fmt.Println(elem.Inspect())
	}
	return receiver
}

func arrayMap(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr))
	for i, elem := range arr {
		val := CallBlock(elem)
		result[i] = val
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arraySelect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if isTruthy(val) {
			result = append(result, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayFind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for _, elem := range arr {
		val := CallBlock(elem)
		if isTruthy(val) {
			return elem
		}
	}
	return R.NilVal
}

func hashEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func hashEachKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	keys := make([]*object.EmeraldValue, 0, len(hash))
	for k := range hash {
		keys = append(keys, k)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  keys,
		Class: R.Classes["Array"],
	}
}

func hashEachValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	values := make([]*object.EmeraldValue, 0, len(hash))
	for _, v := range hash {
		values = append(values, v)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  values,
		Class: R.Classes["Array"],
	}
}

func hashHasKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	_, ok := hash[args[0]]
	if ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringCount(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(len(s)),
			Class: R.Classes["Integer"],
		}
	}
	substr := args[0].Data.(string)
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(count),
		Class: R.Classes["Integer"],
	}
}

func stringCountChars(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(s)),
		Class: R.Classes["Integer"],
	}
}

func stringBytes(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := make([]*object.EmeraldValue, len(s))
	for i, b := range s {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(b),
			Class: R.Classes["Integer"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringChars(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := make([]*object.EmeraldValue, 0)
	for _, c := range s {
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  string(c),
			Class: R.Classes["String"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayConcat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return receiver
	}
	other := args[0].Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr)+len(other))
	copy(result, arr)
	copy(result[len(arr):], other)
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayDeleteAt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	idx := int(args[0].Data.(int64))
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return R.NilVal
	}
	result := arr[idx]
	newArr := make([]*object.EmeraldValue, 0)
	newArr = append(newArr, arr[:idx]...)
	newArr = append(newArr, arr[idx+1:]...)
	receiver.Data = newArr
	return result
}

func hashFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	val, ok := hash[args[0]]
	if ok {
		return val
	}
	return R.NilVal
}

func hashMerge(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	other := args[0].Data.(map[*object.EmeraldValue]*object.EmeraldValue)

	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		result[k] = v
	}
	for k, v := range other {
		result[k] = v
	}

	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func symbolToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  s,
		Class: R.Classes["String"],
	}
}

func symbolToSym(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func symbolLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(s)),
		Class: R.Classes["Integer"],
	}
}

func rangeEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func rangeToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func regexpToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func arrayShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	result := arr[0]
	receiver.Data = arr[1:]
	return result
}

func arrayUnshift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr)+1)
	newArr = append(newArr, args[0])
	newArr = append(newArr, arr...)
	receiver.Data = newArr
	return receiver
}

func arraySample(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	return arr[0]
}

func arrayClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data = make([]*object.EmeraldValue, 0)
	return receiver
}

func arrayInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for _, elem := range arr {
		if elem.Equals(target) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func hashDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	val, ok := hash[args[0]]
	if ok {
		delete(hash, args[0])
	}
	if ok {
		return val
	}
	return R.NilVal
}

func hashClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data = make(map[*object.EmeraldValue]*object.EmeraldValue)
	return receiver
}

func hashHasValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	target := args[0]
	for _, val := range hash {
		if val.Equals(target) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func stringLjust(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(args[0].Data.(int64))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		pad = args[1].Data.(string)
	}
	result := s
	for len(result) < width {
		result += pad
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringRjust(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(args[0].Data.(int64))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		pad = args[1].Data.(string)
	}
	result := s
	for len(result) < width {
		result = pad + result
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringCenter(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(args[0].Data.(int64))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		pad = args[1].Data.(string)
	}
	left := (width - len(s)) / 2
	right := width - len(s) - left
	result := ""
	for i := 0; i < left; i++ {
		result += pad
	}
	result += s
	for i := 0; i < right; i++ {
		result += pad
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

// ========== New Array Methods ==========

func arrayIndexSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	idx := int(args[0].Data.(int64))
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return R.NilVal
	}
	arr[idx] = args[1]
	return args[1]
}

func arrayCount(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) == 0 {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(len(arr)),
			Class: R.Classes["Integer"],
		}
	}
	count := 0
	target := args[0]
	for _, elem := range arr {
		if elem.Equals(target) {
			count++
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(count),
		Class: R.Classes["Integer"],
	}
}

func arrayIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for i, elem := range arr {
		if elem.Equals(target) {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(i),
				Class: R.Classes["Integer"],
			}
		}
	}
	return R.NilVal
}

func arrayRIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for i := len(arr) - 1; i >= 0; i-- {
		if arr[i].Equals(target) {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(i),
				Class: R.Classes["Integer"],
			}
		}
	}
	return R.NilVal
}

func arrayDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	result := R.NilVal
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Equals(target) && result == R.NilVal {
			result = elem
		} else {
			newArr = append(newArr, elem)
		}
	}
	receiver.Data = newArr
	return result
}

func arrayCompact(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Type != object.ValueNil {
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayFlatten(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Type == object.ValueArray {
			nested := elem.Data.([]*object.EmeraldValue)
			for _, n := range nested {
				newArr = append(newArr, n)
			}
		} else {
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayUniq(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	seen := make(map[string]bool)
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		key := elem.Inspect()
		if !seen[key] {
			seen[key] = true
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arraySort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return receiver
	}
	// Simple bubble sort - works for integers
	newArr := make([]*object.EmeraldValue, len(arr))
	copy(newArr, arr)
	for i := 0; i < len(newArr)-1; i++ {
		for j := 0; j < len(newArr)-i-1; j++ {
			v1 := newArr[j].Data.(int64)
			v2 := newArr[j+1].Data.(int64)
			if v1 > v2 {
				newArr[j], newArr[j+1] = newArr[j+1], newArr[j]
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayPlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other := args[0].Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr)+len(other))
	copy(result, arr)
	copy(result[len(arr):], other)
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other := args[0].Data.([]*object.EmeraldValue)
	otherMap := make(map[string]bool)
	for _, o := range other {
		otherMap[o.Inspect()] = true
	}
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if !otherMap[elem.Inspect()] {
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayIntersection(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other := args[0].Data.([]*object.EmeraldValue)
	arrMap := make(map[string]bool)
	for _, a := range arr {
		arrMap[a.Inspect()] = true
	}
	newArr := make([]*object.EmeraldValue, 0)
	seen := make(map[string]bool)
	for _, o := range other {
		key := o.Inspect()
		if arrMap[key] && !seen[key] {
			seen[key] = true
			newArr = append(newArr, o)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayUnion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other := args[0].Data.([]*object.EmeraldValue)
	seen := make(map[string]bool)
	newArr := make([]*object.EmeraldValue, 0)
	for _, a := range arr {
		key := a.Inspect()
		if !seen[key] {
			seen[key] = true
			newArr = append(newArr, a)
		}
	}
	for _, o := range other {
		key := o.Inspect()
		if !seen[key] {
			seen[key] = true
			newArr = append(newArr, o)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayTake(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	n := int(args[0].Data.(int64))
	if n > len(arr) {
		n = len(arr)
	}
	if n < 0 {
		n = len(arr) + n
		if n < 0 {
			n = 0
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[:n],
		Class: R.Classes["Array"],
	}
}

func arrayDrop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	n := int(args[0].Data.(int64))
	if n > len(arr) {
		n = len(arr)
	}
	if n < 0 {
		n = len(arr) + n
		if n < 0 {
			n = 0
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[n:],
		Class: R.Classes["Array"],
	}
}

func arrayAny(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.FalseVal
	}
	// For now, return true if any element is truthy
	for _, elem := range arr {
		if elem.Type != object.ValueNil && elem.Type != object.ValueBool {
			return R.TrueVal
		}
		if elem.Type == object.ValueBool && elem.Data.(bool) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func arrayAll(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	// For now, return true if all elements are truthy
	for _, elem := range arr {
		if elem.Type == object.ValueNil {
			return R.FalseVal
		}
		if elem.Type == object.ValueBool && !elem.Data.(bool) {
			return R.FalseVal
		}
	}
	return R.TrueVal
}

func arrayNone(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	// For now, return true if no element is truthy
	for _, elem := range arr {
		if elem.Type != object.ValueNil && elem.Type != object.ValueBool {
			return R.FalseVal
		}
		if elem.Type == object.ValueBool && elem.Data.(bool) {
			return R.FalseVal
		}
	}
	return R.TrueVal
}

func arrayOne(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	count := 0
	for _, elem := range arr {
		if elem.Type != object.ValueNil && elem.Type != object.ValueBool {
			count++
		}
		if elem.Type == object.ValueBool && elem.Data.(bool) {
			count++
		}
	}
	if count == 1 {
		return R.TrueVal
	}
	return R.FalseVal
}

func arraySum(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	sum := int64(0)
	for _, elem := range arr {
		if v, ok := elem.Data.(int64); ok {
			sum += v
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  sum,
		Class: R.Classes["Integer"],
	}
}

func arrayMax(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	maxVal := arr[0]
	for _, elem := range arr[1:] {
		if v1, ok1 := maxVal.Data.(int64); ok1 {
			if v2, ok2 := elem.Data.(int64); ok2 {
				if v2 > v1 {
					maxVal = elem
				}
			}
		}
	}
	return maxVal
}

func stringGsub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return receiver
	}
	s := receiver.Data.(string)
	old := args[0].Data.(string)
	new := args[1].Data.(string)
	result := ""
	for i := 0; i < len(s); {
		idx := strings.Index(s[i:], old)
		if idx < 0 {
			result += s[i:]
			break
		}
		result += s[i : i+idx]
		result += new
		i += idx + len(old)
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return receiver
	}
	s := receiver.Data.(string)
	old := args[0].Data.(string)
	new := args[1].Data.(string)
	idx := strings.Index(s, old)
	if idx < 0 {
		return receiver
	}
	result := s[:idx] + new + s[idx+len(old):]
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	delim := ","
	if len(args) > 0 {
		delim = args[0].Data.(string)
	}
	parts := strings.Split(s, delim)
	result := make([]*object.EmeraldValue, len(parts))
	for i, p := range parts {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  p,
			Class: R.Classes["String"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringLines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	lines := strings.Split(s, "\n")
	result := make([]*object.EmeraldValue, 0)
	for _, line := range lines {
		if len(line) > 0 {
			result = append(result, &object.EmeraldValue{
				Type:  object.ValueString,
				Data:  line,
				Class: R.Classes["String"],
			})
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringChomp(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := strings.TrimRight(s, "\r\n")
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringChop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := s[:len(s)-1]
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringStripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := strings.TrimSpace(s)
	receiver.Data = result
	return receiver
}

func stringUpcaseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	receiver.Data = result
	return receiver
}

func stringDowncaseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	receiver.Data = result
	return receiver
}

func stringReverseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		result += string(s[i])
	}
	receiver.Data = result
	return receiver
}

func stringConcat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	s := receiver.Data.(string)
	other := args[0].Data.(string)
	receiver.Data = s + other
	return receiver
}

func stringIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	substr := args[0].Data.(string)
	idx := strings.Index(s, substr)
	if idx < 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(idx),
		Class: R.Classes["Integer"],
	}
}

func stringRIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	substr := args[0].Data.(string)
	idx := strings.LastIndex(s, substr)
	if idx < 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(idx),
		Class: R.Classes["Integer"],
	}
}

func stringOrd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(s[0]),
		Class: R.Classes["Integer"],
	}
}

func stringUplus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func stringUminus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
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

func stringSucc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := []byte(s)
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] < 'z' {
			result[i]++
			break
		}
		result[i] = 'a'
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  string(result),
		Class: R.Classes["String"],
	}
}

func arrayMin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	minVal := arr[0]
	for _, elem := range arr[1:] {
		if v1, ok1 := minVal.Data.(int64); ok1 {
			if v2, ok2 := elem.Data.(int64); ok2 {
				if v2 < v1 {
					minVal = elem
				}
			}
		}
	}
	return minVal
}

func hashDig(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	current := receiver
	for _, key := range args {
		if current.Type != object.ValueHash {
			return R.NilVal
		}
		hash := current.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
		var foundVal *object.EmeraldValue
		for k, v := range hash {
			if k.Equals(key) {
				foundVal = v
				break
			}
		}
		if foundVal == nil {
			return R.NilVal
		}
		current = foundVal
	}
	return current
}

func hashMergeBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	other := args[0].Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range other {
		hash[k] = v
	}
	return receiver
}

func hashInvert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		result[v] = k
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func builtinLoop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for {
	}
}

func builtinExit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func builtinSleep(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func builtinRand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  0.5,
		Class: R.Classes["Float"],
	}
}

func builtinSrand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func builtinRaise(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func builtinAbort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func stringLstrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringRstrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			continue
		}
		result = s[:i+1]
		break
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringLstripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	receiver.Data = result
	return receiver
}

func stringRstripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			continue
		}
		result = s[:i+1]
		break
	}
	receiver.Data = result
	return receiver
}

func stringReplace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	newStr, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	receiver.Data = newStr
	return receiver
}

func stringInsert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	insertStr, ok := args[1].Data.(string)
	if !ok {
		return R.NilVal
	}
	s := receiver.Data.(string)
	if idx < 0 {
		idx = int64(len(s)) + idx + 1
	}
	if idx > int64(len(s)) {
		idx = int64(len(s))
	}
	result := s[:idx] + insertStr + s[idx:]
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSwapcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
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

func stringDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	deleteStr, ok := args[0].Data.(string)
	if !ok {
		return receiver
	}
	s := receiver.Data.(string)
	result := ""
	for i := 0; i < len(s); i++ {
		found := false
		for j := 0; j < len(deleteStr); j++ {
			if s[i] == deleteStr[j] {
				found = true
				break
			}
		}
		if !found {
			result += string(s[i])
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSqueeze(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := string(s[0])
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			result += string(s[i])
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val float64
	for _, c := range s {
		if c >= '0' && c <= '9' || c == '.' {
			// Simple parsing - just convert the first number found
		}
	}
	// Use fmt.Sscanf for proper float parsing
	_, err := fmt.Sscanf(s, "%f", &val)
	if err != nil {
		return &object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  0.0,
			Class: R.Classes["Float"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  val,
		Class: R.Classes["Float"],
	}
}

func stringHex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			var digit int64
			if c >= '0' && c <= '9' {
				digit = int64(c - '0')
			} else if c >= 'a' && c <= 'f' {
				digit = int64(c - 'a' + 10)
			} else {
				digit = int64(c - 'A' + 10)
			}
			val = val*16 + digit
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}

func stringOct(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if c >= '0' && c <= '7' {
			val = val*8 + int64(c-'0')
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}

func stringUnpack(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return R.NilVal
	}
	format, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	if format == "C" || format == "c" {
		result := make([]*object.EmeraldValue, len(s))
		for i, c := range s {
			result[i] = &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(c),
				Class: R.Classes["Integer"],
			}
		}
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  result,
			Class: R.Classes["Array"],
		}
	}
	return R.NilVal
}

func arrayInsert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	if idx < 0 {
		idx = int64(len(arr)) + idx + 1
	}
	if idx > int64(len(arr)) {
		idx = int64(len(arr))
	}
	newArr := make([]*object.EmeraldValue, 0, len(arr)+len(args)-1)
	newArr = append(newArr, arr[:idx]...)
	for i := 1; i < len(args); i++ {
		newArr = append(newArr, args[i])
	}
	newArr = append(newArr, arr[idx:]...)
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arraySlice(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	start := 0
	if args[0].Type == object.ValueInteger {
		start = int(args[0].Data.(int64))
	}
	length := len(arr)
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		length = int(args[1].Data.(int64))
	}
	if start < 0 {
		start = len(arr) + start
	}
	if start < 0 {
		start = 0
	}
	if start > len(arr) {
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{},
			Class: R.Classes["Array"],
		}
	}
	if length > len(arr)-start {
		length = len(arr) - start
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[start : start+length],
		Class: R.Classes["Array"],
	}
}

func arrayValuesAt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, arg := range args {
		if arg.Type == object.ValueInteger {
			idx := int(arg.Data.(int64))
			if idx < 0 {
				idx = len(arr) + idx
			}
			if idx >= 0 && idx < len(arr) {
				result = append(result, arr[idx])
			} else {
				result = append(result, R.NilVal)
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayZip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	other := args[0].Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	maxLen := len(arr)
	if len(other) > maxLen {
		maxLen = len(other)
	}
	for i := 0; i < maxLen; i++ {
		row := make([]*object.EmeraldValue, 0)
		if i < len(arr) {
			row = append(row, arr[i])
		} else {
			row = append(row, R.NilVal)
		}
		if i < len(other) {
			row = append(row, other[i])
		} else {
			row = append(row, R.NilVal)
		}
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  row,
			Class: R.Classes["Array"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayEachIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i := 0; i < len(arr); i++ {
		fmt.Println(i)
	}
	return receiver
}

func arrayEachWithIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i, elem := range arr {
		fmt.Printf("%d: %s\n", i, elem.Inspect())
	}
	return receiver
}

func arrayRotate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return receiver
	}
	n := 1
	if len(args) > 0 && args[0].Type == object.ValueInteger {
		n = int(args[0].Data.(int64))
	}
	n = n % len(arr)
	if n < 0 {
		n += len(arr)
	}
	result := make([]*object.EmeraldValue, len(arr))
	copy(result, arr[n:])
	copy(result[len(arr)-n:], arr[:n])
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayShuffle(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr))
	copy(result, arr)
	for i := len(result) - 1; i > 0; i-- {
		j := i
		result[i], result[j] = result[j], result[i]
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	if idx < 0 {
		idx = int64(len(arr)) + idx
	}
	if idx >= 0 && idx < int64(len(arr)) {
		return arr[idx]
	}
	if len(args) >= 2 {
		return args[1]
	}
	return R.NilVal
}

func arrayReject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if !isTruthy(val) {
			result = append(result, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func hashToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for k, v := range hash {
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{k, v},
			Class: R.Classes["Array"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func hashSelect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if v.Type != object.ValueNil {
			result[k] = v
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func hashReject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if v.Type == object.ValueNil {
			result[k] = v
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func hashTransformKeys(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func hashTransformValues(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func hashAssoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if k.Equals(args[0]) {
			return &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  []*object.EmeraldValue{k, v},
				Class: R.Classes["Array"],
			}
		}
	}
	return R.NilVal
}

func hashRassoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if v.Equals(args[0]) {
			return &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  []*object.EmeraldValue{k, v},
				Class: R.Classes["Array"],
			}
		}
	}
	return R.NilVal
}

func hashShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := receiver.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		delete(hash, k)
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{k, v},
			Class: R.Classes["Array"],
		}
	}
	return R.NilVal
}

func hashReplace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	other, ok := args[0].Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	if !ok {
		return receiver
	}
	receiver.Data = other
	return receiver
}

type SpecRunner struct {
	PassCount    int
	FailCount    int
	SkipCount    int
	ExampleCount int
	Verbose      bool
}

var specRunner *SpecRunner

func InitSpecRunner() *SpecRunner {
	if specRunner == nil {
		specRunner = &SpecRunner{
			PassCount:    0,
			FailCount:    0,
			SkipCount:    0,
			ExampleCount: 0,
			Verbose:      false,
		}
	}
	return specRunner
}

func GetSpecRunner() *SpecRunner {
	return specRunner
}

func RegisterMspec() {
	specRunner = InitSpecRunner()

	expectationClass := object.NewClass("Expectation")
	R.Classes["Expectation"] = expectationClass

	expectationClass.DefineMethod("initialize", &object.Method{
		Name:  "initialize",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				receiver.Data = args[0]
			}
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("should", &object.Method{
		Name:  "should",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("should_not", &object.Method{
		Name:  "should_not",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			specRunner.ExampleCount++
			if len(args) == 0 {
				specRunner.FailCount++
				fmt.Printf("    FAILED: expected a matcher\n")
				return R.NilVal
			}

			actualValue := receiver.Data.(*object.EmeraldValue)
			matcher := args[0]

			if !actualValue.Equals(matcher) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected not %v\n", matcher.Inspect())
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("to", &object.Method{
		Name:  "to",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("not_to", &object.Method{
		Name:  "not_to",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			if receiver.Equals(args[0]) {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("be", &object.Method{
		Name:  "be",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("be_true", &object.Method{
		Name:  "be_true",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueBool && receiver.Data.(bool) == true {
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected true\n")
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("be_false", &object.Method{
		Name:  "be_false",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueBool && receiver.Data.(bool) == false {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected false\n")
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("be_nil", &object.Method{
		Name:  "be_nil",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueNil {
				specRunner.PassCount++
				return R.NilVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected nil, got %v\n", receiver.Inspect())
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("be_an_instance_of", &object.Method{
		Name:  "be_an_instance_of",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			expectedClass, ok := args[0].Data.(*object.Class)
			if !ok {
				return R.NilVal
			}
			if receiver.Class != nil && receiver.Class.Name == expectedClass.Name {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected instance of %s, got %v\n", expectedClass.Name, receiver.Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("include", &object.Method{
		Name:  "include",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("start_with", &object.Method{
		Name:  "start_with",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			actualValue := receiver.Data.(*object.EmeraldValue)
			s, ok1 := actualValue.Data.(string)
			prefix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasPrefix(s, prefix) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to start with %v\n", actualValue.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("start_with?", &object.Method{
		Name:  "start_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			actualValue := receiver.Data.(*object.EmeraldValue)
			s, ok1 := actualValue.Data.(string)
			prefix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasPrefix(s, prefix) {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("end_with", &object.Method{
		Name:  "end_with",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			actualValue := receiver.Data.(*object.EmeraldValue)
			s, ok1 := actualValue.Data.(string)
			suffix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasSuffix(s, suffix) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to end with %v\n", actualValue.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("end_with?", &object.Method{
		Name:  "end_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			actualValue := receiver.Data.(*object.EmeraldValue)
			s, ok1 := actualValue.Data.(string)
			suffix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasSuffix(s, suffix) {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("match", &object.Method{
		Name:  "match",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("empty", &object.Method{
		Name:  "empty",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if s, ok := receiver.Data.(string); ok && len(s) == 0 {
				specRunner.PassCount++
				return R.TrueVal
			}
			if arr, ok := receiver.Data.([]*object.EmeraldValue); ok && len(arr) == 0 {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to be empty\n", receiver.Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod(">", &object.Method{
		Name:  ">",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a > b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v > %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod(">=", &object.Method{
		Name:  ">=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a >= b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v >= %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("<", &object.Method{
		Name:  "<",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a < b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v < %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("<=", &object.Method{
		Name:  "<=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a <= b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v <= %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	objClass := R.Classes["Object"]

	objClass.DefineMethod("describe", &object.Method{
		Name:  "describe",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			specRunner = InitSpecRunner()
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("\n%s\n", desc)
				}
			}
			return R.NilVal
		},
	})

	objClass.DefineMethod("it", &object.Method{
		Name:  "it",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("  ✓ %s\n", desc)
				}
			}
			return R.NilVal
		},
	})

	objClass.DefineMethod("expect", &object.Method{
		Name:  "expect",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			expClass := R.Classes["Expectation"]
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: expClass,
			}
		},
	})

	objClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	objClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	objClass.DefineMethod("it_behaves_like", &object.Method{
		Name:  "it_behaves_like",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			if name, ok := args[0].Data.(string); ok {
				fmt.Printf("  behaves like %s\n", name)
			}
			return R.NilVal
		},
	})
}

// Proc methods
func procCall(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	// For now, return nil - actual implementation requires VM integration
	// This will be properly implemented when block calling is fully integrated
	return R.NilVal
}

func procArity(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.Fn != nil {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(proc.Fn.Params)),
				Class: R.Classes["Integer"],
			}
		}
	} else if receiver.Type == object.ValueClosure {
		closure := receiver.Data.(*object.Closure)
		if closure.Fn != nil {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(closure.Fn.Params)),
				Class: R.Classes["Integer"],
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(0),
		Class: R.Classes["Integer"],
	}
}

func procIsLambda(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.IsLambda {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func moduleInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueModule {
		return R.NilVal
	}
	module := receiver.Data.(*object.Module)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			mixin := arg.Data.(*object.Module)
			module.Include(mixin)
		}
	}
	return R.NilVal
}

func moduleExtend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueModule {
		return R.NilVal
	}
	module := receiver.Data.(*object.Module)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			mixin := arg.Data.(*object.Module)
			module.Extend(mixin)
		}
	}
	return R.NilVal
}

func modulePrepend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func classInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	class := receiver.Data.(*object.Class)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			module := arg.Data.(*object.Module)
			class.Include(module)
		}
	}
	return R.NilVal
}

func classExtend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	class := receiver.Data.(*object.Class)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			module := arg.Data.(*object.Module)
			class.Extend(module)
		}
	}
	return R.NilVal
}

func classPrepend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	class := receiver.Data.(*object.Class)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			module := arg.Data.(*object.Module)
			class.Prepend(module)
		}
	}
	return R.NilVal
}
