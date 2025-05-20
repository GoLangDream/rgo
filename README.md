# RGo
[![Go](https://github.com/GoLangDream/rgo/actions/workflows/test.yml/badge.svg)](https://github.com/GoLangDream/rgo/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GoLangDream/rgo)](https://goreportcard.com/report/github.com/GoLangDream/rgo)
[![Coverage Status](https://coveralls.io/repos/github/GoLangDream/rgo/badge.svg?branch=main)](https://coveralls.io/github/GoLangDream/rgo?branch=main)

RGo 是一个 Golang 库，提供类似 Ruby 中常用类的功能。它实现了 `RString`、`RInteger`、`RArray`、`RHash` 和 `RClass` 这些主要类，并且保持了与 Ruby 中对应类相似的 API 设计。

## 特性

- 通过嵌入结构体和接口模拟继承，确保公共方法不用重复编写
- 实现了 Ruby 中 String、Integer、Array、Hash 和 Class 类常用的方法
- 使用 Ginkgo 和 Gomega 进行完整的测试
- 支持链式调用和函数式编程风格
- 提供丰富的数组操作方法，包括：
  - 数组操作（Compact、Flatten等）
  - 数组变换（Map、Select、Reject等）
  - 数组查询（Index、Count、Any等）
  - 数组切片（Slice、Take、Drop等）
  - 数组分组（GroupBy、Partition等）
  - 数组迭代（Each、EachWithIndex等）
- 提供完整的哈希表操作，包括：
  - 基本操作（Get、Set、Delete等）
  - 转换方法（ToJSON、ToYAML、ToXML等）
  - 迭代和过滤（Each、Select、Reject等）
  - 合并操作（Merge、MergeBang等）
- 提供 Ruby 风格的类系统，支持：
  - 实例方法和类方法
  - 实例变量和类变量
  - 方法缺失处理
  - 类继承
  - 元编程特性

## 安装

```bash
go get github.com/GoLangDream/rgo
```

## 使用示例

### RString

```go
import "github.com/GoLangDream/rgo"

str := rgo.NewRString("hello world")
upStr := str.Upcase()                 // 返回 "HELLO WORLD"
capStr := str.Capitalize()            // 返回 "Hello world"
contains := str.Include("hello")      // 返回 true
parts := str.Split(" ")               // 返回包含 ["hello", "world"] 的 RArray
```

更多 RString 的详细文档请参考 [RString.md](docs/RString.md)

### RInteger

`RInteger` 是一个整数类型的包装器，提供了丰富的整数操作方法。

### 创建

```go
// 创建一个新的 RInteger
i := rgo.NewRInteger(42)
```

### 基本运算

```go
// 加法
result := i.Add(10)      // 与原生 int 相加
result = i.AddRInt(j)    // 与另一个 RInteger 相加

// 减法
result := i.Sub(10)      // 与原生 int 相减
result = i.SubRInt(j)    // 与另一个 RInteger 相减

// 乘法
result := i.Mul(10)      // 与原生 int 相乘
result = i.MulRInt(j)    // 与另一个 RInteger 相乘

// 除法
result := i.Div(10)      // 与原生 int 相除
result = i.DivRInt(j)    // 与另一个 RInteger 相除

// 取模
result := i.Mod(10)      // 与原生 int 取模
result = i.ModRInt(j)    // 与另一个 RInteger 取模

// 幂运算
result := i.Pow(2)       // 与原生 int 的幂
result = i.PowRInt(j)    // 与另一个 RInteger 的幂
```

### 比较操作

```go
// 相等比较
i.Eq(10)      // 与原生 int 比较
i.EqRInt(j)   // 与另一个 RInteger 比较

// 大于比较
i.Gt(10)      // 与原生 int 比较
i.GtRInt(j)   // 与另一个 RInteger 比较

// 小于比较
i.Lt(10)      // 与原生 int 比较
i.LtRInt(j)   // 与另一个 RInteger 比较
```

### 其他操作

```go
// 获取绝对值
abs := i.Abs()

// 获取最大值
max := rgo.MaxRInt(i, j)

// 获取最小值
min := rgo.MinRInt(i, j)

// 转换为字符串
str := i.ToString()

// 转换为原生 int
n := i.ToInt()
```

更多 RInteger 的详细文档请参考 [RInteger.md](docs/RInteger.md)

### RArray

```go
import "github.com/GoLangDream/rgo"

// 创建数组
arr := rgo.NewRArray([]rgo.Object{
    rgo.NewRString("a"),
    rgo.NewRString("b"),
    rgo.NewRInteger(1),
})

// 数组操作
first := arr.First()                  // 返回 "a"
length := arr.Length()                // 返回 3
joined := arr.Join(", ")              // 返回 "a, b, 1"

// 数组变换
mapped := arr.Map(func(obj rgo.Object) rgo.Object {
    if str, ok := obj.(rgo.RString); ok {
        return rgo.NewRString(str.ToString() + "!")
    }
    return obj
})
// mapped 包含 ["a!", "b!", 1]

// 数组查询
hasA := arr.Include(rgo.NewRString("a"))  // 返回 true
count := arr.Count(rgo.NewRString("a"))   // 返回 1

// 数组切片
subArr := arr.Slice(0, 2)  // 返回 ["a", "b"]

// 数组分组
groups := arr.GroupBy(func(obj rgo.Object) rgo.Object {
    if _, ok := obj.(rgo.RString); ok {
        return rgo.NewRString("string")
    }
    return rgo.NewRString("integer")
})
// groups 包含 {"string": ["a", "b"], "integer": [1]}

// 数组迭代
arr.Each(func(obj rgo.Object) {
    fmt.Println(obj.ToString())
})

// 使用EachWithIndex
arr.EachWithIndex(func(obj rgo.Object, index int) {
    fmt.Printf("%d: %s\n", index, obj.ToString())
})
```

更多 RArray 的详细文档请参考 [RArray.md](docs/RArray.md)

### RHash

```go
import "github.com/GoLangDream/rgo"

// 创建哈希表
hash := rgo.NewHash()
hash.Set("name", "John")
hash.Set("age", 30)

// 基本操作
value, exists := hash.Get("name")     // 返回 "John", true
size := hash.Size()                   // 返回 2
keys := hash.Keys()                   // 返回 ["age", "name"]（按字符串排序）

// 转换方法
jsonStr := hash.ToJSON()              // 返回 {"age":30,"name":"John"}
yamlStr := hash.ToYAML()              // 返回格式化的 YAML 字符串

// 迭代和过滤
hash.Each(func(key, value any) {
    fmt.Printf("%v: %v\n", key, value)
})

filtered := hash.Select(func(key, value any) bool {
    return key == "name"
})

// 合并操作
otherHash := rgo.NewHash()
otherHash.Set("city", "New York")
merged := hash.Merge(otherHash)
```

更多 RHash 的详细文档请参考 [RHash.md](docs/RHash.md)

### RClass

RClass 提供了一个类似 Ruby 的类系统，支持元编程特性。

```go
import "github.com/GoLangDream/rgo"

// 创建类
calculator := RClassBuilder("Calculator", func(c *RClass) {
    // 定义实例方法
    RDefineMethod(c, "Add", func(a, b int) int {
        return a + b
    })

    // 定义类方法
    RDefineClassMethod(c, "Create", func() *RClass {
        return c.New()
    })

    // 定义类变量
    SetClassVar(c, "@@version", "1.0.0")
})

// 创建实例
calc := calculator.New()
result := calc.Call("Add", 2, 3).(int) // 返回 5

// 调用类方法
newCalc := calculator.Call("Create").(*RClass)

// 使用实例变量
person := RClassBuilder("Person", func(c *RClass) {
    RDefineMethod(c, "SetName", func(name string) {
        SetInstanceVar(c, "@name", name)
    })
    RDefineMethod(c, "GetName", func() string {
        return GetInstanceVar(c, "@name").(string)
    })
})

// 使用类变量
version := GetClassVar(calculator, "@@version").(string) // 返回 "1.0.0"
```

RClass 支持以下特性：

1. 实例方法：通过 `RDefineMethod` 定义，只能被实例调用
2. 类方法：通过 `RDefineClassMethod` 定义，只能被类调用
3. 实例变量：通过 `SetInstanceVar` 和 `GetInstanceVar` 操作
4. 类变量：通过 `SetClassVar` 和 `GetClassVar` 操作
5. 方法缺失处理：通过 `SetMethodMissing` 设置
6. 类继承：通过 `Inherit` 方法实现

更多 RClass 的详细文档请参考 [RClass.md](docs/RClass.md)

## 测试

```bash
go test -v
```

## 贡献

欢迎提交 Pull Request 和 Issue！

## 许可证

MIT License
