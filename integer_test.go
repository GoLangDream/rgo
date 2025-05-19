package rgo_test

import (
	"testing"

	. "github.com/GoLangDream/rgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RInteger", func() {
	var (
		zero     RInteger
		positive RInteger
		negative RInteger
	)

	BeforeEach(func() {
		zero = NewRInteger(0)
		positive = NewRInteger(42)
		negative = NewRInteger(-10)
	})

	Context("基本属性检查", func() {
		It("应该正确检查零值", func() {
			Expect(zero.Zero()).To(BeTrue())
			Expect(positive.Zero()).To(BeFalse())
			Expect(negative.Zero()).To(BeFalse())
		})

		It("应该正确检查正负性", func() {
			Expect(positive.Positive()).To(BeTrue())
			Expect(negative.Positive()).To(BeFalse())
			Expect(negative.Negative()).To(BeTrue())
			Expect(zero.Positive()).To(BeFalse())
			Expect(zero.Negative()).To(BeFalse())
		})

		It("应该正确检查奇偶性", func() {
			Expect(NewRInteger(42).Even()).To(BeTrue())
			Expect(NewRInteger(43).Even()).To(BeFalse())
			Expect(NewRInteger(43).Odd()).To(BeTrue())
			Expect(zero.Even()).To(BeTrue())
		})
	})

	Context("数学运算", func() {
		It("应该正确执行加法运算", func() {
			Expect(positive.AddRInt(negative).Value()).To(Equal(32))
			Expect(negative.AddRInt(positive).Value()).To(Equal(32))
			Expect(zero.AddRInt(positive).Value()).To(Equal(42))
		})

		It("应该正确执行与原生 int 的加法运算", func() {
			Expect(positive.Add(-10).Value()).To(Equal(32))
			Expect(negative.Add(42).Value()).To(Equal(32))
			Expect(zero.Add(42).Value()).To(Equal(42))
		})

		It("应该正确执行减法运算", func() {
			Expect(positive.SubRInt(negative).Value()).To(Equal(52))
			Expect(negative.SubRInt(positive).Value()).To(Equal(-52))
			Expect(zero.SubRInt(negative).Value()).To(Equal(10))
		})

		It("应该正确执行乘法运算", func() {
			Expect(positive.MulRInt(NewRInteger(2)).Value()).To(Equal(84))
			Expect(negative.MulRInt(NewRInteger(3)).Value()).To(Equal(-30))
			Expect(zero.MulRInt(positive).Value()).To(Equal(0))
		})

		It("应该正确执行除法运算", func() {
			Expect(positive.DivRInt(NewRInteger(2)).Value()).To(Equal(21))
			Expect(negative.DivRInt(NewRInteger(2)).Value()).To(Equal(-5))
			Expect(func() { zero.DivRInt(zero) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确执行取模运算", func() {
			Expect(positive.ModRInt(NewRInteger(5)).Value()).To(Equal(2))
			Expect(negative.ModRInt(NewRInteger(3)).Value()).To(Equal(-1))
			Expect(func() { positive.ModRInt(zero) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确执行幂运算", func() {
			Expect(NewRInteger(2).PowRInt(NewRInteger(3)).Value()).To(Equal(8))
			Expect(NewRInteger(3).PowRInt(NewRInteger(2)).Value()).To(Equal(9))
			Expect(zero.PowRInt(positive).Value()).To(Equal(0))
		})

		It("应该正确计算绝对值", func() {
			Expect(positive.Abs().Value()).To(Equal(42))
			Expect(negative.Abs().Value()).To(Equal(10))
			Expect(zero.Abs().Value()).To(Equal(0))
		})

		It("应该正确计算最大公约数", func() {
			Expect(NewRInteger(12).Gcd(NewRInteger(8)).Value()).To(Equal(4))
			Expect(NewRInteger(12).Gcd(NewRInteger(9)).Value()).To(Equal(3))
			Expect(NewRInteger(-12).Gcd(NewRInteger(8)).Value()).To(Equal(4))
		})

		It("应该正确计算最小公倍数", func() {
			Expect(NewRInteger(12).Lcm(NewRInteger(8)).Value()).To(Equal(24))
			Expect(NewRInteger(12).Lcm(NewRInteger(9)).Value()).To(Equal(36))
			Expect(NewRInteger(0).Lcm(NewRInteger(8)).Value()).To(Equal(0))
		})

		It("应该正确同时返回最大公约数和最小公倍数", func() {
			result := NewRInteger(12).GcdLcm(NewRInteger(8))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(4))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(24))
		})

		It("应该正确同时返回商和余数", func() {
			result := NewRInteger(13).DivMod(NewRInteger(5))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(2))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(3))
		})

		It("应该正确执行向上取整除法", func() {
			Expect(NewRInteger(7).CeilDiv(NewRInteger(2)).Value()).To(Equal(4))
			Expect(NewRInteger(6).CeilDiv(NewRInteger(2)).Value()).To(Equal(3))
			Expect(NewRInteger(-7).CeilDiv(NewRInteger(2)).Value()).To(Equal(-3))
		})

		It("应该正确执行浮点除法", func() {
			Expect(NewRInteger(5).FDiv(NewRInteger(2))).To(Equal(2.5))
			Expect(NewRInteger(-5).FDiv(NewRInteger(2))).To(Equal(-2.5))
			Expect(func() { positive.FDiv(zero) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确处理除零错误", func() {
			Expect(func() { positive.DivRInt(zero) }).To(PanicWith("除数不能为零"))
			Expect(func() { negative.DivRInt(zero) }).To(PanicWith("除数不能为零"))
			Expect(func() { zero.DivRInt(zero) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确处理取模运算的边界情况", func() {
			Expect(func() { positive.ModRInt(zero) }).To(PanicWith("除数不能为零"))
			Expect(func() { negative.ModRInt(zero) }).To(PanicWith("除数不能为零"))
			Expect(func() { zero.ModRInt(zero) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确处理幂运算的边界情况", func() {
			Expect(NewRInteger(0).PowRInt(NewRInteger(0)).Value()).To(Equal(1))
			Expect(NewRInteger(0).PowRInt(NewRInteger(1)).Value()).To(Equal(0))
			Expect(NewRInteger(1).PowRInt(NewRInteger(0)).Value()).To(Equal(1))
			Expect(NewRInteger(1).PowRInt(NewRInteger(1)).Value()).To(Equal(1))
		})

		It("应该正确处理最大公约数的边界情况", func() {
			Expect(NewRInteger(0).Gcd(NewRInteger(5)).Value()).To(Equal(5))
			Expect(NewRInteger(5).Gcd(NewRInteger(0)).Value()).To(Equal(5))
			Expect(NewRInteger(0).Gcd(NewRInteger(0)).Value()).To(Equal(0))
		})

		It("应该正确处理最小公倍数的边界情况", func() {
			Expect(NewRInteger(0).Lcm(NewRInteger(5)).Value()).To(Equal(0))
			Expect(NewRInteger(5).Lcm(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).Lcm(NewRInteger(0)).Value()).To(Equal(0))
		})

		It("应该正确处理同时返回最大公约数和最小公倍数的边界情况", func() {
			result := NewRInteger(0).GcdLcm(NewRInteger(5))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(5))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(0))

			result = NewRInteger(5).GcdLcm(NewRInteger(0))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(5))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(0))

			result = NewRInteger(0).GcdLcm(NewRInteger(0))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(0))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(0))
		})

		It("应该正确处理同时返回商和余数的边界情况", func() {
			result := NewRInteger(0).DivMod(NewRInteger(5))
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(0))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(0))

			Expect(func() { NewRInteger(5).DivMod(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
			Expect(func() { NewRInteger(0).DivMod(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确处理向上取整除法的边界情况", func() {
			Expect(func() { NewRInteger(5).CeilDiv(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
			Expect(func() { NewRInteger(0).CeilDiv(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
		})

		It("应该正确处理浮点除法的边界情况", func() {
			Expect(func() { NewRInteger(5).FDiv(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
			Expect(func() { NewRInteger(0).FDiv(NewRInteger(0)) }).To(PanicWith("除数不能为零"))
		})
	})

	Context("位运算", func() {
		It("应该正确执行位与操作", func() {
			Expect(NewRInteger(0b1010).BitAnd(NewRInteger(0b1100)).Value()).To(Equal(0b1000))
		})

		It("应该正确执行位或操作", func() {
			Expect(NewRInteger(0b1010).BitOr(NewRInteger(0b1100)).Value()).To(Equal(0b1110))
		})

		It("应该正确执行位异或操作", func() {
			Expect(NewRInteger(0b1010).BitXor(NewRInteger(0b1100)).Value()).To(Equal(0b0110))
		})

		It("应该正确执行位取反操作", func() {
			// 注意：Go的位取反是按照补码形式进行的
			Expect(NewRInteger(0b1010).BitNot().Value()).To(Equal(^0b1010))
		})

		It("应该正确执行左移操作", func() {
			Expect(NewRInteger(1).LeftShift(NewRInteger(2)).Value()).To(Equal(4))
			Expect(NewRInteger(5).LeftShift(NewRInteger(1)).Value()).To(Equal(10))
		})

		It("应该正确执行右移操作", func() {
			Expect(NewRInteger(4).RightShift(NewRInteger(1)).Value()).To(Equal(2))
			Expect(NewRInteger(10).RightShift(NewRInteger(1)).Value()).To(Equal(5))
		})

		It("应该正确获取指定位的值", func() {
			n := NewRInteger(0b1010)
			Expect(n.BitAt(NewRInteger(1)).Value()).To(Equal(1))
			Expect(n.BitAt(NewRInteger(0)).Value()).To(Equal(0))
			Expect(n.BitAt(NewRInteger(3)).Value()).To(Equal(1))
			Expect(n.BitAt(NewRInteger(2)).Value()).To(Equal(0))
			Expect(n.BitAt(NewRInteger(4)).Value()).To(Equal(0))
		})

		It("应该正确检查位状态", func() {
			n := NewRInteger(0b1010)                             // 10 in decimal
			Expect(n.AllBits(NewRInteger(0b1000))).To(BeTrue())  // 10 & 8 == 8
			Expect(n.AllBits(NewRInteger(0b1010))).To(BeTrue())  // 10 & 10 == 10
			Expect(n.AllBits(NewRInteger(0b1110))).To(BeFalse()) // 10 & 14 != 14

			Expect(n.AnyBits(NewRInteger(0b1000))).To(BeTrue())  // 10 & 8 > 0
			Expect(n.AnyBits(NewRInteger(0b0100))).To(BeFalse()) // 10 & 4 == 0
			Expect(n.AnyBits(NewRInteger(0b0001))).To(BeFalse()) // 10 & 1 == 0

			Expect(n.NoBits(NewRInteger(0b0100))).To(BeTrue())  // 10 & 4 == 0
			Expect(n.NoBits(NewRInteger(0b0001))).To(BeTrue())  // 10 & 1 == 0
			Expect(n.NoBits(NewRInteger(0b1000))).To(BeFalse()) // 10 & 8 > 0
		})

		It("应该正确计算位长度", func() {
			Expect(NewRInteger(0).BitLength()).To(Equal(0))
			Expect(NewRInteger(1).BitLength()).To(Equal(1))
			Expect(NewRInteger(2).BitLength()).To(Equal(2))
			Expect(NewRInteger(3).BitLength()).To(Equal(2))
			Expect(NewRInteger(4).BitLength()).To(Equal(3))
			Expect(NewRInteger(8).BitLength()).To(Equal(4))
		})

		It("应该正确处理位运算的边界情况", func() {
			Expect(NewRInteger(0).BitAnd(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).BitOr(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).BitXor(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).BitNot().Value()).To(Equal(-1))
		})

		It("应该正确处理位移操作的边界情况", func() {
			Expect(NewRInteger(0).LeftShift(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).RightShift(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).LeftShift(NewRInteger(1)).Value()).To(Equal(0))
			Expect(NewRInteger(0).RightShift(NewRInteger(1)).Value()).To(Equal(0))
		})

		It("应该正确处理位检查的边界情况", func() {
			Expect(NewRInteger(0).BitAt(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).AllBits(NewRInteger(0))).To(BeTrue())
			Expect(NewRInteger(0).AnyBits(NewRInteger(0))).To(BeFalse())
			Expect(NewRInteger(0).NoBits(NewRInteger(0))).To(BeTrue())
		})
	})

	Context("取整和舍入", func() {
		It("应该正确处理向上取整", func() {
			Expect(positive.Ceil().Value()).To(Equal(42))
			Expect(positive.CeilWithPrecision(NewRInteger(-1)).Value()).To(Equal(50))
			Expect(positive.CeilWithPrecision(NewRInteger(-2)).Value()).To(Equal(100))
			Expect(NewRInteger(42).CeilWithPrecision(NewRInteger(1)).Value()).To(Equal(42))
		})

		It("应该正确处理向下取整", func() {
			Expect(positive.Floor().Value()).To(Equal(42))
			Expect(positive.FloorWithPrecision(NewRInteger(-1)).Value()).To(Equal(40))
			Expect(positive.FloorWithPrecision(NewRInteger(-2)).Value()).To(Equal(0))
			Expect(NewRInteger(42).FloorWithPrecision(NewRInteger(1)).Value()).To(Equal(42))
		})

		It("应该正确处理四舍五入", func() {
			Expect(positive.Round().Value()).To(Equal(42))
			Expect(NewRInteger(45).RoundWithPrecision(NewRInteger(-1)).Value()).To(Equal(50))
			Expect(NewRInteger(44).RoundWithPrecision(NewRInteger(-1)).Value()).To(Equal(40))
			Expect(NewRInteger(42).RoundWithPrecision(NewRInteger(1)).Value()).To(Equal(42))
		})

		It("应该正确处理截断", func() {
			Expect(positive.Truncate().Value()).To(Equal(42))
			Expect(positive.TruncateWithPrecision(NewRInteger(-1)).Value()).To(Equal(40))
			Expect(negative.TruncateWithPrecision(NewRInteger(-1)).Value()).To(Equal(-10))
			Expect(NewRInteger(42).TruncateWithPrecision(NewRInteger(1)).Value()).To(Equal(42))
		})

		It("应该正确处理取整和舍入的边界情况", func() {
			Expect(NewRInteger(0).Ceil().Value()).To(Equal(0))
			Expect(NewRInteger(0).Floor().Value()).To(Equal(0))
			Expect(NewRInteger(0).Round().Value()).To(Equal(0))
			Expect(NewRInteger(0).Truncate().Value()).To(Equal(0))

			Expect(NewRInteger(0).CeilWithPrecision(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).FloorWithPrecision(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).RoundWithPrecision(NewRInteger(0)).Value()).To(Equal(0))
			Expect(NewRInteger(0).TruncateWithPrecision(NewRInteger(0)).Value()).To(Equal(0))
		})
	})

	Context("遍历方法", func() {
		It("应该正确执行 Times 方法", func() {
			sum := 0
			NewRInteger(5).Times(func(i RInteger) {
				sum += i.Value()
			})
			Expect(sum).To(Equal(0 + 1 + 2 + 3 + 4))
		})

		It("应该正确执行 UpTo 方法", func() {
			sum := 0
			NewRInteger(1).UpTo(NewRInteger(5), func(i RInteger) {
				sum += i.Value()
			})
			Expect(sum).To(Equal(1 + 2 + 3 + 4 + 5))
		})

		It("应该正确执行 DownTo 方法", func() {
			sum := 0
			NewRInteger(5).DownTo(NewRInteger(1), func(i RInteger) {
				sum += i.Value()
			})
			Expect(sum).To(Equal(5 + 4 + 3 + 2 + 1))
		})

		It("应该正确处理遍历方法的边界情况", func() {
			// Times
			count := 0
			NewRInteger(0).Times(func(i RInteger) {
				count++
			})
			Expect(count).To(Equal(0))

			// UpTo
			count = 0
			NewRInteger(5).UpTo(NewRInteger(4), func(i RInteger) {
				count++
			})
			Expect(count).To(Equal(0))

			// DownTo
			count = 0
			NewRInteger(4).DownTo(NewRInteger(5), func(i RInteger) {
				count++
			})
			Expect(count).To(Equal(0))
		})
	})

	Context("转换方法", func() {
		It("应该正确转换为字符串", func() {
			Expect(positive.ToString()).To(Equal("42"))
			Expect(negative.ToString()).To(Equal("-10"))
			Expect(zero.ToString()).To(Equal("0"))
		})

		It("应该正确转换为 RString", func() {
			Expect(positive.ToRString().ToString()).To(Equal("42"))
		})

		It("应该正确转换为十六进制字符串", func() {
			Expect(NewRInteger(255).ToHex().ToString()).To(Equal("ff"))
			Expect(NewRInteger(16).ToHex().ToString()).To(Equal("10"))
		})

		It("应该正确转换为八进制字符串", func() {
			Expect(NewRInteger(8).ToOct().ToString()).To(Equal("10"))
			Expect(NewRInteger(15).ToOct().ToString()).To(Equal("17"))
		})

		It("应该正确转换为二进制字符串", func() {
			Expect(NewRInteger(5).ToBin().ToString()).To(Equal("101"))
			Expect(NewRInteger(10).ToBin().ToString()).To(Equal("1010"))
		})

		It("应该正确转换为指定进制字符串", func() {
			Expect(NewRInteger(15).ToBase(NewRInteger(16)).ToString()).To(Equal("f"))
			Expect(NewRInteger(15).ToBase(NewRInteger(8)).ToString()).To(Equal("17"))
			Expect(NewRInteger(15).ToBase(NewRInteger(2)).ToString()).To(Equal("1111"))
			Expect(func() { NewRInteger(15).ToBase(NewRInteger(37)) }).To(PanicWith("进制必须在2到36之间"))
		})

		It("应该正确将数字转换为数位数组", func() {
			result := NewRInteger(12345).Digits()
			Expect(result.Length()).To(Equal(5))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(5))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(4))
			Expect(result.Get(2).(RInteger).Value()).To(Equal(3))
			Expect(result.Get(3).(RInteger).Value()).To(Equal(2))
			Expect(result.Get(4).(RInteger).Value()).To(Equal(1))

			// 使用不同进制
			result = NewRInteger(10).Digits(2)
			Expect(result.Length()).To(Equal(4))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(0))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(1))
			Expect(result.Get(2).(RInteger).Value()).To(Equal(0))
			Expect(result.Get(3).(RInteger).Value()).To(Equal(1))
		})

		It("应该正确转换为字符", func() {
			Expect(NewRInteger(65).Chr().ToString()).To(Equal("A"))
			Expect(NewRInteger(97).Chr().ToString()).To(Equal("a"))
		})

		It("应该正确处理转换方法的边界情况", func() {
			Expect(NewRInteger(0).ToString()).To(Equal("0"))
		})
	})

	Context("其他方法", func() {
		It("应该正确返回下一个值", func() {
			Expect(positive.Succ().Value()).To(Equal(43))
			Expect(negative.Succ().Value()).To(Equal(-9))
			Expect(positive.Next().Value()).To(Equal(43))
		})

		It("应该正确返回前一个值", func() {
			Expect(positive.Pred().Value()).To(Equal(41))
			Expect(negative.Pred().Value()).To(Equal(-11))
		})

		It("应该正确返回自身的引用类型", func() {
			Expect(positive.ToInt().Value()).To(Equal(42))
			Expect(positive.ToInteger().Value()).To(Equal(42))
		})

		It("应该正确执行类型强制转换", func() {
			result := positive.Coerce(negative)
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).(RInteger).Value()).To(Equal(-10))
			Expect(result.Get(1).(RInteger).Value()).To(Equal(42))
		})
	})
})

func TestRInteger_Add(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    int
		expected RInteger
	}{
		{"正数相加", NewRInteger(5), 3, NewRInteger(8)},
		{"负数相加", NewRInteger(-5), -3, NewRInteger(-8)},
		{"正负相加", NewRInteger(5), -3, NewRInteger(2)},
		{"零相加", NewRInteger(0), 0, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Add(tt.other)
			if result != tt.expected {
				t.Errorf("Add() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_AddRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    RInteger
		expected RInteger
	}{
		{"正数相加", NewRInteger(5), NewRInteger(3), NewRInteger(8)},
		{"负数相加", NewRInteger(-5), NewRInteger(-3), NewRInteger(-8)},
		{"正负相加", NewRInteger(5), NewRInteger(-3), NewRInteger(2)},
		{"零相加", NewRInteger(0), NewRInteger(0), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.AddRInt(tt.other)
			if result != tt.expected {
				t.Errorf("AddRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_Sub(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    int
		expected RInteger
	}{
		{"正数相减", NewRInteger(5), 3, NewRInteger(2)},
		{"负数相减", NewRInteger(-5), -3, NewRInteger(-2)},
		{"正负相减", NewRInteger(5), -3, NewRInteger(8)},
		{"零相减", NewRInteger(0), 0, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Sub(tt.other)
			if result != tt.expected {
				t.Errorf("Sub() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_SubRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    RInteger
		expected RInteger
	}{
		{"正数相减", NewRInteger(5), NewRInteger(3), NewRInteger(2)},
		{"负数相减", NewRInteger(-5), NewRInteger(-3), NewRInteger(-2)},
		{"正负相减", NewRInteger(5), NewRInteger(-3), NewRInteger(8)},
		{"零相减", NewRInteger(0), NewRInteger(0), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.SubRInt(tt.other)
			if result != tt.expected {
				t.Errorf("SubRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_Mul(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    int
		expected RInteger
	}{
		{"正数相乘", NewRInteger(5), 3, NewRInteger(15)},
		{"负数相乘", NewRInteger(-5), -3, NewRInteger(15)},
		{"正负相乘", NewRInteger(5), -3, NewRInteger(-15)},
		{"零相乘", NewRInteger(0), 0, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Mul(tt.other)
			if result != tt.expected {
				t.Errorf("Mul() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_MulRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    RInteger
		expected RInteger
	}{
		{"正数相乘", NewRInteger(5), NewRInteger(3), NewRInteger(15)},
		{"负数相乘", NewRInteger(-5), NewRInteger(-3), NewRInteger(15)},
		{"正负相乘", NewRInteger(5), NewRInteger(-3), NewRInteger(-15)},
		{"零相乘", NewRInteger(0), NewRInteger(0), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.MulRInt(tt.other)
			if result != tt.expected {
				t.Errorf("MulRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_Div(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    int
		expected RInteger
	}{
		{"正数相除", NewRInteger(6), 3, NewRInteger(2)},
		{"负数相除", NewRInteger(-6), -3, NewRInteger(2)},
		{"正负相除", NewRInteger(6), -3, NewRInteger(-2)},
		{"零除以非零", NewRInteger(0), 5, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Div(tt.other)
			if result != tt.expected {
				t.Errorf("Div() = %v, 期望 %v", result, tt.expected)
			}
		})
	}

	// 测试除以零的情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("Div() 除以零时没有触发 panic")
		}
	}()
	NewRInteger(5).Div(0)
}

func TestRInteger_DivRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    RInteger
		expected RInteger
	}{
		{"正数相除", NewRInteger(6), NewRInteger(3), NewRInteger(2)},
		{"负数相除", NewRInteger(-6), NewRInteger(-3), NewRInteger(2)},
		{"正负相除", NewRInteger(6), NewRInteger(-3), NewRInteger(-2)},
		{"零除以非零", NewRInteger(0), NewRInteger(5), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.DivRInt(tt.other)
			if result != tt.expected {
				t.Errorf("DivRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}

	// 测试除以零的情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("DivRInt() 除以零时没有触发 panic")
		}
	}()
	NewRInteger(5).DivRInt(NewRInteger(0))
}

func TestRInteger_Mod(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    int
		expected RInteger
	}{
		{"正数取模", NewRInteger(7), 3, NewRInteger(1)},
		{"负数取模", NewRInteger(-7), -3, NewRInteger(-1)},
		{"正负取模", NewRInteger(7), -3, NewRInteger(1)},
		{"零取模", NewRInteger(0), 5, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Mod(tt.other)
			if result != tt.expected {
				t.Errorf("Mod() = %v, 期望 %v", result, tt.expected)
			}
		})
	}

	// 测试除以零的情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("Mod() 除以零时没有触发 panic")
		}
	}()
	NewRInteger(5).Mod(0)
}

func TestRInteger_ModRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		other    RInteger
		expected RInteger
	}{
		{"正数取模", NewRInteger(7), NewRInteger(3), NewRInteger(1)},
		{"负数取模", NewRInteger(-7), NewRInteger(-3), NewRInteger(-1)},
		{"正负取模", NewRInteger(7), NewRInteger(-3), NewRInteger(1)},
		{"零取模", NewRInteger(0), NewRInteger(5), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.ModRInt(tt.other)
			if result != tt.expected {
				t.Errorf("ModRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}

	// 测试除以零的情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("ModRInt() 除以零时没有触发 panic")
		}
	}()
	NewRInteger(5).ModRInt(NewRInteger(0))
}

func TestRInteger_Pow(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		exponent int
		expected RInteger
	}{
		{"正数次幂", NewRInteger(2), 3, NewRInteger(8)},
		{"零次幂", NewRInteger(5), 0, NewRInteger(1)},
		{"负次幂", NewRInteger(2), -1, NewRInteger(0)},
		{"零的次幂", NewRInteger(0), 5, NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.Pow(tt.exponent)
			if result != tt.expected {
				t.Errorf("Pow() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}

func TestRInteger_PowRInt(t *testing.T) {
	tests := []struct {
		name     string
		i        RInteger
		exponent RInteger
		expected RInteger
	}{
		{"正数次幂", NewRInteger(2), NewRInteger(3), NewRInteger(8)},
		{"零次幂", NewRInteger(5), NewRInteger(0), NewRInteger(1)},
		{"负次幂", NewRInteger(2), NewRInteger(-1), NewRInteger(0)},
		{"零的次幂", NewRInteger(0), NewRInteger(5), NewRInteger(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.i.PowRInt(tt.exponent)
			if result != tt.expected {
				t.Errorf("PowRInt() = %v, 期望 %v", result, tt.expected)
			}
		})
	}
}
