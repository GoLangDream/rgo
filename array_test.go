package goby_test

import (
	. "github.com/GoLangDream/rgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RArray", func() {
	var (
		emptyArr RArray
		strArr   RArray
		mixedArr RArray
	)

	BeforeEach(func() {
		emptyArr = NewRArray([]Object{})
		strArr = NewRArray([]Object{
			NewRString("a"),
			NewRString("b"),
			NewRString("c"),
		})
		mixedArr = NewRArray([]Object{
			NewRString("hello"),
			NewRInteger(42),
			NewRString("world"),
		})
	})

	Context("基本属性和访问", func() {
		It("应该正确返回数组长度", func() {
			Expect(emptyArr.Length()).To(Equal(0))
			Expect(strArr.Length()).To(Equal(3))
			Expect(mixedArr.Length()).To(Equal(3))
		})

		It("Size 方法应该与 Length 返回相同结果", func() {
			Expect(strArr.Size()).To(Equal(strArr.Length()))
		})

		It("应该正确检测空数组", func() {
			Expect(emptyArr.Empty()).To(BeTrue())
			Expect(strArr.Empty()).To(BeFalse())
		})

		It("应该正确获取第一个和最后一个元素", func() {
			Expect(strArr.First().ToString()).To(Equal("a"))
			Expect(strArr.Last().ToString()).To(Equal("c"))
			Expect(mixedArr.First().ToString()).To(Equal("hello"))
			Expect(mixedArr.Last().ToString()).To(Equal("world"))
			Expect(emptyArr.First()).To(BeNil())
			Expect(emptyArr.Last()).To(BeNil())
		})

		It("应该正确通过索引获取元素", func() {
			Expect(strArr.Get(0).ToString()).To(Equal("a"))
			Expect(strArr.Get(1).ToString()).To(Equal("b"))
			Expect(strArr.Get(2).ToString()).To(Equal("c"))
			Expect(strArr.Get(-1).ToString()).To(Equal("c"))
			Expect(strArr.Get(-2).ToString()).To(Equal("b"))
			Expect(strArr.Get(10)).To(BeNil())
			Expect(strArr.Get(-10)).To(BeNil())
		})
	})

	Context("修改数组", func() {
		It("应该正确添加和移除元素", func() {
			arr := NewRArray([]Object{NewRString("a")})
			arr.Push(NewRString("b"))
			Expect(arr.Length()).To(Equal(2))
			Expect(arr.Last().ToString()).To(Equal("b"))

			popped := arr.Pop()
			Expect(popped.ToString()).To(Equal("b"))
			Expect(arr.Length()).To(Equal(1))
			Expect(arr.Last().ToString()).To(Equal("a"))

			arr.Push(NewRInteger(42))
			Expect(arr.Length()).To(Equal(2))
			Expect(arr.Last().ToString()).To(Equal("42"))
		})
	})

	Context("数组转换", func() {
		It("应该正确连接数组元素", func() {
			Expect(strArr.Join(",").ToString()).To(Equal("a,b,c"))
			Expect(mixedArr.Join(" ").ToString()).To(Equal("hello 42 world"))
			Expect(emptyArr.Join(",").ToString()).To(Equal(""))
		})

		It("应该正确反转数组", func() {
			reversed := strArr.Reverse()
			Expect(reversed.Length()).To(Equal(3))
			Expect(reversed.Get(0).ToString()).To(Equal("c"))
			Expect(reversed.Get(1).ToString()).To(Equal("b"))
			Expect(reversed.Get(2).ToString()).To(Equal("a"))
		})

		It("应该正确去重数组元素", func() {
			dupArr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("a"),
				NewRString("c"),
				NewRString("b"),
			})
			uniq := dupArr.Uniq()
			Expect(uniq.Length()).To(Equal(3))
			Expect(uniq.Get(0).ToString()).To(Equal("a"))
			Expect(uniq.Get(1).ToString()).To(Equal("b"))
			Expect(uniq.Get(2).ToString()).To(Equal("c"))
		})
	})

	Context("数组变换", func() {
		It("应该正确执行 Map 操作", func() {
			result := strArr.Map(func(obj Object) Object {
				if str, ok := obj.(RString); ok {
					return NewRString(str.ToString() + str.ToString())
				}
				return obj
			})
			Expect(result.Length()).To(Equal(3))
			Expect(result.Get(0).ToString()).To(Equal("aa"))
			Expect(result.Get(1).ToString()).To(Equal("bb"))
			Expect(result.Get(2).ToString()).To(Equal("cc"))
		})

		It("应该正确执行 Select 操作", func() {
			result := mixedArr.Select(func(obj Object) bool {
				_, isString := obj.(RString)
				return isString
			})
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).ToString()).To(Equal("hello"))
			Expect(result.Get(1).ToString()).To(Equal("world"))
		})

		It("应该正确执行 Reject 操作", func() {
			result := mixedArr.Reject(func(obj Object) bool {
				_, isString := obj.(RString)
				return isString
			})
			Expect(result.Length()).To(Equal(1))
			Expect(result.Get(0).ToString()).To(Equal("42"))
		})
	})

	Context("检测包含", func() {
		It("应该正确检测元素是否存在", func() {
			Expect(strArr.Include(NewRString("a"))).To(BeTrue())
			Expect(strArr.Include(NewRString("z"))).To(BeFalse())
			Expect(mixedArr.Include(NewRInteger(42))).To(BeTrue())
			Expect(mixedArr.Include(NewRInteger(100))).To(BeFalse())
		})
	})
})
