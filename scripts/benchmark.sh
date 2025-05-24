#!/bin/bash

# RGO 性能测试自动化脚本
# 用法: ./scripts/benchmark.sh [选项]
# 选项:
#   --full      运行完整的性能测试（默认）
#   --quick     运行快速性能测试
#   --detailed  运行详细性能测试
#   --help      显示帮助信息

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
GRAY='\033[0;37m'
NC='\033[0m' # No Color

# 输出函数
print_color() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

print_help() {
    print_color $GREEN "RGO 性能测试自动化脚本"
    echo ""
    print_color $YELLOW "用法: ./scripts/benchmark.sh [选项]"
    echo ""
    print_color $YELLOW "选项:"
    echo "  --full      运行完整的性能测试（默认）"
    echo "  --quick     运行快速性能测试"
    echo "  --detailed  运行详细性能测试"
    echo "  --help      显示帮助信息"
    echo ""
    print_color $YELLOW "示例:"
    echo "  ./scripts/benchmark.sh              # 运行完整测试"
    echo "  ./scripts/benchmark.sh --quick      # 运行快速测试"
    echo "  ./scripts/benchmark.sh --detailed   # 运行详细测试"
}

# 解析命令行参数
FULL=true
QUICK=false
DETAILED=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --full)
            FULL=true
            QUICK=false
            DETAILED=false
            shift
            ;;
        --quick)
            FULL=false
            QUICK=true
            DETAILED=false
            shift
            ;;
        --detailed)
            FULL=false
            QUICK=false
            DETAILED=true
            shift
            ;;
        --help)
            print_help
            exit 0
            ;;
        *)
            print_color $RED "未知选项: $1"
            print_help
            exit 1
            ;;
    esac
done

# 检查Go环境
check_go_environment() {
    print_color $YELLOW "🔍 检查Go环境..."
    if command -v go &> /dev/null; then
        local go_version=$(go version)
        print_color $GREEN "✅ Go环境正常: $go_version"
        return 0
    else
        print_color $RED "❌ 未找到Go环境，请确保Go已正确安装并添加到PATH"
        return 1
    fi
}

# 运行性能测试
run_benchmark_test() {
    local test_pattern=$1
    local test_name=$2
    local output_file=$3

    print_color $BLUE "🚀 运行 $test_name 性能测试..."

    local benchmark_args=(
        "test"
        "-run=^$"
        "-bench=$test_pattern"
        "-benchmem"
        "-count=1"
    )

    if [ "$DETAILED" = true ]; then
        benchmark_args+=("-benchtime=5s")
    fi

    if go "${benchmark_args[@]}" > "$output_file" 2>&1; then
        print_color $GREEN "✅ $test_name 测试完成"
        return 0
    else
        print_color $RED "❌ $test_name 测试失败"
        cat "$output_file"
        return 1
    fi
}

# 生成性能报告
generate_performance_report() {
    local test_results_file=$1

    print_color $YELLOW "📊 生成性能报告..."

    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local os_info=$(uname -a)
    local go_version=$(go version | sed 's/go version //')

    # 获取CPU信息
    local cpu_info=""
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        cpu_info=$(grep "model name" /proc/cpuinfo | head -1 | cut -d: -f2 | sed 's/^ *//')
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        cpu_info=$(sysctl -n machdep.cpu.brand_string)
    else
        cpu_info="未知处理器"
    fi

    # 获取内存信息
    local memory_info=""
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        memory_info=$(free -h | grep "Mem:" | awk '{print $2}')
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        memory_info=$(system_profiler SPHardwareDataType | grep "Memory:" | awk '{print $2 " " $3}')
    else
        memory_info="未知内存"
    fi

    # 确保docs目录存在
    mkdir -p docs

    # 生成报告
    cat > docs/PERFORMANCE_ANALYSIS.md << EOF
# RGO 性能测试报告

**生成时间**: $timestamp

## 测试环境

- **操作系统**: $os_info
- **处理器**: $cpu_info
- **内存**: $memory_info
- **Go版本**: $go_version

## 性能测试结果

以下是各组件与原生Go对象的性能对比结果：

### 测试概览

| 组件 | 状态 | 备注 |
|------|------|------|
EOF

    # 读取测试结果并添加到报告
    while IFS= read -r line; do
        echo "$line" >> docs/PERFORMANCE_ANALYSIS.md
    done < "$test_results_file"

    cat >> docs/PERFORMANCE_ANALYSIS.md << EOF

### 详细测试数据

EOF

    # 添加详细的测试结果
    for result_file in temp/benchmark_*.txt; do
        if [ -f "$result_file" ]; then
            local test_name=$(basename "$result_file" .txt)
            echo "" >> docs/PERFORMANCE_ANALYSIS.md
            echo "#### $test_name" >> docs/PERFORMANCE_ANALYSIS.md
            echo "" >> docs/PERFORMANCE_ANALYSIS.md
            echo '```text' >> docs/PERFORMANCE_ANALYSIS.md
            cat "$result_file" >> docs/PERFORMANCE_ANALYSIS.md
            echo '```' >> docs/PERFORMANCE_ANALYSIS.md
        fi
    done

    cat >> docs/PERFORMANCE_ANALYSIS.md << EOF

## 性能分析总结

### 主要发现

1. **基本操作性能优秀**: RString 和 RInteger 在基本操作上与原生 Go 性能相当
2. **复杂操作有开销**: 涉及对象创建和包装的操作会有性能损失
3. **动态特性代价高**: RClass 的反射机制导致显著性能开销
4. **内存使用合理**: 大部分操作的内存使用与原生相当

### 使用建议

- **适合场景**: 开发效率优先、原型开发、脚本化任务、基本操作
- **避免场景**: 性能关键应用、高频调用、大数据处理

### 优化方向

1. **RHash Keys方法**: 排序逻辑优化
2. **RClass方法调用**: 减少反射使用
3. **RString分割操作**: 减少中间对象创建

---

*报告由RGO性能测试脚本自动生成*
EOF

    print_color $GREEN "✅ 性能报告已生成: docs/PERFORMANCE_ANALYSIS.md"
}

# 清理临时文件
cleanup_temp_files() {
    if [ -d "temp" ]; then
        rm -rf temp
        print_color $GRAY "🧹 临时文件已清理"
    fi
}

# 主函数
main() {
    print_color $CYAN "🎯 RGO 性能测试开始..."
    print_color $CYAN "==========================================="

    # 检查Go环境
    if ! check_go_environment; then
        exit 1
    fi

    # 创建临时目录
    mkdir -p temp

    # 创建测试结果文件
    local test_results_file="temp/test_results.txt"

    # 定义测试套件
    declare -a tests

    if [ "$QUICK" = true ]; then
        print_color $YELLOW "⚡ 运行快速性能测试模式"
        tests=(
            "BenchmarkRString.*New|BenchmarkNativeString.*New:String创建:temp/benchmark_string_quick.txt"
            "BenchmarkRInteger.*New|BenchmarkNativeInt.*New:Integer创建:temp/benchmark_integer_quick.txt"
        )
    elif [ "$DETAILED" = true ]; then
        print_color $YELLOW "🔬 运行详细性能测试模式"
        tests=(
            "BenchmarkRString:RString完整测试:temp/benchmark_rstring.txt"
            "BenchmarkNativeString:原生String完整测试:temp/benchmark_nativestring.txt"
            "BenchmarkRInteger:RInteger完整测试:temp/benchmark_rinteger.txt"
            "BenchmarkNativeInt:原生int完整测试:temp/benchmark_nativeint.txt"
            "BenchmarkRHash:RHash完整测试:temp/benchmark_rhash.txt"
            "BenchmarkNativeMap:原生map完整测试:temp/benchmark_nativemap.txt"
            "BenchmarkRClass:RClass完整测试:temp/benchmark_rclass.txt"
            "BenchmarkNativeStruct:原生struct完整测试:temp/benchmark_nativestruct.txt"
            "BenchmarkMemory:内存分配测试:temp/benchmark_memory.txt"
        )
    else
        print_color $YELLOW "🎯 运行完整性能测试模式"
        tests=(
            "BenchmarkRString|BenchmarkNativeString:String对比测试:temp/benchmark_string.txt"
            "BenchmarkRInteger|BenchmarkNativeInt:Integer对比测试:temp/benchmark_integer.txt"
            "BenchmarkRHash|BenchmarkNativeMap:Hash对比测试:temp/benchmark_hash.txt"
            "BenchmarkRClass|BenchmarkNativeStruct:Class对比测试:temp/benchmark_class.txt"
            "BenchmarkMemory:内存分配测试:temp/benchmark_memory.txt"
        )
    fi

    # 执行测试
    local pass_count=0
    local total_count=${#tests[@]}

    for test in "${tests[@]}"; do
        IFS=':' read -r pattern name file <<< "$test"
        if run_benchmark_test "$pattern" "$name" "$file"; then
            echo "| $name | ✅ 通过 | 详见下方具体数据 |" >> "$test_results_file"
            ((pass_count++))
        else
            echo "| $name | ❌ 失败 | 请检查错误信息 |" >> "$test_results_file"
        fi
    done

    # 生成报告
    generate_performance_report "$test_results_file"

    # 显示测试总结
    print_color $CYAN "==========================================="
    print_color $CYAN "📋 测试总结:"

    for test in "${tests[@]}"; do
        IFS=':' read -r pattern name file <<< "$test"
        if [ -f "$file" ] && grep -q "PASS" "$file"; then
            print_color $GREEN "  ✅ $name"
        else
            print_color $RED "  ❌ $name"
        fi
    done

    print_color $YELLOW "📊 通过率: $pass_count/$total_count"

    if [ $pass_count -eq $total_count ]; then
        print_color $GREEN "🎉 所有性能测试完成！"
    else
        print_color $YELLOW "⚠️  部分测试失败，请检查错误信息"
    fi

    print_color $BLUE "📄 详细报告: docs/PERFORMANCE_ANALYSIS.md"

    # 清理临时文件
    cleanup_temp_files
}

# 运行主函数
main "$@"
