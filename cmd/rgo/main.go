package main

import (
	"fmt"
	"os"

	"github.com/GoLangDream/rgo/rvm"
	"github.com/GoLangDream/rgo/rvm/compiler"
	"github.com/GoLangDream/rgo/rvm/lexer"
	"github.com/GoLangDream/rgo/rvm/parser"
	"github.com/GoLangDream/rgo/vm/object"
	"github.com/GoLangDream/rgo/core"
)

func main() {
	fmt.Fprintf(os.Stderr, "DEBUG main: starting\n")
	core.Init()
	fmt.Fprintf(os.Stderr, "DEBUG main: core initialized\n")

	args := os.Args[1:]

	if len(args) > 0 {
		filename := args[0]
		file, err := os.Open(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		fileInfo, _ := file.Stat()
		content := make([]byte, fileInfo.Size())
		file.Read(content)

		l := lexer.New(string(content))
		fmt.Fprintf(os.Stderr, "DEBUG main: lexer created\n")
		p := parser.New(l)
		fmt.Fprintf(os.Stderr, "DEBUG main: parser created\n")
		program := p.ParseProgram()
		fmt.Fprintf(os.Stderr, "DEBUG main: ParseProgram done, stmts=%d\n", len(program.Statements))

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
		fmt.Fprintf(os.Stderr, "DEBUG: bytecode instructions len=%d\n", len(bytecode.Instructions))
		v := rvm.New(bytecode)
		fmt.Fprintf(os.Stderr, "DEBUG: VM created\n")
		err = v.Run()
		fmt.Fprintf(os.Stderr, "DEBUG: Run completed, err=%v\n", err)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
			os.Exit(1)
		}

		result := v.LastPoppedStackElement()
		if result != nil {
			fmt.Println(formatValue(result))
		}
	}
}

func formatValue(v *object.EmeraldValue) string {
	if v == nil {
		return "nil"
	}
	return v.Inspect()
}
