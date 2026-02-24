# RGo 实现计划

## 项目目标

用 Go 从头实现一个完整的 Ruby 运行时，最终能够运行 Rails 框架。

## 整体架构

```
源代码 (.rb)
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Lexer (vm/lexer)                                      │
│  - 词法分析: 字符流 → Token 流                          │
│  - 使用通道 (channel) 并行处理                         │
│  - 支持字符串模板的嵌套词法分析                         │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Parser (vm/parser)                                    │
│  - Pratt Parser (运算符优先级解析器)                    │
│  - 递归下降 + 优先级表                                 │
│  - AST: 抽象语法树                                     │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Compiler (vm/compiler)                                │
│  - AST → 字节码                                        │
│  - 符号表管理                                          │
│  - 常量池管理                                          │
│  - 作用域管理                                          │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Virtual Machine (vm)                                  │
│  - 栈机器 (Stack Machine)                              │
│  - Fetch-Decode-Execute 循环                           │
│  - 基于 Fiber 的执行单元                               │
│  - 栈 + 调用帧 (Frame)                                 │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Core Classes (core/)                                  │
│  - Object, Class, Module                              │
│  - String, Array, Hash, Integer, Float                │
│  - Symbol, Regexp, Range, Proc, Method                │
│  - IO, File, Thread, Exception                        │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Standard Library (stdlib/)                            │
│  - Kernel, Enumerable, Comparable                     │
│  - Marshal, JSON, FileUtils, Pathname                 │
│  - Time, Date, DateTime                               │
└─────────────────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  Rails Compatibility Layer (rails/)                    │
│  - ActionDispatch, ActionController                   │
│  - ActiveRecord, ActionView                           │
│  - Rails Router                                       │
└─────────────────────────────────────────────────────────┘
```

---

## 实现阶段

### 阶段 1: 基础架构 (第 1-4 周)

#### 1.1 项目结构

```
rgo/
├── cmd/rgo/              # CLI 入口
│   └── main.go
├── vm/                   # 虚拟机核心
│   ├── lexer/            # 词法分析器
│   │   ├── lexer.go
│   │   ├── token.go
│   │   └── position.go
│   ├── parser/           # 语法解析器
│   │   ├── parser.go
│   │   ├── ast/
│   │   │   ├── node.go
│   │   │   ├── expression.go
│   │   │   └── statement.go
│   │   └── precedence.go
│   ├── compiler/         # 字节码编译器
│   │   ├── compiler.go
│   │   ├── bytecode.go
│   │   ├── opcode.go
│   │   └── symbol.go
│   ├── vm/              # 虚拟机
│   │   ├── vm.go
│   │   ├── frame.go
│   │   ├── fiber.go
│   │   └── stack.go
│   └── object/          # 对象系统
│       ├── value.go
│       ├── class.go
│       ├── module.go
│       └── context.go
├── core/                # Ruby 核心类
│   ├── init.go
│   ├── basic_object.go
│   ├── object.go
│   ├── module.go
│   ├── class.go
│   ├── kernel.go
│   ├── string.go
│   ├── array.go
│   ├── hash.go
│   ├── integer.go
│   ├── float.go
│   ├── symbol.go
│   ├── regexp.go
│   ├── range.go
│   ├── proc.go
│   ├── method.go
│   ├── io.go
│   ├── file.go
│   ├── thread.go
│   ├── exception.go
│   ├── true.go
│   ├── false.go
│   └── nil.go
├── stdlib/              # 标准库
│   ├── enumerable.go
│   ├── comparable.go
│   ├── marshal.go
│   ├── json.go
│   ├── time.go
│   └── file_utils.go
├── rails/               # Rails 兼容层
│   ├── action_dispatch/
│   ├── action_controller/
│   ├── active_record/
│   └── action_view/
├── spec/                # 测试套件
│   └── ruby/           # ruby/spec 克隆
└── tests/              # 内部测试
```

#### 1.2 词法分析器 (Lexer)

**文件**: `vm/lexer/lexer.go`

**核心数据结构**:

```go
type Lexer struct {
    input        *Input
    position     int      // 当前字符位置
    readPosition int     // 下一个字符位置
    ch           rune     // 当前字符
    line         int
    column       int
}
```

**实现任务**:
- [ ] 实现 `readChar()` - 读取下一个字符
- [ ] 实现 `peekChar()` - 查看下一个字符（不消费）
- [ ] 实现 Token 定义 (token.go)
- [ ] 实现标识符和关键字识别
- [ ] 实现数字词法分析（支持二进制、八进制、十六进制）
- [ ] 实现字符串词法分析（单引号、双引号）
- [ ] 实现字符串插值 `"Hello #{name}"`
- [ ] 实现 heredoc
- [ ] 实现运算符和符号识别

#### 1.3 解析器 (Parser) - Pratt Parser

**文件**: `vm/parser/parser.go`

**核心数据结构**:

```go
type Parser struct {
    lexer        *lexer.Lexer
    curToken     lexer.Token
    peekToken    lexer.Token
    prefixFns    map[lexer.TokenType]prefixParseFn
    infixFns     map[lexer.TokenType]infixParseFn
    errors       []string
}
```

**优先级表** (从低到高):

```go
const (
    LOWEST      int = iota
    MODIFIER    // val = 10 if true
    ASSIGN      // =
    TERNARY     // ?, :
    BOOL_OR     // ||
    BOOL_AND    // &&
    COMPARATOR  // ==
    ORDERING    // > or <
    SUM         // + or -
    PRODUCT     // * /
    PREFIX      // -X or !X
    CALL        // myFunction(X)
    ACCESSOR    // myHash.property
)
```

**实现任务**:
- [ ] 实现 AST 节点定义
- [ ] 实现 Pratt Parser 核心逻辑
- [ ] 实现前缀表达式解析（字面量、一元运算符）
- [ ] 实现中缀表达式解析（二元运算符）
- [ ] 实现方法调用解析
- [ ] 实现 if/unless/case 解析
- [ ] 实现循环 (while/until/for) 解析
- [ ] 实现方法定义解析
- [ ] 实现类/模块定义解析
- [ ] 实现块 (block) 解析

#### 1.4 编译器 - AST 到字节码

**文件**: `vm/compiler/compiler.go`

**核心数据结构**:

```go
type Compiler struct {
    constants    []object.EmeraldValue  // 常量池
    symbolTable  *SymbolTable            // 符号表
    scopes       []CompilationScope      // 作用域栈
    scopeIndex   int
}
```

**字节码操作码** (vm/compiler/opcode.go):

```go
const (
    // 常量操作
    OpPushConstant      // 从常量池推送常量到栈
    OpPop               // 弹出栈顶元素
    
    // 算术运算
    OpAdd, OpSub, OpMul, OpDiv
    OpMod, OpPow
    OpNeg
    
    // 比较运算
    OpEqual, OpNotEqual
    OpGreaterThan, OpGreaterThanOrEqual
    OpLessThan, OpLessThanOrEqual
    
    // 逻辑运算
    OpTrue, OpFalse
    OpBang
    
    // 变量操作
    OpGetGlobal, OpSetGlobal
    OpGetLocal, OpSetLocal
    OpGetFree, OpSetFree
    
    // 方法调用
    OpSend               // 发送消息/方法调用
    OpSendWithBlock
    OpDefineMethod       // 定义方法
    OpDefineClassMethod
    OpReturn, OpReturnValue
    
    // 类和模块
    OpOpenClass, OpOpenModule
    OpInstanceVarSet, OpInstanceVarGet
    OpClassVarSet, OpClassVarGet
    
    // 控制流
    OpJump, OpJumpTruthy, OpJumpNotTruthy
    OpJumpNil, OpJumpNotNil
    
    // 闭包
    OpClosure, OpCurrentClosure
    OpGetOuter, OpSetOuter
)
```

**实现任务**:
- [ ] 实现 AST 到字节码的转换
- [ ] 实现符号表管理
- [ ] 实现常量池管理
- [ ] 实现作用域管理
- [ ] 实现指令编码/解码

#### 1.5 虚拟机 (VM)

**文件**: `vm/vm.go`

**核心数据结构**:

```go
type VM struct {
    constants    []object.EmeraldValue  // 常量池
    globalVars   []object.EmeraldValue // 全局变量
    fiber        *Fiber                // 当前 Fiber
}

type Fiber struct {
    stack       []object.EmeraldValue  // 操作栈
    sp          int                    // 栈指针
    frames      []*Frame               // 调用帧
    fp          int                    // 帧指针
}

type Frame struct {
    fn           *object.EmeraldFunction  // 函数
    ip           int                      // 指令指针
    basePointer  int                      // 基指针
}
```

**实现任务**:
- [ ] 实现 Fetch-Decode-Execute 循环
- [ ] 实现栈操作
- [ ] 实现调用帧管理
- [ ] 实现方法调用
- [ ] 实现块和闭包
- [ ] 实现异常处理

---

### 阶段 2: 核心类库 (第 5-12 周)

#### 2.1 对象系统基础

**类层次结构**:

```
BasicObject
    │
    └─► Object
            │
            ├─► Module
            │       │
            │       └─► Class
            │
            ├─► Integer
            ├─► Float
            ├─► String
            ├─► Array
            ├─► Hash
            ├─► Symbol
            ├─► Regexp
            ├─► Range
            ├─► Proc
            ├─► Method
            ├─► IO
            ├─► File
            ├─► Thread
            ├─► Exception
            ├─► TrueClass
            ├─► FalseClass
            └─► NilClass
```

**核心接口** (vm/object/value.go):

```go
type EmeraldValue interface {
    Type() ValueType
    Inspect() string
    Class() *Class
    Methods() map[string]*Method
    InstanceVariableGet(name string) EmeraldValue
    InstanceVariableSet(name string, value EmeraldValue) EmeraldValue
    Equal(other EmeraldValue) bool
    HashKey() string
}
```

**实现任务**:
- [ ] 实现 BasicObject
- [ ] 实现 Object
- [ ] 实现 Module
- [ ] 实现 Class (包括单例类)
- [ ] 实现 Kernel
- [ ] 实现 TrueClass/FalseClass/NilClass

#### 2.2 数值类型

**实现任务**:
- [ ] 实现 Integer (整数)
- [ ] 实现 Float (浮点数)
- [ ] 实现 Numeric (数值基类)
- [ ] 实现 Complex (复数) - 可选
- [ ] 实现 Rational (有理数) - 可选

#### 2.3 字符串和符号

**实现任务**:
- [ ] 实现 String
  - [ ] 字符串拼接 `+`
  - [ ] 字符串插值 `#{}`
  - [ ] 正则表达式匹配 `=~`, `!~`
  - [ ] 字符编码支持
- [ ] 实现 Symbol
- [ ] 实现 Regexp

#### 2.4 容器类型

**实现任务**:
- [ ] 实现 Array
  - [ ] 索引访问 `[]`
  - [ ] 元素操作 `push`, `pop`, `shift`, `unshift`
  - [ ] 迭代 `each`, `map`, `select`, `reject`
  - [ ] 展开 `flatten`, `compact`
- [ ] 实现 Hash
  - [ ] 键值访问
  - [ ] 默认值
  - [ ] 迭代
- [ ] 实现 Range
- [ ] 实现 Enumerable

#### 2.5 其他核心类

**实现任务**:
- [ ] 实现 Proc/Lambda
- [ ] 实现 Method
- [ ] 实现 IO
- [ ] 实现 File
- [ ] 实现 Thread
- [ ] 实现 Exception (异常层次)

---

### 阶段 3: 标准库 (第 13-20 周)

#### 3.1 核心模块

**实现任务**:
- [ ] 实现 Kernel#require
- [ ] 实现 Kernel#load
- [ ] 实现 Kernel#eval
- [ ] 实现 Enumerable
- [ ] 实现 Comparable

#### 3.2 常用库

**实现任务**:
- [ ] 实现 Marshal (对象序列化)
- [ ] 实现 JSON
- [ ] 实现 Time
- [ ] 实现 Date/DateTime
- [ ] 实现 Pathname
- [ ] 实现 FileUtils

---

### 阶段 4: Ruby Spec 测试 (持续进行)

#### 4.1 测试框架搭建

**任务**:
- [ ] 克隆 ruby/spec 到 `spec/ruby`
- [ ] 创建测试运行器适配器
- [ ] 配置 MSpec 或自定义测试运行器

#### 4.2 测试执行顺序

**优先级** (从高到低):

1. **语言特性** (`language/`)
   - [ ] `language/literals/*` - 字面量
   - [ ] `language/assignment/*` - 赋值
   - [ ] `language/method/*` - 方法定义和调用
   - [ ] `language/class/*` - 类定义
   - [ ] `language/module/*` - 模块定义
   - [ ] `language/control_execution/*` - 控制流

2. **核心类** (`core/`)
   - [ ] `core/object/*` - 基础对象
   - [ ] `core/string/*` - 字符串
   - [ ] `core/array/*` - 数组
   - [ ] `core/hash/*` - 哈希
   - [ ] `core/integer/*` - 整数
   - [ ] `core/float/*` - 浮点数
   - [ ] `core/symbol/*` - 符号
   - [ ] `core/regexp/*` - 正则

3. **标准库** (`library/`)
   - [ ] `library/json/*` - JSON
   - [ ] `library/time/*` - 时间

---

### 阶段 5: Rails 兼容层 (第 21-36 周)

#### 5.1 核心组件

**实现任务**:

1. **ActionDispatch** - HTTP 处理
   - [ ] Request/Response 对象
   - [ ] Routing
   - [ ] Middleware 栈

2. **ActionController** - 控制器
   - [ ] Base 控制器
   - [ ] 参数解析
   - [ ] 会话管理
   - [ ] Cookies

3. **ActiveRecord** - ORM
   - [ ] 基础 Model
   - [ ] 关系映射 (has_many, belongs_to)
   - [ ] 查询构建器

4. **ActionView** - 视图
   - [ ] 模板引擎
   - [ ] ERB 支持
   - [ ] 布局和局部视图

#### 5.2 Rails 命令行工具

**实现任务**:
- [ ] `rails new` - 创建新应用
- [ ] `rails generate` - 代码生成器
- [ ] `rails server` - 启动服务器
- [ ] `rails console` - REPL

---

## 关键技术决策

### 1. 栈机器 vs 寄存器机器

**决策**: 采用栈机器 (Stack Machine)

**理由**:
- 实现简单
- 易于调试
- 兼容 Emerald 的设计

### 2. GC 策略

**决策**: 利用 Go 原生垃圾回收

**理由**:
- Go GC 成熟稳定
- 减少内存管理复杂度
- 专注于 Ruby 语义实现

### 3. 方法分发

**决策**: 双重方法表 (内置方法 + 用户方法)

**理由**:
- 性能优化
- 支持元编程
- 清晰的权限控制

### 4. 字符串实现

**决策**: 使用 Go 原生 strings 包

**理由**:
- Go 字符串处理高效
- UTF-8 支持完善
- 减少重复实现

---

## 里程碑

| 阶段 | 里程碑 | 预计时间 |
|------|--------|----------|
| 1 | 能运行 `puts "Hello World"` | 第 4 周 |
| 2 | 基本 Ruby 语法支持 | 第 12 周 |
| 3 | 核心类库完善 | 第 20 周 |
| 4 | 通过 ruby/spec 基础测试 | 第 24 周 |
| 5 | 运行简单 Rails 应用 | 第 36 周 |

---

## 参考资源

1. **Emerald** - https://github.com/mathiashsteffensen/emerald
2. **Ruby Spec Suite** - https://github.com/ruby/spec
3. **Pratt Parser** - https://en.wikipedia.org/wiki/Operator-precedence_parser#Pratt_parsing
4. **Ruby 语言规范** - https://ruby-spec.org

---

## 当前进度 (2026-02-24)

### 已完成

- [x] 项目目录结构搭建
- [x] 词法分析器 (Lexer) - `vm/lexer/`
  - Token 定义
  - 关键字识别
  - 数字、字符串、符号词法分析
  - 运算符识别
- [x] 解析器 (Parser) - `vm/parser/`
  - Pratt Parser 核心实现
  - AST 节点定义
  - 基本表达式解析
  - 语句解析
- [x] 字节码编译器 (Compiler) - `vm/compiler/`
  - Opcode 定义
  - 字节码生成
  - 符号表管理
- [x] CLI 入口 - `cmd/rgo/main.go`

### 已创建文件

```
rgo/
├── cmd/rgo/
│   └── main.go                 # CLI 入口
├── rvm/
│   ├── lexer/
│   │   ├── lexer.go           # 词法分析器
│   │   └── token.go          # Token 定义
│   ├── parser/
│   │   ├── parser.go          # Pratt Parser
│   │   └── ast/
│   │       └── node.go       # AST 节点
│   ├── compiler/
│   │   ├── compiler.go        # 字节码编译器
│   │   ├── opcode.go          # 操作码定义
│   │   └── bytecode.go      # 字节码工具
│   └── executor.go           # 虚拟机实现
├── docs/
│   └── IMPLEMENTATION_PLAN.md # 实现计划
```

### 待完成

- [x] 虚拟机 (VM) 实现
- [x] 基本内置方法 (部分)
- [ ] 完整对象系统
- [ ] Ruby 核心类库
- [ ] Ruby 标准库
- [ ] Rails 兼容层
- [ ] ruby/spec 测试集成

### 运行测试

```bash
# 编译
go build -o rgo ./cmd/rgo/

# 运行 Ruby 文件
./rgo test.rb

# 或使用 go run
go run cmd/rgo/main.go test.rb
```

### 验证示例

```ruby
# 输入
1 + 2
# 输出
3

# 输入
x = 5
x + 3
# 输出
8

# 输入
"hello" + " world"
# 输出
hello world

# 输入
10 - 5
# 输出
5

# 输入
5 > 3
# 输出
true
```

### Ruby Spec 测试

已克隆 ruby/spec 到 `spec/ruby` 目录，包含：

- `language/` - 语言特性测试 (数字、字符串、数组、类、方法等)
- `core/` - 核心类测试
- `library/` - 标准库测试

注意：ruby/spec 需要使用 MSpec 框架运行，需要原生 Ruby 环境。

### 已实现功能

- 整数运算: `+`, `-`, `*`, `/`, `%`, `**`
- 浮点数运算
- 字符串连接
- 变量赋值和引用
- 比较运算: `>`, `<`, `>=`, `<=`, `==`, `!=`
- 数组字面量 `[1, 2, 3]`
- 哈希字面量 `{a: 1, b: 2}`
- 内置方法:
  - Integer: `to_s`, `chr`, `odd?`, `even?`, `zero?`, `abs`
  - String: `length`, `size`, `empty?`, `to_s`, `upcase`, `downcase`
  - Array: `length`, `size`, `first`, `last`, `push`, `pop`, `empty?`, `join`, `reverse`

### 待完善

- 控制流 (if/unless/while/until)
- 方法定义 (def)
- 类定义 (class/module)
- 闭包和块
- 更多内置方法
- 对象系统 (一切皆对象)
