# RGO 快速性能测试演示脚本

Write-Host "RGO 性能测试演示" -ForegroundColor Green
Write-Host "===========================================" -ForegroundColor Cyan

# 检查Go环境
Write-Host "检查Go环境..." -ForegroundColor Yellow
try {
    $goVersion = go version
    Write-Host "Go环境正常: $goVersion" -ForegroundColor Green
}
catch {
    Write-Host "未找到Go环境" -ForegroundColor Red
    exit 1
}

# 创建临时目录
if (-not (Test-Path "temp")) {
    New-Item -ItemType Directory -Path "temp" | Out-Null
}

Write-Host ""
Write-Host "运行快速性能测试..." -ForegroundColor Yellow
Write-Host ""

# 运行 RString 测试
Write-Host "1. 测试 RString 性能..." -ForegroundColor Blue
go test -run=^$ -bench=BenchmarkRStringNew -benchmem -count=1 > temp/rstring_result.txt 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "   RString 测试完成" -ForegroundColor Green
    Get-Content temp/rstring_result.txt | Where-Object { $_ -match "BenchmarkRStringNew" }
} else {
    Write-Host "   RString 测试失败" -ForegroundColor Red
}

Write-Host ""

# 运行 RInteger 测试
Write-Host "2. 测试 RInteger 性能..." -ForegroundColor Blue
go test -run=^$ -bench=BenchmarkRIntegerNew -benchmem -count=1 > temp/rinteger_result.txt 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "   RInteger 测试完成" -ForegroundColor Green
    Get-Content temp/rinteger_result.txt | Where-Object { $_ -match "BenchmarkRIntegerNew" }
} else {
    Write-Host "   RInteger 测试失败" -ForegroundColor Red
}

Write-Host ""

# 运行对比测试
Write-Host "3. 运行对比测试..." -ForegroundColor Blue
go test -run=^$ -bench="BenchmarkRStringNew|BenchmarkNativeStringNew" -benchmem -count=1 > temp/comparison_result.txt 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "   对比测试完成" -ForegroundColor Green
    Write-Host ""
    Write-Host "   对比结果:" -ForegroundColor Cyan
    Get-Content temp/comparison_result.txt | Where-Object { $_ -match "Benchmark.*New" }
} else {
    Write-Host "   对比测试失败" -ForegroundColor Red
}

Write-Host ""
Write-Host "===========================================" -ForegroundColor Cyan
Write-Host "演示完成！" -ForegroundColor Green
Write-Host ""
Write-Host "要运行完整的性能测试，请使用:" -ForegroundColor Yellow
Write-Host "  go test -run=^$ -bench=. -benchmem" -ForegroundColor White
Write-Host ""
Write-Host "性能分析文档请查看:" -ForegroundColor Yellow
Write-Host "  docs/PERFORMANCE_ANALYSIS.md" -ForegroundColor White
Write-Host "  docs/BENCHMARK_GUIDE.md" -ForegroundColor White

# 清理临时文件
if (Test-Path "temp") {
    Remove-Item -Path "temp" -Recurse -Force
}
