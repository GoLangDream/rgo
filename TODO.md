# RGo 待办事项

## 阶段 1 遗留问题

### 解析器问题
- [ ] 解析器无限递归 - `[1]` 等数组导致无限循环
- [ ] `?` 方法名解析错误 - `10.odd?` 解析失败
- [ ] 哈希字面量解析错误 - `{a: 1}` 和 `{"a" => 1}` 都报错
- [ ] `==` 和 `!=` 解析错误
- [ ] `&&` 和 `||` 解析错误
- [ ] `? :` 三元运算符解析错误
- [ ] 多参数表达式解析: `puts 1, 2, 3` 报错
- [ ] 负数解析: `puts -5` 崩溃

### 虚拟机/编译器问题
- [ ] if 语句超时
- [ ] `x += 5` 复合赋值结果错误

### 已修复
- [x] `%` 运算符 - 修复了lexer中对%的错误处理
- [x] 多表达式输出 - 尝试修复但复杂，回退到只输出最后一个

### 已知正常功能 (通过 puts 测试)
- [x] puts 整数运算: `puts 1 + 2`, `puts 10 - 5`, `puts 10 * 5`
- [x] puts 模运算: `puts 17 % 5` (= 2)
- [x] puts 幂运算: `puts 2 ** 10` (= 1024)
- [x] puts 字符串连接: `puts "a" + "b" + "c"`
- [x] puts 字符串方法: `puts "hello".upcase`, `puts "HELLO".downcase`
- [x] puts 字符串索引: `puts "hello"[0]` (= h)
- [x] puts 字符串链式调用: `puts "abc".upcase.downcase`
- [x] puts 比较运算: `puts 10 > 5`, `puts 10 < 5`, `puts 10 >= 5`, `puts 5 <= 10`
- [x] puts 变量: `puts x + 3`
- [x] puts 整数方法: `puts 5.succ`, `puts 10.abs`, `puts 10.div 2`
- [x] puts 字面量: `puts true`, `puts false`, `puts nil`
- [x] puts 浮点数: `puts 1.5`

---

## 阶段 2 待实现

### 对象系统
- [ ] 完善类定义 (class/module)
- [ ] 完善方法定义 (def) - 支持参数和调用
- [ ] 闭包和块机制
- [ ] 实例变量 (@foo)
- [ ] 类变量 (@@foo)
- [ ] 全局变量 ($foo)

### 控制流
- [ ] if/unless/elsif/else
- [ ] while/until
- [ ] for 循环
- [ ] case/when

### 更多内置方法
- [ ] Symbol, Regexp, Range
- [ ] Proc, Lambda
- [ ] IO, File, Thread
- [ ] Exception

---

## 阶段 3 待实现

- [ ] 标准库 (Marshal, JSON, Time, Date)
- [ ] Kernel#require/load/eval

---

## 阶段 4 待实现

- [ ] ruby/spec 测试集成

---

## 阶段 5 待实现

- [ ] Rails 兼容层
