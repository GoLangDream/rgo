# Struct Implementation Plan

> **For agentic workers:** Execute inline with test-driven development; do not use subagents or Git.

**Goal:** 完整实现 Ruby Struct 并清零 core/struct RubySpec。

**Architecture:** 类元数据保存字段与 keyword 模式；Struct 基类提供共享行为；生成类只提供访问器和初始化。实例成员存储与普通 ivar 分离。

**Tech Stack:** Go、RGo VM、vendored RubySpec。

## Global Constraints

- 重任务 `GOMAXPROCS=1 GOFLAGS=-p=1 nice -n 10` 串行运行。
- RubySpec 使用内存、CPU 和 30 秒超时限制。
- 使用 `apply_patch`；不使用 Git。

### Task 1: 元数据与构造

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [ ] 添加失败回归：字段列表、默认/true/false keyword_init、block self/参数、错误路径。
- [ ] 运行聚焦测试确认按预期失败。
- [ ] 保存类字段元数据并实现统一初始化和访问器。
- [ ] 运行 `new_spec.rb`、`initialize_spec.rb` 与聚焦 Go 测试。

### Task 2: 基础成员 API

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [ ] 添加失败回归：`[]`, `[]=`, members, length/size, values/to_a, each/each_pair, values_at。
- [ ] 运行聚焦测试确认失败。
- [ ] 在 Struct 基类实现成员查找、边界错误、冻结检查和 Enumerator size。
- [ ] 运行对应 RubySpec。

### Task 3: 转换、筛选与 dig

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [ ] 添加失败回归：to_h block、select/filter、dig 嵌套和错误传播。
- [ ] 运行聚焦测试确认失败。
- [ ] 实现转换、筛选、dig 与无 block Enumerator。
- [ ] 运行对应 RubySpec。

### Task 4: 值语义与表示

**Files:** `pkg/core/init.go`, `pkg/vm/executor_test.go`

- [ ] 添加失败回归：`==`, `eql?`, hash、inspect/to_s、递归结构、普通 ivar 分离。
- [ ] 运行聚焦测试确认失败。
- [ ] 实现递归安全的比较、hash、inspect 和复制语义。
- [ ] 运行对应 RubySpec。

### Task 5: 完成验证

**Files:** `TODO.md`

- [ ] 串行运行 `vendor/ruby/spec/core/struct`，要求 30 files / 0 failures。
- [ ] 运行相关 Go 测试和 Marshal RubySpec 回归。
- [ ] 更新 TODO，刷新 core 基线并选择下一失败簇。
