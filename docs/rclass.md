# RClass - Ruby 风格的类系统

RClass 是一个模拟 Ruby 类系统的 Go 实现，提供了类似 Ruby 的元编程特性。

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

## 元编程特性

### 实例变量

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

### 类变量

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

### 方法缺失处理

```go
dynamic := RClassBuilder("Dynamic", func(c *RClass) {
    SetMethodMissing(c, func(name string, args ...interface{}) interface{} {
        return "Called " + name + " with " + fmt.Sprint(args)
    })
})

d := dynamic.New()
result := d.Call("UndefinedMethod", "arg1", "arg2").(string)
// 返回 "Called UndefinedMethod with [arg1 arg2]"
```

### 继承

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
result := d.Call("Speak").(string) // 返回 "Woof!"
```

## API 参考

### RClassBuilder

```go
func RClassBuilder(name string, block func(*RClass)) *RClass
```

创建一个新的类。`name` 是类名，`block` 是用于定义类的方法和变量的闭包。

### RDefineMethod

```go
func RDefineMethod(c *RClass, name string, method interface{})
```

定义实例方法。`name` 是方法名，`method` 是方法实现。

### RDefineClassMethod

```go
func RDefineClassMethod(c *RClass, name string, method interface{})
```

定义类方法。

### SetInstanceVar/GetInstanceVar

```go
func SetInstanceVar(c *RClass, name string, value interface{})
func GetInstanceVar(c *RClass, name string) interface{}
```

设置和获取实例变量。

### SetClassVar/GetClassVar

```go
func SetClassVar(c *RClass, name string, value interface{})
func GetClassVar(c *RClass, name string) interface{}
```

设置和获取类变量。

### SetMethodMissing

```go
func SetMethodMissing(c *RClass, handler func(name string, args ...interface{}) interface{})
```

设置方法缺失处理器。

### Inherit

```go
func (c *RClass) Inherit(parent *RClass)
```

设置类的继承关系。

### Call

```go
func (c *RClass) Call(methodName string, args ...interface{}) interface{}
```

调用方法。

### New

```go
func (c *RClass) New() *RClass
```

创建类的新实例。

## 最佳实践

1. 使用 `@` 前缀命名实例变量
2. 使用 `@@` 前缀命名类变量
3. 在类定义块中定义所有方法和变量
4. 使用 `methodMissing` 处理动态方法调用
5. 合理使用继承来复用代码

## 注意事项

1. 所有方法调用都是通过 `Call` 方法进行
2. 方法返回值需要类型断言
3. 类变量在实例间共享
4. 实例变量是实例私有的
5. 方法缺失处理器需要处理所有未定义的方法调用
