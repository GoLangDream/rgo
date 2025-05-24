# RGO 性能测试指南

本指南介绍如何使用 RGO 项目的性能测试工具来评估库的性能表现。

## 快速开始

### 使用 Makefile（推荐）

```bash
# 运行完整性能测试
make benchmark

# 运行快速性能测试（仅测试基本操作）
make benchmark-quick

# 运行详细性能测试（包含所有测试用例，运行时间更长）
make benchmark-detail

# 查看所有可用命令
make help
```

### 直接使用脚本

#### Windows (PowerShell)

```powershell
# 完整测试
.\scripts\benchmark.ps1

# 快速测试
.\scripts\benchmark.ps1 -Quick

# 详细测试
.\scripts\benchmark.ps1 -Detailed

# 查看帮助
.\scripts\benchmark.ps1 -Help
```

#### Linux/Mac (Bash)

```bash
# 完整测试
./scripts/benchmark.sh

# 快速测试
./scripts/benchmark.sh --quick

# 详细测试
./scripts/benchmark.sh --detailed

# 查看帮助
./scripts/benchmark.sh --help
```

## 测试模式说明

### 🎯 完整测试模式（默认）
- **用途**: 日常开发中的性能验证
- **包含**: 所有主要组件的对比测试
- **时间**: 约 2-3 分钟
- **测试项**:
  - RString vs 原生 String
  - RInteger vs 原生 int
  - RHash vs 原生 map
  - RClass vs 原生 struct
  - 内存分配对比

### ⚡ 快速测试模式
- **用途**: 快速验证基本性能
- **包含**: 仅测试对象创建性能
- **时间**: 约 30 秒
- **测试项**:
  - String 创建测试
  - Integer 创建测试

### 🔬 详细测试模式
- **用途**: 深入的性能分析
- **包含**: 所有测试用例的详细版本
- **时间**: 约 10-15 分钟
- **测试项**:
  - 所有组件的完整测试套件
  - 更长的运行时间（5秒/测试）
  - 更准确的性能数据

## 测试报告

### 报告位置
所有测试完成后，会在 `docs/PERFORMANCE_ANALYSIS.md` 生成详细的性能报告。

### 报告内容
- **测试环境信息**: 操作系统、CPU、内存、Go版本
- **测试概览**: 各组件测试状态一览表
- **详细数据**: 具体的基准测试结果
- **性能分析**: 性能差异总结和优化建议

### 报告解读

#### 性能指标说明
- **ns/op**: 每次操作的纳秒数（越小越好）
- **B/op**: 每次操作分配的字节数（越小越好）
- **allocs/op**: 每次操作的内存分配次数（越少越好）

#### 性能等级分类
- **🟢 性能相当** (差异 < 10%): 基本无性能损失
- **🟡 轻微损失** (差异 10-50%): 可接受的性能开销
- **🟠 中等损失** (差异 50-500%): 需要权衡的性能开销
- **🔴 严重损失** (差异 > 500%): 建议避免在性能关键场景使用

## 手动测试

### 运行单个测试

```bash
# 测试 RString 性能
go test -run=^$ -bench=BenchmarkRString -benchmem -count=1

# 测试 RInteger 性能
go test -run=^$ -bench=BenchmarkRInteger -benchmem -count=1

# 测试 RHash 性能
go test -run=^$ -bench=BenchmarkRHash -benchmem -count=1

# 测试 RClass 性能
go test -run=^$ -bench=BenchmarkRClass -benchmem -count=1
```

### 自定义测试参数

```bash
# 运行更长时间的测试
go test -run=^$ -bench=. -benchmem -benchtime=10s

# 运行多次测试取平均值
go test -run=^$ -bench=. -benchmem -count=5

# 生成 CPU 性能分析文件
go test -run=^$ -bench=. -benchmem -cpuprofile=cpu.prof

# 生成内存性能分析文件
go test -run=^$ -bench=. -benchmem -memprofile=mem.prof
```

## 性能优化工作流

### 1. 建立基准
```bash
# 运行完整测试建立基准
make benchmark
# 保存报告: cp docs/PERFORMANCE_ANALYSIS.md docs/baseline_performance.md
```

### 2. 进行优化
在代码中实施性能优化...

### 3. 验证效果
```bash
# 重新运行测试
make benchmark
# 对比新旧报告
```

### 4. 持续监控
```bash
# 在 CI/CD 中运行快速测试
make benchmark-quick
```

## 常见问题

### Q: 测试结果不稳定怎么办？
A:
- 关闭其他占用CPU的程序
- 使用 `-count=5` 运行多次取平均值
- 使用详细测试模式获得更准确的结果

### Q: 如何对比不同版本的性能？
A:
- 在每个版本运行测试后保存报告
- 使用 git 管理不同版本的性能报告
- 可以使用 `benchcmp` 工具对比结果

### Q: 测试失败怎么办？
A:
- 检查 Go 环境是否正确安装
- 确保所有依赖已安装 (`go mod download`)
- 检查是否有语法错误 (`go build ./...`)

### Q: 如何在 CI/CD 中集成性能测试？
A:
```yaml
# GitHub Actions 示例
- name: 运行性能测试
  run: make benchmark-quick

- name: 上传性能报告
  uses: actions/upload-artifact@v2
  with:
    name: performance-report
    path: docs/PERFORMANCE_ANALYSIS.md
```

## 性能测试最佳实践

1. **定期运行**: 建议每次重要更改后都运行性能测试
2. **版本对比**: 保存每个版本的性能报告用于对比
3. **环境一致**: 在相同的环境中运行测试以确保结果可比较
4. **关注趋势**: 重点关注性能变化趋势，而不是单次测试结果
5. **实际场景**: 结合实际使用场景解读测试结果

## 贡献性能改进

如果你发现了性能问题或有优化建议：

1. 运行性能测试确认问题
2. 实施优化方案
3. 运行测试验证改进效果
4. 提交 PR 时包含性能测试报告
5. 在 PR 描述中说明性能改进情况

---

更多信息请参考：
- [性能分析报告](PERFORMANCE_ANALYSIS.md)
- [项目文档](../README.md)
