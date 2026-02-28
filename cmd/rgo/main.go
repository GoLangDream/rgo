package main

import (
	"fmt"
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/vm"
)

func main() {
	core.Init()

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
}

func formatValue(v *object.EmeraldValue) string {
	if v == nil {
		return "nil"
	}
	return v.Inspect()
}
