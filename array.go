// Package goby 提供了一个类似 Ruby 的数组实现
package goby

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// RArray 实现了类似 Ruby 的数组功能
type RArray struct {
	BaseObject
	elements []Object
}

// NewRArray 创建一个新的数组对象
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

// Equal 比较两个数组是否相等
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

// Length 返回数组的长度
func (a RArray) Length() int {
	return len(a.elements)
}

// Size 返回数组的长度（Length 的别名）
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

// Map 对数组中的每个元素应用函数并返回新数组
func (a RArray) Map(fn func(Object) Object) RArray {
	result := make([]Object, len(a.elements))
	for i, elem := range a.elements {
		result[i] = fn(elem)
	}
	return NewRArray(result)
}

// Select 返回满足条件的所有元素组成的新数组
func (a RArray) Select(fn func(Object) bool) RArray {
	var result []Object
	for _, elem := range a.elements {
		if fn(elem) {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Reject 返回不满足条件的所有元素组成的新数组
func (a RArray) Reject(fn func(Object) bool) RArray {
	var result []Object
	for _, elem := range a.elements {
		if !fn(elem) {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Reverse 返回数组的反转副本
func (a RArray) Reverse() RArray {
	reversed := make([]Object, len(a.elements))
	for i, j := 0, len(a.elements)-1; j >= 0; i, j = i+1, j-1 {
		reversed[i] = a.elements[j]
	}
	return NewRArray(reversed)
}

// Shuffle 返回数组的随机打乱副本
func (a RArray) Shuffle() RArray {
	shuffled := make([]Object, len(a.elements))
	copy(shuffled, a.elements)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return NewRArray(shuffled)
}

// Sort 返回数组的排序副本（按字符串表示排序）
func (a RArray) Sort() RArray {
	sorted := make([]Object, len(a.elements))
	copy(sorted, a.elements)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ToString() < sorted[j].ToString()
	})

	return NewRArray(sorted)
}

// Uniq 返回数组的去重副本
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

// Compact 返回移除所有 nil 元素后的新数组
func (a RArray) Compact() RArray {
	var result []Object
	for _, elem := range a.elements {
		if elem != nil {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Flatten 返回展平嵌套数组后的新数组
func (a RArray) Flatten() RArray {
	var result []Object
	for _, elem := range a.elements {
		if arr, ok := elem.(RArray); ok {
			result = append(result, arr.Flatten().elements...)
		} else {
			result = append(result, elem)
		}
	}
	return NewRArray(result)
}

// Index 返回元素首次出现的位置，不存在则返回-1
func (a RArray) Index(obj Object) int {
	for i, elem := range a.elements {
		if elem.Equal(obj) {
			return i
		}
	}
	return -1
}

// RIndex 返回元素最后出现的位置，不存在则返回-1
func (a RArray) RIndex(obj Object) int {
	for i := len(a.elements) - 1; i >= 0; i-- {
		if a.elements[i].Equal(obj) {
			return i
		}
	}
	return -1
}

// Count 计算元素在数组中出现的次数
func (a RArray) Count(obj Object) int {
	count := 0
	for _, elem := range a.elements {
		if elem.Equal(obj) {
			count++
		}
	}
	return count
}

// Any 检查是否有元素满足条件
func (a RArray) Any(fn func(Object) bool) bool {
	for _, elem := range a.elements {
		if fn(elem) {
			return true
		}
	}
	return false
}

// All 检查是否所有元素都满足条件
func (a RArray) All(fn func(Object) bool) bool {
	for _, elem := range a.elements {
		if !fn(elem) {
			return false
		}
	}
	return true
}

// None 检查是否没有元素满足条件
func (a RArray) None(fn func(Object) bool) bool {
	return !a.Any(fn)
}

// Slice 返回指定范围的子数组
func (a RArray) Slice(start, end int) RArray {
	if start < 0 {
		start = len(a.elements) + start
	}
	if end < 0 {
		end = len(a.elements) + end
	}
	if start < 0 {
		start = 0
	}
	if end > len(a.elements) {
		end = len(a.elements)
	}
	if start >= end {
		return NewRArray([]Object{})
	}
	return NewRArray(a.elements[start:end])
}

// SliceFrom 返回从指定位置到结尾的子数组
func (a RArray) SliceFrom(start int) RArray {
	return a.Slice(start, len(a.elements))
}

// Take 返回前n个元素组成的新数组
func (a RArray) Take(n int) RArray {
	if n <= 0 {
		return NewRArray([]Object{})
	}
	if n > len(a.elements) {
		n = len(a.elements)
	}
	return NewRArray(a.elements[:n])
}

// Drop 返回除前n个元素外的所有元素组成的新数组
func (a RArray) Drop(n int) RArray {
	if n <= 0 {
		return a
	}
	if n >= len(a.elements) {
		return NewRArray([]Object{})
	}
	return NewRArray(a.elements[n:])
}

// GroupBy 按指定条件对数组元素进行分组
func (a RArray) GroupBy(fn func(Object) Object) map[string]RArray {
	groups := make(map[string]RArray)
	for _, elem := range a.elements {
		key := fn(elem).ToString()
		if group, exists := groups[key]; exists {
			groups[key] = NewRArray(append(group.elements, elem))
		} else {
			groups[key] = NewRArray([]Object{elem})
		}
	}
	return groups
}

// Partition 将数组分为满足条件和不满足条件的两部分
func (a RArray) Partition(fn func(Object) bool) (RArray, RArray) {
	var truePart, falsePart []Object
	for _, elem := range a.elements {
		if fn(elem) {
			truePart = append(truePart, elem)
		} else {
			falsePart = append(falsePart, elem)
		}
	}
	return NewRArray(truePart), NewRArray(falsePart)
}

// Each 对数组中的每个元素执行操作
func (a RArray) Each(fn func(Object)) {
	for _, elem := range a.elements {
		fn(elem)
	}
}

// EachWithIndex 对数组中的每个元素及其索引执行操作
func (a RArray) EachWithIndex(fn func(Object, int)) {
	for i, elem := range a.elements {
		fn(elem, i)
	}
}

// EachCons 对数组中的每个连续n个元素执行操作
func (a RArray) EachCons(n int, fn func(RArray)) {
	if n <= 0 || n > len(a.elements) {
		return
	}
	for i := 0; i <= len(a.elements)-n; i++ {
		fn(NewRArray(a.elements[i : i+n]))
	}
}

// EachSlice 将数组分成n个元素的切片并执行操作
func (a RArray) EachSlice(n int, fn func(RArray)) {
	if n <= 0 {
		return
	}
	for i := 0; i < len(a.elements); i += n {
		end := i + n
		if end > len(a.elements) {
			end = len(a.elements)
		}
		fn(NewRArray(a.elements[i:end]))
	}
}
