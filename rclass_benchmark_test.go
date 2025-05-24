package rgo

import (
	"testing"
)

// =============================================================================
// RClass 高性能版性能测试
// =============================================================================

// BenchmarkRClassNewInstance 测试 RClass 创建性能
func BenchmarkRClassNewInstance(b *testing.B) {
	class := Class("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = class.New()
	}
}

// BenchmarkRClassNewWithPool 测试 RClass 对象池创建性能
func BenchmarkRClassNewWithPool(b *testing.B) {
	class := Class("TestClass")
	class.EnablePooling()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.New()
		instance.ReturnToPool()
	}
}

// BenchmarkRClassMethodCallNoArgs 测试 RClass 无参数方法调用性能
func BenchmarkRClassMethodCallNoArgs(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// BenchmarkRClassFastCallNoArgs 测试 RClass 快速方法调用性能
func BenchmarkRClassFastCallNoArgs(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.FastCall("greet")
	}
}

// BenchmarkRClassDirectCallNoArgs 测试 RClass 直接方法调用性能
func BenchmarkRClassDirectCallNoArgs(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	method := instance.GetMethodDirectly("greet")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = method.CallDirect(instance)
	}
}

// BenchmarkRClassMethodCallWithSelf 测试带 self 参数的方法调用性能
func BenchmarkRClassMethodCallWithSelf(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("getName", func(self *RClass) string {
		name := self.GetInstanceVar("name")
		if name == nil {
			return "anonymous"
		}
		return name.(string)
	})
	instance := class.New()
	instance.SetInstanceVar("name", "test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("getName")
	}
}

// BenchmarkRClassMethodCallWithOneArg 测试带单个参数的方法调用性能
func BenchmarkRClassMethodCallWithOneArg(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func(name any) string {
		return "Hello, " + name.(string)
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet", "World")
	}
}

// BenchmarkRClassMethodCallWithTwoArgs 测试带两个参数的方法调用性能
func BenchmarkRClassMethodCallWithTwoArgs(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("add", func(a any, b any) any {
		return a.(int) + b.(int)
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("add", 1, 2)
	}
}

// BenchmarkRClassInstanceVarOperations 测试 RClass 实例变量操作性能
func BenchmarkRClassInstanceVarOperations(b *testing.B) {
	class := Class("TestClass")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.SetInstanceVar("name", "test")
		_ = instance.GetInstanceVar("name")
	}
}

// BenchmarkRClassClassVarOperations 测试 RClass 类变量操作性能
func BenchmarkRClassClassVarOperations(b *testing.B) {
	class := Class("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		class.SetClassVar("count", i)
		_ = class.GetClassVar("count")
	}
}

// BenchmarkRClassAttrAccessor 测试 RClass 属性访问器性能
func BenchmarkRClassAttrAccessor(b *testing.B) {
	class := Class("TestClass")
	class.AttrAccessor("name")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.Call("name=", "test")
		_ = instance.Call("name")
	}
}

// BenchmarkRClassMethodDefinition 测试 RClass 方法定义性能
func BenchmarkRClassMethodDefinition(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		class := Class("TestClass")
		class.DefineMethod("test_method", func() string {
			return "test"
		})
	}
}

// BenchmarkRClassInheritance 测试 RClass 继承性能
func BenchmarkRClassInheritance(b *testing.B) {
	parent := Class("Parent")
	parent.DefineMethod("parent_method", func() string {
		return "parent"
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		child := Class("Child")
		child.Inherit(parent)
		instance := child.New()
		_ = instance.Call("parent_method")
	}
}

// BenchmarkRClassSuperCall 测试 RClass Super 方法调用性能
func BenchmarkRClassSuperCall(b *testing.B) {
	parent := Class("Parent")
	parent.DefineMethod("greet", func() string {
		return "Hello from parent"
	})

	child := Class("Child")
	child.Inherit(parent)
	child.DefineMethod("greet", func(self *RClass) string {
		parentGreet := self.Super("greet").(string)
		return parentGreet + " and child"
	})

	instance := child.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// =============================================================================
// 元编程功能性能测试
// =============================================================================

// BenchmarkRClassMethodMissing 测试 method_missing 性能
func BenchmarkRClassMethodMissing(b *testing.B) {
	class := Class("TestClass")
	class.MethodMissing(func(name string, args ...any) any {
		return "Method " + name + " not found"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("unknown_method")
	}
}

// BenchmarkRClassAlias 测试方法别名性能
func BenchmarkRClassAlias(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello"
	})
	class.Alias("hi", "greet")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("hi")
	}
}

// BenchmarkRClassHooks 测试钩子函数性能
func BenchmarkRClassHooks(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello"
	})
	class.Before("greet", func(args ...any) {
		// Before hook
	})
	class.After("greet", func(result any, args ...any) {
		// After hook
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// BenchmarkRClassEigenMethod 测试元类方法性能
func BenchmarkRClassEigenMethod(b *testing.B) {
	class := Class("TestClass")
	instance := class.New()
	instance.DefineEigenMethod("special", func() string {
		return "special"
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("special")
	}
}

// =============================================================================
// 复杂场景性能测试
// =============================================================================

// BenchmarkRClassComplexScenario 测试 RClass 复杂场景性能
func BenchmarkRClassComplexScenario(b *testing.B) {
	// 创建类层次结构
	animal := Class("Animal")
	animal.DefineMethod("speak", func() string {
		return "Some sound"
	})
	animal.DefineMethod("move", func() string {
		return "Moving"
	})
	animal.AttrAccessor("name", "age")

	dog := Class("Dog")
	dog.Inherit(animal)
	dog.DefineMethod("speak", func(self *RClass) string {
		return "Woof! " + self.Super("speak").(string)
	})
	dog.DefineMethod("wagTail", func() string {
		return "Wagging tail"
	})

	employee := Class("Employee")
	employee.Inherit(animal) // 多重继承模拟
	employee.DefineMethod("work", func() string {
		return "Working"
	})
	employee.AttrAccessor("salary")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 创建实例
		buddy := dog.New()
		john := employee.New()

		// 设置属性
		buddy.Call("name=", "Buddy")
		buddy.Call("age=", 3)
		john.Call("name=", "John")
		john.Call("salary=", 50000)

		// 调用方法
		_ = buddy.Call("speak")
		_ = buddy.Call("move")
		_ = buddy.Call("wagTail")
		_ = john.Call("work")
		_ = john.Call("speak")

		// 访问属性
		_ = buddy.Call("name")
		_ = john.Call("salary")
	}
}

// BenchmarkRClassMemoryAllocation 测试 RClass 内存分配
func BenchmarkRClassMemoryAllocation(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("test_method", func() string {
		return "test"
	})
	class.AttrAccessor("name")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.New()
		instance.Call("name=", "test")
		_ = instance.Call("name")
		_ = instance.Call("test_method")
	}
}

// BenchmarkRClassPoolMemoryAllocation 测试 RClass 对象池内存分配
func BenchmarkRClassPoolMemoryAllocation(b *testing.B) {
	class := Class("TestClass")
	class.EnablePooling()
	class.DefineMethod("test_method", func() string {
		return "test"
	})
	class.AttrAccessor("name")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.New()
		instance.Call("name=", "test")
		_ = instance.Call("name")
		_ = instance.Call("test_method")
		instance.ReturnToPool()
	}
}

// =============================================================================
// 并发安全性能测试
// =============================================================================

// BenchmarkRClassConcurrentMethodCall 测试并发方法调用性能
func BenchmarkRClassConcurrentMethodCall(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = instance.Call("greet")
		}
	})
}

// BenchmarkRClassConcurrentInstanceVar 测试并发实例变量操作性能
func BenchmarkRClassConcurrentInstanceVar(b *testing.B) {
	class := Class("TestClass")
	instance := class.New()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			instance.SetInstanceVar("counter", i)
			_ = instance.GetInstanceVar("counter")
			i++
		}
	})
}

// BenchmarkRClassConcurrentClassVar 测试并发类变量操作性能
func BenchmarkRClassConcurrentClassVar(b *testing.B) {
	class := Class("TestClass")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			class.SetClassVar("counter", i)
			_ = class.GetClassVar("counter")
			i++
		}
	})
}

// =============================================================================
// 性能对比测试（与原生 Go 方法调用对比）
// =============================================================================

type testStruct struct {
	name string
}

func (t *testStruct) greet() string {
	return "Hello World"
}

// BenchmarkNativeGoMethodCall 测试原生 Go 方法调用性能（对照组）
func BenchmarkNativeGoMethodCall(b *testing.B) {
	instance := &testStruct{name: "test"}
	greet := func() string {
		return "Hello World"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = greet()
		_ = instance.name
	}
}

type Greeter interface {
	greet() string
}

// BenchmarkNativeGoInterfaceCall 测试原生 Go 接口调用性能（对照组）
func BenchmarkNativeGoInterfaceCall(b *testing.B) {
	var greeter Greeter = &testStruct{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = greeter.greet()
	}
}

// =============================================================================
// 方法缓存性能测试
// =============================================================================

// BenchmarkRClassMethodCacheHit 测试方法缓存命中性能
func BenchmarkRClassMethodCacheHit(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()

	// 预热缓存
	instance.Call("greet")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// BenchmarkRClassMethodCacheMiss 测试方法缓存未命中性能
func BenchmarkRClassMethodCacheMiss(b *testing.B) {
	class := Class("TestClass")
	for i := 0; i < 26; i++ {
		methodName := "method" + string(rune('A'+i))
		class.DefineMethod(methodName, func() string {
			return "result"
		})
	}
	instance := class.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		methodName := "method" + string(rune('A'+i%26))
		_ = instance.Call(methodName)
	}
}

// =============================================================================
// 继承链性能测试
// =============================================================================

// BenchmarkRClassDeepInheritance 测试深继承链性能
func BenchmarkRClassDeepInheritance(b *testing.B) {
	// 创建深度为10的继承链
	classes := make([]*RClass, 10)
	classes[0] = Class("Level0")
	classes[0].DefineMethod("baseMethod", func() string {
		return "base"
	})

	for i := 1; i < 10; i++ {
		levelName := "Level" + string(rune('0'+i))
		classes[i] = Class(levelName)
		classes[i].Inherit(classes[i-1])

		methodName := "method" + string(rune('0'+i))
		levelStr := "level" + string(rune('0'+i))
		classes[i].DefineMethod(methodName, func() string {
			return levelStr
		})
	}

	instance := classes[9].New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("baseMethod") // 调用最顶层的方法
	}
}
