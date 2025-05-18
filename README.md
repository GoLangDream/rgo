# RGo

RGo 是一个 Golang 库，提供类似 Ruby 中常用类的功能。它实现了 `RString`、`RInteger` 和 `RArray` 这三个主要类，并且保持了与 Ruby 中对应类相似的 API 设计。

## 特性

- 通过嵌入结构体和接口模拟继承，确保公共方法不用重复编写
- 实现了 Ruby 中 String、Integer 和 Array 类常用的方法
- 使用 Ginkgo 和 Gomega 进行完整的测试
- 支持链式调用和函数式编程风格
- 提供丰富的数组操作方法，包括：
  - 数组操作（Compact、Flatten等）
  - 数组变换（Map、Select、Reject等）
  - 数组查询（Index、Count、Any等）
  - 数组切片（Slice、Take、Drop等）
  - 数组分组（GroupBy、Partition等）
  - 数组迭代（Each、EachWithIndex等）

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

更多 RString 的详细文档请参考 [RString.md](RString.md)

### RInteger

```go
import "github.com/GoLangDream/rgo"

num := goby.NewRInteger(42)
isEven := num.Even()                  // 返回 true
sum := num.Add(goby.NewRInteger(10))  // 返回 52
abs := goby.NewRInteger(-10).Abs()    // 返回 10
```

更多 RInteger 的详细文档请参考 [RInteger.md](RInteger.md)

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

更多 RArray 的详细文档请参考 [RArray.md](RArray.md)

## 测试

```bash
go test -v
```

## 贡献

欢迎提交 Pull Request 和 Issue！

## 许可证

MIT License

