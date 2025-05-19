# RArray - Ruby风格的数组类

`RArray`是一个Go语言实现的模拟Ruby数组功能的类型。它提供了丰富的数组操作方法，可以让Go开发者使用类似Ruby的数组API。

## 创建RArray

```go
// 创建一个空数组
emptyArr := NewRArray([]Object{})

// 创建一个包含不同类型元素的数组
arr := NewRArray([]Object{
    NewRString("a"),
    NewRString("b"),
    NewRInteger(1),
})
```

## 基本方法

### 数组操作
- `First()`: 返回第一个元素
- `Last()`: 返回最后一个元素
- `Length()`: 返回数组长度
- `Size()`: Length的别名
- `Empty()`: 检查数组是否为空
- `Join(sep)`: 使用指定分隔符连接数组元素
- `Reverse()`: 反转数组
- `Sort()`: 排序数组
- `Uniq()`: 去除重复元素
- `Compact()`: 移除所有nil元素
- `Flatten()`: 展平嵌套数组

### 数组变换
- `Map(fn)`: 对每个元素应用函数并返回新数组
- `Select(fn)`: 选择满足条件的元素
- `Reject(fn)`: 排除满足条件的元素
- `Shuffle()`: 随机打乱数组

### 数组查询
- `Include(obj)`: 检查是否包含指定元素
- `Index(obj)`: 返回元素首次出现的位置
- `RIndex(obj)`: 返回元素最后出现的位置
- `Count(obj)`: 计算元素出现次数
- `Any(fn)`: 检查是否有元素满足条件
- `All(fn)`: 检查是否所有元素都满足条件
- `None(fn)`: 检查是否没有元素满足条件

### 数组切片
- `Slice(start, end)`: 返回指定范围的子数组
- `SliceFrom(start)`: 返回从指定位置到结尾的子数组
- `Take(n)`: 返回前n个元素
- `Drop(n)`: 返回除前n个元素外的所有元素

### 数组分组
- `GroupBy(fn)`: 按指定条件分组
- `Partition(fn)`: 将数组分为满足条件和不满足条件的两部分

### 数组迭代
- `Each(fn)`: 对每个元素执行操作
- `EachWithIndex(fn)`: 对每个元素及其索引执行操作
- `EachCons(n, fn)`: 对每个连续n个元素执行操作
- `EachSlice(n, fn)`: 将数组分成n个元素的切片并执行操作

## 使用示例

```go
import "github.com/GoLangDream/rgo"

// 创建数组
arr := rgo.NewRArray([]rgo.Object{
    rgo.NewRString("a"),
    rgo.NewRString("b"),
    rgo.NewRInteger(1),
})

// 数组操作
first := arr.First()                  // 返回 "a"
length := arr.Length()                // 返回 3
joined := arr.Join(", ")              // 返回 "a, b, 1"

// 数组变换
mapped := arr.Map(func(obj rgo.Object) rgo.Object {
    if str, ok := obj.(rgo.RString); ok {
        return rgo.NewRString(str.ToString() + "!")
    }
    return obj
})
// mapped 包含 ["a!", "b!", 1]

// 数组查询
hasA := arr.Include(rgo.NewRString("a"))  // 返回 true
count := arr.Count(rgo.NewRString("a"))   // 返回 1

// 数组切片
subArr := arr.Slice(0, 2)  // 返回 ["a", "b"]

// 数组分组
groups := arr.GroupBy(func(obj rgo.Object) rgo.Object {
    if _, ok := obj.(rgo.RString); ok {
        return rgo.NewRString("string")
    }
    return rgo.NewRString("integer")
})
// groups 包含 {"string": ["a", "b"], "integer": [1]}

// 数组迭代
arr.Each(func(obj rgo.Object) {
    fmt.Println(obj.ToString())
})

// 使用EachWithIndex
arr.EachWithIndex(func(obj rgo.Object, index int) {
    fmt.Printf("%d: %s\n", index, obj.ToString())
})

// 使用EachCons
arr.EachCons(2, func(subArr rgo.RArray) {
    fmt.Println(subArr.ToString())
})

// 使用EachSlice
arr.EachSlice(2, func(subArr rgo.RArray) {
    fmt.Println(subArr.ToString())
})
```

## 完整方法列表

以下是RArray提供的所有方法：

| 方法名 | 功能描述 |
|--------|----------|
| `ToString()` | 返回数组的字符串表示 |
| `Equal(other)` | 比较两个对象是否相等 |
| `Length()` | 返回数组长度 |
| `Size()` | Length的别名 |
| `Empty()` | 检查数组是否为空 |
| `First()` | 返回第一个元素 |
| `Last()` | 返回最后一个元素 |
| `Include(obj)` | 检查是否包含指定元素 |
| `Push(obj)` | 将元素添加到数组末尾 |
| `Pop()` | 移除并返回最后一个元素 |
| `Join(sep)` | 使用指定分隔符连接数组元素 |
| `Map(fn)` | 对每个元素应用函数并返回新数组 |
| `Select(fn)` | 选择满足条件的元素 |
| `Reject(fn)` | 排除满足条件的元素 |
| `Reverse()` | 反转数组 |
| `Shuffle()` | 随机打乱数组 |
| `Sort()` | 排序数组 |
| `Uniq()` | 去除重复元素 |
| `Get(index)` | 获取指定索引的元素 |
| `ToArray()` | 返回底层数组 |
| `Compact()` | 移除所有nil元素 |
| `Flatten()` | 展平嵌套数组 |
| `Index(obj)` | 返回元素首次出现的位置 |
| `RIndex(obj)` | 返回元素最后出现的位置 |
| `Count(obj)` | 计算元素出现次数 |
| `Any(fn)` | 检查是否有元素满足条件 |
| `All(fn)` | 检查是否所有元素都满足条件 |
| `None(fn)` | 检查是否没有元素满足条件 |
| `Slice(start, end)` | 返回指定范围的子数组 |
| `SliceFrom(start)` | 返回从指定位置到结尾的子数组 |
| `Take(n)` | 返回前n个元素 |
| `Drop(n)` | 返回除前n个元素外的所有元素 |
| `GroupBy(fn)` | 按指定条件分组 |
| `Partition(fn)` | 将数组分为满足条件和不满足条件的两部分 |
| `Each(fn)` | 对每个元素执行操作 |
| `EachWithIndex(fn)` | 对每个元素及其索引执行操作 |
| `EachCons(n, fn)` | 对每个连续n个元素执行操作 |
| `EachSlice(n, fn)` | 将数组分成n个元素的切片并执行操作 |
