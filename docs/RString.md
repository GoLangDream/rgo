# RString - Ruby风格的字符串类

`RString`是一个Go语言实现的模拟Ruby字符串功能的类型。它提供了丰富的字符串操作方法，可以让Go开发者使用类似Ruby的字符串API。

## 创建RString

```go
// 创建一个空字符串
emptyStr := NewRString("")

// 创建一个普通字符串
str := NewRString("hello world")
```

## 基本方法

### 长度和判空

```go
str.Length()   // 返回字符串字符数（支持Unicode）
str.Size()     // Length的别名
str.Empty()    // 检查字符串是否为空
```

### 大小写转换

```go
str.Upcase()       // 转为大写
str.Downcase()     // 转为小写
str.Capitalize()   // 首字母大写
str.SwapCase()     // 交换大小写
```

### 修剪和去除

```go
str.Strip()    // 去除两端空白
str.Chomp()    // 去除尾部换行符
```

### 查找和替换

```go
str.Include("hello")       // 检查是否包含子串
str.Index("hello")         // 返回子串首次出现的位置，不存在返回-1
str.RIndex("hello")        // 返回子串最后出现的位置，不存在返回-1
str.Count("hello")         // 计算子串出现次数

str.ReplaceAll("hello", "hi")  // 替换所有匹配项
str.Gsub("h.llo", "hi")        // 使用正则表达式全局替换
str.Sub("h.llo", "hi")         // 使用正则表达式替换第一个匹配
```

### 前缀和后缀

```go
str.StartsWith("hello")    // 检查是否以指定前缀开始
str.EndsWith("world")      // 检查是否以指定后缀结束
```

### 分割和连接

```go
str.Split(" ")             // 按分隔符分割字符串，返回RArray
str.Concat(otherStr)       // 连接两个字符串
```

### 子串提取

```go
str.Slice(0, 5)            // 提取子串，支持负索引
str.SliceFrom(6)           // 从指定位置到结尾提取子串
```

### 格式和对齐

```go
str.Center(20)             // 居中对齐，默认用空格填充
str.Center(20, "-")        // 居中对齐，用指定字符填充
str.Ljust(20)              // 左对齐
str.Rjust(20)              // 右对齐
```

### 字符和编码

```go
str.Ord()                  // 返回第一个字符的ASCII码值
str.Chars()                // 返回字符数组
```

### 迭代方法

```go
str.Each(func(char RString) {
    // 对每个字符执行操作
})

str.EachLine(func(line RString) {
    // 对每一行执行操作
})
```

### 重复

```go
str.Times(3)               // 重复字符串3次
```

### 转换方法

```go
str.ToInt()                // 转换为整数
str.Inspect()              // 返回带引号的字符串表示
```

### Rails扩展方法

```go
str.ToCamelCase()          // 转换为驼峰命名法（snake_case -> camelCase）
str.ToSnakeCase()          // 转换为蛇形命名法（camelCase -> snake_case）
```

## 和其他类型的交互

```go
// 转换为Go原生字符串
goStr := str.ToString()

// 和RArray交互
arr := str.Split(" ")
joinedStr := arr.Join("-")
```

## 完整方法列表

以下是RString提供的所有方法：

| 方法名 | 功能描述 |
|--------|----------|
| `ToString()` | 返回Go原生字符串 |
| `Equal(other)` | 比较两个对象是否相等 |
| `Length()` | 返回字符串长度 |
| `Size()` | Length的别名 |
| `Empty()` | 检查字符串是否为空 |
| `Capitalize()` | 将字符串首字母大写 |
| `Downcase()` | 将字符串转为小写 |
| `Upcase()` | 将字符串转为大写 |
| `Strip()` | 去除字符串两端的空白 |
| `Chomp()` | 去除字符串末尾的换行符 |
| `Include(substr)` | 检查字符串是否包含子串 |
| `Split(sep)` | 按照分隔符分割字符串 |
| `StartsWith(prefix)` | 检查字符串是否以指定前缀开始 |
| `EndsWith(suffix)` | 检查字符串是否以指定后缀结束 |
| `Reverse()` | 反转字符串 |
| `ReplaceAll(old, new)` | 替换字符串中的所有匹配项 |
| `Match(pattern)` | 检查字符串是否匹配指定正则表达式 |
| `Gsub(pattern, repl)` | 使用正则表达式进行全局替换 |
| `Count(substr)` | 计算指定字符串在当前字符串中出现的次数 |
| `Index(substr)` | 返回子字符串在当前字符串中第一次出现的位置 |
| `RIndex(substr)` | 返回子字符串在当前字符串中最后一次出现的位置 |
| `Slice(start, end)` | 返回指定范围的子字符串 |
| `SliceFrom(start)` | 返回从指定位置开始到字符串结尾的子字符串 |
| `Concat(other)` | 连接两个字符串并返回新字符串 |
| `Center(width, [padStr])` | 返回居中字符串，使用指定字符填充 |
| `Ljust(width, [padStr])` | 返回左对齐字符串，使用指定字符填充 |
| `Rjust(width, [padStr])` | 返回右对齐字符串，使用指定字符填充 |
| `Sub(pattern, repl)` | 使用正则表达式替换第一个匹配项 |
| `Ord()` | 返回字符串第一个字符的ASCII码值 |
| `Chars()` | 返回字符串中的所有字符组成的数组 |
| `Each(fn)` | 对字符串中的每个字符执行指定操作 |
| `EachLine(fn)` | 对字符串中的每一行执行指定操作 |
| `Times(n)` | 重复字符串指定次数 |
| `ToInt()` | 将字符串转换为整数 |
| `Inspect()` | 返回字符串的可打印形式（带引号） |
| `SwapCase()` | 交换字符串中字母的大小写 |
| `ToCamelCase()` | 转换字符串为驼峰命名 |
| `ToSnakeCase()` | 转换字符串为蛇形命名 |
```
