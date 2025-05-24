# RGo - Go 语言的 Ruby 风格库
[![Go](https://github.com/GoLangDream/rgo/actions/workflows/test.yml/badge.svg)](https://github.com/GoLangDream/rgo/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/GoLangDream/rgo)](https://goreportcard.com/report/github.com/GoLangDream/rgo)
[![Coverage Status](https://coveralls.io/repos/github/GoLangDream/rgo/badge.svg?branch=main)](https://coveralls.io/github/GoLangDream/rgo?branch=main)

RGo 是一个 Go 语言库，提供了类似 Ruby 的编程体验。它包含了字符串、数组、哈希表、整数和类系统等常用数据类型的 Ruby 风格实现。

## 安装

```bash
go get github.com/GoLangDream/rgo
```

## 特性

- 类似 Ruby 的字符串操作
- 类似 Ruby 的数组操作
- 类似 Ruby 的哈希表操作
- 类似 Ruby 的整数操作
- 类似 Ruby 的类系统

## 性能

RGO 在基本操作上与原生 Go 对象保持接近的性能，同时提供 Ruby 风格的便利 API。

### 性能总览

| 组件 | 基本操作 | 复杂操作 | 推荐使用场景 |
|------|----------|----------|-------------|
| **RString** | 🟢 性能相当 | 🟡 轻微损失 | 字符串处理、文本操作 |
| **RInteger** | 🟢 性能相当 | 🟡 轻微损失 | 数值计算、算术运算 |
| **RHash** | 🟢 性能相当 | 🟠 中等损失 | 键值存储、配置管理 |
| **RClass** | 🟠 中等损失 | 🔴 严重损失 | 原型开发、动态编程 |

- 🟢 **性能相当** (< 10% 差异): 适合性能敏感的应用
- 🟡 **轻微损失** (10-50% 差异): 大多数应用场景可接受
- 🟠 **中等损失** (50-500% 差异): 需要权衡开发效率vs性能
- 🔴 **严重损失** (> 500% 差异): 建议仅用于非性能关键场景

### 运行性能测试

```bash
# 快速测试
make benchmark-quick

# 完整测试
make benchmark

# 详细测试
make benchmark-detail
```

📊 **详细性能分析**: [性能测试报告](docs/PERFORMANCE_ANALYSIS.md) | [性能测试指南](docs/BENCHMARK_GUIDE.md)

## 快速开始

### RString

```go
import "github.com/GoLangDream/rgo"

str := rgo.NewRString("hello")
str.Upcase()           // 返回 "HELLO"
str.Downcase()         // 返回 "hello"
str.Capitalize()       // 返回 "Hello"
str.Reverse()          // 返回 "olleh"
str.Include("ell")     // 返回 true
str.StartsWith("he")   // 返回 true
str.EndsWith("lo")     // 返回 true
```

更多 RString 的详细文档请参考 [RString.md](docs/RString.md)

### RArray

```go
arr := rgo.NewRArray([]rgo.Object{
    rgo.NewRString("a"),
    rgo.NewRString("b"),
    rgo.NewRString("c"),
})

// 数组操作
arr.Push(rgo.NewRString("d"))
arr.Pop()              // 返回 "d"
arr.Reverse()          // 返回 ["c", "b", "a"]
arr.Shuffle()          // 随机打乱数组
arr.Sort()             // 排序数组
arr.Uniq()             // 去重

// 数组变换
arr.Map(func(obj rgo.Object) rgo.Object {
    return obj.(rgo.RString).Upcase()
})

// 数组查询
arr.Include(rgo.NewRString("a"))  // 返回 true
arr.Index(rgo.NewRString("b"))    // 返回 1
```

更多 RArray 的详细文档请参考 [RArray.md](docs/RArray.md)

### RHash

```go
hash := rgo.NewHash()
hash.Set("name", "John")
hash.Set("age", 30)

// 获取值
name := hash.Get("name")  // 返回 "John"
age := hash.Get("age")    // 返回 30

// 删除键值对
hash.Delete("age")

// 检查键是否存在
if hash.HasKey("name") {
    // 键存在
}

// 获取所有键
keys := hash.Keys()

// 获取所有值
values := hash.Values()
```

更多 RHash 的详细文档请参考 [RHash.md](docs/RHash.md)

### RInteger

```go
i := rgo.NewRInteger(42)

// 数学运算
i.Add(8)                // 返回 50
i.Sub(2)                // 返回 40
i.Mul(2)                // 返回 80
i.Div(4)                // 返回 20

// 位运算
i.BitAnd(0x0F)         // 按位与
i.BitOr(0xF0)          // 按位或
i.BitXor(0xFF)         // 按位异或
i.LeftShift(2)         // 左移
i.RightShift(1)        // 右移

// 数学函数
i.Abs()                // 绝对值
i.Gcd(18)              // 最大公约数
i.Lcm(18)              // 最小公倍数
i.Pow(2)               // 幂运算
```

更多 RInteger 的详细文档请参考 [RInteger.md](docs/RInteger.md)

### RClass

```go
// 创建一个 Person 类
Person := rgo.Class("Person").
    AttrAccessor("name", "age").  // 定义 name 和 age 的读写属性
    Define("initialize", func(name string, age int) *rgo.RClass {
        p := rgo.Class("Person").New()
        p.SetInstanceVar("name", name)
        p.SetInstanceVar("age", age)
        return p
    }).
    Define("introduce", func(self *rgo.RClass) string {
        name := self.GetInstanceVar("name").(string)
        age := self.GetInstanceVar("age").(int)
        return fmt.Sprintf("Hi, I'm %s and I'm %d years old.", name, age)
    })

// 创建一个 Student 类，继承自 Person
Student := rgo.Class("Student").
    Inherit(Person).
    AttrAccessor("grade").
    Define("initialize", func(name string, age int, grade string) *rgo.RClass {
        s := rgo.Class("Student").New()
        s.SetInstanceVar("name", name)
        s.SetInstanceVar("age", age)
        s.SetInstanceVar("grade", grade)
        return s
    })

// 创建实例
person := Person.Call("initialize", "John", 30).(*rgo.RClass)
student := Student.Call("initialize", "Alice", 15, "10th").(*rgo.RClass)

// 使用属性访问器
fmt.Println(person.Call("name"))  // 输出: John
person.Call("name=", "Johnny")
fmt.Println(person.Call("name"))  // 输出: Johnny

// 调用方法
fmt.Println(person.Call("introduce"))   // 输出: Hi, I'm Johnny and I'm 30 years old.

// 类方法示例
Math := rgo.Class("Math").
    DefineClass("add", func(a, b int) int {
        return a + b
    }).
    DefineClass("subtract", func(a, b int) int {
        return a - b
    })

// 调用类方法
sum := Math.Call("add", 2, 3).(int)           // 返回 5
diff := Math.Call("subtract", 5, 3).(int)     // 返回 2

// 方法缺失处理
Dynamic := rgo.Class("Dynamic").
    MethodMissing(func(name string, args ...any) any {
        return fmt.Sprintf("Called %s with args: %v", name, args)
    })

// 调用未定义的方法
result := Dynamic.New().Call("undefined_method", "arg1", "arg2").(string)
fmt.Println(result)  // 输出: Called undefined_method with args: [arg1 arg2]
```

RClass 提供了以下特性：
1. 类定义和方法定义
2. 实例方法和类方法
3. 属性访问器（读写、只读、只写）
4. 实例变量和类变量
5. 继承和方法重写
6. 父类方法调用（Super）
7. 方法缺失处理
8. 类型检查
9. 线程安全

更多 RClass 的详细文档请参考 [RClass.md](docs/RClass.md)

## 测试

```bash
go test -v
```

## 贡献

欢迎提交 Pull Request 和 Issue！

## 许可证

MIT License
