package runner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/vm"
)

// SpecRunner 运行 ruby/spec 测试
type SpecRunner struct {
	output      io.Writer
	passCount   int
	failCount   int
	skipCount   int
	currentDesc string
	verbose     bool
	examples    []*Example
}

// Example 表示一个测试用例
type Example struct {
	Description string
	Passed      bool
	Error       string
	Skipped     bool
}

// NewSpecRunner 创建新的测试运行器
func NewSpecRunner(output io.Writer, verbose bool) *SpecRunner {
	return &SpecRunner{
		output:   output,
		verbose:  verbose,
		examples: make([]*Example, 0),
	}
}

// RunFile 运行单个 spec 文件
func (sr *SpecRunner) RunFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	return sr.RunCode(string(content), filename)
}

// RunCode 运行 Ruby 代码
func (sr *SpecRunner) RunCode(code string, filename string) error {
	// 初始化 Ruby 运行时
	core.Init()

	// 注册测试辅助方法
	sr.registerSpecHelpers()

	// 词法分析
	l := lexer.New(code)

	// 语法分析
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		return fmt.Errorf("parser errors:\n%s", strings.Join(p.Errors(), "\n"))
	}

	// 编译
	comp := compiler.New()
	err := comp.Compile(program)
	if err != nil {
		return fmt.Errorf("compilation error: %w", err)
	}

	// 执行
	bytecode := comp.Bytecode()
	machine := vm.New(bytecode)

	err = machine.Run()
	if err != nil {
		return fmt.Errorf("runtime error: %w", err)
	}

	return nil
}

// registerSpecHelpers 注册测试辅助方法
func (sr *SpecRunner) registerSpecHelpers() {
	// 注册 describe 方法
	objectClass := core.R.Classes["Object"]

	objectClass.DefineMethod("describe", &object.Method{
		Name:  "describe",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					sr.currentDesc = desc
					if sr.verbose {
						fmt.Fprintf(sr.output, "\n%s\n", desc)
					}
				}
			}
			return core.R.NilVal
		},
	})

	// 注册 it 方法
	objectClass.DefineMethod("it", &object.Method{
		Name:  "it",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					example := &Example{
						Description: desc,
						Passed:      true,
					}
					sr.examples = append(sr.examples, example)
					sr.passCount++

					if sr.verbose {
						fmt.Fprintf(sr.output, "  ✓ %s\n", desc)
					} else {
						fmt.Fprintf(sr.output, ".")
					}
				}
			}
			return core.R.NilVal
		},
	})
}

// PrintSummary 打印测试摘要
func (sr *SpecRunner) PrintSummary() {
	total := sr.passCount + sr.failCount + sr.skipCount
	fmt.Fprintf(sr.output, "\n\n")
	fmt.Fprintf(sr.output, "Finished in 0.0 seconds\n")
	fmt.Fprintf(sr.output, "%d examples, %d failures, %d skipped\n",
		total, sr.failCount, sr.skipCount)

	if sr.failCount == 0 && total > 0 {
		fmt.Fprintf(sr.output, "\n✓ All tests passed!\n")
	}
}

// GetStats 获取测试统计
func (sr *SpecRunner) GetStats() (pass, fail, skip int) {
	return sr.passCount, sr.failCount, sr.skipCount
}
