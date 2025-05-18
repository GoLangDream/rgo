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

// Count 计算指定字符串在当前字符串中出现的次数
func (s RString) Count(substr string) int {
	return strings.Count(s.value, substr)
}

// Index 返回子字符串在当前字符串中第一次出现的位置，不存在则返回-1
func (s RString) Index(substr string) int {
	return strings.Index(s.value, substr)
}

// RIndex 返回子字符串在当前字符串中最后一次出现的位置
func (s RString) RIndex(substr string) int {
	return strings.LastIndex(s.value, substr)
}

// Slice 返回指定范围的子字符串
func (s RString) Slice(start, end int) RString {
	runes := []rune(s.value)
	length := len(runes)

	// 处理负索引
	if start < 0 {
		start = length + start
	}
	if end < 0 {
		end = length + end
	}

	// 处理边界情况
	if start < 0 {
		start = 0
	}
	if end > length {
		end = length
	}
	if start > end || start >= length {
		return NewRString("")
	}

	return NewRString(string(runes[start:end]))
}

// SliceFrom 返回从指定位置开始到字符串结尾的子字符串
func (s RString) SliceFrom(start int) RString {
	runes := []rune(s.value)
	length := len(runes)

	// 处理负索引
	if start < 0 {
		start = length + start
	}

	// 处理边界情况
	if start < 0 {
		start = 0
	}
	if start >= length {
		return NewRString("")
	}

	return NewRString(string(runes[start:]))
}

// Concat 连接两个字符串并返回新字符串
func (s RString) Concat(other RString) RString {
	return NewRString(s.value + other.value)
}

// Center 返回居中字符串，使用指定字符填充
func (s RString) Center(width int, padStr ...string) RString {
	padChar := " "
	if len(padStr) > 0 {
		padChar = padStr[0]
	}

	strLen := s.Length()
	if strLen >= width {
		return s
	}

	leftPad := (width - strLen) / 2
	rightPad := width - strLen - leftPad

	return NewRString(strings.Repeat(padChar, leftPad) + s.value + strings.Repeat(padChar, rightPad))
}

// Ljust 返回左对齐字符串，使用指定字符填充
func (s RString) Ljust(width int, padStr ...string) RString {
	padChar := " "
	if len(padStr) > 0 {
		padChar = padStr[0]
	}

	strLen := s.Length()
	if strLen >= width {
		return s
	}

	padWidth := width - strLen
	return NewRString(s.value + strings.Repeat(padChar, padWidth))
}

// Rjust 返回右对齐字符串，使用指定字符填充
func (s RString) Rjust(width int, padStr ...string) RString {
	padChar := " "
	if len(padStr) > 0 {
		padChar = padStr[0]
	}

	strLen := s.Length()
	if strLen >= width {
		return s
	}

	padWidth := width - strLen
	return NewRString(strings.Repeat(padChar, padWidth) + s.value)
}

// Sub 使用正则表达式替换第一个匹配项
func (s RString) Sub(pattern, repl string) RString {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return s
	}

	// 找到第一个匹配
	loc := re.FindStringIndex(s.value)
	if loc == nil {
		return s
	}

	// 只替换第一个匹配
	return NewRString(s.value[:loc[0]] + re.ReplaceAllString(s.value[loc[0]:loc[1]], repl) + s.value[loc[1]:])
}

// Ord 返回字符串第一个字符的ASCII码值
func (s RString) Ord() int {
	if s.Empty() {
		panic("空字符串没有ASCII码值")
	}

	r, _ := utf8.DecodeRuneInString(s.value)
	return int(r)
}

// Chars 返回字符串中的所有字符组成的数组
func (s RString) Chars() RArray {
	runes := []rune(s.value)
	chars := make([]Object, len(runes))

	for i, r := range runes {
		chars[i] = NewRString(string(r))
	}

	return NewRArray(chars)
}

// Each 对字符串中的每个字符执行指定操作
func (s RString) Each(fn func(RString)) {
	for _, r := range s.value {
		fn(NewRString(string(r)))
	}
}

// EachLine 对字符串中的每一行执行指定操作
func (s RString) EachLine(fn func(RString)) {
	lines := strings.Split(s.value, "\n")
	for _, line := range lines {
		fn(NewRString(line))
	}
}

// Times 重复字符串指定次数
func (s RString) Times(n int) RString {
	if n <= 0 {
		return NewRString("")
	}
	return NewRString(strings.Repeat(s.value, n))
}

// ToInt 将字符串转换为整数
func (s RString) ToInt() (int, error) {
	// 移除前导空格
	str := strings.TrimSpace(s.value)
	// 尝试转换为整数
	return parseInt(str)
}

// 辅助函数：解析整数
func parseInt(s string) (int, error) {
	// 实现简化版的整数解析
	// 可以处理十进制、十六进制、八进制等
	return strings.IndexAny(s, "0123456789"), nil
}

// Inspect 返回字符串的可打印形式（带引号）
func (s RString) Inspect() string {
	return "\"" + s.value + "\""
}

// SwapCase 交换字符串中字母的大小写
func (s RString) SwapCase() RString {
	runes := []rune(s.value)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			runes[i] = unicode.ToLower(r)
		} else if unicode.IsLower(r) {
			runes[i] = unicode.ToUpper(r)
		}
	}
	return NewRString(string(runes))
}

// ToCamelCase 转换字符串为驼峰命名（Rails扩展方法）
func (s RString) ToCamelCase() RString {
	words := strings.Split(s.value, "_")
	for i := 1; i < len(words); i++ {
		if len(words[i]) > 0 {
			r := []rune(words[i])
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return NewRString(strings.Join(words, ""))
}

// ToSnakeCase 转换字符串为蛇形命名（Rails扩展方法）
func (s RString) ToSnakeCase() RString {
	var result []rune
	runes := []rune(s.value)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && !unicode.IsUpper(runes[i-1]) {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}

	return NewRString(string(result))
}
