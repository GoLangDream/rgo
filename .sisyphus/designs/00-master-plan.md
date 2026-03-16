# RGo 实施总体规划

**创建时间**: 2026-03-16  
**目标**: 完成所有 Ruby spec，使 rgo 能像 ruby 命令执行 Rails

## 项目现状总结

### 架构完整度 ✅
- Lexer: 140+ tokens
- Parser: 40+ AST nodes
- Compiler: 80+ opcodes
- VM: Stack-based execution
- 247 个 Go 单元测试全部通过

### Ruby Spec 覆盖
- 总计 59 个核心类
- 515+ spec 文件
- 数千个测试用例

### 实现缺口
- 核心类方法覆盖率: 20-30%
- Enumerable 模块: 未实现
- Exception 系统: 未实现
- Symbol/Range 运行时: 未实现
- Regexp 引擎: 未实现
- Module 系统: 部分实现
- IO/File: 未实现

## 10 阶段实施路线图

### 阶段 1: Enumerable 模块 (P0 - 2周)
**文档**: `01-enumerable-module.md`

**目标**: 实现所有集合类的基础迭代方法

**关键交付物**:
- Enumerable 模块基础设施
- 20+ 核心迭代方法
- Array/Hash 混入 Enumerable
- 通过 enumerable/ 下的 spec

**依赖**: 无
**被依赖**: 阶段 2, 3, 4

---

### 阶段 2: Array 核心方法补全 (P0 - 2周)
**文档**: `02-array-methods.md`

**目标**: Array 方法覆盖率达到 80%+

**关键交付物**:
- 60+ Array 方法实现
- 修改方法 (compact, flatten, uniq)
- 集合运算 (&, |, -, +)
- 切片和转换方法
- 通过 array/ 下 80%+ spec

**依赖**: 阶段 1 (Enumerable)
**被依赖**: 阶段 10 (Rails)

---

### 阶段 3: String 核心方法补全 (P0 - 2周)
**文档**: `03-string-methods.md`

**目标**: String 方法覆盖率达到 80%+

**关键交付物**:
- 70+ String 方法实现
- 修改方法 (upcase!, strip!, chomp)
- 查询和替换 (index, sub, gsub)
- 分割和格式化
- 通过 string/ 下 80%+ spec

**依赖**: 阶段 1 (Enumerable)
**被依赖**: 阶段 7 (Regexp), 阶段 10 (Rails)

---

### 阶段 4: Hash 核心方法补全 (P0 - 1周)
**文档**: `04-hash-methods.md`

**目标**: Hash 方法覆盖率达到 80%+

**关键交付物**:
- 30+ Hash 方法实现
- 修改和查询方法
- 迭代和转换
- 默认值处理
- 通过 hash/ 下 80%+ spec

**依赖**: 阶段 1 (Enumerable)
**被依赖**: 阶段 10 (Rails)

---

### 阶段 5: Exception 系统 (P0 - 2周)
**文档**: `05-exception-system.md`

**目标**: 完整的异常处理机制

**关键交付物**:
- Exception 类层次结构
- begin/rescue/ensure/raise 语法
- rescue 修饰符
- 异常类型匹配
- backtrace 支持
- 通过 exception/ 下的 spec

**依赖**: 无
**被依赖**: 阶段 10 (Rails)

---

### 阶段 6: Symbol & Range 运行时 (P1 - 1周)
**文档**: `06-symbol-range.md`

**目标**: 完整的 Symbol 和 Range 对象

**关键交付物**:
- Symbol 运行时和方法
- Range 运行时和迭代
- Symbol/Range 在表达式中的使用
- 通过 symbol/ 和 range/ 下的 spec

**依赖**: 阶段 1 (Enumerable for Range)
**被依赖**: 阶段 10 (Rails)

---

### 阶段 7: Regexp 引擎 (P1 - 3周)
**文档**: `07-regexp-engine.md`

**目标**: 基础正则表达式支持

**关键交付物**:
- 正则表达式字面量
- Go regexp 包集成
- =~, match, match? 方法
- MatchData 对象
- String 正则方法 (scan, gsub)
- 通过 regexp/ 下 60%+ spec

**依赖**: 阶段 3 (String methods)
**被依赖**: 阶段 10 (Rails routing)

---

### 阶段 8: Module 系统 (P1 - 2周)
**文档**: `08-module-system.md`

**目标**: 模块混入和继承

**关键交付物**:
- module 定义完善
- include/extend/prepend
- 方法查找链 (MRO)
- module_function
- 通过 module/ 下的 spec

**依赖**: 无
**被依赖**: 阶段 10 (Rails)

---

### 阶段 9: IO & File (P2 - 3周)
**文档**: `09-io-file.md`

**目标**: 基础文件操作

**关键交付物**:
- File 类 (open, read, write, close)
- IO 类基础
- Dir 类 (entries, glob)
- require/require_relative 完整实现
- 通过 file/ 和 io/ 下 60%+ spec

**依赖**: 阶段 5 (Exception for error handling)
**被依赖**: 阶段 10 (Rails)

---

### 阶段 10: Rails 兼容性 (P3 - 长期迭代)
**文档**: `10-rails-compatibility.md`

**目标**: 能够运行简单的 Rails 应用

**关键交付物**:
- 标准库 (json, erb, net/http)
- Gem 加载机制
- ActiveSupport 核心扩展
- Rack 兼容层
- 运行 Rails "Hello World"

**依赖**: 阶段 1-9 全部
**被依赖**: 无

---

## 时间线估算

### 核心功能 (阶段 1-6): 10-12 周
- 周 1-2: Enumerable 模块
- 周 3-4: Array 方法补全
- 周 5-6: String 方法补全
- 周 7: Hash 方法补全
- 周 8-9: Exception 系统
- 周 10: Symbol & Range

### 高级功能 (阶段 7-9): 8 周
- 周 11-13: Regexp 引擎
- 周 14-15: Module 系统
- 周 16-18: IO & File

### Rails 兼容 (阶段 10): 长期
- 周 19+: 标准库和 Rails 兼容层

**总计**: 约 18-20 周达到 Rails 基础兼容

## 成功指标

### 阶段 1-6 完成后
- [ ] 通过 1000+ Ruby core specs
- [ ] Array/String/Hash 方法覆盖率 80%+
- [ ] 完整的异常处理
- [ ] Symbol 和 Range 可用

### 阶段 7-9 完成后
- [ ] 通过 1500+ Ruby core specs
- [ ] 正则表达式基础功能
- [ ] Module 混入工作
- [ ] 文件 IO 可用

### 阶段 10 完成后
- [ ] 运行简单的 Rails 应用
- [ ] 加载 ActiveSupport
- [ ] 处理 HTTP 请求
- [ ] 渲染 ERB 模板

## 风险和缓解

### 技术风险
1. **Go 和 Ruby 语义差异**
   - 缓解: 严格遵循 Ruby spec 行为
   - 使用 Ruby 官方 spec 作为验收标准

2. **性能问题**
   - 缓解: 先保证正确性，后优化性能
   - 使用 Go 的并发特性优化

3. **内存管理**
   - 缓解: 利用 Go GC
   - 注意循环引用

### 项目风险
1. **范围蔓延**
   - 缓解: 严格按阶段执行
   - 每个阶段有明确的验收标准

2. **依赖阻塞**
   - 缓解: 识别关键路径
   - 优先实现被依赖的模块

## 下一步行动

1. ✅ 创建总体规划文档 (本文档)
2. ⏭️ 创建阶段 1 详细设计: Enumerable 模块
3. ⏭️ 创建阶段 2 详细设计: Array 方法
4. ⏭️ 创建阶段 3 详细设计: String 方法
5. ⏭️ 创建阶段 4 详细设计: Hash 方法
6. ⏭️ 创建阶段 5 详细设计: Exception 系统

---

**文档版本**: 1.0  
**最后更新**: 2026-03-16
