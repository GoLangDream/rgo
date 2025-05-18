# RGo
[![Go](https://github.com/GoLangDream/goby/actions/workflows/test.yml/badge.svg)](https://github.com/GoLangDream/goby/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GoLangDream/rgo)](https://goreportcard.com/report/github.com/GoLangDream/rgo)
[![Coverage Status](https://coveralls.io/repos/github/GoLangDream/rgo/badge.svg?branch=main)](https://coveralls.io/github/GoLangDream/rgo?branch=main)

RGo 是一个 Golang 库，提供类似 Ruby 中常用类的功能。它实现了 `RString`、`RInteger`、`RArray` 和 `RHash` 这些主要类，并且保持了与 Ruby 中对应类相似的 API 设计。

## 特性

- 通过嵌入结构体和接口模拟继承，确保公共方法不用重复编写
- 实现了 Ruby 中 String、Integer、Array 和 Hash 类常用的方法
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

## 安装

```bash
go get github.com/GoLangDream/rgo
```

## 使用示例

### RString

```go
import "github.com/GoLangDream/rgo"

str := goby.NewRString("hello world")
upStr := str.Upcase()                 // 返回 "HELLO WORLD"
capStr := str.Capitalize()            // 返回 "Hello world"
contains := str.Include("hello")      // 返回 true
parts := str.Split(" ")               // 返回包含 ["hello", "world"] 的 RArray
```

更多 RString 的详细文档请参考 [RString.md](docs/RString.md)

### RInteger

```go
import "github.com/GoLangDream/rgo"

num := goby.NewRInteger(42)
isEven := num.Even()                  // 返回 true
sum := num.Add(goby.NewRInteger(10))  // 返回 52
abs := goby.NewRInteger(-10).Abs()    // 返回 10
```

更多 RInteger 的详细文档请参考 [RInteger.md](docs/RInteger.md)

### RArray

```go
import "github.com/GoLangDream/rgo"

// 创建数组
arr := goby.NewRArray([]goby.Object{
    goby.NewRString("a"),
    goby.NewRString("b"),
    goby.NewRInteger(1),
})

// 数组操作
first := arr.First()                  // 返回 "a"
length := arr.Length()                // 返回 3
joined := arr.Join(", ")              // 返回 "a, b, 1"

// 数组变换
mapped := arr.Map(func(obj goby.Object) goby.Object {
    if str, ok := obj.(goby.RString); ok {
        return goby.NewRString(str.ToString() + "!")
    }
    return obj
})
// mapped 包含 ["a!", "b!", 1]

// 数组查询
hasA := arr.Include(goby.NewRString("a"))  // 返回 true
count := arr.Count(goby.NewRString("a"))   // 返回 1

// 数组切片
subArr := arr.Slice(0, 2)  // 返回 ["a", "b"]

// 数组分组
groups := arr.GroupBy(func(obj goby.Object) goby.Object {
    if _, ok := obj.(goby.RString); ok {
        return goby.NewRString("string")
    }
    return goby.NewRString("integer")
})
// groups 包含 {"string": ["a", "b"], "integer": [1]}

// 数组迭代
arr.Each(func(obj goby.Object) {
    fmt.Println(obj.ToString())
})

// 使用EachWithIndex
arr.EachWithIndex(func(obj goby.Object, index int) {
    fmt.Printf("%d: %s\n", index, obj.ToString())
})
```

更多 RArray 的详细文档请参考 [RArray.md](docs/RArray.md)

### RHash

```go
import "github.com/GoLangDream/rgo"

// 创建哈希表
hash := goby.NewHash()
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
otherHash := goby.NewHash()
otherHash.Set("city", "New York")
merged := hash.Merge(otherHash)
```

更多 RHash 的详细文档请参考 [RHash.md](docs/RHash.md)

## 测试

```bash
go test -v
```

## 贡献

欢迎提交 Pull Request 和 Issue！

## 许可证

MIT License

