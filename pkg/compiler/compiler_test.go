package compiler

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
)

func init() {
	core.Init()
}

func compile(t *testing.T, input string) *Bytecode {
	t.Helper()
	l := lexer.New(input)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	c := New()
	err := c.Compile(program)
	if err != nil {
		t.Fatalf("compile error: %s", err)
	}
	return c.Bytecode()
}

func hasOpcode(instructions Instructions, op Opcode) bool {
	i := 0
	for i < len(instructions) {
		currentOp := Opcode(instructions[i])
		if currentOp == op {
			return true
		}
		def, ok := Lookup(instructions[i])
		if !ok {
			i++
			continue
		}
		width := 1
		for _, w := range def.OperandWidths {
			width += w
		}
		i += width
	}
	return false
}

func functionConstants(bytecode *Bytecode) []*object.Function {
	functions := []*object.Function{}
	for _, constant := range bytecode.Constants {
		if fn, ok := constant.Data.(*object.Function); ok {
			functions = append(functions, fn)
		}
	}
	return functions
}

func TestBlockPassedToMethodCapturesOuterLocalWithLocalOpcode(t *testing.T) {
	bytecode := compile(t, `def call_proc(&p)
  p.call
end
x = 41
call_proc { x + 1 }`)

	foundBlockWithLocal := false
	for _, fn := range functionConstants(bytecode) {
		if hasOpcode(fn.Instructions, OpGetLocal) {
			foundBlockWithLocal = true
			break
		}
	}
	if !foundBlockWithLocal {
		t.Fatalf("expected a top-level block function to read captured x with OpGetLocal")
	}
}

func TestBlockPassedToMethodCapturesEarlierOuterLocalWithLocalOpcode(t *testing.T) {
	bytecode := compile(t, `x = 41
def call_proc(&p)
  p.call
end
call_proc { x + 1 }`)
	foundBlockWithLocal := false
	for _, fn := range functionConstants(bytecode) {
		if hasOpcode(fn.Instructions, OpGetLocal) {
			foundBlockWithLocal = true
			break
		}
	}
	if !foundBlockWithLocal {
		t.Fatalf("expected a top-level block function to read captured x with OpGetLocal")
	}
}

func TestBlockAssignmentUsesOuterLocalOpcodes(t *testing.T) {
	bytecode := compile(t, `i = 0
2.times do
  i += 1
end
i`)
	foundGetOuter := false
	foundSetOuter := false
	for _, fn := range functionConstants(bytecode) {
		if hasOpcode(fn.Instructions, OpGetOuter) {
			foundGetOuter = true
		}
		if hasOpcode(fn.Instructions, OpSetOuter) {
			foundSetOuter = true
		}
	}
	if !foundGetOuter {
		t.Fatal("expected block function to read i with OpGetOuter")
	}
	if !foundSetOuter {
		t.Fatal("expected block function to assign i with OpSetOuter")
	}
}

func TestCompileInterpolatedRegexpWithEncodingModifierCall(t *testing.T) {
	compile(t, `/#{/./}/e.encoding.should == Encoding::EUC_JP`)
}

func TestCompileKeywordLiteralMethodNameAfterDot(t *testing.T) {
	compile(t, `module VariablesSpecs
  def self.false
    false
  end
end

if VariablesSpecs.false
  a = 1
end

1.times do
  defined?(a).should == "local-variable"
end`)
}

func countOpcode(instructions Instructions, op Opcode) int {
	count := 0
	i := 0
	for i < len(instructions) {
		currentOp := Opcode(instructions[i])
		if currentOp == op {
			count++
		}
		def, ok := Lookup(instructions[i])
		if !ok {
			i++
			continue
		}
		width := 1
		for _, w := range def.OperandWidths {
			width += w
		}
		i += width
	}
	return count
}

// === Literals ===

func TestCompileInteger(t *testing.T) {
	bc := compile(t, "42")
	if !hasOpcode(bc.Instructions, OpConstant) {
		t.Error("expected OpConstant")
	}
	if !hasOpcode(bc.Instructions, OpPop) {
		t.Error("expected OpPop for expression statement")
	}
	if len(bc.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(bc.Constants))
	}
	if bc.Constants[0].Type != object.ValueInteger {
		t.Errorf("expected Integer, got %v", bc.Constants[0].Type)
	}
	if bc.Constants[0].Data.(int64) != 42 {
		t.Errorf("expected 42, got %v", bc.Constants[0].Data)
	}
}

func TestCompileFloat(t *testing.T) {
	bc := compile(t, "3.14")
	if len(bc.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(bc.Constants))
	}
	if bc.Constants[0].Type != object.ValueFloat {
		t.Errorf("expected Float, got %v", bc.Constants[0].Type)
	}
	if bc.Constants[0].Data.(float64) != 3.14 {
		t.Errorf("expected 3.14, got %v", bc.Constants[0].Data)
	}
}

func TestCompileString(t *testing.T) {
	bc := compile(t, `"hello"`)
	if len(bc.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(bc.Constants))
	}
	if bc.Constants[0].Type != object.ValueString {
		t.Errorf("expected String, got %v", bc.Constants[0].Type)
	}
	if bc.Constants[0].Data.(string) != "hello" {
		t.Errorf("expected hello, got %v", bc.Constants[0].Data)
	}
}

func TestCompileRegexp(t *testing.T) {
	bc := compile(t, `/foo/`)
	if len(bc.Constants) != 1 {
		t.Fatalf("expected 1 constant, got %d", len(bc.Constants))
	}
	if bc.Constants[0].Type != object.ValueRegexp {
		t.Errorf("expected Regexp, got %v", bc.Constants[0].Type)
	}
	r, ok := bc.Constants[0].Data.(*object.RRegexp)
	if !ok {
		t.Fatalf("expected *object.RRegexp, got %T", bc.Constants[0].Data)
	}
	if r.Pattern != "foo" {
		t.Errorf("expected foo, got %v", r.Pattern)
	}
}

func TestCompileIncludeExpressionAsMethodCall(t *testing.T) {
	bc := compile(t, "include(1)")
	if !hasOpcode(bc.Instructions, OpSend) {
		t.Fatal("expected include expression to compile to OpSend")
	}
}

func TestCompileDefUsesDistinctLocalIndexes(t *testing.T) {
	bc := compile(t, "def f(a, b)\n  a + b\nend")
	var fn *object.Function
	for _, constant := range bc.Constants {
		if constant.Type == object.ValueFunction {
			fn = constant.Data.(*object.Function)
			break
		}
	}
	if fn == nil {
		t.Fatal("expected function constant")
	}

	indexes := []byte{}
	for i := 0; i < len(fn.Instructions); i++ {
		if Opcode(fn.Instructions[i]) == OpGetLocal {
			indexes = append(indexes, fn.Instructions[i+1])
			i++
		}
	}
	if len(indexes) != 2 {
		t.Fatalf("expected 2 OpGetLocal instructions, got %d", len(indexes))
	}
	if indexes[0] != 0 || indexes[1] != 1 {
		t.Fatalf("expected local indexes [0 1], got %v", indexes)
	}
}

func TestCompileBeginRescueEnsureDoesNotCorruptConstants(t *testing.T) {
	bc := compile(t, `x = 0
begin
  raise "e"
rescue
  x = 1
ensure
  x = x + 10
end
x`)
	for i := 0; i < len(bc.Instructions); i++ {
		op := Opcode(bc.Instructions[i])
		def, ok := Lookup(byte(op))
		if !ok {
			continue
		}
		if op == OpConstant {
			idx := int(bc.Instructions[i+1])<<8 | int(bc.Instructions[i+2])
			if idx < 0 || idx >= len(bc.Constants) {
				t.Fatalf("OpConstant at %d references constant %d, only %d constants", i, idx, len(bc.Constants))
			}
		}
		for _, width := range def.OperandWidths {
			i += width
		}
	}
}

func TestCompileInstanceVariableLambdaAssignmentDoesNotCorruptConstants(t *testing.T) {
	bc := compile(t, `@value_to_return = -> _ { true }`)
	for i := 0; i < len(bc.Instructions); i++ {
		op := Opcode(bc.Instructions[i])
		def, ok := Lookup(byte(op))
		if !ok {
			continue
		}
		if op == OpConstant || op == OpSetInstanceVar || op == OpLambda || op == OpClosure {
			idx := int(bc.Instructions[i+1])<<8 | int(bc.Instructions[i+2])
			if idx < 0 || idx >= len(bc.Constants) {
				t.Fatalf("%s at %d references constant %d, only %d constants", def.Name, i, idx, len(bc.Constants))
			}
		}
		for _, width := range def.OperandWidths {
			i += width
		}
	}
}

func TestCompileTrue(t *testing.T) {
	bc := compile(t, "true")
	if !hasOpcode(bc.Instructions, OpTrue) {
		t.Error("expected OpTrue")
	}
}

func TestCompileFalse(t *testing.T) {
	bc := compile(t, "false")
	if !hasOpcode(bc.Instructions, OpFalse) {
		t.Error("expected OpFalse")
	}
}

func TestCompileNil(t *testing.T) {
	bc := compile(t, "nil")
	if !hasOpcode(bc.Instructions, OpNil) {
		t.Error("expected OpNil")
	}
}

// === Arithmetic ===

func TestCompileAddition(t *testing.T) {
	bc := compile(t, "1 + 2")
	if !hasOpcode(bc.Instructions, OpAdd) {
		t.Error("expected OpAdd")
	}
	if len(bc.Constants) != 2 {
		t.Errorf("expected 2 constants, got %d", len(bc.Constants))
	}
}

func TestCompileSubtraction(t *testing.T) {
	bc := compile(t, "10 - 5")
	if !hasOpcode(bc.Instructions, OpSub) {
		t.Error("expected OpSub")
	}
}

func TestCompileMultiplication(t *testing.T) {
	bc := compile(t, "3 * 4")
	if !hasOpcode(bc.Instructions, OpMul) {
		t.Error("expected OpMul")
	}
}

func TestCompileDivision(t *testing.T) {
	bc := compile(t, "10 / 3")
	if !hasOpcode(bc.Instructions, OpDiv) {
		t.Error("expected OpDiv")
	}
}

func TestCompileModulo(t *testing.T) {
	bc := compile(t, "17 % 5")
	if !hasOpcode(bc.Instructions, OpMod) {
		t.Error("expected OpMod")
	}
}

func TestCompilePower(t *testing.T) {
	bc := compile(t, "2 ** 10")
	if !hasOpcode(bc.Instructions, OpPow) {
		t.Error("expected OpPow")
	}
}

// === Comparison ===

func TestCompileEqual(t *testing.T) {
	bc := compile(t, "1 == 2")
	if !hasOpcode(bc.Instructions, OpEqual) {
		t.Error("expected OpEqual")
	}
}

func TestCompileNotEqual(t *testing.T) {
	bc := compile(t, "1 != 2")
	if !hasOpcode(bc.Instructions, OpNotEqual) {
		t.Error("expected OpNotEqual")
	}
}

func TestCompileGreaterThan(t *testing.T) {
	bc := compile(t, "1 > 2")
	if !hasOpcode(bc.Instructions, OpGreaterThan) {
		t.Error("expected OpGreaterThan")
	}
}

func TestCompileLessThan(t *testing.T) {
	bc := compile(t, "1 < 2")
	if !hasOpcode(bc.Instructions, OpLessThan) {
		t.Error("expected OpLessThan")
	}
}

func TestCompileGreaterThanOrEqual(t *testing.T) {
	bc := compile(t, "1 >= 2")
	if !hasOpcode(bc.Instructions, OpGreaterThanOrEqual) {
		t.Error("expected OpGreaterThanOrEqual")
	}
}

func TestCompileLessThanOrEqual(t *testing.T) {
	bc := compile(t, "1 <= 2")
	if !hasOpcode(bc.Instructions, OpLessThanOrEqual) {
		t.Error("expected OpLessThanOrEqual")
	}
}

// === Prefix ===

func TestCompileBang(t *testing.T) {
	bc := compile(t, "!true")
	if !hasOpcode(bc.Instructions, OpBang) {
		t.Error("expected OpBang")
	}
}

func TestCompileNeg(t *testing.T) {
	bc := compile(t, "-5")
	if !hasOpcode(bc.Instructions, OpNeg) {
		t.Error("expected OpNeg")
	}
}

// === Assignment ===

func TestCompileAssignment(t *testing.T) {
	bc := compile(t, "x = 5")
	if !hasOpcode(bc.Instructions, OpSetLocal) {
		t.Error("expected OpSetLocal")
	}
}

func TestCompileVariableReference(t *testing.T) {
	bc := compile(t, "x = 5\nx")
	if countOpcode(bc.Instructions, OpSetLocal) != 1 {
		t.Error("expected 1 OpSetLocal")
	}
	if countOpcode(bc.Instructions, OpGetLocal) != 1 {
		t.Error("expected 1 OpGetLocal")
	}
}

// === Array ===

func TestCompileEmptyArray(t *testing.T) {
	bc := compile(t, "[]")
	if !hasOpcode(bc.Instructions, OpArray) {
		t.Error("expected OpArray")
	}
}

func TestCompileArray(t *testing.T) {
	bc := compile(t, "[1, 2, 3]")
	if !hasOpcode(bc.Instructions, OpArray) {
		t.Error("expected OpArray")
	}
	if len(bc.Constants) != 3 {
		t.Errorf("expected 3 constants, got %d", len(bc.Constants))
	}
}

// === Hash ===

func TestCompileEmptyHash(t *testing.T) {
	bc := compile(t, "{}")
	if !hasOpcode(bc.Instructions, OpHash) {
		t.Error("expected OpHash")
	}
}

func TestCompileHashArrow(t *testing.T) {
	bc := compile(t, `{"a" => 1}`)
	if !hasOpcode(bc.Instructions, OpHash) {
		t.Error("expected OpHash")
	}
}

// === Index ===

func TestCompileIndex(t *testing.T) {
	bc := compile(t, `"hello"[0]`)
	if !hasOpcode(bc.Instructions, OpIndex) {
		t.Error("expected OpIndex")
	}
}

// === If Expression ===

func TestCompileIfExpression(t *testing.T) {
	bc := compile(t, "if true\n  5\nend")
	if !hasOpcode(bc.Instructions, OpJumpNotTruthy) {
		t.Error("expected OpJumpNotTruthy")
	}
	if !hasOpcode(bc.Instructions, OpTrue) {
		t.Error("expected OpTrue for condition")
	}
}

func TestCompileIfElseExpression(t *testing.T) {
	bc := compile(t, "if true\n  1\nelse\n  2\nend")
	if !hasOpcode(bc.Instructions, OpJumpNotTruthy) {
		t.Error("expected OpJumpNotTruthy")
	}
	if !hasOpcode(bc.Instructions, OpJump) {
		t.Error("expected OpJump for else branch")
	}
}

// === Method Call ===

func TestCompileMethodCall(t *testing.T) {
	bc := compile(t, `"hello".upcase`)
	if !hasOpcode(bc.Instructions, OpSend) {
		t.Error("expected OpSend")
	}
}

// === Self ===

func TestCompileSelf(t *testing.T) {
	bc := compile(t, "self")
	if !hasOpcode(bc.Instructions, OpSelf) {
		t.Error("expected OpSelf")
	}
}

// === Multiple Statements ===

func TestCompileMultipleStatements(t *testing.T) {
	bc := compile(t, "1\n2")
	// Each expression statement gets an OpPop
	if countOpcode(bc.Instructions, OpPop) != 2 {
		t.Errorf("expected 2 OpPop, got %d", countOpcode(bc.Instructions, OpPop))
	}
	if len(bc.Constants) != 2 {
		t.Errorf("expected 2 constants, got %d", len(bc.Constants))
	}
}
