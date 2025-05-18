package goby_test

import (
	"fmt"

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

		It("应该正确处理空数组的转换", func() {
			Expect(emptyArr.Join(",").ToString()).To(Equal(""))
			Expect(emptyArr.Reverse().Length()).To(Equal(0))
			Expect(emptyArr.Uniq().Length()).To(Equal(0))
		})

		It("应该正确处理单元素数组的转换", func() {
			singleArr := NewRArray([]Object{NewRString("a")})
			Expect(singleArr.Join(",").ToString()).To(Equal("a"))
			Expect(singleArr.Reverse().Get(0).ToString()).To(Equal("a"))
			Expect(singleArr.Uniq().Get(0).ToString()).To(Equal("a"))
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

		It("应该正确处理空数组的变换", func() {
			result := emptyArr.Map(func(obj Object) Object {
				return obj
			})
			Expect(result.Length()).To(Equal(0))

			result = emptyArr.Select(func(obj Object) bool {
				return true
			})
			Expect(result.Length()).To(Equal(0))

			result = emptyArr.Reject(func(obj Object) bool {
				return true
			})
			Expect(result.Length()).To(Equal(0))
		})

		It("应该正确处理所有元素都被过滤的情况", func() {
			result := strArr.Select(func(obj Object) bool {
				return false
			})
			Expect(result.Length()).To(Equal(0))

			result = strArr.Reject(func(obj Object) bool {
				return true
			})
			Expect(result.Length()).To(Equal(0))
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

	Context("数组操作", func() {
		It("应该正确移除nil元素", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				nil,
				NewRString("b"),
				nil,
				NewRString("c"),
			})
			compacted := arr.Compact()
			Expect(compacted.Length()).To(Equal(3))
			Expect(compacted.Get(0).ToString()).To(Equal("a"))
			Expect(compacted.Get(1).ToString()).To(Equal("b"))
			Expect(compacted.Get(2).ToString()).To(Equal("c"))
		})

		It("应该正确处理空数组的压缩", func() {
			Expect(emptyArr.Compact().Length()).To(Equal(0))
		})

		It("应该正确展平嵌套数组", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRArray([]Object{
					NewRString("b"),
					NewRString("c"),
				}),
				NewRString("d"),
			})
			flattened := arr.Flatten()
			Expect(flattened.Length()).To(Equal(4))
			Expect(flattened.Get(0).ToString()).To(Equal("a"))
			Expect(flattened.Get(1).ToString()).To(Equal("b"))
			Expect(flattened.Get(2).ToString()).To(Equal("c"))
			Expect(flattened.Get(3).ToString()).To(Equal("d"))
		})

		It("应该正确处理多层嵌套数组的展平", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRArray([]Object{
					NewRString("b"),
					NewRArray([]Object{
						NewRString("c"),
						NewRString("d"),
					}),
				}),
				NewRString("e"),
			})
			flattened := arr.Flatten()
			Expect(flattened.Length()).To(Equal(5))
			Expect(flattened.Get(0).ToString()).To(Equal("a"))
			Expect(flattened.Get(1).ToString()).To(Equal("b"))
			Expect(flattened.Get(2).ToString()).To(Equal("c"))
			Expect(flattened.Get(3).ToString()).To(Equal("d"))
			Expect(flattened.Get(4).ToString()).To(Equal("e"))
		})
	})

	Context("数组查询", func() {
		It("应该正确查找元素位置", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("a"),
				NewRString("c"),
			})
			Expect(arr.Index(NewRString("a"))).To(Equal(0))
			Expect(arr.Index(NewRString("b"))).To(Equal(1))
			Expect(arr.Index(NewRString("d"))).To(Equal(-1))
			Expect(arr.RIndex(NewRString("a"))).To(Equal(2))
			Expect(arr.RIndex(NewRString("b"))).To(Equal(1))
			Expect(arr.RIndex(NewRString("d"))).To(Equal(-1))
		})

		It("应该正确计算元素出现次数", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("a"),
				NewRString("c"),
				NewRString("a"),
			})
			Expect(arr.Count(NewRString("a"))).To(Equal(3))
			Expect(arr.Count(NewRString("b"))).To(Equal(1))
			Expect(arr.Count(NewRString("d"))).To(Equal(0))
		})

		It("应该正确检查元素条件", func() {
			arr := NewRArray([]Object{
				NewRInteger(1),
				NewRInteger(2),
				NewRInteger(3),
				NewRInteger(4),
			})
			Expect(arr.Any(func(obj Object) bool {
				return obj.(RInteger).Value() > 3
			})).To(BeTrue())
			Expect(arr.All(func(obj Object) bool {
				return obj.(RInteger).Value() > 0
			})).To(BeTrue())
			Expect(arr.None(func(obj Object) bool {
				return obj.(RInteger).Value() > 5
			})).To(BeTrue())
		})

		It("应该正确处理空数组的查询", func() {
			Expect(emptyArr.Index(NewRString("a"))).To(Equal(-1))
			Expect(emptyArr.RIndex(NewRString("a"))).To(Equal(-1))
			Expect(emptyArr.Count(NewRString("a"))).To(Equal(0))
			Expect(emptyArr.Any(func(obj Object) bool { return true })).To(BeFalse())
			Expect(emptyArr.All(func(obj Object) bool { return true })).To(BeTrue())
			Expect(emptyArr.None(func(obj Object) bool { return true })).To(BeTrue())
		})
	})

	Context("数组切片", func() {
		It("应该正确返回子数组", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("c"),
				NewRString("d"),
				NewRString("e"),
			})
			Expect(arr.Slice(1, 3).Length()).To(Equal(2))
			Expect(arr.Slice(1, 3).Get(0).ToString()).To(Equal("b"))
			Expect(arr.Slice(1, 3).Get(1).ToString()).To(Equal("c"))
			Expect(arr.Slice(-3, -1).Length()).To(Equal(2))
			Expect(arr.Slice(-3, -1).Get(0).ToString()).To(Equal("c"))
			Expect(arr.Slice(-3, -1).Get(1).ToString()).To(Equal("d"))
			Expect(arr.Slice(10, 20).Length()).To(Equal(0))
			Expect(arr.Slice(-20, -10).Length()).To(Equal(0))
		})

		It("应该正确处理空数组的切片", func() {
			Expect(emptyArr.Slice(0, 1).Length()).To(Equal(0))
			Expect(emptyArr.Slice(-1, 1).Length()).To(Equal(0))
		})
	})

	Context("数组分组", func() {
		It("应该正确按条件分组", func() {
			arr := NewRArray([]Object{
				NewRInteger(1),
				NewRInteger(2),
				NewRInteger(3),
				NewRInteger(4),
				NewRInteger(5),
			})
			groups := arr.GroupBy(func(obj Object) Object {
				if obj.(RInteger).Value()%2 == 0 {
					return NewRString("even")
				}
				return NewRString("odd")
			})
			Expect(groups["even"].Length()).To(Equal(2))
			Expect(groups["odd"].Length()).To(Equal(3))
		})

		It("应该正确分区", func() {
			arr := NewRArray([]Object{
				NewRInteger(1),
				NewRInteger(2),
				NewRInteger(3),
				NewRInteger(4),
				NewRInteger(5),
			})
			even, odd := arr.Partition(func(obj Object) bool {
				return obj.(RInteger).Value()%2 == 0
			})
			Expect(even.Length()).To(Equal(2))
			Expect(odd.Length()).To(Equal(3))
		})
	})

	Context("数组迭代", func() {
		It("应该正确执行Each操作", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("c"),
			})
			var result []string
			arr.Each(func(obj Object) {
				result = append(result, obj.ToString())
			})
			Expect(result).To(Equal([]string{"a", "b", "c"}))
		})

		It("应该正确执行EachWithIndex操作", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("c"),
			})
			var result []string
			arr.EachWithIndex(func(obj Object, index int) {
				result = append(result, fmt.Sprintf("%s:%d", obj.ToString(), index))
			})
			Expect(result).To(Equal([]string{"a:0", "b:1", "c:2"}))
		})

		It("应该正确执行EachCons操作", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("c"),
				NewRString("d"),
			})
			var result []string
			arr.EachCons(2, func(subArr RArray) {
				result = append(result, subArr.Join("").ToString())
			})
			Expect(result).To(Equal([]string{"ab", "bc", "cd"}))
		})

		It("应该正确执行EachSlice操作", func() {
			arr := NewRArray([]Object{
				NewRString("a"),
				NewRString("b"),
				NewRString("c"),
				NewRString("d"),
			})
			var result []string
			arr.EachSlice(2, func(subArr RArray) {
				result = append(result, subArr.Join("").ToString())
			})
			Expect(result).To(Equal([]string{"ab", "cd"}))
		})
	})

	Context("数组比较", func() {
		It("应该正确比较数组相等性", func() {
			arr1 := NewRArray([]Object{NewRString("a"), NewRString("b")})
			arr2 := NewRArray([]Object{NewRString("a"), NewRString("b")})
			arr3 := NewRArray([]Object{NewRString("a"), NewRString("c")})
			Expect(arr1.Equal(arr2)).To(BeTrue())
			Expect(arr1.Equal(arr3)).To(BeFalse())
			Expect(arr1.Equal(emptyArr)).To(BeFalse())
		})
	})
})
