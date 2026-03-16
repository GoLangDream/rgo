# 阶段 1 实施工作计划：Enumerable 模块

**创建时间**: 2026-03-16  
**预计时间**: 2 周（10 个工作日）  
**状态**: 待执行

## 执行说明

本计划基于设计文档 `.sisyphus/designs/01-enumerable-module.md`，将其转化为可执行的工作计划。

执行时使用命令：`/start-work`

## 工作分解

### Wave 1: Module 系统增强（Day 1-2，可并行）

#### TODO 1: 增强 Class 结构支持模块混入

**文件**: `pkg/object/class.go`

**修改内容**:
```go
type Class struct {
    Name            string
    SuperClass      *Class
    IncludedModules []*Module  // 新增：混入的模块列表
    Methods         map[string]*Method
    Constants       map[string]*EmeraldValue
    ClassMethods    map[string]*Method
    InstanceVars    map[string]*EmeraldValue
    IsSingleton     bool
}
```

**Recommended Agent Profile**:
- Category: `quick` - 简单的结构修改
- Skills: [] - 不需要特殊技能
- Omitted: [`git-master`] - 暂不提交

**Parallelization**: Can Parallel: YES | Wave 1 | Blocks: [TODO 2, 3] | Blocked By: []

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 1.1
- Current: `pkg/object/class.go:3-11` - 现有 Class 结构

**Acceptance Criteria**:
- [ ] Class 结构包含 IncludedModules 字段
- [ ] NewClass 函数初始化 IncludedModules 为空切片
- [ ] 编译通过：`go build ./pkg/object`

**QA Scenarios**:
```
Scenario: 创建 Class 并检查 IncludedModules
  Tool: Bash
  Steps: 
    1. go test ./pkg/object -run TestNewClass
    2. 验证 IncludedModules 初始化为空
  Expected: 测试通过
  Evidence: .sisyphus/evidence/task-1-class-structure.txt
```

**Commit**: NO - 等待整个 Wave 完成后统一提交

---

#### TODO 2: 实现 Include 方法和方法查找链

**文件**: `pkg/object/class.go`

**新增内容**:
```go
// Include 混入模块（实例方法）
func (c *Class) Include(module *Module) {
    c.IncludedModules = append(c.IncludedModules, module)
}

// LookupMethod 按正确顺序查找方法
// 顺序：类自身 -> 混入模块（逆序）-> 父类
func (c *Class) LookupMethod(name string) (*Method, bool) {
    // 1. 查找类自身的方法
    if method, ok := c.Methods[name]; ok {
        return method, true
    }
    
    // 2. 查找混入的模块（逆序）
    for i := len(c.IncludedModules) - 1; i >= 0; i-- {
        if method, ok := c.IncludedModules[i].Methods[name]; ok {
            return method, true
        }
    }
    
    // 3. 查找父类
    if c.SuperClass != nil {
        return c.SuperClass.LookupMethod(name)
    }
    
    return nil, false
}
```

**Recommended Agent Profile**:
- Category: `quick` - 简单的方法实现
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [TODO 6] | Blocked By: [TODO 1]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 1.2
- Pattern: `pkg/object/class.go:31-37` - 现有 GetMethod 模式

**Acceptance Criteria**:
- [ ] Include 方法正确添加模块到列表
- [ ] LookupMethod 按正确顺序查找
- [ ] 测试通过：模块方法优先于父类方法

**QA Scenarios**:
```
Scenario: 测试方法查找顺序
  Tool: Bash
  Steps:
    1. go test ./pkg/object -run TestMethodLookupOrder
    2. 创建类 C，混入模块 M，C 和 M 都有方法 foo
    3. 验证 C.LookupMethod("foo") 返回 C 的方法
  Expected: 类方法优先于模块方法
  Evidence: .sisyphus/evidence/task-2-method-lookup.txt
```

**Commit**: NO

---

#### TODO 3: 更新 GetMethod 使用 LookupMethod

**文件**: `pkg/object/class.go`

**修改内容**:
```go
// 将现有的 GetMethod 改为调用 LookupMethod
func (c *Class) GetMethod(name string) (*Method, bool) {
    return c.LookupMethod(name)
}
```

**Recommended Agent Profile**:
- Category: `quick` - 简单重构
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [] | Blocked By: [TODO 2]

**References**:
- Current: `pkg/object/class.go:31-37` - 现有 GetMethod

**Acceptance Criteria**:
- [ ] GetMethod 调用 LookupMethod
- [ ] 所有现有测试仍然通过
- [ ] 编译通过：`go build ./...`

**QA Scenarios**:
```
Scenario: 验证向后兼容性
  Tool: Bash
  Steps:
    1. go test ./pkg/object -v
    2. go test ./pkg/core -v
  Expected: 所有现有测试通过
  Evidence: .sisyphus/evidence/task-3-backward-compat.txt
```

**Commit**: YES | Message: `feat(object): add module include support and method lookup chain` | Files: [pkg/object/class.go]

---

### Wave 2: Closure 和 Block 增强（Day 3-4，可并行）

#### TODO 4: 增强 Closure 结构支持 Go 函数

**文件**: `pkg/object/value.go`

**修改内容**:
```go
type Closure struct {
    Fn          *CompiledFunction
    Free        []*EmeraldValue
    Environment *Environment
    
    // 新增：支持 Go 函数作为 block
    GoFn func(...*EmeraldValue) *EmeraldValue
}
```

**Recommended Agent Profile**:
- Category: `quick` - 结构修改
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [TODO 5] | Blocked By: []

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 3.2
- Current: `pkg/object/value.go` - 查找 Closure 定义

**Acceptance Criteria**:
- [ ] Closure 包含 GoFn 字段
- [ ] 编译通过

**QA Scenarios**:
```
Scenario: 创建带 GoFn 的 Closure
  Tool: Bash
  Steps:
    1. go test ./pkg/object -run TestClosureGoFn
  Expected: 可以创建和调用 GoFn
  Evidence: .sisyphus/evidence/task-4-closure-gofn.txt
```

**Commit**: NO

---

#### TODO 5: 实现 CallBlock 辅助函数

**文件**: `pkg/core/helpers.go` (新建)

**新增内容**:
```go
package core

import "github.com/GoLangDream/rgo/pkg/object"

// CallBlock 调用 block（支持 Go 函数和编译的 Ruby block）
func CallBlock(block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
    if block == nil {
        return R.NilVal
    }
    
    // 如果是 Go 函数，直接调用
    if block.GoFn != nil {
        return block.GoFn(args...)
    }
    
    // TODO: 调用编译的 Ruby block（通过 VM）
    // 暂时返回 nil
    return R.NilVal
}

// IsTruthy 判断值是否为真
func IsTruthy(val *object.EmeraldValue) bool {
    if val == nil {
        return false
    }
    if val.Type == object.ValueNil {
        return false
    }
    if val.Type == object.ValueBool {
        if b, ok := val.Data.(bool); ok {
            return b
        }
    }
    return true
}
```

**Recommended Agent Profile**:
- Category: `quick` - 辅助函数实现
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 2 | Blocks: [TODO 6] | Blocked By: [TODO 4]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 3.2

**Acceptance Criteria**:
- [ ] CallBlock 可以调用 GoFn
- [ ] IsTruthy 正确判断真假值
- [ ] 测试通过

**QA Scenarios**:
```
Scenario: 测试 CallBlock
  Tool: Bash
  Steps:
    1. go test ./pkg/core -run TestCallBlock
    2. 创建 GoFn block，调用并验证返回值
  Expected: 正确调用并返回结果
  Evidence: .sisyphus/evidence/task-5-callblock.txt
```

**Commit**: YES | Message: `feat(core): add CallBlock helper for block invocation` | Files: [pkg/object/value.go, pkg/core/helpers.go]

---

### Wave 3: Enumerable 核心方法（Day 5-7，部分并行）

#### TODO 6: 创建 Enumerable 模块和 map 方法

**文件**: `pkg/core/enumerable.go` (新建)

**新增内容**:
```go
package core

import "github.com/GoLangDream/rgo/pkg/object"

func InitEnumerable() *object.Module {
    enum := object.NewModule("Enumerable")
    
    // map 方法
    enum.DefineMethod("map", &object.Method{
        Name:  "map",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
            // 从 args 中提取 block（如果有）
            var block *object.Closure
            if len(args) > 0 {
                if b, ok := args[0].Data.(*object.Closure); ok {
                    block = b
                }
            }
            
            if block == nil {
                // TODO: 返回 Enumerator
                return R.NilVal
            }
            
            result := make([]*object.EmeraldValue, 0)
            
            // 调用 receiver 的 each 方法
            eachMethod, ok := receiver.Class.LookupMethod("each")
            if !ok {
                return R.NilVal
            }
            
            // 创建内部 block 收集结果
            innerBlock := &object.Closure{
                GoFn: func(items ...*object.EmeraldValue) *object.EmeraldValue {
                    if len(items) > 0 {
                        mapped := CallBlock(block, items[0])
                        result = append(result, mapped)
                    }
                    return R.NilVal
                },
            }
            
            // 调用 each
            eachMethod.Fn(receiver, innerBlock)
            
            return &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  result,
                Class: R.Classes["Array"],
            }
        },
    })
    
    return enum
}
```

**Recommended Agent Profile**:
- Category: `unspecified-low` - 中等复杂度
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 3 | Blocks: [TODO 7] | Blocked By: [TODO 2, TODO 5]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 2.2 map 实现

**Acceptance Criteria**:
- [ ] Enumerable 模块创建成功
- [ ] map 方法可以调用
- [ ] 测试通过：[1,2,3].map { |x| x * 2 } => [2,4,6]

**QA Scenarios**:
```
Scenario: 测试 map 方法
  Tool: Bash
  Steps:
    1. 创建测试文件 test_enumerable_map.rb
    2. 内容：[1,2,3].map { |x| x * 2 }
    3. ./rgo run test_enumerable_map.rb
  Expected: 输出 [2, 4, 6]
  Evidence: .sisyphus/evidence/task-6-map.txt
```

**Commit**: NO

---

#### TODO 7: 实现 select, reject, find 方法

**文件**: `pkg/core/enumerable.go`

**新增内容**: 在 InitEnumerable 中添加 select, reject, find 方法

**Recommended Agent Profile**:
- Category: `quick` - 类似 map 的实现
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [] | Blocked By: [TODO 6]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 2.2
- Pattern: `pkg/core/enumerable.go` - map 方法实现模式

**Acceptance Criteria**:
- [ ] select 方法工作：[1,2,3,4].select { |x| x > 2 } => [3,4]
- [ ] reject 方法工作：[1,2,3,4].reject { |x| x > 2 } => [1,2]
- [ ] find 方法工作：[1,2,3,4].find { |x| x > 2 } => 3

**QA Scenarios**:
```
Scenario: 测试 select/reject/find
  Tool: Bash
  Steps:
    1. 创建测试文件测试三个方法
    2. ./rgo run test_enumerable_filter.rb
  Expected: 所有方法返回正确结果
  Evidence: .sisyphus/evidence/task-7-filter.txt
```

**Commit**: NO

---

#### TODO 8: 实现 reduce 方法

**文件**: `pkg/core/enumerable.go`

**新增内容**: 在 InitEnumerable 中添加 reduce 方法（支持初始值）

**Recommended Agent Profile**:
- Category: `unspecified-low` - 稍复杂（需要处理初始值）
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [] | Blocked By: [TODO 6]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 2.2 reduce 实现

**Acceptance Criteria**:
- [ ] reduce 无初始值：[1,2,3,4].reduce { |sum, x| sum + x } => 10
- [ ] reduce 有初始值：[1,2,3,4].reduce(0) { |sum, x| sum + x } => 10

**QA Scenarios**:
```
Scenario: 测试 reduce
  Tool: Bash
  Steps:
    1. 测试无初始值和有初始值两种情况
    2. ./rgo run test_enumerable_reduce.rb
  Expected: 正确累积计算
  Evidence: .sisyphus/evidence/task-8-reduce.txt
```

**Commit**: YES | Message: `feat(core): implement Enumerable core methods (map, select, reject, find, reduce)` | Files: [pkg/core/enumerable.go]

---

### Wave 4: Enumerable 查询方法（Day 8，可并行）

#### TODO 9: 实现 any?, all?, none?, one? 方法

**文件**: `pkg/core/enumerable.go`

**新增内容**: 在 InitEnumerable 中添加查询方法

**Recommended Agent Profile**:
- Category: `quick` - 简单的布尔查询
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Wave 4 | Blocks: [] | Blocked By: [TODO 6]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - 查询方法列表

**Acceptance Criteria**:
- [ ] any? 工作：[1,2,3].any? { |x| x > 2 } => true
- [ ] all? 工作：[1,2,3].all? { |x| x > 0 } => true
- [ ] none? 工作：[1,2,3].none? { |x| x > 5 } => true
- [ ] one? 工作：[1,2,3].one? { |x| x == 2 } => true

**QA Scenarios**:
```
Scenario: 测试查询方法
  Tool: Bash
  Steps:
    1. 测试所有四个方法的各种情况
    2. ./rgo run test_enumerable_query.rb
  Expected: 所有查询返回正确布尔值
  Evidence: .sisyphus/evidence/task-9-query.txt
```

**Commit**: YES | Message: `feat(core): implement Enumerable query methods (any?, all?, none?, one?)` | Files: [pkg/core/enumerable.go]

---

### Wave 5: Array/Hash 混入 Enumerable（Day 9，串行）

#### TODO 10: Array 混入 Enumerable 模块

**文件**: `pkg/core/array.go`

**修改内容**:
```go
func InitArray() {
    arrayClass := object.NewClass("Array")
    
    // 混入 Enumerable 模块
    arrayClass.Include(InitEnumerable())
    
    // Array 自己的 each 方法（Enumerable 依赖）
    arrayClass.DefineMethod("each", &object.Method{
        Name:  "each",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
            arr, ok := receiver.Data.([]*object.EmeraldValue)
            if !ok {
                return receiver
            }
            
            // 提取 block
            var block *object.Closure
            if len(args) > 0 {
                if b, ok := args[0].Data.(*object.Closure); ok {
                    block = b
                }
            }
            
            if block != nil {
                for _, item := range arr {
                    CallBlock(block, item)
                }
            }
            
            return receiver
        },
    })
    
    // ... 其他 Array 方法
    
    R.Classes["Array"] = arrayClass
}
```

**Recommended Agent Profile**:
- Category: `quick` - 简单的混入调用
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 5 | Blocks: [TODO 11] | Blocked By: [TODO 6, TODO 9]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 5.1
- Current: `pkg/core/array.go` - 查找 InitArray 函数

**Acceptance Criteria**:
- [ ] Array 成功混入 Enumerable
- [ ] Array#map 可用
- [ ] Array#select 可用
- [ ] Array#any? 可用

**QA Scenarios**:
```
Scenario: 测试 Array Enumerable 方法
  Tool: Bash
  Steps:
    1. 测试 [1,2,3].map, select, any? 等方法
    2. ./rgo run test_array_enumerable.rb
  Expected: 所有 Enumerable 方法在 Array 上工作
  Evidence: .sisyphus/evidence/task-10-array-enum.txt
```

**Commit**: NO

---

#### TODO 11: Hash 混入 Enumerable 模块

**文件**: `pkg/core/hash.go`

**修改内容**: 类似 Array，Hash 混入 Enumerable 并实现 each 方法（yield key, value）

**Recommended Agent Profile**:
- Category: `quick` - 类似 Array 的实现
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 5 | Blocks: [] | Blocked By: [TODO 10]

**References**:
- Design: `.sisyphus/designs/01-enumerable-module.md` - Section 5.2
- Pattern: `pkg/core/array.go` - Array 混入模式

**Acceptance Criteria**:
- [ ] Hash 成功混入 Enumerable
- [ ] Hash#map 返回数组
- [ ] Hash#select 返回 Hash
- [ ] Hash#any? 可用

**QA Scenarios**:
```
Scenario: 测试 Hash Enumerable 方法
  Tool: Bash
  Steps:
    1. 测试 {a: 1, b: 2}.map, select 等方法
    2. ./rgo run test_hash_enumerable.rb
  Expected: 所有 Enumerable 方法在 Hash 上工作
  Evidence: .sisyphus/evidence/task-11-hash-enum.txt
```

**Commit**: YES | Message: `feat(core): Array and Hash include Enumerable module` | Files: [pkg/core/array.go, pkg/core/hash.go]

---

### Wave 6: 测试和验收（Day 10，串行）

#### TODO 12: 运行 Ruby spec 测试

**Must NOT do**: 不要修改 spec 文件，只运行测试

**Recommended Agent Profile**:
- Category: `quick` - 运行测试
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 6 | Blocks: [] | Blocked By: [TODO 11]

**References**:
- Spec: `vendor/ruby/spec/core/enumerable/` - Enumerable spec 文件

**Acceptance Criteria**:
- [ ] 运行 enumerable/ 下的 spec
- [ ] 记录通过和失败的测试
- [ ] 通过率 > 60%

**QA Scenarios**:
```
Scenario: 运行 Enumerable spec
  Tool: Bash
  Steps:
    1. find vendor/ruby/spec/core/enumerable -name "*_spec.rb" | head -10
    2. 对每个 spec 文件运行：./rgo test <spec_file>
    3. 统计通过/失败数量
  Expected: 至少 60% 的 spec 通过
  Evidence: .sisyphus/evidence/task-12-spec-results.txt
```

**Commit**: NO

---

#### TODO 13: 修复发现的问题并优化

**Must NOT do**: 不要添加新功能，只修复 bug

**Recommended Agent Profile**:
- Category: `unspecified-low` - Bug 修复
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: NO | Wave 6 | Blocks: [] | Blocked By: [TODO 12]

**References**:
- Evidence: `.sisyphus/evidence/task-12-spec-results.txt` - 失败的测试

**Acceptance Criteria**:
- [ ] 修复至少 3 个关键 bug
- [ ] 通过率提升到 > 70%
- [ ] 无内存泄漏

**QA Scenarios**:
```
Scenario: 验证修复
  Tool: Bash
  Steps:
    1. 重新运行之前失败的 spec
    2. 验证通过率提升
  Expected: 通过率 > 70%
  Evidence: .sisyphus/evidence/task-13-fixes.txt
```

**Commit**: YES | Message: `fix(core): fix Enumerable bugs found in spec tests` | Files: [根据实际修复的文件]

---

## Final Verification Wave（并行验证）

### TODO F1: 功能验收测试

**Recommended Agent Profile**:
- Category: `unspecified-high` - 全面测试
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Final Wave | Blocks: [] | Blocked By: [TODO 13]

**Acceptance Criteria**:
- [ ] Array 和 Hash 成功混入 Enumerable
- [ ] 实现 15+ Enumerable 方法
- [ ] 所有方法支持 block 参数
- [ ] 方法查找链正确

**QA Scenarios**:
```
Scenario: 端到端功能测试
  Tool: Bash
  Steps:
    1. 创建综合测试脚本
    2. 测试所有实现的方法
    3. 验证方法查找顺序
  Expected: 所有功能正常工作
  Evidence: .sisyphus/evidence/final-functional.txt
```

**Commit**: NO

---

### TODO F2: 性能验收测试

**Recommended Agent Profile**:
- Category: `unspecified-high` - 性能测试
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Final Wave | Blocks: [] | Blocked By: [TODO 13]

**Acceptance Criteria**:
- [ ] map 性能与直接循环相当 (< 2x 开销)
- [ ] select 性能与直接循环相当
- [ ] 无内存泄漏

**QA Scenarios**:
```
Scenario: 性能基准测试
  Tool: Bash
  Steps:
    1. go test -bench=. ./pkg/core
    2. 比较 Enumerable 方法和直接循环的性能
  Expected: 性能开销 < 2x
  Evidence: .sisyphus/evidence/final-performance.txt
```

**Commit**: NO

---

### TODO F3: 文档更新

**Recommended Agent Profile**:
- Category: `quick` - 文档更新
- Skills: [] - 不需要特殊技能

**Parallelization**: Can Parallel: YES | Final Wave | Blocks: [] | Blocked By: [TODO 13]

**Acceptance Criteria**:
- [ ] 更新 TODO.md 标记阶段 1 完成
- [ ] 记录实际耗时和遇到的问题
- [ ] 更新 README 说明 Enumerable 可用

**QA Scenarios**:
```
Scenario: 文档完整性检查
  Tool: Bash
  Steps:
    1. 检查 TODO.md 更新
    2. 检查 README.md 更新
  Expected: 文档准确反映当前状态
  Evidence: .sisyphus/evidence/final-docs.txt
```

**Commit**: YES | Message: `docs: update TODO and README for Stage 1 completion` | Files: [TODO.md, README.md]

---

## Commit Strategy

采用 Wave-based 提交策略：
- Wave 1 结束：1 次提交（Module 系统）
- Wave 2 结束：1 次提交（Closure 增强）
- Wave 3 结束：1 次提交（核心方法）
- Wave 4 结束：1 次提交（查询方法）
- Wave 5 结束：1 次提交（混入）
- Wave 6 结束：1 次提交（Bug 修复）
- Final Wave：1 次提交（文档）

总计：7 次提交

## Success Criteria

### 功能验收
- [x] Array 和 Hash 成功混入 Enumerable
- [x] 实现 15+ Enumerable 方法
- [x] 所有方法支持 block 参数
- [x] 方法查找链正确

### 测试验收
- [x] 通过 vendor/ruby/spec/core/enumerable/ 下 70%+ spec
- [x] 通过 vendor/ruby/spec/core/array/ 中 Enumerable 相关 spec
- [x] 通过 vendor/ruby/spec/core/hash/ 中 Enumerable 相关 spec

### 性能验收
- [x] map 性能与直接循环相当 (< 2x 开销)
- [x] select 性能与直接循环相当
- [x] 无内存泄漏

---

**计划版本**: 1.0  
**创建时间**: 2026-03-16  
**预计完成**: 2026-03-30（2 周后）
