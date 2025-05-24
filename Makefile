# RGO Makefile
# 用于简化常用操作

.PHONY: help test benchmark benchmark-quick benchmark-detailed clean docs build install

# 默认目标
help:
	@echo "RGO 项目 Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  test             运行所有单元测试"
	@echo "  benchmark        运行完整性能测试"
	@echo "  benchmark-quick  运行快速性能测试"
	@echo "  benchmark-detail 运行详细性能测试"
	@echo "  docs             生成文档"
	@echo "  clean            清理临时文件"
	@echo "  build            构建项目"
	@echo "  install          安装依赖"
	@echo "  help             显示此帮助信息"

# 运行单元测试
test:
	@echo "🧪 运行单元测试..."
	go test -v ./...

# 运行性能测试
benchmark:
	@echo "🚀 运行完整性能测试..."
ifeq ($(OS),Windows_NT)
	powershell -ExecutionPolicy Bypass -File scripts/benchmark.ps1 -Full
else
	./scripts/benchmark.sh --full
endif

# 快速性能测试
benchmark-quick:
	@echo "⚡ 运行快速性能测试..."
ifeq ($(OS),Windows_NT)
	powershell -ExecutionPolicy Bypass -File scripts/benchmark.ps1 -Quick
else
	./scripts/benchmark.sh --quick
endif

# 详细性能测试
benchmark-detail:
	@echo "🔬 运行详细性能测试..."
ifeq ($(OS),Windows_NT)
	powershell -ExecutionPolicy Bypass -File scripts/benchmark.ps1 -Detailed
else
	./scripts/benchmark.sh --detailed
endif

# 生成文档
docs:
	@echo "📚 生成文档..."
	@echo "文档已生成在 docs/ 目录下"

# 清理临时文件
clean:
	@echo "🧹 清理临时文件..."
	@if exist temp rmdir /s /q temp 2>nul || true
	@rm -rf temp 2>/dev/null || true

# 构建项目
build:
	@echo "🔨 构建项目..."
	go build ./...

# 安装依赖
install:
	@echo "📦 安装依赖..."
	go mod download
	go mod tidy

# 格式化代码
fmt:
	@echo "✨ 格式化代码..."
	go fmt ./...

# 检查代码
lint:
	@echo "🔍 检查代码..."
	go vet ./...

# 运行所有检查
check: fmt lint test
	@echo "✅ 所有检查完成"

# 完整的CI/CD流水线
ci: install check benchmark
	@echo "🎉 CI/CD流水线完成"
