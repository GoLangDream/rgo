package rgo

import (
	"strconv"
	"strings"
	"testing"
)

// TestStruct 用于测试的原生结构体
type TestStruct struct{}

// Greet 测试方法
func (t TestStruct) Greet() string {
	return "Hello World"
}

// =============================================================================
// RString 性能测试
// =============================================================================

// BenchmarkRStringNew 测试 RString 创建性能
func BenchmarkRStringNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewRString("Hello World")
	}
}

// BenchmarkNativeStringNew 测试原生字符串创建性能
func BenchmarkNativeStringNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = "Hello World"
	}
}

// BenchmarkRStringLength 测试 RString 长度计算性能
func BenchmarkRStringLength(b *testing.B) {
	str := NewRString("Hello World, 你好世界！")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str.Length()
	}
}

// BenchmarkNativeStringLength 测试原生字符串长度计算性能
func BenchmarkNativeStringLength(b *testing.B) {
	str := "Hello World, 你好世界！"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = len([]rune(str))
	}
}

// BenchmarkRStringConcat 测试 RString 连接性能
func BenchmarkRStringConcat(b *testing.B) {
	str1 := NewRString("Hello")
	str2 := NewRString(" World")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str1.Concat(str2)
	}
}

// BenchmarkNativeStringConcat 测试原生字符串连接性能
func BenchmarkNativeStringConcat(b *testing.B) {
	str1 := "Hello"
	str2 := " World"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str1 + str2
	}
}

// BenchmarkRStringUpcase 测试 RString 大写转换性能
func BenchmarkRStringUpcase(b *testing.B) {
	str := NewRString("Hello World")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str.Upcase()
	}
}

// BenchmarkNativeStringUpcase 测试原生字符串大写转换性能
func BenchmarkNativeStringUpcase(b *testing.B) {
	str := "Hello World"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strings.ToUpper(str)
	}
}

// BenchmarkRStringSplit 测试 RString 分割性能
func BenchmarkRStringSplit(b *testing.B) {
	str := NewRString("hello,world,golang,benchmark")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str.Split(",")
	}
}

// BenchmarkNativeStringSplit 测试原生字符串分割性能
func BenchmarkNativeStringSplit(b *testing.B) {
	str := "hello,world,golang,benchmark"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strings.Split(str, ",")
	}
}

// BenchmarkRStringReplace 测试 RString 替换性能
func BenchmarkRStringReplace(b *testing.B) {
	str := NewRString("Hello World Hello World Hello World")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = str.ReplaceAll("Hello", "Hi")
	}
}

// BenchmarkNativeStringReplace 测试原生字符串替换性能
func BenchmarkNativeStringReplace(b *testing.B) {
	str := "Hello World Hello World Hello World"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strings.ReplaceAll(str, "Hello", "Hi")
	}
}

// =============================================================================
// RInteger 性能测试
// =============================================================================

// BenchmarkRIntegerNew 测试 RInteger 创建性能
func BenchmarkRIntegerNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewRInteger(42)
	}
}

// BenchmarkNativeIntNew 测试原生 int 创建性能
func BenchmarkNativeIntNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = 42
	}
}

// BenchmarkRIntegerAdd 测试 RInteger 加法性能
func BenchmarkRIntegerAdd(b *testing.B) {
	num := NewRInteger(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num.Add(42)
	}
}

// BenchmarkNativeIntAdd 测试原生 int 加法性能
func BenchmarkNativeIntAdd(b *testing.B) {
	num := 100
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num + 42
	}
}

// BenchmarkRIntegerMul 测试 RInteger 乘法性能
func BenchmarkRIntegerMul(b *testing.B) {
	num := NewRInteger(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num.Mul(42)
	}
}

// BenchmarkNativeIntMul 测试原生 int 乘法性能
func BenchmarkNativeIntMul(b *testing.B) {
	num := 100
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num * 42
	}
}

// BenchmarkRIntegerPow 测试 RInteger 幂运算性能
func BenchmarkRIntegerPow(b *testing.B) {
	num := NewRInteger(2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num.Pow(10)
	}
}

// BenchmarkNativeIntPow 测试原生 int 幂运算性能
func BenchmarkNativeIntPow(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := 1
		base, exp := 2, 10
		for j := 0; j < exp; j++ {
			result *= base
		}
		_ = result
	}
}

// BenchmarkRIntegerToString 测试 RInteger 转字符串性能
func BenchmarkRIntegerToString(b *testing.B) {
	num := NewRInteger(123456)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = num.ToString()
	}
}

// BenchmarkNativeIntToString 测试原生 int 转字符串性能
func BenchmarkNativeIntToString(b *testing.B) {
	num := 123456
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strconv.Itoa(num)
	}
}

// BenchmarkRIntegerChainedOps 测试 RInteger 链式操作性能
func BenchmarkRIntegerChainedOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		num := NewRInteger(10)
		_ = num.Add(5).Mul(2).Sub(3)
	}
}

// BenchmarkNativeIntChainedOps 测试原生 int 链式操作性能
func BenchmarkNativeIntChainedOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		num := 10
		_ = (num+5)*2 - 3
	}
}

// =============================================================================
// RHash 性能测试
// =============================================================================

// BenchmarkRHashNew 测试 RHash 创建性能
func BenchmarkRHashNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewHash()
	}
}

// BenchmarkNativeMapNew 测试原生 map 创建性能
func BenchmarkNativeMapNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = make(map[any]any)
	}
}

// BenchmarkRHashSet 测试 RHash 设置性能
func BenchmarkRHashSet(b *testing.B) {
	hash := NewHash()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash.Set("key", "value")
	}
}

// BenchmarkNativeMapSet 测试原生 map 设置性能
func BenchmarkNativeMapSet(b *testing.B) {
	m := make(map[any]any)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m["key"] = "value"
	}
}

// BenchmarkRHashGet 测试 RHash 获取性能
func BenchmarkRHashGet(b *testing.B) {
	hash := NewHash()
	hash.Set("key", "value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hash.Get("key")
	}
}

// BenchmarkNativeMapGet 测试原生 map 获取性能
func BenchmarkNativeMapGet(b *testing.B) {
	m := make(map[any]any)
	m["key"] = "value"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m["key"]
	}
}

// BenchmarkRHashDelete 测试 RHash 删除性能
func BenchmarkRHashDelete(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hash := NewHash()
		hash.Set("key", "value")
		_ = hash.Delete("key")
	}
}

// BenchmarkNativeMapDelete 测试原生 map 删除性能
func BenchmarkNativeMapDelete(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := make(map[any]any)
		m["key"] = "value"
		delete(m, "key")
	}
}

// BenchmarkRHashMerge 测试 RHash 合并性能
func BenchmarkRHashMerge(b *testing.B) {
	hash1 := NewHash()
	hash2 := NewHash()
	for i := 0; i < 100; i++ {
		hash1.Set(i, i*2)
		hash2.Set(i+100, (i+100)*2)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hash1.Merge(hash2)
	}
}

// BenchmarkNativeMapMerge 测试原生 map 合并性能
func BenchmarkNativeMapMerge(b *testing.B) {
	m1 := make(map[any]any)
	m2 := make(map[any]any)
	for i := 0; i < 100; i++ {
		m1[i] = i * 2
		m2[i+100] = (i + 100) * 2
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := make(map[any]any)
		for k, v := range m1 {
			result[k] = v
		}
		for k, v := range m2 {
			result[k] = v
		}
		_ = result
	}
}

// BenchmarkRHashKeys 测试 RHash 获取键列表性能
func BenchmarkRHashKeys(b *testing.B) {
	hash := NewHash()
	for i := 0; i < 1000; i++ {
		hash.Set(i, i*2)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hash.Keys()
	}
}

// BenchmarkNativeMapKeys 测试原生 map 获取键列表性能
func BenchmarkNativeMapKeys(b *testing.B) {
	m := make(map[any]any)
	for i := 0; i < 1000; i++ {
		m[i] = i * 2
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keys := make([]any, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		_ = keys
	}
}

// =============================================================================
// RClass 性能测试
// =============================================================================

// BenchmarkRClassNew 测试 RClass 创建性能
func BenchmarkRClassNew(b *testing.B) {
	class := Class("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = class.New()
	}
}

// BenchmarkNativeStructNew 测试原生结构体创建性能
func BenchmarkNativeStructNew(b *testing.B) {
	type TestStruct2 struct {
		name string
		data map[string]any
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TestStruct2{
			name: "TestStruct",
			data: make(map[string]any),
		}
	}
}

// BenchmarkRClassDefineMethod 测试 RClass 方法定义性能
func BenchmarkRClassDefineMethod(b *testing.B) {
	class := Class("TestClass")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		methodName := "method" + strconv.Itoa(i)
		class.DefineMethod(methodName, func() string {
			return "hello"
		})
	}
}

// BenchmarkNativeStructMethod 测试原生结构体方法定义性能（模拟）
func BenchmarkNativeStructMethod(b *testing.B) {
	type TestStruct3 struct {
		methods map[string]func() string
	}
	s := TestStruct3{
		methods: make(map[string]func() string),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		methodName := "method" + strconv.Itoa(i)
		s.methods[methodName] = func() string {
			return "hello"
		}
	}
}

// BenchmarkRClassMethodCall 测试 RClass 方法调用性能
func BenchmarkRClassMethodCall(b *testing.B) {
	class := Class("TestClass")
	class.DefineMethod("greet", func() string {
		return "Hello World"
	})
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = instance.Call("greet")
	}
}

// BenchmarkNativeStructMethodCall 测试原生结构体方法调用性能
func BenchmarkNativeStructMethodCall(b *testing.B) {
	s := TestStruct{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Greet()
	}
}

// BenchmarkRClassInstanceVar 测试 RClass 实例变量操作性能
func BenchmarkRClassInstanceVar(b *testing.B) {
	class := Class("TestClass")
	instance := class.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance.SetInstanceVar("name", "test")
		_ = instance.GetInstanceVar("name")
	}
}

// BenchmarkNativeStructField 测试原生结构体字段操作性能
func BenchmarkNativeStructField(b *testing.B) {
	type TestStruct4 struct {
		name string
	}
	s := &TestStruct4{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.name = "test"
		_ = s.name
	}
}

// BenchmarkRClassAttrAccessor 测试 RClass 属性访问器性能func BenchmarkRClassAttrAccessor(b *testing.B) {	class := Class("TestClass")	class.DefineMethod("name", func() any {		return "test"	})	class.DefineMethod("name=", func(value any) any {		return value	})	instance := class.New()	b.ResetTimer()	for i := 0; i < b.N; i++ {		_ = instance.Call("name=", "test")		_ = instance.Call("name")	}}

// =============================================================================
// 综合性能测试
// =============================================================================

// BenchmarkRStringComplexOps 测试 RString 复杂操作性能
func BenchmarkRStringComplexOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		str := NewRString("Hello World")
		parts := str.Upcase().Reverse().Split(" ")
		if parts.Size() > 0 {
			result := parts.Get(0).(RString).Downcase()
			_ = result.ToString()
		}
	}
}

// BenchmarkNativeStringComplexOps 测试原生字符串复杂操作性能
func BenchmarkNativeStringComplexOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		str := "Hello World"
		upper := strings.ToUpper(str)
		runes := []rune(upper)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		reversed := string(runes)
		parts := strings.Split(reversed, " ")
		if len(parts) > 0 {
			result := strings.ToLower(parts[0])
			_ = result
		}
	}
}

// BenchmarkRIntegerComplexOps 测试 RInteger 复杂操作性能
func BenchmarkRIntegerComplexOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		num := NewRInteger(10)
		result := num.Add(5).Mul(2).Pow(2).Sub(50)
		_ = result.ToString()
	}
}

// BenchmarkNativeIntComplexOps 测试原生 int 复杂操作性能
func BenchmarkNativeIntComplexOps(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		num := 10
		temp := (num + 5) * 2
		// 模拟幂运算
		powered := temp * temp
		result := powered - 50
		_ = strconv.Itoa(result)
	}
}

// BenchmarkMemoryAllocation 测试内存分配对比
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("RString", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			str := NewRString("test")
			_ = str.Upcase()
		}
	})

	b.Run("NativeString", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			str := "test"
			_ = strings.ToUpper(str)
		}
	})

	b.Run("RInteger", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			num := NewRInteger(42)
			_ = num.Add(1)
		}
	})

	b.Run("NativeInt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			num := 42
			_ = num + 1
		}
	})

	b.Run("RHash", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			hash := NewHash()
			hash.Set("key", "value")
			_, _ = hash.Get("key")
		}
	})

	b.Run("NativeMap", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m := make(map[any]any)
			m["key"] = "value"
			_, _ = m["key"]
		}
	})
}
