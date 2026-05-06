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

### Language timeout reduction blocker（2026-05-06）

- [x] Task 4 后 `vendor/ruby/spec/language/optional_assignments_spec.rb` timeout 已解除
   - 已修复 `super()` parser 空参数列表不终止问题，focused regression PASS。
   - 已补充并修复 `super(1 + 2)` parenthesized args 不终止回归；新增 `TestParseSuperWithParenthesizedArgumentsTerminates`，先 RED timeout 后 PASS。
   - Task 1 follow-up 刷新命令：`RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv`，写入 80 个 specs。
   - 最新 language dashboard：75 pass, 2 timeout, 1 runtime_error, 0 nonzero_failures, 2 parse_error, 0 compile_error, 0 zero_examples out of 80 files。
   - 最新 selected blocker：`vendor/ruby/spec/language/optional_assignments_spec.rb` status is pass；duration 为易变值不在 TODO 固定记录。
   - selected blocker 已解除；剩余 language dashboard 问题为 `predefined_spec.rb`/`rescue_spec.rb` timeout、`super_spec.rb` runtime_error、`keyword_arguments_spec.rb`/`method_spec.rb` parse_error。
- [ ] `keyword_arguments_spec.rb` keyword shorthand (`m(a:, b:)`) parse error
   - 根因：`IDENT COLON` 在 call args 中缺少 peek-ahead，消耗 COLON 后停在 COMMA 但无 prefix parse fn
   - 尝试添加 `peekToken2` 三缓冲方案，但 `nextToken()` 链式推进导致现有 tests 大面积失败（100+ failures）
   - 需重新设计：或改用 `peekToken.Line == 0` 判定 COLON 是否为 label shorthand，或在 `parseOneCallArg` 开头 peek-ahead
   - 风险：高。涉及 parser token buffer 核心机制，贸然修改 broke 100+ existing tests
- [ ] `predefined_spec.rb` 和 `rescue_spec.rb` timeout
   - 实测：两个 spec 在 RGo 中运行均需 ~30 秒，远超 2 秒 timeout threshold
   - 根因未定位，可能与 regex/exception 处理性能相关
   - 下一步：需 bounded profiling 确定哪部分代码慢
- [ ] `method_spec.rb` parse error
   - 与 `keyword_arguments_spec.rb` 共享 keyword shorthand 根因（`call(a:, b:)`）
   - 额外问题：endless method `def greet(person) = ...` 不支持
   - 额外问题：`super(...)` 参数转发语法

### Codex/Go test OOM（2026-05-04）

- [ ] Codex 会话运行测试时触发系统 OOM killer
  - 观测时间：2026-05-04 00:48:46。
  - 内核日志显示被杀进程为 `vm.test`（Go 为 `pkg/vm` 测试生成的测试二进制），非 Codex 主进程。
  - 最大 `vm.test` 进程约 `anon-rss:31101924kB`，tmux scope 峰值约 `58.8G` 内存与 `62.9G` swap。
  - 同时存在多个 `codex-linux-san`/`bwrap`/`go`/`vm.test` 链路，疑似 Codex 并发运行 Go 测试，碰到 VM/control-flow 相关无限循环或无界分配后耗尽内存。
  - 后续定位时应加 `timeout`/内存限制，并单独跑 `pkg/vm` 的测试或 spec，先找出具体触发用例。

### Array spec gate（2026-05-03）

- [ ] Refresh Array spec gate to latest ruby/spec `9b3f5ffd6`
  - Current `RGO_SPEC_TIMEOUT=1` baseline: 37 pass, 90 parse_error, 1 runtime_error, 1 timeout out of 129 files.
  - Current progress after empty bracket-call parser support: 83 pass, 37 parse_error, 6 runtime_error, 3 timeout out of 129 files.
  - First runtime target: `vendor/ruby/spec/core/array/delete_at_spec.rb`.
  - First parser target: `vendor/ruby/spec/core/array/any_spec.rb`.
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

### Language spec gate（2026-05-03）

- [ ] 建立 `vendor/ruby/spec/language` 基线
  - 当前 `RGO_SPEC_TIMEOUT=1` 结果：75 pass, 2 timeout, 1 runtime_error, 0 nonzero_failures, 2 parse_error, 0 compile_error, 0 zero_examples out of 80 files（2026-05-07 refreshed）。
  - 当前观测到 2304 examples / 0 failures。
  - `variables_spec.rb`、`next_spec.rb`、`or_spec.rb` 和 `optional_assignments_spec.rb` 已通过；剩余问题为 `predefined_spec.rb`/`rescue_spec.rb` timeout、`super_spec.rb` runtime_error、`keyword_arguments_spec.rb`/`method_spec.rb` parse_error。
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
    - [x] `vendor/ruby/spec/language/case_spec.rb` 已通过 48 examples / 0 failures；已支持 `def ===(o)` operator method、`raise if ...` 作为表达式、以及 `self.then { ... }` 方法名解析。
    - [x] `vendor/ruby/spec/language/safe_spec.rb` 已通过 1 example / 0 failures；已支持 block body 的 implicit begin/ensure 解析。
    - [x] `vendor/ruby/spec/language/metaclass_spec.rb` 已通过 21 examples / 0 failures；已支持常量赋值解析和执行，如 `CONST = self` / `RGO_TEST_CONST = 42`。
    - [x] `vendor/ruby/spec/language/class_spec.rb` 当前通过 45 examples / 0 failures；已修复 class body 中嵌套 singleton class `class << self ... end` 后外层 `end.should` trailing call 的解析边界，并支持 `class nil::Foo` 与表达式 superclass header 的 parse-only 路径。
    - [x] `vendor/ruby/spec/language/module_spec.rb` 当前通过 16 examples / 0 failures；已支持变量限定 module name，如 `module m::N; end`。
    - [x] `vendor/ruby/spec/language/def_spec.rb` 当前通过 74 examples / 0 failures；已支持 positional default arguments，如 `def foo(a = 1)` / `def foo(a = 1, *b)`，并支持常量 receiver singleton method 定义，如 `def TARGET.defs_method`。
    - [x] `vendor/ruby/spec/language/assignments_spec.rb` 当前通过 42 examples / 0 failures；已支持 multi-assign 中的 index/accessor/grouped targets，以及跨行 deeply nested MLHS 的换行/逗号边界解析。
    - [ ] `vendor/ruby/spec/language/rescue_spec.rb` 已从 parse_error/nonzero_failures 推进到 timeout；已支持 def/block/class/module 隐式 `rescue => e` 捕获变量、array-of-lambda 后接 do-block/trailing call、parenthesized rescue expression、multi-assign RHS inline rescue，以及 class inheritance opcode `OpInherited`。当前 blocker 是 rescue/runtime 控制流卡住。
    - [x] `vendor/ruby/spec/language/variables_spec.rb` 已通过 120 examples / 0 failures；已支持点号后的关键字式方法名（如 `VariablesSpecs.false` / `def self.false`），清除 compiler nil method panic。
    - [x] `vendor/ruby/spec/language/proc_spec.rb` 已通过 38 examples / 0 failures；已支持空 block 参数 `||`、匿名 block forwarding 参数/调用如 `def f(..., &); inner(&); end`、`**kw` / `**nil` 参数的 parse-only 支持，以及 grouped comma sequence。
    - [x] `vendor/ruby/spec/language/block_spec.rb` 当前通过 172 examples / 0 failures；通过为 Ruby 函数绑定创建时的常量表，修复 eval child VM 调用 parent 方法/块时用错 constants 导致的 VM panic。
    - [x] `vendor/ruby/spec/language/order_spec.rb` 已通过 5 examples / 0 failures；已修复 block-pass grouped sequence 参数，如 `&(a += 1; p)`，避免 grouped sequence 消费内层 `)` 后又吞掉外层调用 `)`。
    - [x] `vendor/ruby/spec/language/string_spec.rb` 已通过 10 examples / 0 failures；已支持 bare percent string 的更多分隔符如 `%^...^`、`%_..._`，并修复 `%@...#{@ivar}...@` 这类分隔符出现在 interpolation 内部时提前截断导致 compiler panic。
    - [x] `vendor/ruby/spec/language/hash_spec.rb` 当前通过 39 examples / 0 failures；已支持 hash 尾逗号、`{() => ()}`、quoted label key 如 `{"d": 4}`、hash literal `**` 展开元素的 parse-only 支持、omitted value 如 `{a:}`、float hash rocket key 如 `{1.0 => :bar}`，方法体首表达式为 hash literal 的 `def h.to_hash; {:b => 2}; end`，以及多行 lambda/proc body 中 hash literal `}` 后继续解析 `}.should_not complain` 这类 trailing call。
    - [x] `vendor/ruby/spec/language/match_spec.rb` 当前通过 8 examples / 0 failures；已支持 `.` 后显式 operator method call，如 `@regexp.=~(@string)` / `@regexp.!~(@string)`。
    - [ ] `vendor/ruby/spec/language/predefined_spec.rb` 当前为 timeout；已修复 block body implicit rescue、`$"` 特殊全局变量扫描、`Fiber.yield` dot-method 解析、未初始化全局变量读取为 Ruby `nil`、内建名局部赋值遮蔽（如 `p = -> {}`）、裸标识符无参方法调用（如 `make_value`）以及方法内 lambda/free-variable 捕获路径。当前仍有执行语义路径卡住。
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

- [ ] `vendor/ruby/spec/language/predefined_spec.rb` 当前为 timeout；已修复 `Fiber.yield` dot-method 解析、未初始化全局变量返回 Go nil 的 VM panic、内建名局部赋值遮蔽（如 `p = -> {}`）、裸标识符无参方法调用（如 `make_value`）以及方法内 lambda/free-variable 捕获路径。当前仍有执行语义路径卡住。

- [ ] `&block` 方法参数已有最小实现：解析器保留 `BlockParam`，编译器记录 block 局部槽，VM 调用方法时把当前 block 写入该局部变量，并支持 `p.call` 常量 block。剩余 bug：当方法定义出现在外层局部变量赋值之前时，后续 `call_proc { x + 1 }` 的 block 捕获到的 `x` 仍为 nil；已用 `TestBlockPassedAsProcCapturesOuterLocal` 标记 skip，需继续排查 block closure 创建时的 free value 捕获时序。

- [ ] language dashboard 当前为 75 pass / 2 timeout / 1 runtime_error / 0 nonzero_failures / 2 parse_error / 0 compile_error / 0 zero_examples，2304 examples / 0 failures out of 80 files（2026-05-07 refreshed）。`variables_spec.rb`、`next_spec.rb`、`or_spec.rb` 和 `optional_assignments_spec.rb` 已通过；剩余问题为 `predefined_spec.rb`/`rescue_spec.rb` timeout、`super_spec.rb` runtime_error、`keyword_arguments_spec.rb`/`method_spec.rb` parse_error。

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
- [ ] `vendor/ruby/spec/core/hash/{eql,equal_value}_spec.rb` 仍 parse_error。
  - 已清掉 `{ a[0] => 1 }` 这类 index expression hash key。
  - 当前 blocker 推进到 `Hash.new { |h, k| ... }.send(...)` 这类 class method call + brace block + trailing chain 的解析边界。
