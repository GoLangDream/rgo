package rgo

import (
	"fmt"
	"testing"
)

func TestRClassBasic(t *testing.T) {
	calculator := RClassBuilder("Calculator", func(c *RClass) {
		// 定义实例方法
		RDefineMethod(c, "Add", func(a, b int) int {
			return a + b
		})

		RDefineMethod(c, "Subtract", func(a, b int) int {
			return a - b
		})

		// 定义类方法
		RDefineClassMethod(c, "Create", func() *RClass {
			return c.New()
		})
	})

	// 创建实例
	calc := calculator.New()

	// 测试实例方法
	result := calc.Call("Add", 2, 3).(int)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}

	result = calc.Call("Subtract", 5, 3).(int)
	if result != 2 {
		t.Errorf("Expected 2, got %d", result)
	}
}

func TestRClassInheritance(t *testing.T) {
	animal := RClassBuilder("Animal", func(c *RClass) {
		RDefineMethod(c, "Speak", func() string {
			return "Some sound"
		})
	})

	dog := RClassBuilder("Dog", func(c *RClass) {
		RDefineMethod(c, "Speak", func() string {
			return "Woof!"
		})
	})

	// 设置继承关系
	dog.Inherit(animal)

	// 创建实例
	d := dog.New()

	// 测试方法重写
	result := d.Call("Speak").(string)
	if result != "Woof!" {
		t.Errorf("Expected 'Woof!', got %s", result)
	}
}

func TestRClassMethodMissing(t *testing.T) {
	dynamic := RClassBuilder("Dynamic", func(c *RClass) {
		SetMethodMissing(c, func(name string, args ...any) any {
			return "Called " + name + " with " + fmt.Sprint(args)
		})
	})

	// 创建实例
	d := dynamic.New()

	// 测试未定义的方法
	result := d.Call("UndefinedMethod", "arg1", "arg2").(string)
	expected := "Called UndefinedMethod with [arg1 arg2]"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestRClassInstanceVars(t *testing.T) {
	person := RClassBuilder("Person", func(c *RClass) {
		RDefineMethod(c, "SetName", func(name string) {
			SetInstanceVar(c, "@name", name)
		})

		RDefineMethod(c, "GetName", func() string {
			return GetInstanceVar(c, "@name").(string)
		})
	})

	// 创建实例
	p := person.New()

	// 设置和获取实例变量
	p.Call("SetName", "John")
	result := p.Call("GetName").(string)
	if result != "John" {
		t.Errorf("Expected 'John', got %s", result)
	}
}

func TestRClassClassVars(t *testing.T) {
	counter := RClassBuilder("Counter", func(c *RClass) {
		SetClassVar(c, "@@count", 0)

		RDefineMethod(c, "Increment", func() int {
			count := GetClassVar(c, "@@count").(int)
			count++
			SetClassVar(c, "@@count", count)
			return count
		})
	})

	// 创建两个实例
	c1 := counter.New()
	c2 := counter.New()

	// 测试类变量共享
	result1 := c1.Call("Increment").(int)
	if result1 != 1 {
		t.Errorf("Expected 1, got %d", result1)
	}

	result2 := c2.Call("Increment").(int)
	if result2 != 2 {
		t.Errorf("Expected 2, got %d", result2)
	}
}

func TestRClassClassMethods(t *testing.T) {
	// 创建一个带有类方法的类
	math := RClassBuilder("Math", func(c *RClass) {
		// 定义类方法
		RDefineClassMethod(c, "Pi", func() float64 {
			return 3.14159
		})

		RDefineClassMethod(c, "Square", func(n int) int {
			return n * n
		})

		// 定义实例方法
		RDefineMethod(c, "Add", func(a, b int) int {
			return a + b
		})
	})

	// 测试类方法
	pi := math.Call("Pi").(float64)
	if pi != 3.14159 {
		t.Errorf("Expected 3.14159, got %f", pi)
	}

	square := math.Call("Square", 5).(int)
	if square != 25 {
		t.Errorf("Expected 25, got %d", square)
	}

	// 创建实例并测试实例方法
	instance := math.New()
	sum := instance.Call("Add", 2, 3).(int)
	if sum != 5 {
		t.Errorf("Expected 5, got %d", sum)
	}

	// 验证实例不能直接调用类方法
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic when calling class method on instance")
			}
		}()
		instance.Call("Pi")
	}()
}
