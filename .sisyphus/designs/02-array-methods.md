# 阶段 2: Array 核心方法补全技术设计

**优先级**: P0  
**预计时间**: 2 周  
**依赖**: 阶段 1 (Enumerable 模块)  
**被依赖**: 阶段 10 (Rails)

## 目标

将 Array 方法覆盖率从当前的 20% 提升到 80%+，实现 60+ 核心方法，通过 vendor/ruby/spec/core/array/ 下 80%+ 的 spec。

## 当前状态

### 已实现 (19 个方法)
- 基础: `length`, `size`, `empty?`, `clear`
- 访问: `first`, `last`, `[]`, `sample`
- 修改: `push`, `pop`, `shift`, `unshift`, `delete_at`, `concat`
- 迭代: `each`, `map`, `select`, `find`
- 转换: `join`, `reverse`
- 查询: `include?`

### 需要实现 (60+ 个方法)

#### P0 - 高频方法 (第 1 周)
**修改方法**:
- `<<` - 追加元素 (push 的别名)
- `insert(index, *objects)` - 在指定位置插入
- `fill(obj, start=0, length=nil)` - 填充
- `compact` / `compact!` - 移除 nil
- `flatten(level=-1)` / `flatten!` - 扁平化
- `uniq` / `uniq!` - 去重
- `[]=` - 索引赋值

**查询方法**:
- `index(obj)` / `find_index` - 查找索引
- `rindex(obj)` - 反向查找索引
- `count(obj=nil)` - 计数
- `empty?` - 是否为空 (已实现)
- `any?` / `all?` / `none?` - 条件查询 (Enumerable)

**迭代方法**:
- `each_index` - 迭代索引
- `each_with_index` - 迭代元素和索引 (Enumerable)
- `map!` / `collect!` - 原地映射
- `select!` / `filter!` - 原地筛选
- `reject` / `reject!` - 反向筛选
- `keep_if` / `delete_if` - 条件保留/删除

#### P1 - 重要方法 (第 2 周)
**集合运算**:
- `&` - 交集
- `|` - 并集
- `-` - 差集
- `+` - 连接

**排序方法**:
- `sort` / `sort!` - 排序
- `sort_by` - 按条件排序
- `shuffle` / `shuffle!` - 随机打乱
- `rotate(count=1)` / `rotate!` - 旋转

**切片方法**:
- `slice(index)` / `slice(start, length)` - 切片
- `slice!(index)` - 删除并返回切片
- `take(n)` - 取前 n 个
- `take_while` - 取满足条件的前缀
- `drop(n)` - 跳过前 n 个
- `drop_while` - 跳过满足条件的前缀
- `values_at(*indices)` - 获取多个索引的值

**转换方法**:
- `to_h` - 转换为 Hash
- `to_s` / `inspect` - 转换为字符串
- `to_a` - 转换为数组 (返回自身)
- `pack(template)` - 打包为二进制字符串

**其他方法**:
- `zip(*arrays)` - 合并多个数组
- `transpose` - 转置
- `product(*arrays)` - 笛卡尔积
- `permutation(n=nil)` - 排列
- `combination(n)` - 组合
- `repeated_permutation(n)` - 重复排列
- `repeated_combination(n)` - 重复组合

## 技术设计

### 1. 修改方法实现

#### 1.1 << 操作符

```go
// pkg/core/array.go
arrayClass.DefineMethod("<<", &object.Method{
    Name:  "<<",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok || len(args) == 0 {
            return receiver
        }
        
        arr = append(arr, args[0])
        receiver.Data = arr
        return receiver  // 返回 self 以支持链式调用
    },
})
```

#### 1.2 compact / compact!

```go
arrayClass.DefineMethod("compact", &object.Method{
    Name:  "compact",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make([]*object.EmeraldValue, 0, len(arr))
        for _, item := range arr {
            if item.Type != object.ValueNil {
                result = append(result, item)
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})

arrayClass.DefineMethod("compact!", &object.Method{
    Name:  "compact!",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok {
            return R.NilVal
        }
        
        result := make([]*object.EmeraldValue, 0, len(arr))
        changed := false
        for _, item := range arr {
            if item.Type != object.ValueNil {
                result = append(result, item)
            } else {
                changed = true
            }
        }
        
        if !changed {
            return R.NilVal  // 没有变化返回 nil
        }
        
        receiver.Data = result
        return receiver
    },
})
```

#### 1.3 flatten / flatten!

```go
func flattenArray(arr []*object.EmeraldValue, level int) []*object.EmeraldValue {
    if level == 0 {
        return arr
    }
    
    result := make([]*object.EmeraldValue, 0)
    for _, item := range arr {
        if item.Type == object.ValueArray {
            nested, ok := item.Data.([]*object.EmeraldValue)
            if ok {
                flattened := flattenArray(nested, level-1)
                result = append(result, flattened...)
            } else {
                result = append(result, item)
            }
        } else {
            result = append(result, item)
        }
    }
    return result
}

arrayClass.DefineMethod("flatten", &object.Method{
    Name:  "flatten",
    Arity: -1,  // 可选参数
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        level := -1  // 默认完全扁平化
        if len(args) > 0 {
            if lvl, ok := args[0].Data.(int64); ok {
                level = int(lvl)
            }
        }
        
        result := flattenArray(arr, level)
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

#### 1.4 uniq / uniq!

```go
arrayClass.DefineMethod("uniq", &object.Method{
    Name:  "uniq",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        seen := make(map[string]bool)
        result := make([]*object.EmeraldValue, 0)
        
        for _, item := range arr {
            key := item.Inspect()  // 使用 Inspect 作为唯一键
            if !seen[key] {
                seen[key] = true
                result = append(result, item)
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

### 2. 集合运算

#### 2.1 交集 (&)

```go
arrayClass.DefineMethod("&", &object.Method{
    Name:  "&",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr1, ok1 := receiver.Data.([]*object.EmeraldValue)
        if !ok1 || len(args) == 0 {
            return receiver
        }
        
        arr2, ok2 := args[0].Data.([]*object.EmeraldValue)
        if !ok2 {
            return receiver
        }
        
        // 构建 arr2 的查找表
        set2 := make(map[string]bool)
        for _, item := range arr2 {
            set2[item.Inspect()] = true
        }
        
        // 查找交集 (保持 arr1 的顺序，去重)
        seen := make(map[string]bool)
        result := make([]*object.EmeraldValue, 0)
        for _, item := range arr1 {
            key := item.Inspect()
            if set2[key] && !seen[key] {
                seen[key] = true
                result = append(result, item)
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

#### 2.2 并集 (|)

```go
arrayClass.DefineMethod("|", &object.Method{
    Name:  "|",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr1, ok1 := receiver.Data.([]*object.EmeraldValue)
        if !ok1 || len(args) == 0 {
            return receiver
        }
        
        arr2, ok2 := args[0].Data.([]*object.EmeraldValue)
        if !ok2 {
            return receiver
        }
        
        seen := make(map[string]bool)
        result := make([]*object.EmeraldValue, 0)
        
        // 添加 arr1 的元素 (去重)
        for _, item := range arr1 {
            key := item.Inspect()
            if !seen[key] {
                seen[key] = true
                result = append(result, item)
            }
        }
        
        // 添加 arr2 的元素 (去重)
        for _, item := range arr2 {
            key := item.Inspect()
            if !seen[key] {
                seen[key] = true
                result = append(result, item)
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

### 3. 排序方法

#### 3.1 sort / sort!

```go
import "sort"

arrayClass.DefineMethod("sort", &object.Method{
    Name:  "sort",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        // 复制数组
        result := make([]*object.EmeraldValue, len(arr))
        copy(result, arr)
        
        // 排序
        sort.Slice(result, func(i, j int) bool {
            if block != nil {
                // 使用自定义比较 block
                cmp := CallBlock(block, result[i], result[j])
                if cmpInt, ok := cmp.Data.(int64); ok {
                    return cmpInt < 0
                }
                return false
            }
            
            // 默认比较 (使用 <=> 操作符)
            return compareValues(result[i], result[j]) < 0
        })
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})

// 辅助函数: 比较两个值
func compareValues(a, b *object.EmeraldValue) int {
    // Integer 比较
    if a.Type == object.ValueInteger && b.Type == object.ValueInteger {
        aInt := a.Data.(int64)
        bInt := b.Data.(int64)
        if aInt < bInt {
            return -1
        } else if aInt > bInt {
            return 1
        }
        return 0
    }
    
    // String 比较
    if a.Type == object.ValueString && b.Type == object.ValueString {
        aStr := a.Data.(string)
        bStr := b.Data.(string)
        if aStr < bStr {
            return -1
        } else if aStr > bStr {
            return 1
        }
        return 0
    }
    
    // Float 比较
    if a.Type == object.ValueFloat && b.Type == object.ValueFloat {
        aFloat := a.Data.(float64)
        bFloat := b.Data.(float64)
        if aFloat < bFloat {
            return -1
        } else if aFloat > bFloat {
            return 1
        }
        return 0
    }
    
    return 0
}
```

### 4. 切片方法

#### 4.1 slice

```go
arrayClass.DefineMethod("slice", &object.Method{
    Name:  "slice",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok || len(args) == 0 {
            return R.NilVal
        }
        
        length := len(arr)
        
        if len(args) == 1 {
            // slice(index)
            index, ok := args[0].Data.(int64)
            if !ok {
                return R.NilVal
            }
            
            // 处理负索引
            if index < 0 {
                index = int64(length) + index
            }
            
            if index < 0 || index >= int64(length) {
                return R.NilVal
            }
            
            return arr[index]
        } else if len(args) == 2 {
            // slice(start, length)
            start, ok1 := args[0].Data.(int64)
            sliceLen, ok2 := args[1].Data.(int64)
            if !ok1 || !ok2 {
                return R.NilVal
            }
            
            // 处理负索引
            if start < 0 {
                start = int64(length) + start
            }
            
            if start < 0 || start >= int64(length) {
                return R.NilVal
            }
            
            end := start + sliceLen
            if end > int64(length) {
                end = int64(length)
            }
            
            result := arr[start:end]
            return &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  result,
                Class: R.Classes["Array"],
            }
        }
        
        return R.NilVal
    },
})
```

#### 4.2 take / drop

```go
arrayClass.DefineMethod("take", &object.Method{
    Name:  "take",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok || len(args) == 0 {
            return receiver
        }
        
        n, ok := args[0].Data.(int64)
        if !ok || n < 0 {
            return receiver
        }
        
        if n > int64(len(arr)) {
            n = int64(len(arr))
        }
        
        result := arr[:n]
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})

arrayClass.DefineMethod("drop", &object.Method{
    Name:  "drop",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        arr, ok := receiver.Data.([]*object.EmeraldValue)
        if !ok || len(args) == 0 {
            return receiver
        }
        
        n, ok := args[0].Data.(int64)
        if !ok || n < 0 {
            return receiver
        }
        
        if n > int64(len(arr)) {
            n = int64(len(arr))
        }
        
        result := arr[n:]
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

## 实施计划

### 第 1 周: P0 高频方法

**Day 1-2: 修改方法**
- [ ] 实现 <<, insert, fill
- [ ] 实现 compact/compact!, flatten/flatten!
- [ ] 实现 uniq/uniq!, []=
- [ ] 测试: 修改方法

**Day 3-4: 查询和迭代方法**
- [ ] 实现 index, rindex, count
- [ ] 实现 each_index, map!, select!
- [ ] 实现 reject/reject!, keep_if, delete_if
- [ ] 测试: 查询和迭代

**Day 5: 集成测试**
- [ ] 运行 array/ 下的 spec
- [ ] 修复发现的问题
- [ ] 性能测试

### 第 2 周: P1 重要方法

**Day 6-7: 集合运算和排序**
- [ ] 实现 &, |, -, +
- [ ] 实现 sort/sort!, sort_by
- [ ] 实现 shuffle/shuffle!, rotate/rotate!
- [ ] 测试: 集合和排序

**Day 8-9: 切片和转换**
- [ ] 实现 slice/slice!, take/drop
- [ ] 实现 take_while, drop_while, values_at
- [ ] 实现 to_h, to_s, inspect
- [ ] 测试: 切片和转换

**Day 10: 高级方法和验收**
- [ ] 实现 zip, transpose, product
- [ ] 实现 permutation, combination
- [ ] 运行完整 spec 套件
- [ ] 性能优化

## 验收标准

### 功能验收
- [ ] 实现 60+ Array 方法
- [ ] 所有修改方法 (!) 正确修改原数组
- [ ] 所有非修改方法返回新数组
- [ ] 支持负索引
- [ ] 支持 block 参数

### 测试验收
- [ ] 通过 vendor/ruby/spec/core/array/ 下 80%+ spec
- [ ] 所有 P0 方法 100% 通过
- [ ] 所有 P1 方法 80%+ 通过

### 性能验收
- [ ] 基础操作 (push, pop, []) 与 Go slice 性能相当
- [ ] 排序性能与 Go sort 相当
- [ ] 无内存泄漏

## 风险和缓解

### 技术风险

**风险 1: 负索引处理复杂**
- 影响: 中
- 概率: 低
- 缓解: 创建统一的索引规范化函数

**风险 2: 修改方法 (!) 的副作用**
- 影响: 高
- 概率: 中
- 缓解: 严格测试，确保只修改原数组

**风险 3: 排序的稳定性**
- 影响: 低
- 概率: 低
- 缓解: 使用 Go 的 sort.SliceStable

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
