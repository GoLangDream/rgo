# RGo 运行状况分析总结

## 执行状态：部分可用 ⚠️

RGo **可以运行基础 Ruby 代码**，但 **Enumerable 方法（map, select 等）不工作**。

## ✅ 能工作的功能

### 核心语言特性
- 变量和常量
- 算术和逻辑运算符
- 控制流：if/else, while, for, case/when
- 方法定义和调用（def, return, 参数）
- 类定义和继承（class, initialize）
- Block 语法解析（`do...end` 和 `{ }`）

### 数据结构
- Array: 创建、索引访问、length, first, last, push, pop
- Hash: 创建、键访问、keys, values, size
- String: upcase, downcase, capitalize, strip, reverse, split, include?
- Integer: 算术运算、比较、even?, odd?, times
- Float: 算术运算、比较

### 示例可运行代码
```ruby
# 变量和运算
x = 10
y = 20
puts x + y  # 输出 30

# 控制流
if x < y
  puts "x is less"
end

# 方法
def add(a, b)
  a + b
end
puts add(5, 3)  # 输出 8

# 类
class Person
  def initialize(name)
    @name = name
  end
end
p = Person.new("Alice")
```

## ❌ 不工作的功能

### 关键问题：Enumerable 方法不调用 block

**问题**：Array#map, select, reject, find 等方法是占位符实现，不调用传入的 block。

```ruby
# 期望：[2, 4, 6]
# 实际：[1, 2, 3]
[1,2,3].map { |n| n * 2 }

# 期望：[2, 3]
# 实际：[1, 2, 3]
[1,2,3].select { |n| n > 1 }
```

**根本原因**：
1. VM 的 OpSend 忽略 blockArg（line 443: `_ = blockArg`）
2. Block 没有传递给方法
3. 方法实现是占位符（只复制数组）

### 其他缺失功能

1. **字符串插值**：`"Hello #{name}"` 不工作
2. **Hash block 解构**：`{a:1}.each { |k,v| ... }` 参数不解构
3. **方法链 + 索引**：`h.keys[0]` 解析错误（解析为方法调用）

## 🔴 关键修复优先级

### P0 - 修复 Enumerable 方法（阻塞 40%+ specs）

**需要修复的方法**：
- Array: map, select, reject, find, reduce
- Hash: map, select, reject, each (with destructuring)

**修复步骤**：
1. 修改 VM OpSend：从栈弹出 block 并传递给方法
2. 重写 arrayMap：调用 block 并收集结果
3. 重写 arraySelect：调用 block 并过滤
4. 重写 arrayReject, arrayFind 等

**预期影响**：
- Array specs: 40% → 70%+
- Hash specs: 15% → 50%+

### P1 - 字符串插值（阻塞 20%+ String specs）

在 parser 中添加 `#{}` 支持。

### P2 - Hash block 参数解构

修复 `{a:1}.each { |k,v| ... }` 的参数传递。

## 📊 Spec 测试结果

当前通过率（前 20 个 specs）：
- Array: 8/20 (40%)
- String: 11/20 (55%)
- Hash: 3/20 (15%)
- Integer: 6/20 (30%)
- Float: 6/20 (30%)

**注意**：真实通过率可能更低，因为许多 specs 使用 MSpec DSL（describe/it），需要完整的 block 支持。

## 🎯 立即行动项

1. **修复 VM block 传递**（pkg/vm/executor.go line 441-453）
2. **重写 Array#map**（pkg/core/init.go line 1666）
3. **重写 Array#select**（pkg/core/init.go line 1679）
4. **测试并提交**
5. **运行 specs 验证改进**

## 结论

RGo **基础可用但不完整**。核心语言特性工作正常，但 **Enumerable 方法（Ruby 最常用的功能）完全不工作**。修复 block 传递和 Enumerable 方法实现是让 RGo 真正可用的关键。

预计修复后，spec 通过率可从当前的 30-40% 提升到 60-70%。
