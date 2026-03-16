# 阶段 1: Enumerable 模块技术设计

**优先级**: P0  
**预计时间**: 2 周  
**依赖**: 无  
**被依赖**: 阶段 2 (Array), 阶段 3 (String), 阶段 4 (Hash)

## 目标

实现 Ruby 的 Enumerable 模块，为所有集合类提供统一的迭代方法。这是通过大量 Ruby spec 的基础。

## 背景

Enumerable 是 Ruby 中最重要的模块之一，提供了 40+ 个迭代和查询方法。任何实现了 `each` 方法的类都可以混入 Enumerable，自动获得所有这些方法。

### Ruby 中的 Enumerable

```ruby
module Enumerable
  def map
    result = []
    each { |item| result << yield(item) }
    result
  end
  
  def select
    result = []
    each { |item| result << item if yield(item) }
    result
  end
end

class Array
  include Enumerable
  # 只需实现 each
end
```

## 当前状态

### 已实现
- Array#each - 基础迭代
- Hash#each - 键值对迭代
- 部分方法在 Array 中单独实现 (map, select, find)

### 缺失
- Enumerable 模块基础设施
- 30+ Enumerable 方法
- Block 参数传递机制
- Enumerator 对象（无 block 时返回）

## 技术设计

### 1. 模块系统增强

#### 1.1 Module 对象结构

```go
// pkg/object/module.go
type Module struct {
    Name    string
    Methods map[string]*Method
    // 模块级别的实例变量
    ModuleVars map[string]*EmeraldValue
}

// 模块方法定义
type ModuleMethod struct {
    Name       string
    Arity      int
    Fn         func(receiver *EmeraldValue, block *Closure, args ...*EmeraldValue) *EmeraldValue
    NeedsBlock bool  // 是否需要 block
}
```

#### 1.2 Include 机制

```go
// pkg/object/class.go
type Class struct {
    Name           string
    Superclass     *Class
    IncludedModules []*Module  // 新增: 混入的模块列表
    Methods        map[string]*Method
    // ...
}

// 方法查找顺序: 
// 1. 类自身的方法
// 2. 最后 include 的模块
// 3. 之前 include 的模块
// 4. 父类
func (c *Class) LookupMethod(name string) *Method {
    // 先查找类自身
    if method, ok := c.Methods[name]; ok {
        return method
    }
    
    // 查找混入的模块 (逆序)
    for i := len(c.IncludedModules) - 1; i >= 0; i-- {
        if method, ok := c.IncludedModules[i].Methods[name]; ok {
            return method
        }
    }
    
    // 查找父类
    if c.Superclass != nil {
        return c.Superclass.LookupMethod(name)
    }
    
    return nil
}
```

### 2. Enumerable 模块实现

#### 2.1 核心方法列表

**P0 - 必须实现 (第 1 周)**:
- `map` / `collect` - 转换每个元素
- `select` / `filter` / `find_all` - 筛选元素
- `reject` - 反向筛选
- `find` / `detect` - 查找第一个匹配
- `reduce` / `inject` - 累积计算
- `each_with_index` - 带索引迭代
- `any?` - 是否有元素满足条件
- `all?` - 是否所有元素满足条件
- `none?` - 是否没有元素满足条件
- `one?` - 是否恰好一个元素满足条件
- `count` - 计数
- `first` - 第一个元素
- `take` - 取前 n 个
- `drop` - 跳过前 n 个

**P1 - 重要 (第 2 周)**:
- `sort` / `sort_by` - 排序
- `min` / `max` / `min_by` / `max_by` - 最值
- `group_by` - 分组
- `partition` - 分区
- `zip` - 合并多个集合
- `each_cons` - 连续元素
- `each_slice` - 切片迭代
- `flat_map` / `collect_concat` - 扁平化映射
- `grep` - 模式匹配
- `include?` / `member?` - 包含检查

#### 2.2 实现示例

```go
// pkg/core/enumerable.go
package core

func InitEnumerable() *object.Module {
    enum := &object.Module{
        Name:    "Enumerable",
        Methods: make(map[string]*object.Method),
    }
    
    // map 方法
    enum.Methods["map"] = &object.Method{
        Name:  "map",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
            if block == nil {
                // TODO: 返回 Enumerator
                return R.NilVal
            }
            
            result := make([]*object.EmeraldValue, 0)
            
            // 调用 receiver 的 each 方法
            eachMethod := receiver.Class.LookupMethod("each")
            if eachMethod == nil {
                // 错误: 没有 each 方法
                return R.NilVal
            }
            
            // 创建一个内部 block，收集结果
            eachMethod.Fn(receiver, &object.Closure{
                Fn: func(item *object.EmeraldValue) *object.EmeraldValue {
                    // 对每个元素调用用户的 block
                    mapped := block.Fn(item)
                    result = append(result, mapped)
                    return R.NilVal
                },
            })
            
            return &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  result,
                Class: R.Classes["Array"],
            }
        },
    }
    
    // select 方法
    enum.Methods["select"] = &object.Method{
        Name:  "select",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
            if block == nil {
                return R.NilVal
            }
            
            result := make([]*object.EmeraldValue, 0)
            
            eachMethod := receiver.Class.LookupMethod("each")
            if eachMethod == nil {
                return R.NilVal
            }
            
            eachMethod.Fn(receiver, &object.Closure{
                Fn: func(item *object.EmeraldValue) *object.EmeraldValue {
                    condition := block.Fn(item)
                    if IsTruthy(condition) {
                        result = append(result, item)
                    }
                    return R.NilVal
                },
            })
            
            return &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  result,
                Class: R.Classes["Array"],
            }
        },
    }
    
    // reduce 方法
    enum.Methods["reduce"] = &object.Method{
        Name:  "reduce",
        Arity: -1,  // 可变参数
        Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
            var accumulator *object.EmeraldValue
            var startIndex int
            
            // 处理初始值
            if len(args) > 0 {
                accumulator = args[0]
                startIndex = 0
            } else {
                // 使用第一个元素作为初始值
                startIndex = 1
                // TODO: 获取第一个元素
            }
            
            eachMethod := receiver.Class.LookupMethod("each")
            if eachMethod == nil {
                return R.NilVal
            }
            
            index := 0
            eachMethod.Fn(receiver, &object.Closure{
                Fn: func(item *object.EmeraldValue) *object.EmeraldValue {
                    if index < startIndex {
                        accumulator = item
                    } else {
                        accumulator = block.Fn(accumulator, item)
                    }
                    index++
                    return R.NilVal
                },
            })
            
            return accumulator
        },
    }
    
    return enum
}
```

### 3. Block 和 Closure 增强

#### 3.1 当前 Closure 结构

```go
// pkg/object/value.go
type Closure struct {
    Fn          *CompiledFunction
    Free        []*EmeraldValue
    Environment *Environment
}
```

#### 3.2 需要的增强

```go
type Closure struct {
    Fn          *CompiledFunction
    Free        []*EmeraldValue
    Environment *Environment
    
    // 新增: 支持 Go 函数作为 block
    GoFn func(...*EmeraldValue) *EmeraldValue
}

// Block 调用辅助函数
func CallBlock(block *Closure, args ...*EmeraldValue) *EmeraldValue {
    if block == nil {
        return R.NilVal
    }
    
    if block.GoFn != nil {
        return block.GoFn(args...)
    }
    
    // 调用编译的 Ruby block
    // TODO: 通过 VM 执行
    return R.NilVal
}
```

### 4. VM 支持

#### 4.1 新增 Opcode

```go
// pkg/compiler/opcode.go
const (
    // ... 现有 opcodes
    
    OpYield         // 调用 block
    OpBlockGiven    // 检查是否传入了 block
    OpCallWithBlock // 带 block 的方法调用
)
```

#### 4.2 VM 执行

```go
// pkg/vm/executor.go
case OpYield:
    numArgs := int(ins[ip+1])
    args := make([]*object.EmeraldValue, numArgs)
    for i := numArgs - 1; i >= 0; i-- {
        args[i] = vm.pop()
    }
    
    // 获取当前 frame 的 block
    block := vm.currentFrame().block
    if block == nil {
        return fmt.Errorf("no block given")
    }
    
    result := CallBlock(block, args...)
    vm.push(result)
    ip += 2

case OpBlockGiven:
    block := vm.currentFrame().block
    if block != nil {
        vm.push(core.R.TrueVal)
    } else {
        vm.push(core.R.FalseVal)
    }
    ip++
```

### 5. 集成到现有类

#### 5.1 Array 混入 Enumerable

```go
// pkg/core/array.go
func InitArray() {
    arrayClass := &object.Class{
        Name:            "Array",
        IncludedModules: []*object.Module{InitEnumerable()},
        Methods:         make(map[string]*object.Method),
    }
    
    // Array 自己的 each 方法
    arrayClass.Methods["each"] = &object.Method{
        Name:  "each",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
            arr, ok := receiver.Data.([]*object.EmeraldValue)
            if !ok {
                return R.NilVal
            }
            
            for _, item := range arr {
                if block != nil {
                    CallBlock(block, item)
                }
            }
            
            return receiver
        },
    }
    
    R.Classes["Array"] = arrayClass
}
```

#### 5.2 Hash 混入 Enumerable

```go
// pkg/core/hash.go
func InitHash() {
    hashClass := &object.Class{
        Name:            "Hash",
        IncludedModules: []*object.Module{InitEnumerable()},
        Methods:         make(map[string]*object.Method),
    }
    
    // Hash 的 each 方法 (yield key, value)
    hashClass.Methods["each"] = &object.Method{
        Name:  "each",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
            hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
            if !ok {
                return R.NilVal
            }
            
            for key, value := range hash {
                keyVal := &object.EmeraldValue{
                    Type:  object.ValueString,
                    Data:  key,
                    Class: R.Classes["String"],
                }
                if block != nil {
                    CallBlock(block, keyVal, value)
                }
            }
            
            return receiver
        },
    }
    
    R.Classes["Hash"] = hashClass
}
```

## 实施计划

### 第 1 周: 基础设施和核心方法

**Day 1-2: 模块系统增强**
- [ ] 实现 Module 对象结构
- [ ] 实现 include 机制
- [ ] 实现方法查找链
- [ ] 测试: 基础 include 功能

**Day 3-4: Block/Closure 增强**
- [ ] 增强 Closure 结构
- [ ] 实现 OpYield, OpBlockGiven
- [ ] 实现 CallBlock 辅助函数
- [ ] 测试: Block 传递和调用

**Day 5: 核心迭代方法 (Part 1)**
- [ ] 实现 map/collect
- [ ] 实现 select/filter
- [ ] 实现 reject
- [ ] 测试: 基础迭代

### 第 2 周: 完整方法集

**Day 6-7: 核心迭代方法 (Part 2)**
- [ ] 实现 find/detect
- [ ] 实现 reduce/inject
- [ ] 实现 each_with_index
- [ ] 测试: 查找和累积

**Day 8-9: 查询方法**
- [ ] 实现 any?/all?/none?/one?
- [ ] 实现 count
- [ ] 实现 include?/member?
- [ ] 测试: 查询功能

**Day 10: 高级方法**
- [ ] 实现 sort/sort_by
- [ ] 实现 min/max/min_by/max_by
- [ ] 实现 group_by/partition
- [ ] 测试: 排序和分组

## 验收标准

### 功能验收
- [ ] Array 和 Hash 成功混入 Enumerable
- [ ] 实现 20+ Enumerable 方法
- [ ] 所有方法支持 block 参数
- [ ] 方法查找链正确 (类 -> 模块 -> 父类)

### 测试验收
- [ ] 通过 vendor/ruby/spec/core/enumerable/ 下 80%+ spec
- [ ] 通过 vendor/ruby/spec/core/array/ 中 Enumerable 相关 spec
- [ ] 通过 vendor/ruby/spec/core/hash/ 中 Enumerable 相关 spec

### 性能验收
- [ ] map 性能与直接循环相当 (< 2x 开销)
- [ ] select 性能与直接循环相当
- [ ] 无内存泄漏

## 风险和缓解

### 技术风险

**风险 1: Block 参数传递复杂**
- 影响: 高
- 概率: 中
- 缓解: 先实现简单的 Go 函数 block，再支持编译的 Ruby block

**风险 2: 方法查找性能**
- 影响: 中
- 概率: 低
- 缓解: 使用方法缓存，避免每次都遍历模块链

**风险 3: Enumerator 对象缺失**
- 影响: 中
- 概率: 高
- 缓解: 无 block 时暂时返回 nil，后续阶段实现 Enumerator

### 项目风险

**风险 1: 时间估算不足**
- 影响: 中
- 概率: 中
- 缓解: 优先实现 P0 方法，P1 方法可延后

## 参考资料

- Ruby Enumerable 文档: https://ruby-doc.org/core/Enumerable.html
- Ruby Spec: vendor/ruby/spec/core/enumerable/
- MRI 实现: enum.c

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
