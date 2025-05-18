package goby

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RString 实现类似 Ruby 中 String 类的功能
type RString struct {
	BaseObject
	value string
}

// NewRString 创建一个新的 RString 对象
func NewRString(s string) RString {
	return RString{
		BaseObject: NewBaseObject("String"),
		value:      s,
	}
}

// ToString 返回字符串表示
func (s RString) ToString() string {
	return s.value
}

// Equal 比较两个对象是否相等
func (s RString) Equal(other Object) bool {
	if otherStr, ok := other.(RString); ok {
		return s.value == otherStr.value
	}
	return false
}

// Length 返回字符串长度（等同于 Ruby 的 length 或 size 方法）
func (s RString) Length() int {
	return utf8.RuneCountInString(s.value)
}

// Size 是 Length 的别名
func (s RString) Size() int {
	return s.Length()
}

// Empty 检查字符串是否为空
func (s RString) Empty() bool {
	return s.value == ""
}

// Capitalize 将字符串首字母大写
func (s RString) Capitalize() RString {
	if s.Empty() {
		return s
	}

	runes := []rune(s.value)
	runes[0] = unicode.ToUpper(runes[0])
	return NewRString(string(runes))
}

// Downcase 将字符串转为小写
func (s RString) Downcase() RString {
	return NewRString(strings.ToLower(s.value))
}

// Upcase 将字符串转为大写
func (s RString) Upcase() RString {
	return NewRString(strings.ToUpper(s.value))
}

// Strip 去除字符串两端的空白
func (s RString) Strip() RString {
	return NewRString(strings.TrimSpace(s.value))
}

// Chomp 去除字符串末尾的换行符
func (s RString) Chomp() RString {
	return NewRString(strings.TrimRight(s.value, "\r\n"))
}

// Include 检查字符串是否包含子串
func (s RString) Include(substr string) bool {
	return strings.Contains(s.value, substr)
}

// Split 按照分隔符分割字符串
func (s RString) Split(sep string) RArray {
	parts := strings.Split(s.value, sep)
	strs := make([]Object, len(parts))
	for i, part := range parts {
		strs[i] = NewRString(part)
	}
	return NewRArray(strs)
}

// StartsWith 检查字符串是否以指定前缀开始
func (s RString) StartsWith(prefix string) bool {
	return strings.HasPrefix(s.value, prefix)
}

// EndsWith 检查字符串是否以指定后缀结束
func (s RString) EndsWith(suffix string) bool {
	return strings.HasSuffix(s.value, suffix)
}

// Reverse 反转字符串
func (s RString) Reverse() RString {
	runes := []rune(s.value)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return NewRString(string(runes))
}

// ReplaceAll 替换字符串中的所有匹配项
func (s RString) ReplaceAll(old, new string) RString {
	return NewRString(strings.ReplaceAll(s.value, old, new))
}

// Match 检查字符串是否匹配指定正则表达式
func (s RString) Match(pattern string) bool {
	matched, _ := regexp.MatchString(pattern, s.value)
	return matched
}

// Gsub 使用正则表达式进行全局替换
func (s RString) Gsub(pattern, repl string) RString {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return s
	}
	return NewRString(re.ReplaceAllString(s.value, repl))
}
