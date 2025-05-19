package rgo_test

import (
	. "github.com/GoLangDream/rgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Object", func() {
	var (
		str RString
		num RInteger
		arr RArray
	)

	BeforeEach(func() {
		str = NewRString("hello")
		num = NewRInteger(123)
		arr = NewRArray([]Object{
			NewRString("a"),
			NewRInteger(1),
		})
	})

	Context("基础对象方法", func() {
		It("应该返回正确的类名", func() {
			Expect(str.Class()).To(Equal("String"))
			Expect(num.Class()).To(Equal("Integer"))
			Expect(arr.Class()).To(Equal("Array"))
		})

		It("应该正确检查对象类型", func() {
			Expect(str.IsA("String")).To(BeTrue())
			Expect(str.IsA("Integer")).To(BeFalse())
			Expect(num.IsA("Integer")).To(BeTrue())
			Expect(arr.IsA("Array")).To(BeTrue())
		})

		It("应该正确返回字符串表示", func() {
			Expect(str.ToString()).To(Equal("hello"))
			Expect(num.ToString()).To(Equal("123"))
			Expect(arr.ToString()).To(Equal("[a, 1]"))
		})

		It("应该正确比较相等性", func() {
			Expect(str.Equal(NewRString("hello"))).To(BeTrue())
			Expect(str.Equal(NewRString("world"))).To(BeFalse())
			Expect(num.Equal(NewRInteger(123))).To(BeTrue())
			Expect(num.Equal(NewRInteger(456))).To(BeFalse())

			sameArr := NewRArray([]Object{
				NewRString("a"),
				NewRInteger(1),
			})
			Expect(arr.Equal(sameArr)).To(BeTrue())

			diffArr := NewRArray([]Object{
				NewRString("b"),
				NewRInteger(2),
			})
			Expect(arr.Equal(diffArr)).To(BeFalse())
		})
	})
})
