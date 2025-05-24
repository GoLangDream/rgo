package rgo

import (
	"testing"
)

func TestRClassOptimized(t *testing.T) {
	// 测试类创建
	person := ClassOptimized("Person")
	if person.Name() != "Person" {
		t.Errorf("Expected class name to be 'Person', got '%s'", person.Name())
	}

	// 测试属性访问器
	person.AttrAccessor("age")

	// 测试方法缺失处理
	person.MethodMissing(func(name string, args ...any) any {
		return "Method not found"
	})

	// 测试实例方法
	person.DefineMethod("say_hello", func() string {
		return "Hello!"
	})

	// 测试类方法
	person.DefineClassMethod("create", func(name string) *RClassOptimized {
		instance := person.New()
		instance.SetInstanceVar("name", name)
		return instance
	})

	// 测试实例创建和方法调用
	john := person.New()
	result := john.Call("say_hello").(string)
	if result != "Hello!" {
		t.Errorf("Expected 'Hello!', got '%s'", result)
	}

	// 测试类方法调用
	mary := person.Call("create", "Mary").(*RClassOptimized)
	name := mary.GetInstanceVar("name").(string)
	if name != "Mary" {
		t.Errorf("Expected 'Mary', got '%s'", name)
	}

	// 测试属性访问器
	john.Call("age=", 30)
	age := john.Call("age").(int)
	if age != 30 {
		t.Errorf("Expected 30, got %d", age)
	}

	// 测试继承
	employee := ClassOptimized("Employee")
	employee.Inherit(person)
	employee.DefineMethod("say_hello", func() string {
		return "Hello, I am an employee!"
	})

	bob := employee.New()
	result = bob.Call("say_hello").(string)
	if result != "Hello, I am an employee!" {
		t.Errorf("Expected 'Hello, I am an employee!', got '%s'", result)
	}

	// 测试父类方法调用
	bob.Call("age=", 35)
	age = bob.Call("age").(int)
	if age != 35 {
		t.Errorf("Expected 35, got %d", age)
	}

	// 测试类型检查
	if !bob.IsA("Employee") {
		t.Error("Expected bob to be an Employee")
	}
	if !bob.IsA("Person") {
		t.Error("Expected bob to be a Person")
	}
	if bob.IsA("Animal") {
		t.Error("Expected bob not to be an Animal")
	}

	// 测试方法缺失处理
	result = john.Call("undefined_method").(string)
	if result != "Method not found" {
		t.Errorf("Expected 'Method not found', got '%s'", result)
	}

	// 测试方法列表
	methods := person.Methods()
	if len(methods) < 2 {
		t.Errorf("Expected at least 2 methods, got %d", len(methods))
	}

	classMethods := person.ClassMethods()
	if len(classMethods) < 1 {
		t.Errorf("Expected at least 1 class method, got %d", len(classMethods))
	}

	// 测试变量列表
	john.SetInstanceVar("name", "John")
	instanceVars := john.InstanceVars()
	if len(instanceVars) < 1 {
		t.Errorf("Expected at least 1 instance variable, got %d", len(instanceVars))
	}

	person.SetClassVar("count", 0)
	classVars := person.ClassVars()
	if len(classVars) < 1 {
		t.Errorf("Expected at least 1 class variable, got %d", len(classVars))
	}
}

// 测试方法缓存功能
func TestRClassOptimizedMethodCache(t *testing.T) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("test_method", func() string {
		return "cached"
	})

	instance := class.New()

	// 第一次调用
	result1 := instance.Call("test_method").(string)
	// 第二次调用（应该使用缓存）
	result2 := instance.Call("test_method").(string)

	if result1 != "cached" || result2 != "cached" {
		t.Errorf("Method cache test failed")
	}
}

// 测试快速调用功能
func TestRClassOptimizedFastCall(t *testing.T) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("fast_method", func() string {
		return "fast"
	})

	instance := class.New()

	// 测试快速调用
	result := instance.FastCall("fast_method").(string)
	if result != "fast" {
		t.Errorf("Expected 'fast', got '%s'", result)
	}
}

// 测试对象池功能
func TestRClassOptimizedPool(t *testing.T) {
	class := ClassOptimized("PoolTest")
	class.DefineMethod("pool_method", func() string {
		return "pooled"
	})

	// 从池创建实例
	instance := class.NewFromPool()
	result := instance.Call("pool_method").(string)
	if result != "pooled" {
		t.Errorf("Expected 'pooled', got '%s'", result)
	}

	// 返回到池
	instance.ReturnToPool()
}

// 测试直接方法调用
func TestRClassOptimizedDirectCall(t *testing.T) {
	class := ClassOptimized("DirectTest")
	class.DefineMethod("direct_method", func() string {
		return "direct"
	})

	instance := class.New()

	// 获取方法指针
	method := instance.GetMethodDirectly("direct_method")
	if method == nil {
		t.Error("Method should not be nil")
	}

	// 直接调用
	result := method.CallDirect(instance).(string)
	if result != "direct" {
		t.Errorf("Expected 'direct', got '%s'", result)
	}
}
