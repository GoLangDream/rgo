package rgo

import (
	"testing"
)

// =============================================================================
// RClass 原版 vs 优化版性能测试
// =============================================================================

// BenchmarkRClassOriginalNew 测试原版 RClass 创建性能
func BenchmarkRClassOriginalNew(b *testing.B) {
	class := Class("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = class.New()
	}
}

// BenchmarkRClassOptimizedNew 测试优化版 RClass 创建性能
func BenchmarkRClassOptimizedNew(b *testing.B) {
	class := ClassOptimized("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = class.New()
	}
}

// BenchmarkRClassOriginalMethodCall 测试原版 RClass 方法调用性能
func BenchmarkRClassOriginalMethodCall(b *testing.B) {
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

// BenchmarkRClassOptimizedMethodCall 测试优化版 RClass 方法调用性能
func BenchmarkRClassOptimizedMethodCall(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// BenchmarkRClassOptimizedFastCall 测试优化版 RClass 快速方法调用性能
func BenchmarkRClassOptimizedFastCall(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.FastCall("greet")
	}
}

// BenchmarkRClassOptimizedDirectCall 测试优化版 RClass 直接方法调用性能
func BenchmarkRClassOptimizedDirectCall(b *testing.B) {
	class := ClassOptimized("TestClass")
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

// BenchmarkRClassOriginalInstanceVar 测试原版 RClass 实例变量操作性能
func BenchmarkRClassOriginalInstanceVar(b *testing.B) {
	class := Class("TestClass")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.SetInstanceVar("name", "test")
		_ = instance.GetInstanceVar("name")
	}
}

// BenchmarkRClassOptimizedInstanceVar 测试优化版 RClass 实例变量操作性能
func BenchmarkRClassOptimizedInstanceVar(b *testing.B) {
	class := ClassOptimized("TestClass")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.SetInstanceVar("name", "test")
		_ = instance.GetInstanceVar("name")
	}
}

// BenchmarkRClassOriginalAttrAccessor 测试原版 RClass 属性访问器性能
func BenchmarkRClassOriginalAttrAccessor(b *testing.B) {
	class := Class("TestClass")
	class.AttrAccessor("name")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.Call("name=", instance, "test")
		_ = instance.Call("name", instance)
	}
}

// BenchmarkRClassOptimizedAttrAccessor 测试优化版 RClass 属性访问器性能
func BenchmarkRClassOptimizedAttrAccessor(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.AttrAccessor("name")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.Call("name=", "test")
		_ = instance.Call("name")
	}
}

// BenchmarkRClassOriginalMethodDefinition 测试原版 RClass 方法定义性能
func BenchmarkRClassOriginalMethodDefinition(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		class := Class("TestClass")
		class.DefineMethod("test_method", func() string {
			return "test"
		})
	}
}

// BenchmarkRClassOptimizedMethodDefinition 测试优化版 RClass 方法定义性能
func BenchmarkRClassOptimizedMethodDefinition(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		class := ClassOptimized("TestClass")
		class.DefineMethod("test_method", func() string {
			return "test"
		})
	}
}

// BenchmarkRClassOriginalInheritance 测试原版 RClass 继承性能
func BenchmarkRClassOriginalInheritance(b *testing.B) {
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

// BenchmarkRClassOptimizedInheritance 测试优化版 RClass 继承性能
func BenchmarkRClassOptimizedInheritance(b *testing.B) {
	parent := ClassOptimized("Parent")
	parent.DefineMethod("parent_method", func() string {
		return "parent"
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		child := ClassOptimized("Child")
		child.Inherit(parent)
		instance := child.New()
		_ = instance.Call("parent_method")
	}
}

// BenchmarkRClassOptimizedPool 测试优化版 RClass 对象池性能
func BenchmarkRClassOptimizedPool(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("test_method", func() string {
		return "test"
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.NewFromPool()
		_ = instance.Call("test_method")
		instance.ReturnToPool()
	}
}

// =============================================================================
// 复杂场景性能测试
// =============================================================================

// BenchmarkRClassOriginalComplexScenario 测试原版 RClass 复杂场景性能
func BenchmarkRClassOriginalComplexScenario(b *testing.B) {
	// 创建类层次结构
	animal := Class("Animal")
	animal.DefineMethod("speak", func(self *RClass) string {
		return "Some sound"
	})
	animal.AttrAccessor("name")

	dog := Class("Dog")
	dog.Inherit(animal)
	dog.DefineMethod("speak", func(self *RClass) string {
		return "Woof!"
	})
	dog.DefineMethod("fetch", func(self *RClass) string {
		return "Fetching..."
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 创建实例
		myDog := dog.New()

		// 设置属性
		myDog.Call("name=", myDog, "Buddy")

		// 调用方法
		_ = myDog.Call("name", myDog)
		_ = myDog.Call("speak", myDog)
		_ = myDog.Call("fetch", myDog)

		// 检查类型
		_ = myDog.IsA("Dog")
		_ = myDog.IsA("Animal")
	}
}

// BenchmarkRClassOptimizedComplexScenario 测试优化版 RClass 复杂场景性能
func BenchmarkRClassOptimizedComplexScenario(b *testing.B) {
	// 创建类层次结构
	animal := ClassOptimized("Animal")
	animal.DefineMethod("speak", func(self *RClassOptimized) string {
		return "Some sound"
	})
	animal.AttrAccessor("name")

	dog := ClassOptimized("Dog")
	dog.Inherit(animal)
	dog.DefineMethod("speak", func(self *RClassOptimized) string {
		return "Woof!"
	})
	dog.DefineMethod("fetch", func(self *RClassOptimized) string {
		return "Fetching..."
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 创建实例
		myDog := dog.New()

		// 设置属性
		myDog.Call("name=", "Buddy")

		// 调用方法
		_ = myDog.Call("name")
		_ = myDog.Call("speak")
		_ = myDog.Call("fetch")

		// 检查类型
		_ = myDog.IsA("Dog")
		_ = myDog.IsA("Animal")
	}
}

// =============================================================================
// 内存分配测试
// =============================================================================

// BenchmarkRClassOriginalMemoryAllocation 测试原版 RClass 内存分配
func BenchmarkRClassOriginalMemoryAllocation(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("test", func() string {
		return "test"
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.New()
		_ = instance.Call("test")
	}
}

// BenchmarkRClassOptimizedMemoryAllocation 测试优化版 RClass 内存分配
func BenchmarkRClassOptimizedMemoryAllocation(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("test", func() string {
		return "test"
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.New()
		_ = instance.Call("test")
	}
}

// BenchmarkRClassOptimizedPoolMemoryAllocation 测试优化版 RClass 对象池内存分配
func BenchmarkRClassOptimizedPoolMemoryAllocation(b *testing.B) {
	class := ClassOptimized("TestClass")
	class.DefineMethod("test", func() string {
		return "test"
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance := class.NewFromPool()
		_ = instance.Call("test")
		instance.ReturnToPool()
	}
}
