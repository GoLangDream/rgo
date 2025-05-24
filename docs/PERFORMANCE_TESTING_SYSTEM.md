# RGO 性能测试系统

本文档介绍 RGO 项目的性能测试自动化系统，包括脚本、工具和使用方法。

## 系统概览

RGO 性能测试系统提供了完整的自动化性能测试解决方案，可以：

- 🚀 自动运行性能基准测试
- 📊 生成详细的性能分析报告
- 🔄 支持多种测试模式（快速、完整、详细）
- 🌐 跨平台支持（Windows、Linux、Mac）
- 📈 对比 RGO 与原生 Go 的性能差异

## 文件结构

```
rgo/
├── scripts/                    # 性能测试脚本
│   ├── benchmark.ps1          # Windows PowerShell 脚本
│   ├── benchmark.sh           # Linux/Mac Bash 脚本
│   ├── quick_benchmark.ps1    # Windows 快速演示脚本
│   └── quick_benchmark.bat    # Windows 批处理演示脚本
├── docs/                      # 文档目录
│   ├── PERFORMANCE_ANALYSIS.md    # 性能分析报告
│   ├── BENCHMARK_GUIDE.md         # 性能测试指南
│   └── PERFORMANCE_TESTING_SYSTEM.md  # 本文档
├── benchmark_test.go          # Go 基准测试文件
├── Makefile                   # 构建和测试自动化
└── README.md                  # 项目主文档
```

## 核心组件

### 1. 基准测试文件 (`benchmark_test.go`)

包含完整的性能测试套件：

- **RString 测试**: 字符串操作性能对比
- **RInteger 测试**: 整数操作性能对比
- **RHash 测试**: 哈希表操作性能对比
- **RClass 测试**: 类系统性能对比
- **内存分配测试**: 内存使用情况分析

### 2. 自动化脚本

#### Windows PowerShell (`scripts/benchmark.ps1`)
- 功能完整的性能测试自动化脚本
- 支持三种测试模式：快速、完整、详细
- 自动生成性能报告
- 彩色输出和进度显示

#### Linux/Mac Bash (`scripts/benchmark.sh`)
- 跨平台兼容的 shell 脚本
- 与 PowerShell 版本功能对等
- 自动检测系统信息
- 支持所有测试模式

#### 演示脚本
- `quick_benchmark.ps1`: PowerShell 快速演示
- `quick_benchmark.bat`: Windows 批处理演示

### 3. 构建自动化 (`Makefile`)

提供简化的命令接口：

```bash
make benchmark        # 完整性能测试
make benchmark-quick   # 快速性能测试
make benchmark-detail  # 详细性能测试
make help             # 显示帮助信息
```

## 使用方法

### 快速开始

```bash
# 使用 Makefile（推荐）
make benchmark

# 直接使用脚本
# Windows
.\scripts\benchmark.ps1

# Linux/Mac
./scripts/benchmark.sh
```

### 演示测试

```bash
# Windows PowerShell
.\scripts\quick_benchmark.ps1

# Windows 批处理
.\scripts\quick_benchmark.bat
```

### 手动测试

```bash
# 测试特定组件
go test -run=^$ -bench=BenchmarkRString -benchmem -count=1

# 运行所有测试
go test -run=^$ -bench=. -benchmem
```

## 测试模式

### 🎯 完整测试模式（默认）
- **用途**: 日常开发中的性能验证
- **时间**: 约 2-3 分钟
- **包含**: 所有主要组件的对比测试

### ⚡ 快速测试模式
- **用途**: 快速验证基本性能
- **时间**: 约 30 秒
- **包含**: 仅测试对象创建性能

### 🔬 详细测试模式
- **用途**: 深入的性能分析
- **时间**: 约 10-15 分钟
- **包含**: 所有测试用例的详细版本

## 性能指标

系统测量以下关键性能指标：

- **ns/op**: 每次操作的纳秒数（越小越好）
- **B/op**: 每次操作分配的字节数（越小越好）
- **allocs/op**: 每次操作的内存分配次数（越少越好）

## 报告生成

### 自动报告
运行测试后自动生成 `docs/PERFORMANCE_ANALYSIS.md`，包含：

- 测试环境信息
- 性能测试结果概览
- 详细的基准测试数据
- 性能分析和优化建议

## 性能等级分类

- 🟢 **性能相当** (< 10% 差异): 基本无性能损失
- 🟡 **轻微损失** (10-50% 差异): 可接受的性能开销
- 🟠 **中等损失** (50-500% 差异): 需要权衡的性能开销
- 🔴 **严重损失** (> 500% 差异): 建议避免在性能关键场景使用

## 最佳实践

1. **定期运行**: 每次重要更改后运行性能测试
2. **版本对比**: 保存每个版本的性能报告用于对比
3. **环境一致**: 在相同环境中运行测试确保结果可比较
4. **关注趋势**: 重点关注性能变化趋势
5. **实际场景**: 结合实际使用场景解读测试结果

---

更多信息请参考：
- [性能测试指南](BENCHMARK_GUIDE.md)
- [性能分析报告](PERFORMANCE_ANALYSIS.md)
- [项目主文档](../README.md)
