package goby_test

import (
	. "github.com/GoLangDream/rgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RString", func() {
	var (
		emptyStr RString
		str      RString
		multiStr RString
	)

	BeforeEach(func() {
		emptyStr = NewRString("")
		str = NewRString("hello world")
		multiStr = NewRString("hello\nworld")
	})

	Context("长度相关方法", func() {
		It("应该返回正确的字符串长度", func() {
			Expect(emptyStr.Length()).To(Equal(0))
			Expect(str.Length()).To(Equal(11))
			Expect(multiStr.Length()).To(Equal(11))
		})

		It("Size 方法应该与 Length 返回相同结果", func() {
			Expect(str.Size()).To(Equal(str.Length()))
		})

		It("应该正确检测空字符串", func() {
			Expect(emptyStr.Empty()).To(BeTrue())
			Expect(str.Empty()).To(BeFalse())
		})
	})

	Context("变换方法", func() {
		It("应该正确大写首字母", func() {
			Expect(NewRString("hello").Capitalize().ToString()).To(Equal("Hello"))
			Expect(NewRString("Hello").Capitalize().ToString()).To(Equal("Hello"))
			Expect(emptyStr.Capitalize().ToString()).To(Equal(""))
		})

		It("应该正确转换大小写", func() {
			Expect(str.Upcase().ToString()).To(Equal("HELLO WORLD"))
			Expect(str.Downcase().ToString()).To(Equal("hello world"))
		})

		It("应该正确去除空白", func() {
			whiteStr := NewRString("  hello world  ")
			Expect(whiteStr.Strip().ToString()).To(Equal("hello world"))
		})

		It("应该正确去除换行符", func() {
			crlfStr := NewRString("hello\r\n")
			Expect(crlfStr.Chomp().ToString()).To(Equal("hello"))
		})

		It("应该正确反转字符串", func() {
			Expect(str.Reverse().ToString()).To(Equal("dlrow olleh"))
			Expect(emptyStr.Reverse().ToString()).To(Equal(""))
		})
	})

	Context("查找和替换", func() {
		It("应该正确检测子串", func() {
			Expect(str.Include("hello")).To(BeTrue())
			Expect(str.Include("goodbye")).To(BeFalse())
		})

		It("应该正确检测前缀和后缀", func() {
			Expect(str.StartsWith("hello")).To(BeTrue())
			Expect(str.StartsWith("world")).To(BeFalse())
			Expect(str.EndsWith("world")).To(BeTrue())
			Expect(str.EndsWith("hello")).To(BeFalse())
		})

		It("应该正确替换字符串", func() {
			Expect(str.ReplaceAll("hello", "hi").ToString()).To(Equal("hi world"))
			Expect(str.ReplaceAll("nonexistent", "hi").ToString()).To(Equal("hello world"))
		})

		It("应该正确使用正则表达式", func() {
			Expect(str.Match(`hello.*`)).To(BeTrue())
			Expect(str.Match(`^world`)).To(BeFalse())
			Expect(str.Gsub(`hello`, "hi").ToString()).To(Equal("hi world"))
			Expect(str.Gsub(`\w+`, "word").ToString()).To(Equal("word word"))
		})
	})

	Context("分割字符串", func() {
		It("应该正确分割字符串", func() {
			result := str.Split(" ")
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).ToString()).To(Equal("hello"))
			Expect(result.Get(1).ToString()).To(Equal("world"))
		})
	})
})
