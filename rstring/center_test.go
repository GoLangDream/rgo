package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("rstring.Center", func() {

	When("当有需要把一个长度是5的字符串放在中间的时候", func() {
		var str = "12345"
		Context("如果需要放的宽度小于等于5", func() {
			It("无论填充是什么，都返回字符串本身", func() {
				Expect(Center(str, 3, " ")).To(
					Equal(str))
				Expect(Center(str, 4, "123")).To(
					Equal(str))
				Expect(Center(str, 5, "abc")).To(
					Equal(str))
			})
		})

		Context("如果需要放的宽度大于5", func() {
			It("会使用填充，填充两端并把字符串放在中间", func() {
				Expect(CenterWithSpacePad(str, 6)).To(
					Equal("12345 "))
				Expect(Center(str, 6, " ")).To(
					Equal("12345 "))
				Expect(Center(str, 7, "ab")).To(
					Equal("a12345a"))
				Expect(Center(str, 8, "ab")).To(
					Equal("a12345ab"))
				Expect(Center(str, 9, "ab")).To(
					Equal("ab12345ab"))
			})
		})
	})
})
