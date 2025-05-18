// Package goby 提供了一个类似 Ruby 的哈希表实现
package goby

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// HashClass 定义了哈希表的类名
	HashClass = "Hash"
)

// RHash 实现了类似 Ruby 的哈希表功能
type RHash struct {
	BaseObject
	value map[any]any
}

// NewHash 创建一个新的空哈希表
func NewHash() *RHash {
	return &RHash{
		BaseObject: NewBaseObject(HashClass),
		value:      make(map[any]any),
	}
}

// NewHashWithMap 使用给定的 map 创建一个新的哈希表
func NewHashWithMap(m map[any]any) *RHash {
	return &RHash{
		BaseObject: NewBaseObject(HashClass),
		value:      m,
	}
}

// Get 返回指定键对应的值
func (h *RHash) Get(key any) (any, bool) {
	val, ok := h.value[key]
	return val, ok
}

// Set 设置指定键的值
func (h *RHash) Set(key, value any) {
	h.value[key] = value
}

// Delete 删除指定键的值对，并返回被删除的值
func (h *RHash) Delete(key any) any {
	val := h.value[key]
	delete(h.value, key)
	return val
}

// Size 返回哈希表中的键值对数量
func (h *RHash) Size() int {
	return len(h.value)
}

// Keys 返回哈希表中的所有键，按键的字符串表示排序
func (h *RHash) Keys() []any {
	keys := make([]string, 0, len(h.value))
	for k := range h.value {
		keys = append(keys, fmt.Sprintf("%v", k))
	}
	sort.Strings(keys)
	result := make([]any, len(keys))
	for i, k := range keys {
		result[i] = k
	}
	return result
}

// Values 返回哈希表中的所有值
func (h *RHash) Values() []any {
	values := make([]any, 0, len(h.value))
	for _, v := range h.value {
		values = append(values, v)
	}
	return values
}

// Clear 清空哈希表中的所有键值对
func (h *RHash) Clear() {
	h.value = make(map[any]any)
}

// HasKey 检查哈希表是否包含指定的键
func (h *RHash) HasKey(key any) bool {
	_, ok := h.value[key]
	return ok
}

// HasValue 检查哈希表是否包含指定的值
func (h *RHash) HasValue(value any) bool {
	for _, v := range h.value {
		if reflect.DeepEqual(v, value) {
			return true
		}
	}
	return false
}

// Merge 将另一个哈希表合并到当前哈希表，返回新的哈希表
func (h *RHash) Merge(other *RHash) *RHash {
	result := NewHash()
	for k, v := range h.value {
		result.Set(k, v)
	}
	for k, v := range other.value {
		result.Set(k, v)
	}
	return result
}

// MergeBang 将另一个哈希表合并到当前哈希表（原地修改）
func (h *RHash) MergeBang(other *RHash) {
	for k, v := range other.value {
		h.Set(k, v)
	}
}

// ToString 返回哈希表的字符串表示
func (h *RHash) ToString() string {
	return fmt.Sprintf("%v", h.value)
}

// Inspect 返回哈希表的详细字符串表示
func (h *RHash) Inspect() string {
	return fmt.Sprintf("%v", h.value)
}

// Each 遍历哈希表中的每个键值对
func (h *RHash) Each(fn func(key, value any)) {
	for k, v := range h.value {
		fn(k, v)
	}
}

// Select 返回一个新的哈希表，包含所有满足条件的键值对
func (h *RHash) Select(fn func(key, value any) bool) *RHash {
	result := NewHash()
	for k, v := range h.value {
		if fn(k, v) {
			result.Set(k, v)
		}
	}
	return result
}

// Reject 返回一个新的哈希表，包含所有不满足条件的键值对
func (h *RHash) Reject(fn func(key, value any) bool) *RHash {
	result := NewHash()
	for k, v := range h.value {
		if !fn(k, v) {
			result.Set(k, v)
		}
	}
	return result
}

// TransformKeys 返回一个新的哈希表，其中的键经过转换函数处理
func (h *RHash) TransformKeys(fn func(key any) any) *RHash {
	result := NewHash()
	for k, v := range h.value {
		result.Set(fn(k), v)
	}
	return result
}

// TransformValues 返回一个新的哈希表，其中的值经过转换函数处理
func (h *RHash) TransformValues(fn func(value any) any) *RHash {
	result := NewHash()
	for k, v := range h.value {
		result.Set(k, fn(v))
	}
	return result
}

// Fetch 获取指定键的值，如果键不存在则返回默认值
func (h *RHash) Fetch(key any, defaultValue ...any) (any, error) {
	if val, ok := h.value[key]; ok {
		return val, nil
	}
	if len(defaultValue) > 0 {
		return defaultValue[0], nil
	}
	return nil, fmt.Errorf("key not found: %v", key)
}

// Default 返回哈希表的默认值
func (h *RHash) Default() any {
	return nil
}

// DefaultProc 返回哈希表的默认处理函数
func (h *RHash) DefaultProc() any {
	return nil
}

// CompareByIdentity 设置哈希表使用身份比较
func (h *RHash) CompareByIdentity() {
	// Go 语言默认使用身份比较
}

// CompareByIdentityQ 检查哈希表是否使用身份比较
func (h *RHash) CompareByIdentityQ() bool {
	return true
}

// ToA 将哈希表转换为键值对数组
func (h *RHash) ToA() RArray {
	pairs := make([]Object, 0, len(h.value))
	for k, v := range h.value {
		pair := NewRArray([]Object{
			NewRString(fmt.Sprintf("%v", k)),
			NewRString(fmt.Sprintf("%v", v)),
		})
		pairs = append(pairs, pair)
	}
	return NewRArray(pairs)
}

// ToH 返回哈希表的副本
func (h *RHash) ToH() *RHash {
	return NewHashWithMap(h.value)
}

// ToS 将哈希表转换为字符串
func (h *RHash) ToS() RString {
	return NewRString(h.ToString())
}

// ToProc 返回一个函数，该函数返回指定键的值
func (h *RHash) ToProc() any {
	return func(key any) any {
		if val, ok := h.value[key]; ok {
			return val
		}
		return nil
	}
}

// ToJSON 将哈希表转换为 JSON 字符串
func (h *RHash) ToJSON() RString {
	orderedMap := make(map[string]any)
	for k, v := range h.value {
		orderedMap[fmt.Sprintf("%v", k)] = v
	}

	jsonBytes, err := json.Marshal(orderedMap)
	if err != nil {
		return NewRString("{}")
	}
	return NewRString(string(jsonBytes))
}

// ToYAML 将哈希表转换为 YAML 字符串
func (h *RHash) ToYAML() RString {
	orderedMap := make(map[string]any)
	for k, v := range h.value {
		orderedMap[fmt.Sprintf("%v", k)] = v
	}

	yamlBytes, err := yaml.Marshal(orderedMap)
	if err != nil {
		return NewRString("{}")
	}
	return NewRString(string(yamlBytes))
}

// ToXML 将哈希表转换为 XML 字符串
func (h *RHash) ToXML() RString {
	type HashEntry struct {
		Key   any `xml:"key"`
		Value any `xml:"value"`
	}

	type Hash struct {
		XMLName xml.Name    `xml:"hash"`
		Entries []HashEntry `xml:"entry"`
	}

	entries := make([]HashEntry, 0)
	keys := h.Keys()
	for _, k := range keys {
		entries = append(entries, HashEntry{Key: k, Value: h.value[k]})
	}

	hash := Hash{Entries: entries}
	xmlBytes, err := xml.MarshalIndent(hash, "", "  ")
	if err != nil {
		return NewRString("<hash></hash>")
	}
	return NewRString(string(xmlBytes))
}

// ToHTML 将哈希表转换为 HTML 字符串
func (h *RHash) ToHTML() RString {
	var sb strings.Builder
	sb.WriteString("<div class=\"hash\">\n")

	keys := h.Keys()
	for _, k := range keys {
		sb.WriteString("  <div class=\"entry\">\n")
		sb.WriteString(fmt.Sprintf("    <span class=\"key\">%v</span>\n", k))
		sb.WriteString(fmt.Sprintf("    <span class=\"value\">%v</span>\n", h.value[k]))
		sb.WriteString("  </div>\n")
	}

	sb.WriteString("</div>")
	return NewRString(sb.String())
}

// ToCSV 将哈希表转换为 CSV 字符串
func (h *RHash) ToCSV() RString {
	var sb strings.Builder
	writer := csv.NewWriter(&sb)

	writer.Write([]string{"key", "value"})

	keys := h.Keys()
	for _, k := range keys {
		writer.Write([]string{fmt.Sprintf("%v", k), fmt.Sprintf("%v", h.value[k])})
	}

	writer.Flush()
	return NewRString(strings.TrimSpace(sb.String()))
}

// ToTSV 将哈希表转换为 TSV 字符串
func (h *RHash) ToTSV() RString {
	var sb strings.Builder

	sb.WriteString("key\tvalue\n")

	keys := h.Keys()
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%v\t%v\n", k, h.value[k]))
	}

	return NewRString(strings.TrimSpace(sb.String()))
}

// Equal 比较两个哈希表是否相等
func (h *RHash) Equal(other Object) bool {
	if otherHash, ok := other.(*RHash); ok {
		return reflect.DeepEqual(h.value, otherHash.value)
	}
	return false
}
