package goby

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
)

// RInteger 实现类似 Ruby 中 Integer 类的功能
type RInteger struct {
	BaseObject
	value int
}

// NewRInteger 创建一个新的 RInteger 对象
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

// Equal 比较两个对象是否相等
func (i RInteger) Equal(other Object) bool {
	if otherInt, ok := other.(RInteger); ok {
		return i.value == otherInt.value
	}
	return false
}

// Value 返回 RInteger 的底层 int 值
func (i RInteger) Value() int {
	return i.value
}

// Add 加法操作
func (i RInteger) Add(other RInteger) RInteger {
	return NewRInteger(i.value + other.value)
}

// Sub 减法操作
func (i RInteger) Sub(other RInteger) RInteger {
	return NewRInteger(i.value - other.value)
}

// Mul 乘法操作
func (i RInteger) Mul(other RInteger) RInteger {
	return NewRInteger(i.value * other.value)
}

// Div 除法操作
func (i RInteger) Div(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value / other.value)
}

// Mod 取模操作
func (i RInteger) Mod(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return NewRInteger(i.value % other.value)
}

// Modulo 是 Mod 的别名（Ruby兼容）
func (i RInteger) Modulo(other RInteger) RInteger {
	return i.Mod(other)
}

// Pow 幂运算
func (i RInteger) Pow(exponent RInteger) RInteger {
	return NewRInteger(int(math.Pow(float64(i.value), float64(exponent.value))))
}

// BitAnd 位与操作（&）
func (i RInteger) BitAnd(other RInteger) RInteger {
	return NewRInteger(i.value & other.value)
}

// BitOr 位或操作（|）
func (i RInteger) BitOr(other RInteger) RInteger {
	return NewRInteger(i.value | other.value)
}

// BitXor 异或操作（^）
func (i RInteger) BitXor(other RInteger) RInteger {
	return NewRInteger(i.value ^ other.value)
}

// BitNot 按位取反操作（~）
func (i RInteger) BitNot() RInteger {
	return NewRInteger(^i.value)
}

// LeftShift 左移操作（<<）
func (i RInteger) LeftShift(count RInteger) RInteger {
	return NewRInteger(i.value << uint(count.value))
}

// RightShift 右移操作（>>）
func (i RInteger) RightShift(count RInteger) RInteger {
	return NewRInteger(i.value >> uint(count.value))
}

// BitAt 获取指定位的值（[]）
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

// 辅助函数，计算一个无符号整数的位数
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

// Abs 返回绝对值
func (i RInteger) Abs() RInteger {
	if i.value < 0 {
		return NewRInteger(-i.value)
	}
	return i
}

// Even 判断是否为偶数
func (i RInteger) Even() bool {
	return i.value%2 == 0
}

// Odd 判断是否为奇数
func (i RInteger) Odd() bool {
	return i.value%2 != 0
}

// Zero 判断是否为零
func (i RInteger) Zero() bool {
	return i.value == 0
}

// Positive 判断是否为正数
func (i RInteger) Positive() bool {
	return i.value > 0
}

// Negative 判断是否为负数
func (i RInteger) Negative() bool {
	return i.value < 0
}

// Gcd 求最大公约数
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

// Lcm 求最小公倍数
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

// GcdLcm 同时返回最大公约数和最小公倍数
func (i RInteger) GcdLcm(other RInteger) RArray {
	gcd := i.Gcd(other)
	lcm := i.Lcm(other)

	return NewRArray([]Object{gcd, lcm})
}

// DivMod 同时返回除法和取模的结果
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

// CeilDiv 向上取整除法
func (i RInteger) CeilDiv(other RInteger) RInteger {
	if other.value == 0 {
		panic("除数不能为零")
	}

	// 如果是正负不同，则直接使用普通除法
	if (i.value < 0) != (other.value < 0) {
		return NewRInteger(i.value / other.value)
	}

	// 否则使用ceil除法
	return NewRInteger((i.value + other.value - 1) / other.value)
}

// Ceil 向上取整（对整数来说就是自身）
func (i RInteger) Ceil() RInteger {
	return i
}

// Ceil 指定精度的向上取整
func (i RInteger) CeilWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	// 对于负精度，需要将对应位数置零
	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	if remainder > 0 {
		return NewRInteger(i.value + (pow - remainder))
	} else if remainder < 0 {
		return NewRInteger(i.value - remainder)
	}

	return i
}

// Floor 向下取整（对整数来说就是自身）
func (i RInteger) Floor() RInteger {
	return i
}

// FloorWithPrecision 指定精度的向下取整
func (i RInteger) FloorWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	// 对于负精度，需要将对应位数置零
	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	if remainder > 0 {
		return NewRInteger(i.value - remainder)
	} else if remainder < 0 {
		return NewRInteger(i.value - (remainder + pow))
	}

	return i
}

// Round 四舍五入（对整数来说就是自身）
func (i RInteger) Round() RInteger {
	return i
}

// RoundWithPrecision 指定精度的四舍五入
func (i RInteger) RoundWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	// 对于负精度，需要将对应位数进行四舍五入
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

// Truncate 截断（对整数来说就是自身）
func (i RInteger) Truncate() RInteger {
	return i
}

// TruncateWithPrecision 指定精度的截断
func (i RInteger) TruncateWithPrecision(digits RInteger) RInteger {
	if digits.value >= 0 {
		return i
	}

	// 对于负精度，需要将对应位数置零
	pow := int(math.Pow(10, float64(-digits.value)))
	remainder := i.value % pow

	return NewRInteger(i.value - remainder)
}

// Succ 返回下一个整数（相当于+1）
func (i RInteger) Succ() RInteger {
	return NewRInteger(i.value + 1)
}

// Next 是 Succ 的别名
func (i RInteger) Next() RInteger {
	return i.Succ()
}

// Pred 返回前一个整数（相当于-1）
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

// ToRString 转换为 RString
func (i RInteger) ToRString() RString {
	return NewRString(i.ToString())
}

// ToInt 返回自身（类似Ruby的to_i）
func (i RInteger) ToInt() RInteger {
	return i
}

// ToInteger 返回自身（类似Ruby的to_int）
func (i RInteger) ToInteger() RInteger {
	return i
}

// ToFloat 转换为浮点数
func (i RInteger) ToFloat() float64 {
	return float64(i.value)
}

// ToRational 转换为有理数（使用big.Rat模拟）
func (i RInteger) ToRational() *big.Rat {
	return big.NewRat(int64(i.value), 1)
}

// FDiv 浮点除法
func (i RInteger) FDiv(other RInteger) float64 {
	if other.value == 0 {
		panic("除数不能为零")
	}
	return float64(i.value) / float64(other.value)
}

// ToHex 转换为十六进制字符串
func (i RInteger) ToHex() RString {
	return NewRString(fmt.Sprintf("%x", i.value))
}

// ToOct 转换为八进制字符串
func (i RInteger) ToOct() RString {
	return NewRString(fmt.Sprintf("%o", i.value))
}

// ToBin 转换为二进制字符串
func (i RInteger) ToBin() RString {
	return NewRString(fmt.Sprintf("%b", i.value))
}

// ToBase 转换为指定进制字符串
func (i RInteger) ToBase(base RInteger) RString {
	if base.value < 2 || base.value > 36 {
		panic("进制必须在2到36之间")
	}
	return NewRString(strconv.FormatInt(int64(i.value), base.value))
}

// Digits 将数字转换为按位数字的数组
// 例如 12345.Digits() 返回 [5,4,3,2,1]
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

// Chr 返回当前数值对应的字符
func (i RInteger) Chr() RString {
	return NewRString(string(rune(i.value)))
}

// Ord 返回自身（与 RString 的 Ord 方法保持一致）
func (i RInteger) Ord() RInteger {
	return i
}

// Size 返回整数的字节表示长度
func (i RInteger) Size() int {
	return strconv.IntSize / 8 // 返回当前平台整数占用的字节数
}

// Coerce 将另一个对象转为兼容类型
func (i RInteger) Coerce(other Object) RArray {
	switch v := other.(type) {
	case RInteger:
		return NewRArray([]Object{v, i})
	default:
		// 尝试转换为数字
		if str, ok := other.(RString); ok {
			val, err := strconv.Atoi(str.ToString())
			if err == nil {
				return NewRArray([]Object{NewRInteger(val), i})
			}
		}
		panic("无法转换为兼容类型")
	}
}
