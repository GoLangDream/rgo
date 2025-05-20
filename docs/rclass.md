# RClass

RClass 是 RGo 库中提供的一个类似 Ruby 的类系统实现。它支持实例方法、类方法、实例变量、类变量、方法缺失处理以及类继承等特性。

## 基本用法

### 创建类

```go
calculator := RClassBuilder("Calculator", func(c *RClass) {
    // 定义实例方法
    RDefineMethod(c, "Add", func(a, b int) int {
        return a + b
    })
})
```

### 创建实例

```go
calc := calculator.New()
result := calc.Call("Add", 2, 3).(int) // 返回 5
```

## 实例方法

实例方法是类的实例可以调用的方法。使用 `RDefineMethod` 定义：

```go
person := RClassBuilder("Person", func(c *RClass) {
    RDefineMethod(c, "SetName", func(name string) {
        SetInstanceVar(c, "@name", name)
    })
    RDefineMethod(c, "GetName", func() string {
        return GetInstanceVar(c, "@name").(string)
    })
})

p := person.New()
p.Call("SetName", "John")
name := p.Call("GetName").(string) // 返回 "John"
```

## 类方法

类方法是类本身可以调用的方法。使用 `RDefineClassMethod` 定义：

```go
math := RClassBuilder("Math", func(c *RClass) {
    RDefineClassMethod(c, "Pi", func() float64 {
        return 3.14159
    })
})

pi := math.Call("Pi").(float64) // 返回 3.14159
```

## 实例变量

实例变量是属于实例的变量，使用 `SetInstanceVar` 和 `GetInstanceVar` 操作：

```go
person := RClassBuilder("Person", func(c *RClass) {
    RDefineMethod(c, "SetAge", func(age int) {
        SetInstanceVar(c, "@age", age)
    })
    RDefineMethod(c, "GetAge", func() int {
        return GetInstanceVar(c, "@age").(int)
    })
})

p := person.New()
p.Call("SetAge", 25)
age := p.Call("GetAge").(int) // 返回 25
```

## 类变量

类变量是属于类的变量，所有实例共享。使用 `SetClassVar` 和 `GetClassVar` 操作：

```go
counter := RClassBuilder("Counter", func(c *RClass) {
    SetClassVar(c, "@@count", 0)
    RDefineMethod(c, "Increment", func() int {
        count := GetClassVar(c, "@@count").(int)
        count++
        SetClassVar(c, "@@count", count)
        return count
    })
})

c1 := counter.New()
c2 := counter.New()
c1.Call("Increment") // 返回 1
c2.Call("Increment") // 返回 2
```

## 方法缺失处理

当调用未定义的方法时，可以通过 `SetMethodMissing` 设置处理器：

```go
dynamic := RClassBuilder("Dynamic", func(c *RClass) {
    SetMethodMissing(c, func(name string, args ...any) any {
        return "Called " + name + " with " + fmt.Sprint(args)
    })
})

d := dynamic.New()
result := d.Call("UndefinedMethod", "arg1", "arg2").(string)
// 返回 "Called UndefinedMethod with [arg1 arg2]"
```

## 类继承

通过 `Inherit` 方法实现类继承：

```go
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

dog.Inherit(animal)
d := dog.New()
sound := d.Call("Speak").(string) // 返回 "Woof!"
```

## 方法查询

使用 `RespondTo` 检查是否响应某个方法：

```go
if dog.RespondTo("Speak") {
    // 方法存在
}
```

使用 `Methods` 获取所有可用的方法名：

```go
methods := dog.Methods() // 返回所有方法名的切片
```

## 注意事项

1. 类方法和实例方法是分开存储的，不能混用
2. 类变量在所有实例间共享
3. 实例变量是每个实例独立的
4. 继承时，子类可以重写父类的方法
5. 方法调用使用 `Call` 方法，需要类型断言获取返回值
