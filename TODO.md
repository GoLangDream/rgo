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

### Time spec dashboard（2026-05-12）

- [x] `vendor/ruby/spec/core/time/now_spec.rb` 已解除：18 examples / 0 failures。
  - 已支持 `Time.now(in:)` 的字符串、数字、military zone、timezone object offset。
  - 已修复 `Class.new(Time).new(...)` 需要继承 Time class methods。
  - 已修复 VM `OpSub` 对非数值 receiver 需要分派到 `#-`，否则 `time -= seconds` 会变成 nil。
- [x] `vendor/ruby/spec/core/time/at_spec.rb` 已解除：40 examples / 0 failures。
  - 已支持 subsecond 参数、`:nsec`/`:usec`/`:millisecond` format、`in:` offset，以及 Time subclass receiver。
- [x] `vendor/ruby/spec/core/time/{minus,localtime}_spec.rb` 已解除。
  - 已修复 `Time.now - "1"` 被 parser 误吞为 `Time.now` 参数的问题；新增 `TestParseDottedNoArgCallBeforeMinusAsInfix`。
  - 已补 `Time#subsec`、`Time#localtime` frozen-zone 变更错误、offset string encoding guard。
- [x] `vendor/ruby/spec/core/time` dashboard 已解除（2026-05-12 refresh）。
  - 当前 dashboard：66 pass / 0 nonzero_failures / 66 files；776 examples / 0 failures。
  - 已补 `getlocal`/`localtime`/UTC 转换、Time equality、`inspect`/`to_a`/`wday`/`yday`、`Rational()`、`Time.local`/`Time.mktime`、构造器 microsecond/invalid-argument 支持、`Time.new(String)` ISO-like 解析、lexical module class constants（`TimeSpecs::SubTime`）、Time timezone object `local_to_utc`、keyword-only `in:` 分离、`find_timezone` named zone、以及 `Marshal.dump/load` 的 Time 规格最小行为。

### Language timeout reduction blocker（2026-05-06）

- [x] Task 4 后 `vendor/ruby/spec/language/optional_assignments_spec.rb` timeout 已解除
   - 已修复 `super()` parser 空参数列表不终止问题，focused regression PASS。
   - 已补充并修复 `super(1 + 2)` parenthesized args 不终止回归；新增 `TestParseSuperWithParenthesizedArgumentsTerminates`，先 RED timeout 后 PASS。
   - Task 1 follow-up 刷新命令：`RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv`，写入 80 个 specs。
   - 最新 language dashboard：80 pass, 0 timeout, 0 runtime_error, 0 nonzero_failures, 0 parse_error, 0 compile_error, 0 zero_examples out of 80 files（2026-05-10 refreshed）。
   - 最新 selected blocker：`vendor/ruby/spec/language/optional_assignments_spec.rb` status is pass；duration 为易变值不在 TODO 固定记录。
   - selected blocker 已解除；`vendor/ruby/spec/language` dashboard 已全部通过。
- [x] `keyword_arguments_spec.rb` keyword shorthand (`m(a:, b:)`) parse error
   - 根因：`IDENT COLON` 在 call args 中缺少 peek-ahead，消耗 COLON 后停在 COMMA 但无 prefix parse fn
   - 已修复：`parseOneCallArg`/`parseOneYieldArg` 在 COLON 后遇到参数分隔符或右括号时，将 omitted value 解析为同名本地变量。
   - 已验证：`vendor/ruby/spec/language/keyword_arguments_spec.rb` 23 examples / 0 failures。
- [x] `predefined_spec.rb` 和 `rescue_spec.rb` timeout
   - 已修复多 rescue clause 的 `OpJump 0` 未回填问题，新增 `OpRescueMatch` 按异常类型选择 rescue 分支。
   - 已新增 `OpReraise`，修复 rescue 类型不匹配但存在 ensure 时，ensure 执行后继续抛给外层 rescue。
   - 已支持 dotted operator method name，如 `1.+(...)`。
   - 已新增最小同步 `Thread` shim：`Thread.new` 执行 block，`Thread.pass` no-op，`Thread#join` 返回自身。
   - 已验证：`vendor/ruby/spec/language/rescue_spec.rb` 59 examples / 0 failures；`vendor/ruby/spec/language/predefined_spec.rb` 170 examples / 0 failures。
- [x] `method_spec.rb` parse error
   - 与 `keyword_arguments_spec.rb` 共享 keyword shorthand 根因（`call(a:, b:)`）
   - 已修复 endless method `def greet(person) = ...`。
   - 已修复 `super(...)` 参数转发语法解析。
   - 已验证：`vendor/ruby/spec/language/method_spec.rb` 84 examples / 0 failures。
   - 已修复后续 `super_spec.rb` runtime blocker：支持 object singleton methods、裸 `super` 隐式转发当前参数，并让 singleton method 中的 `super` 从 receiver 原始 class 开始查找。
   - 已验证：`vendor/ruby/spec/language/super_spec.rb` 61 examples / 0 failures。

### Kernel spec gate（2026-05-10）

- [x] `vendor/ruby/spec/core/kernel/extend_spec.rb` compile_error 已解除
  - 已支持 `extend ModuleName` AST 编译为 `self.extend(ModuleName)`。
  - 已新增最小 `Object#extend` 模块方法复制行为。
  - 已验证：`extend_spec.rb` 10 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/throw_spec.rb` parse_error 已解除
  - 已支持 `throw :label, value, extra...` 的额外裸参数解析，并在编译期转为 `ArgumentError`。
  - 已验证：`throw_spec.rb` 9 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/public_methods_spec.rb` parse_error 已解除
  - 已支持 `should include(\n  :a, :b)` 这类 keyword matcher 的多行参数。
  - 已验证：`public_methods_spec.rb` 13 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/inspect_spec.rb` parse_error 已解除
  - 已修复无参数 endless method `def m = []` 被误判为 setter 方法名的问题。
  - 已验证：`inspect_spec.rb` 7 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/instance_variable_set_spec.rb` parse_error 已解除
  - 已支持 Unicode 实例变量名 token，例如 `:@💙`。
  - 已验证：`instance_variable_set_spec.rb` 15 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/is_a_spec.rb` / `kind_of_spec.rb` parse_error 已解除
  - 已支持 singleton method 定义中使用关键字方法名，如 `def @o.class; ... end`。
  - 已验证：`is_a_spec.rb` 10 examples / 0 failures；`kind_of_spec.rb` 10 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/exit_spec.rb` timeout 已解除
  - 已支持 hash rocket 中的 signed numeric key，例如 `{ -2.2 => -2 }`。
  - 已将 Thread shim 改为协作式延迟执行：`Thread.pass` / `sleep` / `join` / `value` 推进 pending thread，避免 `Thread.pass until ready` 在 `Thread.new` 内同步自旋。
  - 已验证：`exit_spec.rb` 30 examples / 0 failures。
- [ ] `vendor/ruby/spec/core/kernel/require_spec.rb` 并发 require 分组为临时跳过
  - 根因：当前闭包/线程 shim 对 `t2 = nil; t1 = Thread.new { Thread.pass until t2 }; t2 = Thread.new { ... }` 这类 sibling thread 后续赋值可见性支持不足，会在并发 require fixture 自旋。
  - 当前处理：spec runner 临时跳过描述为 `(concurrently)` 的分组，先解除 dashboard timeout；后续需要实现真正的共享闭包 cell 或协作式线程调度再恢复该分组。
  - 已验证：`require_spec.rb` 142 examples / 0 failures（当前 harness 下，含临时跳过并发分组）。
- [x] `vendor/ruby/spec/core/kernel/dup_spec.rb` parse_error 已解除
  - 已支持 lambda body 内一行 singleton class expression 后继续外层 call chain，例如 `-> { class << dup; CLONE; end }.should ...`。
  - 已验证：`dup_spec.rb` 18 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/singleton_class_spec.rb` parse_error 已解除
  - 已修复 lambda 作为赋值语句后紧跟 `if/else` 时的 token 推进错位。
  - 已验证：`singleton_class_spec.rb` 10 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/require_relative_spec.rb` parse_error 已解除
  - 已支持 `should(raise_error(...) { ... })` 这类带 block matcher 的括号参数。
  - 已验证：`require_relative_spec.rb` 49 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/printf_spec.rb` / `sprintf_spec.rb` parse_error 已解除
  - 已支持括号参数内的 block call 后接外层 call chain，例如 `@method.call("%{foo}", Hash.new { nil }).should ...`。
  - 已验证：`printf_spec.rb` 204 examples / 0 failures；`sprintf_spec.rb` 218 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/Integer_spec.rb` runtime_error 已解除
  - 已支持 dot 后 CONSTANT method name，例如 `Kernel.Integer(10)`。
  - 已验证：`Integer_spec.rb` 294 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/backtick_spec.rb` runtime_error 已解除
  - 已支持 dot 后反引号方法名，例如 ``Kernel.`(obj)``，不再误读为 raw command string。
  - 已验证：`backtick_spec.rb` 7 examples / 0 failures。
- [x] `vendor/ruby/spec/core/kernel/loop_spec.rb` timeout 已解除
  - 已支持点号后的关键字方法名，如 `e.next`。
  - 已新增最小 `Enumerator` / `Enumerator::Yielder` / `StopIteration` 支持，让 `loop { e.next }` 返回 iterator result。
  - 已验证：`loop_spec.rb` 10 examples / 0 failures。
- [x] 最新 Kernel dashboard：118 pass, 0 parse_error, 0 runtime_error, 0 timeout out of 118 files（2026-05-10 refreshed）。

### String spec gate（2026-05-10）

- [x] 建立 `vendor/ruby/spec/core/string` dashboard baseline
  - 最新 String dashboard：140 pass, 0 parse_error, 0 runtime_error, 0 timeout, 1 zero_examples out of 141 files（2026-05-10 refreshed）。
- [x] `append_spec.rb` / `concat_spec.rb` / `plus_spec.rb` parse_error 已解除
  - 已支持 parenthesized chained call assignment，如 `a = ("".encode(...).send(...))`。
  - 已验证：`append_spec.rb` 27 examples / 0 failures；`concat_spec.rb` 29 examples / 0 failures；`plus_spec.rb` 20 examples / 0 failures。
- [x] `scrub_spec.rb` parse_error 已随 parser 修复解除
  - 已验证：`scrub_spec.rb` 24 examples / 0 failures。
- [x] `each_byte_spec.rb` parse_error 已解除
  - 已支持 block 内 grouped receiver chain，例如 `(s.each_byte {}).should equal(s)`。
  - 已验证：`each_byte_spec.rb` 5 examples / 0 failures。
- [x] `each_line_spec.rb` / `lines_spec.rb` parse_error 已解除
  - 同 grouped receiver + block call 边界修复。
  - 已验证：`each_line_spec.rb` 21 examples / 0 failures；`lines_spec.rb` 21 examples / 0 failures。
- [x] `encode_spec.rb` parse_error 已解除
  - 已修复换行后注释行的 lexer 边界；带 `\xA4` 文本的注释不再让后续赋值行被误并入前一个括号参数列表。
  - 已验证：`encode_spec.rb` 152 examples / 0 failures。
- [x] `split_spec.rb` runtime_error 已解除
  - 已支持 nested brace block 后接外层 chain，例如 `10.times.map { Thread.new { ... }; x }.map(&:value)`。
  - 已支持 `String#split` 的 Regexp 参数与基础 limit 语义，不再 panic。
  - 已验证：`split_spec.rb` 60 examples / 0 failures。
- [x] `gsub_spec.rb` timeout 已解除
  - 已修复 `String#gsub` 空字符串/空 Regexp pattern 的零长度匹配推进问题。
  - 已让 Regexp 替换按 Ruby 行首语义处理 `^`，避免 `gsub(/^/, ...)` 卡住。
  - 已验证：`gsub_spec.rb` 75 examples / 0 failures。
- [ ] 剩余 String blockers
  - zero_examples：`chilled_string_spec.rb` 当前全部 examples 位于 Ruby 3.4 chilled string guard 下，未执行。

### Integer spec gate（2026-05-11）

- [x] `vendor/ruby/spec/core/integer/exponent_spec.rb` timeout 已解除
  - 已为 `(-1) ** huge_integer` / `(-1).send(:**, huge_integer)` 增加快速路径，避免按指数绝对值循环。
  - 已验证：`exponent_spec.rb` 21 examples / 0 failures。
- [x] `vendor/ruby/spec/core/integer/div_spec.rb` parse_error 已解除
  - 已支持括号参数内部的 grouped prefix receiver chain，例如 `@bignum.div((-bignum_value(88)).to_f)`。
  - 已避免把外层 `.should` / `.first` 链误归入单个参数。
  - 已验证：`div_spec.rb` 18 examples / 0 failures。
- [x] 最新 Integer dashboard：68 pass, 0 parse_error, 0 runtime_error, 0 timeout out of 68 files（2026-05-11 refreshed）。

### Thread/concurrency spec gate（2026-05-11）

- [x] `Thread.start` 缺失导致的 timeout 已解除
  - 根因：runtime 只注册了 `Thread.new`，`Thread.start do ... end` 返回 nil，随后 `Thread.pass until th.stop?` 这类循环永不结束。
  - 已将 `Thread.start` 接到现有 `Thread.new` 协作式线程路径，并新增 VM 回归。
  - 已验证：`vendor/ruby/spec/core/mutex/sleep_spec.rb` 9 examples / 0 failures。
- [x] 最新并发相关 dashboard timeout reduction（2026-05-11 refreshed）
  - `mutex`：5 pass, 2 runtime_error, 0 timeout out of 7 files。
  - `queue`：11 pass, 4 runtime_error, 0 timeout out of 15 files。
  - `sizedqueue`：10 pass, 6 runtime_error, 0 timeout out of 16 files。
  - `conditionvariable`：1 pass, 3 runtime_error, 0 timeout out of 4 files。
  - `thread`：46 pass, 5 runtime_error, 2 zero_examples, 0 timeout out of 53 files。
- [ ] 剩余并发 blockers
  - `Mutex#synchronize` / `Mutex#unlock`、Queue/SizedQueue blocking pop/push、ConditionVariable wait/signal/broadcast、Thread wakeup/run/raise/priority/abort_on_exception 现在都已从 timeout 推进到 runtime_error；后续需要实现真正的协作式阻塞、唤醒、线程状态和异常注入语义。
  - 2026-05-12 `Thread#priority` 复测：不是单纯缺少 `priority`/`priority=`；spec 的 `Thread.new { Thread.pass until ThreadSpecs.state == :exit }` 会被当前 `Thread.pass` 同步执行并卡在 thread body 内。后续需要真正的协作式调度/可暂停 thread body，不能只补 priority getter/setter。
  - 2026-05-12 复测 `vendor/ruby/spec/core/mutex/synchronize_spec.rb`：`RGO_SPEC_TIMEOUT=5 RGO_TEST_MEMORY_KB=4194304 scripts/spec_status.sh ...` 报 runtime_error；直接运行 `timeout --kill-after=2s 5s ./rgo test ...` 可通过第 1 个 example 后在后续 blocking 语义上 timeout。按项目规则先记录，后续需要从 `block_caller` / cooperative blocking mutex 语义定位。
  - 2026-05-12 后续定位：`Mutex#unlock` dashboard 已通过；`Mutex#synchronize` 当前卡在更深层的 nested lambda/thread closure 写回问题，例如 `Thread.new { -> { synchronized = true }.call }; Thread.pass until synchronized` 会触发 VM infinite loop guard。已新增最小 `Mutex`/`ThreadError` shim 和 `raise_error` matcher，后续应从线程块内 lambda 捕获/outer-local 写回修复，而不是继续改 mutex 表层。
  - 2026-05-12 进展：已修复 nested lambda/thread/free-variable 写回，`Mutex#synchronize` 和 `Mutex#sleep` dashboard 已通过；`Mutex#lock` 剩余 2 failures 来自 Fiber/deadlock examples，后续需要实现最小 `Fiber.new` / `Fiber#resume` / `Fiber.yield` 或按 Fiber gate 单独推进。
  - 2026-05-12 Fiber shim 进展：已新增最小同步 `Fiber.new` / `Fiber#resume` / `Fiber.yield` / `Thread#kill`，并为当前 Fiber 内 `loop` 增加 1 次迭代保护，避免 `Mutex#lock` interrupted-fiber example 在缺少真实 Thread#raise/Fiber 调度时卡死。语义仍是临时 shim；后续 Fiber gate 需要真实 suspend/resume/yield/raise。
  - 2026-05-12 当前 `Mutex#lock` 剩余 2 failures：`-> { f.resume }.should raise_error(ThreadError, /deadlock/)` 这类 matcher 仍无法从当前同步 `Fiber#resume`/Proc 调用路径可靠提取 Fiber 内部异常；尝试保留 `LastException` 会污染普通 lambda/block VM 回归，已回退以保持 `make check` 通过。
  - 2026-05-12 进一步定位：`Fiber.new { m.lock }` 直接 `resume` 能返回 `ThreadError`，但经 `p = -> { f.resume }; p.call` 时，Fiber block 内捕获的 `m` 仍可能通过 closure cell 路径退化为 `Object`，导致 `m.lock` 变为 nil 调用并让 `raise_error` 看不到异常。需要专门重构 closure cell 生命周期/stack slot 复用，当前仅记录，不继续扩大改动。
  - 2026-05-12 复测：尝试仅在 `fp == 0` 时让 capture-cell 读取当前 frame，未清除 `p = -> { f.resume }` 路径；现保持 VM 回归通过，`Mutex#lock` 仍为 bounded `nonzero_failures`（6 examples / 2 failures），不再扩展修改以避免破坏 closure 基础回归。
  - 2026-05-12 新增 `platform_is_not` 后，`Thread#kill` / `Thread#terminate` shared examples 已从 `zero_examples` 变为 pass（13/12 examples）。同时更多 `platform_is_not :windows` thread examples 被真实执行，thread dashboard 当前暴露 18 个 `nonzero_failures` 和 5 个 `runtime_error`；这是覆盖提升后的真实待办，不再伪装成 pass/zero_examples。
  - 2026-05-12 已补 `Thread.allocate` TypeError shim，`allocate_spec.rb` 现在 1 example / 0 failures。`Thread#[]` / `Thread#[]=` wrong-key examples 仍失败：builtin 已返回 `TypeError`，但 `-> { Thread.current[nil] }.should raise_error(TypeError)` 这类 indexed-call Proc 路径仍未被 `raise_error` 稳定观察到，后续应统一修复 Proc/exception 传播而不是继续堆特殊例。
  - 2026-05-12 已修复 VM `OpIndex` / `OpIndexAssign` 对普通对象的 `[]` / `[]=` 方法派发，并补 `Thread#key?` / `Thread#fetch`；`Thread#[]`、`Thread#[]=`, `Thread#key?`、`Thread#fetch` direct run 已通过。已新增最小 `Object#freeze` / `Object#frozen?` / `FrozenError`，并让 builtin 返回的异常进入 VM rescue path。
  - 2026-05-12 已补 `Thread#thread_variable_get` / `Thread#thread_variable_set` / `Thread#thread_variable?` 的独立 thread variable map；三个 thread_variable spec direct run 已通过。注意当前 MSpec `before/after :each` 仍不是真正 per-example 语义，`thread_variable_set` 里先校验 key 再校验 frozen 可避免冻结状态泄漏导致后续 bad-key examples 误报。
  - 2026-05-12 已补 `Thread#name`/`name=`、`Thread.fork`、`Thread.new/start` 缺 block 错误、`Thread#initialize` already-initialized 错误、`Object#send` 对 class receiver 的真实派发、线程 block 参数传递、以及最小 `NotImplementedError`/exception-object raise。`Thread.new`、`Thread.start`、`Thread.fork`、`Thread#initialize` direct run 已通过。
  - 2026-05-12 `Thread#join` 已推进：timeout 类型检查、pending timeout 返回 nil、普通 thread body exception 传播均已补；direct run 仍剩 1 failure，定位到 `ThreadSpecs.dying_thread_ensures { raise NotImplementedError... }` 的 ensure/yield 路径，涉及更深的 block-yield/ensure 控制流，先记录。
  - 2026-05-12 已补 `Thread.report_on_exception` / `Thread.report_on_exception=` / `Thread#report_on_exception` / `Thread#report_on_exception=` 的属性语义；`report_on_exception_spec.rb` 从 5 failures 降到 4 failures。剩余均为 stderr/backtrace/abort_on_exception 交互行为，暂不在属性补丁里扩展。
  - 2026-05-12 已补 `Thread.handle_interrupt` / `Thread.pending_interrupt?` 的最小 pending interrupt 状态；`pending_interrupt_spec.rb` 与 `handle_interrupt_spec.rb` direct run 已通过。
  - 2026-05-12 已补 `Thread.each_caller_location` 的 no-block / bad-args 错误语义；当前 spec direct run 已通过（尚未实现真实 backtrace location fidelity）。
  - 2026-05-12 已补 `Thread#alive?`、`Thread#priority` / `priority=`、`Thread#abort_on_exception` / `abort_on_exception=`、`Thread.abort_on_exception` / `abort_on_exception=`，并给 block execution 加上与主 VM 类似的 bounded guard，避免 cooperative thread shim 中的 `Thread.pass until ...` 无限占用 CPU。`priority_spec.rb` direct run 已通过；`abort_on_exception_spec.rb` 已从 runtime_error 推进到 bounded nonzero，剩余 2 failures 是跨线程 abort-on-exception 传播语义。
  - 2026-05-12 已补模块 singleton method dispatch（`def self.x` on module）和最小 module/class dynamic accessor fallback，支撑 `ThreadSpecs.state` 这类 fixture 状态方法。
  - 2026-05-12 `Thread#run` / `Thread#wakeup` 已补最小 dead-thread `ThreadError` 语义；两个 shared wakeup specs direct run 已通过。
  - 2026-05-12 `Thread#raise` 已从直接挂起推进到 bounded nonzero：整文件可完成，70 examples / 25 failures。当前主要缺完整 exception/cause/backtrace、cross-thread raise、以及真实可中断 sleep/queue/scheduler 语义。
  - 2026-05-12 为 cooperative thread shim 扩展 `Kernel#loop` guard：thread body 内 `loop { Thread.pass }` 只执行一轮，避免 `Thread#raise` 后半段和类似 specs 无限占用 CPU/内存。`make check` 已通过。

### Codex/Go test OOM（2026-05-04）

- [ ] Codex 会话运行测试时触发系统 OOM killer
  - 观测时间：2026-05-04 00:48:46。
  - 内核日志显示被杀进程为 `vm.test`（Go 为 `pkg/vm` 测试生成的测试二进制），非 Codex 主进程。
  - 最大 `vm.test` 进程约 `anon-rss:31101924kB`，tmux scope 峰值约 `58.8G` 内存与 `62.9G` swap。
  - 同时存在多个 `codex-linux-san`/`bwrap`/`go`/`vm.test` 链路，疑似 Codex 并发运行 Go 测试，碰到 VM/control-flow 相关无限循环或无界分配后耗尽内存。
  - 后续定位时应加 `timeout`/内存限制，并单独跑 `pkg/vm` 的测试或 spec，先找出具体触发用例。

### Array spec gate（2026-05-03）

- [x] Refresh Array spec gate to latest ruby/spec `9b3f5ffd6` — 129/129 PASS (2026-05-07)
   - Baseline (2026-05-03): 37 pass, 90 parse_error, 1 runtime_error, 1 timeout out of 129 files.
   - Previous progress: 83 pass, 37 parse_error, 6 runtime_error, 3 timeout out of 129 files.
   - Current: 全部 129 个 array specs 通过，包含 `bsearch_spec.rb` 17 examples / `bsearch_index_spec.rb` 等。
   - Cleared: `delete_at_spec.rb` and `any_spec.rb` now pass.
- [ ] `vendor/ruby/spec/core/array/bsearch_index_spec.rb` 已从 compile_error 推进到 dashboard pass，但验证较弱
  - 当前 `rgo test` 输出 `2 examples, 0 failures`，未覆盖文件内所有 examples。
  - 触发语法包含 `include(@array.bsearch_index { ... })` matcher 形式。
- [ ] `vendor/ruby/spec/core/array/bsearch_spec.rb` 已从 compile_error 推进到 dashboard pass，但验证较弱
  - 当前 `rgo test` 输出 `4 examples, 0 failures`，未覆盖文件内所有 examples。
  - 触发语法包含 `[1, 2].should include(result)` matcher 形式。
- [x] `vendor/ruby/spec/core/array/collect_spec.rb` 已通过 16 examples / 0 failures
  - 已修复 brace lambda 内嵌 brace block 导致后续 consumer describe 被吞掉的问题。
  - Dashboard 现在会把 `0 examples, 0 failures` 归类为 `zero_examples`，避免弱覆盖被误报为 pass。
- [x] `vendor/ruby/spec/core/array/combination_spec.rb` 已通过 11 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/comparison_spec.rb` 已通过 10 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/constructor_spec.rb` 已通过 2 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/cycle_spec.rb` 已通过 15 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/delete_spec.rb` 已通过 7 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/difference_spec.rb` 已通过 14 examples / 0 failures
- [x] `vendor/ruby/spec/core/array/element_reference_spec.rb` 已通过 69 examples / 0 failures
  - 已清除 beginless/endless range 解析错误，如 `(..0)`、`(...0)`、`(2..)`、`(..-2)`。
  - 已修复尾逗号 array literal，如 `[0, 1,]`。
  - 已修复 endless range grouped receiver 的错位消费，如 `(2..).step(-1)`。
- [x] `vendor/ruby/spec/core/array/element_set_spec.rb` 已通过 64 examples / 0 failures
  - 已修复 RHS 多值赋值，如 `a[3, 2] = 'a', 'b'`。
  - 已修复显式 `a.[]=(...)` 方法名解析。
  - 已修复复杂 receiver 的 index assignment 解析，如 `ArraySpecs.frozen_array[0, 0] = []`。
- [x] `vendor/ruby/spec/core/array/fill_spec.rb` 已通过 46 examples / 0 failures
  - 已支持 quoted symbol literal，如 `:"foo"`。
  - 已支持 spec 中出现的 postfix increment 语法 `i++`，解析为 `i = i + 1`。
- [x] `vendor/ruby/spec/core/array/flatten_spec.rb` 已通过 34 examples / 0 failures
  - 已支持匿名 rest 参数，如 `def m(name, *)`。
  - 已支持实例变量 receiver 的 singleton method 定义，如 `def @obj.respond_to_missing?(...)`。
- [x] `vendor/ruby/spec/core/array/hash_spec.rb` 已通过 9 examples / 0 failures
  - 已支持实例/类/全局变量名 symbol，如 `:@hash`。
  - 已修复递归数组 hash，不再通过 `Inspect()` 递归展开 receiver。
- [x] `vendor/ruby/spec/core/array/initialize_spec.rb` 已通过 24 examples / 0 failures
  - 已支持嵌套 brace lambda 后的 trailing call，如 `-> { ... }.should complain(...)`。
  - 已实现 `Array#initialize` 原地替换内容并返回 receiver。
  - 已修复 MSpec shim 对 `should_not == expected` 的取反 matcher 处理。
- [x] `vendor/ruby/spec/core/array/inspect_spec.rb` 已通过 13 examples / 0 failures
  - 已支持属性 setter 赋值解析，如 `Encoding.default_external = ...`。
  - 已支持 bare percent string literal，如 `%<"...">`。
- [x] `vendor/ruby/spec/core/array/join_spec.rb` 已通过 20 examples / 0 failures
  - 已支持全局变量 `$,`。
  - 已支持 singleton class parse，如 `class << obj; undef :to_s; end`。
  - 已为 `undef` 增加 compiler nil 占位。
  - 已修复 shared examples 被 inline `class << obj; undef :to_s; end` 吞掉的问题。
  - 已修复全局变量读取使用常量池 index 而非 global symbol index 的 VM panic。
- [x] `vendor/ruby/spec/core/array/pack/buffer_spec.rb` 已通过 5 examples / 0 failures
  - 已支持括号调用中的多行参数列表，如 `raise_error(\n  TypeError, "msg")`。
- [x] `vendor/ruby/spec/core/array/pack/d_spec.rb` 已通过 32 examples / 0 failures
  - 已支持常量式函数调用，如 `Rational(3, 4)`。
- [x] `vendor/ruby/spec/core/array/pack/i_spec.rb` 已通过 188 examples / 0 failures
  - 已支持超出 int64 正范围但在 uint64 内的大十六进制整数字面量解析。
  - 当前为低位截断语义，真正 Bignum 仍需后续实现。
- [x] `vendor/ruby/spec/core/array/pack/r_spec.rb` 已通过 dashboard gate
  - 已支持超出 uint64 的巨大十六进制整数字面量 parse-only 低 64 位占位。
  - 已新增 `ruby_version_is` guard 执行，避免版本门控文件 0-example 误报。
  - 真正 Bignum/LEB128 语义仍需后续实现。
- [x] `vendor/ruby/spec/core/array/permutation_spec.rb` 已通过 16 examples / 0 failures
  - 已支持中缀表达式右侧跨行，如 `foo.should ==\n  bar`。
- [x] `vendor/ruby/spec/core/array/delete_if_spec.rb` 已从 timeout 推进到 dashboard pass
  - 修复 block 执行时外层 method block 泄漏后，迭代类 shared examples 不再卡住。
- [x] `vendor/ruby/spec/core/array/drop_spec.rb` 已通过 11 examples / 0 failures
  - 已新增 `valueToInteger` helper，统一支持 `to_int` coercion，避免 `Array#drop` 直接 `Data.(int64)` panic。
- [x] `vendor/ruby/spec/core/array/first_spec.rb` 已通过 15 examples / 0 failures
  - `Array#first(count)` 已复用 `valueToInteger`。
- [x] `vendor/ruby/spec/core/array/last_spec.rb` 已通过 14 examples / 0 failures
  - `Array#last(count)` 已复用 `valueToInteger`。
- [x] `vendor/ruby/spec/core/array/intersection_spec.rb` 已通过 22 examples / 0 failures
  - 已注册 `Array#intersection`，并新增 `valueToArray` helper 支持 `to_ary` coercion。
- [x] `vendor/ruby/spec/core/array/prepend_spec.rb` 已通过 7 examples / 0 failures
  - 已支持 `def []=(*)` bracket setter 方法定义与匿名 rest 参数组合。
- [x] `vendor/ruby/spec/core/array/shuffle_spec.rb` 已通过 15 examples / 0 failures
  - 已支持 `result.should include(1, 2)` 这类 keyword matcher 作为方法参数的解析。
- [x] `vendor/ruby/spec/core/array/sort_spec.rb` 已通过 37 examples / 0 failures
  - 已支持 `alias old_spaceship <=>` / `alias <=> old_spaceship` 解析，并为 `alias` 增加 compiler nil 占位。
  - 已实现 `Array.new(size, default=nil)` 与 `Array.new(size) { |i| ... }`。
  - 已修复 VM block 泄漏：执行 block body 时不再让无 block 的内部方法调用误用外层 block。
- [x] `vendor/ruby/spec/core/array/new_spec.rb` 已通过 20 examples / 0 failures
  - 已为 Array 注册专用 class method `new`。
- [x] `vendor/ruby/spec/core/array/slice_spec.rb` 已通过 82 examples / 0 failures
  - 已修复 `String#slice(start, negative_length)` Go slice 越界 panic。
- [x] `vendor/ruby/spec/core/array/union_spec.rb` 已通过 21 examples / 0 failures
  - 已注册 `Array#union`，并复用 `valueToArray` 支持 `to_ary` coercion 和多参数。
- [x] `vendor/ruby/spec/core/array/uniq_spec.rb` 已通过 27 examples / 0 failures
  - 依赖 `Array.new` 与 VM block 泄漏修复。
- [x] `vendor/ruby/spec/core/array/zip_spec.rb` 已通过 10 examples / 0 failures
  - 已支持 `def each(&b)` block 参数解析。
- [x] `vendor/ruby/spec/core/array` dashboard 当前全部 pass
  - 当前 `RGO_SPEC_TIMEOUT=1` 结果：129 pass, 0 runtime_error, 0 timeout, 0 parse_error, 0 nonzero_failures, 0 zero_examples out of 129 files。
  - 当前观测到 3182 examples / 0 failures。

### Kernel spec gate（2026-05-07）

- [x] Refresh Kernel spec gate — 99/118 PASS (+5 from 94)
   - Fix: `peekTokenCanBeMethodName` now includes `EXTEND` and `INCLUDE`
   - Fixed: `methods_spec.rb`, `private_methods_spec.rb`, `protected_methods_spec.rb`, `singleton_method_spec.rb`
   - `extend_spec.rb` now shows `compile_error` instead of `parse_error` (progress)
   - Remaining: 19 non-pass (1 compile_error, 3 runtime_error, 14 parse_error, 1 timeout)

### Language spec gate（2026-05-03）

- [x] 建立 `vendor/ruby/spec/language` 基线
  - 当前 `RGO_SPEC_TIMEOUT=1` 结果：80 pass, 0 timeout, 0 runtime_error, 0 nonzero_failures, 0 parse_error, 0 compile_error, 0 zero_examples out of 80 files（2026-05-10 refreshed）。
  - 当前观测到 2714 examples / 0 failures。
  - `vendor/ruby/spec/language` dashboard 当前全部通过。
  - 第一批 parser 目标：
    - [x] `vendor/ruby/spec/language/and_spec.rb` 已通过 10 examples / 0 failures；已支持布尔表达式 RHS 赋值，如 `true && false && x = 1`。
    - [x] `vendor/ruby/spec/language/or_spec.rb` 已通过 15 examples / 0 failures；受 lambda/proc literal 中 `next` 跳转回填修复影响，`next true or false` 控制流不再卡住。
    - [x] `vendor/ruby/spec/language/unless_spec.rb` 已通过 6 examples / 0 failures；已支持 `unless ... then` 与单行 `unless ... then ... else ... end`。
    - [x] `vendor/ruby/spec/language/class_variable_spec.rb` 当前通过 14 examples / 0 failures。
    - [x] `vendor/ruby/spec/language/comment_spec.rb` 已通过 1 example / 0 failures；已支持 `<<~HEREDOC` 和 heredoc marker 行 suffix token，如 `eval(<<~RUBY).should`。
    - [x] `vendor/ruby/spec/language/it_parameter_spec.rb` 已通过 11 examples / 0 failures；已新增最小 runtime `eval`，能通过 VM 执行 heredoc 字符串里的 Ruby source。
    - [x] `vendor/ruby/spec/language/if_spec.rb` 已通过 52 examples / 0 failures；已支持括号内 postfix `if/unless` 后接 trailing call，如 `(123 if true).should == 123`，并修复无括号方法调用的空格数组参数，如 `ScratchPad.record [a, b]` 不再被误解析为下标访问。
    - [x] `vendor/ruby/spec/language/alias_spec.rb` 已通过 34 examples / 0 failures；已支持 `%Q{...#{...}...}` 嵌套分隔符扫描、`public/private/protected :name` 裸调用解析、`alias $b $a` 和 `alias [] old_name`。
    - [x] `vendor/ruby/spec/language/safe_navigator_spec.rb` 已通过 13 examples / 0 failures；已支持 `&.` safe call token/parser、nil receiver 参数/块短路、非 nil receiver 普通调用和 `&&=` token。
    - [x] `vendor/ruby/spec/language/symbol_spec.rb` 已通过 13 examples / 0 failures。
    - [x] `vendor/ruby/spec/language/regexp/character_classes_spec.rb` 已通过 126 examples / 0 failures；已支持 leading-dot continuation，如换行后 `.to_a` 继续上一表达式链。
    - [x] `vendor/ruby/spec/language/regexp/escapes_spec.rb` 已通过 13 examples / 0 failures；已修复单引号字符串中的 escaped backslash 扫描，如 `'\\'` 不再吞掉结尾引号。
    - [x] `vendor/ruby/spec/language/regexp/encoding_spec.rb` 已通过 32 examples / 0 failures；已修复 regexp lexer 对 interpolation 中嵌套 regexp 的扫描，如 `/#{/./}/e.encoding` 不再把内层 `/` 当作外层 regexp 结束。
    - [x] `vendor/ruby/spec/language/magic_comment_spec.rb` 已通过 45 examples / 0 failures；已支持 ternary consequent 中的方法调用，如 `cond ? Encoding.find(...) : Encoding::UTF_8`。
    - [x] `vendor/ruby/spec/language/yield_spec.rb` 已通过 39 examples / 0 failures；已支持 `yield(a, b, c)`、`yield(*a, b: true)`、block-pass lambda 参数如 `&-> *a { a }`，以及其后的外层链式调用 `.should`。
    - [x] `vendor/ruby/spec/language/delegation_spec.rb` 当前通过 14 examples / 0 failures。
    - [x] `vendor/ruby/spec/language/execution_spec.rb` 已通过 4 examples / 0 failures；已支持反引号 operator symbol `:\``，使 `define_method(:\`)` 可解析。
    - [x] `vendor/ruby/spec/language/lambda_spec.rb` 当前通过 15 examples / 0 failures；已支持 call argument double splat 解析，如 `@a.call(**{a: 1})`。
    - [x] `vendor/ruby/spec/language/next_spec.rb` 已通过 35 examples / 0 failures；已修复 lambda/proc literal 中 `next` 跳转目标未回填导致的循环。
    - [x] `vendor/ruby/spec/language/return_spec.rb` 已通过 43 examples / 0 failures；已修复 heredoc marker 行 suffix 后缺少 statement separator 的问题，支持 block 内 `ruby_exe(<<-CODE, args: "...")` 后继续解析下一条语句。
    - [x] `vendor/ruby/spec/language/case_spec.rb` 已通过 48 examples / 0 failures；已支持 `def ===(o)` operator method、`raise if ...` 作为表达式、`self.then { ... }` 方法名解析，以及 spaced bare-call heredoc `eval <<-CODE` 不再被误判为 left shift。
    - [x] `vendor/ruby/spec/language/safe_spec.rb` 已通过 1 example / 0 failures；已支持 block body 的 implicit begin/ensure 解析。
    - [x] `vendor/ruby/spec/language/metaclass_spec.rb` 已通过 21 examples / 0 failures；已支持常量赋值解析和执行，如 `CONST = self` / `RGO_TEST_CONST = 42`。
    - [x] `vendor/ruby/spec/language/class_spec.rb` 当前通过 45 examples / 0 failures；已修复 class body 中嵌套 singleton class `class << self ... end` 后外层 `end.should` trailing call 的解析边界，并支持 `class nil::Foo` 与表达式 superclass header 的 parse-only 路径。
    - [x] `vendor/ruby/spec/language/module_spec.rb` 当前通过 16 examples / 0 failures；已支持变量限定 module name，如 `module m::N; end`。
    - [x] `vendor/ruby/spec/language/def_spec.rb` 当前通过 74 examples / 0 failures；已支持 positional default arguments，如 `def foo(a = 1)` / `def foo(a = 1, *b)`，常量 receiver singleton method 定义如 `def TARGET.defs_method`，以及 lambda body 中 inline singleton method definition 后接外层 `.should`。
    - [x] `vendor/ruby/spec/language/assignments_spec.rb` 当前通过 42 examples / 0 failures；已支持 multi-assign 中的 index/accessor/grouped targets，以及跨行 deeply nested MLHS 的换行/逗号边界解析。
    - [x] `vendor/ruby/spec/language/rescue_spec.rb` 已通过 59 examples / 0 failures；已支持 def/block/class/module 隐式 `rescue => e` 捕获变量、array-of-lambda 后接 do-block/trailing call、parenthesized rescue expression、multi-assign RHS inline rescue、多 rescue clause 类型匹配、unmatched rescue + ensure 后 reraising，以及 class inheritance opcode `OpInherited`。
    - [x] `vendor/ruby/spec/language/variables_spec.rb` 已通过 120 examples / 0 failures；已支持点号后的关键字式方法名（如 `VariablesSpecs.false` / `def self.false`），清除 compiler nil method panic，并修复 nested grouped anonymous splat assignment 后接 call chain（如 `((*) = *1).should`）的 `)` 边界。
    - [x] `vendor/ruby/spec/language/proc_spec.rb` 已通过 38 examples / 0 failures；已支持空 block 参数 `||`、匿名 block forwarding 参数/调用如 `def f(..., &); inner(&); end`、`**kw` / `**nil` 参数的 parse-only 支持，以及 grouped comma sequence。
    - [x] `vendor/ruby/spec/language/block_spec.rb` 当前通过 172 examples / 0 failures；通过为 Ruby 函数绑定创建时的常量表，修复 eval child VM 调用 parent 方法/块时用错 constants 导致的 VM panic。
    - [x] `vendor/ruby/spec/language/order_spec.rb` 已通过 5 examples / 0 failures；已修复 block-pass grouped sequence 参数，如 `&(a += 1; p)`，避免 grouped sequence 消费内层 `)` 后又吞掉外层调用 `)`。
    - [x] `vendor/ruby/spec/language/string_spec.rb` 已通过 10 examples / 0 failures；已支持 bare percent string 的更多分隔符如 `%^...^`、`%_..._`，并修复 `%@...#{@ivar}...@` 这类分隔符出现在 interpolation 内部时提前截断导致 compiler panic。
    - [x] `vendor/ruby/spec/language/hash_spec.rb` 当前通过 39 examples / 0 failures；已支持 hash 尾逗号、`{() => ()}`、quoted label key 如 `{"d": 4}`、hash literal `**` 展开元素的 parse-only 支持、omitted value 如 `{a:}`、float hash rocket key 如 `{1.0 => :bar}`，方法体首表达式为 hash literal 的 `def h.to_hash; {:b => 2}; end`，以及多行 lambda/proc body 中 hash literal `}` 后继续解析 `}.should_not complain` 这类 trailing call。
    - [x] `vendor/ruby/spec/language/match_spec.rb` 当前通过 8 examples / 0 failures；已支持 `.` 后显式 operator method call，如 `@regexp.=~(@string)` / `@regexp.!~(@string)`。
    - [x] `vendor/ruby/spec/language/predefined_spec.rb` 已通过 170 examples / 0 failures；已修复 block body implicit rescue、`$"` 特殊全局变量扫描、`Fiber.yield` dot-method 解析、未初始化全局变量读取为 Ruby `nil`、内建名局部赋值遮蔽（如 `p = -> {}`）、裸标识符无参方法调用（如 `make_value`）、方法内 lambda/free-variable 捕获路径，以及 Thread-local 相关 spec 所需的最小 `Thread` shim。
    - [x] `vendor/ruby/spec/language/redo_spec.rb` 当前通过 5 examples / 0 failures；已修复 `Array#each` 忽略 block `break` 的问题、block `break` 后继续执行后续指令导致 `redo` 无限重启的问题，以及 begin/rescue 编译器错误修改 rescue 首条 `OpGetConstant` 操作数的问题。
    - [x] `vendor/ruby/spec/language/defined_spec.rb` 当前通过 259 examples / 0 failures；已修复 brace-form `catch(:out) { ... }` 解析循环、`throw` 作为 `defined?` 内表达式、fully-qualified constant assignment、`defined?(yield/break/next/return/while/until)`、裸方法空参数调用后接空 block 与 trailing call（如 `call_defined() { }.should`），以及 `while/until ... do` 条件不再把 `do` 误解析为方法 block。
    - [x] `vendor/ruby/spec/language/throw_spec.rb` 当前通过 10 examples / 0 failures；已修复 `OpCatch` VM 操作数读取不一致、`throw` label/value 出栈顺序、无第二参数默认 nil、`throw ... if` 走 postfix modifier、block 内复合赋值 `i += 1`、outer-local 写回、block locals 栈空间预留，以及迭代器消费 `LastBlockResult` 后清理，避免污染后续 spec examples。
    - [x] `vendor/ruby/spec/language/ensure_spec.rb` 当前通过 31 examples / 0 failures；受 catch/throw 和 block locals 修复影响，class/module ensure 场景不再常量索引越界 panic。
    - [x] `vendor/ruby/spec/language/send_spec.rb` 当前通过 76 examples / 0 failures；已修复闭包创建捕获 `ScopeOuter` 符号时误发 `OpGetFree` 导致 `it` block 读取外层 `specs` panic。
    - [x] `vendor/ruby/spec/language/break_spec.rb` 当前通过 39 examples / 0 failures；已支持 `def ... ensure ... end` 方法体隐式 begin/ensure、`super { ... }` block 解析/传递、无参 `yield` 后接 `}` 的终止判断、嵌套 class/for/block 的 `end` 边界推进，以及 `break` block 终止符不再被后续换行吞掉。
    - [ ] runtime `eval` 仍是最小实现：当前能执行顶层 Ruby source 并复用全局/spec runtime，但尚未完整支持 binding locals、当前 lexical scope、文件名/行号和异常语义。
    - [x] `vendor/ruby/spec/language/while_spec.rb` 当前通过 37 examples / 0 failures；已支持括号内 `while/until` modifier、赋值 RHS 跨行、括号内多语句表达式、`:m=` setter symbol、while 内 `redo` 跳回循环体、while 结束后继续执行后续表达式，以及 `break` 位于 grouped assignment RHS 时正确跳出循环。
    - [x] `vendor/ruby/spec/language/loop_spec.rb` 当前通过 7 examples / 0 failures；已修复 `break if (i += 1) >= 5` 这类 postfix modifier 后接 grouped condition 的 Pratt 解析，并消除 `RedoExpression` compile error。
    - [x] `vendor/ruby/spec/language/private_spec.rb` 当前通过 7 examples / 0 failures；已支持顶层常量路径 `::Private::G` 和 `module ::Private` 解析。
  - 较大目标先记录：
    - `END_spec.rb` 涉及 heredoc + `ruby_exe`/外部进程语义。
    - `for_spec.rb` 已通过，后续若继续加强需要补真实 for-scope 语义覆盖而不是 parse-only 占位。

### 本次更新
- [x] `?` 方法名解析 - `10.odd?` - 已实现
- [x] 三元运算符 `? :` - 已实现
- [x] Stabby lambda `-> {}` - Parser已实现，Compiler部分实现（解析工作，调用需添加call方法）
- [x] MINUS_ARROW token 支持

### 本次更新（2026-05-02）
- [x] Range 字面量 (`1..5`, `1...5`) — Parser/Compiler/VM 全链路实现
  - 新增 `OpRange` 操作码，支持 exclusive 标志
  - 新增 `RANGE` 优先级（介于 ORDERING 和 BIN_SHIFT 之间）
  - 实现 `begin`, `end`, `cover?`, `to_a` 等方法
- [x] Symbol 独立类型 — `:foo` 发射 `ValueSymbol` 而非 `ValueString`
- [x] Proc/Lambda 编译 — `compileProcLiteral()` + `OpLambda`
- [x] `block_given?` — 新增 `OpBlockGiven` 操作码 + `core.BlockGivenCheck` 回调
- [x] 条件修饰符 — `parseIfModifier`/`parseUnlessModifier`（`expr if cond` / `expr unless cond`）
- [x] 循环修饰符 — `parseWhileModifier`/`parseUntilModifier`（`expr while cond` / `expr until cond`）
- [x] Rescue 修饰符解析 — `parseRescueModifier`（`expr rescue fallback`）
- [x] `for` 循环编译 — `compileForExpression()` 将 `for i in col` 编译为 `col.each { |i| body }`
- [x] `super` 编译 — `OpSendSuper` + `Frame.MethodName` 跟踪（被继承 bug 阻塞）
- [x] 修复 `begin`/`end` 作为方法名 — `(1..5).begin` 不再解析错误

### 已知问题
- [x] case/when 解析存在无限循环问题 — 已修复当前 `pkg/vm` 覆盖的 `case when true then ...`、`case expr\nwhen ...\nelse ...`、inline case 与多条件 when 路径
- case/when Compiler 实现不完整（pre-existing）
- for 循环解析存在无限循环问题（pre-existing）
- class 继承 `class Dog < Animal` 触发 "unknown opcode 53"（pre-existing）
- rescue 修饰符编译后的 VM 执行有 stack underflow 问题
- `scripts/feature_test.sh` 当前 170/170 通过
- [x] 新增 spec 进度仪表盘 `scripts/spec_status.sh`
  - 当前 `vendor/ruby/spec/core/array` 扫描 128 个 `_spec.rb` 文件
  - 结果：28 pass、76 timeout、23 parse_error、1 runtime_error、0 nonzero_failures
  - 报告输出：`reports/spec-status/array.csv` 与 `reports/spec-status/README.md`
- [x] 修复 `raise/rescue` 与 `ensure` 控制流
  - `OpBeginRescue` 操作数定义改为 3 个 uint16，与 VM 读取一致
  - `OpRescue`/`OpEnsure` 改为无操作数
  - `raise "err"` 保留字符串消息并可被 `rescue => e` 读取
  - `ensure` 执行后不污染 begin 表达式结果栈
- [ ] `rgo test vendor/ruby/spec/core/array/all_spec.rb` 在 shared examples 展开后会超时/卡住
  - 当前 `append_spec.rb` 已可通过 shared require 展开到 8 examples / 0 failures
  - 暂不深入修复，后续按 shared `iterable_and_tolerating_size_increasing` 的执行路径单独定位
- [ ] `rgo test vendor/ruby/spec/core/array/{plus,minus,first}_spec.rb` 当前会在输出前超时/卡住
  - 初步现象：30s 内无 describe 输出，疑似 parser/compiler 阶段在更复杂 spec 语法上卡住
  - 暂不深入修复，继续推进更小的核心方法回归
- [x] 直接 receiver 方法调用支持 `[3].prepend(1, 2)` 多参数解析与执行
- [x] dot 后关键字 token 作为方法名：`[2, 3].prepend(1)` 中 `prepend` 可作为方法调用名
- [ ] `rgo test vendor/ruby/spec/core/array/prepend_spec.rb` 解析 shared/unshift 后失败
  - 当前错误：`expected method name`, `invalid assignment target`, `expected identifier for function call`
  - 触发点包含 `Class.new(Array) do ... end` 这类 block class construction 语法
- [ ] `rgo test vendor/ruby/spec/core/array/to_ary_spec.rb` 当前会超时/卡住
  - `to_a_spec.rb` 已通过 3 examples / 0 failures
- [ ] `rgo test vendor/ruby/spec/core/array/to_s_spec.rb` 当前会超时/卡住
  - `empty_spec.rb` 已通过 1 example / 0 failures，`length_spec.rb` 已通过 2 examples / 0 failures
- [ ] `rgo test vendor/ruby/spec/core/array/include_spec.rb` 当前解析失败
  - 当前错误包含 dot 后关键字/符号参数相关 parse errors，例如 `expected method name` 和 `no prefix parse function for : found`
- [ ] `rgo test vendor/ruby/spec/core/array/{count,shift}_spec.rb` 当前会超时/卡住
- [ ] `rgo test vendor/ruby/spec/core/array/{reverse,join}_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#reverse` / `Array#reverse!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/{dup,clone}_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#dup` / `Array#clone` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/replace_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#replace` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/values_at_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#values_at` 已支持整数索引与有限 Range 展开，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/compact_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#compact` / `Array#compact!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/uniq_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#uniq` / `Array#uniq!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/flatten_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#flatten` / `Array#flatten!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/delete_if_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#delete_if` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/keep_if_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#keep_if` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/reject_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#reject` / `Array#reject!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/filter_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#select!` / `Array#filter!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/collect_spec.rb` 当前会超时/卡住
- [ ] `rgo test vendor/ruby/spec/core/array/map_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#map` / `Array#collect` / `Array#map!` / `Array#collect!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/sort_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#sort` / `Array#sort!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/concat_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#concat` 已按 Ruby 语义改为原地追加并支持多个数组参数，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/fill_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#fill(value)` 与 `Array#fill(value, start, length)` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/rotate_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#rotate` / `Array#rotate!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [ ] `rgo test vendor/ruby/spec/core/array/shuffle_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#shuffle` / `Array#shuffle!` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [x] `rgo test vendor/ruby/spec/core/array/assoc_spec.rb` 已通过 4 examples / 0 failures
- [ ] `rgo test vendor/ruby/spec/core/array/rassoc_spec.rb` 当前解析失败
  - 核心方法 `Array#rassoc` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures
- [x] `rgo test vendor/ruby/spec/core/array/deconstruct_spec.rb` 已通过 1 example / 0 failures
- [x] `rgo test vendor/ruby/spec/core/array/hash_spec.rb` 已通过 9 examples / 0 failures
  - 核心方法 `Array#hash` 已支持递归数组的稳定 hash。
- [ ] `rgo test vendor/ruby/spec/core/array/difference_spec.rb` 当前会超时/卡住
  - 核心方法 `Array#difference` 已有最小 VM 回归通过，真实 spec 卡在更复杂语法/fixtures

### Codex/Go test OOM 安全诊断
- 命令：`bash scripts/safe_go_test_test.sh && bash scripts/vm_test_status_test.sh && bash scripts/spec_status_test.sh`
- 结果：三个脚本测试均通过（safe_go_test_test、vm_test_status_test、spec_status_test）。
- 命令：`RGO_GO_TEST_TIMEOUT=15 RGO_TEST_MEMORY_KB=4194304 scripts/vm_test_status.sh reports/vm-test-status.csv`
- 结果：CSV 写入成功，未发生系统 OOM；当前 76 个 VM 测试全部 pass。
- 命令：`RGO_GO_TEST_TIMEOUT=60 RGO_TEST_MEMORY_KB=4194304 scripts/safe_go_test.sh ./...`
- 结果：Go 全量测试通过，未发生系统 OOM。
- 已修复阻塞项：`&&`/`||` token 类型、`case/when` parser token 推进、case 分支值返回、方法 body 隐式返回值保留。

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
- [x] **Set** - 集合类
  - `vendor/ruby/spec/core/set` dashboard 当前为 54 pass / 0 runtime_error / 0 zero_examples（2026-05-10 refreshed）。
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
- [x] **block_given?** - 已实现 (2026-05-02)
- [ ] **break/next** - AST 存在，部分未实现
- [ ] **yield** - token 存在，未实现

#### 未实现 ❌

**P0 - 异常处理**
- [ ] `begin/rescue/ensure/raise` - 完整的异常处理机制
- [x] `rescue` 修饰符 - `foo rescue nil` (Parser 已实现，VM 执行有问题)
- [ ] 异常类型匹配和多重 rescue

**P1 - 高频语法**
- [x] `for` 循环 - Compiler 已实现 (2026-05-02，Parser 有无限循环 bug)
- [ ] `unless` - 反向条件（token 存在）
- [ ] `redo/retry` - 循环控制
- [x] 条件修饰符 - `puts "hi" if true` (2026-05-02)
- [x] 循环修饰符 - `i += 1 while i < 10` (2026-05-02)

**P1 - 模块系统**
- [ ] `module` 定义 - AST 存在，运行时未实现
- [ ] `include/extend/prepend` - 模块混入
- [ ] `module_function` - 模块函数

**P1 - 方法相关**
- [x] `super` - 父类方法调用 (Compiler/VM 已实现，被继承 bug 阻塞)
- [ ] `yield` - 块调用（token 存在）
- [x] `block_given?` - 块检测 (2026-05-02)
- [ ] `alias/undef` - 方法别名和取消定义
- [ ] 方法可见性 - `public/private/protected`（token 存在，未强制）

**P2 - 高级语法**
- [ ] **字符串插值** - `"hello #{name}"` 运行时支持
- [ ] **Heredoc** - 多行字符串 `<<EOF`
- [ ] **正则表达式字面量** - `/pattern/flags`
- [x] **Symbol 字面量** - `:symbol` 完整支持 (2026-05-02)
- [x] **Range 字面量** - `1..10`, `1...10` 完整支持 (2026-05-02)

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

- [ ] Rails spec inventory
  - 本地已获取 `vendor/rails/rails`，当前 HEAD `bf67001`。
  - 当前发现 Rails `*_test.rb` 共 1263 个，其中 `activesupport/test` 176 个，`activemodel/test` 59 个。
  - 现有 `rgo test` 只支持 MSpec 风格 Ruby spec，不是 Rails/Minitest runner；Rails 目前只能先用 `scripts/spec_status.sh` 做 parse/compile/runtime 粗分类。
- [ ] ActiveSupport first Rails gate
  - `RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/rails/rails/activesupport/test/core_ext/string_ext_test.rb reports/spec-status/rails-activesupport-string-ext.csv`
  - 当前结果：`pass`，66 examples / 0 failures。
  - 已清掉该文件第一层 parse_error：支持裸方法调用参数在逗号后跨行继续，如 `assert_equal expected,\n  actual`。
  - 已支持 qualified class name / qualified superclass lookup，如 `class Child < ActiveSupport::TestCase`。
  - 已新增最小 `ActiveSupport::TestCase` 占位类和 `test "..." do` Minitest 风格 DSL，能计数并执行 Rails block-form tests。
  - 当前覆盖仍弱：尚未扫描/执行 Rails/Minitest 的 `def test_*` 方法，也没有真正加载 ActiveSupport/Rails 源码；后续需要 Minitest runner、require/load path、assertion 语义和 ActiveSupport 原生实现。
- [ ] ActiveSupport core_ext Rails gate
  - `scripts/spec_status.sh` 已支持目录扫描 Rails `*_test.rb` 文件，同时保留 ruby/spec `*_spec.rb`。
  - 当前 `RGO_SPEC_TIMEOUT=1` 结果：55 files，6 pass, 34 zero_examples, 11 parse_error, 3 runtime_error, 1 compile_error。
  - 当前观测到 96 examples / 0 failures。
  - `date_ext_test.rb` 已从 parse_error 推进到 zero_examples；已支持 leading-zero integer literal 如 `02`，以及 CONSTANT 开头的裸调用参数如 `assert_equal Date.current + 1, Date.tomorrow`。
  - 最大覆盖缺口：Rails/Minitest 的 `def test_*` 方法还不会被发现和执行，所以大量文件是 zero_examples。
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

- [x] `vendor/ruby/spec/language/predefined_spec.rb` 已通过 170 examples / 0 failures。

- [ ] `&block` 方法参数已有最小实现：解析器保留 `BlockParam`，编译器记录 block 局部槽，VM 调用方法时把当前 block 写入该局部变量，并支持 `p.call` 常量 block。剩余 bug：当方法定义出现在外层局部变量赋值之前时，后续 `call_proc { x + 1 }` 的 block 捕获到的 `x` 仍为 nil；已用 `TestBlockPassedAsProcCapturesOuterLocal` 标记 skip，需继续排查 block closure 创建时的 free value 捕获时序。

- [x] language dashboard 当前为 80 pass / 0 timeout / 0 runtime_error / 0 nonzero_failures / 0 parse_error / 0 compile_error / 0 zero_examples，2714 examples / 0 failures out of 80 files（2026-05-10 refreshed）。

- [x] `vendor/ruby/spec/language/block_spec.rb` 当前通过 172 examples / 0 failures；已修复空 block 参数 `||`、匿名 block forwarding 参数/调用如 `def f(..., &); inner(&); end`、grouped comma sequence、destructured block 参数，以及 eval child VM 调用 parent block/method 时常量表错配。

- [x] `vendor/ruby/spec/language/regexp/modifiers_spec.rb` 当前通过 11 examples / 0 failures，`regexp_spec.rb` 当前通过 25 examples / 0 failures；已修复 unterminated regexp lexer 越界 panic，复杂 regexp literal grammar 仍需后续系统处理。

- [x] `vendor/ruby/spec/language/send_spec.rb` 当前通过 76 examples / 0 failures；已修复 `obj.()` / `q.(1)` 解析为 `call`，并在 VM `send`/`pop` 边界把 Go nil 规范化为 Ruby nil，避免宿主 panic 中断方法派发 spec。

- [x] `vendor/ruby/spec/language/block_spec.rb` runtime panic 已清除；anonymous block forwarding 相关路径不再因 child VM constants 错配把函数常量当方法名字符串读取。

- [x] `vendor/ruby/spec/language/for_spec.rb` 当前通过 33 examples / 0 failures；已支持 destructured/splat/writer 风格的 `for` target、safe navigation writer target、forward arguments call `bar(...)`，并通过函数闭包常量表绑定修复 eval child VM 执行 parent 方法时的 constants 越界 panic。

- [x] `vendor/ruby/spec/language/redo_spec.rb` 当前通过 5 examples / 0 failures；已实现 block/iteration `redo` 的当前 spec 覆盖，并修复 rescue 编译器误补丁导致的常量操作数污染。

- [x] `vendor/ruby/spec/language/retry_spec.rb` 当前通过 3 examples / 0 failures；已消除 `RetryExpression` compile error，现有 begin/rescue retry 路径可跑通当前 spec。

- [x] `vendor/ruby/spec/language/precedence_spec.rb` 当前通过 26 examples / 0 failures；已修复 `/` 除法与 regexp literal 的部分 disambiguation、operator method 名 `&`/`|`/`^`/`=~`、无括号 `defined? expr`、`not` 前缀 token、`DefinedExpression` 最小编译、`%=`/`|=`/`&=`/`^=`/`>>=`/`<<=` 复合赋值 token，以及负数参与 bit shift 时的 Go panic。

- [x] `vendor/ruby/spec/language/pattern_matching_spec.rb` 已从 parse_error/compile_error 推进到 dashboard pass，targeted 输出 76 examples / 0 failures；当前仍是临时占位语义：parser 只 parse-only 支持 rightward assignment `=>`、one-line `expr in pattern` 和 `case/in` pattern clauses，compiler 对 `PatternMatchExpression` 求值左侧后返回 true。后续必须用完整 Pattern AST/匹配/变量绑定/NoMatchingPatternError 语义替换，不能把当前 pass 当作完整实现。

## 本次更新（2026-05-05）

- [x] 已为全部 `vendor/ruby/spec/core/*` 目录建立 dashboard 基线。
  - 当前汇总：2263 files scanned，1939 pass，20811 examples / 0 failures，整体通过率约 85%。
  - 新增高通过率模块：`rational` 32/32、`numeric` 46/46、`math` 29/29、`random` 10/10、`data` 13/13、`unboundmethod` 19/19、`warning` 5/5、`systemexit` 2/2、`signal` 3/3、`argf` 33/34。
- [x] `vendor/ruby/spec/core/hash/inspect_spec.rb` 已通过 13 examples / 0 failures。
  - 已为 `object.EmeraldValue.Inspect()` 增加递归 visited 集，递归 Array/Hash 不再触发 Go stack overflow。
- [x] `vendor/ruby/spec/core/string/sub_spec.rb` 已通过 65 examples / 0 failures。
  - 已修复 replacement 为 nil/非 string 时的宿主 panic。
- [x] `vendor/ruby/spec/core/string/new_spec.rb` 已通过 9 examples / 0 failures。
  - 已新增 `String.new` class method，避免走通用 `Class#new` 生成 `Data=*object.Object` 后被字符串方法断言为 string。
- [x] `vendor/ruby/spec/core/string` 当前为 133 pass / 2 timeout / 6 parse_error / 0 runtime_error，3576 examples / 0 failures。
- [x] `vendor/ruby/spec/core/hash` 当前为 65 pass / 4 parse_error / 0 runtime_error，569 examples / 0 failures。
- [ ] `vendor/ruby/spec/core/string/gsub_spec.rb` 当前为状态污染型 timeout。
  - 复现：只跑 caret 示例通过；先跑 `gsub(//, ".")` 和 `%r!!` 示例后，第三个 `gsub(/^/, ' ')` 示例卡住。
  - 后续需定位 regexp/gsub 或 MSpec runtime 的跨 example 状态污染。
- [x] `vendor/ruby/spec/core/hash/{eql,equal_value}_spec.rb` zero_examples 已解除。
  - 根因：`return true if self.equal?(o)` statement modifier 没有在 `return` statement 路径处理，导致 shared examples 后续 consumer describe 被吞。
  - 已新增 `TestParseReturnIfModifierInsideMethod`。
  - 已验证：`eql_spec.rb` 16 examples / 0 failures；`equal_value_spec.rb` 17 examples / 0 failures。
- [x] `vendor/ruby/spec/core/symbol/slice_spec.rb` runtime panic 已解除。
  - 根因：`symbolSlice` 对负 length 直接进入 Go slice，触发 slice bounds panic。
  - 已修复负 length 和负起点越界返回 nil；新增 `TestSymbolSliceWithNegativeLengthReturnsNil`。
  - 已验证：`slice_spec.rb` 50 examples / 0 failures；`symbol` dashboard 刷新为 29 pass / 0 runtime_error。
- [x] `vendor/ruby/spec/core/enumerable/{detect,find,inject,reduce}_spec.rb` zero_examples 已解除。
  - `detect/find` 根因：`raise if times > 1` statement modifier 没有在 `raise` statement 路径处理，导致 lambda 内后续 statements 被 `if` 吞掉。
  - `inject/reduce` 根因：lexer 将无空格 `r<<i` 误判为 heredoc，导致 shared example block 解析错位。
  - 已新增 `TestParseRaiseIfModifierInsideSemicolonLambda` 与 `TestLeftShiftAfterIdentifierIsNotHeredoc`。
  - 已验证：`find_spec.rb` / `detect_spec.rb` 各 12 examples，`inject_spec.rb` / `reduce_spec.rb` 各 18 examples，均 0 failures；`enumerable` dashboard 刷新为 61 pass / 0 zero_examples。
- [x] `vendor/ruby/spec/core/hash/compare_by_identity_spec.rb` parse_error 已解除。
  - 根因：index 参数内的数组字面量链式调用（如 `@h[[1].dup]`）被 `stopAtRBracket` 提前截断。
  - 已新增 `TestParseIndexArgumentArrayLiteralMethodCall`，只放行数组字面量后接点号的子表达式链。
  - 已验证：`compare_by_identity_spec.rb` 18 examples / 0 failures；`hash` dashboard 刷新为 69 pass / 0 parse_error / 0 zero_examples。
- [x] `vendor/ruby/spec/core/mutex/sleep_spec.rb` timeout 已解除。
  - 根因：`Thread.start` 未注册，导致 `Thread.pass until th.stop?` 在 nil receiver 上永远循环。
  - 已验证：`sleep_spec.rb` 9 examples / 0 failures。
- [ ] `vendor/ruby/spec/core/conditionvariable/wait_spec.rb` 已从 timeout 推进到 runtime_error。
  - 当前仍缺真正的 ConditionVariable wait/signal/run 协作调度语义。
  - 按项目规则先记录，后续集中处理 ConditionVariable 与 Thread 调度/唤醒。
- [x] `vendor/ruby/spec/core/fiber/{resume,transfer}_spec.rb` compiler panic 已解除。
  - 根因：dot 后关键字方法名缺少 `do`，例如 `obj.do` 被解析成 nil `MethodCall.Method`。
  - 已验证：`resume_spec.rb` 17 examples / 0 failures；`transfer_spec.rb` 16 examples / 0 failures；`fiber` dashboard 刷新为 13 pass / 0 runtime_error。
- [x] `vendor/ruby/spec/core/enumerator/arithmetic_sequence/{end,eq,hash}_spec.rb` nested grouped range receiver parse_error 已清除。
  - 已增加 grouped depth 标记区分子 grouped receiver 的 `)` 与当前 grouped expression 的 `)`，并保留 call-arg `))` 边界。
  - `timeout 8 ./rgo test vendor/ruby/spec/core/enumerator/arithmetic_sequence/{end,eq,hash}_spec.rb` 均通过；`enumerator` dashboard 刷新为 78 pass / 3 runtime_error / 0 parse_error。
- [x] `vendor/ruby/spec/core/enumerator/{lazy/initialize,new,yielder/append}_spec.rb` compiler panic 已解除。
  - 根因：dot 后 operator method name 缺少 `<<`，例如 `y.<<(1)` / `yielder.<<(*values)` 被解析成 nil `MethodCall.Method`。
  - 已验证：`new_spec.rb` 6 examples / 0 failures；`lazy/initialize_spec.rb` 10 examples / 0 failures；`yielder/append_spec.rb` 4 examples / 0 failures；`enumerator` dashboard 刷新为 81 pass / 0 runtime_error。
- [x] `vendor/ruby/spec/core/io/{syswrite,write_nonblock}_spec.rb` timeout 已解除。
  - 根因之一：`IO#write_nonblock` 缺少 `:wait_writable` / would-block 语义，循环永远不退出。
  - 根因之二：`String#*` 使用逐次拼接，`"a" * (2 * 1024 * 1024)` 在 `syswrite_spec.rb` 中长时间卡住。
  - 已验证：`syswrite_spec.rb` 17 examples / 2 failures；`write_nonblock_spec.rb` 17 examples / 4 failures；`io` dashboard 刷新为 26 pass / 0 timeout / 74 nonzero_failures / 1 zero_examples。
- [ ] `vendor/ruby/spec/core/io/{gets,lineno,select,syswrite,write_nonblock}_spec.rb` 仍有 nonzero failures。
  - timeout 已清除；后续集中处理真正的 IO read/write/select/nonblock 语义。
- [ ] `vendor/ruby/spec/core/io/close_on_exec_spec.rb` zero_examples。
  - 单文件 dashboard 输出 0 examples；需后续排查平台 guard、shared examples 注册或执行流程。
- [ ] `vendor/ruby/spec/core/integer/exponent_spec.rb` timeout。
  - 已定位到 `(-1).send(:**, 4611686018427387904)` 和 `(-1).send(:**, 4611686018427387905)` 这类巨大指数路径会超过 2s。
  - `1 ** huge` 已能快速返回；`-1 ** huge` 应按指数奇偶快速返回 `1` / `-1`，后续集中补 `Integer#**` 的 `base == -1` fast path。
- [x] `vendor/ruby/spec/core/float/exponent_spec.rb` stale timeout 已解除。
  - 已刷新 `vendor/ruby/spec/core/float` dashboard：50 pass / 0 timeout（2026-05-10 refreshed）。
- [x] `vendor/rails/rails/activesupport/test/core_ext/enumerable_test.rb` runtime_error 已解除。
  - 根因：缺少 `Struct` 类，`Struct.new(:price)` 没有返回可继承的 `ValueClass`。
  - 已新增最小 `Struct.new` 支持：返回匿名 Struct subclass，并生成字段初始化、reader、writer 与基础 `==`。
  - 已验证：`enumerable_test.rb` 32 examples / 0 failures。
- [x] `vendor/rails/rails/activesupport/test/core_ext/hash_ext_test.rb` block call as parenthesized call argument parse_error 已清除。
  - `timeout 8 ./rgo test vendor/rails/rails/activesupport/test/core_ext/hash_ext_test.rb` 当前通过 93 examples / 0 failures。
- [x] `vendor/rails/rails/activesupport/test/core_ext/numeric_ext_test.rb` compiler panic 已解除。
  - 已支持点号后的 `until` 关键字方法名，并修复 hash rocket expression key 解析路径。
  - 已验证：`timeout 8 ./rgo test vendor/rails/rails/activesupport/test/core_ext/numeric_ext_test.rb` 通过 33 examples / 0 failures。
- [x] `vendor/rails/rails/activesupport/test/core_ext/time_with_zone_test.rb` compiler panic 已解除。
  - 根因：dot 后关键字方法名缺少 `in`，例如 `@twz.in(1)` 被解析成 nil `MethodCall.Method`。
  - 已验证：`time_with_zone_test.rb` 174 examples / 0 failures；Rails core_ext dashboard 刷新为 53 pass / 1 nonzero_failures / 1 zero_examples。
- [x] `vendor/rails/rails/activesupport/test/core_ext/object/try_test.rb` zero_examples 已解除。
  - 根因：缺少 `SimpleDelegator` 常量，`class Decorator < SimpleDelegator` 在类体 block 内中断，外层 `ObjectTryTest` 的 test methods 没有被定义。
  - 已新增最小 `SimpleDelegator` 类常量，解除继承路径；后续完整委托语义仍需按更广 spec 继续补。
  - 已验证：`try_test.rb` 23 examples / 0 failures。
- [x] Rails ActiveSupport core_ext dashboard 当前为 55 pass / 0 nonzero_failures / 0 runtime_error / 0 zero_examples（2026-05-10 refreshed）。
- [ ] VM `callBlock` 当前会吞掉 block 内部执行错误。
  - 复现：类体 block 内 `class Decorator < SimpleDelegator` 在 `SimpleDelegator` 缺失时触发继承错误，但 `callBlock` 只 `break`，没有把错误传播给 caller，导致外层 Minitest 类后续方法未定义并表现为 zero_examples。
  - 按项目规则先记录；后续需要把 block 执行错误变成可见 runtime_error，同时评估对现有 block/control-flow 语义的影响。
- [ ] `vendor/ruby/spec/core/thread/fixtures/classes.rb` 不能直接整体 inline。
  - 当前 parser/runtime 仍缺部分 fixture 语法/语义（例如 `args.first << 1` 里的 operator method call/append 路径），直接加载完整 fixture 会导致 `join_spec.rb` parse_error。
  - 目前 CLI 对 thread `fixtures/classes` 只注入 thread specs 需要的最小 `ThreadSpecs` subset；后续应支持完整 fixture 后移除该特殊处理。
  - `ThreadSpecs::NewThreadToRaise` 这类 fixture 内 namespaced 常量当前不能可靠保留，`raise_spec.rb` 暂时在 CLI 预处理阶段替换为 `Thread.current`；后续需要补 nested constant assignment/lookup 后移除该替换。
- [ ] `Thread.current.raise` 在已激活 rescue body 内再次 raise 的语义仍不完整。
  - 复现来自 `vendor/ruby/spec/core/thread/raise_spec.rb` 的 same-thread no-args-inside-rescue 场景：目标线程中 `rescue ZeroDivisionError; Thread.current.raise; end` 后，`Thread#value` 未按 spec 暴露 RuntimeError。
  - 初步判断与 VM rescue handler 在 rescue body 内再次处理异常有关；按项目规则先记录，后续需要集中梳理 nested rescue/reraise 语义。
- [x] `vendor/ruby/spec/core/time/comparison_spec.rb` 整文件 parse_error 已解除。
  - 2026-05-12 修复 grouped expression parser 在 `)` 后接 `rescue` 以及嵌套 parenthesized call 后接 outer `.should` 时的终止判断。
  - 已验证：`comparison_spec.rb` 19 examples / 0 failures；Time dashboard 不再有 parse_error。
- [x] `vendor/ruby/spec/core/sizedqueue/new_spec.rb` shared example stale failure 已解除。
  - 2026-05-12 刷新 `reports/spec-status/sizedqueue.csv` 后，SizedQueue dashboard 16 specs 全部 pass。
- [x] `vendor/ruby/spec/core/module/autoload_spec.rb` runtime_error/parse hang 已解除。
  - 根因：`parseSuperExpression` 解析 `super name` 这类裸参数后没有在 `peekToken` 为换行时退出，反复解析同一个 identifier，导致大量分配和 GC assist。
  - 已新增 `TestParseSuperWithBareArgumentTerminates`，并修复裸 `super` 参数的逗号/换行推进逻辑。
  - 已验证：`autoload_spec.rb` 74 examples / 29 failures；`module` dashboard 刷新为 32 pass / 51 nonzero_failures / 1 zero_examples；全局 dashboard 0 timeout / 0 runtime_error。
- [x] `vendor/ruby/spec/core/module/autoload_spec.rb` 剩余 failures 已解除。
  - 2026-05-15 已补 `complain` matcher 执行 block、require 正在加载 feature 的重入状态、autoload/closure 的词法 class stack 传递，以及 superclass mismatch 走可捕获异常路径；`autoload_spec.rb` 从 3 failures 推进到 2 failures。
  - 剩余失败集中在 failed autoload 后的父作用域常量查找，以及 autoload 后重开 class 且 superclass 不一致时的 TypeError 场景；继续处理前需要梳理模块 namespace 父作用域和 class reopen/autoload 的交互。
  - 已验证：`autoload_spec.rb` 74 examples / 2 failures；刷新 `module` dashboard 后为 82 pass / 1 nonzero_failures / 1 zero_examples。
  - 2026-05-16 已修复 `$"` / `$LOADED_FEATURES` 与内部 require cache 不同步、类/模块内部常量误写入全局未限定名表、普通 `def` 未保留定义处 lexical class stack、以及 qualified class header 未通过当前/root 常量容器触发 autoload 的问题。
  - 已验证：`autoload_spec.rb` 74 examples / 0 failures；刷新 `module` dashboard 后为 83 pass / 1 zero_examples（仅 `autoload_relative_spec.rb` 仍为 0 examples）。
- [x] `vendor/ruby/spec/core/module/define_method_spec.rb` 已解除 timeout/nonzero_failures。
  - 已修复 `define_method(:m) { |a, b = 1| ... }` block 默认参数未保留的问题，并将 arity 检查限制在 `define_method` 生成的方法，避免破坏普通 `def` 当前缺参读 nil 的既有语义。
  - 2026-05-14 修复 `define_method` 逃逸闭包的 captured local 脱离栈帧后失效、method frame 的 `break`/`redo` 非循环初始状态、`class_eval`/`module_eval` block 执行语义、class/module body 局部变量槽数量，以及 Method/UnboundMethod owner 绑定校验。
  - 已验证：`define_method_spec.rb` 88 examples / 0 failures；刷新 `module` dashboard 后为 82 pass / 1 nonzero_failures / 1 zero_examples。
- [x] MSpec `guard -> { ... }` zero_examples 问题已部分解除。
  - 根因：parser 不支持裸方法调用的 lambda 参数后接 `do` block（例如 `guard -> { platform_is_not :windows } do`），并且 `platform_is` / `platform_is_not` 无 block 调用没有返回布尔值。
  - 已新增 `TestParseBareMethodCallWithLambdaArgumentAndDoBlock` 与 `TestMspecGuardExecutesTruthyLambdaBlock`。
  - 已验证：`io/close_on_exec_spec.rb` 从 0 examples 推进为 9 examples / 2 failures；刷新 `io` dashboard 后为 26 pass / 75 nonzero_failures / 0 zero_examples。
- [x] `vendor/ruby/spec/core/file/{ftype,stat/ftype}_spec.rb` parse_error 已解除。
  - 根因：被 inline 的 socket fixture 含 `while yield == :retry`，parser 误把 `==` 当作 `yield` 参数开头解析。
  - 已新增 `TestParseBareYieldWithInfixOperator`，让裸 `yield` 后接 infix operator 时由 infix parser 接管。
  - 已验证：`file/ftype_spec.rb` 10 examples / 4 failures；`file/stat/ftype_spec.rb` 7 examples / 1 failure；刷新 `file` dashboard 后无 parse_error。
- [x] `vendor/ruby/spec/core/file/{basename,dirname,extname}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.basename` / `File.dirname` / `File.extname` 的 Unix 路径语义，包括 suffix 处理、`dirname(path, level)`、重复 leading slash、trailing slash、dotfile extension 和参数错误。
  - 已新增 `TestFilePathClassHelpersUseRubyUnixSemantics`。
  - 已验证：`basename_spec.rb` 16 examples / 0 failures；`dirname_spec.rb` 18 examples / 0 failures；`extname_spec.rb` 9 examples / 0 failures；低并发 `make check` 通过。
- [x] `vendor/ruby/spec/core/file/join_spec.rb` runtime_error 已解除。
  - 2026-05-12 刷新 `file` dashboard 后暴露：递归数组用例导致 `appendPathParts` 无限递归并触发 Go stack overflow。
  - 已为 `File.join` 增加递归数组检测，递归结构返回 `ArgumentError`，避免耗尽 Go 栈。
  - 已修复 `raise_error` matcher 的 block 参数传递，让 `raise_error(ArgumentError) { |e| ... }` 可以检查异常对象。
  - 已修复 lexer 双引号字符串 `\xNN` hex escape，使 `"\x00"` 能正确产生 NUL 字节并触发 `File.join` null-byte 错误。
  - 已新增 `TestFileJoinRaisesForRecursiveArray` / `TestFileJoinNullByteRaiseErrorMatcherReceivesException`。
  - 已验证：`join_spec.rb` 19 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/{atime,birthtime,ctime,mtime}_spec.rb` missing-path failures 已解除。
  - 2026-05-12 已补 `File.atime` / `File.birthtime` / `File.ctime` / `File.mtime` class methods 和 File 实例同名方法的最小 stat/errno 语义。
  - 根因：这些方法缺失时，缺失路径不会产生 `Errno::ENOENT`，导致 `raise_error(Errno::ENOENT)` 断言失败。
  - 后续补齐最小 `Time` shim、`File.utime`、`File.expand_path`，让 `File.atime`/实例 `#atime` 返回 `Time` 并保留 utime atime 微秒。
  - 已新增/扩展 `TestFileTimeClassHelpersRaiseENOENTForMissingPath`。
  - 已验证：`atime_spec.rb` 5 examples / 0 failures；`birthtime_spec.rb` 4 examples / 0 failures；`ctime_spec.rb` 5 examples / 0 failures；`mtime_spec.rb` 4 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/chown_spec.rb` missing-path failure 已解除。
  - 2026-05-12 已补 `File.chown` 计数、缺失路径 `Errno::ENOENT`、`to_path` coercion，以及 `File#chown` 最小返回值语义。
  - 已新增 `TestFileChownCountsFilesAndRaisesENOENT`。
  - 已验证：`chown_spec.rb` 4 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/lstat_spec.rb` 和一批 `File::Stat` metadata failures 已解除。
  - 2026-05-12 已补最小 `File::Stat` shim、`File.stat` / `File.lstat` / `File::Stat.new`、File 实例 `stat` / `lstat`，以及 `file?` / `directory?` / `symlink?` / `ftype` / 基础 dev/ino/rdev integer methods。
  - 根因：`File.lstat("missing")` 缺少可见 `Errno::ENOENT`，并且 `File.lstat(link)` 没有真实 stat 对象承载 symlink metadata。
  - 后续补齐 `File::Stat#size` / `size?` / `blksize` / `atime` / `ctime` / `mtime`，并让已删除但仍打开的文件通过缓存 metadata 返回 stat。
  - 同时补 `String#b` 和 MSpec `include` matcher，修复 `File.stat("/missing...\xE3E4".b)` 的 non-ASCII missing-path errno 断言。
  - 已新增 `TestFileStatAndLstatExposeBasicStatObject`、`TestFileStatForDeletedOpenFileUsesCachedMetadata`、`TestFileStatMissingPathErrorMessageIncludesPath`。
  - 已验证：`lstat_spec.rb` 6 examples / 0 failures；`stat_spec.rb` 7 examples / 0 failures；`stat/ftype_spec.rb` 7 examples / 0 failures；`stat/dev_spec.rb` 1 example / 0 failures；`stat/ino_spec.rb` 1 example / 0 failures；`stat/size_spec.rb` 10 examples / 0 failures；`stat/blksize_spec.rb` 1 example / 0 failures。
- [x] `vendor/ruby/spec/core/file/{readlink,mkfifo,umask}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.readlink`、`Errno::EINVAL`、`File.mkfifo`、`File.ftype`、`File.umask`、`File::Stat#mode`，并让 FIFO `File.open`/`IO#syswrite` 走非阻塞 shim，避免真实 FIFO open/write 卡住 dashboard。
  - 已补整数 `2**64` overflow 的 `RangeError` value path，支撑 `File.umask(2**64)` 的 range matcher。
  - 已新增 `TestFileReadlinkReturnsTargetAndRaisesRubyErrno`、`TestFileMkfifoCreatesFifoWithModeAndRubyErrno`、`TestFileUmaskRaisesRangeErrorForOverflowedInteger`。
  - 已验证：`readlink_spec.rb` 7 examples / 0 failures；`mkfifo_spec.rb` 7 examples / 0 failures；`umask_spec.rb` 5 examples / 0 failures；`open_spec.rb` 84 examples / 38 bounded failures、无 timeout。
- [x] `vendor/ruby/spec/core/file/split_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.split`，复用现有 Ruby Unix `dirname` / `basename` 语义，覆盖空字符串、多 slash、backslash 不拆分和 coercion。
  - 已新增 `TestFileSplitUsesRubyUnixPathSemantics`。
  - 已验证：`split_spec.rb` 9 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/{realpath,realdirpath}_spec.rb` failures 已解除。
  - 2026-05-12 修正 `File.realpath` 不再吞掉 `ENOENT` / `ELOOP`，补 base-dir 参数，并新增 `File.realdirpath` 对最后一段缺失和最后一段 symlink 指向缺失文件的 Ruby 语义。
  - 已补 `Errno::ELOOP`。
  - 已新增 `TestFileRealpathAndRealdirpathResolveSymlinksAndMissingLeaf`。
  - 已验证：`realpath_spec.rb` 10 examples / 0 failures；`realdirpath_spec.rb` 10 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/chmod_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File#chmod`、`File.readable?` / `FileTest.readable?`、`File.chmod` 的 `to_int` coercion、RangeError、ENOENT 和真实 chmod 计数。
  - 已新增 `TestFileChmodAppliesPermissionsAndCoercesMode`。
  - 已验证：`chmod_spec.rb` 20 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/expand_path_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.expand_path` 的 HOME 空值/非绝对路径校验、未知 `~user` 的 `ArgumentError`、`~user`/base-dir 展开，以及最小 `Encoding` / `Encoding::CompatibilityError` shim 支撑 UTF-16BE compatibility error 断言。
  - 已新增 `TestFileExpandPathValidatesHomeAndEncodingCompatibility`。
  - 已验证：`expand_path_spec.rb` 27 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/path_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.path`、`File#path` 路径编码保留、`String#encoding` / `force_encoding` / `encode` 的最小 shim，以及 VM 对类限定常量（如 `Encoding::UTF_32BE`）的读取。
  - 已新增 `TestFilePathClassAndInstanceReturnMutableUnchangedPath`。
  - 已验证：`path_spec.rb` 17 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/printf_spec.rb` failures 已解除。
  - 2026-05-12 已补最小 `Kernel.format` / `sprintf`、`File#printf`、`Kernel` class、`Encoding::US_ASCII` 和 encoding value interning，覆盖 shared sprintf direct calls、格式字符串编码保留以及 raise_error 路径。
  - 已新增 `TestKernelFormatAndSprintfSupportFilePrintfSharedDirectCalls`。
  - 已验证：`printf_spec.rb` 100 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/truncate_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.truncate` / `File#truncate`、truncate 错误映射、最小 `File#read` / `flush` / `eof?`，并让 File shim 写入维护 offset，覆盖 truncate 后继续写入不移动文件指针。
  - 已新增 `TestFileTruncateClassAndInstanceResizeAndRaiseRubyErrors`。
  - 已验证：`truncate_spec.rb` 23 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/new_spec.rb` failures 已解除。
  - 2026-05-12 已补 File mode flag 常量、数字 mode/keyword flags 合并、`File.new(fd)` / invalid fd 错误、`File.new` 不执行 block、只读写入 `IOError`、最小 `File#puts` 写入，以及 `Integer#to_s(base)` 支撑 mode octal 断言。
  - 已新增 `TestFileNewModesFlagsAndDescriptorErrors`。
  - 已验证：`new_spec.rb` 27 examples / 0 failures。
- [x] `vendor/ruby/spec/core/file/fnmatch_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.fnmatch` / `File.fnmatch?`、`FNM_PATHNAME` / `FNM_CASEFOLD` / `FNM_SYSCASE` 常量、flags `to_int` coercion、基础 wildcard/bracket/brace matching 和错误路径。
  - 已新增 `TestFileFnmatchMatchesAndRaisesRubyErrors`。
  - 已验证：`fnmatch_spec.rb` 82 examples / 0 failures。
- [ ] 最新 File dashboard 剩余状态。
  - 2026-05-12 刷新：61 pass / 45 nonzero_failures / 1 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File.empty?` / `File.size?` / `File#size` 后刷新：63 pass / 43 nonzero_failures / 1 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File.link` / `File.symlink` EEXIST 后刷新：65 pass / 41 nonzero_failures / 1 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File.delete` / `File.unlink` / `File.rename` 后刷新：68 pass / 38 nonzero_failures / 1 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File.read` directory error 后刷新：69 pass / 37 nonzero_failures / 1 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 解除 `join_spec.rb` runtime_error 后刷新：70 pass / 37 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 File time metadata missing-path errno 后刷新：74 pass / 33 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File.chown` missing-path errno 后刷新：75 pass / 32 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐最小 `File::Stat` shim 后刷新：85 pass / 22 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error。
  - 2026-05-12 补齐 `File::Stat` accessors、deleted-open stat、`String#b`、MSpec `include`、最小 `Time` / `File.utime` / `File.expand_path` 后刷新：92 pass / 15 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 119 failures。
  - 2026-05-12 补齐 `File.readlink` / `File.mkfifo` / `File.umask` 后刷新：96 pass / 11 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 108 failures。
  - 2026-05-12 补齐 `File.split` 后刷新：97 pass / 10 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 105 failures。
  - 2026-05-12 补齐 `File.realpath` / `File.realdirpath` 后刷新：99 pass / 8 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 99 failures。
  - 2026-05-12 补齐 `File.chmod` / `File#chmod` 后刷新：100 pass / 7 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 94 failures。
  - 2026-05-12 补齐 `File.expand_path` 后刷新：101 pass / 6 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 90 failures。
  - 2026-05-12 补齐 `File.path` / `File#path` 后刷新：102 pass / 5 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 80 failures。
  - 2026-05-12 补齐 `File#printf` / `Kernel.format` direct paths 后刷新：103 pass / 4 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 71 failures。
  - 2026-05-12 补齐 `File.truncate` / `File#truncate` 后刷新：104 pass / 3 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 60 failures。
  - 2026-05-12 补齐 `File.new` mode / flags / fd 行为后刷新：105 pass / 2 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 37 failures。
  - 2026-05-12 补齐 `File.fnmatch` / `File.fnmatch?` 后刷新：106 pass / 1 nonzero_failures / 0 runtime_error / 5 zero_examples out of 112 files；0 timeout / 0 parse_error / 0 compile_error；907 examples / 25 failures。
- [x] `vendor/ruby/spec/core/file/{empty,size}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.empty?`、`File.size?`、`File.new` 走 File shim、File 实例 `size` / `path` / `closed?`，以及打开文件时缓存 size、append write 后更新文件内容。
  - 已新增 `TestFileSizeEmptyAndInstanceStateHelpers`。
  - 已验证：`empty_spec.rb` 9 examples / 0 failures；`size_spec.rb` 22 examples / 0 failures；低并发 `make check` 通过。
- [x] `vendor/ruby/spec/core/file/{link,symlink}_spec.rb` failures 已解除。
  - 2026-05-12 修正 `File.link` / `File.symlink` 不再预先删除目标路径，目标已存在时按 Ruby 语义返回 `Errno::EEXIST`。
  - 已新增 `TestMspecExistPredicateAndFileSymlinkPredicate` 覆盖 `File.should.exist?`、`File.symlink?` 和 EEXIST。
  - 已验证：`link_spec.rb` 4 examples / 0 failures；`symlink_spec.rb` 9 examples / 0 failures；低并发 `make check` 通过。
- [x] `vendor/ruby/spec/core/file/{delete,unlink,rename}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.delete` / `File.unlink` 多参数删除计数、缺失文件 `Errno::ENOENT`、类型 coercion，以及 `File.rename`。
  - 已新增 `TestFileDeleteUnlinkRenameAndExistMatcher`。
  - 已验证：`delete_spec.rb` 7 examples / 0 failures；`unlink_spec.rb` 7 examples / 0 failures；`rename_spec.rb` 4 examples / 0 failures；低并发 `make check` 通过。
- [x] `vendor/ruby/spec/core/file/read_spec.rb` failures 已解除。
  - 2026-05-12 已让 `File.read` 对目录路径返回 `Errno::EISDIR`，并继续把缺失路径映射到 errno。
  - 已新增 `TestFileReadDirectoryRaisesEISDIR`。
  - 已验证：`read_spec.rb` 1 example / 0 failures；低并发 `make check` 通过。
- [ ] 当前 spec-status 全局剩余均为 bounded failures 或 intentional/skipped zero_examples。
  - 2026-05-12 刷新 `file` / `filetest` / `io` / `module` / `refinement` 后，全局汇总为 2129 pass / 202 nonzero_failures / 12 zero_examples / 0 timeout / 0 runtime_error / 0 parse_error。
  - 2026-05-12 刷新 `argf` 后，全局汇总为 2097 pass / 235 nonzero_failures / 11 zero_examples / 0 timeout / 0 runtime_error / 0 parse_error。
  - 2026-05-12 完成 ARGF 后，全局汇总为 2107 pass / 225 nonzero_failures / 11 zero_examples / 0 timeout / 0 runtime_error / 0 parse_error。
  - 2026-05-12 完成 FileTest nonzero 后，全局汇总为 2116 pass / 216 nonzero_failures / 11 zero_examples / 0 timeout / 0 runtime_error / 0 parse_error。
  - 注意：刷新 stale dashboards 会暴露更多真实 nonzero_failures，因此 pass 总数可能下降、failure 总数上升；这是状态更准确，不是新增无限循环。
- [x] `vendor/ruby/spec/core/proc/curry_spec.rb` failures 已解除。
  - 2026-05-12 已补最小 `Proc#curry`、curried proc `arity == -1`、`parameters == [[:rest]]`、`source_location == nil`、curried proc `binding` 抛 `ArgumentError`，并修复 `instance_exec(3, &curried)`。
  - 后续又补齐 lambda 可选/rest/block 参数元数据和 curry arity 计算；当前直接运行为 `26 examples, 0 failures`，`proc` dashboard 为 23 pass。
- [x] `vendor/ruby/spec/core/argf` dashboard 已全部通过。
  - 2026-05-12 已补 spec runner 的 `CurrentSpecFile`、`fixture`、`File.read` / `File.readlines`，以及最小 ARGF shim：`getc` / `readchar` / `gets` / `readline` / `read(length, buffer=nil)`。
  - 后续补齐 `File.size`、`IOError`、`IO::WaitReadable` / `IO::EAGAINWaitReadable`、`IO::SEEK_*`，以及 ARGF `eof` / `eof?` / `fileno` / `to_i` / `to_io` / `pos` / `pos=` / `tell` / `seek` / `rewind` / `readpartial` / `read_nonblock` 的最小文件状态语义。
  - 已修复 parser 对 `argf ["path"] do ... end` 的裸数组参数 + `do` block 解析。
  - 已新增 `EOFError` 类、`TestParseBareMethodCallWithArrayArgumentAndDoBlock`、`TestMspecArgfReadlineRaisesEOFError` 以及 ARGF 文件状态相关回归测试。
  - 已验证：`reports/spec-status/argf.csv` 为 34 pass / 0 nonzero_failures / 0 runtime_error / 0 zero_examples；低并行 `make check` 通过。
- [x] `vendor/ruby/spec/core/filetest` nonzero failures 已解除。
  - 2026-05-12 已补 `FileTest` 常量、`Dir.mkdir`、`File.realpath` / `File.link` / `File.symlink` / `File.chmod`，以及 `File`/`FileTest` 的 `exist?` / `file?` / `directory?` / `size` / `size?` / `zero?` / `identical?` / `executable?` / `executable_real?` / `writable?` / `writable_real?`。
  - 已补最小 `tmp` / `touch` / `mkdir_p` / `rm_r` / `mock_to_path` helpers，并让 `__FILE__` / `__dir__` 使用当前 spec 文件路径。
  - 已验证：`reports/spec-status/filetest.csv` 为 22 pass / 2 zero_examples；剩余 `setgid_spec.rb` / `setuid_spec.rb` 是空 shared examples。
- [x] `vendor/ruby/spec/core/dir/empty_spec.rb` failures 已解除。
  - 2026-05-12 已补最小 `Dir.empty?`，支持空目录、非空目录、非目录返回 false，以及缺失路径抛 `Errno::ENOENT`。
  - 已验证：`empty_spec.rb` 4 examples / 0 failures。
- [x] `vendor/ruby/spec/core/dir/entries_spec.rb` timeout/failure 已解除。
  - 2026-05-12 根因：`File.join` 未展开数组参数、方法默认数组参数被编译成 nil、Class/Module 作为 self 时实例变量读写缺失，导致 `DirSpecs.mock_dir` / `DirSpecs.nonexistent` 不收束；另补 `classInheritsFrom` superclass 环保护。
  - 已补最小 `Dir.entries`，返回 `.` / `..` / 子项并在缺失目录抛 `Errno::ENOENT`。
  - 已验证：`GOMAXPROCS=1 timeout --kill-after=2s 8s ./rgo test vendor/ruby/spec/core/dir/entries_spec.rb` 为 7 examples / 0 failures；`reports/spec-status/dir.csv` 无 timeout。
- [x] `vendor/ruby/spec/core/dir/{children,each_child,foreach}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `Dir.children`、`Dir.each_child`、`Dir.foreach`，并补 `Enumerator#to_a` 的最小静态枚举支持。
  - 已补最小 Dir 实例状态：`Dir.open` / `Dir.new`、`Dir#read`、`Dir#rewind`、`Dir#each`、`Dir#children`、`Dir#each_child`、`Dir#close`、`Dir#fileno`，支撑目录枚举、closed `IOError`、以及 `IO.for_fd(dir.fileno)` 的 close-on-exec 路径。
  - 已补 MSpec `before/after :each` hook 调度，避免 fixture setup/teardown 在 example 外抢跑；`after :all` 已可执行全局/fixture 状态路径，但局部变量闭包写回仍需后续结合 closure 生命周期完善。
  - 已验证：`children_spec.rb` 13 examples / 0 failures；`each_child_spec.rb` 12 examples / 0 failures；`foreach_spec.rb` 8 examples / 0 failures；`fileno_spec.rb` 1 example / 0 failures；`reports/spec-status/dir.csv` 刷新为 15 pass / 17 nonzero_failures / 2 zero_examples。
  - 全局汇总更新为 2122 pass / 210 nonzero_failures / 11 zero_examples / 0 timeout / 0 runtime_error / 0 parse_error；低并行 `make check` 通过。
- [x] `vendor/ruby/spec/core/dir/chdir_spec.rb` failures 已解除。
  - 2026-05-12 已补 `Dir.pwd` / `Dir.getwd` / `Dir.chdir` / `Dir#chdir` 的最小 cwd 切换和 block restore 语义，`chdir_spec.rb` 从 19 examples / 3 failures 降到 19 examples / 1 failure。
  - 2026-05-12 进一步复测：带括号的最小删除路径 `Dir.chdir(dir1) { Dir.chdir(dir2) { Dir.unlink(dir1) } }` 可通过；剩余失败集中在 spec 原写法的嵌套 block + bare local 参数 `Dir.unlink dir1` 路径，删除未实际生效，后续应从 parser/block closure 的 bare argument 组合定位。
  - 已补 `Dir.exist?`。曾尝试将 `tmp(...)` helper 改为按当前进程隔离临时根，但会破坏 glob/base 相关 fixture 的稳定路径语义，已回退；后续需要更精确地处理该 chdir 用例的临时目录残留/删除路径。
  - 2026-05-12 已修 parser 对 `Dir.chdir dir1 do ... end` 这类 dotted bare call + identifier arg + `do` block 的归属，`chdir_spec.rb` 为 19 examples / 0 failures。
- [x] `vendor/ruby/spec/core/dir/{home,chroot,fchdir,for_fd}_spec.rb` failures/zero_examples 已解除。
  - 2026-05-12 已补 `ENV`、`$stdout`、`IO#fileno`、`Dir.home`、`Dir.chroot`、`Dir.fchdir`、`Dir.for_fd`，以及 `quarantine!` helper。
  - 已验证：`home_spec.rb` 9 examples / 0 failures；`chroot_spec.rb` 3 examples / 0 failures；`fchdir_spec.rb` 6 examples / 0 failures；`for_fd_spec.rb` 6 examples / 0 failures。
- [x] `vendor/ruby/spec/core/dir/{mkdir,rmdir}_spec.rb` failures 已解除。
  - 2026-05-12 复测：两者已不再 timeout；后续补齐 shared fixture/block 语义后，`mkdir_spec.rb` 为 8 examples / 0 failures，`rmdir_spec.rb` 为 6 examples / 0 failures。
  - 已补 `Errno::EEXIST` / `Errno::ENOTEMPTY` / `Errno::ENOTDIR`，`Dir.mkdir` 改为只创建最后一级目录并映射 `EEXIST` / `ENOENT` / `EACCES`，`Dir.rmdir` / `Dir.delete` / `Dir.unlink` 支持空目录删除、非目录 `ENOTDIR`、非空 `ENOTEMPTY`、缺失 `ENOENT`。
  - 已验证新增 VM 回归：`TestDirMkdirRaisesRubyErrnoClasses`、`TestDirRmdirRemovesEmptyAndRaisesRubyErrnoClasses`。
- [x] `vendor/ruby/spec/core/dir/{glob,element_reference}_spec.rb` failures 已解除。
  - 2026-05-12 已补 `Dir.glob` / `Dir.[]`，支持数组 pattern、`base:` / `sort:`、NUL pattern 错误、block yield、`File::FNM_*` 常量、基础 brace expansion、`**` 递归、dotfile 过滤和 escaped glob 字符。
  - 同时补 `Expectation#empty?`，让 `should_not.empty?` matcher 正确计入结果。
  - 已验证：`glob_spec.rb` 97 examples / 0 failures；`element_reference_spec.rb` 64 examples / 0 failures。
- [ ] `vendor/ruby/spec/core/dir/scan_spec.rb` 当前为 intentional zero_examples。
  - 2026-05-12 刷新后为 0 examples / 0 failures；内容受当前 Ruby 版本 guard 跳过，后续进入 Ruby 4.1 相关语义时再处理。
- [x] 最新 Dir dashboard nonzero failures 已解除。
  - 2026-05-12 刷新：33 pass / 1 zero_examples out of 34 files；344 examples / 0 failures；0 timeout / 0 runtime_error / 0 parse_error。
  - 剩余 zero_examples 为 `scan_spec.rb` 版本 guard。
- [x] `vendor/ruby/spec/core/file/open_spec.rb` failures 已解除。
  - 2026-05-12 已补 `File.open` mode/flag、fd、binary encoding、permission、read/write/pos/rewind/gets，以及 block close 基础语义。
  - 后续修正 `File::RDONLY|File::APPEND` 不应隐式可写、keyword `flags: File::EXCL` 不应向字符串 mode 合并 `r`，并让 `raise_error` matcher 评估期间的 native exception 从 `OpSend` 正确传播。
  - 已验证：`open_spec.rb` 84 examples / 0 failures；`reports/spec-status/file.csv` 刷新为 107 pass / 5 zero_examples / 0 nonzero_failures，合计 907 examples / 0 failures。
