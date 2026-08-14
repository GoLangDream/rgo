package vm

import (
	"fmt"
	"testing"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
)

func TestDebugLocationIR(t *testing.T) {
	core.InitWithMspec()
	source := `class S
  attr_accessor :shape, :location_name
  def location_name
    @location_name || (shape && shape["locationName"])
  end
end`
	p := parser.New(lexer.New(source))
	program := p.ParseProgram()
	c := compiler.New()
	if err := c.Compile(program); err != nil {
		t.Fatal(err)
	}
	for _, constant := range c.Bytecode().Constants {
		if constant == nil || constant.Type != object.ValueFunction {
			continue
		}
		fn, ok := constant.Data.(*object.Function)
		if !ok || fn == nil {
			continue
		}
		plan, ok := compileRegisterIR(fn)
		if !ok {
			continue
		}
		if fn.Name == "location_name" {
			for i, op := range plan.instructions {
				fmt.Printf("IR %d: op=%d dst=%d left=%d right=%d name=%q target=%d\n", i, op.op, op.dst, op.left, op.right, op.name, op.target)
			}
			fmt.Printf("sends=%d frame=%t seq=%s\n", plan.sendCount, plan.requiresFrame, registerIROpcodeSequence(fn))
		}
	}
}
