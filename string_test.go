package goby_test

import (
	. "github.com/GoLangDream/goby"
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
})
