# 阶段 4: Hash 核心方法补全技术设计

**优先级**: P0  
**预计时间**: 1 周  
**依赖**: 阶段 1 (Enumerable 模块)  
**被依赖**: 阶段 10 (Rails)

## 目标

将 Hash 方法覆盖率从当前的 30% 提升到 80%+，实现 30+ 核心方法，通过 vendor/ruby/spec/core/hash/ 下 80%+ 的 spec。

## 当前状态

### 已实现 (17 个方法)
- 访问: `[]`, `[]=`, `fetch`
- 查询: `keys`, `values`, `length`, `size`, `empty?`, `key?`, `has_key?`, `include?`, `has_value?`
- 迭代: `each`, `each_key`, `each_value`
- 修改: `merge`, `delete`, `clear`

### 需要实现 (30+ 个方法)

#### P0 - 高频方法 (Day 1-3)
**修改方法**:
- `merge!` / `update` - 原地合并
- `delete_if` - 删除满足条件的键值对
- `keep_if` - 保留满足条件的键值对
- `select!` / `filter!` - 原地筛选
- `reject!` - 原地反向筛选
- `compact` / `compact!` - 移除 nil 值
- `replace(other_hash)` - 完全替换

**查询方法**:
- `dig(*keys)` - 深度访问
- `fetch_values(*keys)` - 获取多个值
- `value?(value)` - 是否包含值
- `member?(key)` - 是否包含键（has_key? 的别名）
- `default` / `default=` - 默认值
- `default_proc` / `default_proc=` - 默认值 Proc

**迭代方法**:
- `each_pair` - 迭代键值对（each 的别名）
- `map` - 映射（返回数组）
- `select` / `filter` - 筛选
- `reject` - 反向筛选
- `transform_keys` / `transform_keys!` - 转换键
- `transform_values` / `transform_values!` - 转换值

#### P1 - 重要方法 (Day 4-5)
**转换方法**:
- `to_a` - 转换为数组
- `to_h` - 转换为 Hash（返回自身）
- `to_s` / `inspect` - 转换为字符串
- `invert` - 键值互换
- `flatten(level=1)` - 扁平化

**其他方法**:
- `assoc(key)` - 查找键值对
- `rassoc(value)` - 反向查找键值对
- `shift` - 删除并返回第一个键值对
- `compare_by_identity` - 使用对象标识比较
- `rehash` - 重新计算哈希值
- `slice(*keys)` - 提取子集
- `except(*keys)` - 排除键

## 技术设计

### 1. 修改方法实现

#### 1.1 merge! / update

```go
// pkg/core/hash.go
hashClass.DefineMethod("merge!", &object.Method{
    Name:  "merge!",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok || len(args) == 0 {
            return receiver
        }
        
        otherHash, ok := args[0].Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        // 合并到原 hash
        for key, value := range otherHash {
            if block != nil {
                // 如果提供了 block，用于处理冲突
                if existingValue, exists := hash[key]; exists {
                    keyVal := &object.EmeraldValue{
                        Type:  object.ValueString,
                        Data:  key,
                        Class: R.Classes["String"],
                    }
                    hash[key] = CallBlock(block, keyVal, existingValue, value)
                } else {
                    hash[key] = value
                }
            } else {
                hash[key] = value
            }
        }
        
        return receiver
    },
})

// update 是 merge! 的别名
hashClass.DefineMethod("update", hashClass.Methods["merge!"])
```

#### 1.2 delete_if / keep_if

```go
hashClass.DefineMethod("delete_if", &object.Method{
    Name:  "delete_if",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if block == nil {
            // TODO: 返回 Enumerator
            return receiver
        }
        
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        // 收集要删除的键
        keysToDelete := make([]string, 0)
        for key, value := range hash {
            keyVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  key,
                Class: R.Classes["String"],
            }
            
            result := CallBlock(block, keyVal, value)
            if IsTruthy(result) {
                keysToDelete = append(keysToDelete, key)
            }
        }
        
        // 删除键
        for _, key := range keysToDelete {
            delete(hash, key)
        }
        
        return receiver
    },
})

hashClass.DefineMethod("keep_if", &object.Method{
    Name:  "keep_if",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if block == nil {
            return receiver
        }
        
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        keysToDelete := make([]string, 0)
        for key, value := range hash {
            keyVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  key,
                Class: R.Classes["String"],
            }
            
            result := CallBlock(block, keyVal, value)
            if !IsTruthy(result) {  // 反向逻辑
                keysToDelete = append(keysToDelete, key)
            }
        }
        
        for _, key := range keysToDelete {
            delete(hash, key)
        }
        
        return receiver
    },
})
```

#### 1.3 compact / compact!

```go
hashClass.DefineMethod("compact", &object.Method{
    Name:  "compact",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make(map[string]*object.EmeraldValue)
        for key, value := range hash {
            if value.Type != object.ValueNil {
                result[key] = value
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})

hashClass.DefineMethod("compact!", &object.Method{
    Name:  "compact!",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return R.NilVal
        }
        
        keysToDelete := make([]string, 0)
        for key, value := range hash {
            if value.Type == object.ValueNil {
                keysToDelete = append(keysToDelete, key)
            }
        }
        
        if len(keysToDelete) == 0 {
            return R.NilVal  // 没有变化返回 nil
        }
        
        for _, key := range keysToDelete {
            delete(hash, key)
        }
        
        return receiver
    },
})
```

### 2. 查询方法

#### 2.1 dig

```go
hashClass.DefineMethod("dig", &object.Method{
    Name:  "dig",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        if len(args) == 0 {
            return R.NilVal
        }
        
        current := receiver
        
        for _, keyArg := range args {
            if current.Type == object.ValueNil {
                return R.NilVal
            }
            
            if current.Type == object.ValueHash {
                hash, ok := current.Data.(map[string]*object.EmeraldValue)
                if !ok {
                    return R.NilVal
                }
                
                key := keyArg.Inspect()
                value, exists := hash[key]
                if !exists {
                    return R.NilVal
                }
                current = value
            } else if current.Type == object.ValueArray {
                arr, ok := current.Data.([]*object.EmeraldValue)
                if !ok {
                    return R.NilVal
                }
                
                index, ok := keyArg.Data.(int64)
                if !ok {
                    return R.NilVal
                }
                
                if index < 0 {
                    index = int64(len(arr)) + index
                }
                
                if index < 0 || index >= int64(len(arr)) {
                    return R.NilVal
                }
                
                current = arr[index]
            } else {
                return R.NilVal
            }
        }
        
        return current
    },
})
```

#### 2.2 fetch_values

```go
hashClass.DefineMethod("fetch_values", &object.Method{
    Name:  "fetch_values",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make([]*object.EmeraldValue, len(args))
        
        for i, keyArg := range args {
            key := keyArg.Inspect()
            value, exists := hash[key]
            if !exists {
                // TODO: 抛出 KeyError 异常
                return R.NilVal
            }
            result[i] = value
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})
```

#### 2.3 default / default_proc

```go
// 需要扩展 Hash 的内部结构
type HashData struct {
    Data        map[string]*object.EmeraldValue
    Default     *object.EmeraldValue
    DefaultProc *object.Closure
}

hashClass.DefineMethod("default", &object.Method{
    Name:  "default",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hashData, ok := receiver.Data.(*HashData)
        if !ok {
            return R.NilVal
        }
        
        if hashData.Default != nil {
            return hashData.Default
        }
        
        return R.NilVal
    },
})

hashClass.DefineMethod("default=", &object.Method{
    Name:  "default=",
    Arity: 1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hashData, ok := receiver.Data.(*HashData)
        if !ok || len(args) == 0 {
            return R.NilVal
        }
        
        hashData.Default = args[0]
        hashData.DefaultProc = nil  // 清除 default_proc
        
        return args[0]
    },
})
```

### 3. 迭代方法

#### 3.1 transform_keys / transform_values

```go
hashClass.DefineMethod("transform_keys", &object.Method{
    Name:  "transform_keys",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if block == nil {
            return receiver
        }
        
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make(map[string]*object.EmeraldValue)
        
        for key, value := range hash {
            keyVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  key,
                Class: R.Classes["String"],
            }
            
            newKey := CallBlock(block, keyVal)
            newKeyStr := newKey.Inspect()
            result[newKeyStr] = value
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})

hashClass.DefineMethod("transform_values", &object.Method{
    Name:  "transform_values",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, block *object.Closure, args ...*object.EmeraldValue) *object.EmeraldValue {
        if block == nil {
            return receiver
        }
        
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make(map[string]*object.EmeraldValue)
        
        for key, value := range hash {
            newValue := CallBlock(block, value)
            result[key] = newValue
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})
```

### 4. 转换方法

#### 4.1 to_a / invert

```go
hashClass.DefineMethod("to_a", &object.Method{
    Name:  "to_a",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make([]*object.EmeraldValue, 0, len(hash))
        
        for key, value := range hash {
            keyVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  key,
                Class: R.Classes["String"],
            }
            
            pair := []*object.EmeraldValue{keyVal, value}
            pairVal := &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  pair,
                Class: R.Classes["Array"],
            }
            
            result = append(result, pairVal)
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueArray,
            Data:  result,
            Class: R.Classes["Array"],
        }
    },
})

hashClass.DefineMethod("invert", &object.Method{
    Name:  "invert",
    Arity: 0,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make(map[string]*object.EmeraldValue)
        
        for key, value := range hash {
            valueStr := value.Inspect()
            keyVal := &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  key,
                Class: R.Classes["String"],
            }
            result[valueStr] = keyVal
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})
```

#### 4.2 slice / except

```go
hashClass.DefineMethod("slice", &object.Method{
    Name:  "slice",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        result := make(map[string]*object.EmeraldValue)
        
        for _, keyArg := range args {
            key := keyArg.Inspect()
            if value, exists := hash[key]; exists {
                result[key] = value
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})

hashClass.DefineMethod("except", &object.Method{
    Name:  "except",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        hash, ok := receiver.Data.(map[string]*object.EmeraldValue)
        if !ok {
            return receiver
        }
        
        // 构建排除键的集合
        excludeKeys := make(map[string]bool)
        for _, keyArg := range args {
            key := keyArg.Inspect()
            excludeKeys[key] = true
        }
        
        result := make(map[string]*object.EmeraldValue)
        for key, value := range hash {
            if !excludeKeys[key] {
                result[key] = value
            }
        }
        
        return &object.EmeraldValue{
            Type:  object.ValueHash,
            Data:  result,
            Class: R.Classes["Hash"],
        }
    },
})
```

## 实施计划

### Day 1-2: 修改方法
- [ ] 实现 merge!/update
- [ ] 实现 delete_if, keep_if
- [ ] 实现 select!/filter!, reject!
- [ ] 实现 compact/compact!
- [ ] 测试: 修改方法

### Day 3: 查询方法
- [ ] 实现 dig, fetch_values
- [ ] 实现 value?, member?
- [ ] 实现 default/default=, default_proc
- [ ] 测试: 查询方法

### Day 4: 迭代和转换
- [ ] 实现 map, select, reject
- [ ] 实现 transform_keys/transform_values
- [ ] 实现 to_a, invert, flatten
- [ ] 测试: 迭代和转换

### Day 5: 其他方法和验收
- [ ] 实现 assoc, rassoc, shift
- [ ] 实现 slice, except
- [ ] 运行完整 spec 套件
- [ ] 性能优化

## 验收标准

### 功能验收
- [ ] 实现 30+ Hash 方法
- [ ] 所有修改方法 (!) 正确修改原 Hash
- [ ] 支持 block 参数
- [ ] dig 支持嵌套访问

### 测试验收
- [ ] 通过 vendor/ruby/spec/core/hash/ 下 80%+ spec
- [ ] 所有 P0 方法 100% 通过

### 性能验收
- [ ] 基础操作 ([], []=) 与 Go map 性能相当
- [ ] 无内存泄漏

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
