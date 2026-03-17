# RGo Ruby 4.0 完整 Spec 实现计划

## TL;DR

> **目标**: 实现 100% Ruby 4.0 spec 通过 (core + language + library),全部用 Go 原生实现
> 
> **交付物**:
> - 完整的 Ruby 4.0 解释器 (纯 Go 实现)
> - 通过所有 Ruby spec 测试 (~3500+ 文件)
> - 性能目标: 初期 <10x MRI,最终超越 MRI
> - CI/CD 自动化测试系统
> - MSpec 适配器 (Go ↔ Ruby spec 桥接)
> 
> **预估工作量**: 4-5 年 (兼职)
> **并行执行**: 是 - 多波次并行开发
> **关键路径**: 基础设施 → 核心类 → 标准库 → 并发特性

---

## Context

### 原始需求
用户要求: "分析 rgo 对 ruby spec 的实现完成情况，并生成一个为了完成所有 spec,确保能正确执行的计划"

### 访谈总结

**关键决策**:
- **目标版本**: Ruby 4.0.1 (2025-12-25 发布,包含最新特性)
- **工作模式**: 兼职开发
- **项目性质**: 生产级解释器
- **性能要求**: 初期 <10x MRI 慢,最终目标超越 MRI
- **完成标准**: 100% spec 通过 (core + language + library ~3500+ 文件)
- **实现方式**: 全部用 Go 原生实现,不依赖 Ruby 运行时
- **CI 状态**: 当前无,需要建立
- **排除项**: C 扩展 (用 Go 等价实现替代,如 sqlite 驱动)

**研究发现**:
- 当前状态: 16/59 核心类 (27%),247 测试通过
- VM 状态: 80 opcodes 定义,45 实现 (56%)
- 关键缺失: 异常处理 (0%),块/闭包 (50%),类型检查 (0%),模块系统 (25%)
- Ruby spec 规模: Kernel (117 文件), String (114), Array (102), Hash (69), Integer (67)

### Metis 审查

**识别的风险**:
- MSpec 适配器可行性 (项目杀手级风险)
- 异常处理可能需要 VM 重写 (2 周 vs. 12 周)
- 范围蔓延 (标准库功能庞大)
- 兼职工作的时间线不确定性

**建议的防护措施**:
- 明确排除列表 (防止范围蔓延)
- 阶段级验收标准 (具体通过率)
- 回归预防策略 (CI 自动化)
- 原型验证任务 (MSpec 适配器,异常处理架构)

---

## Work Objectives

### 核心目标
实现完整的 Ruby 4.0 解释器,通过所有 Ruby spec 测试,性能超越 MRI。

### 具体交付物
1. **基础设施** (Phase 0-1):
   - CI/CD 系统 (GitHub Actions)
   - MSpec 适配器 (Go ↔ Ruby spec 桥接)
   - 异常处理系统 (begin/rescue/ensure/raise)
   - 块/闭包系统 (yield, block_given?, Proc, Lambda)
   - 类型检查系统 (is_a?, respond_to?, kind_of?)
   - 模块系统 (include, extend, prepend)

2. **核心类** (Phase 2-3):
   - 完整实现 59 个核心类
   - Enumerable 模块 (所有集合的基础)
   - Kernel 模块 (117 个全局方法)
   - Array, String, Hash, Integer, Float (完整方法集)
   - Range, Symbol, Regexp (完整实现)
   - Numeric, Rational, Complex (数值类型)
   - Time, Date, DateTime (时间处理)
   - IO, File, Dir (文件系统)
   - Exception 层次结构 (完整异常类)

3. **标准库** (Phase 4-6):
   - JSON (Go 原生 JSON 解析器)
   - Net::HTTP (Go 原生 HTTP 客户端)
   - FileUtils (Go 原生文件操作)
   - ERB (Go 原生模板引擎)
   - Digest (Go 原生哈希算法)
   - URI (Go 原生 URI 解析)
   - CSV, YAML (Go 原生解析器)
   - OpenSSL (Go crypto 包装)

4. **并发特性** (Phase 7):
   - Thread (Go goroutine 映射)
   - Fiber (Go 协程实现)
   - Mutex, Queue (Go sync 包装)

5. **Ruby 4.0 新特性** (Phase 8):
   - Pattern matching (case/in)
   - Numbered parameters (_1, _2)
   - Endless methods (def foo = bar)
   - Rightward assignment (=> pattern)

### Definition of Done
- [ ] 100% Ruby spec 通过 (core + language + library)
- [ ] 所有 247 个现有测试仍然通过
- [ ] CI 在 <30 分钟内运行完整测试套件
- [ ] 性能 <10x MRI (初期),最终目标超越 MRI
- [ ] 所有功能用 Go 原生实现 (无 Ruby 依赖)

### Must Have
- 完整的异常处理系统
- 完整的块/闭包系统
- 完整的模块系统
- 所有核心类的完整方法集
- 标准库的 Go 原生实现
- Ruby 4.0 新特性支持

### Must NOT Have (防护措施)
- ❌ C 扩展兼容性 (用 Go 等价实现)
- ❌ Rails 兼容层 (超出范围)
- ❌ JIT 编译 (性能优化阶段)
- ❌ 垃圾回收调优 (依赖 Go GC)
- ❌ 字节码优化 (初期不优化)
- ❌ MRI 内部特性 (ObjectSpace, TracePoint 等,除非 spec 要求)

---

## Verification Strategy

> **零人工干预** — 所有验证都是代理执行的。不允许例外。
> 需要"用户手动测试/确认"的验收标准是禁止的。

### 测试决策
- **基础设施存在**: 否 (需要建立)
- **自动化测试**: MSpec 适配器 + Go 测试套件
- **框架**: MSpec (Ruby spec 官方框架) + Go testing
- **方法**: 增量 spec 合规性 - 每个阶段跟踪通过率

### QA 策略
每个任务必须包含代理执行的 QA 场景 (见下面的 TODO 模板)。
证据保存到 `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`。

- **语言特性**: 使用 MSpec 运行 language/ specs
- **核心类**: 使用 MSpec 运行 core/ specs
- **标准库**: 使用 MSpec 运行 library/ specs
- **性能**: 使用 Go benchmark 测试

---

## Execution Strategy

### 并行执行波次

> 最大化吞吐量,将独立任务分组到并行波次中。
> 每个波次完成后才开始下一个。
> 目标: 每波 5-8 个任务。少于 3 个任务/波 (除最终波) = 拆分不足。

由于项目规模巨大 (4-5 年),我们将其分为 8 个主要阶段,每个阶段包含多个并行波次。

---

## TODOs

> 实现 + 测试 = 一个任务。永远不要分开。
> 每个任务必须有: 推荐代理配置 + 并行化信息 + QA 场景。
> **没有 QA 场景的任务是不完整的。不允许例外。**

---

## Phase 0: 项目基础设施 (预估 4-6 周兼职)

**目标**: 建立 CI/CD、测试框架、开发工具链

### Wave 0.1 - CI/CD 和测试基础 (并行启动)

- [x] 1. 建立 GitHub Actions CI 工作流

  **What to do**:
  - 创建 `.github/workflows/test.yml`
  - 配置 Go 测试运行 (go test ./...)
  - 配置多版本 Go 测试 (1.21, 1.22, 1.23)
  - 添加测试覆盖率报告 (coveralls 或 codecov)
  - 配置 PR 自动触发测试
  
  **Must NOT do**:
  - 不要添加复杂的部署流程 (仅测试)
  - 不要添加性能基准测试 (Phase 0 不需要)
  
  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
  - **Reason**: 标准 CI 配置,模板化工作
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0.1 (with Tasks 2, 3, 4)
  - **Blocks**: Task 5 (CI 必须先建立)
  - **Blocked By**: None
  
  **References**:
  - GitHub Actions Go 文档: https://docs.github.com/en/actions/automating-builds-and-tests/building-and-testing-go
  - 现有 Makefile: `/home/jimxl/Documents/projects/rgo/Makefile` - 测试命令
  
  **Acceptance Criteria**:
  - [ ] `.github/workflows/test.yml` 文件存在
  - [ ] `git push` 触发 CI 运行
  - [ ] CI 运行所有 Go 测试: `go test ./...`
  - [ ] CI 显示测试覆盖率
  
  **QA Scenarios**:
  ```
  Scenario: CI 工作流正确配置
    Tool: Bash (git + gh CLI)
    Preconditions: GitHub Actions 已启用
    Steps:
      1. 创建测试分支: git checkout -b test-ci
      2. 修改 README.md (触发 CI)
      3. 推送: git push origin test-ci
      4. 检查 CI 状态: gh run list --branch test-ci
      5. 验证测试运行: gh run view --log
    Expected Result: CI 运行成功,显示 "247 tests passed"
    Evidence: .sisyphus/evidence/task-1-ci-workflow.log
  ```
  
  **Commit**: YES
  - Message: `ci: add GitHub Actions workflow for Go tests`
  - Files: `.github/workflows/test.yml`

- [x] 2. MSpec 适配器原型 (Go ↔ Ruby spec 桥接)

  **What to do**:
  - 创建 `pkg/mspec/adapter.go` - MSpec 适配器
  - 实现 `RunSpec(specFile string) (passed, failed int, err error)`
  - 调用本地 Ruby 的 mspec: `ruby -S mspec <file>`
  - 解析 MSpec 输出,提取通过/失败数
  - 创建示例测试: `pkg/mspec/adapter_test.go`
  
  **Must NOT do**:
  - 不要实现完整的 spec runner (仅原型)
  - 不要并行运行 specs (Phase 0 不需要)
  
  **Recommended Agent Profile**:
  - **Category**: `deep`
  - **Skills**: []
  - **Reason**: 需要理解 MSpec 输出格式,设计适配器接口
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0.1 (with Tasks 1, 3, 4)
  - **Blocks**: Phase 1+ 所有 spec 验证任务
  - **Blocked By**: None (本地 Ruby 已安装)
  
  **References**:
  - MSpec 文档: https://github.com/ruby/mspec
  - Ruby spec 示例: `vendor/ruby/spec/core/array/push_spec.rb`
  - 本地 Ruby 路径: 用户确认本地已安装 Ruby
  
  **Acceptance Criteria**:
  - [ ] `pkg/mspec/adapter.go` 文件存在
  - [ ] `RunSpec()` 函数可以运行单个 spec 文件
  - [ ] 正确解析 MSpec 输出 (passed/failed 计数)
  - [ ] `go test ./pkg/mspec` 通过
  
  **QA Scenarios**:
  ```
  Scenario: MSpec 适配器运行简单 spec
    Tool: Bash (go test)
    Preconditions: 本地 Ruby 和 mspec 已安装
    Steps:
      1. 运行适配器测试: go test -v ./pkg/mspec
      2. 验证输出包含: "RunSpec: vendor/ruby/spec/core/array/push_spec.rb"
      3. 验证返回值: passed > 0, failed >= 0
    Expected Result: 测试通过,适配器成功运行 spec
    Evidence: .sisyphus/evidence/task-2-mspec-adapter.log
  
  Scenario: MSpec 适配器处理失败 spec
    Tool: Bash (go test)
    Preconditions: 创建一个故意失败的 spec
    Steps:
      1. 创建测试 spec: echo 'describe("test") { it("fails") { 1.should == 2 } }' > /tmp/fail_spec.rb
      2. 运行适配器: go run pkg/mspec/adapter.go /tmp/fail_spec.rb
      3. 验证返回: failed = 1
    Expected Result: 适配器正确识别失败
    Evidence: .sisyphus/evidence/task-2-mspec-adapter-fail.log
  ```
  
  **Commit**: YES
  - Message: `feat(mspec): add MSpec adapter prototype for Ruby spec integration`
  - Files: `pkg/mspec/adapter.go`, `pkg/mspec/adapter_test.go`

- [x] 3. Opcode 审计和文档

  **What to do**:
  - 审计 `pkg/compiler/opcode.go` 中的 80 个 opcodes
  - 对每个 opcode,检查 `pkg/vm/executor.go` 中的实现
  - 创建 `docs/OPCODES.md` 文档:
    - 列出所有 opcodes
    - 标记实现状态 (✅ 完整, ⚠️ 部分, ❌ 未实现)
    - 记录每个 opcode 的用途
  - 更新 TODO.md 中的 opcode 统计
  
  **Must NOT do**:
  - 不要修复 opcodes (仅审计)
  - 不要添加新 opcodes
  
  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
  - **Reason**: 代码审计和文档工作
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0.1 (with Tasks 1, 2, 4)
  - **Blocks**: Phase 1 基础设施任务 (需要知道哪些 opcodes 缺失)
  - **Blocked By**: None
  
  **References**:
  - Opcode 定义: `pkg/compiler/opcode.go`
  - VM 实现: `pkg/vm/executor.go`
  - 现有 TODO: `TODO.md` - opcode 统计
  
  **Acceptance Criteria**:
  - [ ] `docs/OPCODES.md` 文件存在
  - [ ] 所有 80 个 opcodes 都有文档
  - [ ] 实现状态准确 (与 VM 代码一致)
  - [ ] TODO.md 更新为准确的 opcode 统计
  
  **QA Scenarios**:
  ```
  Scenario: Opcode 文档完整性
    Tool: Bash (grep + wc)
    Preconditions: docs/OPCODES.md 已创建
    Steps:
      1. 统计 opcode.go 中的 opcode 定义: grep -c "^const Op" pkg/compiler/opcode.go
      2. 统计 OPCODES.md 中的 opcode 条目: grep -c "^###" docs/OPCODES.md
      3. 验证数量一致: 两者都应该是 80
    Expected Result: 文档覆盖所有 opcodes
    Evidence: .sisyphus/evidence/task-3-opcode-audit.log
  ```
  
  **Commit**: YES
  - Message: `docs: add opcode audit and implementation status`
  - Files: `docs/OPCODES.md`, `TODO.md`

- [x] 4. 块基础设施测试和文档

  **What to do**:
  - 创建 `pkg/vm/block_test.go` - 测试当前块实现
  - 测试场景:
    - 简单 yield: `def foo; yield; end; foo { puts "hi" }`
    - 块参数: `[1,2,3].each { |x| puts x }`
    - 块返回值: `[1,2,3].map { |x| x * 2 }`
    - block_given?: `def foo; block_given?; end`
  - 记录哪些功能工作,哪些不工作
  - 创建 `docs/BLOCKS.md` - 块实现状态文档
  
  **Must NOT do**:
  - 不要修复块实现 (仅测试)
  - 不要实现新的块功能
  
  **Recommended Agent Profile**:
  - **Category**: `quick`
  - **Skills**: []
  - **Reason**: 测试和文档工作
  
  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 0.1 (with Tasks 1, 2, 3)
  - **Blocks**: Phase 1 块系统任务 (需要知道当前状态)
  - **Blocked By**: None
  
  **References**:
  - VM 实现: `pkg/vm/executor.go` - OpYield, OpBlock
  - 现有测试: `pkg/vm/executor_test.go`
  - TODO.md: 块实现状态
  
  **Acceptance Criteria**:
  - [ ] `pkg/vm/block_test.go` 文件存在
  - [ ] 至少 10 个块测试用例
  - [ ] `docs/BLOCKS.md` 记录当前实现状态
  - [ ] 明确哪些功能工作,哪些不工作
  
  **QA Scenarios**:
  ```
  Scenario: 块测试覆盖率
    Tool: Bash (go test)
    Preconditions: block_test.go 已创建
    Steps:
      1. 运行块测试: go test -v ./pkg/vm -run TestBlock
      2. 统计测试数量: go test -v ./pkg/vm -run TestBlock | grep -c "=== RUN"
      3. 验证至少 10 个测试
    Expected Result: 至少 10 个块测试,部分通过,部分失败
    Evidence: .sisyphus/evidence/task-4-block-test.log
  ```
  
  **Commit**: YES
  - Message: `test: add block infrastructure tests and documentation`
  - Files: `pkg/vm/block_test.go`, `docs/BLOCKS.md`

---

## Phase 1: 核心基础设施 (预估 12-18 周兼职)

**目标**: 实现关键的 VM 基础设施 - 异常处理、块系统、类型检查、模块系统

### Wave 1.1 - 异常处理基础 (串行,依赖 Phase 0)

- [x] 5. 实现 OpRaise, OpRescue, OpRescueMatch opcodes

  **Recommended Agent Profile**: `deep` | **Blocks**: Tasks 6-8 | **Blocked By**: Task 3
  
  **What to do**: 在 `pkg/vm/executor.go` 中实现异常处理 opcodes,添加异常栈展开逻辑
  
  **QA**: 运行 `mspec vendor/ruby/spec/language/rescue_spec.rb`,期望部分通过

- [x] 6. 编译 BeginExpression, RaiseExpression AST 节点

  **Recommended Agent Profile**: `deep` | **Blocks**: Task 7 | **Blocked By**: Task 5
  
  **What to do**: 在 `pkg/compiler/compiler.go` 中实现 begin/rescue/ensure/raise 编译
  
  **QA**: 编译并运行 `begin; raise "test"; rescue => e; e.message; end`,期望返回 "test"

- [x] 7. 实现 Exception 类层次结构

  **Recommended Agent Profile**: `quick` | **Blocks**: Task 8 | **Blocked By**: Task 6
  
  **What to do**: 在 `pkg/core/init.go` 中创建 StandardError, RuntimeError, ArgumentError, TypeError, NameError
  
  **QA**: 运行 `mspec vendor/ruby/spec/core/exception/*_spec.rb`,期望 ≥50% 通过

- [x] 8. 异常处理集成测试

  **Recommended Agent Profile**: `unspecified-high` | **Blocks**: Wave 1.2 | **Blocked By**: Task 7
  
  **What to do**: 创建 `pkg/vm/exception_test.go`,测试嵌套 rescue, ensure, retry
  
  **QA**: 运行 `mspec vendor/ruby/spec/language/rescue_spec.rb`,期望 ≥80% 通过

### Wave 1.2 - 块/闭包系统 (并行,依赖 Wave 1.1)

- [x] 9-12. 实现 OpBlock, OpYield, OpBlockWithArg, OpSendWithBlock opcodes (4 个任务)

  **Recommended Agent Profile**: `deep` (each) | **Parallel Group**: Wave 1.2
  
  **What to do**: 实现块相关 opcodes,支持 yield, block_given?, 块参数
  
  **QA**: 运行 `mspec vendor/ruby/spec/language/block_spec.rb`,期望 ≥70% 通过

- [x] 13. 实现 Proc 和 Lambda 类

  **Recommended Agent Profile**: `deep` | **Parallel Group**: Wave 1.2
  
  **What to do**: 在 `pkg/core/init.go` 中实现 Proc, Lambda,支持 call, arity, lambda?
  
  **QA**: 运行 `mspec vendor/ruby/spec/core/proc/*_spec.rb`,期望 ≥60% 通过

### Wave 1.3 - 类型检查和模块系统 (并行,依赖 Wave 1.2)

- [x] 14-16. 实现 OpIsA, OpRespondTo, OpInclude opcodes (3 个任务)

  **Recommended Agent Profile**: `quick` (each) | **Parallel Group**: Wave 1.3
  
  **What to do**: 实现类型检查和模块混入 opcodes
  
  **QA**: 运行相关 language specs,期望 ≥80% 通过

- [x] 17. 实现 Module#include, #extend, #prepend

  **Recommended Agent Profile**: `deep` | **Parallel Group**: Wave 1.3
  
  **What to do**: 在 `pkg/object/module.go` 中实现模块混入方法
  
  **QA**: 运行 `mspec vendor/ruby/spec/core/module/*_spec.rb`,期望 ≥50% 通过

---

## Phase 2: 核心类完整实现 (预估 24-36 周兼职)

**目标**: 实现所有 59 个核心类,通过 core/ 和 language/ specs

### Wave 2.1 - Enumerable 模块 (关键依赖)

- [x] 18. 实现 Enumerable 模块 (each, map, select, reject, find, reduce 等 50+ 方法)

  **Recommended Agent Profile**: `deep` | **Blocks**: 所有集合类 | **Blocked By**: Wave 1.2 (块系统)
  
  **What to do**: 在 `pkg/core/init.go` 中实现完整的 Enumerable 模块
  
  **QA**: 运行 `mspec vendor/ruby/spec/core/enumerable/*_spec.rb`,期望 ≥90% 通过

### Wave 2.2 - Kernel 模块 (全局方法)

- [x] 19-23. 实现 Kernel 方法 (puts, print, p, raise, require, eval, loop, catch, throw 等 117 个方法,分 5 个任务)

  **Recommended Agent Profile**: `unspecified-high` (each) | **Parallel Group**: Wave 2.2
  
  **What to do**: 在 `pkg/core/init.go` 中实现 Kernel 全局方法
  
  **QA**: 运行 `mspec vendor/ruby/spec/core/kernel/*_spec.rb`,期望 ≥85% 通过

### Wave 2.3-2.10 - 核心类方法补全 (8 个波次,每波 5-8 个类)

**策略**: 按依赖顺序和 spec 文件数量分组

- **Wave 2.3**: String (60→114 方法), Array (55→102 方法), Hash (38→69 方法)
- **Wave 2.4**: Integer (24→67 方法), Float (11→50 方法), Numeric (新增 46 方法)
- **Wave 2.5**: Symbol (4→31 方法), Range (2→35 方法), Regexp (1→25 方法)
- **Wave 2.6**: Time (新增 66 方法), Date (新增), DateTime (新增)
- **Wave 2.7**: IO (新增 80 方法), File (新增 68 方法), Dir (新增 35 方法)
- **Wave 2.8**: Struct (新增 32 方法), Set (新增 52 方法), Enumerator (新增 30 方法)
- **Wave 2.9**: Rational (新增 34 方法), Complex (新增 43 方法), Math (新增 31 方法)
- **Wave 2.10**: Method (新增 27 方法), Binding (新增 11 方法), MatchData (新增 31 方法)

**每个波次的任务数**: 5-8 个任务 (每个类一个任务)

**QA 标准**: 每个类的 spec 通过率 ≥85%

---

## Phase 3: 语言特性补全 (预估 12-18 周兼职)

**目标**: 实现所有语言特性,通过 language/ specs

### Wave 3.1 - 控制流补全

- [x] 实现 for 循环, unless, redo, retry, case/when 完整支持

### Wave 3.2 - 参数和赋值

- [x] 实现多重赋值, splat 操作符, 关键字参数, 默认参数, 块参数

### Wave 3.3 - 高级语法

- [x] 实现字符串插值, heredoc, 正则表达式字面量, 条件/循环修饰符
- [x] 实现 super, alias, undef, 方法可见性 (public/private/protected)
- [x] 实现 pattern matching (case/in), numbered parameters, endless methods, rightward assignment

**QA 标准**: `mspec vendor/ruby/spec/language/` 通过率 ≥95%

---

## Phase 4: 标准库 - 数据格式 (预估 16-24 周兼职)

**目标**: 用 Go 原生实现标准库,通过 library/ specs

### Wave 4.1 - JSON (Go encoding/json)

- [x] 实现 JSON.parse, JSON.generate,映射到 Go encoding/json

### Wave 4.2 - CSV (Go encoding/csv)

- [x] 实现 CSV 解析和生成,映射到 Go encoding/csv
- [x] 实现 YAML 解析和生成,映射到 Go yaml 库
- [x] 实现 REXML,映射到 Go encoding/xml

**QA 标准**: 每个库的 spec 通过率 ≥90%

---

## Phase 5: 标准库 - 网络和加密 (预估 16-24 周兼职)

### Wave 5.1 - Net::HTTP (Go net/http)

- [x] 实现 HTTP 客户端,映射到 Go net/http
- [x] 实现 URI 解析,映射到 Go net/url
- [x] 实现 Socket 编程,映射到 Go net
- [x] 实现 OpenSSL 绑定,映射到 Go crypto
- [x] 实现 MD5, SHA1, SHA256 等,映射到 Go crypto

**QA 标准**: 每个库的 spec 通过率 ≥85%

---

## Phase 6: 标准库 - 系统和工具 (预估 12-18 周兼职)

### Wave 6.1 - FileUtils (Go os + io/fs)

- [ ] 实现文件工具,映射到 Go os 和 io/fs

### Wave 6.2 - Pathname (Go path/filepath)

- [ ] 实现路径名,映射到 Go path/filepath

### Wave 6.3 - Tempfile/Tmpdir (Go os)

- [ ] 实现临时文件,映射到 Go os.CreateTemp

### Wave 6.4 - Logger (Go log)

- [ ] 实现日志记录,映射到 Go log

### Wave 6.5 - ERB (Go text/template)

- [ ] 实现 ERB 模板引擎,映射到 Go text/template

**QA 标准**: 每个库的 spec 通过率 ≥85%

---

## Phase 7: 并发特性 (预估 12-18 周兼职)

**目标**: 用 Go goroutine 和 channel 实现 Ruby 并发

### Wave 7.1 - Thread (Go goroutine)

- [ ] 实现 Thread 类,映射到 Go goroutine

### Wave 7.2 - Fiber (Go 协程)

- [ ] 实现 Fiber 类,用 Go channel 实现协程

### Wave 7.3 - Mutex, Queue (Go sync)

- [x] 实现 Mutex, Queue, SizedQueue,映射到 Go sync

### Wave 7.4 - ConditionVariable (Go sync.Cond)

- [x] 实现 ConditionVariable,映射到 Go sync.Cond

**QA 标准**: 并发 specs 通过率 ≥80%

---

## Phase 8: 性能优化和最终验证 (预估 12-24 周兼职)

**目标**: 优化性能,达到或超越 MRI

### Wave 8.1 - 性能基准测试

- [x] 建立性能基准测试套件
- [x] 对比 MRI 性能,识别瓶颈

### Wave 8.2 - 热点优化

- [x] 优化字符串操作 (最频繁)
- [x] 优化数组操作
- [x] 优化哈希表操作
- [x] 优化方法调用

### Wave 8.3 - 字节码优化

- [x] 实现字节码优化 pass
- [x] 常量折叠
- [x] 死代码消除

### Wave 8.4 - 最终 Spec 验证

- [ ] 运行完整 Ruby spec 套件
- [ ] 修复所有失败的 specs
- [ ] 达到 100% 通过率

**QA 标准**: 
- 100% Ruby spec 通过
- 性能 ≤10x MRI (初期目标达成)
- 识别并优化关键路径,目标超越 MRI

---

## Final Verification Wave (所有任务完成后 - 4 个并行审查)

> 4 个审查代理并行运行。所有必须批准。拒绝 → 修复 → 重新运行。

- [ ] F1. **Spec 合规性审计** — `oracle`
  
  运行完整 Ruby spec 套件 (core + language + library ~3500+ 文件)。统计通过率。对于任何失败的 spec,分析原因并记录。验证 100% 通过率目标达成。
  
  Output: `Core [N/N 100%] | Language [N/N 100%] | Library [N/N 100%] | VERDICT: APPROVE/REJECT`

- [ ] F2. **性能基准审查** — `unspecified-high`
  
  运行性能基准测试套件。对比 MRI 性能。验证初期目标 (<10x MRI) 达成。识别性能瓶颈。生成性能报告。
  
  Output: `Benchmark [N tests] | vs MRI [avg Nx slower] | Bottlenecks [N identified] | VERDICT: APPROVE/REJECT`

- [ ] F3. **代码质量审查** — `unspecified-high`
  
  运行 `go vet`, `golint`, `staticcheck`。检查所有 Go 代码质量。验证无 `panic()` 在生产代码中。检查错误处理完整性。验证测试覆盖率 ≥80%。
  
  Output: `Vet [PASS/FAIL] | Lint [N issues] | Coverage [N%] | VERDICT: APPROVE/REJECT`

- [ ] F4. **文档完整性审查** — `deep`
  
  验证所有公共 API 有文档。检查 README, OPCODES.md, BLOCKS.md 等文档完整性。验证示例代码可运行。检查 CHANGELOG 记录所有重大变更。
  
  Output: `Docs [N/N complete] | Examples [N/N runnable] | CHANGELOG [up-to-date] | VERDICT: APPROVE/REJECT`

---

## Commit Strategy

**提交策略**: 按波次提交,每个波次完成后创建一个提交

**Phase 0**:
- Wave 0.1: `ci: add CI/CD and testing infrastructure`

**Phase 1**:
- Wave 1.1: `feat(vm): implement exception handling system`
- Wave 1.2: `feat(vm): implement block/closure system`
- Wave 1.3: `feat(vm): implement type introspection and module system`

**Phase 2**:
- Wave 2.1: `feat(core): implement Enumerable module`
- Wave 2.2: `feat(core): implement Kernel module`
- Wave 2.3-2.10: `feat(core): complete [ClassName] implementation` (每个波次一个提交)

**Phase 3**:
- Wave 3.1-3.5: `feat(lang): implement [feature name]` (每个波次一个提交)

**Phase 4-6**:
- 每个标准库: `feat(stdlib): implement [library name] with Go native`

**Phase 7**:
- Wave 7.1-7.4: `feat(concurrency): implement [feature] with Go goroutines`

**Phase 8**:
- Wave 8.1-8.4: `perf: [optimization description]`

**Final**:
- `chore: final verification and 100% spec compliance`

---

## Success Criteria

### 验证命令

```bash
# Phase 0 验证
go test ./...                    # 期望: 所有 Go 测试通过
gh run list                      # 期望: CI 运行成功

# Phase 1 验证
mspec vendor/ruby/spec/language/rescue_spec.rb    # 期望: ≥80% 通过
mspec vendor/ruby/spec/language/block_spec.rb     # 期望: ≥70% 通过

# Phase 2 验证
mspec vendor/ruby/spec/core/enumerable/           # 期望: ≥90% 通过
mspec vendor/ruby/spec/core/kernel/               # 期望: ≥85% 通过
mspec vendor/ruby/spec/core/string/               # 期望: ≥85% 通过
mspec vendor/ruby/spec/core/array/                # 期望: ≥85% 通过
mspec vendor/ruby/spec/core/hash/                 # 期望: ≥85% 通过

# Phase 3 验证
mspec vendor/ruby/spec/language/                  # 期望: ≥95% 通过

# Phase 4-6 验证
mspec vendor/ruby/spec/library/json/              # 期望: ≥90% 通过
mspec vendor/ruby/spec/library/net/               # 期望: ≥85% 通过
mspec vendor/ruby/spec/library/fileutils/         # 期望: ≥85% 通过

# Phase 7 验证
mspec vendor/ruby/spec/core/thread/               # 期望: ≥80% 通过
mspec vendor/ruby/spec/core/fiber/                # 期望: ≥80% 通过

# Phase 8 最终验证
mspec vendor/ruby/spec/                           # 期望: 100% 通过
go test -bench=. ./...                            # 期望: <10x MRI
```

### 最终检查清单

- [ ] 100% Ruby 4.0 spec 通过 (core + language + library)
- [ ] 所有 247 个现有 Go 测试仍然通过
- [ ] CI 在 <30 分钟内运行完整测试套件
- [ ] 性能 <10x MRI (初期目标)
- [ ] 所有功能用 Go 原生实现 (无 Ruby 依赖)
- [ ] 测试覆盖率 ≥80%
- [ ] 文档完整 (README, API docs, CHANGELOG)
- [ ] 无已知的关键 bug
- [ ] 性能基准测试套件建立
- [ ] 生产就绪 (稳定性验证)

---

## 项目时间线总结

**总预估**: 4-5 年 (兼职工作)

| 阶段 | 预估时间 (兼职) | 关键交付物 | Spec 通过率目标 |
|------|-----------------|-----------|----------------|
| Phase 0 | 4-6 周 | CI/CD, MSpec 适配器 | N/A |
| Phase 1 | 12-18 周 | 异常处理, 块系统, 模块系统 | Language ≥70% |
| Phase 2 | 24-36 周 | 59 个核心类完整实现 | Core ≥85% |
| Phase 3 | 12-18 周 | 所有语言特性 | Language ≥95% |
| Phase 4 | 16-24 周 | 数据格式标准库 | Library (data) ≥90% |
| Phase 5 | 16-24 周 | 网络和加密标准库 | Library (net) ≥85% |
| Phase 6 | 12-18 周 | 系统和工具标准库 | Library (sys) ≥85% |
| Phase 7 | 12-18 周 | 并发特性 | Concurrency ≥80% |
| Phase 8 | 12-24 周 | 性能优化, 最终验证 | All 100% |
| **总计** | **110-186 周** | **完整 Ruby 4.0 解释器** | **100%** |

**关键里程碑**:
- **6 个月**: Phase 0-1 完成,基础设施就绪
- **1 年**: Phase 2 完成,核心类可用
- **2 年**: Phase 3 完成,语言特性完整
- **3 年**: Phase 4-6 完成,标准库可用
- **4 年**: Phase 7 完成,并发支持
- **4-5 年**: Phase 8 完成,100% spec 通过,生产就绪

---

## 风险管理

### 高风险项 (需要密切监控)

1. **MSpec 适配器可行性** (Phase 0)
   - 风险: 无法可靠运行 Ruby specs
   - 缓解: 早期原型验证 (Task 2)
   - 后备: 用 Go 重写 specs (大幅增加工作量)

2. **异常处理 VM 重写** (Phase 1)
   - 风险: 需要重写 VM 架构 (12+ 周 vs. 2 周)
   - 缓解: 架构审查 (Metis 建议)
   - 后备: 简化异常处理 (仅 raise/rescue)

3. **范围蔓延** (所有阶段)
   - 风险: 标准库功能无限扩展
   - 缓解: 严格的 Must NOT 列表
   - 后备: 标记为 "known exclusions"

4. **性能目标无法达成** (Phase 8)
   - 风险: 无法达到 <10x MRI
   - 缓解: 早期性能基准测试
   - 后备: 调整目标为 "功能完整性优先"

5. **兼职时间不稳定** (所有阶段)
   - 风险: 实际时间线 2-3x 预估
   - 缓解: 保守的时间估算
   - 后备: 分阶段交付,每个阶段独立可用

### 中风险项

6. **Go 类型系统与 Ruby 对象模型不匹配**
   - 缓解: 早期原型验证
   
7. **并发语义差异** (Go goroutine vs. Ruby Thread)
   - 缓解: 详细的并发测试

8. **标准库 API 兼容性**
   - 缓解: 严格遵循 Ruby spec

---

## 下一步行动

**立即开始** (Phase 0, Wave 0.1):
1. 创建 GitHub Actions CI 工作流 (Task 1)
2. 构建 MSpec 适配器原型 (Task 2)
3. 审计 Opcodes (Task 3)
4. 测试块基础设施 (Task 4)

**验证任务** (在 Phase 1 前完成):
- MSpec 适配器可行性验证
- 异常处理架构审查
- 块系统当前状态确认

**长期规划**:
- 每个 Phase 结束后,审查进度和调整计划
- 每 6 个月,重新评估时间线和优先级
- 保持灵活性,根据实际情况调整范围

---

## 附录: 详细任务分解

由于项目规模巨大 (预估 500+ 个任务),完整的任务列表将在执行过程中逐步细化。

**Phase 2-8 的详细任务** 将在 Phase 1 完成后,根据实际进展和经验教训进行细化。

**当前计划** 提供了高层次的路线图和关键里程碑,足以开始 Phase 0-1 的工作。

**建议**: 使用 `/start-work` 开始执行 Phase 0, Wave 0.1 的 4 个任务。完成后,根据实际经验更新后续阶段的计划。

