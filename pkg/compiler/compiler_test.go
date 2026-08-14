package compiler

import (
	"strings"
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
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

func TestCompileCatchWithoutBlockDoesNotSetLastException(t *testing.T) {
	previous := core.LastException
	core.LastException = nil
	t.Cleanup(func() { core.LastException = previous })

	compile(t, `catch(:missing_block)`)
	if core.LastException != nil {
		t.Fatalf("compilation leaked an exception into runtime state: %v", core.LastException)
	}
}

func flipFlopStateIDs(instructions Instructions) []int {
	var ids []int
	for i := 0; i < len(instructions); {
		op := Opcode(instructions[i])
		def, ok := Lookup(instructions[i])
		if !ok {
			i++
			continue
		}
		if op == OpFlipFlopGet && i+2 < len(instructions) {
			ids = append(ids, int(instructions[i+1])<<8|int(instructions[i+2]))
		}
		width := 1
		for _, operandWidth := range def.OperandWidths {
			width += operandWidth
		}
		i += width
	}
	return ids
}

func TestCompileCombinedFlipFlopsWithDistinctState(t *testing.T) {
	bc := compile(t, `10.times { |i| i if (i == 4)...(i == 5) or (i == 7)...(i == 8) }`)
	for _, constant := range bc.Constants {
		fn, ok := constant.Data.(*object.Function)
		if !ok {
			continue
		}
		ids := flipFlopStateIDs(fn.Instructions)
		if len(ids) == 2 {
			if ids[0] == ids[1] {
				t.Fatalf("expected distinct flip-flop states, got %v", ids)
			}
			return
		}
	}
	t.Fatal("expected a block containing two flip-flops")
}

func TestCompileSplatIndexCompoundAssignmentUsesSingleCoercionOpcode(t *testing.T) {
	bc := compile(t, `target[*key] += 1`)
	if !hasOpcode(bc.Instructions, OpIndexSplatCompoundAssign) {
		t.Fatal("expected splat index compound assignment opcode")
	}
}

func TestCompileMultipleSplatsAsSingleExpandableArray(t *testing.T) {
	bc := compile(t, `f(*[:a], *[:b], *[:c], 10)`)
	if !hasOpcode(bc.Instructions, OpSplatToArray) {
		t.Fatal("expected multiple splats to be packed into an expandable array")
	}
}

func TestCompileYieldMultipleSplatsAsSingleExpandableArray(t *testing.T) {
	bc := compile(t, `yield(*[:a], *[:b])`)
	if !hasOpcode(bc.Instructions, OpYieldWithSplat) || !hasOpcode(bc.Instructions, OpSplatToArray) {
		t.Fatal("expected multiple yield splats to be packed into an expandable array")
	}
}

func TestCompileRescueMatchMarksSplatExceptions(t *testing.T) {
	bc := compile(t, `
exceptions = [SecondError]
begin
  raise SecondError
rescue FirstError, *exceptions
  true
end`)
	found := false
	for i := 0; i < len(bc.Instructions); i++ {
		if Opcode(bc.Instructions[i]) != OpRescueMatch {
			continue
		}
		found = true
		count := int(bc.Instructions[i+1])
		mask := int(bc.Instructions[i+2])<<8 | int(bc.Instructions[i+3])
		if count != 2 {
			t.Fatalf("expected rescue match count 2, got %d", count)
		}
		if mask != 2 {
			t.Fatalf("expected splat mask 2, got %d", mask)
		}
	}
	if !found {
		t.Fatal("expected OpRescueMatch")
	}
	if hasOpcode(bc.Instructions, OpSplat) {
		t.Fatal("rescue splat should be expanded by OpRescueMatch, not OpSplat")
	}
}

func TestCompileQualifiedRescueClassInBlock(t *testing.T) {
	bc := compile(t, `Thread.new do
  begin
    raise IO::EAGAINWaitReadable
  rescue IO::WaitReadable
    :rescued
  end
end`)
	var block *object.Function
	for _, candidate := range functionConstants(bc) {
		if candidate.Name == "__block__" {
			block = candidate
			break
		}
	}
	if block == nil {
		t.Fatal("expected block function")
	}
	foundHandler := false
	for i := 0; i < len(block.Instructions); {
		op := Opcode(block.Instructions[i])
		if op == OpBeginRescue {
			foundHandler = true
			rescueOffset := int(block.Instructions[i+1])<<8 | int(block.Instructions[i+2])
			if rescueOffset >= len(block.Instructions) || Opcode(block.Instructions[rescueOffset]) != OpGetConstant {
				t.Fatalf("expected rescue target %d to load IO, got %v", rescueOffset, block.Instructions[rescueOffset])
			}
			nameIndex := int(block.Instructions[rescueOffset+1])<<8 | int(block.Instructions[rescueOffset+2])
			if got := bc.Constants[nameIndex].Data; got != "IO" {
				t.Fatalf("expected rescue target to load IO, got %v", got)
			}
		}
		if op == OpGetConstant {
			nameIndex := int(block.Instructions[i+1])<<8 | int(block.Instructions[i+2])
			if got := bc.Constants[nameIndex].Data; got == "WaitReadable" {
				t.Fatal("qualified rescue class leaked into body as bare WaitReadable")
			}
		}
		def, ok := Lookup(byte(block.Instructions[i]))
		if !ok {
			i++
			continue
		}
		width := 1
		for _, operandWidth := range def.OperandWidths {
			width += operandWidth
		}
		i += width
	}
	if !foundHandler {
		t.Fatal("expected rescue handler")
	}
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

func TestMethodLocalLoadsUseFastOpcodeUnlessCaptured(t *testing.T) {
	bytecode := compile(t, `
def fast_local(value)
	value.to_s
end
def captured_local(value)
  reader = -> { value }
  reader.call
  value
end
`)
	var fast, captured *object.Function
	for _, fn := range functionConstants(bytecode) {
		switch fn.Name {
		case "fast_local":
			fast = fn
		case "captured_local":
			captured = fn
		}
	}
	if fast == nil || !hasOpcode(fast.Instructions, OpGetLocalFast) {
		t.Fatal("expected uncaptured method local to use OpGetLocalFast")
	}
	if captured == nil || !hasOpcode(captured.Instructions, OpGetLocalCell) || !hasOpcode(captured.Instructions, OpGetLocal) {
		t.Fatalf("expected captured method local to retain cell-aware OpGetLocal: %x", captured.Instructions)
	}
}

func TestCompileParameterDestructuringMetadata(t *testing.T) {
	bytecode := compile(t, `def unpack((a, *middle, z)); [a, middle, z]; end`)
	var fn *object.Function
	for _, candidate := range functionConstants(bytecode) {
		if candidate.Name == "unpack" {
			fn = candidate
			break
		}
	}
	if fn == nil {
		t.Fatal("expected compiled unpack function")
	}
	if len(fn.Params) != 1 || len(fn.ParamPatterns) != 1 {
		t.Fatalf("expected one physical parameter and pattern, got %d and %d", len(fn.Params), len(fn.ParamPatterns))
	}
	pattern := fn.ParamPatterns[0]
	if pattern == nil || len(pattern.Children) != 2 || pattern.Rest == nil || pattern.RestIndex != 1 {
		t.Fatalf("unexpected runtime pattern: %#v", pattern)
	}
	for _, name := range []string{"a", "middle", "z"} {
		if _, ok := fn.LocalNames[name]; !ok {
			t.Fatalf("expected local slot for %s", name)
		}
	}
}

func TestCompileMethodCallWithoutMethodNameReturnsError(t *testing.T) {
	program := &ast.Program{
		Statements: []ast.Statement{
			&ast.ExpressionStatement{
				Expression: &ast.MethodCall{
					Token: lexer.Token{Type: lexer.DOT, Literal: ".", Line: 1, Column: 4},
					Receiver: &ast.Identifier{
						Token: lexer.Token{Type: lexer.IDENT, Literal: "obj", Line: 1, Column: 1},
						Value: "obj",
					},
				},
			},
		},
	}

	err := New().Compile(program)
	if err == nil {
		t.Fatal("expected compile error")
	}
	if !strings.Contains(err.Error(), "method call missing method name") {
		t.Fatalf("unexpected error: %s", err)
	}
}

func TestBlockPassedToMethodCapturesOuterLocalWithLocalOpcode(t *testing.T) {
	bytecode := compile(t, `def call_proc(&p)
  p.call
end
x = 41
call_proc { x + 1 }`)

	foundBlockWithLocal := false
	for _, fn := range functionConstants(bytecode) {
		if hasOpcode(fn.Instructions, OpGetLocal) || hasOpcode(fn.Instructions, OpGetLocalFast) {
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
		if hasOpcode(fn.Instructions, OpGetLocal) || hasOpcode(fn.Instructions, OpGetLocalFast) {
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
		if hasOpcode(fn.Instructions, OpGetFree) {
			foundGetOuter = true
		}
		if hasOpcode(fn.Instructions, OpSetFree) {
			foundSetOuter = true
		}
	}
	if !foundGetOuter {
		t.Fatal("expected block function to read captured i with OpGetFree")
	}
	if !foundSetOuter {
		t.Fatal("expected block function to assign captured i with OpSetFree")
	}
}

func TestCompileInterpolatedRegexpWithEncodingModifierCall(t *testing.T) {
	compile(t, `/#{/./}/e.encoding.should == Encoding::EUC_JP`)
}

func TestCompileMethodNameOnLineAfterDot(t *testing.T) {
	compile(t, `Encoding::Converter.
  asciicompat_encoding(Encoding.find("ISO-2022-JP")).
  should == Encoding::Converter.asciicompat_encoding("ISO-2022-JP")`)
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

func TestCompilePrependExpression(t *testing.T) {
	compile(t, `wrapper = Module.new do
  def initialize(...)
    super(...)
  end
end

klass = Class.new(Struct.new(:a)) { prepend wrapper }`)
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
		if op := Opcode(fn.Instructions[i]); op == OpGetLocal || op == OpGetLocalFast {
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

func TestCompileStringLiteralMatchingParameterAsConstant(t *testing.T) {
	bc := compile(t, `def assign_link(hash, href); hash["href"] = href; end`)
	var fn *object.Function
	for _, candidate := range functionConstants(bc) {
		if candidate.Name == "assign_link" {
			fn = candidate
			break
		}
	}
	if fn == nil {
		t.Fatal("expected compiled assign_link function")
	}
	if got := countOpcode(fn.Instructions, OpConstant); got != 1 {
		t.Fatalf("expected literal \"href\" to compile as one constant, got %d", got)
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

func TestCompilerReusesIntegerConstants(t *testing.T) {
	c := New()
	first := c.addConstant(&object.EmeraldValue{Type: object.ValueInteger, Data: int64(42), Class: core.R.Classes["Integer"]})
	second := c.addConstant(&object.EmeraldValue{Type: object.ValueInteger, Data: int64(42), Class: core.R.Classes["Integer"]})
	if first != second {
		t.Fatalf("expected repeated integer constant index %d, got %d", first, second)
	}
	if len(c.constants) != 1 {
		t.Fatalf("expected one integer constant, got %d", len(c.constants))
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

func TestBytecodeBindsCompiledFunctionsToFinalConstantPool(t *testing.T) {
	bc := compile(t, `-> { "value" }`)
	for _, constant := range bc.Constants {
		fn, ok := constant.Data.(*object.Function)
		if !ok {
			continue
		}
		if len(fn.Constants) != len(bc.Constants) || len(bc.Constants) == 0 || &fn.Constants[0] != &bc.Constants[0] {
			t.Fatalf("compiled function does not reference the final bytecode constant pool")
		}
		return
	}
	t.Fatal("expected compiled lambda function constant")
}

func TestCompiledAttributeMethodShapes(t *testing.T) {
	bc := compile(t, `class Shape
  def value
    @value
  end
  def value=(value)
    @value = value
  end
end`)
	shapes := map[string][]byte{}
	for _, constant := range bc.Constants {
		if fn, ok := constant.Data.(*object.Function); ok && (fn.Name == "value" || fn.Name == "value=") {
			shapes[fn.Name] = append([]byte(nil), fn.Instructions...)
		}
	}
	reader := shapes["value"]
	if len(reader) != 4 || Opcode(reader[0]) != OpGetInstanceVar || Opcode(reader[3]) != OpReturnValue {
		t.Fatalf("unexpected attribute reader shape: %#v", reader)
	}
	writer := shapes["value="]
	if len(writer) != 6 || (Opcode(writer[0]) != OpGetLocal && Opcode(writer[0]) != OpGetLocalFast) || Opcode(writer[2]) != OpSetInstanceVar || Opcode(writer[5]) != OpReturnValue {
		t.Fatalf("unexpected attribute writer shape: %#v", writer)
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
