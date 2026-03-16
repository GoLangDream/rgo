# 阶段 3: String 核心方法补全技术设计

**优先级**: P0  
**预计时间**: 2 周  
**依赖**: 阶段 1 (Enumerable 模块)  
**被依赖**: 阶段 7 (Regexp), 阶段 10 (Rails)

## 目标

将 String 方法覆盖率从当前的 25% 提升到 80%+，实现 70+ 核心方法，通过 vendor/ruby/spec/core/string/ 下 80%+ 的 spec。

## 当前状态

### 已实现 (25 个方法)
- 基础: `length`, `size`, `empty?`, `to_s`
- 修改: `+`, `*`, `upcase`, `downcase`, `strip`, `capitalize`, `reverse`
- 访问: `[]`, `slice`
- 查询: `include?`, `start_with?`, `end_with?`
- 转换: `to_i`, `to_sym`, `bytes`, `chars`
- 格式: `ljust`, `rjust`, `center`
- 其他: `count`, `find`

### 需要实现 (70+ 个方法)

#### P0 - 高频方法 (第 1 周)
**修改方法 (! 版本)**:
- `upcase!`, `downcase!`, `capitalize!`, `swapcase!`
- `strip!`, `lstrip!`, `rstrip!`
- `chomp!`, `chop!`
- `delete!`, `tr!`, `squeeze!`
- `reverse!`

**查询方法**:
- `index(substring, offset=0)` - 查找子串位置
- `rindex(substring, offset=nil)` - 反向查找
- `scan(pattern)` - 扫描匹配
- `match(pattern)` - 正则匹配 (需要 Regexp)
- `match?(pattern)` - 是否匹配
- `=~(pattern)` - 匹配操作符
- `ord` - 第一个字符的编码
- `chr` - 返回第一个字符

**替换方法**:
- `sub(pattern, replacement)` - 替换第一个匹配
- `sub!(pattern, replacement)` - 原地替换第一个
- `gsub(pattern, replacement)` - 替换所有匹配
- `gsub!(pattern, replacement)` - 原地替换所有
- `replace(other_str)` - 完全替换

#### P1 - 重要方法 (第 2 周)
**分割方法**:
- `split(pattern=nil, limit=nil)` - 分割字符串
- `lines(separator=$/)` - 按行分割
- `each_line` - 迭代每一行
- `each_char` - 迭代每个字符
- `each_byte` - 迭代每个字节
- `codepoints` - 返回码点数组
- `grapheme_clusters` - 返回字形簇

**格式方法**:
- `%` - 格式化操作符
- `format` - 格式化
- `insert(index, other_str)` - 插入字符串
- `concat(*args)` - 连接字符串
- `prepend(other_str)` - 前置字符串

**转换方法**:
- `to_f` - 转换为浮点数
- `intern` - 转换为 Symbol (to_sym 的别名)
- `hex` - 十六进制转整数
- `oct` - 八进制转整数
- `unpack(template)` - 解包二进制

**编码方法**:
- `encoding` - 返回编码
- `encode(encoding)` - 转换编码
- `force_encoding(encoding)` - 强制编码
- `valid_encoding?` - 是否有效编码
- `ascii_only?` - 是否只包含 ASCII

**其他方法**:
- `succ` / `next` - 后继字符串
- `upto(max_str)` - 迭代到指定字符串
- `sum(n=16)` - 校验和
- `crypt(salt)` - 加密
- `dump` - 转义表示
- `inspect` - 调试表示

## 技术设计

### 1. 修改方法 (! 版本)

#### 1.1 设计原则

Ruby 中的修改方法遵循以下规则:
- 方法名以 `!` 结尾
- 直接修改原字符串
- 如果没有修改，返回 `nil`
- 如果有修改，返回 `self`

#### 1.2 实现模式

```go
// 非修改版本
arrayClass.DefineMethod("upcase", &object.Method{
    Name:  "upcase",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return receiver
        }
        
        result := strings.ToUpper(str)
        
        return &object.EmeraldValue{
            Type:  object.ValueString,
            Data:  result,
            Class: R.Classes["String"],
        }
    },
})

// 修改版本
stringClass.DefineMethod("upcase!", &object.Method{
    Name:  "upcase!",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return R.NilVal
        }
        
        result := strings.ToUpper(str)
        
        // 如果没有变化，返回 nil
        if result == str {
            return R.NilVal
        }
        
        // 修改原字符串
        receiver.Data = result
        return receiver
    },
})
```

#### 1.3 chomp / chomp!

```go
stringClass.DefineMethod("chomp", &object.Method{
    Name:  "chomp",
    Arity: -1,  // 可选参数
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return receiver
        }
        
        separator := "\n"  // 默认分隔符
        if len(args) > 0 {
            if sep, ok := args[0].Data.(string); ok {
                separator = sep
            }
        }
        
        result := str
        if separator == "" {
            // 移除所有尾部换行符
            result = strings.TrimRight(str, "\r\n")
        } else if strings.HasSuffix(str, separator) {
            result = str[:len(str)-len(separator)]
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueString,
            Data:  result,
            Class: R.Classes["String"],
        }
    },
})
```

### 2. 查询方法

#### 2.1 index / rindex

```go
stringClass.DefineMethod("index", &object.Method{
    Name:  "index",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok || len(args) == 0 {
            return R.NilVal
        }
        
        substring, ok := args[0].Data.(string)
        if !ok {
            return R.NilVal
        }
        
        offset := 0
        if len(args) > 1 {
            if off, ok := args[1].Data.(int64); ok {
                offset = int(off)
            }
        }
        
        // 处理负偏移
        if offset < 0 {
            offset = len(str) + offset
        }
        
        if offset < 0 || offset >= len(str) {
            return R.NilVal
        }
        
        index := strings.Index(str[offset:], substring)
        if index == -1 {
            return R.NilVal
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueInteger,
            Data:  int64(offset + index),
            Class: R.Classes["Integer"],
        }
    },
})

stringClass.DefineMethod("rindex", &object.Method{
    Name:  "rindex",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok || len(args) == 0 {
            return R.NilVal
        }
        
        substring, ok := args[0].Data.(string)
        if !ok {
            return R.NilVal
        }
        
        offset := len(str)
        if len(args) > 1 {
            if off, ok := args[1].Data.(int64); ok {
                offset = int(off)
            }
        }
        
        // 处理负偏移
        if offset < 0 {
            offset = len(str) + offset
        }
        
        if offset < 0 {
            return R.NilVal
        }
        if offset > len(str) {
            offset = len(str)
        }
        
        index := strings.LastIndex(str[:offset], substring)
        if index == -1 {
            return R.NilVal
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueInteger,
            Data:  int64(index),
            Class: R.Classes["Integer"],
        }
    },
})
```

#### 2.2 scan

```go
stringClass.DefineMethod("scan", &object.Method{
    Name:  "scan",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok || len(args) == 0 {
            return receiver
        }
        
        pattern, ok := args[0].Data.(string)
        if !ok {
            // TODO: 支持 Regexp 对象
            return R.NilVal
        }
        
        // 简单的字符串扫描 (非正则)
        result := make([]*object.EmeraldValue, 0)
        index := 0
        for index < len(str) {
            pos := strings.Index(str[index:], pattern)
            if pos == -1 {
                break
            }
            
            match := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  pattern,
                Class: R.Classes["String"],
            }
            
            if block != nil {
                CallBlock(block, match)
            } else {
                result = append(result, match)
            }
            
            index += pos + len(pattern)
        }
        
        if block != nil {
            return receiver
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

### 3. 替换方法

#### 3.1 sub / gsub (字符串版本)

```go
stringClass.DefineMethod("sub", &object.Method{
    Name:  "sub",
    Arity: 2,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok || len(args) < 2 {
            return receiver
        }
        
        pattern, ok1 := args[0].Data.(string)
        replacement, ok2 := args[1].Data.(string)
        if !ok1 || !ok2 {
            // TODO: 支持 Regexp 和 block
            return receiver
        }
        
        // 替换第一个匹配
        result := strings.Replace(str, pattern, replacement, 1)
        
        return &object.EmeraldValue{
            Type:  object.ValueString,
            Data:  result,
            Class: R.Classes["String"],
        }
    },
})

stringClass.DefineMethod("gsub", &object.Method{
    Name:  "gsub",
    Arity: 2,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok || len(args) < 2 {
            return receiver
        }
        
        pattern, ok1 := args[0].Data.(string)
        replacement, ok2 := args[1].Data.(string)
        if !ok1 || !ok2 {
            return receiver
        }
        
        // 替换所有匹配
        result := strings.ReplaceAll(str, pattern, replacement)
        
        return &object.EmeraldValue{
            Type:  object.ValueString,
            Data:  result,
            Class: R.Classes["String"],
        }
    },
})
```

### 4. 分割方法

#### 4.1 split

```go
stringClass.DefineMethod("split", &object.Method{
    Name:  "split",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return receiver
        }
        
        var parts []string
        
        if len(args) == 0 {
            // 默认按空白分割
            parts = strings.Fields(str)
        } else {
            separator, ok := args[0].Data.(string)
            if !ok {
                // TODO: 支持 Regexp
                return receiver
            }
            
            limit := -1
            if len(args) > 1 {
                if lim, ok := args[1].Data.(int64); ok {
                    limit = int(lim)
                }
            }
            
            if separator == "" {
                // 分割为单个字符
                parts = strings.Split(str, "")
            } else {
                if limit < 0 {
                    parts = strings.Split(str, separator)
                } else {
                    parts = strings.SplitN(str, separator, limit)
                }
            }
        }
        
        // 转换为 EmeraldValue 数组
        result := make([]*object.EmeraldValue, len(parts))
        for i, part := range parts {
            result[i] = &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  part,
                Class: R.Classes["String"],
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

#### 4.2 lines / each_line

```go
stringClass.DefineMethod("lines", &object.Method{
    Name:  "lines",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return receiver
        }
        
        separator := "\n"
        if len(args) > 0 {
            if sep, ok := args[0].Data.(string); ok {
                separator = sep
            }
        }
        
        var lines []string
        if separator == "" {
            // 每个字符一行
            lines = strings.Split(str, "")
        } else {
            lines = strings.Split(str, separator)
        }
        
        result := make([]*object.EmeraldValue, len(lines))
        for i, line := range lines {
            result[i] = &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  line,
                Class: R.Classes["String"],
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})

stringClass.DefineMethod("each_line", &object.Method{
    Name:  "each_line",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if block == nil {
            // TODO: 返回 Enumerator
            return receiver
        }
        
        str, ok := receiver.Data.(string)
        if !ok {
            return receiver
        }
        
        separator := "\n"
        if len(args) > 0 {
            if sep, ok := args[0].Data.(string); ok {
                separator = sep
            }
        }
        
        lines := strings.Split(str, separator)
        for _, line := range lines {
            lineVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  line,
                Class: R.Classes["String"],
            }
            CallBlock(block, lineVal)
        }
        
        return receiver
    },
})
```

### 5. 转换方法

#### 5.1 to_f / hex / oct

```go
import "strconv"

stringClass.DefineMethod("to_f", &object.Method{
    Name:  "to_f",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return &object.EmeraldValue{
                Type:  object.ValueFloat,
                Data:  0.0,
                Class: R.Classes["Float"],
            }
        }
        
        f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
        if err != nil {
            f = 0.0
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueFloat,
            Data:  f,
            Class: R.Classes["Float"],
        }
    },
})

stringClass.DefineMethod("hex", &object.Method{
    Name:  "hex",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return &object.EmeraldValue{
                Type:  object.ValueInteger,
                Data:  int64(0),
                Class: R.Classes["Integer"],
            }
        }
        
        // 移除 0x 前缀
        str = strings.TrimPrefix(strings.TrimSpace(str), "0x")
        str = strings.TrimPrefix(str, "0X")
        
        i, err := strconv.ParseInt(str, 16, 64)
        if err != nil {
            i = 0
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueInteger,
            Data:  i,
            Class: R.Classes["Integer"],
        }
    },
})

stringClass.DefineMethod("oct", &object.Method{
    Name:  "oct",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        str, ok := receiver.Data.(string)
        if !ok {
            return &object.EmeraldValue{
                Type:  object.ValueInteger,
                Data:  int64(0),
                Class: R.Classes["Integer"],
            }
        }
        
        // 移除 0o 前缀
        str = strings.TrimPrefix(strings.TrimSpace(str), "0o")
        str = strings.TrimPrefix(str, "0O")
        str = strings.TrimPrefix(str, "0")
        
        i, err := strconv.ParseInt(str, 8, 64)
        if err != nil {
            i = 0
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueInteger,
            Data:  i,
            Class: R.Classes["Integer"],
        }
    },
})
```

## 实施计划

### 第 1 周: P0 高频方法

**Day 1-2: 修改方法**
- [ ] 实现所有 ! 版本方法
- [ ] 实现 chomp/chomp!, chop/chop!
- [ ] 实现 delete!, tr!, squeeze!
- [ ] 测试: 修改方法

**Day 3-4: 查询和替换**
- [ ] 实现 index, rindex, scan
- [ ] 实现 sub/sub!, gsub/gsub!
- [ ] 实现 replace
- [ ] 测试: 查询和替换

**Day 5: 集成测试**
- [ ] 运行 string/ 下的 spec
- [ ] 修复发现的问题

### 第 2 周: P1 重要方法

**Day 6-7: 分割和格式**
- [ ] 实现 split, lines, each_line
- [ ] 实现 each_char, each_byte
- [ ] 实现 %, format, insert
- [ ] 测试: 分割和格式

**Day 8-9: 转换和编码**
- [ ] 实现 to_f, hex, oct
- [ ] 实现 encoding, encode
- [ ] 实现 valid_encoding?, ascii_only?
- [ ] 测试: 转换和编码

**Day 10: 其他方法和验收**
- [ ] 实现 succ, upto, sum
- [ ] 实现 dump, inspect
- [ ] 运行完整 spec 套件
- [ ] 性能优化

## 验收标准

### 功能验收
- [ ] 实现 70+ String 方法
- [ ] 所有修改方法 (!) 正确修改原字符串
- [ ] 所有非修改方法返回新字符串
- [ ] 支持负索引
- [ ] 支持 block 参数

### 测试验收
- [ ] 通过 vendor/ruby/spec/core/string/ 下 80%+ spec
- [ ] 所有 P0 方法 100% 通过
- [ ] 所有 P1 方法 80%+ 通过

### 性能验收
- [ ] 基础操作 (+, [], length) 与 Go string 性能相当
- [ ] split 性能与 strings.Split 相当
- [ ] 无内存泄漏

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
