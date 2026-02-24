package main

import (
	"fmt"
	"os"

	"github.com/GoLangDream/rgo/rvm"
	"github.com/GoLangDream/rgo/rvm/compiler"
	"github.com/GoLangDream/rgo/rvm/lexer"
	"github.com/GoLangDream/rgo/rvm/parser"
)

func main() {
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
		v := rvm.New(bytecode)
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

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "nil"
	case []interface{}:
		return fmt.Sprintf("%v", val)
	case map[interface{}]interface{}:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
