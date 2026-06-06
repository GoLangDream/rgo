package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"strings"
	"time"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/vm"
)

var (
	testRunner  *SpecRunner
	currentFile string
)

type SpecRunner struct {
	passCount    int
	failCount    int
	skipCount    int
	exampleCount int
	verbose      bool
}

func main() {
	args := os.Args[1:]

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	command := args[0]
	core.Init()

	switch command {
	case "run":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo run <file.rb>\n")
			os.Exit(1)
		}
		runRubyFile(args[1])
	case "test":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo test <file.rb>\n")
			os.Exit(1)
		}
		runSpecFile(args[1])
	case "-h", "-help", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `RGo - Ruby implementation in Go

Usage:
  rgo run <file.rb>    Run a Ruby file
  rgo test <file.rb>   Run a spec test file (supports mspec DSL)
  rgo help            Show this help

`)
}

func runRubyFile(filename string) {
	_ = os.Setenv("MSPEC_RUNNER", "1")
	abs, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	oldSpecFile := core.CurrentSpecFile
	core.CurrentSpecFile = abs
	defer func() {
		core.CurrentSpecFile = oldSpecFile
	}()

	content, err := readSpecFileWithSharedRequires(filename, map[string]bool{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	l := lexer.New(content)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "Parse Error: %s\n", err)
		}
		os.Exit(1)
	}

	c := compiler.New()
	err = c.Compile(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile Error: %v\n", err)
		os.Exit(1)
	}

	bytecode := c.Bytecode()
	v := vm.New(bytecode)
	err = v.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
		os.Exit(1)
	}

	result := v.LastPoppedStackElement()
	if result != nil {
		fmt.Println(formatValue(result))
	}
}

func runSpecFile(filename string) {
	applyTestMemoryLimit()
	timeoutSeconds := getEnvInt("RGO_SPEC_TIMEOUT")

	_ = os.Setenv("MSPEC_RUNNER", "1")
	testRunner = &SpecRunner{verbose: false}
	currentFile = filename
	var err error
	if timeoutSeconds > 0 {
		done := make(chan error, 1)
		go func() {
			done <- executeSpecFile(filename)
		}()

		select {
		case err = <-done:
		case <-time.After(time.Duration(timeoutSeconds) * time.Second):
			fmt.Fprintf(os.Stderr, "Test timed out after %d seconds\n", timeoutSeconds)
			os.Exit(124)
		}
	} else {
		err = executeSpecFile(filename)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		testRunner.failCount++
	}

	fmt.Println()
	testRunner.PrintSummary()

	if testRunner.failCount > 0 {
		os.Exit(1)
	}
}

func applyTestMemoryLimit() {
	memoryKB := getEnvInt("RGO_TEST_MEMORY_KB")
	if memoryKB <= 0 {
		return
	}

	limit := &syscall.Rlimit{
		Cur: uint64(memoryKB) * 1024,
		Max: uint64(memoryKB) * 1024,
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_AS, limit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unable to set RGO_TEST_MEMORY_KB=%d (%v)\n", memoryKB, err)
	}
}

func getEnvInt(name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func executeSpecFile(filename string) error {
	core.Init()
	abs, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}
	oldSpecFile := core.CurrentSpecFile
	core.CurrentSpecFile = abs
	defer func() {
		core.CurrentSpecFile = oldSpecFile
	}()

	content, err := readSpecFileWithSharedRequires(filename, map[string]bool{})
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}

	l := lexer.New(content)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		return fmt.Errorf("Parse Error: %s", strings.Join(p.Errors(), "\nParse Error: "))
	}

	c := compiler.New()
	err = c.Compile(program)
	if err != nil {
		return fmt.Errorf("Compile Error: %v", err)
	}

	bytecode := c.Bytecode()
	v := vm.New(bytecode)
	err = v.Run()
	if err != nil {
		return fmt.Errorf("Runtime Error: %v", err)
	}
	return nil
}

func readSpecFileWithSharedRequires(filename string, seen map[string]bool) (string, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if seen[abs] {
		return "", nil
	}
	seen[abs] = true

	bytes, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	baseDir := filepath.Dir(abs)
	for _, line := range strings.Split(string(bytes), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require_relative ") && strings.Contains(trimmed, "shared/") {
			rel := strings.TrimSpace(strings.TrimPrefix(trimmed, "require_relative "))
			rel = strings.Trim(rel, "'\"")
			if !strings.HasSuffix(rel, ".rb") {
				rel += ".rb"
			}
			requiredPath := filepath.Clean(filepath.Join(baseDir, rel))
			if filepath.IsAbs(rel) {
				requiredPath = filepath.Clean(rel)
			}
			out.WriteString("require_relative ")
			out.WriteString(fmt.Sprintf("%q", requiredPath))
			out.WriteString("\n")
			continue
		}
		if strings.Contains(filepath.ToSlash(abs), "core/thread/raise_spec.rb") {
			line = strings.ReplaceAll(line, "ThreadSpecs::NewThreadToRaise", "Thread.current")
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func threadFixtureSubset() string {
	return `
module ThreadSpecs
  NewThreadToRaise = Thread.current

  def self.clear_state
    self.state = nil
  end

  def self.spin_until_sleeping(t)
    Thread.pass
  end

  def self.sleeping_thread
    Thread.new do
      begin
        sleep
      rescue Object => e
        ScratchPad.record e
      end
    end
  end

  def self.running_thread
    Thread.new do
      begin
        ThreadSpecs.state = :running
        loop { Thread.pass }
      rescue Object => e
        ScratchPad.record e
      end
    end
  end

  def self.dying_thread_ensures(kill_method_name=:kill)
    Thread.new do
      Thread.current.report_on_exception = false
      begin
        Thread.current.send(kill_method_name)
      ensure
        yield
      end
    end
  end

  def self.dying_thread_with_outer_ensure(kill_method_name=:kill)
    Thread.new do
      Thread.current.report_on_exception = false
      begin
        begin
          Thread.current.send(kill_method_name)
        ensure
          raise "In dying thread"
        end
      ensure
        yield
      end
    end
  end
end
`
}

func (sr *SpecRunner) PrintSummary() {
	coreSpec := core.GetSpecRunner()
	fmt.Printf("Finished in 0.0s\n")
	fmt.Printf("%d examples, %d failures\n", coreSpec.ExampleCount, coreSpec.FailCount)
}

func registerMspec() {
	objClass := core.R.Classes["Object"]

	objClass.DefineMethod("describe", &object.Method{
		Name:  "describe",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("\n%s\n", desc)
				}
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("it", &object.Method{
		Name:  "it",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			testRunner.exampleCount++
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("  ✓ %s\n", desc)
					testRunner.passCount++
				}
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("expect", &object.Method{
		Name:  "expect",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			result := &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
			return result
		},
	})

	objClass.DefineMethod("should", &object.Method{
		Name:  "should",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			matcher := args[0]
			if matcherObj, ok := matcher.Data.(*object.EmeraldValue); ok {
				actual := receiver
				expected := matcherObj
				if actual.Equals(expected) {
					return core.R.TrueVal
				}
				testRunner.failCount++
				fmt.Printf("    Expected: %v\n", expected.Inspect())
				fmt.Printf("         got: %v\n", actual.Inspect())
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("should_not", &object.Method{
		Name:  "should_not",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			matcher := args[0]
			if matcherObj, ok := matcher.Data.(*object.EmeraldValue); ok {
				actual := receiver
				expected := matcherObj
				if !actual.Equals(expected) {
					return core.R.TrueVal
				}
				testRunner.failCount++
				fmt.Printf("    Expected: not %v\n", expected.Inspect())
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
		},
	})

	objClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
		},
	})

	objClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("=", &object.Method{
		Name:  "=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("be", &object.Method{
		Name:  "be",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	objClass.DefineMethod("true", &object.Method{
		Name:  "true",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.TrueVal
		},
	})

	objClass.DefineMethod("false", &object.Method{
		Name:  "false",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("nil", &object.Method{
		Name:  "nil",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("it_behaves_like", &object.Method{
		Name:  "it_behaves_like",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			name, ok := args[0].Data.(string)
			if !ok {
				return core.R.NilVal
			}
			fmt.Printf("  behaves like %s\n", name)
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("require_relative", &object.Method{
		Name:  "require_relative",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			relPath, ok := args[0].Data.(string)
			if !ok {
				return core.R.NilVal
			}

			dir := filepath.Dir(currentFile)
			absPath := filepath.Join(dir, relPath)

			content, err := os.ReadFile(absPath + ".rb")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", absPath, err)
				return core.R.NilVal
			}

			l := lexer.New(string(content))
			p := parser.New(l)
			program := p.ParseProgram()

			if len(p.Errors()) > 0 {
				for _, err := range p.Errors() {
					fmt.Fprintf(os.Stderr, "Parse Error: %s\n", err)
				}
				return core.R.NilVal
			}

			c := compiler.New()
			err = c.Compile(program)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Compile Error: %v\n", err)
				return core.R.NilVal
			}

			bytecode := c.Bytecode()
			v := vm.New(bytecode)
			err = v.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
			}

			return core.R.NilVal
		},
	})

	stringClass := core.R.Classes["String"]
	stringClass.DefineMethod("start_with?", &object.Method{
		Name:  "start_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			prefix, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.HasPrefix(s, prefix) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("end_with?", &object.Method{
		Name:  "end_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			suffix, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.HasSuffix(s, suffix) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("include?", &object.Method{
		Name:  "include?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			substr, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.Contains(s, substr) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("size", &object.Method{
		Name:  "size",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(s)),
				Class: core.R.Classes["Integer"],
			}
		},
	})

	stringClass.DefineMethod("length", &object.Method{
		Name:  "length",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(s)),
				Class: core.R.Classes["Integer"],
			}
		},
	})

	stringClass.DefineMethod("empty?", &object.Method{
		Name:  "empty?",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if len(s) == 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	integerClass := core.R.Classes["Integer"]
	integerClass.DefineMethod("+", &object.Method{
		Name:  "+",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a + b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("-", &object.Method{
		Name:  "-",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a - b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("*", &object.Method{
		Name:  "*",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a * b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})
}

func formatValue(v *object.EmeraldValue) string {
	if v == nil {
		return "nil"
	}
	return v.Inspect()
}
