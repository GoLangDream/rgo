# RClass - Ruby 风格的 Go 类系统

RClass 是一个模仿 Ruby 类系统的 Go 包，提供了类似 Ruby 的类定义、方法调用和属性访问等功能。

## 基本用法

```go
package main

import (
    "fmt"
    "github.com/yourusername/rgo"
)

func main() {
    // 创建一个新的类
    person := rgo.Class("Person")

    // 定义实例方法
    person.DefineMethod("say_hello", func(self *rgo.RClass) string {
        return "Hello!"
    })

    // 定义类方法
    person.DefineClassMethod("create", func(name string) *rgo.RClass {
        instance := person.New()
        instance.SetInstanceVar("name", name)
        return instance
    })

    // 创建实例
    john := person.New()

    // 调用实例方法
    fmt.Println(john.Call("say_hello")) // 输出: Hello!

    // 调用类方法
    mary := person.Call("create", "Mary").(*rgo.RClass)
    fmt.Println(mary.GetInstanceVar("name")) // 输出: Mary
}
```

## 属性访问器

RClass 提供了类似 Ruby 的属性访问器功能：

```go
person := rgo.Class("Person")

// 定义读写属性
person.AttrAccessor("name", "age")

// 定义只读属性
person.AttrReader("id")

// 定义只写属性
person.AttrWriter("password")

// 使用属性
john := person.New()
john.Call("name=", "John")
john.Call("age=", 30)
fmt.Println(john.Call("name")) // 输出: John
fmt.Println(john.Call("age"))  // 输出: 30
```

## 继承

RClass 支持类继承：

```go
animal := rgo.Class("Animal")
animal.DefineMethod("speak", func(self *rgo.RClass) string {
    return "Some sound"
})

dog := rgo.Class("Dog")
dog.Inherit(animal)
dog.DefineMethod("speak", func(self *rgo.RClass) string {
    return "Woof!"
})

// 调用父类方法
dog.DefineMethod("make_sound", func(self *rgo.RClass) string {
    return self.Super("speak").(string)
})

spot := dog.New()
fmt.Println(spot.Call("speak"))      // 输出: Woof!
fmt.Println(spot.Call("make_sound")) // 输出: Some sound
```

## 方法缺失处理

可以定义方法缺失处理器来处理未定义的方法调用：

```go
person := rgo.Class("Person")
person.MethodMissing(func(name string, args ...any) any {
    return fmt.Sprintf("Method %s not found with args: %v", name, args)
})

john := person.New()
fmt.Println(john.Call("undefined_method", "arg1", "arg2"))
// 输出: Method undefined_method not found with args: [arg1 arg2]
```

## 类变量和实例变量

RClass 支持类变量和实例变量：

```go
person := rgo.Class("Person")

// 设置类变量
person.SetClassVar("count", 0)

// 定义实例方法
person.DefineMethod("initialize", func(self *rgo.RClass) *rgo.RClass {
    count := person.GetClassVar("count").(int)
    person.SetClassVar("count", count+1)
    self.SetInstanceVar("id", count)
    return self
})

// 创建实例
john := person.New()
mary := person.New()

fmt.Println(john.GetInstanceVar("id"))  // 输出: 0
fmt.Println(mary.GetInstanceVar("id"))  // 输出: 1
fmt.Println(person.GetClassVar("count")) // 输出: 2
```

## 方法列表

可以获取类的方法列表：

```go
person := rgo.Class("Person")
person.DefineMethod("say_hello", func(self *rgo.RClass) string {
    return "Hello!"
})
person.DefineClassMethod("create", func(name string) *rgo.RClass {
    return person.New()
})

// 获取实例方法列表
fmt.Println(person.Methods()) // 输出: [say_hello]

// 获取类方法列表
fmt.Println(person.ClassMethods()) // 输出: [create]
```

## 变量列表

可以获取类的变量列表：

```go
person := rgo.Class("Person")
person.SetInstanceVar("name", "John")
person.SetClassVar("count", 0)

// 获取实例变量列表
fmt.Println(person.InstanceVars()) // 输出: [name]

// 获取类变量列表
fmt.Println(person.ClassVars()) // 输出: [count]
```

## 类型检查

RClass 提供了类型检查功能：

```go
person := rgo.Class("Person")
employee := rgo.Class("Employee")
employee.Inherit(person)

john := employee.New()
fmt.Println(john.IsA("Employee")) // 输出: true
fmt.Println(john.IsA("Person"))   // 输出: true
fmt.Println(john.IsA("Animal"))   // 输出: false
```

## 注意事项

1. 所有方法调用都是线程安全的
2. 实例方法需要接收 `self *RClass` 作为第一个参数
3. 类方法不需要接收 `self` 参数
4. 属性访问器会自动生成 getter 和 setter 方法
5. 继承是单继承的，不支持多重继承
6. 方法缺失处理器可以处理未定义的方法调用
7. 类变量和实例变量都是类型安全的
8. 所有方法都支持链式调用

## 最佳实践

1. 使用 `DefineMethod` 定义实例方法
2. 使用 `DefineClassMethod` 定义类方法
3. 使用 `AttrAccessor`、`AttrReader` 和 `AttrWriter` 定义属性
4. 使用 `Inherit` 实现继承
5. 使用 `MethodMissing` 处理未定义的方法
6. 使用 `SetInstanceVar` 和 `GetInstanceVar` 操作实例变量
7. 使用 `SetClassVar` 和 `GetClassVar` 操作类变量
8. 使用 `Methods` 和 `ClassMethods` 获取方法列表
9. 使用 `InstanceVars` 和 `ClassVars` 获取变量列表
10. 使用 `IsA` 进行类型检查
