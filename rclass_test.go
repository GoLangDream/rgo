package rgo

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RClass", func() {

	Describe("基础功能测试", func() {
		It("应该能够创建类和实例", func() {
			person := Class("Person")
			Expect(person.Name()).To(Equal("Person"))

			john := person.New()
			Expect(john.Name()).To(Equal("Person"))
		})

		It("应该能够定义和调用实例方法", func() {
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})

			john := person.New()
			result := john.Call("greet").(string)
			Expect(result).To(Equal("Hello!"))
		})

		It("应该能够定义和调用带参数的方法", func() {
			person := Class("Person")
			person.DefineMethod("greet", func(name any) string {
				return "Hello, " + name.(string) + "!"
			})

			john := person.New()
			result := john.Call("greet", "John").(string)
			Expect(result).To(Equal("Hello, John!"))
		})

		It("应该能够定义和调用带 self 参数的方法", func() {
			person := Class("Person")
			person.DefineMethod("introduce", func(self *RClass) string {
				name := self.GetInstanceVar("name")
				if name == nil {
					return "I am anonymous"
				}
				return "I am " + name.(string)
			})

			john := person.New()
			john.SetInstanceVar("name", "John")
			result := john.Call("introduce").(string)
			Expect(result).To(Equal("I am John"))
		})

		It("应该能够定义类方法", func() {
			person := Class("Person")
			person.DefineClassMethod("species", func() string {
				return "Homo sapiens"
			})

			result := person.Call("species").(string)
			Expect(result).To(Equal("Homo sapiens"))
		})
	})

	Describe("属性访问器测试", func() {
		It("应该能够使用 AttrAccessor", func() {
			person := Class("Person")
			person.AttrAccessor("age", "name")

			john := person.New()
			john.Call("age=", 30)
			john.Call("name=", "John")

			age := john.Call("age").(int)
			name := john.Call("name").(string)

			Expect(age).To(Equal(30))
			Expect(name).To(Equal("John"))
		})

		It("应该能够使用 AttrReader", func() {
			person := Class("Person")
			person.AttrReader("id")

			john := person.New()
			john.SetInstanceVar("id", 123)

			id := john.Call("id").(int)
			Expect(id).To(Equal(123))

			// 不应该有 setter 方法
			Expect(john.RespondTo("id=")).To(BeFalse())
		})

		It("应该能够使用 AttrWriter", func() {
			person := Class("Person")
			person.AttrWriter("secret")

			john := person.New()
			john.Call("secret=", "password123")

			secret := john.GetInstanceVar("secret").(string)
			Expect(secret).To(Equal("password123"))

			// 不应该有 getter 方法
			Expect(john.RespondTo("secret")).To(BeFalse())
		})
	})

	Describe("继承测试", func() {
		It("应该能够继承父类的方法", func() {
			animal := Class("Animal")
			animal.DefineMethod("speak", func() string {
				return "Some sound"
			})

			dog := Class("Dog")
			dog.Inherit(animal)

			buddy := dog.New()
			result := buddy.Call("speak").(string)
			Expect(result).To(Equal("Some sound"))
		})

		It("应该能够重写父类的方法", func() {
			animal := Class("Animal")
			animal.DefineMethod("speak", func() string {
				return "Some sound"
			})

			dog := Class("Dog")
			dog.Inherit(animal)
			dog.DefineMethod("speak", func() string {
				return "Woof!"
			})

			buddy := dog.New()
			result := buddy.Call("speak").(string)
			Expect(result).To(Equal("Woof!"))
		})

		It("应该能够调用 Super 方法", func() {
			animal := Class("Animal")
			animal.DefineMethod("speak", func() string {
				return "Some sound"
			})

			dog := Class("Dog")
			dog.Inherit(animal)
			dog.DefineMethod("speak", func(self *RClass) string {
				parentSound := self.Super("speak").(string)
				return parentSound + " - Woof!"
			})

			buddy := dog.New()
			result := buddy.Call("speak").(string)
			Expect(result).To(Equal("Some sound - Woof!"))
		})

		It("应该能够正确判断 IsA 关系", func() {
			animal := Class("Animal")
			dog := Class("Dog")
			dog.Inherit(animal)

			buddy := dog.New()
			Expect(buddy.IsA("Dog")).To(BeTrue())
			Expect(buddy.IsA("Animal")).To(BeTrue())
			Expect(buddy.IsA("Cat")).To(BeFalse())
		})
	})

	Describe("元编程功能测试", func() {
		It("应该能够处理 method_missing", func() {
			person := Class("Person")
			person.MethodMissing(func(name string, args ...any) any {
				return "Method " + name + " not found"
			})

			john := person.New()
			result := john.Call("unknown_method").(string)
			Expect(result).To(Equal("Method unknown_method not found"))
		})

		It("应该能够创建方法别名", func() {
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})
			person.Alias("hi", "greet")

			john := person.New()
			result1 := john.Call("greet").(string)
			result2 := john.Call("hi").(string)

			Expect(result1).To(Equal("Hello!"))
			Expect(result2).To(Equal("Hello!"))
		})

		It("应该能够添加 before hooks", func() {
			var hookCalled bool
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})
			person.Before("greet", func(args ...any) {
				hookCalled = true
			})

			john := person.New()
			john.Call("greet")
			Expect(hookCalled).To(BeTrue())
		})

		It("应该能够添加 after hooks", func() {
			var hookResult any
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})
			person.After("greet", func(result any, args ...any) {
				hookResult = result
			})

			john := person.New()
			john.Call("greet")
			Expect(hookResult).To(Equal("Hello!"))
		})

		It("应该能够使用元类（EigenClass）", func() {
			person := Class("Person")
			john := person.New()

			// 为单个对象定义方法
			john.DefineEigenMethod("special_ability", func() string {
				return "I can fly!"
			})

			// 只有这个对象有这个方法
			result := john.Call("special_ability").(string)
			Expect(result).To(Equal("I can fly!"))

			// 其他对象没有这个方法
			mary := person.New()
			Expect(mary.RespondTo("special_ability")).To(BeFalse())
		})
	})

	Context("性能优化特性", func() {
		It("支持对象池优化", func() {
			class := Class("PooledClass")
			class.EnablePooling()
			class.DefineMethod("getValue", func() string {
				return "pooled"
			})

			instance1 := class.New()
			Expect(instance1.Call("getValue")).To(Equal("pooled"))

			// 回收到池中
			instance1.ReturnToPool()

			// 从池中获取新实例
			instance2 := class.New()
			Expect(instance2.Call("getValue")).To(Equal("pooled"))
		})

		It("支持快速调用跳过钩子", func() {
			class := Class("FastCallClass")
			class.DefineMethod("greet", func() string {
				return "hello"
			})

			hookCalled := false
			class.Before("greet", func(args ...any) {
				hookCalled = true
			})

			instance := class.New()

			// 普通调用会触发钩子
			result := instance.Call("greet")
			Expect(result).To(Equal("hello"))
			Expect(hookCalled).To(BeTrue())

			// 重置标志
			hookCalled = false

			// 快速调用跳过钩子
			result = instance.FastCall("greet")
			Expect(result).To(Equal("hello"))
			Expect(hookCalled).To(BeFalse())
		})

		It("支持直接方法调用", func() {
			class := Class("DirectCallClass")
			class.DefineMethod("greet", func() string {
				return "hello"
			})

			instance := class.New()

			// 获取编译后的方法
			method := instance.GetMethodDirectly("greet")
			Expect(method).NotTo(BeNil())

			// 直接调用
			result := method.CallDirect(instance)
			Expect(result).To(Equal("hello"))
		})

		It("方法缓存高效工作", func() {
			class := Class("CachedClass")
			class.DefineMethod("getValue", func() string {
				return "cached"
			})

			instance := class.New()

			// 第一次调用建立缓存
			result1 := instance.Call("getValue")
			Expect(result1).To(Equal("cached"))

			// 后续调用应该命中缓存
			result2 := instance.Call("getValue")
			Expect(result2).To(Equal("cached"))

			result3 := instance.FastCall("getValue")
			Expect(result3).To(Equal("cached"))
		})

		It("继承链中的别名正确解析", func() {
			animal := Class("Animal")
			animal.DefineMethod("sound", func() string {
				return "generic sound"
			})
			animal.Alias("noise", "sound")

			dog := Class("Dog")
			dog.Inherit(animal)
			dog.DefineMethod("sound", func() string {
				return "woof"
			})

			instance := dog.New()

			// 别名应该在继承链中正确解析
			result := instance.Call("noise")
			Expect(result).To(Equal("woof"))
		})

		It("继承链中的钩子正确执行", func() {
			var results []string

			animal := Class("Animal")
			animal.DefineMethod("speak", func() string {
				return "animal speaks"
			})
			animal.Before("speak", func(args ...any) {
				results = append(results, "animal before")
			})

			dog := Class("Dog")
			dog.Inherit(animal)
			dog.Before("speak", func(args ...any) {
				results = append(results, "dog before")
			})

			instance := dog.New()
			instance.Call("speak")

			// 两个类的钩子都应该执行
			Expect(results).To(ContainElement("animal before"))
			Expect(results).To(ContainElement("dog before"))
		})

		It("元类方法查找优先级正确", func() {
			class := Class("EigenTestClass")
			class.DefineMethod("greet", func() string {
				return "class method"
			})

			instance := class.New()

			// 普通方法调用
			result := instance.Call("greet")
			Expect(result).To(Equal("class method"))

			// 为实例定义单例方法
			instance.DefineEigenMethod("greet", func() string {
				return "eigen method"
			})

			// 应该优先调用单例方法
			result = instance.Call("greet")
			Expect(result).To(Equal("eigen method"))
		})
	})

	Describe("变量操作测试", func() {
		It("应该能够操作实例变量", func() {
			person := Class("Person")
			john := person.New()

			john.SetInstanceVar("name", "John")
			john.SetInstanceVar("age", 30)

			name := john.GetInstanceVar("name").(string)
			age := john.GetInstanceVar("age").(int)

			Expect(name).To(Equal("John"))
			Expect(age).To(Equal(30))

			vars := john.InstanceVars()
			Expect(vars).To(ContainElement("name"))
			Expect(vars).To(ContainElement("age"))
		})

		It("应该能够操作类变量", func() {
			person := Class("Person")

			person.SetClassVar("species", "Homo sapiens")
			person.SetClassVar("count", 0)

			species := person.GetClassVar("species").(string)
			count := person.GetClassVar("count").(int)

			Expect(species).To(Equal("Homo sapiens"))
			Expect(count).To(Equal(0))

			vars := person.ClassVars()
			Expect(vars).To(ContainElement("species"))
			Expect(vars).To(ContainElement("count"))
		})

		It("子类应该共享父类的类变量", func() {
			animal := Class("Animal")
			animal.SetClassVar("count", 0)

			dog := Class("Dog")
			dog.Inherit(animal)

			// 子类应该能够访问父类的类变量
			count := dog.GetClassVar("count").(int)
			Expect(count).To(Equal(0))

			// 修改类变量应该影响父类
			dog.SetClassVar("count", 5)
			parentCount := animal.GetClassVar("count").(int)
			Expect(parentCount).To(Equal(5))
		})
	})

	Describe("方法信息查询测试", func() {
		It("应该能够查询方法是否存在", func() {
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})

			john := person.New()
			Expect(john.RespondTo("greet")).To(BeTrue())
			Expect(john.RespondTo("unknown")).To(BeFalse())
		})

		It("应该能够获取方法列表", func() {
			person := Class("Person")
			person.DefineMethod("greet", func() string {
				return "Hello!"
			})
			person.DefineMethod("walk", func() string {
				return "Walking..."
			})

			john := person.New()
			methods := john.Methods()
			Expect(methods).To(ContainElement("greet"))
			Expect(methods).To(ContainElement("walk"))
		})

		It("应该能够获取类方法列表", func() {
			person := Class("Person")
			person.DefineClassMethod("species", func() string {
				return "Homo sapiens"
			})
			person.DefineClassMethod("create", func() *RClass {
				return person.New()
			})

			classMethods := person.ClassMethods()
			Expect(classMethods).To(ContainElement("species"))
			Expect(classMethods).To(ContainElement("create"))
		})
	})

	Describe("错误处理测试", func() {
		It("调用不存在的方法应该 panic", func() {
			person := Class("Person")
			john := person.New()

			Expect(func() {
				john.Call("unknown_method")
			}).To(Panic())
		})

		It("参数数量不匹配应该 panic", func() {
			person := Class("Person")
			person.DefineMethod("greet", func(name any) string {
				return "Hello, " + name.(string)
			})

			john := person.New()

			Expect(func() {
				john.Call("greet") // 缺少参数
			}).To(Panic())

			Expect(func() {
				john.Call("greet", "John", "extra") // 参数过多
			}).To(Panic())
		})

		It("调用不存在的父类方法应该 panic", func() {
			person := Class("Person")
			john := person.New()

			Expect(func() {
				john.Super("unknown_method")
			}).To(Panic())
		})
	})
})

// 兼容性测试 - 保持与原有测试的兼容
func TestRClass(t *testing.T) {
	// 测试类创建
	person := Class("Person")
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
	person.DefineMethod("say_hello", func(self *RClass) string {
		return "Hello!"
	})

	// 测试类方法
	person.DefineClassMethod("create", func(name any) *RClass {
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
	mary := person.Call("create", "Mary").(*RClass)
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
	employee := Class("Employee")
	employee.Inherit(person)
	employee.DefineMethod("say_hello", func(self *RClass) string {
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
