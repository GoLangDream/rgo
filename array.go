package goby

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// RArray 实现类似 Ruby 中 Array 类的功能
type RArray struct {
	BaseObject
	elements []Object
}

// NewRArray 创建一个新的 RArray 对象
func NewRArray(elements []Object) RArray {
	return RArray{
		BaseObject: NewBaseObject("Array"),
		elements:   elements,
	}
}

// ToString 返回数组的字符串表示
func (a RArray) ToString() string {
	strs := make([]string, len(a.elements))
	for i, elem := range a.elements {
		strs[i] = elem.ToString()
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}

// Equal 比较两个对象是否相等
func (a RArray) Equal(other Object) bool {
	otherArr, ok := other.(RArray)
	if !ok || len(a.elements) != len(otherArr.elements) {
		return false
	}

	for i, elem := range a.elements {
		if !elem.Equal(otherArr.elements[i]) {
			return false
		}
	}
	return true
}

// Length 返回数组长度
func (a RArray) Length() int {
	return len(a.elements)
}

// Size 是 Length 的别名
func (a RArray) Size() int {
	return a.Length()
}

// Empty 检查数组是否为空
func (a RArray) Empty() bool {
	return len(a.elements) == 0
}

// First 返回数组的第一个元素
func (a RArray) First() Object {
	if a.Empty() {
		return nil
	}
	return a.elements[0]
}

// Last 返回数组的最后一个元素
func (a RArray) Last() Object {
	if a.Empty() {
		return nil
	}
	return a.elements[len(a.elements)-1]
}

// Include 检查数组是否包含指定元素
func (a RArray) Include(obj Object) bool {
	for _, elem := range a.elements {
		if elem.Equal(obj) {
			return true
		}
	}
	return false
}

// Push 将元素添加到数组末尾
func (a *RArray) Push(obj Object) *RArray {
	a.elements = append(a.elements, obj)
	return a
}

// Pop 移除并返回数组的最后一个元素
func (a *RArray) Pop() Object {
	if a.Empty() {
		return nil
	}

	lastIndex := len(a.elements) - 1
	lastElement := a.elements[lastIndex]
	a.elements = a.elements[:lastIndex]
	return lastElement
}

// Join 使用指定分隔符连接数组元素
func (a RArray) Join(sep string) RString {
	strs := make([]string, len(a.elements))
	for i, elem := range a.elements {
		strs[i] = elem.ToString()
	}
	return NewRString(strings.Join(strs, sep))
}

// Map 对数组每个元素应用函数并返回新数组
func (a RArray) Map(fn func(Object) Object) RArray {
	result := make([]Object, len(a.elements))
	for i, elem := range a.elements {
		result[i] = fn(elem)
	}
	return NewRArray(result)
}

// Select 返回满足条件的元素
func (a RArray) Select(fn func(Object) bool) RArray {
	var result []Object
	for _, elem := range a.elements {
		if fn(elem) {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Reject 返回不满足条件的元素
func (a RArray) Reject(fn func(Object) bool) RArray {
	var result []Object
	for _, elem := range a.elements {
		if !fn(elem) {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Reverse 反转数组
func (a RArray) Reverse() RArray {
	reversed := make([]Object, len(a.elements))
	for i, j := 0, len(a.elements)-1; j >= 0; i, j = i+1, j-1 {
		reversed[i] = a.elements[j]
	}
	return NewRArray(reversed)
}

// Shuffle 随机打乱数组元素顺序
func (a RArray) Shuffle() RArray {
	shuffled := make([]Object, len(a.elements))
	copy(shuffled, a.elements)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return NewRArray(shuffled)
}

// Sort 对数组元素进行排序（按字符串表示）
func (a RArray) Sort() RArray {
	sorted := make([]Object, len(a.elements))
	copy(sorted, a.elements)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ToString() < sorted[j].ToString()
	})

	return NewRArray(sorted)
}

// Uniq 返回去重后的数组
func (a RArray) Uniq() RArray {
	seen := make(map[string]bool)
	var result []Object

	for _, elem := range a.elements {
		str := elem.ToString()
		if !seen[str] {
			seen[str] = true
			result = append(result, elem)
		}
	}

	return NewRArray(result)
}

// Get 获取指定索引的元素
func (a RArray) Get(index int) Object {
	if index < 0 {
		index = len(a.elements) + index
	}

	if index < 0 || index >= len(a.elements) {
		return nil
	}

	return a.elements[index]
}

// ToArray 返回底层数组
func (a RArray) ToArray() []Object {
	return a.elements
}
