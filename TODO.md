# RGo 待办事项

## 阶段 0：补测试（已完成）

按依赖顺序从底层往上补：

- [x] `pkg/lexer/` 单元测试 — 32 tests PASS
- [x] `pkg/object/` 单元测试 — 16 tests PASS
- [x] `pkg/vm/` 端到端测试 — 59 tests PASS（新增 38 个控制流测试）
- [x] `pkg/parser/` 单元测试 — 45 tests PASS
- [x] `pkg/compiler/` 单元测试 — 31 tests PASS
- [x] `pkg/core/` 测试 — 62 tests PASS

**项目总计：247 个测试，全部通过**

## 阶段 0.6：本次会话新增功能

### 新增
- [x] class 定义 — 基本解析和编译支持
- [x] 实例变量 (@foo) — 解析和赋值支持（部分）
- [x] 常量 — CONSTANT token 类型支持（大写字母开头）
- [x] def 方法定义和调用 — 端到端验证
- [x] return 语句 — 基本功能验证
- [x] if/elsif/else — 完整支持
- [x] while/until 循环 — 完整支持

### 修复
- [x] CONSTANT token 类型识别
- [x] parseClassExpression 实现
- [x] InstanceVariable AST 节点编译支持
- [x] OpGetInstanceVar/OpSetInstanceVar VM 实现
- [x] parseAssignExpression 支持 InstanceVariable
- [x] 方法调用栈管理 (Bp 计算、fp 管理)
- [x] 方法返回值处理 (OpReturnValue)

## 阶段 0.5：已修复的 bug

### 项目结构整理
- [x] 重组目录结构 — 将 `rvm/` 和 `vm/` 合并为 `pkg/` 统一目录
- [x] 删除无用文件 — scripts/、rgo 二进制、10 个过时文档
- [x] 清理 debug 输出 — main.go、executor.go
- [x] 清理依赖 — go.mod 零外部依赖
- [x] 精简 Makefile

### Lexer 修复（通过补测试发现并修复）
- [x] `readChar()` EOF 时不更新 position — 导致所有标识符/数字/符号丢失最后一个字符
- [x] `NextToken()` 末尾多余 readChar — 对已推进位置的分支多吃一个字符
- [x] 行内注释 `#` 未被跳过 — `a # comment` 中 `#` 后的内容被当作 token
- [x] 双引号字符串转义序列偏移 — `"hello\nworld"` 产生错误结果
- [x] 全局变量 `$` 解析偏移 — `$stdout` 变成 `$$stdout`
- [x] 字符串插值 `#{}` 解析错误 — `"hello #{name}"` 多包含了 `{` 和 `"`
- [x] `[]` 被当作单个 token — 空数组 `[]` 无法解析，改为始终拆分为 `[` 和 `]`

### Parser 修复（通过补测试发现并修复）
- [x] `==` 未注册到优先级表 — `1 == 2` 无法解析为中缀表达式
- [x] `!=`（BANG_EQUAL）未注册为中缀运算符 — `1 != 2` 无法解析
- [x] `&&` 和 `||`（AND/OR）未注册到优先级表和中缀处理 — 逻辑表达式无法解析
- [x] `parseArrayLiteral` 无限循环 — 使用 cur-based 循环但 parseExpression 不推进 curToken，改为 peek-based
- [x] `parseCallExpression` 无限循环 + 未检查类型断言 — 同上，且 `fn.(*ast.Identifier)` 会 panic
- [x] `parseMethodCall` 参数循环无限循环 + 错误的 expectPeek — 同上
- [x] `parseHashLiteral` 与中缀 COLON 处理冲突 — 重写为支持 symbol shorthand `{a: 1}` 和 hash rocket `{"a" => 1}`
- [x] `parseIfExpression` expectPeek 副作用 — `expectPeek(THEN)` 在输入用换行时添加错误，改为先检查 peekTokenIs
- [x] `parseIfExpression` NEWLINE 处理 — body 循环中 NEWLINE 被当作表达式解析，添加 `skipCurNewlines` 辅助函数
- [x] `parseWhileExpression` body 无限循环 — 同 parseIfExpression 的 NEWLINE 处理问题
- [x] `parseUntilExpression` body 无限循环 — 同上
- [x] `parseDefExpression` body 无限循环 — 同上
- [x] `parseDefExpression` 参数解析无限循环 — 改为 peek-based 模式
- [x] 清理 `parseExpression` 中的 debug println 和 os.Stderr 输出

### Compiler 修复（端到端测试驱动）
- [x] `&&` 和 `||` 短路求值 — 使用 OpDup + OpJumpTruthy/OpJumpNotTruthy 实现
- [x] `if/elsif/else` 跳转逻辑错误 — 修复 jump 指令回填，添加 elsif 支持
- [x] `BlockExpression` 作为值 — 添加 `compileBlockAsValue` 辅助方法，仅用于 if 分支
- [x] `if` 无 else 时栈下溢 — 条件为 false 时推 nil
- [x] `while` 循环栈下溢 — 循环结束后推 nil（Ruby while 返回 nil）
- [x] `until` 循环支持 — 使用 OpJumpTruthy（条件为真时跳出）

### VM 修复
- [x] `>=` 运算符逻辑错误 — `5 >= 5` 返回 false（只检查了 > 没检查 ==）
- [x] `<=` 运算符逻辑错误 — 同上

## 阶段 1：修复解析器 bug（已完成）

### 低优先级（需要Lexer修改）
- [ ] `?` 方法名解析 - `10.odd?` — 需要修改Lexer将`?`作为标识符的一部分
- [ ] `? :` 三元运算符 — 需要修改Lexer和Parser
- [ ] 字符串插值 `"hello #{name}"` — 运行时支持

### 已修复
- [x] 多参数表达式 `puts 1, 2, 3` — 已支持（作为方法参数）

## 阶段 2：控制流和方法定义（已完成）

- [x] if/elsif/else — 端到端验证通过（9 个测试）
- [x] while 循环 — 端到端验证通过（3 个测试）
- [x] until 循环 — 端到端验证通过（3 个测试）
- [x] def 方法定义（带参数）— 端到端验证通过（7 个测试）
- [x] return 语句 — 基本功能验证通过（return 5 等）
  - ⚠️ 注意：return 后的 unreachable code 有问题

## 阶段 3：对象系统（已完成）

- [x] class 定义 — 基本解析和编译支持已实现
- [x] 实例变量 (@foo) — 解析和赋值支持
- [x] 类变量 (@@foo) — 解析和赋值支持
- [x] 全局变量 ($foo) — 解析和赋值支持
- [ ] 闭包和 block — 需要实现

## 阶段 4：更多内置类型和标准库

- [ ] Symbol, Regexp, Range
- [ ] Proc, Lambda
- [ ] IO, File
- [ ] Exception
- [ ] 标准库 (Marshal, JSON, Time, Date)
- [ ] Kernel#require/load/eval

## 阶段 5：ruby/spec 测试集成

- [ ] 克隆 ruby/spec，创建测试运行器适配器

## 阶段 6：Rails 兼容层

- [ ] ActionDispatch, ActionController, ActiveRecord, ActionView

---

## 已验证正常的功能（端到端测试）

### 基础运算
- [x] 整数运算: `1 + 2`, `10 - 5`, `3 * 4`, `10 / 3`
- [x] 模运算: `17 % 5` (= 2)
- [x] 幂运算: `2 ** 10` (= 1024)
- [x] 浮点数: `1.5`, `1.5 + 2.5`, `1 + 1.5`
- [x] 运算符优先级: `2 + 3 * 4` = 14

### 字符串
- [x] 字符串连接: `"hello" + " " + "world"`
- [x] 字符串索引: `"hello"[0]` = "h"

### 比较和逻辑
- [x] 比较运算: `10 > 5`, `3 < 7`, `5 >= 5`, `5 <= 10`
- [x] 等值比较: `1 == 1`, `1 == 2`, `1 != 2`
- [x] 逻辑运算: `true && false`, `true || false`
- [x] 逻辑短路: `false && x` 不求值 x, `true || x` 不求值 x
- [x] 逻辑返回值: `1 && 2` = 2, `nil || 42` = 42

### 前缀运算符
- [x] 负号: `-5`
- [x] 逻辑非: `!true`, `!false`

### 变量
- [x] 变量赋值和引用: `x = 5; x + 3`
- [x] 多变量: `a = 10; b = 20; a + b`

### 字面量
- [x] 布尔字面量: `true`, `false`
- [x] nil 字面量: `nil`
- [x] 数组字面量: `[]`, `[1]`, `[1, 2, 3]`
- [x] 哈希字面量: `{}`, `{a: 1}`, `{"a" => 1}`

### 控制流
- [x] if 语句: `if true\n  5\nend`
- [x] if/else: `if false\n  1\nelse\n  2\nend`
- [x] if/elsif/else: `if x > 10\n  1\nelsif x > 5\n  2\nelse\n  3\nend`
- [x] if 条件: `if x > 5 && x < 10`
- [x] while 循环: `while x < 5\n  x = x + 1\nend`
- [x] while 求和: `sum = 0; i = 1; while i <= 10\n  sum = sum + i; i = i + 1\nend`
- [x] until 循环: `until x >= 5\n  x = x + 1\nend`
- [x] until 求和: `sum = 0; i = 1; until i > 10\n  sum = sum + i; i = i + 1\nend`

### 方法调用
- [x] 函数调用: `puts()`, `puts(1, 2)`
- [x] 方法调用: `"hello".upcase`, `"hello".slice(0, 3)`
