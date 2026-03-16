# RGo 技术设计文档索引

本目录包含 RGo 项目的完整技术设计文档，涵盖从 Enumerable 模块到 Rails 兼容性的 10 个实施阶段。

## 文档列表

### 📋 总体规划
- **[00-master-plan.md](00-master-plan.md)** - 项目总体规划和路线图
  - 10 阶段实施路线图
  - 时间线估算（18-20 周）
  - 成功指标和风险管理

### 🔧 详细设计文档

#### 阶段 1-5: 核心功能 (P0, 10-12 周)

- **[01-enumerable-module.md](01-enumerable-module.md)** - Enumerable 模块 (2 周)
  - Module 系统增强
  - 20+ 迭代方法实现
  - Block/Closure 增强
  - VM Opcode 支持

- **[02-array-methods.md](02-array-methods.md)** - Array 方法补全 (2 周)
  - 60+ Array 方法
  - 修改、查询、迭代方法
  - 集合运算和排序
  - 切片和转换

- **[03-string-methods.md](03-string-methods.md)** - String 方法补全 (2 周)
  - 70+ String 方法
  - 修改方法 (!)
  - 查询、替换、分割
  - 转换和编码

- **[04-hash-methods.md](04-hash-methods.md)** - Hash 方法补全 (1 周)
  - 30+ Hash 方法
  - 修改和查询方法
  - 迭代和转换
  - dig 和 default 处理

- **[05-exception-system.md](05-exception-system.md)** - Exception 系统 (2 周)
  - 异常类层次结构
  - begin/rescue/ensure/raise 语法
  - Backtrace 支持
  - VM 异常处理

#### 阶段 6-10: 高级功能和 Rails 兼容 (P1-P3, 8+ 周)

- **[06-10-remaining-stages.md](06-10-remaining-stages.md)** - 剩余阶段概要
  - 阶段 6: Symbol & Range 运行时 (1 周)
  - 阶段 7: Regexp 引擎 (3 周)
  - 阶段 8: Module 系统 (2 周)
  - 阶段 9: IO & File (3 周)
  - 阶段 10: Rails 兼容性 (长期)

## 文档使用指南

### 阅读顺序

1. **首次阅读**: 从 `00-master-plan.md` 开始，了解整体规划
2. **实施前**: 阅读对应阶段的详细设计文档
3. **实施中**: 参考设计文档中的代码示例和实施计划
4. **验收时**: 使用文档中的验收标准

### 文档结构

每个详细设计文档包含：
- **目标**: 该阶段要达成的目标
- **当前状态**: 已实现和需要实现的功能
- **技术设计**: 详细的实现方案和代码示例
- **实施计划**: 按天分解的任务列表
- **验收标准**: 功能、测试、性能三个维度
- **风险和缓解**: 潜在问题和应对方案

### 代码示例

所有设计文档中的代码示例都是可执行的 Go 代码，可以直接参考实施。

## 实施状态

### ✅ 已完成
- [x] 总体规划文档
- [x] 阶段 1-5 详细设计
- [x] 阶段 6-10 概要设计

### ⏭️ 待完成
- [ ] 设计文档审核
- [ ] 阶段 1 实施
- [ ] 阶段 2-10 实施

## 时间线总览

```
核心功能 (阶段 1-6):  10-12 周
├─ Week 1-2:   Enumerable 模块
├─ Week 3-4:   Array 方法
├─ Week 5-6:   String 方法
├─ Week 7:     Hash 方法
├─ Week 8-9:   Exception 系统
└─ Week 10:    Symbol & Range

高级功能 (阶段 7-9):  8 周
├─ Week 11-13: Regexp 引擎
├─ Week 14-15: Module 系统
└─ Week 16-18: IO & File

Rails 兼容 (阶段 10):  长期
└─ Week 19+:   标准库、Gem、ActiveSupport、Rack
```

## 成功指标

### 阶段 1-6 完成后
- 通过 1000+ Ruby core specs
- Array/String/Hash 方法覆盖率 80%+
- 完整的异常处理
- Symbol 和 Range 可用

### 阶段 7-9 完成后
- 通过 1500+ Ruby core specs
- 正则表达式基础功能
- Module 混入工作
- 文件 IO 可用

### 阶段 10 完成后
- 运行简单的 Rails 应用
- 加载 ActiveSupport
- 处理 HTTP 请求
- 渲染 ERB 模板

## 贡献指南

### 添加新设计文档

1. 使用现有文档作为模板
2. 包含所有必需章节
3. 提供详细的代码示例
4. 更新本 README

### 更新现有文档

1. 保持文档版本号
2. 记录更新时间
3. 说明更新原因

## 参考资料

- Ruby 官方文档: https://ruby-doc.org/
- Ruby Spec: vendor/ruby/spec/
- MRI 源码: https://github.com/ruby/ruby
- RGo TODO: ../TODO.md

---

**最后更新**: 2026-03-16  
**文档总数**: 7 个  
**总页数**: 约 100 页
