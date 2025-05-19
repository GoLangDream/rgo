# RInteger - Ruby风格的整数类

`RInteger`是一个Go语言实现的模拟Ruby整数功能的类型。它提供了丰富的整数操作方法，可以让Go开发者使用类似Ruby的整数API。

## 创建RInteger

```go
// 创建一个新的 RInteger
i := NewRInteger(42)
```

## 基本方法

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
- `AddInt(other)`: 与原生 int 类型相加
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

## 使用示例

```go
import "github.com/GoLangDream/rgo"

num := rgo.NewRInteger(42)
isEven := num.Even()                  // 返回 true
sum := num.Add(rgo.NewRInteger(10))  // 返回 52
abs := rgo.NewRInteger(-10).Abs()    // 返回 10

// 基本数学运算
a := rgo.NewRInteger(10)
b := rgo.NewRInteger(3)
gcd := a.Gcd(b)                       // 最大公约数: 1
lcm := a.Lcm(b)                       // 最小公倍数: 30
divmod := a.DivMod(b)                 // 返回商和余数: [3, 1]

// 位运算
bits := rgo.NewRInteger(10)           // 二进制: 1010
mask := rgo.NewRInteger(12)           // 二进制: 1100
bits.BitAnd(mask)                      // 位与: 8 (1000)
bits.BitOr(mask)                       // 位或: 14 (1110)
bits.BitXor(mask)                      // 位异或: 6 (0110)
bits.LeftShift(rgo.NewRInteger(1))    // 左移: 20 (10100)
bits.BitAt(rgo.NewRInteger(1))        // 获取第1位: 1
bits.AllBits(rgo.NewRInteger(8))      // 检查位: true (8=1000)

// 格式转换
n := rgo.NewRInteger(255)
n.ToHex()                              // 十六进制: "ff"
n.ToOct()                              // 八进制: "377"
n.ToBin()                              // 二进制: "11111111"
n.ToBase(rgo.NewRInteger(36))         // 36进制: "73"
n.Chr()                                // ASCII字符: "ÿ"
n.Digits()                             // 返回各位数字: [5, 5, 2]

// 取整与舍入
n = rgo.NewRInteger(42)
n.RoundWithPrecision(rgo.NewRInteger(-1))  // 四舍五入到十位: 40
n.CeilWithPrecision(rgo.NewRInteger(-1))   // 向上取整到十位: 50
n.FloorWithPrecision(rgo.NewRInteger(-1))  // 向下取整到十位: 40

// 遍历示例
rgo.NewRInteger(5).Times(func(i rgo.RInteger) {
    fmt.Println(i.ToString())         // 打印 0, 1, 2, 3, 4
})
```

## 注意事项

1. 所有数学运算方法都会返回新的 `RInteger` 实例，不会修改原实例
2. 除法和取模操作在除数为零时会触发 panic
3. 幂运算在指数为负数时会返回 0
4. 所有比较操作都返回布尔值
