# RGo 待办事项

## 本次调试记录（2026-06-21）

- [x] `vendor/ruby/spec/core/string/modulo_spec.rb` 已解除：从 `213 examples / 158 failures` 降到 `213 examples / 0 failures`。本轮补齐 `String#%` 操作符分派、基础 `sprintf` 数值/字符格式、Kernel.Integer/Kernel.Float 转换、部分非法格式校验、`$DEBUG=true` unused arguments、`to_ary` 参数展开校验、Hash.new named format 识别，以及当前 spec 覆盖的 encoding 错误。
- [x] `vendor/ruby/spec/core/kernel/{sprintf,format,printf,putc}_spec.rb` 已解除：当前分别为 `218/0`、`4/0`、`204/0`、`19/0`。本轮补齐 `format` verbose unused-argument warning、`printf` 的目标 IO / `$stdout` 写入语义、`putc` 的 Kernel 转发与 closed stream 错误，并修正 mspec `new_io` 默认可写临时 IO 行为。
- [x] `vendor/ruby/spec/core/kernel/{send,public_send,sleep,srand,system}_spec.rb` 已解除：当前分别为 `20/0`、`24/0`、`15/0`、`12/0`、`25/0`。本轮修正 `send`/`public_send`/`__send__` 的可变 arity、`sleep` 的真实 subsecond 等待、`Process.clock_gettime` 的真实浮点时间、`ruby_exe --disable-gems` 过滤、`ruby_cmd` 源码 quoting，以及 CLI `SystemExit` 状态码传播。
- [x] `vendor/ruby/spec/core/kernel/raise_spec.rb` 已解除：从 `72 examples / 28 failures` 降到 `72 examples / 0 failures`。本轮补齐 `OpRaise` 非异常对象 TypeError、`Kernel#raise` 基础参数校验、`Exception#cause` 字段/方法、`cause:` keyword 设置/校验、自动 cause chaining、circular cause 检测，并区分 `Exception.new` 构造出的异常对象与真正抛出的异常信号。
- [x] `vendor/ruby/spec/core/kernel/singleton_methods_spec.rb` 已解除：从 `85 examples / 35 failures` 降到 `85 examples / 0 failures`。本轮实现 `Kernel#singleton_methods`、`Module#ancestors` 最小反射语义，并修正 `Class#extend`/`Module#extend_object` 对 class receiver 的存储方式，使 extended module 方法在继承查询中可见但不会出现在 `singleton_methods(false)`。
- [x] `vendor/ruby/spec/core/module/autoload_relative_spec.rb` 已解除：从 `16 examples / 1 failure` 降到 `16 examples / 0 failures`。本轮修正 class/module body 内 unqualified class/module 定义优先使用当前词法常量容器，避免 `ModuleSpecs::AliasingSuper::Parent` 误命中外层 `ModuleSpecs::Parent` class，并补充 `autoload_relative` 加载已注册常量的回归测试。
- [x] `vendor/ruby/spec/core/string/sub_spec.rb` 与 `vendor/ruby/spec/core/string/gsub_spec.rb` 已解除：当前分别为 `65 examples / 0 failures`、`75 examples / 0 failures`。本轮补齐 `String#sub/#gsub/#sub!/#gsub!` 的可变 arity、TypeError 参数转换、bang FrozenError、无 replacement 的 Enumerator/ArgumentError 分支、block/hash replacement 基础语义、bang 修改检测，以及当前 spec 覆盖的 block replacement encoding compatibility。
- [x] `vendor/ruby/spec/core/dir/{inspect,mkdir}_spec.rb` 已解除：当前分别为 `3 examples / 0 failures`、`8 examples / 0 failures`。本轮实现 `Dir#inspect`，并修正 `Dir.mkdir` mode 参数缺少 `to_int` 时应把内部 `NoMethodError` 转为 Ruby 期望的 `TypeError`。
- [x] `vendor/ruby/spec/core/basicobject/instance_exec_spec.rb` 已解除：从 `17 examples / 1 failure` 降到 `17 examples / 0 failures`。本轮补齐 `Module#include` 的 `included` hook 调用，并让 `instance_exec` 的 class variable lookup/set 使用 block 词法 scope，同时保留 receiver eval context 供方法定义使用。
- [x] `vendor/ruby/spec/core/hash/empty_spec.rb` 已解除：从 `2 examples / 4 failures` 降到 `2 examples / 0 failures`。本轮修正 mspec `should.empty?` matcher，使其支持 Hash，并在无法直接识别容器内部结构时委托实际对象的 `empty?`。
- [x] `vendor/ruby/spec/core/hash/{new,clear,default,default_proc}_spec.rb` 已解除：当前分别为 `9/0`、`4/0`、`6/0`、`12/0`。本轮补齐 `Hash.new` 原生 ValueHash 构造、`capacity:` keyword 校验、位置参数/default block 冲突校验、`Hash#default/default=/default_proc/default_proc=` 基础语义、default proc `to_proc` coercion、lambda arity 校验，并让 `Hash#clear` 保留默认值/default proc。
- [x] `vendor/ruby/spec/core/hash/shift_spec.rb` 已解除：从 `8 examples / 2 failures` 降到 `8 examples / 0 failures`。本轮让 `Hash#shift` 按插入顺序移除首个键值对、同步维护 `RHash.Keys`，并对 frozen receiver 抛出 `FrozenError`。
- [x] `vendor/ruby/spec/core/hash/replace_spec.rb` 已解除：从 `12 examples / 2 failures` 降到 `12 examples / 0 failures`。本轮让 `Hash#replace` 复制参数 Hash 的 pairs/插入顺序/default/default_proc，并在 frozen receiver 上抛出 `FrozenError`。
- [x] `vendor/ruby/spec/core/hash/merge_spec.rb` 已解除：从 `14 examples / 1 failure` 降到 `14 examples / 0 failures`。本轮补齐 `Hash#merge` 对重复 key 的 block 调用、返回副本语义、插入顺序/default/default_proc 保留，并让 `Hash#merge!` 支持多个参数、重复 key block 和 frozen receiver。
- [x] `vendor/ruby/spec/core/hash/delete_spec.rb` 已解除：从 `7 examples / 2 failures` 降到 `7 examples / 0 failures`。本轮让 `Hash#delete` 在 missing key 时调用 block、对 frozen receiver 抛出 `FrozenError`，并同步维护 `RHash.Keys`。
- [x] `vendor/ruby/spec/core/hash/compare_by_identity_spec.rb` 已解除：从 `18 examples / 1 failure` 降到 `18 examples / 0 failures`。本轮实现 `Hash#compare_by_identity` / `compare_by_identity?`，在 `RHash` 中保存 identity 标记，并让 lookup、assignment、fetch、delete、merge/replace/dup/clone 保留或遵守 identity 语义。
- [x] `vendor/ruby/spec/core/hash/keep_if_spec.rb` 已解除：从 `9 examples / 2 failures` 降到 `9 examples / 0 failures`。本轮实现 `Hash#keep_if` 的原地过滤、frozen 检查，以及 no-block sized Enumerator。
- [x] `vendor/ruby/spec/core/hash/{reject,delete_if,constructor}_spec.rb` 已解除：当前分别为 `22/0`、`9/0`、`20/0`。本轮实现 `Hash#reject` 的新 Hash 返回语义、`Hash#reject!` / `Hash#delete_if` 的原地过滤和 no-block sized Enumerator，并补齐 `Hash.[]` / Hash 子类 `[]` 构造路径，确保子类构造不调用 `initialize` 且复制时不保留 default/default_proc/compare_by_identity。
- [x] `vendor/ruby/spec/core/hash/{compact,to_a}_spec.rb` 已解除：当前分别为 `9/0`、`2/0`。本轮实现 `Hash#compact` / `compact!` 的状态保留、nil value 过滤、frozen receiver 校验，并让 `Hash#to_a` / `entries` 按插入顺序返回 key/value pair 数组。
- [x] `vendor/ruby/spec/core/hash/{element_set,store}_spec.rb` 已解除：当前均为 `12/0`。本轮让 `Hash#[]=` / `Hash#store` 在 frozen receiver 上抛出 `FrozenError`，并显式注册 `store` 到同一实现。
- [x] `vendor/ruby/spec/core/hash/{flatten,values_at}_spec.rb` 已解除：当前分别为 `9/0`、`1/0`。本轮实现 `Hash#flatten` 的 `to_a.flatten(level)` 语义与参数 TypeError，并实现 `Hash#values_at` 复用 `Hash#[]` 的 default/default_proc lookup。
- [x] `vendor/ruby/spec/core/hash/{try_convert,fetch,fetch_values}_spec.rb` 已解除：当前分别为 `8/0`、`18/0`、`7/0`。本轮实现 `Hash.try_convert` 的 `to_hash` 转换与异常传播、`Hash#fetch` 的 KeyError receiver/key/default/block/arity 语义，并实现 `Hash#fetch_values`。
- [x] `vendor/ruby/spec/core/hash/{each,each_pair}_spec.rb` 已解除：当前均为 `12/0`。本轮修正严格 callable（`&method` / lambda）接收单个 `[key, value]` pair 的 Hash#each yield 语义，同时保留普通 block 的 key/value 解构。
- [x] `vendor/ruby/spec/core/hash/{lt,lte,gt,gte}_spec.rb` 已解除：当前均为 `10/0`。本轮实现 `Hash#<` / `<=` / `>` / `>=` 的 Hash/to_hash 参数转换、TypeError 和严格/非严格子集比较语义。
- [x] `vendor/ruby/spec/core/hash/{dig,initialize,rehash,transform_values,transform_keys}_spec.rb` 已解除：当前分别为 `9/0`、`7/0`、`6/0`、`15/0`、`23/0`。本轮补齐 `Hash#dig` 的 default/default_proc 与嵌套 `dig` 语义、`Hash#initialize`/`rehash` frozen receiver 校验、`Hash#transform_values!` 的原地转换/frozen/no-block Enumerator，以及 `Hash#transform_keys`/`transform_keys!` 的 hash 参数、block、Enumerator 和冲突 key 语义。
- [x] `vendor/ruby/spec/core/hash/{inspect,to_s,to_proc,ruby2_keywords_hash}_spec.rb` 已解除：当前分别为 `13/0`、`13/0`、`11/0`、`11/0`。本轮注册 Hash 自身 `inspect/to_s`，补齐 Ruby 3.3 风格 `=>` 格式、递归 Hash 展示和对象 `inspect`/`to_s` 调用；实现 `Hash#to_proc` 一元 lambda/default lookup；实现 `Hash.ruby2_keywords_hash` 的复制、标记、default/default_proc/compare_by_identity 和 Hash ivar 保留。
- [x] `vendor/ruby/spec/core/hash/{filter,select,update}_spec.rb` 已解除：当前分别为 `21/0`、`21/0`、`11/0`。本轮修正 numbered/implicit `it` parser preflight 对 `_1:` hash label 和 mspec `it "..."` DSL 的误判，使 shared examples 正常展开；并补齐 `Hash#select`/`filter` 的 Enumerator、顺序、compare_by_identity 保留，以及 `select!`/`filter!` 的原地过滤、nil/no-change 和 frozen 语义。
- [x] `vendor/ruby/spec/core/hash` 最新局部刷新已全绿：`69 pass / 69 files`，报告临时写入 `/tmp/rgo-hash-status.csv`。
- [x] `vendor/ruby/spec/core/nil` 最新局部刷新已全绿：`18 pass / 18 files`。本轮补齐 `NilClass#rationalize` 返回 `Rational(0, 1)` 兼容值、忽略单参数，并对多参数抛 `ArgumentError`。
- [x] `vendor/ruby/spec/core/threadgroup` 最新局部刷新已全绿：`5 pass / 5 files`。本轮把默认 ThreadGroup 对象挂到 `ThreadGroup::Default`，确保它与 `Thread.main.group` 是同一个 group。
- [x] `vendor/ruby/spec/core/builtin_constants/builtin_constants_spec.rb` 已解除：从 `17 examples / 9 failures` 降到 `17 examples / 0 failures`。本轮定义 `RUBY_VERSION`、`RUBY_PATCHLEVEL`、`RUBY_COPYRIGHT`、`RUBY_DESCRIPTION`、`RUBY_ENGINE`、`RUBY_ENGINE_VERSION`、`RUBY_PLATFORM`、`RUBY_RELEASE_DATE`、`RUBY_REVISION`，并让普通顶层常量查找回落到 `Object` 常量表。
- [x] `vendor/ruby/spec/core/enumerator/yielder` 最新局部刷新已全绿：`4 pass / 4 files`。本轮让 `Enumerator::Yielder#<<` 在多参数调用时抛 `ArgumentError`，同时保留单参数返回 self 和数组参数不二次包装的语义。
- [x] `vendor/ruby/spec/core/queue` 最新局部刷新已全绿：`15 pass / 15 files`。本轮修正 `Queue.new` 对初始 enumerable 的 `to_a` coercion：无 `to_a` 时按实际类名抛 TypeError，`to_a` 返回非 Array 时包含返回类型，且保留 `NoMethodError` 传播。
- [x] `vendor/ruby/spec/library/base64` 最新局部刷新已全绿：`6 pass / 6 files`。本轮补齐 `require "base64"` 的本地 shim，并实现 `Base64.strict_decode64` 的严格 CR/LF、padding、非法字符校验和 BINARY 编码结果。
- [x] `vendor/ruby/spec/library/shellwords/shellwords_spec.rb` 已解除：从 `7 examples / 2 failures` 降到 `7 examples / 0 failures`。本轮补齐 `require "shellwords"` 的本地 shim，并实现当前 spec 覆盖的 `Shellwords.shellwords` / `shellsplit` quote、escape、misquote `ArgumentError` 和双引号内反斜线 POSIX 语义。
- [x] `vendor/ruby/spec/library/timeout` 最新局部刷新已全绿：`timeout_spec.rb` 为 `6 examples / 0 failures`，`error_spec.rb` 为 `1 example / 0 failures`。本轮补齐 `require "timeout"` 的本地 shim、`Timeout` 模块、`Timeout::Error < RuntimeError`，并实现当前 spec 覆盖的 `Timeout.timeout` 返回值、负数参数和指定异常/消息语义。
- [x] `vendor/ruby/spec/library/English` 最新局部刷新已全绿：`2 pass / 2 files`，`27 examples / 0 failures`。本轮补齐 `require "English"` 的本地 shim 和标准 English 全局变量 aliases，并修正 VM 读取 alias global 时对 `$!` / `$@` 的动态解析，确保 rescue modifier 中 `$ERROR_INFO` / `$ERROR_POSITION` 可见。
- [x] `vendor/ruby/spec/core/kernel/eval_spec.rb` 剩余隐藏 failure 已解除：根因为双引号语义 heredoc 未解码 `\t`，导致 magic encoding 注释前的 tab 被保留为反斜线+t，eval 源跳过常量定义。本轮为非单引号 heredoc 补常见 escape 解码并新增 lexer 回归；`eval_spec.rb` 当前 `56 examples / 0 failures`。
- [x] `vendor/ruby/spec/core/kernel/{caller,exit}_spec.rb` 已解除：`caller_spec.rb` 当前 `14 examples / 0 failures`，`exit_spec.rb` 当前 `30 examples / 0 failures`。本轮修正顶层 VM 对未被 rescue 的 `SystemExit` 返回值继续执行的问题、补齐 `Object#exit!` private 方法、让 `exit!` 跳过 `at_exit` handlers，并把 `exit` 参数缺少 `to_int` 时的内部 `NoMethodError` 规整为 `TypeError`。
- [ ] `vendor/ruby/spec/core/kernel` 当前刷新剩余 2 个非 pass：`gsub_spec.rb` / `sub_spec.rb` 因整个文件受 `ruby_version_is ""..."1.9"` guard 包裹，当前均为 `zero_examples`。后续刷新 kernel 汇总时应把这类版本 guard 产物单独归类，避免误当作功能失败。
- [ ] VM 全包 Go 测试需要单独排查：本轮 focused 测试通过，但 `go test ./pkg/vm -count=1 -json` 从 `TestRequiredEnumerableEachDefinerYieldsAllElements` 即出现既有 fixture 失败（返回 Object 而非 Array），随后在 `TestRubyExeInThreadCanBeSignaledBeforeJoin` 附近以 143 结束。按项目规则暂记录，后续单独收敛全包 gate。
- [ ] `vendor/ruby/spec/core/data/to_s_spec.rb` 暂缓：当前运行时没有真正的 `Data` 类实现，`Data` 常量解析为 `nil`，导致 `DataSpecs::Measure` 等 fixture 依赖 mspec shim 漏洞继续执行；匿名递归 Data `to_s` 实际返回字符串 `"nil"`。后续应补原生 `Data.define`/实例存储/inspect/to_s 语义后再回收 data specs。

## 本次调试记录（2026-06-20）

- [ ] 后续需要实现真正的 Bignum/任意精度整数：当前 `12345678901234567890` 这类 literal 仍会落入 `int64` 溢出路径。本轮为解除 `SecureRandom.random_number` spec 的 bignum 上界断言，先在 mspec numeric matcher 中对“非负实际值 < 溢出负上界”做兼容处理；这不是完整 Bignum 语义。

## 本次调试记录（2026-06-19）

- [x] 已解除 `vendor/ruby/spec/language/END_spec.rb` 剩余失败：补齐 `ruby_exe` lightweight lifecycle 对 nested `at_exit`/`END`、handler 内 `exit`、handler/main exception 文案、`-r` fixture 后主脚本 parse error 的模拟；并修复 `String#lines` 默认保留行分隔符。
- [x] 已解除 `vendor/ruby/spec/language/return_spec.rb` 剩余失败：顶层 `return 10` 现在输出 `warning: argument of top-level return is ignored` 且保持退出码 0。
- [x] focused 回归通过：`TestRubyExeEndHandlerExitSkipsRemainingHandlerBody`、`TestRubyExeTopLevelReturnArgumentWarnsAndExitsZero`、`TestRubyExeNestedAtExitRunsImmediatelyAfterOuterHandler`、`TestRubyExeEndSharedExceptionScenarios`、`TestRubyExeEndHandlerSeesLastMainException`、`TestRubyExeRequiredEndHandlerRunsWhenMainScriptParseFails`、`TestStringLinesPreservesDefaultRecordSeparators`、`TestEvalIgnoresSpacedCallPatternInsideComments`、`TestRaiseErrorMatcherPrefersUnhandledBlockExceptionOverRescuePreviousException`、`TestBareConstantLookupFallsBackToObjectConstants`、`TestUnmatchedRescueRunsEnsureBeforeOuterRescue`、`TestUnmatchedSplatRescueReraisesOriginalException`、rescue splat current-exception focused tests。
- [x] `vendor/ruby/spec/language` 最新刷新已全绿：`80 pass / 80 files`，`2863 examples / 0 failures`。
- [x] `./scripts/safe_go_test.sh ./...` 最新运行已全绿；此前剩余的 `TestUnmatchedRescueRunsEnsureBeforeOuterRescue` / `TestUnmatchedSplatRescueReraisesOriginalException` 已通过 rescue active-frame stack-depth guard 与 `LastRaisedResult` 作用域修复解除。

## 本次调试记录（2026-06-17）

- [x] 上次记录的 Go 工具链阻塞已解除：当前环境可运行 `go version`（`go1.26.4-X:nodwarf5 linux/amd64`）。
- [x] 已恢复 `vendor/ruby/spec` submodule：`git submodule update --init --recursive vendor/ruby/spec` 成功 checkout 到 `9b3f5ffd67174671135dcb3d93a1f0fd3f7df218`。
- [x] 重新运行 focused Go 测试后，`pkg/parser` 已通过；此前 `pkg/vm` 失败集中在 Process 相关测试，当前已解除：
  - `TestProcessSpawnWaitAndLastStatus`：spawn pid 与 waited pid 不一致，实际返回 `[#<23>, 10000, 10000, 127]`。
  - `TestProcessWait2AndWaitallUsePendingChildren`：`pkg/core/init.go:8713` 的 `processWait2` 出现 nil pointer panic。
- [x] Process focused VM 测试已刷新：`RGO_GO_TEST_TIMEOUT=20 ./scripts/vm_test_status.sh reports/vm-process-test-status.csv 'Process'` 当前 `29 pass / 0 error`。本轮通过 `rubyExeCommand()` 替换 `Process.spawn("ruby ...")` 的真实 Ruby 依赖，并让 `Process.wait` 在取真实子进程状态前等待后台 `cmd.Wait()` 完成，解除 pid/status mismatch 与 `processWait2` panic。
- [x] VM 全量逐测试报告已刷新：`RGO_GO_TEST_TIMEOUT=20 ./scripts/vm_test_status.sh reports/vm-test-status.csv '^Test'` 完成，当前 `922 tests` 全部 pass。本轮已解除系统 `ruby` 缺失导致的 `FileWrite` / `touch` / 普通 `ruby_exe` 失败，Process wait/status 失败，以及真实 CLI 子进程无参数 `sleep` 不阻塞导致的 signal trap 竞态。
  - 已确认 `command -v ruby` 当前无输出；`ruby_exe` 注册于 `pkg/core/init.go:1520`，实现入口 `methodRubyExe` 在 `pkg/core/init.go:9506`。本轮已让 no-arg `ruby_exe` 优先返回当前仓库 `./rgo`，并给 `rgo` CLI 增加 `-e <code>` 与 `ARGV` 支持。
  - 2026-06-18 follow-up：已修复 `rgo -e 'puts ARGV[0]' hello` 输出 `hello`、`rgo run <script> <path>` 中 `File.write(ARGV[0], ...)` 可落盘；`ARGV` 现在同时写入 VM 顶层常量表与 `Object::ARGV`。
  - 2026-06-18 follow-up：已确认根因不是 signal handler 注册，而是 CLI 子进程的无参数 `sleep` 立即返回，父进程发 TERM 前子进程已退出；已在真实 CLI 路径设置 `RGO_REAL_SLEEP=1`，并让非协作式线程上下文的无参数 `sleep` 进入宿主阻塞，保留 VM 内协作式线程/fork 现有行为。`TestRubyExeInThreadCanBeSignaledBeforeJoin` 与 `./scripts/safe_go_test.sh ./pkg/vm -count=1` 当前均通过。
- [x] 非 VM Go 包测试已验证通过：`./scripts/safe_go_test.sh ./cmd/rgo ./pkg/compiler ./pkg/core ./pkg/lexer ./pkg/object ./pkg/parser ./pkg/parser/ast` 全部 pass/no test files；`./scripts/safe_go_test.sh ./pkg/vm -count=1` 当前也通过。
- [x] 脚本自测已验证通过：`scripts/safe_go_test_test.sh`、`scripts/vm_test_status_test.sh`、`scripts/spec_status_test.sh` 均 PASS；`spec_status_test.sh` 中预期的 invalid memory limit 分支输出 `Invalid RGO_TEST_MEMORY_KB: invalid`。
- [x] `make lint` 当前已通过；已删除 `pkg/core/init.go` 中 `cmd.Stderr` / `cmd.Stdout` 的无效自赋值。
- [x] focused Ruby spec gate 已恢复可运行：`language/block_spec.rb` 通过，`166 examples / 0 failures`。
- [ ] `vendor/ruby/spec/language` 当前刷新结果：`76 pass / 80 files`，`2864 examples / 6 failures`。失败文件为：
  - `vendor/ruby/spec/language/BEGIN_spec.rb`：`7 examples / 1 failures`
  - `vendor/ruby/spec/language/END_spec.rb`：`14 examples / 2 failures`
  - `vendor/ruby/spec/language/return_spec.rb`：`43 examples / 1 failures`
  - `vendor/ruby/spec/language/source_encoding_spec.rb`：`6 examples / 2 failures`
  - 已保存 failure logs 到 `reports/spec-status/language-failure-logs/`，但当前日志只显示 examples 汇总，没有展开具体断言信息；后续需要进一步定位 mspec failure 输出。
- [ ] Ruby spec 全量 gate 已刷新：`RGO_SPEC_TIMEOUT=5 RGO_TEST_MEMORY_KB=2000000 ./scripts/full_spec_gate.sh --ruby-only` 完成，报告写入 `reports/spec-status/ruby-spec-full.csv`，共 `3809 specs`。
  - 当前结果：`2571 pass`、`890 nonzero_failures`、`36 runtime_error`、`310 zero_examples`、`2 timeout`，合计 `33248 examples / 6115 failures`。
  - timeout 仅剩 2 个：`vendor/ruby/spec/core/enumerator/lazy/force_spec.rb`、`vendor/ruby/spec/core/rational/exponent_spec.rb`。
    - `force_spec.rb` 日志显示已通过 `passes given arguments to receiver.each`、nested lazy 的 `calls all block and returns an Array` / `works with an infinite enumerable` 后超时。
    - `rational/exponent_spec.rb` 日志显示已通过 Rational 与 Integer exponent 的前几个场景，在 Bignum 场景中通过 `returns Rational(0) when self is Rational(0) and the exponent is positive` 后超时。
  - 非 pass 顶层分布：`core 643`、`library 540`、`optional 31`、`command_line 13`、`security 7`、`language 4`。
  - 本次全量运行产生未跟踪空文件 `192.0.2.1`，疑似 net/http 或 socket spec 副产物；尚未删除。
- [ ] Rails gate 当前前置阻塞已结构化：`./scripts/full_spec_gate.sh --rails-only` 在缺少 `/home/jimxl/Documents/projects/rgo/vendor/rails/rails` 时退出 0，并写入 `reports/spec-status/rails-spec-full.csv`：`rails,target_missing,0,2,`。当前 `git submodule status` 只列出 `vendor/ruby/spec`，仓库内没有 `vendor/rails` 目录；后续仍需要恢复 Rails vendor tree 或调整 Rails gate 目标。

## 本次调试记录（2026-06-16）

- [x] 当前执行环境缺少 Go 工具链，`scripts/spec_status.sh` 在构建 `rgo` 时失败：`go: command not found`。已确认 `go` / `tinygo` / `gccgo` 均不在 PATH，且仓库根目录没有可复用的 `./rgo` 二进制；尝试 `sudo pacman -S --needed --noconfirm go` 需要交互式 sudo 密码，当前工具无法完成。另，`vendor/ruby/spec` 是空的 submodule 目录，focused Ruby spec gate 还需要先执行 submodule 初始化。2026-06-17 已确认 Go 工具链可用，并已恢复 `vendor/ruby/spec` submodule。

## 本次调试记录（2026-06-04）

- [x] `rescue A, *[B]` 这类 rescue splat 数组字面量 parser 边界问题已修复；新增 parser/vm 回归测试覆盖数组字面量 splat rescue。
- [x] `vendor/ruby/spec/core/array/fixtures/classes.rb` 通过 `require_relative` 动态加载时，正常 `begin ... rescue NameError ... end` 被动态语法校验误判为 `SyntaxError: unexpected rescue modifier`，导致 `ArraySpecs.frozen_array` fixture 未加载并连带影响 `append_spec` 等 frozen array 场景。已改为逐行校验 rescue 子句，并保留 `VM.New` 前设置的 `CurrentSpecFile`；已验证 `append_spec.rb` / `at_spec.rb`。
- [x] 2026-06-07 刷新 `vendor/ruby/spec/language` 后暴露的 3 个当前 spec 回归已解除：`block_spec.rb` / `method_spec.rb` 的 `&nil` block 拒收语义，以及 `predefined_spec.rb` 的 `$@` 赋值校验和 rescue `$!` 生命周期。已刷新 `reports/spec-status/language-current.csv`：81 pass / 2871 examples / 0 failures。
- [x] 2026-06-07 全量 Ruby spec timeout 跟进：`vendor/ruby/spec/core/enumerator/lazy/select_spec.rb` 已解除 timeout。补齐 `Enumerator::Lazy#select/filter/take/first/force`、`Array#lazy`、`Range#lazy`、`Object#to_enum/enum_for` 的最小运行路径，并修复 `&:even?` block pass 与 `Range#first(n)`；已验证该 spec 11 examples / 0 failures。
- [x] 2026-06-13 全量 Ruby spec timeout 跟进：`vendor/ruby/spec/core/enumerable/select_spec.rb`、`vendor/ruby/spec/core/struct/select_spec.rb`、`vendor/ruby/spec/core/string/{ljust,rjust,center}_spec.rb` 已解除 timeout。修复 `String#ljust/#rjust/#center` exact-width padding、empty pad `ArgumentError`、String subclass receiver/pad coercion panic、基础 encoding 传播；已验证对应 5 个 spec 文件均 pass。
- [x] 2026-06-13 剩余 Ruby spec timeout 复测：`vendor/ruby/spec/core/io/select_spec.rb` 与 `vendor/ruby/spec/core/process/kill_spec.rb` 在 `RGO_SPEC_TIMEOUT=10` 下均已通过；根因由 `Kernel#loop` 误用 `currentThread != nil` 的一轮保护解除。
- [x] 2026-06-13 `vendor/ruby/spec/core/kernel/{global_variables,catch,throw,proc,public_send,remove_instance_variable,at_exit,sleep,kind_of,is_a,instance_of,initialize_copy,clone,singleton_class,define_singleton_method,extend,instance_variable_get,instance_variable_set,abort,system}_spec.rb` 已解除。补齐内建 `Array`/`Hash`/`Range` 对 VM `Enumerable` module 的 include 关系，修复 `catch` 无 block 的 `LocalJumpError`、非 Symbol throw tag 的 identity 匹配、unmatched throw 使用 `UncaughtThrowError`，`Kernel#proc` 无 block 时抛出 `ArgumentError`，`public_send` 参数校验错误的 backtrace 顶层 frame，`remove_instance_variable` 在 frozen receiver 前先校验实例变量名，shared fixture fallback/absolute path 与 `at_exit` 无 block 的 `ArgumentError` 语义，`sleep` duration 校验、返回 Integer 和 stale `LastException` 传播问题，`kind_of?`/`is_a?`/`instance_of?` 参数校验与 module ancestry 判断，`initialize_copy` 的返回 self、frozen receiver 和 same-class 校验，`clone(freeze:)` keyword、frozen state 与 `initialize_clone` 调用语义，`String#-@` frozen deduped string 与 frozen String singleton class 拒收语义，`define_singleton_method` 参数校验、per-receiver singleton method、UnboundMethod owner 校验和 spec DSL `test` 不再污染所有 Object 实例，`extend` 的参数、类型和 frozen receiver 校验，`instance_variable_get` 名称校验与 spec DSL `stub!` 返回值安装，`instance_variable_set` 名称校验和 frozen receiver 前的校验顺序，`abort` 的 `$stderr` 捕获、非 String `TypeError`、`IOStub` 和 require-loaded closure global name 解析，以及 `system` 的同步命令执行、shell 选择、`exception:` 和 `$?` 状态。

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

## 阶段 2：Spec 全量执行检查（2026-06-02）

### Language block follow-up（2026-06-02）

- [x] `vendor/ruby/spec/language/block_spec.rb` 中 `to_ary` 自身 raise 的场景已解除（2026-06-04）：根因为 rescue exception expression 会吞掉 `=> e` 并丢失变量绑定；已补 parser 停止 hash rocket 与 rescue variable binding 回归。已验证：165 examples / 0 failures。
- [x] `vendor/ruby/spec/language/singleton_class_spec.rb` 已解除（2026-06-03）：已修复 singleton class `new/allocate` TypeError、对象/string/class singleton superclass 链、按需 effective singleton class、`Class#superclass`、Class/Module `==` 等价、mspec `be_kind_of` 对 singleton effective class 的判断、`require` 动态语法校验进入 class/module 时保留 method scope，以及 mspec `bignum_value` helper。已验证：53 examples / 0 failures。
- [x] `vendor/ruby/spec/language/variables_spec.rb` 已解除（2026-06-03）：已补多赋值 `to_ary` / `to_a` 非 Array TypeError、嵌套 MRHS splat coercion、非 ASCII 大写常量动态赋值 SyntaxError，以及 `respond_to?` 默认 `respond_to_missing?` 路径。已验证：118 examples / 0 failures。
- [x] `vendor/ruby/spec/language/numbered_parameters_spec.rb` 已解除（2026-06-03）：已补动态 eval 中 numbered parameter 赋值、显式 block 参数、嵌套 numbered block 的 SyntaxError 校验；`raise_error` message matcher 只对 numbered parameter 相关断言启用 message 校验，避免扩大既有 shim 文案差异。已验证：12 examples / 0 failures。
- [x] `vendor/ruby/spec/language/it_parameter_spec.rb` 已解除（2026-06-04）：已补 Ruby 3.4 隐式 `it` 参数检测、block/proc/lambda arity 与参数元数据、`it` 与 numbered parameter 混用 SyntaxError，以及动态 eval 语法检查忽略嵌套字符串字面量。已验证：15 examples / 0 failures。
- [x] `vendor/ruby/spec/language/constants_spec.rb` 回归已解除（2026-06-04）：`raise_error` matcher 中 scoped constant 缺失的 `NameError` 不再被后续 `::` 链覆盖成 `TypeError`。已验证：100 examples / 0 failures。
- [x] `vendor/ruby/spec/language/proc_spec.rb` 已解除（2026-06-04）：已补 `**nil` 拒收关键字语义，以及 lambda/proc 单解构参数 `|(a, b)|` 在 arity 检查前执行 `to_ary` coercion。已验证：40 examples / 0 failures。
- [x] `vendor/ruby/spec/language/lambda_spec.rb` 已解除（2026-06-04）：已补 `lambda` 方法必须有显式 block、匿名 keyword rest `**` 不接收位置参数、`**nil` 拒收关键字，以及单解构 lambda 参数的 autosplat/coercion 顺序。已验证：68 examples / 0 failures。
- [x] `vendor/ruby/spec/language/def_spec.rb` 已解除（2026-06-04）：已补 frozen 对象/类/singleton class 定义方法时的 `FrozenError`、`FrozenError#receiver`、重复 rest 参数 `SyntaxError`，以及 eval 中 class method 定义不污染实例方法表。已验证：73 examples / 0 failures。
- [x] `vendor/ruby/spec/language/pattern_matching_spec.rb` 已解除（2026-06-04）：已补 eval 中非法 pattern 的 `SyntaxError` 预检，并为 `Object[]` / `Object[a: ...]` pattern 增加 `deconstruct` / `deconstruct_keys` 返回类型校验。已验证：113 examples / 0 failures。
- [x] `vendor/ruby/spec/language/method_spec.rb` 已解除（2026-06-04）：已补 method call splat 的 `to_a` 语义、空格方法调用参数列表动态 `SyntaxError`、匿名/命名 keyword rest、`**nil` 与空 keyword splat、裸 hash rocket / `**` keyword send 标记，以及普通位置 Hash 不应满足 keyword 方法 arity 的边界。已验证：168 examples / 0 failures。
- [x] Language 历史刷新记录（2026-06-04）：当时已解除 `def_spec.rb`、`pattern_matching_spec.rb`、`method_spec.rb`、`class_spec.rb`、`predefined_spec.rb`、`regexp/back-references_spec.rb`、`rescue_spec.rb`、`block_spec.rb` 等收尾项，并刷新 `reports/spec-status/language-current.csv` 为 81 files pass / 0 failures。2026-06-07 已重新解除当前 spec 回归并刷新为 81 pass / 0 failures。

### Ruby Spec（`vendor/ruby/spec`）
- 已新增并验证 `scripts/full_spec_gate.sh` 可稳定跑通所有 ruby spec 文件，报告写到 `reports/spec-status/ruby-spec-full.csv`。
- 运行参数（当前验证）：`RGO_SPEC_TIMEOUT=10 RGO_SPEC_CPU_SECONDS=120 RGO_TEST_MEMORY_KB=2000000 ./scripts/full_spec_gate.sh`
- 结果：
  - `pass`: 2039
  - `nonzero_failures`: 804
  - `runtime_error`: 81
  - `zero_examples`: 880
  - `timeout`: 7
- 阻塞点：
  - 存在大量 `nonzero_failures` 与 `zero_examples`，多数文件仍未通过（总计 3,811 个 ruby spec 文件中 1,772 个非 pass），需按目录分批修复。

### Rails Spec（`vendor/rails/rails`）
- 已将全量 rails 任务接入 `scripts/full_spec_gate.sh`，默认任务为：
  - `activesupport:test actionpack:test actionview:test activemodel:test activejob:test actionmailer:test actionmailbox:test actiontext:test actioncable:test activestorage:test activerecord:test:sqlite3:test railties:test`
- 当前阻塞在依赖安装：`bundle check` 失败，未通过 `bundle exec rake` 启动，任务统一报 `bundle_missing`。
- 报告状态：12 个任务全部 `bundle_missing`，日志写在 `reports/spec-status/rails-task-logs/bundle_check.log`。
- 建议复位动作：
  - 在 `vendor/rails/rails` 下执行 `bundle install` 后再继续 `./scripts/full_spec_gate.sh --rails-only`。
  - 已复核：
    - `bundle install --local`：当前系统无本地 gem 缓存可满足依赖清单。
    - `bundle install`：当前环境无法访问 `index.rubygems.org`（`Could not reach host index.rubygems.org`）。

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

### IO class-method blockers（2026-05-19）

- [x] `IO` 类方法可见性与检索不一致：`IO.methods` 当前未包含 `IO.pipe` / `IO.for_fd` / `IO.copy_stream` 等应有方法，`IO.respond_to?` 与 `IO.method(:name)` 行为与 `ClassMethods` 不一致。已补齐类方法枚举与类接收者查找链（`methodSingletonClass` 首次创建单例类时同步 `ClassMethods`）。
- [x] `IO.copy_stream` 已与 Ruby spec 对齐边界行为核对：类方法可见性与路径分发稳定化，补齐偏移量不变更源位置、管道 offset 异常、对象 read/write 分发与长度行为等回归场景。

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

- [x] `vendor/ruby/spec/language/return_spec.rb` 剩余 1 个失败已解除（2026-05-23）
   - 已修复动态源码中 `def m; next; end` / `def m; redo; end` 应匹配 `SyntaxError` 的路径，并补了 focused VM 回归。
   - 已修复 `File.write` 类方法未注册导致临时 fixture 不落盘的问题；新增 `TestFileWriteClassMethodCreatesFile`。
   - 已修复 “within a block within a class”：`{ return }` 不再误吞 block terminator，class body block 中的 `return` 匹配 `LocalJumpError`。
   - 当前已验证：`vendor/ruby/spec/language/return_spec.rb` 43 examples / 0 failures。

- [x] `vendor/ruby/spec/language/source_encoding_spec.rb` 剩余 2 个失败已解除（2026-05-23）
   - 已修复 `touch(path, "wb")` 可选 mode 不被接受、block 内 `f.write` 因 IO shim 缺少 writable mode 而不落盘的问题；新增 `TestTouchWithModeYieldsWritableFile`。
   - 已补 `String#bytesize`、按 byte 返回的 `String#bytes`、`Array#pack('C*')`，并修复 `\xNN` 字符串 escape 保留 raw byte；新增 focused lexer/VM 回归。
   - 已验证：`vendor/ruby/spec/language/source_encoding_spec.rb` 6 examples / 0 failures。

- [x] `vendor/ruby/spec/language/symbol_spec.rb` 中 block 参数解构已解除（2026-05-23）
   - 已修复非 lambda block 多参数接收单个 Array 时的解构行为；新增 `TestBlockDestructuresSingleArrayArgumentForMultipleParams`。
   - 已验证：`vendor/ruby/spec/language/symbol_spec.rb` 13 examples / 0 failures。

- [x] `vendor/ruby/spec/language/ensure_spec.rb` 剩余 2 个 backtrace 断言已解除（2026-05-23）
   - 已修复 `lambda { raise; ensure; }` 在动态源码中应匹配 `SyntaxError` 的路径；新增 `TestEnsureInsideBraceBlockMatchesSyntaxErrorMatcher`。
   - 已修复普通编译路径下 `__LINE__` 返回 `nil` 的问题；新增 `TestLineKeywordCompilesToSourceLine`。
   - 已修复局部变量减数字面量被误编译为 bare method call 的窄场景；新增 `TestLocalVariableMinusLiteralCompilesAsSubtraction`。
   - 已修复 `raiseException` 中 `ensureActive` 上报和回溯构造链路：避免异常分发后误清空状态，并将 ensure 回溯的第二帧收敛为 `block` 标签与正确源行映射。
   - 当前已验证：`vendor/ruby/spec/language/ensure_spec.rb` 31 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/character_classes_spec.rb` 剩余 2 个失败已解除（2026-05-23）
   - 已补 `RegexpError` 类与 `Regexp.new` 对未闭合 unicode property 的错误路径；新增 `TestRegexpNewUnterminatedUnicodePropertyRaisesRegexpError`。
   - 已让动态 `eval('/[[:alpha:]-[:digit:]]/')` 的非法字符类范围匹配 `SyntaxError`；新增 `TestInvalidRegexpCharacterClassRangeMatchesSyntaxErrorMatcher`。
   - 已验证：`vendor/ruby/spec/language/regexp/character_classes_spec.rb` 126 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/interpolation_spec.rb` 剩余 2 个失败已解除（2026-05-23）
   - 已让 regexp literal 标记并编译 `#{...}` interpolation，通过 `Regexp.new` 在运行时构造 pattern。
   - 已补 malformed interpolation 产生 `RegexpError` 的窄路径；新增 `TestInterpolatedRegexpMalformedPatternRaisesRegexpError`。
   - 已验证：`vendor/ruby/spec/language/regexp/interpolation_spec.rb` 9 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/escapes_spec.rb` 剩余 3 个失败已解除（2026-05-23）
   - 已补动态 regexp literal 中非法 `\x` / `\c` escape 的 `SyntaxError` 验证；新增 `TestInvalidRegexpEscapesMatchSyntaxErrorMatcher`。
   - 已验证：`vendor/ruby/spec/language/regexp/escapes_spec.rb` 13 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/modifiers_spec.rb` 剩余 3 个失败已解除（2026-05-23）
   - 已让 lexer 捕获 `/a` modifier 并在动态 regexp syntax validation 中拒绝 unsupported modifier。
   - 已补 `(?o)` / `(?o:)` inline modifier 的 `SyntaxError` 验证；新增 `TestInvalidRegexpModifiersMatchSyntaxErrorMatcher`。
   - 已验证：`vendor/ruby/spec/language/regexp/modifiers_spec.rb` 11 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/grouping_spec.rb` 剩余 3 个失败已解除（2026-05-23）
   - 已补动态 regexp literal unbalanced grouping 的 `SyntaxError` 验证。
   - 已补 `Regexp.new("(?<1a>a)")` / `Regexp.new("(?<-a>a)")` 的 `RegexpError` 验证；新增 `TestInvalidRegexpGroupingMatchesExpectedErrors`。
   - 已验证：`vendor/ruby/spec/language/regexp/grouping_spec.rb` 7 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp/encoding_spec.rb` 剩余 6 个失败已解除（2026-05-23）
   - 已补 regexp match/match?/=~ 对非 ASCII-compatible string encoding、fixed encoding mismatch、broken UTF-8 byte sequence 的错误路径。
   - 已补 `Regexp::FIXEDENCODING` 与 `Regexp.new(..., FIXEDENCODING)` fixed encoding metadata；新增 `TestRegexpEncodingMismatchRaisesExpectedErrors`。
   - 已验证：`vendor/ruby/spec/language/regexp/encoding_spec.rb` 32 examples / 0 failures。

- [x] `vendor/ruby/spec/language/regexp_spec.rb` 剩余 10 个失败已解除（2026-05-23）
   - 已补动态 `%r` malformed delimiter 的 `SyntaxError` 验证；新增 `TestInvalidPercentRegexpDelimitersMatchSyntaxError`。
   - 已补 mspec expectation `=~` 对 Regexp actual 的 dispatch，并把 spec 覆盖的 conditional regexp forms 规范化为可执行 pattern；新增 `TestConditionalRegexpPositiveMatches`。
   - 已验证：`vendor/ruby/spec/language/regexp_spec.rb` 25 examples / 0 failures。

- [x] `vendor/ruby/spec/language/super_spec.rb` 剩余 3 个失败已解除（2026-05-23）
   - 已让 missing super target 抛出带 `super` 文案的 `NoMethodError`，不再静默返回 `nil`。
   - 已标记 `define_method` 生成的函数，并让 implicit-argument `super` 从该路径抛出 `RuntimeError`；新增 `TestSuperMissingAndDefineMethodImplicitArgsRaiseExpectedErrors`。
   - 已验证：`vendor/ruby/spec/language/super_spec.rb` 61 examples / 0 failures。

- [x] `vendor/ruby/spec/language/class_variable_spec.rb` 剩余 3 个失败已解除（2026-05-23）
   - 已让 top-level class variable read/write 抛出 `RuntimeError: class variable access from toplevel`。
   - 已让祖先类后续定义同名 class variable 时，子类读取 overtaken class variable 抛出 `RuntimeError`；新增 `TestClassVariableToplevelAndOvertakenAccessRaiseRuntimeError`。
   - 已验证：`vendor/ruby/spec/language/class_variable_spec.rb` 14 examples / 0 failures。

- [x] `vendor/ruby/spec/language/retry_spec.rb` 剩余 4 个失败已解除（2026-05-23）
   - 已补动态 syntax validation：`retry` 只能出现在 rescue body 内，其他 eval 场景匹配 `SyntaxError`；新增 `TestInvalidRetryMatchesSyntaxErrorMatcher`。
   - 已验证：`vendor/ruby/spec/language/retry_spec.rb` 3 examples / 0 failures。

- [x] `vendor/ruby/spec/language/throw_spec.rb` 剩余 4 个失败已解除（2026-05-23）
   - 已让 unmatched throw 抛出 `ArgumentError`，并保持 catch/throw label 类型严格匹配（string 不匹配 symbol）。
   - 已补 `UncaughtThrowError` 类，并隔离 Thread block 的 parent catch stack；新增 `TestThrowUnmatchedAndThreadExitRaiseExpectedErrors`。
   - 已验证：`vendor/ruby/spec/language/throw_spec.rb` 10 examples / 0 failures。

- [x] `vendor/ruby/spec/language/metaclass_spec.rb` 剩余 4 个失败已解除（2026-05-23）
   - 已让 `class << true/false/nil` 返回对应 immediate class，并让 integer/symbol singleton class 打开抛出 `TypeError`。
   - 已补 scoped constant lookup 对非 class/module receiver 抛出 `TypeError`，并拆分 `Object#dup` / `Object#clone`：`dup` 丢弃 singleton class 常量，`clone` 保留。
   - 已验证：`vendor/ruby/spec/language/metaclass_spec.rb` 21 examples / 0 failures。

- [x] `vendor/ruby/spec/language/module_spec.rb` 剩余 6 个失败已解除（2026-05-23）
   - 已让 `module Existing::Const` 在 Const 已存在但不是 Module 时抛出 `TypeError`，不再覆盖为新模块。
   - 已补 lambda 中 captured local 作为 scoped module root（如 `module container::Value`）的常量解析。
   - 已验证：`vendor/ruby/spec/language/module_spec.rb` 16 examples / 0 failures。

- [x] `vendor/ruby/spec/language/assignments_spec.rb` 剩余 5 个失败已解除（2026-05-23）
   - 已让 scoped constant plain assignment 走 `OpSetScopedConstant`，保证 RHS 先求值，再对非 class/module receiver 抛出 `TypeError`。
   - 已补动态 eval 对 Ruby 3.4 index assignment 中 block arg / keyword arg 的 `SyntaxError` 验证。
   - 已验证：`vendor/ruby/spec/language/assignments_spec.rb` 38 examples / 0 failures。

- [x] `vendor/ruby/spec/language/return_spec.rb` 剩余 1 个失败已解除（2026-05-23）
   - 已修复 `{ return }` 中 `return` 后紧跟 `}` 时 parser 错误吞掉 block terminator 的问题。
   - 已让 class/module body 中执行的 block 使用 `return` 时抛出 `LocalJumpError`，同时保留 class body 直接 `return` 的 `SyntaxError`。
   - 已验证：`vendor/ruby/spec/language/return_spec.rb` 43 examples / 0 failures。

- [x] `vendor/ruby/spec/language/constants_spec.rb` 剩余 2 个失败已解除（2026-05-23）
   - 已修复 non-class/non-module top-level constant qualifier（如 `CS_CONST1::CS_CONST`）应抛出 `TypeError`。
   - 已补动态 eval 中 method 内 constant assignment 的 `SyntaxError: dynamic constant assignment` 验证。
   - 已移除按 `Class#name` 推断 lexical parent 的 constant fallback，避免 `class A::B; def self.x; C; end; end` 错误搜索 `A::C`。
   - 已补 `NameError#receiver/#name` 元数据、private constant owner 追踪、bare namespace module 合成，以及 scoped lookup 对 core qualified constants / `Process::*` constants / autoload nested constants 的兼容。
   - 已验证：`vendor/ruby/spec/language/constants_spec.rb` 100 examples / 0 failures。

- [x] Task 4 后 `vendor/ruby/spec/language/optional_assignments_spec.rb` timeout 已解除
   - 已修复 `super()` parser 空参数列表不终止问题，focused regression PASS。
   - 已补充并修复 `super(1 + 2)` parenthesized args 不终止回归；新增 `TestParseSuperWithParenthesizedArgumentsTerminates`，先 RED timeout 后 PASS。
   - Task 1 follow-up 刷新命令：`RGO_SPEC_TIMEOUT=1 scripts/spec_status.sh vendor/ruby/spec/language reports/spec-status/language.csv`，写入 80 个 specs。
   - 最新 language dashboard：25 pass, 0 timeout, 1 runtime_error, 51 nonzero_failures, 3 parse_error, 0 compile_error, 0 zero_examples out of 80 files（2026-05-16 refreshed）。
   - 最新 selected blocker：`vendor/ruby/spec/language/optional_assignments_spec.rb` status is pass（74 examples / 0 failures）；duration 为易变值不在 TODO 固定记录。
   - 2026-05-16 follow-up：新增 `TestUndefinedScopedConstantCompoundAssignmentsRaiseNameError`，修复 scoped constant `&&=` / compound assignment 对未定义常量应触发 `NameError` 的行为；selected blocker 已解除。
   - 2026-05-23 follow-up：新增 runtime scoped `defined?` 检查和 `remove_const` 清理 VM qualified constant cache，修复 `Object::A &&=` / `Object::A +=` 在 spec cleanup 后残留的问题；已验证 74 examples / 0 failures。
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
- [ ] `vendor/ruby/spec/core/kernel/require_spec.rb`
  - 根因：当前闭包/线程 shim 对 `t2 = nil; t1 = Thread.new { Thread.pass until t2 }; t2 = Thread.new { ... }` 这类 sibling thread 后续赋值可见性支持不足，会在并发 require fixture 里出现共享状态问题；并发分组目前仍未恢复且该文件仍有 51 例失败。
  - 已修复：`Process::Status` 实例化对象在 `Process::Status#clone` 等共享路径上不再触发 `interface conversion` panic（`*core.processStatusData` 被误当成 `*object.Object` 的断言崩溃）。
  - 已补救：`singleton_class` / `Class` 变量设置 / `Module#extend_object` 在 `ValueObject` 且非普通 `*object.Object` 时改为返回可恢复错误分支，不再直接 panic。
  - 已修复：`require` / `require_relative` 参数处理改为严格的 `pathFromSingleArg + coercePath` 流程（单参数校验 + `to_path`/`to_str` 转换 + 类型报错），不再对非字符串/参数不足返回 `false`。
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
  - 当前 `RGO_SPEC_TIMEOUT=1` 结果：25 pass, 0 timeout, 1 runtime_error, 51 nonzero_failures, 3 parse_error, 0 compile_error, 0 zero_examples out of 80 files（2026-05-16 refreshed）。
  - 当前观测到 2636 examples / 397 failures。
  - `vendor/ruby/spec/language` dashboard 当前未全部通过。
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

- [x] `&block` 方法参数已有最小实现：解析器保留 `BlockParam`，编译器记录 block 局部槽，VM 调用方法时把当前 block 写入该局部变量，并支持 `p.call` 常量 block。`TestBlockPassedAsProcCapturesOuterLocal` 与 `TestBlockPassedAsProcCapturesEarlierOuterLocal` 已恢复执行，验证 `block` 能稳定捕获方法外层局部变量。

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
- [x] `vendor/ruby/spec/core/string/gsub_spec.rb` 状态污染型 timeout 已解除。
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

- [ ] `File.open` FIFO 并发阻塞模型仍有阻塞回归。
  - `ioWaitForFIFOPeer` 在 writer/reader 同时 `Thread.new` 的场景里仍会长期阻塞，不一定触发 `reader` 的调度执行。
  - 复现：`RGO_SPEC_TIMEOUT=30 ./rgo test /tmp/file-open-fifo-x-no-clean2.rb`
  - 标记：需继续优化 FIFO 条件等待与协作式线程调度/唤醒语义。
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

### Kernel 并发 require blocker（2026-05-24）

- [x] `vendor/ruby/spec/core/kernel/shared/require.rb` 的 `Thread.current[:...]=...` 下标赋值解析错误已修复。`vendor/ruby/spec/core/kernel/require.rb`/`shared/require.rb` 已通过纯 parser 验证，可复用的回归用例已加入 `pkg/parser/parser_test.go`。
- [ ] `vendor/ruby/spec/core/kernel/require_spec.rb` 运行期仍在 `ProcessStatus#clone` 路径触发 panic（当前测试中止于 `interface conversion: interface {} is *core.processStatusData, not *object.Object`）；需补齐该运行时兼容分支后继续恢复并发分组行为验证。

### 当前内部测试失败（2026-06-02，已修复）

- [x] `pkg/vm` 的 `TestKernelLoopReturnsEnumeratorStopResult` 当前失败。
  - 复现：`scripts/safe_go_test.sh ./...`
  - 现象：`pkg/vm/executor_test.go:4306` 期望 `loop { e.next }` 在 Enumerator 结束时返回 `:stopped`，当前得到 `nil`。
  - 根因：`core.Init()` 会重建 runtime，但没有清掉包级 `currentThread` / `currentFiber`，导致前序测试调用 `Thread.current` 后，后续顶层 `loop` 被误判为 thread/fiber 内执行并提前返回 `nil`。
  - 修复：`core.Init()` 复位 `currentThread` / `currentFiber`；新增 `TestKernelLoopIgnoresPreviousThreadCurrentState` 覆盖顺序依赖。

### Kernel loop timeout（2026-06-02，已修复）

- [x] `vendor/ruby/spec/core/kernel/loop_spec.rb` timeout 已解除。
  - 根因：`Enumerator#each` 的 loop enumerator 分支在 yielded block 设置 `LastException` 时没有返回异常，导致 `raise ... unless args.empty?` 这类 block 异常被无限循环吞掉。
  - 修复：`enumeratorEach` 在 loop/static enumerator yield 后检查并返回 `LastException`；新增 `TestSpecRunnerLoopEnumeratorBreakDoesNotPoisonNextExample` 覆盖真实 spec runner 复现。
  - 已验证：`RGO_SPEC_TIMEOUT=10 scripts/spec_status.sh vendor/ruby/spec/core/kernel/loop_spec.rb /tmp/rgo-loop-final.csv` 为 10 examples / 0 failures。

### Language variables multiassign follow-up（2026-06-03，已修复）

- [x] `vendor/ruby/spec/language/variables_spec.rb` nested MRHS splat coercion 失败已解除。
  - 已修复单 RHS MLHS `to_ary` 非 Array TypeError、单 LHS splat `*a = x` 的 `to_ary` TypeError、MRHS splat `to_a` 非 Array TypeError，以及非 ASCII 大写常量动态赋值的 SyntaxError 识别。
  - 最后一例定位：`-> { a, *b, (c, d) = 1, 2, 3, *x }.should raise_error(TypeError)`，其中 `x.should_receive(:to_ary).and_return(x)`。
  - 根因：`respond_to?` 内部以普通 public call 调用默认 private `respond_to_missing?`，导致 MRHS splat 的 `to_a` 检查覆盖了前面的 TypeError；已补默认 `Object#respond_to_missing?` 并让 `respond_to?` 内部直接调用 builtin 默认实现。

### Language break follow-up（2026-06-03）

- [x] `vendor/ruby/spec/language/break_spec.rb` 已解除（2026-06-03）：captured block `&b` 转发、`yield` 作为裸调用参数、active Proc block break unwind、以及 super block forwarding 的 `break` 传播均已对齐。
  - 已修复动态源码中非法 `break` 的 SyntaxError 校验：`def m; break; end` 与 class/module body 中 `break` 现在匹配 SyntaxError。
  - 已修复 captured block 直接 `Proc#call` 的 `break`：非 lambda proc call 遇到 `BlockBreak` 时返回 `LocalJumpError`，对应 `break_spec.rb` 从 7 failures 降到 4 failures。
  - 已修复 `note yield` 一类裸调用参数解析：`yield` 现在可作为 method-call argument，不再被拆成两个独立语句。
  - 已为 `&block` 转成的 Proc 记录 break owner frame：owner 仍在栈上时 `break` 按 block unwind 传播；owner 已退出时转为 `LocalJumpError`。
  - 已验证：`vendor/ruby/spec/language/break_spec.rb` 39 examples / 0 failures。

### Array fixture dynamic require follow-up（2026-06-06，已修复）

- [x] `vendor/ruby/spec/core/array/fixtures/classes.rb` 经 `require_relative` 动态加载后，`ArraySpecs` 模块存在但 `ArraySpecs::MyArray` 为 `nil`，导致 `ArraySpecs::MyArray[1, 2, 3]` 实际变成 `nil.[]` 并在 `select_spec.rb` 的 subclass example 卡住。
  - 已确认最小 `module M; class C < Array; end; end` 在普通 `rgo run` 中可正常定义 `M::C`，问题限定在动态 require/eval fixture 路径。
  - 根因是 VM `OpArray` 对数组字面量硬限制 `n > 100`，fixture 中 `CHI_SQUARED_CRITICAL_VALUES` 有 101 个元素，导致 module body 在该常量处中断，后续 `MyArray` 等常量没有执行。
  - 已将限制调整为 `StackSize` 边界并新增 `TestLargeArrayLiteralConstantInModuleBodyContinuesExecution`；已验证 `select_spec.rb` 20 examples / 0 failures。

### Command-call argument precedence follow-up（2026-06-06）

- [ ] `puts (1..5).to_a.inspect` / `puts (1..5).include?(3)` 当前在 feature smoke 中会先输出 `1..5`，说明 command-call 参数后的 dot-chain 优先级仍与 Ruby 存在差异。
  - 当前 `Range#to_a` / `Range#include?` 本身可用；`scripts/feature_test.sh` 已改为显式 `puts((1..5).to_a.inspect)` 和 `puts((1..5).include?(3))`，避免该 parser follow-up 阻塞 array gate。

### Rational exponent bignum follow-up（2026-06-13）

- [ ] `vendor/ruby/spec/core/rational/exponent_spec.rb` 已解除 `0 ** -1` / `Rational(..., bignum_value) ** -4` 触发的 Go integer divide-by-zero panic，但完整 spec 继续进入 bignum exponent examples 后运行时间过长。
  - 已补 `0 ** -1` 抛出 `ZeroDivisionError` 的 VM 回归，并让 `OpPow` 对 exception 结果走 `raiseException`。
  - 后续需要补真正的 Rational/bignum 表示与指数语义；当前 `Rational()` 仍主要降级为 Integer/Float，无法通过该文件的精确 Rational 断言。

### BasicObject instance_exec class variable scope（2026-06-13）

- [ ] `vendor/ruby/spec/core/basicobject/instance_exec_spec.rb` 仍有 1 个 failure：`base.instance_exec { @@count = 2 }` 在 `def self.included(base)` 的 block 中应写入 block 定义处的模块 class variable scope（`BasicObjectSpecs::InstExec`），当前写到了 `instance_exec` receiver 的模块 scope。
  - 最小复现：`module M; def self.run(base); base.instance_exec { @@x = 1 }; end; end; module N; end; M.run(N)` 后当前 `N.class_variables == [:@@x]`，但应为 `M.class_variables == [:@@x]`。
  - 已确认普通 `module M; @@x = 1; end` class variable 存储可用；问题集中在 singleton method 内创建 block 时没有捕获方法的 lexical class variable scope。

### Enumerable fixture loop follow-up（2026-06-13）

- [x] `EnumerableSpecs::EachDefiner#each` 经 `require_relative` 加载后只 yield 首个元素的问题已解除。
  - 根因：`require` 路径会调用 `Thread.current` 初始化 `currentThread`，而 `Kernel#loop` 用 `currentThread != nil` 作为线程内一轮执行保护，导致后续普通 loop 都只执行 1 轮。
  - 已改为通过 VM 的 thread block depth 判断真实线程 block 执行上下文，并保留 fiber body 的一轮保护；已验证 `vendor/ruby/spec/core/enumerable` 61 files 全部 pass。

### Kernel caller mspec runner frame follow-up（2026-06-21）

- [ ] `vendor/ruby/spec/core/kernel/caller_spec.rb` 当前已不是 runner frame failure；最新局部刷新状态为 `zero_examples`，需要后续定位为什么该文件在单独/目录刷新时未注册 examples。
  - 已修复同文件中 `puts caller(0)` 的数组逐行输出、`__LINE__ - 1` 被误解析为 bare method call、以及 at_exit caller 多出 `__main__` 帧/缺少 `block in <main>` label 的问题。
  - runner frame 兼容已不再是当前可复现失败点；后续先查 example 注册/guard 状态。
