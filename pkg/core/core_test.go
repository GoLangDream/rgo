package core

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/object"
)

func init() {
	Init()
}

func mkInt(v int64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: v, Class: R.Classes["Integer"]}
}

func mkFloat(v float64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueFloat, Data: v, Class: R.Classes["Float"]}
}

func mkStr(v string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueString, Data: v, Class: R.Classes["String"]}
}

func mkArr(elems ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: elems, Class: R.Classes["Array"]}
}

func callMethod(t *testing.T, receiver *object.EmeraldValue, name string, args ...*object.EmeraldValue) *object.EmeraldValue {
	t.Helper()
	method, ok := receiver.Class.GetMethod(name)
	if !ok {
		t.Fatalf("method %s not found on %s", name, receiver.Class.Name)
	}
	fn := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)
	return fn(receiver, args...)
}

func assertInt(t *testing.T, val *object.EmeraldValue, expected int64) {
	t.Helper()
	if val.Type != object.ValueInteger {
		t.Fatalf("expected Integer, got %v", val.Type)
	}
	if val.Data.(int64) != expected {
		t.Errorf("expected %d, got %d", expected, val.Data.(int64))
	}
}

func assertFloat(t *testing.T, val *object.EmeraldValue, expected float64) {
	t.Helper()
	if val.Type != object.ValueFloat {
		t.Fatalf("expected Float, got %v", val.Type)
	}
	if val.Data.(float64) != expected {
		t.Errorf("expected %f, got %f", expected, val.Data.(float64))
	}
}

func assertStr(t *testing.T, val *object.EmeraldValue, expected string) {
	t.Helper()
	if val.Type != object.ValueString {
		t.Fatalf("expected String, got %v", val.Type)
	}
	if val.Data.(string) != expected {
		t.Errorf("expected %q, got %q", expected, val.Data.(string))
	}
}

func assertBool(t *testing.T, val *object.EmeraldValue, expected bool) {
	t.Helper()
	if expected {
		if val != R.TrueVal {
			t.Errorf("expected true, got %v", val)
		}
	} else {
		if val != R.FalseVal {
			t.Errorf("expected false, got %v", val)
		}
	}
}

func assertNil(t *testing.T, val *object.EmeraldValue) {
	t.Helper()
	if val != R.NilVal {
		t.Errorf("expected nil, got %v", val)
	}
}

// === Init ===

func TestInitCreatesClasses(t *testing.T) {
	expected := []string{
		"BasicObject", "Object", "Module", "Class",
		"TrueClass", "FalseClass", "NilClass",
		"Integer", "Float", "String", "Array", "Hash",
		"Symbol", "Regexp", "Range",
	}
	for _, name := range expected {
		if _, ok := R.Classes[name]; !ok {
			t.Errorf("class %s not found", name)
		}
	}
}

func TestInitClassHierarchy(t *testing.T) {
	if R.Classes["Object"].SuperClass != R.Classes["BasicObject"] {
		t.Error("Object should inherit from BasicObject")
	}
	if R.Classes["Integer"].SuperClass != R.Classes["Object"] {
		t.Error("Integer should inherit from Object")
	}
	if R.Classes["String"].SuperClass != R.Classes["Object"] {
		t.Error("String should inherit from Object")
	}
}

func TestInitSingletons(t *testing.T) {
	if R.TrueVal == nil || R.TrueVal.Data != true {
		t.Error("TrueVal not initialized")
	}
	if R.FalseVal == nil || R.FalseVal.Data != false {
		t.Error("FalseVal not initialized")
	}
	if R.NilVal == nil || R.NilVal.Type != object.ValueNil {
		t.Error("NilVal not initialized")
	}
}

// === Integer Methods ===

func TestIntAdd(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(3), "+", mkInt(4)), 7)
}

func TestIntAddFloat(t *testing.T) {
	assertFloat(t, callMethod(t, mkInt(3), "+", mkFloat(1.5)), 4.5)
}

func TestIntSub(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(10), "-", mkInt(3)), 7)
}

func TestIntMul(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(3), "*", mkInt(4)), 12)
}

func TestIntDiv(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(10), "/", mkInt(3)), 3)
}

func TestIntDivByZero(t *testing.T) {
	assertNil(t, callMethod(t, mkInt(10), "/", mkInt(0)))
}

func TestIntMod(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(17), "%", mkInt(5)), 2)
}

func TestIntModByZero(t *testing.T) {
	assertNil(t, callMethod(t, mkInt(17), "%", mkInt(0)))
}

func TestIntPow(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(2), "**", mkInt(10)), 1024)
}

func TestIntToS(t *testing.T) {
	assertStr(t, callMethod(t, mkInt(42), "to_s"), "42")
}

func TestIntSucc(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(5), "succ"), 6)
}

func TestIntPred(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(5), "pred"), 4)
}

func TestIntChr(t *testing.T) {
	assertStr(t, callMethod(t, mkInt(65), "chr"), "A")
}

func TestIntOdd(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(3), "odd?"), true)
	assertBool(t, callMethod(t, mkInt(4), "odd?"), false)
}

func TestIntEven(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(4), "even?"), true)
	assertBool(t, callMethod(t, mkInt(3), "even?"), false)
}

func TestIntZero(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(0), "zero?"), true)
	assertBool(t, callMethod(t, mkInt(1), "zero?"), false)
}

func TestIntAbs(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(-5), "abs"), 5)
	assertInt(t, callMethod(t, mkInt(5), "abs"), 5)
}

func TestIntToF(t *testing.T) {
	assertFloat(t, callMethod(t, mkInt(5), "to_f"), 5.0)
}

// === Float Methods ===

func TestFloatAdd(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(1.5), "+", mkFloat(2.5)), 4.0)
}

func TestFloatAddInt(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(1.5), "+", mkInt(2)), 3.5)
}

func TestFloatSub(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(5.5), "-", mkFloat(2.0)), 3.5)
}

func TestFloatMul(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(2.5), "*", mkFloat(4.0)), 10.0)
}

func TestFloatDiv(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(10.0), "/", mkFloat(4.0)), 2.5)
}

func TestFloatDivByZero(t *testing.T) {
	assertNil(t, callMethod(t, mkFloat(10.0), "/", mkFloat(0)))
}

func TestFloatToS(t *testing.T) {
	assertStr(t, callMethod(t, mkFloat(3.14), "to_s"), "3.14")
}

func TestFloatToI(t *testing.T) {
	assertInt(t, callMethod(t, mkFloat(3.14), "to_i"), 3)
}

// === String Methods ===

func TestStringAdd(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "+", mkStr(" world")), "hello world")
}

func TestStringMul(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("ab"), "*", mkInt(3)), "ababab")
}

func TestStringLength(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("hello"), "length"), 5)
}

func TestStringSize(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("hello"), "size"), 5)
}

func TestStringEmpty(t *testing.T) {
	assertBool(t, callMethod(t, mkStr(""), "empty?"), true)
	assertBool(t, callMethod(t, mkStr("x"), "empty?"), false)
}

func TestStringUpcase(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "upcase"), "HELLO")
}

func TestStringDowncase(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("HELLO"), "downcase"), "hello")
}

func TestStringCapitalize(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "capitalize"), "Hello")
}

func TestStringReverse(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "reverse"), "olleh")
}

func TestStringInclude(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello world"), "include?", mkStr("world")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "include?", mkStr("xyz")), false)
}

func TestStringStartWith(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello"), "start_with?", mkStr("hel")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "start_with?", mkStr("xyz")), false)
}

func TestStringEndWith(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello"), "end_with?", mkStr("llo")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "end_with?", mkStr("xyz")), false)
}

func TestStringIndex(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(0)), "h")
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(4)), "o")
}

func TestStringIndexNegative(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(-1)), "o")
}

func TestStringIndexOutOfBounds(t *testing.T) {
	assertNil(t, callMethod(t, mkStr("hello"), "[]", mkInt(10)))
}

func TestStringToI(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("42"), "to_i"), 42)
}

func TestStringToS(t *testing.T) {
	s := mkStr("hello")
	result := callMethod(t, s, "to_s")
	if result != s {
		t.Error("to_s should return self for strings")
	}
}

// === Array Methods ===

func TestArrayLength(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2), mkInt(3))
	assertInt(t, callMethod(t, arr, "length"), 3)
}

func TestArrayFirst(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20))
	assertInt(t, callMethod(t, arr, "first"), 10)
}

func TestArrayFirstEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "first"))
}

func TestArrayLast(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20))
	assertInt(t, callMethod(t, arr, "last"), 20)
}

func TestArrayLastEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "last"))
}

func TestArrayPush(t *testing.T) {
	arr := mkArr(mkInt(1))
	result := callMethod(t, arr, "push", mkInt(2))
	elems := result.Data.([]*object.EmeraldValue)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	assertInt(t, elems[1], 2)
}

func TestArrayPop(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2))
	assertInt(t, callMethod(t, arr, "pop"), 2)
}

func TestArrayPopEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "pop"))
}

func TestArrayEmpty(t *testing.T) {
	assertBool(t, callMethod(t, mkArr(), "empty?"), true)
	assertBool(t, callMethod(t, mkArr(mkInt(1)), "empty?"), false)
}

func TestArrayReverse(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2), mkInt(3))
	result := callMethod(t, arr, "reverse")
	elems := result.Data.([]*object.EmeraldValue)
	assertInt(t, elems[0], 3)
	assertInt(t, elems[1], 2)
	assertInt(t, elems[2], 1)
}

func TestArrayIndex(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20), mkInt(30))
	assertInt(t, callMethod(t, arr, "[]", mkInt(1)), 20)
}

func TestArrayIndexNegative(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20), mkInt(30))
	assertInt(t, callMethod(t, arr, "[]", mkInt(-1)), 30)
}

func TestArrayIndexOutOfBounds(t *testing.T) {
	arr := mkArr(mkInt(1))
	assertNil(t, callMethod(t, arr, "[]", mkInt(5)))
}

// === Object Methods ===

func TestObjectNilQuestion(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(1), "nil?"), false)
	assertBool(t, callMethod(t, R.NilVal, "nil?"), true)
}

func TestObjectRespondTo(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(1), "respond_to?", mkStr("+")), true)
	assertBool(t, callMethod(t, mkInt(1), "respond_to?", mkStr("nonexistent")), false)
}

func TestObjectToS(t *testing.T) {
	result := callMethod(t, mkInt(42), "to_s")
	if result.Type != object.ValueString {
		t.Errorf("expected String, got %v", result.Type)
	}
}
