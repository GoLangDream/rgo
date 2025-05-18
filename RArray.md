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

### 数组变换
- `Map(fn)`: 对每个元素应用函数并返回新数组
- `Select(fn)`: 选择满足条件的元素
- `Reject(fn)`: 排除满足条件的元素
- `Compact()`: 移除所有nil元素
- `Flatten()`: 展平嵌套数组
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
arr := goby.NewRArray([]goby.Object{
    goby.NewRString("a"),
    goby.NewRString("b"),
    goby.NewRInteger(1),
})

// 数组操作
first := arr.First()                  // 返回 "a"
length := arr.Length()                // 返回 3
joined := arr.Join(", ")              // 返回 "a, b, 1"

// 数组变换
mapped := arr.Map(func(obj goby.Object) goby.Object {
    if str, ok := obj.(goby.RString); ok {
        return goby.NewRString(str.ToString() + "!")
    }
    return obj
})
// mapped 包含 ["a!", "b!", 1]

// 数组查询
hasA := arr.Include(goby.NewRString("a"))  // 返回 true
count := arr.Count(goby.NewRString("a"))   // 返回 1

// 数组切片
subArr := arr.Slice(0, 2)  // 返回 ["a", "b"]

// 数组迭代
arr.Each(func(obj goby.Object) {
    fmt.Println(obj.ToString())
})
```
