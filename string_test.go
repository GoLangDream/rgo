package goby_test

import (
	. "github.com/GoLangDream/rgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("String", func() {
	When("内容都是ascii的时候", func() {
		It("返回true", func() {
			var str = NewString("abcdeadf")
			Expect(str.AsciiOnly()).To(BeTrue())
		})
	})

	When("内容包含特殊符号的时候", func() {
		It("返回false", func() {
			var str = NewString("abc你好啊!efs")
			Expect(str.AsciiOnly()).To(BeFalse())
		})
	})

	When("第一个字母是小写，其他字母有大小写的情况下", func() {
		It("能把第一个字母大写，其他的全部小写", func() {
			var str = NewString("abc Test WORD")
			Expect(str.Capitalize()).To(
				Equal(NewString("Abc test word")))
		})
	})

	When("第一个字母是大写，其他字母有大小写的情况下", func() {
		It("能把第一个字母大写，其他的全部小写", func() {
			var str = NewString("ABc Test WORD")
			Expect(str.Capitalize()).To(
				Equal(NewString("Abc test word")))
		})
	})

	It("能把字符串的字母都变成小写", func() {
		var str = NewString("ABc Test WORD")
		Expect(str.Downcase()).To(
			Equal(NewString("abc test word")))
	})

	When("当有需要把一个长度是5的字符串放在中间的时候", func() {
		var str = NewString("12345")
		Context("如果需要放的宽度小于等于5", func() {
			It("无论填充是什么，都返回字符串本身", func() {
				Expect(str.Center(3, " ")).To(
					Equal(str))
				Expect(str.Center(4, "123")).To(
					Equal(str))
				Expect(str.Center(5, "abc")).To(
					Equal(str))
			})
		})

		Context("如果需要放的宽度大于5", func() {
			It("会使用填充，填充两端并把字符串放在中间", func() {
				Expect(str.CenterWithSpacePad(6)).To(
					Equal(NewString("12345 ")))
				Expect(str.Center(6, " ")).To(
					Equal(NewString("12345 ")))
				Expect(str.Center(7, "ab")).To(
					Equal(NewString("a12345a")))
				Expect(str.Center(8, "ab")).To(
					Equal(NewString("a12345ab")))
				Expect(str.Center(9, "ab")).To(
					Equal(NewString("ab12345ab")))
			})
		})
	})
})
