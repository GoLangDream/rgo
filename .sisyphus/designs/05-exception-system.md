# 阶段 5: Exception 系统技术设计

**优先级**: P0  
**预计时间**: 2 周  
**依赖**: 无  
**被依赖**: 阶段 9 (IO/File), 阶段 10 (Rails)

## 目标

实现完整的 Ruby 异常处理机制，包括异常类层次结构、begin/rescue/ensure/raise 语法、backtrace 支持，通过 vendor/ruby/spec/core/exception/ 下的 spec。

## 背景

Ruby 的异常系统是其错误处理的核心，Rails 大量使用异常来处理各种错误情况。没有异常系统，无法运行任何实际的 Ruby 代码。

### Ruby 异常层次结构

```
Exception
├── NoMemoryError
├── ScriptError
│   ├── LoadError
│   ├── NotImplementedError
│   └── SyntaxError
├── SecurityError
├── SignalException
│   └── Interrupt
├── StandardError (默认 rescue 捕获的基类)
│   ├── ArgumentError
│   │   └── UncaughtThrowError
│   ├── EncodingError
│   ├── FiberError
│   ├── IOError
│   │   └── EOFError
│   ├── IndexError
│   │   ├── KeyError
│   │   └── StopIteration
│   ├── LocalJumpError
│   ├── NameError
│   │   └── NoMethodError
│   ├── RangeError
│   │   └── FloatDomainError
│   ├── RegexpError
│   ├── RuntimeError (默认 raise 的类)
│   ├── SystemCallError
│   │   └── Errno::*
│   ├── ThreadError
│   ├── TypeError
│   └── ZeroDivisionError
├── SystemExit
├── SystemStackError
└── fatal (无法捕获)
```

## 当前状态

### 已实现
- 无异常系统
- VM 错误通过 Go error 返回
- 无法捕获和处理运行时错误

### 需要实现

#### P0 - 核心功能 (第 1 周)
**异常类层次**:
- Exception 基类
- StandardError 及常用子类
- RuntimeError, ArgumentError, TypeError
- NameError, NoMethodError
- IndexError, KeyError
- ZeroDivisionError

**语法支持**:
- `begin...rescue...end` - 异常捕获
- `begin...rescue...else...end` - 无异常时执行
- `begin...rescue...ensure...end` - 总是执行
- `raise` - 抛出异常
- `rescue` 修饰符 - `expr rescue default`

**异常对象**:
- `message` - 错误消息
- `backtrace` - 调用栈
- `cause` - 原因异常
- `exception(message)` - 创建异常

#### P1 - 高级功能 (第 2 周)
**多重 rescue**:
- 按类型匹配异常
- 多个 rescue 子句
- 捕获多个异常类型

**异常变量**:
- `rescue => e` - 捕获异常到变量
- `$!` - 当前异常
- `$@` - 当前 backtrace

**其他异常类**:
- ScriptError 系列
- SystemCallError 系列
- 自定义异常类

## 技术设计

### 1. 异常对象结构

#### 1.1 Exception 类定义

```go
// pkg/object/exception.go
package object

type ExceptionData struct {
    Class     *Class
    Message   string
    Backtrace []string
    Cause     *EmeraldValue  // 原因异常
}

func NewException(class *Class, message string) *EmeraldValue {
    return &EmeraldValue{
        Type: ValueException,
        Data: &ExceptionData{
            Class:     class,
            Message:   message,
            Backtrace: make([]string, 0),
            Cause:     nil,
        },
        Class: class,
    }
}

// 异常类型常量
const (
    ValueException ValueType = "EXCEPTION"
)
```

#### 1.2 异常类层次初始化

```go
// pkg/core/exception.go
package core

func InitExceptions() {
    // Exception 基类
    exceptionClass := &object.Class{
        Name:       "Exception",
        Superclass: R.Classes["Object"],
        Methods:    make(map[string]*object.Method),
    }
    
    // message 方法
    exceptionClass.DefineMethod("message", &object.Method{
        Name:  "message",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
            excData, ok := receiver.Data.(*object.ExceptionData)
            if !ok {
                return R.NilVal
            }
            
            return &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  excData.Message,
                Class: R.Classes["String"],
            }
        },
    })
    
    // backtrace 方法
    exceptionClass.DefineMethod("backtrace", &object.Method{
        Name:  "backtrace",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
            excData, ok := receiver.Data.(*object.ExceptionData)
            if !ok {
                return R.NilVal
            }
            
            result := make([]*object.EmeraldValue, len(excData.Backtrace))
            for i, line := range excData.Backtrace {
                result[i] = &object.EmeraldValue{
                    Type:  object.ValueString,
                    Data:  line,
                    Class: R.Classes["String"],
                }
            }
            
            return &object.EmeraldValue{
                Type:  object.ValueArray,
                Data:  result,
                Class: R.Classes["Array"],
            }
        },
    })
    
    // to_s 方法
    exceptionClass.DefineMethod("to_s", &object.Method{
        Name:  "to_s",
        Arity: 0,
        Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
            excData, ok := receiver.Data.(*object.ExceptionData)
            if !ok {
                return &object.EmeraldValue{
                    Type:  object.ValueString,
                    Data:  "",
                    Class: R.Classes["String"],
                }
            }
            
            return &object.EmeraldValue{
                Type:  object.ValueString,
                Data:  excData.Message,
                Class: R.Classes["String"],
            }
        },
    })
    
    R.Classes["Exception"] = exceptionClass
    
    // StandardError
    standardErrorClass := &object.Class{
        Name:       "StandardError",
        Superclass: exceptionClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["StandardError"] = standardErrorClass
    
    // RuntimeError
    runtimeErrorClass := &object.Class{
        Name:       "RuntimeError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["RuntimeError"] = runtimeErrorClass
    
    // ArgumentError
    argumentErrorClass := &object.Class{
        Name:       "ArgumentError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["ArgumentError"] = argumentErrorClass
    
    // TypeError
    typeErrorClass := &object.Class{
        Name:       "TypeError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["TypeError"] = typeErrorClass
    
    // NameError
    nameErrorClass := &object.Class{
        Name:       "NameError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["NameError"] = nameErrorClass
    
    // NoMethodError
    noMethodErrorClass := &object.Class{
        Name:       "NoMethodError",
        Superclass: nameErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["NoMethodError"] = noMethodErrorClass
    
    // IndexError
    indexErrorClass := &object.Class{
        Name:       "IndexError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["IndexError"] = indexErrorClass
    
    // KeyError
    keyErrorClass := &object.Class{
        Name:       "KeyError",
        Superclass: indexErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["KeyError"] = keyErrorClass
    
    // ZeroDivisionError
    zeroDivisionErrorClass := &object.Class{
        Name:       "ZeroDivisionError",
        Superclass: standardErrorClass,
        Methods:    make(map[string]*object.Method),
    }
    R.Classes["ZeroDivisionError"] = zeroDivisionErrorClass
}
```

### 2. AST 节点

#### 2.1 Begin/Rescue/Ensure 表达式

```go
// pkg/parser/ast/node.go
type BeginExpression struct {
    Token       token.Token
    Body        *BlockExpression
    RescueClauses []*RescueClause
    ElseClause  *BlockExpression
    EnsureClause *BlockExpression
}

type RescueClause struct {
    ExceptionTypes []Expression  // 捕获的异常类型
    Variable       *Identifier   // 异常变量 (rescue => e)
    Body           *BlockExpression
}

type RaiseExpression struct {
    Token     token.Token
    Exception Expression  // 异常类或异常对象
    Message   Expression  // 错误消息
}

type RescueModifier struct {
    Token   token.Token
    Expr    Expression  // 可能抛出异常的表达式
    Default Expression  // 异常时的默认值
}
```

#### 2.2 Parser 实现

```go
// pkg/parser/parser.go
func (p *Parser) parseBeginExpression() ast.Expression {
    expr := &ast.BeginExpression{
        Token:         p.curToken,
        RescueClauses: make([]*ast.RescueClause, 0),
    }
    
    p.nextToken()  // 跳过 BEGIN
    
    // 解析 body
    expr.Body = p.parseBlockExpression(token.RESCUE, token.ELSE, token.ENSURE, token.END)
    
    // 解析 rescue 子句
    for p.curTokenIs(token.RESCUE) {
        rescue := &ast.RescueClause{
            ExceptionTypes: make([]ast.Expression, 0),
        }
        
        p.nextToken()
        
        // 解析异常类型
        if !p.curTokenIs(token.ARROW) && !p.curTokenIs(token.NEWLINE) {
            rescue.ExceptionTypes = append(rescue.ExceptionTypes, p.parseExpression(LOWEST))
            
            // 多个异常类型: rescue TypeError, ArgumentError
            for p.peekTokenIs(token.COMMA) {
                p.nextToken()  // 跳过当前
                p.nextToken()  // 跳过 COMMA
                rescue.ExceptionTypes = append(rescue.ExceptionTypes, p.parseExpression(LOWEST))
            }
        }
        
        // 解析异常变量: rescue => e
        if p.peekTokenIs(token.ARROW) {
            p.nextToken()  // 跳过当前
            p.nextToken()  // 跳过 =>
            rescue.Variable = &ast.Identifier{
                Token: p.curToken,
                Value: p.curToken.Literal,
            }
        }
        
        p.skipCurNewlines()
        
        // 解析 rescue body
        rescue.Body = p.parseBlockExpression(token.RESCUE, token.ELSE, token.ENSURE, token.END)
        
        expr.RescueClauses = append(expr.RescueClauses, rescue)
    }
    
    // 解析 else 子句
    if p.curTokenIs(token.ELSE) {
        p.nextToken()
        expr.ElseClause = p.parseBlockExpression(token.ENSURE, token.END)
    }
    
    // 解析 ensure 子句
    if p.curTokenIs(token.ENSURE) {
        p.nextToken()
        expr.EnsureClause = p.parseBlockExpression(token.END)
    }
    
    if !p.expectPeek(token.END) {
        return nil
    }
    
    return expr
}

func (p *Parser) parseRaiseExpression() ast.Expression {
    expr := &ast.RaiseExpression{
        Token: p.curToken,
    }
    
    p.nextToken()
    
    if p.curTokenIs(token.NEWLINE) || p.curTokenIs(token.SEMICOLON) {
        // raise 无参数 - 重新抛出当前异常
        return expr
    }
    
    // 解析异常类或消息
    expr.Exception = p.parseExpression(LOWEST)
    
    // 解析消息 (如果有)
    if p.peekTokenIs(token.COMMA) {
        p.nextToken()  // 跳过当前
        p.nextToken()  // 跳过 COMMA
        expr.Message = p.parseExpression(LOWEST)
    }
    
    return expr
}
```

### 3. Compiler 实现

#### 3.1 新增 Opcode

```go
// pkg/compiler/opcode.go
const (
    // ... 现有 opcodes
    
    OpRaise         // 抛出异常
    OpSetupRescue   // 设置 rescue 处理器
    OpPopRescue     // 移除 rescue 处理器
    OpSetupEnsure   // 设置 ensure 处理器
    OpPopEnsure     // 移除 ensure 处理器
    OpGetException  // 获取当前异常
)
```

#### 3.2 Compiler 编译

```go
// pkg/compiler/compiler.go
func (c *Compiler) Compile(node ast.Node) error {
    switch node := node.(type) {
    // ... 其他 case
    
    case *ast.BeginExpression:
        return c.compileBeginExpression(node)
    
    case *ast.RaiseExpression:
        return c.compileRaiseExpression(node)
    
    case *ast.RescueModifier:
        return c.compileRescueModifier(node)
    }
    
    return nil
}

func (c *Compiler) compileBeginExpression(node *ast.BeginExpression) error {
    // 设置 rescue 处理器
    rescueJumps := make([]int, 0)
    
    if len(node.RescueClauses) > 0 {
        // OpSetupRescue <rescue_handler_pos>
        setupPos := c.emit(OpSetupRescue, 9999)  // 占位符
        
        // 编译 body
        if err := c.Compile(node.Body); err != nil {
            return err
        }
        
        // 如果没有异常，跳过所有 rescue 子句
        afterRescuePos := c.emit(OpJump, 9999)
        rescueJumps = append(rescueJumps, afterRescuePos)
        
        // 回填 rescue 处理器位置
        rescueHandlerPos := len(c.currentInstructions())
        c.changeOperand(setupPos, rescueHandlerPos)
        
        // 编译 rescue 子句
        for i, rescue := range node.RescueClauses {
            // 获取当前异常
            c.emit(OpGetException)
            
            // 检查异常类型
            if len(rescue.ExceptionTypes) > 0 {
                // TODO: 类型匹配逻辑
            }
            
            // 如果有异常变量，赋值
            if rescue.Variable != nil {
                c.emit(OpGetException)
                symbol := c.symbolTable.Define(rescue.Variable.Value)
                c.emit(OpSetLocal, symbol.Index)
            }
            
            // 编译 rescue body
            if err := c.Compile(rescue.Body); err != nil {
                return err
            }
            
            // 跳过后续 rescue 子句
            if i < len(node.RescueClauses)-1 {
                jumpPos := c.emit(OpJump, 9999)
                rescueJumps = append(rescueJumps, jumpPos)
            }
        }
        
        // 回填所有跳转
        afterPos := len(c.currentInstructions())
        for _, jumpPos := range rescueJumps {
            c.changeOperand(jumpPos, afterPos)
        }
        
        c.emit(OpPopRescue)
    } else {
        // 没有 rescue，直接编译 body
        if err := c.Compile(node.Body); err != nil {
            return err
        }
    }
    
    // 编译 else 子句
    if node.ElseClause != nil {
        if err := c.Compile(node.ElseClause); err != nil {
            return err
        }
    }
    
    // 编译 ensure 子句
    if node.EnsureClause != nil {
        c.emit(OpSetupEnsure)
        if err := c.Compile(node.EnsureClause); err != nil {
            return err
        }
        c.emit(OpPopEnsure)
    }
    
    return nil
}

func (c *Compiler) compileRaiseExpression(node *ast.RaiseExpression) error {
    if node.Exception == nil {
        // raise 无参数 - 重新抛出
        c.emit(OpRaise, 0)  // 0 表示重新抛出
        return nil
    }
    
    // 编译异常对象或消息
    if err := c.Compile(node.Exception); err != nil {
        return err
    }
    
    if node.Message != nil {
        if err := c.Compile(node.Message); err != nil {
            return err
        }
        c.emit(OpRaise, 2)  // 2 个参数
    } else {
        c.emit(OpRaise, 1)  // 1 个参数
    }
    
    return nil
}
```

### 4. VM 执行

#### 4.1 异常处理器栈

```go
// pkg/vm/executor.go
type RescueHandler struct {
    HandlerIP int  // rescue 处理器的指令位置
    StackSize int  // 设置时的栈大小
}

type VM struct {
    // ... 现有字段
    
    currentException *object.EmeraldValue
    rescueHandlers   []RescueHandler
    ensureHandlers   []int  // ensure 子句的指令位置
}
```

#### 4.2 Opcode 执行

```go
func (vm *VM) Run() error {
    for ip < len(ins) {
        op := code.Opcode(ins[ip])
        
        switch op {
        case code.OpRaise:
            numArgs := int(ins[ip+1])
            
            var exception *object.EmeraldValue
            
            if numArgs == 0 {
                // 重新抛出当前异常
                if vm.currentException == nil {
                    return fmt.Errorf("no exception to re-raise")
                }
                exception = vm.currentException
            } else if numArgs == 1 {
                // raise ExceptionClass 或 raise "message"
                arg := vm.pop()
                
                if arg.Type == object.ValueString {
                    // raise "message" - 创建 RuntimeError
                    message, _ := arg.Data.(string)
                    exception = object.NewException(
                        core.R.Classes["RuntimeError"],
                        message,
                    )
                } else {
                    // raise ExceptionClass - 创建异常实例
                    exception = object.NewException(
                        arg.Class,
                        "",
                    )
                }
            } else if numArgs == 2 {
                // raise ExceptionClass, "message"
                message := vm.pop()
                exceptionClass := vm.pop()
                
                messageStr, _ := message.Data.(string)
                exception = object.NewException(
                    exceptionClass.Class,
                    messageStr,
                )
            }
            
            // 添加 backtrace
            vm.addBacktrace(exception)
            
            // 查找 rescue 处理器
            if len(vm.rescueHandlers) > 0 {
                handler := vm.rescueHandlers[len(vm.rescueHandlers)-1]
                vm.rescueHandlers = vm.rescueHandlers[:len(vm.rescueHandlers)-1]
                
                // 恢复栈
                vm.sp = handler.StackSize
                
                // 设置当前异常
                vm.currentException = exception
                
                // 跳转到 rescue 处理器
                ip = handler.HandlerIP
                continue
            }
            
            // 没有处理器，返回错误
            return fmt.Errorf("uncaught exception: %s", exception.Inspect())
        
        case code.OpSetupRescue:
            handlerIP := int(code.ReadUint16(ins[ip+1:]))
            vm.rescueHandlers = append(vm.rescueHandlers, RescueHandler{
                HandlerIP: handlerIP,
                StackSize: vm.sp,
            })
            ip += 3
        
        case code.OpPopRescue:
            if len(vm.rescueHandlers) > 0 {
                vm.rescueHandlers = vm.rescueHandlers[:len(vm.rescueHandlers)-1]
            }
            ip++
        
        case code.OpGetException:
            if vm.currentException != nil {
                vm.push(vm.currentException)
            } else {
                vm.push(core.R.NilVal)
            }
            ip++
        
        case code.OpSetupEnsure:
            // TODO: 实现 ensure 处理
            ip++
        
        case code.OpPopEnsure:
            // TODO: 实现 ensure 清理
            ip++
        }
    }
    
    return nil
}

func (vm *VM) addBacktrace(exception *object.EmeraldValue) {
    excData, ok := exception.Data.(*object.ExceptionData)
    if !ok {
        return
    }
    
    // 添加当前调用栈
    for i := len(vm.frames) - 1; i >= 0; i-- {
        frame := vm.frames[i]
        fn := frame.fn
        line := fmt.Sprintf("%s:%d", fn.Name, frame.ip)
        excData.Backtrace = append(excData.Backtrace, line)
    }
}
```

### 5. Kernel#raise 方法

```go
// pkg/core/kernel.go
kernelModule.DefineMethod("raise", &object.Method{
    Name:  "raise",
    Arity: -1,
    Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
        var exception *object.EmeraldValue
        
        if len(args) == 0 {
            // raise - 重新抛出当前异常
            // 这应该由 VM 处理
            return R.NilVal
        } else if len(args) == 1 {
            arg := args[0]
            if arg.Type == object.ValueString {
                message, _ := arg.Data.(string)
                exception = object.NewException(
                    R.Classes["RuntimeError"],
                    message,
                )
            } else {
                exception = object.NewException(
                    arg.Class,
                    "",
                )
            }
        } else {
            exceptionClass := args[0]
            message, _ := args[1].Data.(string)
            exception = object.NewException(
                exceptionClass.Class,
                message,
            )
        }
        
        // 触发异常 (通过 VM)
        // TODO: 需要特殊处理
        return exception
    },
})
```

## 实施计划

### 第 1 周: 核心功能

**Day 1-2: 异常类层次**
- [ ] 实现 Exception 基类
- [ ] 实现 StandardError 及子类
- [ ] 实现异常对象方法 (message, backtrace)
- [ ] 测试: 异常类创建

**Day 3-4: AST 和 Parser**
- [ ] 实现 BeginExpression, RescueClause AST
- [ ] 实现 RaiseExpression AST
- [ ] 实现 Parser 解析逻辑
- [ ] 测试: 语法解析

**Day 5: Compiler**
- [ ] 实现新增 Opcode
- [ ] 实现 begin/rescue/ensure 编译
- [ ] 实现 raise 编译
- [ ] 测试: 编译输出

### 第 2 周: VM 和高级功能

**Day 6-7: VM 执行**
- [ ] 实现异常处理器栈
- [ ] 实现 OpRaise 执行
- [ ] 实现 OpSetupRescue/OpPopRescue
- [ ] 测试: 异常抛出和捕获

**Day 8: Backtrace 和调试**
- [ ] 实现 backtrace 生成
- [ ] 实现异常消息格式化
- [ ] 实现 $! 和 $@ 全局变量
- [ ] 测试: Backtrace

**Day 9: 多重 rescue 和类型匹配**
- [ ] 实现异常类型匹配
- [ ] 实现多个 rescue 子句
- [ ] 实现 rescue 修饰符
- [ ] 测试: 复杂 rescue

**Day 10: 验收和优化**
- [ ] 运行 exception/ 下的 spec
- [ ] 修复发现的问题
- [ ] 性能优化
- [ ] 文档更新

## 验收标准

### 功能验收
- [ ] 实现完整的异常类层次
- [ ] begin/rescue/ensure/raise 语法工作
- [ ] 异常类型匹配正确
- [ ] Backtrace 生成正确
- [ ] rescue 修饰符工作

### 测试验收
- [ ] 通过 vendor/ruby/spec/core/exception/ 下 80%+ spec
- [ ] 所有核心异常类可用
- [ ] 异常传播正确

### 性能验收
- [ ] 异常抛出开销可接受 (< 1ms)
- [ ] 无内存泄漏

## 风险和缓解

### 技术风险

**风险 1: VM 控制流复杂**
- 影响: 高
- 概率: 高
- 缓解: 参考其他 VM 实现，逐步测试

**风险 2: Backtrace 生成困难**
- 影响: 中
- 概率: 中
- 缓解: 先实现简单版本，后续优化

**风险 3: ensure 语义复杂**
- 影响: 中
- 概率: 中
- 缓解: 参考 Ruby spec，严格测试

---

**文档版本**: 1.0  
**创建时间**: 2026-03-16  
**状态**: 待审核
