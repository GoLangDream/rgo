package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(content))
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
	testRunner = &SpecRunner{verbose: false}
	currentFile = filename

	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	core.RegisterMspec()

	l := lexer.New(string(content))
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
		testRunner.failCount++
	}

	fmt.Println()
	testRunner.PrintSummary()

	if testRunner.failCount > 0 {
		os.Exit(1)
	}
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
			fmt.Fprintf(os.Stderr, "DEBUG expect: args=%v len=%d\n", args, len(args))
			if len(args) == 0 {
				return core.R.NilVal
			}
			fmt.Fprintf(os.Stderr, "DEBUG expect: arg0=%v\n", args[0])
			result := &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
			fmt.Fprintf(os.Stderr, "DEBUG expect: result=%v\n", result)
			return result
		},
	})

	objClass.DefineMethod("should", &object.Method{
		Name:  "should",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			fmt.Fprintf(os.Stderr, "DEBUG should: receiver=%v args=%v\n", receiver, args)
			if len(args) == 0 {
				return core.R.NilVal
			}
			matcher := args[0]
			if matcherObj, ok := matcher.Data.(*object.EmeraldValue); ok {
				actual := receiver
				expected := matcherObj
				fmt.Fprintf(os.Stderr, "DEBUG should: actual=%v expected=%v\n", actual, expected)
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
