# RGo

RGo 是一个 Golang 库，提供类似 Ruby 中常用类的功能。它实现了 `RString`、`RInteger` 和 `RArray` 这三个主要类，并且保持了与 Ruby 中对应类相似的 API 设计。

## 特性

- 通过嵌入结构体和接口模拟继承，确保公共方法不用重复编写
- 实现了 Ruby 中 String、Integer 和 Array 类常用的方法
- 使用 Ginkgo 和 Gomega 进行完整的测试

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

### RInteger

```go
import "github.com/GoLangDream/rgo"

num := goby.NewRInteger(42)
isEven := num.Even()                  // 返回 true
sum := num.Add(goby.NewRInteger(10))  // 返回 52
abs := goby.NewRInteger(-10).Abs()    // 返回 10

// 基本数学运算
a := goby.NewRInteger(10)
b := goby.NewRInteger(3)
gcd := a.Gcd(b)                       // 最大公约数: 1
lcm := a.Lcm(b)                       // 最小公倍数: 30
divmod := a.DivMod(b)                 // 返回商和余数: [3, 1]

// 位运算
bits := goby.NewRInteger(10)           // 二进制: 1010
mask := goby.NewRInteger(12)           // 二进制: 1100
bits.BitAnd(mask)                      // 位与: 8 (1000)
bits.BitOr(mask)                       // 位或: 14 (1110)
bits.BitXor(mask)                      // 位异或: 6 (0110)
bits.LeftShift(goby.NewRInteger(1))    // 左移: 20 (10100)
bits.BitAt(goby.NewRInteger(1))        // 获取第1位: 1
bits.AllBits(goby.NewRInteger(8))      // 检查位: true (8=1000)

// 格式转换
n := goby.NewRInteger(255)
n.ToHex()                              // 十六进制: "ff"
n.ToOct()                              // 八进制: "377"
n.ToBin()                              // 二进制: "11111111"
n.ToBase(goby.NewRInteger(36))         // 36进制: "73"
n.Chr()                                // ASCII字符: "ÿ"
n.Digits()                             // 返回各位数字: [5, 5, 2]

// 取整与舍入
n = goby.NewRInteger(42)
n.RoundWithPrecision(goby.NewRInteger(-1))  // 四舍五入到十位: 40
n.CeilWithPrecision(goby.NewRInteger(-1))   // 向上取整到十位: 50
n.FloorWithPrecision(goby.NewRInteger(-1))  // 向下取整到十位: 40

// 遍历示例
goby.NewRInteger(5).Times(func(i goby.RInteger) {
    fmt.Println(i.ToString())         // 打印 0, 1, 2, 3, 4
})
```

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
```

## RInteger 完整方法列表

RInteger 类提供了丰富的方法，完全兼容 Ruby 的 Integer 类：

### 查询方法
- `Even()`: 检查是否为偶数
- `Odd()`: 检查是否为奇数
- `Zero()`: 检查是否为零
- `Positive()`: 检查是否为正数
- `Negative()`: 检查是否为负数
- `AllBits(mask)`: 检查所有指定位是否都为1
- `AnyBits(mask)`: 检查是否有任意指定位为1
- `NoBits(mask)`: 检查所有指定位是否都为0
- `BitLength()`: 获取二进制表示的位数

### 数学运算
- `Add(other)`: 加法
- `Sub(other)`: 减法
- `Mul(other)`: 乘法
- `Div(other)`: 整除
- `Mod(other)`, `Modulo(other)`: 取模
- `Pow(exponent)`: 幂运算
- `Abs()`: 绝对值
- `Gcd(other)`: 最大公约数
- `Lcm(other)`: 最小公倍数
- `GcdLcm(other)`: 同时返回最大公约数和最小公倍数
- `DivMod(other)`: 同时返回除法和取模结果
- `CeilDiv(other)`: 向上取整除法
- `FDiv(other)`: 浮点除法

### 位运算
- `BitAnd(other)`: 位与运算 (&)
- `BitOr(other)`: 位或运算 (|)
- `BitXor(other)`: 位异或运算 (^)
- `BitNot()`: 按位取反运算 (~)
- `LeftShift(count)`: 左移运算 (<<)
- `RightShift(count)`: 右移运算 (>>)
- `BitAt(pos)`: 获取指定位的值 ([])

### 取整与舍入
- `Ceil()`: 向上取整
- `Floor()`: 向下取整
- `Round()`: 四舍五入
- `Truncate()`: 截断
- `CeilWithPrecision(digits)`: 指定精度向上取整
- `FloorWithPrecision(digits)`: 指定精度向下取整
- `RoundWithPrecision(digits)`: 指定精度四舍五入
- `TruncateWithPrecision(digits)`: 指定精度截断

### 转换方法
- `ToString()`: 转为字符串
- `ToRString()`: 转为 RString 对象
- `ToInt()`: 返回自身
- `ToInteger()`: 返回自身
- `ToFloat()`: 转为浮点数
- `ToRational()`: 转为有理数
- `ToHex()`: 转为十六进制字符串
- `ToOct()`: 转为八进制字符串
- `ToBin()`: 转为二进制字符串
- `ToBase(base)`: 转为指定进制字符串
- `Digits(base...)`: 将数字转为各位数字数组
- `Chr()`: 返回对应ASCII字符
- `Ord()`: 返回对应数值

### 其他方法
- `Succ()`, `Next()`: 返回下一个整数
- `Pred()`: 返回上一个整数
- `Times(fn)`: 执行指定次数
- `UpTo(limit, fn)`: 递增执行
- `DownTo(limit, fn)`: 递减执行
- `Coerce(other)`: 类型强制转换
- `Size()`: 返回整数表示所占字节数

## 测试

```bash
go test -v
```

## 贡献

欢迎提交 Pull Request 和 Issue！

