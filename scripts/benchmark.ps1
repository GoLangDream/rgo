# RGO 性能测试自动化脚本
# 用法: .\scripts\benchmark.ps1 [选项]
# 选项:
#   -Full      运行完整的性能测试（默认）
#   -Quick     运行快速性能测试
#   -Detailed  运行详细性能测试
#   -Help      显示帮助信息

param(
    [switch]$Full,
    [switch]$Quick,
    [switch]$Detailed,
    [switch]$Help
)

# 显示帮助信息
if ($Help) {
    Write-Host "RGO 性能测试自动化脚本" -ForegroundColor Green
    Write-Host ""
    Write-Host "用法: .\scripts\benchmark.ps1 [选项]" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "选项:" -ForegroundColor Yellow
    Write-Host "  -Full      运行完整的性能测试（默认）"
    Write-Host "  -Quick     运行快速性能测试"
    Write-Host "  -Detailed  运行详细性能测试"
    Write-Host "  -Help      显示帮助信息"
    Write-Host ""
    Write-Host "示例:" -ForegroundColor Yellow
    Write-Host "  .\scripts\benchmark.ps1              # 运行完整测试"
    Write-Host "  .\scripts\benchmark.ps1 -Quick       # 运行快速测试"
    Write-Host "  .\scripts\benchmark.ps1 -Detailed    # 运行详细测试"
    exit 0
}

# 设置默认选项
if (-not $Quick -and -not $Detailed) {
    $Full = $true
}

# 颜色函数
function Write-ColorOutput {
    param([string]$Message, [string]$Color = "White")
    Write-Host $Message -ForegroundColor $Color
}

# 检查Go环境
function Test-GoEnvironment {
    Write-ColorOutput "检查Go环境..." "Yellow"
    try {
        $goVersion = go version
        Write-ColorOutput "Go环境正常: $goVersion" "Green"
        return $true
    }
    catch {
        Write-ColorOutput "未找到Go环境，请确保Go已正确安装并添加到PATH" "Red"
        return $false
    }
}

# 运行性能测试
function Invoke-BenchmarkTest {
    param(
        [string]$TestPattern,
        [string]$TestName,
        [string]$OutputFile
    )

    Write-ColorOutput "运行 $TestName 性能测试..." "Blue"

    $benchmarkArgs = @(
        "test"
        "-run=^$"
        "-bench=$TestPattern"
        "-benchmem"
        "-count=1"
    )

    if ($Detailed) {
        $benchmarkArgs += "-benchtime=5s"
    }

    try {
        $result = & go $benchmarkArgs 2>&1
        $result | Out-File -FilePath $OutputFile -Encoding UTF8

        if ($LASTEXITCODE -eq 0) {
            Write-ColorOutput "$TestName 测试完成" "Green"
            return $true
        } else {
            Write-ColorOutput "$TestName 测试失败" "Red"
            Write-ColorOutput "错误输出: $result" "Red"
            return $false
        }
    }
    catch {
        Write-ColorOutput "运行 $TestName 测试时发生错误: $_" "Red"
        return $false
    }
}

# 生成性能报告
function New-PerformanceReport {
    param([hashtable]$TestResults)

    Write-ColorOutput "生成性能报告..." "Yellow"

    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $systemInfo = @{
        OS = [System.Environment]::OSVersion.ToString()
        Processor = try { (Get-WmiObject -Class Win32_Processor | Select-Object -First 1).Name } catch { "未知处理器" }
        Memory = try { [math]::Round((Get-WmiObject -Class Win32_ComputerSystem).TotalPhysicalMemory / 1GB, 2) } catch { 0 }
        GoVersion = try { (go version) -replace "go version ", "" } catch { "未知版本" }
    }

    $reportContent = @"
# RGO 性能测试报告

**生成时间**: $timestamp

## 测试环境

- **操作系统**: $($systemInfo.OS)
- **处理器**: $($systemInfo.Processor)
- **内存**: $($systemInfo.Memory) GB
- **Go版本**: $($systemInfo.GoVersion)

## 性能测试结果

以下是各组件与原生Go对象的性能对比结果：

### 测试概览

| 组件 | 状态 | 备注 |
|------|------|------|
"@

    foreach ($test in $TestResults.Keys) {
        $status = if ($TestResults[$test]) { "通过" } else { "失败" }
        $reportContent += "| $test | $status | 详见下方具体数据 |`n"
    }

    $reportContent += @"

### 详细测试数据

"@

    # 添加详细的测试结果
    $testFiles = @(
        @{File = "temp/benchmark_string.txt"; Title = "String对比测试"}
        @{File = "temp/benchmark_integer.txt"; Title = "Integer对比测试"}
        @{File = "temp/benchmark_hash.txt"; Title = "Hash对比测试"}
        @{File = "temp/benchmark_class.txt"; Title = "Class对比测试"}
        @{File = "temp/benchmark_memory.txt"; Title = "内存分配测试"}
    )

    foreach ($testFile in $testFiles) {
        if (Test-Path $testFile.File) {
            $reportContent += "`n#### $($testFile.Title)`n`n"
            $reportContent += "``````text`n"
            $reportContent += Get-Content $testFile.File -Raw -ErrorAction SilentlyContinue
            $reportContent += "`n``````n"
        }
    }

    $reportContent += @"

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
"@

    # 确保docs目录存在
    if (-not (Test-Path "docs")) {
        New-Item -ItemType Directory -Path "docs" | Out-Null
    }

    # 写入报告文件
    $reportContent | Out-File -FilePath "docs/PERFORMANCE_ANALYSIS.md" -Encoding UTF8
    Write-ColorOutput "性能报告已生成: docs/PERFORMANCE_ANALYSIS.md" "Green"
}

# 清理临时文件
function Clear-TempFiles {
    if (Test-Path "temp") {
        Remove-Item -Path "temp" -Recurse -Force
        Write-ColorOutput "临时文件已清理" "Gray"
    }
}

# 主函数
function Main {
    Write-ColorOutput "RGO 性能测试开始..." "Cyan"
    Write-ColorOutput "===========================================" "Cyan"

    # 检查Go环境
    if (-not (Test-GoEnvironment)) {
        exit 1
    }

    # 创建临时目录
    if (-not (Test-Path "temp")) {
        New-Item -ItemType Directory -Path "temp" | Out-Null
    }

    # 性能测试结果
    $testResults = @{}

    # 运行不同的测试套件
    if ($Quick) {
        Write-ColorOutput "运行快速性能测试模式" "Yellow"
        $tests = @(
            @{Pattern = "BenchmarkRString.*New|BenchmarkNativeString.*New"; Name = "String创建"; File = "temp/benchmark_string_quick.txt"}
            @{Pattern = "BenchmarkRInteger.*New|BenchmarkNativeInt.*New"; Name = "Integer创建"; File = "temp/benchmark_integer_quick.txt"}
        )
    }
    elseif ($Detailed) {
        Write-ColorOutput "运行详细性能测试模式" "Yellow"
        $tests = @(
            @{Pattern = "BenchmarkRString"; Name = "RString完整测试"; File = "temp/benchmark_rstring.txt"}
            @{Pattern = "BenchmarkNativeString"; Name = "原生String完整测试"; File = "temp/benchmark_nativestring.txt"}
            @{Pattern = "BenchmarkRInteger"; Name = "RInteger完整测试"; File = "temp/benchmark_rinteger.txt"}
            @{Pattern = "BenchmarkNativeInt"; Name = "原生int完整测试"; File = "temp/benchmark_nativeint.txt"}
            @{Pattern = "BenchmarkRHash"; Name = "RHash完整测试"; File = "temp/benchmark_rhash.txt"}
            @{Pattern = "BenchmarkNativeMap"; Name = "原生map完整测试"; File = "temp/benchmark_nativemap.txt"}
            @{Pattern = "BenchmarkRClass"; Name = "RClass完整测试"; File = "temp/benchmark_rclass.txt"}
            @{Pattern = "BenchmarkNativeStruct"; Name = "原生struct完整测试"; File = "temp/benchmark_nativestruct.txt"}
            @{Pattern = "BenchmarkMemory"; Name = "内存分配测试"; File = "temp/benchmark_memory.txt"}
        )
    }
    else {
        Write-ColorOutput "运行完整性能测试模式" "Yellow"
        $tests = @(
            @{Pattern = "BenchmarkRString|BenchmarkNativeString"; Name = "String对比测试"; File = "temp/benchmark_string.txt"}
            @{Pattern = "BenchmarkRInteger|BenchmarkNativeInt"; Name = "Integer对比测试"; File = "temp/benchmark_integer.txt"}
            @{Pattern = "BenchmarkRHash|BenchmarkNativeMap"; Name = "Hash对比测试"; File = "temp/benchmark_hash.txt"}
            @{Pattern = "BenchmarkRClass|BenchmarkNativeStruct"; Name = "Class对比测试"; File = "temp/benchmark_class.txt"}
            @{Pattern = "BenchmarkMemory"; Name = "内存分配测试"; File = "temp/benchmark_memory.txt"}
        )
    }

    # 执行测试
    foreach ($test in $tests) {
        $testResults[$test.Name] = Invoke-BenchmarkTest -TestPattern $test.Pattern -TestName $test.Name -OutputFile $test.File
    }

    # 生成报告
    New-PerformanceReport -TestResults $testResults

    # 显示测试总结
    Write-ColorOutput "===========================================" "Cyan"
    Write-ColorOutput "测试总结:" "Cyan"

    $passCount = ($testResults.Values | Where-Object { $_ }).Count
    $totalCount = $testResults.Count

    foreach ($test in $testResults.Keys) {
        $status = if ($testResults[$test]) { "通过" } else { "失败" }
        $color = if ($testResults[$test]) { "Green" } else { "Red" }
        Write-ColorOutput "  $status - $test" $color
    }

    $resultColor = if ($passCount -eq $totalCount) { "Green" } else { "Yellow" }
    Write-ColorOutput "通过率: $passCount/$totalCount" $resultColor

    if ($passCount -eq $totalCount) {
        Write-ColorOutput "所有性能测试完成！" "Green"
    } else {
        Write-ColorOutput "部分测试失败，请检查错误信息" "Yellow"
    }

    Write-ColorOutput "详细报告: docs/PERFORMANCE_ANALYSIS.md" "Blue"

    # 清理临时文件
    Clear-TempFiles
}

# 运行主函数
Main
