// Package goby 提供了一个类似 Ruby 的整数实现
package goby

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
)

// RInteger 实现了类似 Ruby 的整数功能
type RInteger struct {
	BaseObject
	value int
}

// NewRInteger 创建一个新的整数对象
func NewRInteger(value int) RInteger {
	return RInteger{
		BaseObject: NewBaseObject("Integer"),
		value:      value,
	}
}

// ToString 返回整数的字符串表示
func (i RInteger) ToString() string {
	return strconv.Itoa(i.value)
}

// Equal 比较两个整数是否相等
func (i RInteger) Equal(other Object) bool {
	if otherInt, ok := other.(RInteger); ok {
		return i.value == otherInt.value
	}
	return false
}

// Value 返回整数的底层值
func (i RInteger) Value() int {
	return i.value
}

// Add 返回整数与原生 int 的和
func (i RInteger) Add(other int) RInteger {
	return NewRInteger(i.value + other)
}

// AddRInt 返回两个 RInteger 的和
func (i RInteger) AddRInt(other RInteger) RInteger {
	return NewRInteger(i.value + other.value)
}

// Sub 返回整数与原生 int 的差
func (i RInteger) Sub(other int) RInteger {
	return NewRInteger(i.value - other)
}

// SubRInt 返回两个 RInteger 的差
func (i RInteger) SubRInt(other RInteger) RInteger {
	return NewRInteger(i.value - other.value)
}

// Mul 返回整数与原生 int 的积
func (i RInteger) Mul(other int) RInteger {
	return NewRInteger(i.value * other)
}

// MulRInt 返回两个 RInteger 的积
func (i RInteger) MulRInt(other RInteger) RInteger {
	return NewRInteger(i.value * other.value)
}

// Div 返回整数与原生 int 的商
func (i RInteger) Div(other int) RInteger {
	if other == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value / other)
}

// DivRInt 返回两个 RInteger 的商
func (i RInteger) DivRInt(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value / other.value)
}

// Mod 返回整数与原生 int 的余数
func (i RInteger) Mod(other int) RInteger {
	if other == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value % other)
}

// ModRInt 返回两个 RInteger 的余数
func (i RInteger) ModRInt(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value % other.value)
}

// Modulo 返回整数与原生 int 的余数（Mod 的别名）
func (i RInteger) Modulo(other int) RInteger {
	return i.Mod(other)
}

// ModuloRInt 返回两个 RInteger 的余数（ModRInt 的别名）
func (i RInteger) ModuloRInt(other RInteger) RInteger {
	return i.ModRInt(other)
}

// Pow 返回整数与原生 int 的幂
func (i RInteger) Pow(exponent int) RInteger {
	return NewRInteger(int(math.Pow(float64(i.value), float64(exponent))))
}

// PowRInt 返回两个 RInteger 的幂
func (i RInteger) PowRInt(exponent RInteger) RInteger {
	return NewRInteger(int(math.Pow(float64(i.value), float64(exponent.value))))
}

// BitAnd 返回两个整数的按位与结果
func (i RInteger) BitAnd(other RInteger) RInteger {
	return NewRInteger(i.value & other.value)
}

// BitOr 返回两个整数的按位或结果
func (i RInteger) BitOr(other RInteger) RInteger {
	return NewRInteger(i.value | other.value)
}

// BitXor 返回两个整数的按位异或结果
func (i RInteger) BitXor(other RInteger) RInteger {
	return NewRInteger(i.value ^ other.value)
}

// BitNot 返回整数的按位取反结果
func (i RInteger) BitNot() RInteger {
	return NewRInteger(^i.value)
}

// LeftShift 返回整数左移指定位数的结果
func (i RInteger) LeftShift(count RInteger) RInteger {
	return NewRInteger(i.value << uint(count.value))
}

// RightShift 返回整数右移指定位数的结果
func (i RInteger) RightShift(count RInteger) RInteger {
	return NewRInteger(i.value >> uint(count.value))
}

// BitAt 返回整数指定位置的值
func (i RInteger) BitAt(pos RInteger) RInteger {
	if pos.value < 0 {
		return NewRInteger(0)
	}

	bit := (i.value >> uint(pos.value)) & 1
	return NewRInteger(bit)
}

// AllBits 检查是否所有指定的位都是1
func (i RInteger) AllBits(mask RInteger) bool {
	return (i.value & mask.value) == mask.value
}

// AnyBits 检查是否任何指定的位是1
func (i RInteger) AnyBits(mask RInteger) bool {
	return (i.value & mask.value) != 0
}

// NoBits 检查是否所有指定的位都是0
func (i RInteger) NoBits(mask RInteger) bool {
	return (i.value & mask.value) == 0
}

// BitLength 返回表示此整数的二进制形式的位数
func (i RInteger) BitLength() int {
	if i.value == 0 {
		return 0
	}

	v := uint(i.value)
	if i.value < 0 {
		v = uint(-i.value - 1)
	}

	return bits(v) + 1
}

// bits 计算一个无符号整数的位数
func bits(v uint) int {
	if v == 0 {
		return 0
	}
	n := 0
	for ; v > 0; v >>= 1 {
		n++
	}
	return n - 1
}

// Abs 返回整数的绝对值
func (i RInteger) Abs() RInteger {
	if i.value < 0 {
		return NewRInteger(-i.value)
	}
	return i
}

// Even 检查整数是否为偶数
func (i RInteger) Even() bool {
	return i.value%2 == 0
}

// Odd 检查整数是否为奇数
func (i RInteger) Odd() bool {
	return i.value%2 != 0
}

// Zero 检查整数是否为零
func (i RInteger) Zero() bool {
	return i.value == 0
}

// Positive 检查整数是否为正数
func (i RInteger) Positive() bool {
	return i.value > 0
}

// Negative 检查整数是否为负数
func (i RInteger) Negative() bool {
	return i.value < 0
}

// Gcd 返回两个整数的最大公约数
func (i RInteger) Gcd(other RInteger) RInteger {
	a, b := i.value, other.value
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}

	for b != 0 {
		a, b = b, a%b
	}

	return NewRInteger(a)
}

// Lcm 返回两个整数的最小公倍数
func (i RInteger) Lcm(other RInteger) RInteger {
	if i.value == 0 || other.value == 0 {
		return NewRInteger(0)
	}

	a, b := i.value, other.value
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}

	return NewRInteger((a / i.Gcd(other).value) * b)
}

// GcdLcm 同时返回两个整数的最大公约数和最小公倍数
func (i RInteger) GcdLcm(other RInteger) RArray {
	gcd := i.Gcd(other)
	lcm := i.Lcm(other)

	return NewRArray([]Object{gcd, lcm})
}

// DivMod 同时返回两个整数的商和余数
func (i RInteger) DivMod(other RInteger) RArray {
	if other.value == 0 {
		panic("除数不能为零")
	}

	quotient := i.value / other.value
	remainder := i.value % other.value

	return NewRArray([]Object{
		NewRInteger(quotient),
		NewRInteger(remainder),
	})
}

// CeilDiv 返回两个整数的向上取整除法结果
func (i RInteger) CeilDiv(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}

	if (i.value < 0) != (other.value < 0) {
		return NewRInteger(i.value / other.value)
	}

	return NewRInteger((i.value + other.value - 1) / other.value)
}

// Ceil 返回整数的向上取整结果
func (i RInteger) Ceil() RInteger {
	return i
}

// CeilWithPrecision 返回整数指定精度的向上取整结果
func (i RInteger) CeilWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	if remainder > 0 {
		return NewRInteger(i.value + (pow - remainder))
	} else if remainder < 0 {
		return NewRInteger(i.value - remainder)
	}

	return i
}

// Floor 返回整数的向下取整结果
func (i RInteger) Floor() RInteger {
	return i
}

// FloorWithPrecision 返回整数指定精度的向下取整结果
func (i RInteger) FloorWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	if remainder > 0 {
		return NewRInteger(i.value - remainder)
	} else if remainder < 0 {
		return NewRInteger(i.value - (remainder + pow))
	}

	return i
}

// Round 返回整数的四舍五入结果
func (i RInteger) Round() RInteger {
	return i
}

// RoundWithPrecision 返回整数指定精度的四舍五入结果
func (i RInteger) RoundWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	pow := int(math.Pow(10, float64(-digits.value)))
	half := pow / 2

	remainder := i.value % pow
	if remainder >= half {
		return NewRInteger(i.value + (pow - remainder))
	} else if remainder <= -half {
		return NewRInteger(i.value - (remainder + pow))
	} else if remainder > 0 {
		return NewRInteger(i.value - remainder)
	} else {
		return NewRInteger(i.value - remainder)
	}
}

// Truncate 返回整数的截断结果
func (i RInteger) Truncate() RInteger {
	return i
}

// TruncateWithPrecision 返回整数指定精度的截断结果
func (i RInteger) TruncateWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	return NewRInteger(i.value - remainder)
}

// Succ 返回整数的下一个值
func (i RInteger) Succ() RInteger {
	return NewRInteger(i.value + 1)
}

// Next 返回整数的下一个值（Succ 的别名）
func (i RInteger) Next() RInteger {
	return i.Succ()
}

// Pred 返回整数的前一个值
func (i RInteger) Pred() RInteger {
	return NewRInteger(i.value - 1)
}

// Times 执行指定次数的操作
func (i RInteger) Times(fn func(RInteger)) {
	for j := 0; j < i.value; j++ {
		fn(NewRInteger(j))
	}
}

// UpTo 从当前值递增到指定值
func (i RInteger) UpTo(limit RInteger, fn func(RInteger)) {
	for j := i.value; j <= limit.value; j++ {
		fn(NewRInteger(j))
	}
}

// DownTo 从当前值递减到指定值
func (i RInteger) DownTo(limit RInteger, fn func(RInteger)) {
	for j := i.value; j >= limit.value; j-- {
		fn(NewRInteger(j))
	}
}

// ToRString 将整数转换为字符串
func (i RInteger) ToRString() RString {
	return NewRString(i.ToString())
}

// ToInt 返回整数本身
func (i RInteger) ToInt() RInteger {
	return i
}

// ToInteger 返回整数本身
func (i RInteger) ToInteger() RInteger {
	return i
}

// ToFloat 将整数转换为浮点数
func (i RInteger) ToFloat() float64 {
	return float64(i.value)
}

// ToRational 将整数转换为有理数
func (i RInteger) ToRational() *big.Rat {
	return big.NewRat(int64(i.value), 1)
}

// FDiv 返回两个整数的浮点除法结果
func (i RInteger) FDiv(other RInteger) float64 {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return float64(i.value) / float64(other.value)
}

// ToHex 将整数转换为十六进制字符串
func (i RInteger) ToHex() RString {
	return NewRString(fmt.Sprintf("%x", i.value))
}

// ToOct 将整数转换为八进制字符串
func (i RInteger) ToOct() RString {
	return NewRString(fmt.Sprintf("%o", i.value))
}

// ToBin 将整数转换为二进制字符串
func (i RInteger) ToBin() RString {
	return NewRString(fmt.Sprintf("%b", i.value))
}

// ToBase 将整数转换为指定进制的字符串
func (i RInteger) ToBase(base RInteger) RString {
	if base.value < 2 || base.value > 36 {
		panic("进制必须在2到36之间")
	}
	return NewRString(strconv.FormatInt(int64(i.value), base.value))
}

// Digits 将整数转换为按位数字的数组
func (i RInteger) Digits(base ...int) RArray {
	baseValue := 10
	if len(base) > 0 {
		baseValue = base[0]
		if baseValue < 2 {
			panic("进制必须大于等于2")
		}
	}

	if i.value == 0 {
		return NewRArray([]Object{NewRInteger(0)})
	}

	n := i.value
	if n < 0 {
		n = -n
	}

	var digits []Object
	for n > 0 {
		digits = append(digits, NewRInteger(n%baseValue))
		n /= baseValue
	}

	return NewRArray(digits)
}

// Chr 返回整数对应的 Unicode 字符
func (i RInteger) Chr() RString {
	return NewRString(string(rune(i.value)))
}

// Ord 返回整数本身
func (i RInteger) Ord() RInteger {
	return i
}

// Size 返回整数的字节表示长度
func (i RInteger) Size() int {
	return strconv.IntSize / 8
}

// Coerce 将另一个对象转换为兼容类型
func (i RInteger) Coerce(other Object) RArray {
	switch v := other.(type) {
	case RInteger:
		return NewRArray([]Object{v, i})
	default:
		if str, ok := other.(RString); ok {
			val, err := strconv.Atoi(str.ToString())
			if err == nil {
				return NewRArray([]Object{NewRInteger(val), i})
			}
		}
		panic("无法转换为兼容类型")
	}
}
