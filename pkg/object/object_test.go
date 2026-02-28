package object

import (
	"testing"
)

func TestNewValue(t *testing.T) {
	v := NewValue(ValueInteger, int64(42), nil)
	if v.Type != ValueInteger {
		t.Errorf("expected ValueInteger, got %v", v.Type)
	}
	if v.Data.(int64) != 42 {
		t.Errorf("expected 42, got %v", v.Data)
	}
}

func TestInspect(t *testing.T) {
	tests := []struct {
		name     string
		value    *EmeraldValue
		expected string
	}{
		{"nil", &EmeraldValue{Type: ValueNil}, "nil"},
		{"true", &EmeraldValue{Type: ValueBool, Data: true}, "true"},
		{"false", &EmeraldValue{Type: ValueBool, Data: false}, "false"},
		{"integer", &EmeraldValue{Type: ValueInteger, Data: int64(42)}, "42"},
		{"float", &EmeraldValue{Type: ValueFloat, Data: 3.14}, "3.14"},
		{"string", &EmeraldValue{Type: ValueString, Data: "hello"}, "hello"},
		{"empty array", &EmeraldValue{Type: ValueArray, Data: []*EmeraldValue{}}, "[]"},
		{"array", &EmeraldValue{Type: ValueArray, Data: []*EmeraldValue{
			{Type: ValueInteger, Data: int64(1)},
			{Type: ValueInteger, Data: int64(2)},
		}}, "[1, 2]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.value.Inspect()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTypeName(t *testing.T) {
	tests := []struct {
		vtype    ValueType
		expected string
	}{
		{ValueNil, "NilClass"},
		{ValueBool, "TrueClass"},
		{ValueInteger, "Integer"},
		{ValueFloat, "Float"},
		{ValueString, "String"},
		{ValueArray, "Array"},
		{ValueHash, "Hash"},
		{ValueSymbol, "Symbol"},
		{ValueClass, "Class"},
		{ValueModule, "Module"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			v := &EmeraldValue{Type: tt.vtype}
			if v.TypeName() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, v.TypeName())
			}
		})
	}
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name     string
		value    *EmeraldValue
		expected bool
	}{
		{"nil is falsy", &EmeraldValue{Type: ValueNil}, false},
		{"false is falsy", &EmeraldValue{Type: ValueBool, Data: false}, false},
		{"true is truthy", &EmeraldValue{Type: ValueBool, Data: true}, true},
		{"integer is truthy", &EmeraldValue{Type: ValueInteger, Data: int64(0)}, true},
		{"string is truthy", &EmeraldValue{Type: ValueString, Data: ""}, true},
		{"nil pointer is falsy", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.value.IsTruthy()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEquals(t *testing.T) {
	tests := []struct {
		name     string
		a, b     *EmeraldValue
		expected bool
	}{
		{"nil == nil", &EmeraldValue{Type: ValueNil}, &EmeraldValue{Type: ValueNil}, true},
		{"true == true", &EmeraldValue{Type: ValueBool, Data: true}, &EmeraldValue{Type: ValueBool, Data: true}, true},
		{"true != false", &EmeraldValue{Type: ValueBool, Data: true}, &EmeraldValue{Type: ValueBool, Data: false}, false},
		{"42 == 42", &EmeraldValue{Type: ValueInteger, Data: int64(42)}, &EmeraldValue{Type: ValueInteger, Data: int64(42)}, true},
		{"42 != 43", &EmeraldValue{Type: ValueInteger, Data: int64(42)}, &EmeraldValue{Type: ValueInteger, Data: int64(43)}, false},
		{"3.14 == 3.14", &EmeraldValue{Type: ValueFloat, Data: 3.14}, &EmeraldValue{Type: ValueFloat, Data: 3.14}, true},
		{"hello == hello", &EmeraldValue{Type: ValueString, Data: "hello"}, &EmeraldValue{Type: ValueString, Data: "hello"}, true},
		{"hello != world", &EmeraldValue{Type: ValueString, Data: "hello"}, &EmeraldValue{Type: ValueString, Data: "world"}, false},
		{"int != string", &EmeraldValue{Type: ValueInteger, Data: int64(1)}, &EmeraldValue{Type: ValueString, Data: "1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Equals(tt.b)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNewClass(t *testing.T) {
	c := NewClass("Person")
	if c.Name != "Person" {
		t.Errorf("expected name Person, got %s", c.Name)
	}
	if c.Methods == nil {
		t.Error("Methods map should be initialized")
	}
	if c.ClassMethods == nil {
		t.Error("ClassMethods map should be initialized")
	}
}

func TestClassMethodDefinition(t *testing.T) {
	c := NewClass("Foo")
	m := &Method{Name: "bar", Arity: 0}
	c.DefineMethod("bar", m)

	got, ok := c.GetMethod("bar")
	if !ok {
		t.Fatal("expected method to be found")
	}
	if got.Name != "bar" {
		t.Errorf("expected method name bar, got %s", got.Name)
	}
}

func TestClassInheritance(t *testing.T) {
	parent := NewClass("Animal")
	parent.DefineMethod("speak", &Method{Name: "speak"})

	child := NewClass("Dog")
	child.SuperClass = parent

	// Child should find parent's method
	m, ok := child.GetMethod("speak")
	if !ok {
		t.Fatal("expected to find inherited method")
	}
	if m.Name != "speak" {
		t.Errorf("expected method name speak, got %s", m.Name)
	}

	// Child's own method should override parent
	child.DefineMethod("speak", &Method{Name: "speak-override"})
	m, ok = child.GetMethod("speak")
	if !ok {
		t.Fatal("expected to find overridden method")
	}
	if m.Name != "speak-override" {
		t.Errorf("expected overridden method, got %s", m.Name)
	}
}

func TestClassNewInstance(t *testing.T) {
	c := NewClass("Person")
	instance := c.NewInstance()

	if instance.Type != ValueObject {
		t.Errorf("expected ValueObject, got %v", instance.Type)
	}
	if instance.Class != c {
		t.Error("instance class should reference the class")
	}

	obj := instance.Data.(*Object)
	if obj.Class != c {
		t.Error("object class should reference the class")
	}
}

func TestObjectInstanceVars(t *testing.T) {
	c := NewClass("Person")
	obj := NewObject(c)

	nameVal := &EmeraldValue{Type: ValueString, Data: "Alice"}
	obj.SetInstanceVar("@name", nameVal)

	got := obj.GetInstanceVar("@name")
	if got == nil {
		t.Fatal("expected instance var to be set")
	}
	if got.Data.(string) != "Alice" {
		t.Errorf("expected Alice, got %v", got.Data)
	}

	// Non-existent var returns nil
	if obj.GetInstanceVar("@age") != nil {
		t.Error("expected nil for non-existent var")
	}
}

func TestObjectRespondTo(t *testing.T) {
	c := NewClass("Foo")
	c.DefineMethod("bar", &Method{Name: "bar"})
	obj := NewObject(c)

	if !obj.RespondTo("bar") {
		t.Error("expected object to respond to bar")
	}
	if obj.RespondTo("baz") {
		t.Error("expected object not to respond to baz")
	}
}

func TestNewModule(t *testing.T) {
	m := NewModule("Enumerable")
	if m.Name != "Enumerable" {
		t.Errorf("expected name Enumerable, got %s", m.Name)
	}
}

func TestModuleMethodLookup(t *testing.T) {
	parent := NewModule("Base")
	parent.DefineMethod("base_method", &Method{Name: "base_method"})

	child := NewModule("Child")
	child.Parent = parent

	m, ok := child.GetMethod("base_method")
	if !ok {
		t.Fatal("expected to find parent method")
	}
	if m.Name != "base_method" {
		t.Errorf("expected base_method, got %s", m.Name)
	}
}

func TestModuleInclude(t *testing.T) {
	enumerable := NewModule("Enumerable")
	enumerable.DefineMethod("each", &Method{Name: "each"})
	enumerable.DefineMethod("map", &Method{Name: "map"})

	target := NewModule("MyModule")
	target.DefineMethod("map", &Method{Name: "my_map"}) // existing method

	target.Include(enumerable)

	// "each" should be included
	m, ok := target.GetMethod("each")
	if !ok {
		t.Fatal("expected each to be included")
	}
	if m.Name != "each" {
		t.Errorf("expected each, got %s", m.Name)
	}

	// "map" should NOT be overwritten (Include doesn't overwrite)
	m, _ = target.GetMethod("map")
	if m.Name != "my_map" {
		t.Errorf("Include should not overwrite existing methods, got %s", m.Name)
	}
}

func TestModuleExtend(t *testing.T) {
	mixin := NewModule("Mixin")
	mixin.DefineMethod("helper", &Method{Name: "helper"})

	target := NewModule("Target")
	target.DefineMethod("helper", &Method{Name: "old_helper"})

	target.Extend(mixin)

	// Extend DOES overwrite
	m, _ := target.GetMethod("helper")
	if m.Name != "helper" {
		t.Errorf("Extend should overwrite, got %s", m.Name)
	}
}

func TestClassConstants(t *testing.T) {
	parent := NewClass("Base")
	val := &EmeraldValue{Type: ValueInteger, Data: int64(42)}
	parent.DefineConstant("VERSION", val)

	child := NewClass("Child")
	child.SuperClass = parent

	got, ok := child.GetConstant("VERSION")
	if !ok {
		t.Fatal("expected to find inherited constant")
	}
	if got.Data.(int64) != 42 {
		t.Errorf("expected 42, got %v", got.Data)
	}
}
