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

## 阶段 0.6：本次会话新增功能（2025-03-04）

### 新增
- [x] `?` 方法名解析 - `10.odd?` — 修改Lexer将`?`作为标识符的一部分
- [x] 三元运算符 `? :` — 修改Parser和Compiler支持
- [x] case/when 解析 — Parser支持（Compiler部分支持）
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
- [ ] `?` 方法名解析 - `10.odd?` — 已完成（2025-03-04）
- [ ] `? :` 三元运算符 — 已完成（2025-03-04）
- [ ] 字符串插值 `"hello #{name}"` — 运行时支持

### 已修复
- [x] 多参数表达式 `puts 1, 2, 3` — 已支持（作为方法参数）

## 新发现的问题（2025-03-04）

### 本次更新
- [x] `?` 方法名解析 - `10.odd?` - 已实现
- [x] 三元运算符 `? :` - 已实现
- [x] Stabby lambda `-> {}` - Parser已实现，Compiler部分实现（解析工作，调用需添加call方法）
- [x] MINUS_ARROW token 支持

### 已知问题
- case/when 解析存在无限循环问题
- case/when Compiler 实现不完整
- RangeExpression (1..5) 未实现
- ForExpression (for i in arr) 未实现
- RescueClause/BeginExpression 未完全实现
- 很多 AST 节点类型未在 Compiler 中实现
- Lambda/Proc 调用需要添加 call 方法

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
- [x] Lambda (stabby lambda `-> {}`) — Parser已实现
- [ ] 闭包和 block — 部分实现

## 阶段 4：Ruby Spec 对照 - 缺失功能清单

**基于 `vendor/ruby/spec/` 分析（Ruby 3.3+ 规范）**

RGo 当前状态：
- ✅ Lexer: 140+ tokens（覆盖完整）
- ✅ Parser: 40+ AST 节点（语法支持良好）
- ✅ Compiler: 80+ opcodes（编译能力强）
- ⚠️ Core 类: 仅 17/59 类，方法不完整
- ❌ 标准库: 几乎为空

### 4.1 核心类方法补全（优先级 P0-P1）

#### P0 - 必须实现（Enumerable 依赖）
- [ ] **Enumerable 模块** - 所有集合类的基础
  - `each`, `map`, `select`, `reject`, `find`, `reduce`, `any?`, `all?`, `none?`, `one?`
  - `sort`, `sort_by`, `group_by`, `partition`, `take`, `drop`, `first`, `last`
  - `min`, `max`, `min_by`, `max_by`, `count`, `sum`, `zip`

#### P0 - Array 核心方法（100+ spec 文件）
当前：19 个方法 | 需要：80+ 方法

**已实现**: `length`, `size`, `first`, `last`, `push`, `pop`, `empty?`, `join`, `reverse`, `[]`, `each`, `map`, `select`, `find`, `concat`, `delete_at`, `shift`, `unshift`, `sample`, `clear`, `include`

**缺失（高频）**:
- [ ] 修改: `<<`, `insert`, `fill`, `replace`, `compact`, `compact!`, `flatten`, `flatten!`, `uniq`, `uniq!`
- [ ] 查询: `index`, `rindex`, `count`, `empty?`, `include?`, `any?`, `all?`, `none?`
- [ ] 迭代: `each_index`, `each_with_index`, `map!`, `select!`, `reject`, `reject!`, `keep_if`, `delete_if`
- [ ] 转换: `to_h`, `to_s`, `inspect`, `to_a`, `pack`
- [ ] 集合: `&` (交集), `|` (并集), `-` (差集), `+` (连接)
- [ ] 排序: `sort`, `sort!`, `sort_by`, `shuffle`, `shuffle!`, `rotate`, `rotate!`
- [ ] 切片: `slice`, `slice!`, `take`, `take_while`, `drop`, `drop_while`, `values_at`
- [ ] 其他: `zip`, `transpose`, `product`, `permutation`, `combination`, `repeated_permutation`

#### P0 - String 核心方法（100+ spec 文件）
当前：25 个方法 | 需要：100+ 方法

**已实现**: `+`, `*`, `length`, `size`, `empty?`, `to_s`, `upcase`, `downcase`, `strip`, `[]`, `capitalize`, `include?`, `start_with?`, `end_with?`, `reverse`, `to_i`, `count`, `bytes`, `chars`, `find`, `slice`, `to_sym`, `ljust`, `rjust`, `center`

**缺失（高频）**:
- [ ] 修改: `upcase!`, `downcase!`, `capitalize!`, `swapcase`, `strip!`, `lstrip`, `rstrip`, `chomp`, `chop`, `delete`, `tr`, `squeeze`
- [ ] 查询: `index`, `rindex`, `scan`, `match`, `match?`, `=~`, `ord`, `chr`
- [ ] 替换: `sub`, `sub!`, `gsub`, `gsub!`, `replace`
- [ ] 分割: `split`, `lines`, `each_line`, `each_char`, `each_byte`, `codepoints`, `grapheme_clusters`
- [ ] 格式: `%` (格式化), `center`, `ljust`, `rjust`, `insert`, `concat`
- [ ] 转换: `to_i`, `to_f`, `to_sym`, `intern`, `hex`, `oct`, `unpack`
- [ ] 编码: `encoding`, `encode`, `force_encoding`, `valid_encoding?`, `ascii_only?`
- [ ] 其他: `succ`, `next`, `upto`, `sum`, `crypt`, `dump`, `inspect`

#### P0 - Hash 核心方法（69 spec 文件）
当前：17 个方法 | 需要：50+ 方法

**已实现**: `[]`, `[]=`, `keys`, `values`, `length`, `size`, `empty?`, `each`, `each_key`, `each_value`, `key?`, `has_key?`, `include?`, `fetch`, `merge`, `delete`, `clear`, `has_value?`

**缺失（高频）**:
- [ ] 修改: `merge!`, `update`, `delete_if`, `keep_if`, `select!`, `reject!`, `compact`, `compact!`
- [ ] 查询: `dig`, `fetch_values`, `value?`, `member?`
- [ ] 迭代: `each_pair`, `map`, `select`, `reject`, `filter`, `transform_keys`, `transform_values`
- [ ] 转换: `to_a`, `to_h`, `to_s`, `inspect`, `invert`, `flatten`
- [ ] 默认值: `default`, `default=`, `default_proc`, `default_proc=`
- [ ] 其他: `assoc`, `rassoc`, `shift`, `replace`, `compare_by_identity`, `rehash`

#### P0 - Integer 扩展方法
当前：20 个方法 | 需要：40+ 方法

**已实现**: `+`, `-`, `*`, `/`, `%`, `**`, `to_s`, `succ`, `pred`, `chr`, `odd?`, `even?`, `zero?`, `abs`, `to_f`, `times`, `upto`, `downto`, `gcd`, `lcm`, `divmod`

**缺失**:
- [ ] 位运算: `&`, `|`, `^`, `~`, `<<`, `>>`, `[]` (bit access)
- [ ] 数学: `ceil`, `floor`, `round`, `truncate`, `magnitude`, `remainder`, `fdiv`
- [ ] 查询: `positive?`, `negative?`, `finite?`, `infinite?`, `integer?`
- [ ] 转换: `to_i`, `to_int`, `to_r`, `digits`, `bit_length`, `size`
- [ ] 迭代: `step`, `next`, `pred`
- [ ] 其他: `coerce`, `numerator`, `denominator`, `rationalize`

#### P0 - Float 扩展方法
当前：11 个方法 | 需要：30+ 方法

**已实现**: `+`, `-`, `*`, `/`, `to_s`, `to_i`, `floor`, `ceil`, `round`, `abs`

**缺失**:
- [ ] 数学: `truncate`, `magnitude`, `fdiv`, `quo`, `remainder`, `divmod`, `modulo`
- [ ] 查询: `finite?`, `infinite?`, `nan?`, `zero?`, `positive?`, `negative?`
- [ ] 转换: `to_f`, `to_r`, `rationalize`, `numerator`, `denominator`
- [ ] 其他: `next_float`, `prev_float`, `coerce`

#### P0 - Kernel 模块（全局方法）
当前：约 10 个 | 需要：50+ 方法

**已实现**: `puts`, `print`, `p`, `gets`, `class`, `to_s`, `inspect`, `nil?`, `is_a?`, `respond_to?`

**缺失（高频）**:
- [ ] IO: `printf`, `putc`, `readline`, `readlines`, `getc`, `gets`
- [ ] 对象: `send`, `__send__`, `public_send`, `method`, `methods`, `instance_variables`
- [ ] 类型: `kind_of?`, `instance_of?`, `is_a?`, `respond_to?`, `respond_to_missing?`
- [ ] 转换: `Array()`, `Hash()`, `String()`, `Integer()`, `Float()`, `Rational()`, `Complex()`
- [x] 控制: `loop`, `catch`, `throw`, `fail`, `exit`, `abort`, `at_exit` (raise/catch/throw 已实现)
- [ ] 求值: `eval`, `instance_eval`, `class_eval`, `module_eval`, `binding`
- [ ] 加载: `require`, `require_relative`, `load`, `autoload`, `autoload?`
- [ ] 其他: `sleep`, `rand`, `srand`, `caller`, `caller_locations`, `warn`, `set_trace_func`

### 4.2 未实现的核心类（优先级 P1-P2）

#### P1 - 基础对象类型
- [x] **Symbol** - 已实现 (2026-03-22)
  - 方法: `to_s`, `to_sym`, `id2name`, `inspect`, `length`, `size`, `empty?`, `upcase`, `downcase`, `capitalize`, `swapcase`, `succ`, `next`, `==`, `===`, `[]`, `slice`
   
- [x] **Range** - 已实现 (2026-03-22)
  - 方法: `begin`, `end`, `exclude_end?`, `first`, `last`, `size`, `cover?`, `include?`, `member?`, `each`, `to_a`, `==`, `===`

- [ ] **Regexp** - 部分实现 (2026-03-22)
  - 方法: `to_s`, `source`, `=~`, `===`, `match`, `match?`, `==` 已实现
  - 缺失: `names`, `named_captures`, `options`, `casefold?`, `escape`, `union`, `try_convert`

#### P1 - 闭包和块
- [x] **Proc** - 已实现 (2026-03-22)
  - 方法: `call`, `[]`, `arity`, `lambda?`, `to_proc`, `to_s`, `inspect`, `===` 已实现
  - 缺失: `yield`, `parameters`, `binding`, `source_location`, `curry`, `<<`, `>>`

- [x] **Method** - 已实现 (2026-03-22)
  - 方法: `call`, `[]`, `arity`, `owner`, `receiver`, `name`, `to_s`, `inspect` 已实现
  - 缺失: `parameters`, `source_location`, `original_name`, `unbind`, `super_method`, `to_proc`

- [x] **Binding** - 已实现 (2026-03-22)
  - 方法: `local_variables`, `eval` 已实现
  - 缺失: `local_variable_get`, `local_variable_set`, `local_variable_defined?`, `receiver`, `source_location`

#### P2 - 异常系统（59 个 spec 文件）
- [x] **Exception** 基类及子类 - 已实现 (2026-03-22)
  - `Exception`, `StandardError`, `RuntimeError`, `ArgumentError`, `TypeError`, `NameError`, `NoMethodError`
  - `IndexError`, `KeyError`, `RangeError`, `ZeroDivisionError`, `SyntaxError`, `LoadError`
  - 方法: `message`, `backtrace`, `to_s`, `inspect`
- [x] begin/rescue/ensure 语法支持 - Parser/Compiler/VM 已实现基础支持 (2026-03-22)
- [ ] `backtrace_locations`, `cause`, `full_message`, `exception`, `set_backtrace` 方法未实现
- [ ] `IOError`, `EOFError`, `SystemCallError`, `Errno::*`, `SystemExit`, `SystemStackError` 等未实现
- [ ] rescue 修饰符 `foo rescue nil` 未实现
- [ ] 多重 rescue `rescue Error1, Error2` 未实现

#### P2 - 数值类型
- [ ] **Numeric** - 数值基类
  - 方法: `+@`, `-@`, `abs`, `magnitude`, `coerce`, `divmod`, `fdiv`, `modulo`, `remainder`, `quo`, `real?`, `integer?`, `zero?`, `nonzero?`, `finite?`, `infinite?`, `positive?`, `negative?`, `step`, `truncate`, `floor`, `ceil`, `round`

- [ ] **Rational** - 有理数
  - 方法: `numerator`, `denominator`, `to_i`, `to_f`, `to_r`, `to_s`, `inspect`, `+`, `-`, `*`, `/`, `**`, `%`, `divmod`, `fdiv`, `abs`, `magnitude`, `ceil`, `floor`, `round`, `truncate`

- [ ] **Complex** - 复数
  - 方法: `real`, `imaginary`, `imag`, `abs`, `magnitude`, `arg`, `angle`, `phase`, `conjugate`, `conj`, `polar`, `rect`, `rectangular`, `to_i`, `to_f`, `to_r`, `to_c`, `to_s`, `inspect`

#### P2 - 对象系统完善
- [ ] **Object** 方法补全
  - 缺失: `dup`, `clone`, `freeze`, `frozen?`, `taint`, `tainted?`, `untaint`, `trust`, `untrust`, `untrusted?`, `tap`, `then`, `yield_self`, `method_missing`, `singleton_class`, `singleton_methods`, `define_singleton_method`, `extend`, `instance_variable_get`, `instance_variable_set`, `instance_variable_defined?`, `remove_instance_variable`

- [ ] **Class** 方法补全
  - 缺失: `new`, `allocate`, `superclass`, `ancestors`, `included_modules`, `instance_methods`, `public_instance_methods`, `protected_instance_methods`, `private_instance_methods`, `constants`, `const_get`, `const_set`, `const_defined?`, `remove_const`, `class_variables`, `class_variable_get`, `class_variable_set`, `class_variable_defined?`, `remove_class_variable`

- [ ] **Module** 方法补全
  - 缺失: `include`, `prepend`, `extend`, `included`, `prepended`, `extended`, `module_function`, `attr_reader`, `attr_writer`, `attr_accessor`, `alias_method`, `undef_method`, `remove_method`, `define_method`, `method_defined?`, `public`, `private`, `protected`, `public_class_method`, `private_class_method`, `module_eval`, `class_eval`, `constants`, `const_missing`

#### P2 - 其他核心类
- [ ] **Struct** - 结构体类
- [ ] **Time** - 时间处理
- [ ] **Date** / **DateTime** - 日期处理
- [ ] **Set** - 集合类
- [ ] **Enumerator** - 枚举器
- [ ] **MatchData** - 正则匹配结果
- [ ] **Math** 模块 - 数学函数
- [ ] **Comparable** 模块 - 比较功能
- [ ] **Random** - 随机数生成器

#### P3 - IO 和文件系统
- [ ] **IO** - 输入输出基类
- [ ] **File** - 文件操作
- [ ] **Dir** - 目录操作
- [ ] **FileTest** - 文件测试
- [ ] **Encoding** - 字符编码

#### P3 - 并发（暂缓）
- [ ] **Thread** - 线程支持
- [ ] **Fiber** - 纤程
- [ ] **Mutex** - 互斥锁
- [ ] **Queue** / **SizedQueue** - 线程安全队列
- [ ] **ConditionVariable** - 条件变量

#### P3 - 系统和进程
- [ ] **Process** - 进程管理
- [ ] **Signal** - 信号处理
- [ ] **GC** - 垃圾回收控制
- [ ] **ObjectSpace** - 对象空间
- [ ] **Marshal** - 对象序列化
- [ ] **TracePoint** - 追踪点
- [ ] **Warning** - 警告处理

### 4.3 语言特性补全（60+ 规范文件）

#### 已实现 ✅
- [x] 方法定义 (`def`)
- [x] 类定义 (`class`)
- [x] 条件语句 (`if/elsif/else`)
- [x] 循环 (`while/until`)
- [x] 返回语句 (`return`)
- [x] 变量赋值（局部、实例、类、全局、常量）
- [x] 三元运算符 (`? :`)

#### 部分实现 ⚠️
- [ ] **case/when** - Parser 支持，Compiler 不完整
- [ ] **Lambda** - Parser 支持，运行时不完整
- [ ] **Block** - 部分支持，缺少 `yield` 和 `block_given?`
- [ ] **break/next** - AST 存在，部分未实现

#### 未实现 ❌

**P0 - 异常处理**
- [ ] `begin/rescue/ensure/raise` - 完整的异常处理机制
- [ ] `rescue` 修饰符 - `foo rescue nil`
- [ ] 异常类型匹配和多重 rescue

**P1 - 高频语法**
- [ ] `for` 循环 - AST 存在，Compiler 未实现
- [ ] `unless` - 反向条件（token 存在）
- [ ] `redo/retry` - 循环控制
- [ ] 条件修饰符 - `puts "hi" if true`
- [ ] 循环修饰符 - `i += 1 while i < 10`

**P1 - 模块系统**
- [ ] `module` 定义 - AST 存在，运行时未实现
- [ ] `include/extend/prepend` - 模块混入
- [ ] `module_function` - 模块函数

**P1 - 方法相关**
- [ ] `super` - 父类方法调用（token 存在）
- [ ] `yield` - 块调用（token 存在）
- [ ] `block_given?` - 块检测
- [ ] `alias/undef` - 方法别名和取消定义
- [ ] 方法可见性 - `public/private/protected`（token 存在，未强制）

**P2 - 高级语法**
- [ ] **字符串插值** - `"hello #{name}"` 运行时支持
- [ ] **Heredoc** - 多行字符串 `<<EOF`
- [ ] **正则表达式字面量** - `/pattern/flags`
- [ ] **Symbol 字面量** - `:symbol` 完整支持
- [ ] **Range 字面量** - `1..10`, `1...10` 完整支持

**P2 - 参数和赋值**
- [ ] **多重赋值** - `a, b = 1, 2`
- [ ] **并行赋值** - `a, b = b, a`
- [ ] **Splat 操作符** - `*args`, `**kwargs`
- [ ] **关键字参数** - `def foo(a:, b: 1)`
- [ ] **块参数** - `def foo(&block)`
- [ ] **默认参数** - `def foo(a = 1)`

**P3 - 高级特性**
- [ ] **模式匹配** - `case/in` (Ruby 3.0+)
- [ ] **安全导航** - `obj&.method`
- [ ] **BEGIN/END** - 程序开始/结束钩子
- [ ] **Singleton class** - 单例类 `class << obj`
- [ ] **常量解析** - `::` 完整支持
- [ ] **defined?** - 定义检查（token 存在）
- [ ] **Encoding 注释** - `# encoding: utf-8`

### 4.4 标准库（50+ 库，优先级 P3）

#### 数据格式
- [ ] `json` - JSON 解析和生成
- [ ] `csv` - CSV 处理
- [ ] `yaml` - YAML 处理
- [ ] `rexml` - XML 处理

#### 网络
- [ ] `net/http` - HTTP 客户端
- [ ] `net/ftp` - FTP 客户端
- [ ] `socket` - Socket 编程
- [ ] `uri` - URI 处理

#### Web
- [ ] `cgi` - CGI 支持
- [ ] `erb` - 模板引擎

#### 加密
- [ ] `openssl` - OpenSSL 绑定
- [ ] `digest` - 摘要算法（MD5, SHA1, SHA256 等）

#### 数学
- [ ] `matrix` - 矩阵运算
- [ ] `prime` - 质数
- [ ] `bigdecimal` - 高精度小数

#### 系统
- [ ] `fileutils` - 文件工具
- [ ] `pathname` - 路径名
- [ ] `tmpdir` - 临时目录
- [ ] `tempfile` - 临时文件

#### 文本
- [ ] `stringscanner` - 字符串扫描
- [ ] `strscan` - 字符串扫描

#### 工具
- [ ] `pp` - 美化打印
- [ ] `pstore` - 持久化存储
- [ ] `dbm` - DBM 数据库
- [ ] `logger` - 日志记录

### 4.5 实现优先级总结

**P0 - 核心基础（必须，3-6 个月）**
1. ✅ Enumerable 模块 - 所有集合依赖
2. ✅ Array 核心方法 - `each`, `map`, `select`, `reject`, `push`, `pop`, `shift`, `unshift`, `compact`, `flatten`, `uniq`, `sort` 等
3. ✅ String 核心方法 - `split`, `gsub`, `sub`, `scan`, `match`, `strip`, `chomp`, `upcase!`, `downcase!` 等
4. ✅ Hash 核心方法 - `merge`, `merge!`, `select`, `reject`, `dig`, `transform_keys`, `transform_values` 等
5. ✅ Kernel 方法 - `puts`, `print`, `p`, `raise`, `require`, `eval`, `loop`, `catch`, `throw`
6. ✅ Exception 系统 - `begin/rescue/ensure/raise` + 异常类层次结构

**P1 - 常用功能（重要，6-12 个月）**
7. Range 对象 - `1..10` 完整实现和迭代
8. Symbol 对象 - `:symbol` 完整实现
9. Regexp 对象 - 正则表达式匹配引擎
10. Block/Proc/Lambda - 完整的闭包支持和 `yield`
11. Module 系统 - `include/extend/prepend` 完整实现
12. IO/File - 基本文件读写操作
13. 字符串插值 - `"hello #{name}"` 运行时支持

**P2 - 高级功能（增强，12-18 个月）**
14. 多重赋值和 Splat - `a, b = 1, 2` 和 `*args`
15. 关键字参数 - `def foo(a:, b: 1)`
16. 方法可见性 - `public/private/protected` 强制
17. Time/Date 类 - 时间日期处理
18. Struct - 结构体类
19. Method/Binding 对象 - 方法对象化

**P3 - 标准库（扩展，18+ 个月）**
20. JSON 库 - `require 'json'`
21. Net::HTTP - HTTP 客户端
22. FileUtils - 文件工具
23. ERB - 模板引擎
24. Digest - 摘要算法

## 阶段 5：ruby/spec 测试集成

- [ ] 创建 MSpec 适配器，运行 ruby/spec 测试
- [ ] 针对已实现功能运行对应的 spec 文件
- [ ] 逐步增加通过的 spec 数量
- [ ] 目标：通过 core/array, core/string, core/hash, core/integer 基础测试

## 阶段 6：Rails 兼容层（远期目标，24+ 个月）

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
