package goby_test

import (
	"testing"

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

		It("应该正确交换大小写", func() {
			Expect(NewRString("Hello World").SwapCase().ToString()).To(Equal("hELLO wORLD"))
			Expect(NewRString("123").SwapCase().ToString()).To(Equal("123"))
		})

		It("应该正确转换为驼峰命名", func() {
			Expect(NewRString("hello_world").ToCamelCase().ToString()).To(Equal("helloWorld"))
			Expect(NewRString("user_id").ToCamelCase().ToString()).To(Equal("userId"))
		})

		It("应该正确转换为蛇形命名", func() {
			Expect(NewRString("helloWorld").ToSnakeCase().ToString()).To(Equal("hello_world"))
			Expect(NewRString("userId").ToSnakeCase().ToString()).To(Equal("user_id"))
		})

		It("应该正确处理空字符串的变换", func() {
			Expect(emptyStr.Capitalize().ToString()).To(Equal(""))
			Expect(emptyStr.Upcase().ToString()).To(Equal(""))
			Expect(emptyStr.Downcase().ToString()).To(Equal(""))
			Expect(emptyStr.Strip().ToString()).To(Equal(""))
			Expect(emptyStr.Chomp().ToString()).To(Equal(""))
			Expect(emptyStr.Reverse().ToString()).To(Equal(""))
			Expect(emptyStr.SwapCase().ToString()).To(Equal(""))
			Expect(emptyStr.ToCamelCase().ToString()).To(Equal(""))
			Expect(emptyStr.ToSnakeCase().ToString()).To(Equal(""))
		})

		It("应该正确处理特殊字符的变换", func() {
			specialStr := NewRString("!@#$%^&*()")
			Expect(specialStr.Upcase().ToString()).To(Equal("!@#$%^&*()"))
			Expect(specialStr.Downcase().ToString()).To(Equal("!@#$%^&*()"))
			Expect(specialStr.SwapCase().ToString()).To(Equal("!@#$%^&*()"))
		})

		It("应该正确处理Unicode字符的变换", func() {
			unicodeStr := NewRString("你好世界")
			Expect(unicodeStr.Upcase().ToString()).To(Equal("你好世界"))
			Expect(unicodeStr.Downcase().ToString()).To(Equal("你好世界"))
			Expect(unicodeStr.SwapCase().ToString()).To(Equal("你好世界"))
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

		It("应该正确使用Sub替换第一个匹配项", func() {
			Expect(str.Sub(`\w+`, "hi").ToString()).To(Equal("hi world"))
			Expect(NewRString("one two three").Sub(`\w+`, "word").ToString()).To(Equal("word two three"))
		})

		It("应该正确计算子串出现次数", func() {
			Expect(NewRString("hello hello world").Count("hello")).To(Equal(2))
			Expect(NewRString("abababab").Count("ab")).To(Equal(4))
			Expect(emptyStr.Count("any")).To(Equal(0))
		})

		It("应该正确处理空字符串的查找和替换", func() {
			Expect(emptyStr.Include("any")).To(BeFalse())
			Expect(emptyStr.StartsWith("any")).To(BeFalse())
			Expect(emptyStr.EndsWith("any")).To(BeFalse())
			Expect(emptyStr.ReplaceAll("any", "new").ToString()).To(Equal(""))
			Expect(emptyStr.Match(`.*`)).To(BeTrue())
			Expect(emptyStr.Gsub(`.*`, "new").ToString()).To(Equal("new"))
			Expect(emptyStr.Sub(`.*`, "new").ToString()).To(Equal("new"))
			Expect(emptyStr.Count("any")).To(Equal(0))
		})

		It("应该正确处理正则表达式的特殊情况", func() {
			Expect(str.Match(`^$`)).To(BeFalse())
			Expect(str.Gsub(`^$`, "new").ToString()).To(Equal("hello world"))
			Expect(str.Sub(`^$`, "new").ToString()).To(Equal("hello world"))
		})

		It("应该正确处理多次替换", func() {
			repeatStr := NewRString("hello hello hello")
			Expect(repeatStr.ReplaceAll("hello", "hi").ToString()).To(Equal("hi hi hi"))
			Expect(repeatStr.Gsub(`hello`, "hi").ToString()).To(Equal("hi hi hi"))
		})
	})

	Context("分割字符串", func() {
		It("应该正确分割字符串", func() {
			result := str.Split(" ")
			Expect(result.Length()).To(Equal(2))
			Expect(result.Get(0).ToString()).To(Equal("hello"))
			Expect(result.Get(1).ToString()).To(Equal("world"))
		})

		It("应该正确处理空字符串的分割", func() {
			result := emptyStr.Split(" ")
			Expect(result.Length()).To(Equal(1))
			Expect(result.Get(0).ToString()).To(Equal(""))
		})

		It("应该正确处理不包含分隔符的字符串", func() {
			result := str.Split(",")
			Expect(result.Length()).To(Equal(1))
			Expect(result.Get(0).ToString()).To(Equal("hello world"))
		})

		It("应该正确处理连续分隔符", func() {
			spacesStr := NewRString("hello   world")
			result := spacesStr.Split(" ")
			Expect(result.Length()).To(Equal(4))
			Expect(result.Get(0).ToString()).To(Equal("hello"))
			Expect(result.Get(3).ToString()).To(Equal("world"))
		})
	})

	Context("索引和切片", func() {
		It("应该返回正确的索引位置", func() {
			Expect(str.Index("hello")).To(Equal(0))
			Expect(str.Index("world")).To(Equal(6))
			Expect(str.Index("nonexistent")).To(Equal(-1))
		})

		It("应该返回正确的最后索引位置", func() {
			repeatStr := NewRString("hello world hello")
			Expect(repeatStr.RIndex("hello")).To(Equal(12))
			Expect(str.RIndex("world")).To(Equal(6))
			Expect(str.RIndex("nonexistent")).To(Equal(-1))
		})

		It("应该返回正确的子串", func() {
			Expect(str.Slice(0, 5).ToString()).To(Equal("hello"))
			Expect(str.Slice(6, 11).ToString()).To(Equal("world"))
			Expect(str.Slice(-5, -1).ToString()).To(Equal("worl"))
			Expect(str.Slice(20, 25).ToString()).To(Equal(""))
		})

		It("应该返回正确的从某位置开始的子串", func() {
			Expect(str.SliceFrom(6).ToString()).To(Equal("world"))
			Expect(str.SliceFrom(-5).ToString()).To(Equal("world"))
			Expect(str.SliceFrom(20).ToString()).To(Equal(""))
		})

		It("应该正确处理空字符串的索引和切片", func() {
			Expect(emptyStr.Index("any")).To(Equal(-1))
			Expect(emptyStr.RIndex("any")).To(Equal(-1))
			Expect(emptyStr.Slice(0, 1).ToString()).To(Equal(""))
			Expect(emptyStr.SliceFrom(0).ToString()).To(Equal(""))
		})

		It("应该正确处理越界索引", func() {
			Expect(str.Index("nonexistent")).To(Equal(-1))
			Expect(str.RIndex("nonexistent")).To(Equal(-1))
			Expect(str.Slice(100, 200).ToString()).To(Equal(""))
			Expect(str.SliceFrom(100).ToString()).To(Equal(""))
		})

		It("应该正确处理负索引", func() {
			Expect(str.Slice(-5, -1).ToString()).To(Equal("worl"))
			Expect(str.SliceFrom(-5).ToString()).To(Equal("world"))
		})
	})

	Context("格式化和对齐", func() {
		It("应该正确居中对齐字符串", func() {
			Expect(NewRString("hello").Center(11).ToString()).To(Equal("   hello   "))
			Expect(NewRString("hello").Center(11, "-").ToString()).To(Equal("---hello---"))
			Expect(NewRString("hello").Center(4).ToString()).To(Equal("hello"))
		})

		It("应该正确左对齐字符串", func() {
			Expect(NewRString("hello").Ljust(10).ToString()).To(Equal("hello     "))
			Expect(NewRString("hello").Ljust(10, "*").ToString()).To(Equal("hello*****"))
		})

		It("应该正确右对齐字符串", func() {
			Expect(NewRString("hello").Rjust(10).ToString()).To(Equal("     hello"))
			Expect(NewRString("hello").Rjust(10, "*").ToString()).To(Equal("*****hello"))
		})

		It("应该正确处理空字符串的对齐", func() {
			Expect(emptyStr.Center(5).ToString()).To(Equal("     "))
			Expect(emptyStr.Ljust(5).ToString()).To(Equal("     "))
			Expect(emptyStr.Rjust(5).ToString()).To(Equal("     "))
		})

		It("应该正确处理长度小于填充长度的情况", func() {
			Expect(NewRString("hi").Center(1).ToString()).To(Equal("hi"))
			Expect(NewRString("hi").Ljust(1).ToString()).To(Equal("hi"))
			Expect(NewRString("hi").Rjust(1).ToString()).To(Equal("hi"))
		})

		It("应该正确处理自定义填充字符", func() {
			Expect(NewRString("hi").Center(5, "*").ToString()).To(Equal("*hi**"))
			Expect(NewRString("hi").Ljust(5, "*").ToString()).To(Equal("hi***"))
			Expect(NewRString("hi").Rjust(5, "*").ToString()).To(Equal("***hi"))
		})
	})

	Context("字符操作", func() {
		It("应该返回正确的ASCII码值", func() {
			Expect(NewRString("A").Ord()).To(Equal(65))
			Expect(NewRString("a").Ord()).To(Equal(97))
		})

		It("应该返回正确的字符数组", func() {
			chars := str.Chars()
			Expect(chars.Length()).To(Equal(11))
			Expect(chars.Get(0).ToString()).To(Equal("h"))
			Expect(chars.Get(5).ToString()).To(Equal(" "))
		})

		It("应该正确处理空字符串的字符操作", func() {
			Expect(func() { emptyStr.Ord() }).To(PanicWith("空字符串没有ASCII码值"))
			chars := emptyStr.Chars()
			Expect(chars.Length()).To(Equal(0))
		})

		It("应该正确处理Unicode字符的字符操作", func() {
			unicodeStr := NewRString("你好")
			chars := unicodeStr.Chars()
			Expect(chars.Length()).To(Equal(2))
			Expect(chars.Get(0).ToString()).To(Equal("你"))
			Expect(chars.Get(1).ToString()).To(Equal("好"))
		})
	})

	Context("字符串操作", func() {
		It("应该正确连接字符串", func() {
			Expect(str.Concat(NewRString("!")).ToString()).To(Equal("hello world!"))
			Expect(emptyStr.Concat(str).ToString()).To(Equal("hello world"))
		})

		It("应该正确重复字符串", func() {
			Expect(NewRString("ab").Times(3).ToString()).To(Equal("ababab"))
			Expect(NewRString("ab").Times(0).ToString()).To(Equal(""))
		})

		It("应该正确处理空字符串的连接", func() {
			Expect(emptyStr.Concat(NewRString("hello")).ToString()).To(Equal("hello"))
			Expect(NewRString("hello").Concat(emptyStr).ToString()).To(Equal("hello"))
			Expect(emptyStr.Concat(emptyStr).ToString()).To(Equal(""))
		})

		It("应该正确处理字符串的重复", func() {
			Expect(NewRString("ab").Times(0).ToString()).To(Equal(""))
			Expect(NewRString("ab").Times(1).ToString()).To(Equal("ab"))
			Expect(NewRString("ab").Times(3).ToString()).To(Equal("ababab"))
		})
	})

	Context("检查和调试", func() {
		It("应该返回正确的检查字符串", func() {
			Expect(str.Inspect()).To(Equal(`"hello world"`))
			Expect(emptyStr.Inspect()).To(Equal(`""`))
		})
	})
})

func TestRString_Gsub(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pattern  string
		repl     string
		expected string
	}{
		{
			name:     "无效的正则表达式",
			input:    "hello",
			pattern:  "[",
			repl:     "x",
			expected: "hello",
		},
		{
			name:     "有效的正则表达式替换",
			input:    "hello world",
			pattern:  "o",
			repl:     "x",
			expected: "hellx wxrld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			result := s.Gsub(tt.pattern, tt.repl)
			if result.ToString() != tt.expected {
				t.Errorf("Gsub() = %v, want %v", result.ToString(), tt.expected)
			}
		})
	}
}

func TestRString_Sub(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pattern  string
		repl     string
		expected string
	}{
		{
			name:     "无效的正则表达式",
			input:    "hello",
			pattern:  "[",
			repl:     "x",
			expected: "hello",
		},
		{
			name:     "无匹配项",
			input:    "hello",
			pattern:  "xyz",
			repl:     "x",
			expected: "hello",
		},
		{
			name:     "替换第一个匹配项",
			input:    "hello world",
			pattern:  "o",
			repl:     "x",
			expected: "hellx world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			result := s.Sub(tt.pattern, tt.repl)
			if result.ToString() != tt.expected {
				t.Errorf("Sub() = %v, want %v", result.ToString(), tt.expected)
			}
		})
	}
}

func TestRString_Ord(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int
		shouldPanic bool
	}{
		{
			name:        "空字符串",
			input:       "",
			expected:    0,
			shouldPanic: true,
		},
		{
			name:        "ASCII字符",
			input:       "A",
			expected:    65,
			shouldPanic: false,
		},
		{
			name:        "中文字符",
			input:       "中",
			expected:    20013,
			shouldPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil && !tt.shouldPanic {
					t.Errorf("Ord() panic = %v, want no panic", r)
				}
			}()

			s := NewRString(tt.input)
			result := s.Ord()
			if !tt.shouldPanic && result != tt.expected {
				t.Errorf("Ord() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRString_Chars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: []string{},
		},
		{
			name:     "ASCII字符串",
			input:    "hello",
			expected: []string{"h", "e", "l", "l", "o"},
		},
		{
			name:     "中文字符串",
			input:    "你好",
			expected: []string{"你", "好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			result := s.Chars()

			if len(result.ToArray()) != len(tt.expected) {
				t.Errorf("Chars() length = %v, want %v", len(result.ToArray()), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if str, ok := result.ToArray()[i].(RString); !ok || str.ToString() != expected {
					t.Errorf("Chars()[%d] = %v, want %v", i, result.ToArray()[i], expected)
				}
			}
		})
	}
}

func TestRString_Each(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: []string{},
		},
		{
			name:     "ASCII字符串",
			input:    "hello",
			expected: []string{"h", "e", "l", "l", "o"},
		},
		{
			name:     "中文字符串",
			input:    "你好",
			expected: []string{"你", "好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			var result []string

			s.Each(func(r RString) {
				result = append(result, r.ToString())
			})

			if len(result) != len(tt.expected) {
				t.Errorf("Each() length = %v, want %v", len(result), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Each()[%d] = %v, want %v", i, result[i], expected)
				}
			}
		})
	}
}

func TestRString_EachLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "单行字符串",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "多行字符串",
			input:    "hello\nworld\n你好",
			expected: []string{"hello", "world", "你好"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			var result []string

			s.EachLine(func(r RString) {
				result = append(result, r.ToString())
			})

			if len(result) != len(tt.expected) {
				t.Errorf("EachLine() length = %v, want %v", len(result), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("EachLine()[%d] = %v, want %v", i, result[i], expected)
				}
			}
		})
	}
}

func TestRString_Times(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		times    int
		expected string
	}{
		{
			name:     "零次重复",
			input:    "hello",
			times:    0,
			expected: "",
		},
		{
			name:     "负数重复",
			input:    "hello",
			times:    -1,
			expected: "",
		},
		{
			name:     "一次重复",
			input:    "hello",
			times:    1,
			expected: "hello",
		},
		{
			name:     "多次重复",
			input:    "hello",
			times:    3,
			expected: "hellohellohello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			result := s.Times(tt.times)
			if result.ToString() != tt.expected {
				t.Errorf("Times() = %v, want %v", result.ToString(), tt.expected)
			}
		})
	}
}

func TestRString_ToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: -1,
		},
		{
			name:     "纯数字字符串",
			input:    "123",
			expected: 0,
		},
		{
			name:     "带空格的数字字符串",
			input:    "  456  ",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRString(tt.input)
			result, _ := s.ToInt()
			if result != tt.expected {
				t.Errorf("ToInt() = %v, want %v", result, tt.expected)
			}
		})
	}
}
