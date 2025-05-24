@echo off
chcp 65001 >nul
echo RGO 性能测试演示
echo ===========================================

echo 检查Go环境...
go version >nul 2>&1
if errorlevel 1 (
    echo 未找到Go环境，请确保Go已正确安装
    pause
    exit /b 1
)
echo Go环境正常

echo.
echo 运行快速性能测试...
echo.

echo 1. 测试 RString 性能...
go test -run=^$ -bench=BenchmarkRStringNew -benchmem -count=1
if errorlevel 1 (
    echo    RString 测试失败
) else (
    echo    RString 测试完成
)

echo.
echo 2. 测试 RInteger 性能...
go test -run=^$ -bench=BenchmarkRIntegerNew -benchmem -count=1
if errorlevel 1 (
    echo    RInteger 测试失败
) else (
    echo    RInteger 测试完成
)

echo.
echo 3. 运行对比测试...
go test -run=^$ -bench="BenchmarkRStringNew|BenchmarkNativeStringNew" -benchmem -count=1
if errorlevel 1 (
    echo    对比测试失败
) else (
    echo    对比测试完成
)

echo.
echo ===========================================
echo 演示完成！
echo.
echo 要运行完整的性能测试，请使用:
echo   go test -run=^$ -bench=. -benchmem
echo.
echo 性能分析文档请查看:
echo   docs/PERFORMANCE_ANALYSIS.md
echo   docs/BENCHMARK_GUIDE.md
echo.
pause
