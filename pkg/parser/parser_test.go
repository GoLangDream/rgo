package parser

import (
	"testing"
	"time"

	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

func parse(t *testing.T, input string) *ast.Program {
	t.Helper()
	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	return program
}

func parseExpr(t *testing.T, input string) ast.Expression {
	t.Helper()
	program := parse(t, input)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}
	return stmt.Expression
}

func TestParseAllSpecSharedExampleCompletes(t *testing.T) {
	input := `describe :array_iterable_and_tolerating_size_increasing, shared: true do
  before do
    @value_to_return ||= -> _ { nil }
  end

  it "tolerates increasing an array size during iteration" do
    array = [1, 2, 3]
    array_to_join = [:a, :b, :c] + (4..100).to_a

    ScratchPad.record []
    i = 0

    array.send(@method) do |e|
      ScratchPad << e
      array << array_to_join[i] if i < array_to_join.size
      i += 1
      @value_to_return.call(e)
    end

    ScratchPad.recorded.should == [1, 2, 3] + array_to_join
  end
end`

	input += `

describe "Array#all?" do
  @value_to_return = -> _ { true }
  it_behaves_like :array_iterable_and_tolerating_size_increasing, :all?

  it "ignores the block if there is an argument" do
    -> {
      ['bar', 'foobar'].all?(/bar/) { false }.should == true
    }.should complain(/given block not used/)
  end
end`

	done := make(chan struct{})
	var errors []string
	statementCount := 0
	go func() {
		l := lexer.New(input)
		p := New(l)
		program := p.ParseProgram()
		statementCount = len(program.Statements)
		errors = p.Errors()
		close(done)
	}()

	select {
	case <-done:
		if len(errors) > 0 {
			t.Fatalf("parse errors: %v", errors)
		}
		if statementCount != 2 {
			t.Fatalf("expected 2 top-level statements, got %d", statementCount)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("parser did not complete")
	}
}

func TestParseChainedPredicateMethodCall(t *testing.T) {
	input := "empty_array.should_not.any?"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) == 0 {
		t.Fatal("expected at least one statement")
	}
}

func TestParseChainedCallAfterBraceBlock(t *testing.T) {
	input := "['bar', 'foobar'].any?(/bar/) { false }.should == true"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d: %s", len(program.Statements), program.String())
	}
}

func TestParseCatchWithBraceBlockAndThrowCallChainCompletes(t *testing.T) {
	input := `catch(:out) { throw(:out, 42).foo }`
	done := make(chan struct{})
	var errors []string
	statementCount := 0

	go func() {
		l := lexer.New(input)
		p := New(l)
		program := p.ParseProgram()
		statementCount = len(program.Statements)
		errors = p.Errors()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("parser timed out on catch with brace block")
	}
	if len(errors) > 0 {
		t.Fatalf("parse errors: %v", errors)
	}
	if statementCount != 1 {
		t.Fatalf("expected 1 statement, got %d", statementCount)
	}
}

func TestParseDefinedWithThrowCallChain(t *testing.T) {
	parse(t, `defined?(throw(:out, 42).foo).should == :unreachable`)
}

func TestParseThrowWithBareSecondArgument(t *testing.T) {
	parse(t, `throw :exit, :msg`)
	parse(t, `catch(1) { throw 1, 2 }.should == 2`)
}

func TestParseDefinedWithQualifiedConstantAssignment(t *testing.T) {
	parse(t, `defined?(Object::A = 2).should == "assignment"`)
	parse(t, `defined?(Object::A += 1).should == "assignment"`)
	parse(t, `defined?(Object::A ||= true).should == "assignment"`)
	parse(t, `defined?(Object::A &&= true).should == "assignment"`)
}

func TestParseDefinedWithControlFlowExpression(t *testing.T) {
	parse(t, `defined?(yield)`)
	parse(t, `defined?(break).should == "expression"`)
	parse(t, `defined?(next).should == "expression"`)
	parse(t, `defined?(return).should == "expression"`)
	parse(t, `defined?(while x do y end).should == "expression"`)
	parse(t, `defined?(until x do y end).should == "expression"`)
}

func TestParseCallWithEmptyBraceBlock(t *testing.T) {
	input := `[1, 2].send(:initialize, 1, "x", true) {}`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBareCallWithEmptyBraceBlockAndTrailingCall(t *testing.T) {
	parse(t, `call_defined() { }.should == "yield"`)
}

func TestParseNestedLambdaWithBraceBlockAndTrailingCall(t *testing.T) {
	input := `-> {
  -> { [1, 2, 3].send(:initialize) { raise } }.should_not raise_error
}.should complain(/x/, verbose: true)`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParsePredicateCallWithBraceBlockParameters(t *testing.T) {
	input := "empty_array.any? {|v| 1 == 1 }.should == false"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBareCallWithSymbolArgAndDoBlock(t *testing.T) {
	input := `before :each do
  @enum = [1, 2, 42].bsearch_index
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseIncludeCallWithBlockCallArgument(t *testing.T) {
	input := "[1, 2].should include(@array.bsearch_index { |x| 1 - x / 4 })"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) == 0 {
		t.Fatal("expected at least one statement")
	}
}

func TestParseMultilineCallWithClosingParenOnOwnLine(t *testing.T) {
	input := `-> { Hash.new(unknown: true) }.should complain(
  Regexp.new(Regexp.escape("Calling Hash.new with keyword arguments is deprecated and will be removed in Ruby 3.4; use Hash.new({ key: value }) instead"))
)`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseIncludeCallWithParenthesizedBlockArgument(t *testing.T) {
	input := "[1, 2].should include(@array.bsearch_index { |x| (1 - x / 4) * (2**100) })"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) == 0 {
		t.Fatal("expected at least one statement")
	}
}

func TestParseIncludeCallWithChainedGroupedReceiverInBlock(t *testing.T) {
	input := "[1, 2].should include(@array.bsearch_index { |x| (2**100).coerce((1 - x / 4) * (2**100)).first })"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) == 0 {
		t.Fatal("expected at least one statement")
	}
}

func TestParseLambdaContainingBraceBlockKeepsTrailingCallOnLambda(t *testing.T) {
	input := `describe :sample_shared, shared: true do
  it "runs nested block" do
    -> { enumerator.each { |x| x } }.should raise_error(FrozenError)
  end
end

describe "consumer" do
  it_behaves_like :sample_shared, :each
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected consumer describe to remain top-level, got %d statements: %s", len(program.Statements), program.String())
	}
}

func TestParseChainedGroupedReceiverWithNestedGroupedArgument(t *testing.T) {
	input := "(2**100).coerce((1 - x / 4) * (2**100)).first"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseNestedTernaryInBraceBlock(t *testing.T) {
	input := "[0, 1, 2].bsearch { |x| x < 2 ? 1.0 : x > 2 ? -1.0 : 0.0 }"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseInfixRightHandSideAcrossNewline(t *testing.T) {
	input := "left.should ==\n  right.should"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseCaseWhenThenInsideBlock(t *testing.T) {
	input := `a.fill do |i|
  case i
  when 0 then -1
  when 1 then -2
  when 2 then raise StandardError, "Oops"
  else 0
  end
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseConstantIndexCallWithMultipleArguments(t *testing.T) {
	input := "ArraySpecs::MyArray[1, 2, 3]"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseRaiseWithExceptionClassAndMessage(t *testing.T) {
	input := "raise StandardError, 'Oops'"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseCallArgumentsAcrossNewlines(t *testing.T) {
	input := `raise_error(
  TypeError, "buffer must be String, not Array")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseInstanceVariableIndexAssignment(t *testing.T) {
	input := "@array[0] = 1"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseAttributeAssignment(t *testing.T) {
	input := `Encoding.default_external = Encoding.find("UTF-8")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseIndexAssignmentWithMultipleValues(t *testing.T) {
	input := `a[3, 2] = "a", "b", "c"`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParsePostfixIncrement(t *testing.T) {
	input := "i++\nindex"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(program.Statements))
	}
}

func TestParseComplexReceiverIndexAssignment(t *testing.T) {
	input := `[1, 2, 3, 4, 5][2, -1] = [7, 8]`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseMethodCallReceiverIndexAssignment(t *testing.T) {
	input := `ArraySpecs.frozen_array[0, 0] = []`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseOperatorSymbolArgument(t *testing.T) {
	input := "obj.should_receive(:<=>).with(other)"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseQuotedSymbolArgument(t *testing.T) {
	input := `raise_error(TypeError, :"foo")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseInstanceVariableSymbolArgument(t *testing.T) {
	input := `obj.instance_variable_set(:@hash, hash)`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseArrayClassBracketMethodCall(t *testing.T) {
	input := `Array.[](5, true, nil, "a")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseConstantFunctionCall(t *testing.T) {
	input := `Rational(3, 4).to_f`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseExplicitBracketAssignmentMethodCall(t *testing.T) {
	input := `a.[]=(2..4, 10)`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBracketAssignmentMethodDefinitionWithAnonymousRest(t *testing.T) {
	input := `def []=(*)
  raise "[]= is called"
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}
	def, ok := stmt.Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", stmt.Expression)
	}
	if def.Name.Value != "[]=" {
		t.Fatalf("expected method name []=, got %q", def.Name.Value)
	}
	if def.RestParam == nil || def.RestParam.Value != "_" {
		t.Fatalf("expected anonymous rest parameter, got %#v", def.RestParam)
	}
}

func TestParseSetterMethodDefinitionWithAnonymousRest(t *testing.T) {
	program := parse(t, `def foobar=(*)
  1
end`)
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	def, ok := stmt.Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", stmt.Expression)
	}
	if def.Name.Value != "foobar=" {
		t.Fatalf("expected method name foobar=, got %q", def.Name.Value)
	}
	if def.RestParam == nil || def.RestParam.Value != "_" {
		t.Fatalf("expected anonymous rest parameter, got %#v", def.RestParam)
	}
}

func TestParseMethodDefinitionWithBlockParameter(t *testing.T) {
	input := `def each(&b)
  [3, 4].each(&b)
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	def, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", program.Statements[0])
	}
	if def.BlockParam == nil || def.BlockParam.Value != "b" {
		t.Fatalf("expected block parameter b, got %#v", def.BlockParam)
	}
}

func TestParseArrayClassBracketCallWithManyArguments(t *testing.T) {
	input := `Array[5, true, nil, "a"]`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBareCallWithMultipleArgsAsMethodArgument(t *testing.T) {
	input := `result.should include(1, 2)`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBareIncludeMatcherWithBlockArgument(t *testing.T) {
	input := `[1, 2].should include(@array.bsearch_index { |x| 1 - x / 4 })`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseConstantBracketCallWithNoArguments(t *testing.T) {
	input := "ArraySpecs::MyArray[]"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBlockPassArgument(t *testing.T) {
	input := "@array.cycle(2, &@prc)"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBlockPassGroupedSequenceArgument(t *testing.T) {
	parse(t, "@obj.foo1(a += 1, &(a += 1; p)).should == [1, true]")
}

func TestParseSingletonOperatorMethodDefinition(t *testing.T) {
	input := "def x.==(other) 3 == other end"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseTripleEqualsMethodDefinition(t *testing.T) {
	parse(t, `def ===(other)
  true
end`)
}

func TestParseRaiseWithPostfixIfInGroupedExpression(t *testing.T) {
	parse(t, `(raise if 2 + 2 == 3; /a/)`)
}

func TestParseThenAsMethodName(t *testing.T) {
	parse(t, `self.then { value }`)
}

func TestParseYieldAsMethodName(t *testing.T) {
	expr := parseExpr(t, "Fiber.yield")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil {
		t.Fatal("expected method name yield, got nil")
	}
	if call.Method.Value != "yield" {
		t.Fatalf("expected method yield, got %s", call.Method.Value)
	}
}

func TestParseBacktickOperatorSymbolArgument(t *testing.T) {
	parse(t, "runner.singleton_class.define_method(:`) do |str|\nend")
}

func TestParseAliasWithSpaceshipMethodNames(t *testing.T) {
	input := `begin
  class Integer
    alias old_spaceship <=>
  end
ensure
  class Integer
    alias <=> old_spaceship
  end
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseAliasWithGlobalVariables(t *testing.T) {
	parse(t, "alias $b $a")
}

func TestParseAliasWithBracketMethodName(t *testing.T) {
	parse(t, "alias old_get []")
}

func TestParseVisibilityMethodCallsWithSymbolArguments(t *testing.T) {
	parse(t, "public :foo\nprivate :bar\nprotected :baz")
}

func TestParseBareCallWithGlobalVariablesInArrayArgument(t *testing.T) {
	parse(t, "p [$a, $b]")
}

func TestParseGlobalVariableAliasSequence(t *testing.T) {
	parse(t, "$a = 1; alias $b $a; p [$a, $b]; $b = 2; p [$a, $b]")
}

func TestParseMultiAssignWithInstanceAndGlobalVariables(t *testing.T) {
	parse(t, "@verbose, $VERBOSE = $VERBOSE, nil")
}

func TestParseRubyExeGlobalAliasExpectation(t *testing.T) {
	parse(t, `code = '$a = 1; alias $b $a; p [$a, $b]; $b = 2; p [$a, $b]'
ruby_exe(code).should == "[1, 1]\n[2, 2]\n"`)
}

func TestParseAnonymousRestParameterInSingletonMethodDefinition(t *testing.T) {
	input := "def bo.method_missing(name, *)\n  [1, 2]\nend"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseInstanceVariableSingletonMethodDefinition(t *testing.T) {
	input := "def @obj.respond_to_missing?(name, priv) false end"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseSingletonClassExpression(t *testing.T) {
	input := "class << obj; undef :to_s; end"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseSingletonClassExpressionDoesNotConsumeFollowingStatement(t *testing.T) {
	input := `describe :sample, shared: true do
  it "runs" do
    class << obj; undef :to_s; end
  end
end

describe "consumer" do
  it_behaves_like :sample, :join
end`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected consumer describe to remain top-level, got %d statements: %s", len(program.Statements), program.String())
	}
}

func TestParseUnlessWithThen(t *testing.T) {
	input := "unless false then\n  'baz'\nend.should == 'baz'"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseOneLineUnlessWithThenAndElse(t *testing.T) {
	input := "unless false then 'foo'; else 'bar'; end.should == 'foo'"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseWhileWithDo(t *testing.T) {
	input := "while i < 3 do\n  i += 1\nend"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseWhileWithDoAndSameLineBody(t *testing.T) {
	input := "while i < 3 do i += 1\nend"

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseRangeArgumentWithSpaces(t *testing.T) {
	input := `a.send(:[], "a" .. "b")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseBeginlessRangeArgument(t *testing.T) {
	input := `a.send(:[], (..0))`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseExclusiveBeginlessRangeArgument(t *testing.T) {
	input := `a.send(:[], (...0))`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseEndlessRangeArgument(t *testing.T) {
	input := `a.send(:[], (2..))`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseEndlessRangeMethodCallArgument(t *testing.T) {
	input := `@array.send(@method, (2..).step(-1)).should == [2, 1, 0]`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseNegativeBeginlessRangeArgument(t *testing.T) {
	input := `a.send(:[], (..-2))`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseNegativeEndlessRangeArgument(t *testing.T) {
	input := `a.send(:[], (-3..).step(-1))`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseArrayLiteralWithTrailingComma(t *testing.T) {
	input := `[0, 1, 2,]`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

// === Literals ===

func TestParseIntegerLiteral(t *testing.T) {
	expr := parseExpr(t, "42")
	lit, ok := expr.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expected IntegerLiteral, got %T", expr)
	}
	if lit.Value != 42 {
		t.Errorf("expected 42, got %d", lit.Value)
	}
}

func TestParseLargeHexIntegerLiteralDoesNotError(t *testing.T) {
	expr := parseExpr(t, "0xdef0abcd34127856")
	if _, ok := expr.(*ast.IntegerLiteral); !ok {
		t.Fatalf("expected IntegerLiteral, got %T", expr)
	}
}

func TestParseHugeHexIntegerLiteralDoesNotError(t *testing.T) {
	expr := parseExpr(t, "0xffffffffffffffffffffffff")
	if _, ok := expr.(*ast.IntegerLiteral); !ok {
		t.Fatalf("expected IntegerLiteral, got %T", expr)
	}
}

func TestParseFloatLiteral(t *testing.T) {
	expr := parseExpr(t, "3.14")
	lit, ok := expr.(*ast.FloatLiteral)
	if !ok {
		t.Fatalf("expected FloatLiteral, got %T", expr)
	}
	if lit.Value != 3.14 {
		t.Errorf("expected 3.14, got %f", lit.Value)
	}
}

func TestParseStringLiteral(t *testing.T) {
	expr := parseExpr(t, `"hello"`)
	lit, ok := expr.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("expected StringLiteral, got %T", expr)
	}
	if lit.Value != "hello" {
		t.Errorf("expected hello, got %s", lit.Value)
	}
}

func TestParseSquigglyHeredocWithTrailingFluentDot(t *testing.T) {
	parse(t, "code = <<~CODE\n  10\nCODE\n.codepoints")
}

func TestParseSquigglyHeredocWithMarkerLineSuffix(t *testing.T) {
	parse(t, "eval(<<~CODE).should == nil\n  10\nCODE\n")
}

func TestParseIndentedHeredocWithKeywordArgumentInsideBlock(t *testing.T) {
	parse(t, `it "warns" do
  err = ruby_exe(<<-END_OF_CODE, args: "2>&1")
    return 10
  END_OF_CODE
  err.should =~ /warning/
end`)
}

func TestParseBooleanTrue(t *testing.T) {
	expr := parseExpr(t, "true")
	b, ok := expr.(*ast.Boolean)
	if !ok {
		t.Fatalf("expected Boolean, got %T", expr)
	}
	if !b.Value {
		t.Error("expected true")
	}
}

func TestParseBooleanFalse(t *testing.T) {
	expr := parseExpr(t, "false")
	b, ok := expr.(*ast.Boolean)
	if !ok {
		t.Fatalf("expected Boolean, got %T", expr)
	}
	if b.Value {
		t.Error("expected false")
	}
}

func TestParseNil(t *testing.T) {
	expr := parseExpr(t, "nil")
	_, ok := expr.(*ast.NilExpression)
	if !ok {
		t.Fatalf("expected NilExpression, got %T", expr)
	}
}

func TestParseCaseSubjectAndBranchBody(t *testing.T) {
	expr := parseExpr(t, "case 1\nwhen 1\n  10\nelse\n  20\nend")
	caseExpr, ok := expr.(*ast.CaseExpression)
	if !ok {
		t.Fatalf("expected CaseExpression, got %T", expr)
	}
	if caseExpr.Expression == nil {
		t.Fatal("expected case subject expression")
	}
	if len(caseExpr.Clauses) != 1 {
		t.Fatalf("expected 1 clause, got %d", len(caseExpr.Clauses))
	}
	if len(caseExpr.Clauses[0].Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(caseExpr.Clauses[0].Conditions))
	}
	if len(caseExpr.Clauses[0].Body.Statements) != 1 {
		t.Fatalf("expected 1 branch statement, got %d", len(caseExpr.Clauses[0].Body.Statements))
	}
	if caseExpr.Else == nil || len(caseExpr.Else.Statements) != 1 {
		t.Fatalf("expected else branch with 1 statement")
	}
}

func TestParseIdentifier(t *testing.T) {
	expr := parseExpr(t, "foo")
	ident, ok := expr.(*ast.Identifier)
	if !ok {
		t.Fatalf("expected Identifier, got %T", expr)
	}
	if ident.Value != "foo" {
		t.Errorf("expected foo, got %s", ident.Value)
	}
}

func TestParseSymbol(t *testing.T) {
	expr := parseExpr(t, ":hello")
	sym, ok := expr.(*ast.SymbolLiteral)
	if !ok {
		t.Fatalf("expected SymbolLiteral, got %T", expr)
	}
	if sym.Value != ":hello" {
		t.Errorf("expected :hello, got %s", sym.Value)
	}
}

func TestParseRegexpLiteral(t *testing.T) {
	expr := parseExpr(t, `/foo/i`)
	re, ok := expr.(*ast.RegexpLiteral)
	if !ok {
		t.Fatalf("expected RegexpLiteral, got %T", expr)
	}
	if re.Pattern != "foo" {
		t.Errorf("expected pattern foo, got %s", re.Pattern)
	}
	if re.Options != "i" {
		t.Errorf("expected option i, got %s", re.Options)
	}
}

func TestParseLambdaWithBareParameter(t *testing.T) {
	expr := parseExpr(t, `-> _ { true }`)
	proc, ok := expr.(*ast.ProcLiteral)
	if !ok {
		t.Fatalf("expected ProcLiteral, got %T", expr)
	}
	if len(proc.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(proc.Params))
	}
	if proc.Params[0].Value != "_" {
		t.Errorf("expected param _, got %s", proc.Params[0].Value)
	}
	if proc.Body == nil || len(proc.Body.Statements) != 1 {
		t.Fatalf("expected body with 1 statement, got %#v", proc.Body)
	}
}

func TestParseLambdaWithBareParameterInsideBlock(t *testing.T) {
	program := parse(t, `m { -> _ { true } }`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

// === Infix Expressions ===

func TestParseAddition(t *testing.T) {
	expr := parseExpr(t, "1 + 2")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "+" {
		t.Errorf("expected +, got %s", infix.Operator)
	}
	assertIntLit(t, infix.Left, 1)
	assertIntLit(t, infix.Right, 2)
}

func TestParseSubtraction(t *testing.T) {
	expr := parseExpr(t, "10 - 5")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "-" {
		t.Errorf("expected -, got %s", infix.Operator)
	}
}

func TestParseMultiplication(t *testing.T) {
	expr := parseExpr(t, "3 * 4")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "*" {
		t.Errorf("expected *, got %s", infix.Operator)
	}
}

func TestParsePower(t *testing.T) {
	expr := parseExpr(t, "2 ** 10")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "**" {
		t.Errorf("expected **, got %s", infix.Operator)
	}
}

func TestParseDoubleSplatCallArgument(t *testing.T) {
	parse(t, `@a.call(**{a: 1})`)
}

func TestParseComparison(t *testing.T) {
	tests := []struct {
		input string
		op    string
	}{
		{"1 > 2", ">"},
		{"1 < 2", "<"},
		{"1 >= 2", ">="},
		{"1 <= 2", "<="},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr := parseExpr(t, tt.input)
			infix, ok := expr.(*ast.InfixExpression)
			if !ok {
				t.Fatalf("expected InfixExpression, got %T", expr)
			}
			if infix.Operator != tt.op {
				t.Errorf("expected %s, got %s", tt.op, infix.Operator)
			}
		})
	}
}

// === Operator Precedence ===

func TestOperatorPrecedence(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1 + 2 * 3", "(1 + (2 * 3))"},
		{"1 * 2 + 3", "((1 * 2) + 3)"},
		{"1 + 2 + 3", "((1 + 2) + 3)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr := parseExpr(t, tt.input)
			if expr.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, expr.String())
			}
		})
	}
}

// === Prefix Expressions ===

func TestParsePrefixBang(t *testing.T) {
	expr := parseExpr(t, "!true")
	prefix, ok := expr.(*ast.PrefixExpression)
	if !ok {
		t.Fatalf("expected PrefixExpression, got %T", expr)
	}
	if prefix.Operator != "!" {
		t.Errorf("expected !, got %s", prefix.Operator)
	}
}

func TestParsePrefixMinus(t *testing.T) {
	expr := parseExpr(t, "-5")
	prefix, ok := expr.(*ast.PrefixExpression)
	if !ok {
		t.Fatalf("expected PrefixExpression, got %T", expr)
	}
	if prefix.Operator != "-" {
		t.Errorf("expected -, got %s", prefix.Operator)
	}
}

// === Assignment ===

func TestParseAssignment(t *testing.T) {
	expr := parseExpr(t, "x = 5")
	assign, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected AssignExpression, got %T", expr)
	}
	if assign.Name.Value != "x" {
		t.Errorf("expected x, got %s", assign.Name.Value)
	}
	assertIntLit(t, assign.Value, 5)
}

// === Method Call ===

func TestParseMethodCallDot(t *testing.T) {
	expr := parseExpr(t, `"hello".upcase`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "upcase" {
		t.Errorf("expected upcase, got %s", call.Method.Value)
	}
}

// === Grouped Expression ===

func TestParseGroupedExpression(t *testing.T) {
	expr := parseExpr(t, "(1 + 2) * 3")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "*" {
		t.Errorf("expected *, got %s", infix.Operator)
	}
	// Left should be (1 + 2)
	left, ok := infix.Left.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected left to be InfixExpression, got %T", infix.Left)
	}
	if left.Operator != "+" {
		t.Errorf("expected +, got %s", left.Operator)
	}
}

// === Multiple Statements ===

func TestParseMultipleStatements(t *testing.T) {
	program := parse(t, "x = 1\ny = 2")
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(program.Statements))
	}
}

// === Instance/Class/Global Variables ===

func TestParseInstanceVariable(t *testing.T) {
	expr := parseExpr(t, "@name")
	iv, ok := expr.(*ast.InstanceVariable)
	if !ok {
		t.Fatalf("expected InstanceVariable, got %T", expr)
	}
	if iv.Name != "@name" {
		t.Errorf("expected @name, got %s", iv.Name)
	}
}

func TestParseClassVariable(t *testing.T) {
	expr := parseExpr(t, "@@count")
	cv, ok := expr.(*ast.ClassVariable)
	if !ok {
		t.Fatalf("expected ClassVariable, got %T", expr)
	}
	if cv.Name != "@@count" {
		t.Errorf("expected @@count, got %s", cv.Name)
	}
}

func TestParseGlobalVariable(t *testing.T) {
	expr := parseExpr(t, "$stdout")
	gv, ok := expr.(*ast.GlobalVariable)
	if !ok {
		t.Fatalf("expected GlobalVariable, got %T", expr)
	}
	if gv.Name != "$stdout" {
		t.Errorf("expected $stdout, got %s", gv.Name)
	}
}

func TestParseSpecialGlobalVariableDotAssignment(t *testing.T) {
	expr := parseExpr(t, "$. = 0")
	assign, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected AssignExpression, got %T", expr)
	}
	if assign.Name.Value != "$." {
		t.Fatalf("expected $. assignment, got %s", assign.Name.Value)
	}
}

// === Self ===

func TestParseSelf(t *testing.T) {
	expr := parseExpr(t, "self")
	_, ok := expr.(*ast.SelfExpression)
	if !ok {
		t.Fatalf("expected SelfExpression, got %T", expr)
	}
}

// === String Index ===

func TestParseStringIndex(t *testing.T) {
	expr := parseExpr(t, `"hello"[0]`)
	idx, ok := expr.(*ast.IndexExpression)
	if !ok {
		t.Fatalf("expected IndexExpression, got %T", expr)
	}
	assertIntLit(t, idx.Index, 0)
}

// === helpers ===

func assertIntLit(t *testing.T, expr ast.Expression, expected int64) {
	t.Helper()
	lit, ok := expr.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expected IntegerLiteral, got %T", expr)
	}
	if lit.Value != expected {
		t.Errorf("expected %d, got %d", expected, lit.Value)
	}
}

// === Equality and Inequality (was: BANG_EQUAL not registered) ===

func TestParseEqual(t *testing.T) {
	expr := parseExpr(t, "1 == 2")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "==" {
		t.Errorf("expected ==, got %s", infix.Operator)
	}
	assertIntLit(t, infix.Left, 1)
	assertIntLit(t, infix.Right, 2)
}

func TestParseNotEqual(t *testing.T) {
	expr := parseExpr(t, "1 != 2")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "!=" {
		t.Errorf("expected !=, got %s", infix.Operator)
	}
	assertIntLit(t, infix.Left, 1)
	assertIntLit(t, infix.Right, 2)
}

// === Logical AND/OR (was: AND/OR not registered) ===

func TestParseLogicalAnd(t *testing.T) {
	expr := parseExpr(t, "true && false")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "&&" {
		t.Errorf("expected &&, got %s", infix.Operator)
	}
}

func TestParseLogicalOr(t *testing.T) {
	expr := parseExpr(t, "true || false")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "||" {
		t.Errorf("expected ||, got %s", infix.Operator)
	}
}

func TestParseAssignmentAsRightHandSideOfBooleanAnd(t *testing.T) {
	expr := parseExpr(t, "true && false && x = 1")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	assign, ok := infix.Right.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected right side assignment, got %T", infix.Right)
	}
	if assign.Name.Value != "x" {
		t.Fatalf("expected assignment to x, got %s", assign.Name.Value)
	}
	assertIntLit(t, assign.Value, 1)
}

func TestParseAssignmentAsRightHandSideOfBooleanOr(t *testing.T) {
	expr := parseExpr(t, "x = true || false || y = 1")
	assign, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected AssignExpression, got %T", expr)
	}
	infix, ok := assign.Value.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected assignment value to be InfixExpression, got %T", assign.Value)
	}
	right, ok := infix.Right.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected boolean right side assignment, got %T", infix.Right)
	}
	if right.Name.Value != "y" {
		t.Fatalf("expected assignment to y, got %s", right.Name.Value)
	}
	assertIntLit(t, right.Value, 1)
}

func TestParseAssignmentAsNestedRightHandSideOfBooleanExpression(t *testing.T) {
	expr := parseExpr(t, "x = 1 || false && y = 2")
	assign, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected AssignExpression, got %T", expr)
	}
	orExpr, ok := assign.Value.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected assignment value to be InfixExpression, got %T", assign.Value)
	}
	andExpr, ok := orExpr.Right.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected nested right side InfixExpression, got %T", orExpr.Right)
	}
	right, ok := andExpr.Right.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected nested boolean right side assignment, got %T", andExpr.Right)
	}
	if right.Name.Value != "y" {
		t.Fatalf("expected assignment to y, got %s", right.Name.Value)
	}
	assertIntLit(t, right.Value, 2)
}

func TestParseLogicalAndOr(t *testing.T) {
	// || has lower precedence than &&
	expr := parseExpr(t, "a && b || c")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "||" {
		t.Errorf("expected || at top, got %s", infix.Operator)
	}
	left, ok := infix.Left.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected left to be InfixExpression, got %T", infix.Left)
	}
	if left.Operator != "&&" {
		t.Errorf("expected && on left, got %s", left.Operator)
	}
}

// === Array Literal (was: infinite loop) ===

func TestParseEmptyArray(t *testing.T) {
	expr := parseExpr(t, "[]")
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(arr.Elements))
	}
}

func TestParseSingleElementArray(t *testing.T) {
	expr := parseExpr(t, "[1]")
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 1 {
		t.Errorf("expected 1 element, got %d", len(arr.Elements))
	}
	assertIntLit(t, arr.Elements[0], 1)
}

func TestParseMultiElementArray(t *testing.T) {
	expr := parseExpr(t, "[1, 2, 3]")
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr.Elements))
	}
	assertIntLit(t, arr.Elements[0], 1)
	assertIntLit(t, arr.Elements[1], 2)
	assertIntLit(t, arr.Elements[2], 3)
}

func TestParseMethodCallOnArrayLiteral(t *testing.T) {
	expr := parseExpr(t, "[1,2,3].length")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "length" {
		t.Fatalf("expected length, got %s", call.Method.Value)
	}
	arr, ok := call.Receiver.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral receiver, got %T", call.Receiver)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParseArrayLiteralAsBareMethodArgument(t *testing.T) {
	expr := parseExpr(t, "puts [1,2,3].length")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "puts" {
		t.Fatalf("expected puts, got %s", call.Method.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.MethodCall); !ok {
		t.Fatalf("expected method call arg, got %T", call.Args[0])
	}
}

func TestParseHashIndexAssignment(t *testing.T) {
	parse(t, "h = {}; h[:x] = 42")
}

// === Hash Literal (was: conflicts with infix COLON) ===

func TestParseEmptyHash(t *testing.T) {
	expr := parseExpr(t, "{}")
	hash, ok := expr.(*ast.HashLiteral)
	if !ok {
		t.Fatalf("expected HashLiteral, got %T", expr)
	}
	if len(hash.Pairs) != 0 {
		t.Errorf("expected 0 pairs, got %d", len(hash.Pairs))
	}
}

func TestParseHashWithSymbolShorthand(t *testing.T) {
	expr := parseExpr(t, "{a: 1, b: 2}")
	hash, ok := expr.(*ast.HashLiteral)
	if !ok {
		t.Fatalf("expected HashLiteral, got %T", expr)
	}
	if len(hash.Pairs) != 2 {
		t.Errorf("expected 2 pairs, got %d", len(hash.Pairs))
	}
}

func TestParseHashWithArrow(t *testing.T) {
	expr := parseExpr(t, `{"a" => 1}`)
	hash, ok := expr.(*ast.HashLiteral)
	if !ok {
		t.Fatalf("expected HashLiteral, got %T", expr)
	}
	if len(hash.Pairs) != 1 {
		t.Errorf("expected 1 pair, got %d", len(hash.Pairs))
	}
}

func TestParseHashLiteralWithTrailingComma(t *testing.T) {
	parse(t, "h = {a: 1, b: 2,}")
}

func TestParseHashLiteralWithEmptyGroupedKeyAndValue(t *testing.T) {
	parse(t, "h = {() => ()}")
}

func TestParseHashLiteralWithQuotedLabelKey(t *testing.T) {
	parse(t, `h = {"d": 4}`)
}

func TestParseHashLiteralWithDoubleSplatElement(t *testing.T) {
	parse(t, "h = {a: 1, **{b: 2}, c: 3}")
	parse(t, "h = {**other, a: 1}")
}

func TestParseHashLiteralWithOmittedValue(t *testing.T) {
	parse(t, "h = {a:}")
	parse(t, "h = {a:, b:, c:,}")
}

func TestParseOneLineMethodWithSymbolKeyHashBody(t *testing.T) {
	parse(t, "def h.to_hash; {:b => 2, :c => 3}; end")
}

func TestParseMethodCallWithSpaceBeforeArrayArgument(t *testing.T) {
	expr := parseExpr(t, "ScratchPad.record [a, b]")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected ArrayLiteral argument, got %T", call.Args[0])
	}
}

func TestParseMatchOperatorAsExplicitMethodCall(t *testing.T) {
	tests := []string{`@regexp.=~(@string)`, `@regexp.!~(@string)`}
	for _, input := range tests {
		expr := parseExpr(t, input)
		call, ok := expr.(*ast.MethodCall)
		if !ok {
			t.Fatalf("expected MethodCall for %q, got %T", input, expr)
		}
		if call.Method == nil {
			t.Fatalf("expected method for %q", input)
		}
	}
}

// === If Expression (was: timeout / expectPeek side effects) ===

func TestParseIfExpression(t *testing.T) {
	program := parse(t, "if true\n  5\nend")
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}
	ifExpr, ok := stmt.Expression.(*ast.IfExpression)
	if !ok {
		t.Fatalf("expected IfExpression, got %T", stmt.Expression)
	}
	if ifExpr.Consequent == nil {
		t.Fatal("expected consequent block")
	}
	if len(ifExpr.Consequent.Statements) != 1 {
		t.Errorf("expected 1 consequent statement, got %d", len(ifExpr.Consequent.Statements))
	}
}

func TestParseIfElseExpression(t *testing.T) {
	program := parse(t, "if true\n  1\nelse\n  2\nend")
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	ifExpr, ok := stmt.Expression.(*ast.IfExpression)
	if !ok {
		t.Fatalf("expected IfExpression, got %T", stmt.Expression)
	}
	if ifExpr.Consequent == nil {
		t.Fatal("expected consequent block")
	}
	if ifExpr.Alternative == nil {
		t.Fatal("expected alternative block")
	}
}

func TestParseIfElsifElseExpression(t *testing.T) {
	program := parse(t, "if true\n  1\nelsif false\n  2\nelse\n  3\nend")
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	ifExpr, ok := stmt.Expression.(*ast.IfExpression)
	if !ok {
		t.Fatalf("expected IfExpression, got %T", stmt.Expression)
	}
	if len(ifExpr.ElsIf) != 1 {
		t.Errorf("expected 1 elsif, got %d", len(ifExpr.ElsIf))
	}
	if ifExpr.Alternative == nil {
		t.Fatal("expected alternative block")
	}
}

func TestParseIfWithThen(t *testing.T) {
	program := parse(t, "if true then 5 end")
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	_, ok := stmt.Expression.(*ast.IfExpression)
	if !ok {
		t.Fatalf("expected IfExpression, got %T", stmt.Expression)
	}
}

func TestParseGroupedPostfixIfWithTrailingCall(t *testing.T) {
	parse(t, "(123 if true).should == 123")
}

func TestParseGroupedPostfixUnlessWithTrailingCall(t *testing.T) {
	parse(t, "(123 unless false).should == 123")
}

func TestParseGroupedPostfixWhileWithTrailingCall(t *testing.T) {
	parse(t, "(i += 1 while i < 10).should == nil")
}

func TestParseGroupedPostfixUntilWithTrailingCall(t *testing.T) {
	parse(t, "(i += 1 until i == 10).should == nil")
}

func TestParseTernaryWithNextConsequentInsideWhileModifier(t *testing.T) {
	parse(t, "((i += 1) == 3 ? next : j += i) while i <= 10")
}

func TestParseGroupedMultiStatementExpression(t *testing.T) {
	parse(t, "a[1] ||= (break if c\nc = false)")
}

func TestParseAssignmentValueAcrossNewline(t *testing.T) {
	parse(t, "a[1] ||=\n  (\n    break if c\n    c = false\n  )")
}

func TestParseSetterSymbolArgument(t *testing.T) {
	parse(t, "a.should_receive(:m=)")
}

func TestParseSafeNavigatorCall(t *testing.T) {
	expr := parseExpr(t, "nil&.unknown")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if !call.Safe {
		t.Fatal("expected safe method call")
	}
	if call.Method.Value != "unknown" {
		t.Fatalf("expected unknown method, got %s", call.Method.Value)
	}
}

func TestParseSafeNavigatorAndAssign(t *testing.T) {
	parse(t, "(obj&.m &&= false).should == false")
}

func TestParseTopLevelConstantResolution(t *testing.T) {
	parse(t, "::Private::G.new")
}

func TestParseModuleWithTopLevelConstantName(t *testing.T) {
	parse(t, "module ::Private\nend")
}

func TestParseLeadingDotContinuation(t *testing.T) {
	parse(t, `"abc".match(/a/)
  .to_a.should == ["a"]`)
}

func TestParseImplicitBeginEnsureInBlock(t *testing.T) {
	program := parse(t, `it "x" do
  $SAFE = 42
ensure
  $SAFE = nil
end`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseImplicitBeginEnsureInDef(t *testing.T) {
	parse(t, `def two
  yield
ensure
  ScratchPad << :two_ensure
end`)
}

func TestParseSuperWithBraceBlock(t *testing.T) {
	parse(t, `super { break 1 }`)
}

func TestParseClassWithMultipleMethodsAndImplicitEnsure(t *testing.T) {
	parse(t, `class BreakTest2
  def one
    two { yield }
  end

  def two
    yield
  ensure
    ScratchPad << :two_ensure
  end

  def three
    begin
      one { break }
      ScratchPad << :three_post
    ensure
      ScratchPad << :three_ensure
    end
  end
end`)
}

func TestParseClassWithNonConstantQualifiedName(t *testing.T) {
	parse(t, `class nil::Foo
end`)
}

func TestParseClassWithExpressionSuperclass(t *testing.T) {
	parse(t, `class TestClass < Module.new
end`)
}

func TestParseClassExpressionWithNestedSingletonClassAndTrailingCallInBlock(t *testing.T) {
	parse(t, `describe "x" do
  it "returns" do
    class ClassSpecs::Singleton; class << self; :singleton; end; end.should == :singleton
  end
end`)
}

func TestParseModuleWithVariableQualifiedConstantName(t *testing.T) {
	parse(t, `m = Module.new
module m::N; end`)
}

func TestParseImplicitBeginRescueVariableInDef(t *testing.T) {
	parse(t, `def a
  raise "message"
rescue => e
  ScratchPad << e.message
end`)
}

func TestParseImplicitBeginRescueVariableInClass(t *testing.T) {
	parse(t, `class RescueSpecs::C
  raise "message"
rescue => e
  ScratchPad << e.message
end`)
}

func TestParseArrayLiteralAtEndOfExpression(t *testing.T) {
	parse(t, `[:caught, :caught]`)
}

func TestParseMethodDefinitionWithDefaultArgument(t *testing.T) {
	program := parse(t, `def foo(a = 1)
  a
end`)
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if len(defn.Params) != 1 || defn.Params[0].Value != "a" {
		t.Fatalf("expected positional parameter a, got %#v", defn.Params)
	}
	if len(defn.ParamDefaults) != 1 || defn.ParamDefaults[0] == nil {
		t.Fatalf("expected default value for a, got %#v", defn.ParamDefaults)
	}
}

func TestParseMethodDefinitionWithConstantReceiver(t *testing.T) {
	parse(t, `def TARGET.defs_method
  self
end`)
}

func TestParseMultiAssignWithIndexTargets(t *testing.T) {
	parse(t, `object[:a], object[:b] = :a, :b`)
}

func TestParseMultiAssignWithGroupedAccessorTargets(t *testing.T) {
	parse(t, `(object.a, object.b), c = [:a, :b], nil`)
}

func TestParseMultiAssignWithNestedTargetsAcrossLines(t *testing.T) {
	parse(t, `(ScratchPad << :a; o).a,
  ((ScratchPad << :b; o).b,
  ((ScratchPad << :c; o).c, (ScratchPad << :d; o).d),
  (ScratchPad << :e; o).e),
(ScratchPad << :f; o).f = (ScratchPad << :value; :value)`)
}

func TestParseMultiAssignWithTrailingCommaBeforeAssign(t *testing.T) {
	parse(t, `a, = 1
b, c, = []`)
}

func TestParseGroupedAnonymousSplatAssignment(t *testing.T) {
	parse(t, `(* = 1).should == 1`)
}

func TestParseGroupedMultiAssignExpressionWithSplat(t *testing.T) {
	parse(t, `(a, *b, (c, d) = 1, 2, 3, *x).should == [1, 2, 3, 4, 5]`)
}

func TestParseDoBlockRescueComparedToArrayLiteral(t *testing.T) {
	parse(t, `[->{raise ArbitraryException}, ->{raise SpecificExampleException}].map do |block|
  begin
    block.call
  rescue SpecificExampleException, ArbitraryException
    :caught
  end
end.should == [:caught, :caught]`)
}

func TestParseRescueWithParenthesizedRaiseExpression(t *testing.T) {
	parse(t, `begin
  raise "from block"
rescue (raise "from rescue expression")
end`)
}

func TestParseMultiAssignWithInlineRescueValue(t *testing.T) {
	parse(t, `a, b = raise rescue [1, 2]`)
}

func TestParseBareCallArgumentsAcrossNewlineAfterComma(t *testing.T) {
	parse(t, `assert_equal "__#{safe_char}_",
             ERB::Util.xml_name_escape("#{unsafe_char * 2}#{safe_char}#{unsafe_char}")`)
}

func TestParseBareCallArgumentStartingWithConstant(t *testing.T) {
	parse(t, `assert_equal Date.current + 1, Date.tomorrow`)
}

func TestParseMultilineLambdaWithTrailingCall(t *testing.T) {
	parse(t, `-> {
  h = {a: 2, b: 3, c: 1}
  @h = eval "{a: 1, **h, c: 3}"
}.should_not complain`)
}

func TestParseImplicitBeginRescueInBlock(t *testing.T) {
	parse(t, `Fiber.new do
  raise "hi"
rescue
  Fiber.yield
end.resume`)
}

func TestParseYieldWithParenthesizedArguments(t *testing.T) {
	program := parse(t, `def m(a, b, c)
  yield(a, b, c)
end`)
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	stmt := defn.Body.Statements[0].(*ast.ExpressionStatement)
	yield := stmt.Expression.(*ast.YieldExpression)

	if len(yield.Args) != 3 {
		t.Fatalf("expected 3 yield args, got %d", len(yield.Args))
	}
}

func TestParseYieldWithTrailingCall(t *testing.T) {
	parse(t, "yield.should == expected")
}

func TestParseYieldWithParenthesizedSplatAndKeywordArgument(t *testing.T) {
	program := parse(t, `def k(a)
  yield(*a, b: true)
end`)
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	stmt := defn.Body.Statements[0].(*ast.ExpressionStatement)
	yield := stmt.Expression.(*ast.YieldExpression)

	if len(yield.Args) != 1 {
		t.Fatalf("expected 1 yield arg, got %d", len(yield.Args))
	}
	if len(yield.KeywordArgs) != 1 {
		t.Fatalf("expected 1 yield keyword arg, got %d", len(yield.KeywordArgs))
	}
}

func TestParseBlockPassLambdaArgument(t *testing.T) {
	parse(t, "@y.s([], &-> *a { a })\n")
}

func TestParseBlockPassLambdaArgumentWithMultipleParams(t *testing.T) {
	parse(t, `@y.s(1, &-> a, b { [a, b] })`)
}

func TestParseMethodBodyAfterLambdaLiteral(t *testing.T) {
	program := parse(t, `def make_value
  p = -> { 42 }
  p.call
end`)
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if len(defn.Body.Statements) != 2 {
		t.Fatalf("expected 2 body statements, got %d: %s", len(defn.Body.Statements), defn.Body.String())
	}
}

func TestParseBareMethodCallWithTrailingBlock(t *testing.T) {
	program := parse(t, `call_proc { x + 1 }`)
	call, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", program.Statements[0])
	}
	if call.Method.Value != "call_proc" || call.Block == nil {
		t.Fatalf("expected call_proc with trailing block, got %s", call.String())
	}
}

func TestParseBlockWithEmptyPipes(t *testing.T) {
	parse(t, "@y.z { || 1 }")
}

func TestParseBlockWithDestructuredParameters(t *testing.T) {
	parse(t, "@y.m([[1, 2, 3], 4]) { |(_, a, _), _| a }")
	parse(t, "@y.m([1, [2, 3, 4]]) { |_, (_, a, _)| a }")
	parse(t, "@y.m([[1, 2, 3], 4]) { |(_, a, _), _| a }.should == 2")
}

func TestParseGroupedCommaSequence(t *testing.T) {
	expr := parseExpr(t, "(_, a, _)")
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
}

func TestParseAnonymousBlockForwardingParameter(t *testing.T) {
	parse(t, "def pos_kwrest(arg1, **kw, &); inner(&); end")
}

func TestParseChainedCallAfterBlockPassLambdaArgument(t *testing.T) {
	parse(t, "@y.s([], &-> *a { a }).should == [[]]\n")
}

func TestParseTernaryConsequentMethodCall(t *testing.T) {
	parse(t, `default = (@method == :locale) ? Encoding.find("locale") : Encoding::UTF_8`)
}

func TestParseGroupedBreakWhileModifierWithTrailingCall(t *testing.T) {
	parse(t, "(break while true).should == nil")
}

func TestParseBreakIfWithGroupedCondition(t *testing.T) {
	parse(t, "break if (i += 1) >= 5")
}

func TestParseNextWithMultipleValues(t *testing.T) {
	program := parse(t, "next 1, 2, 3")
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	next, ok := stmt.Expression.(*ast.NextExpression)
	if !ok {
		t.Fatalf("expected NextExpression, got %T", stmt.Expression)
	}
	value, ok := next.Value.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral next value, got %T", next.Value)
	}
	if len(value.Elements) != 3 {
		t.Fatalf("expected 3 next values, got %d", len(value.Elements))
	}
}

func TestParseSplatAssignmentTarget(t *testing.T) {
	parse(t, "*a = yield()")
}

func TestParseMultiAssignmentWithSplatTarget(t *testing.T) {
	parse(t, "a, b, *c = yield()")
}

func TestParseConsecutiveNestedDestructuredBlockParams(t *testing.T) {
	parse(t, `describe "taking nested |a, ((b, c), d)|" do
  it "destructures" do
    @y.m { |a, ((b, c), d)| [a, b, c, d] }.should == [nil, nil, nil, nil]
    @y.m(1, 2) { |a, ((b, c), d)| [a, b, c, d] }.should == [1, 2, nil, nil]
    @y.m(1, [2, 3]) { |a, ((b, c), d)| [a, b, c, d] }.should == [1, 2, nil, 3]
    @y.m(1, [[2, 3], 4]) { |a, ((b, c), d)| [a, b, c, d] }.should == [1, 2, 3, 4]
  end
end

describe "arguments with _" do
  describe "taking |*a, b:|" do
    it "merges the hash into the splatted array" do
      @y.k { |*a, b:| [a, b] }.should == [[], true]
    end
  end

  it "extracts arguments with _" do
    @y.m([[1, 2, 3], 4]) { |(_, a, _), _| a }.should == 2
    @y.m([1, [2, 3, 4]]) { |_, (_, a, _)| a }.should == 3
  end

  it "assigns the first variable named" do
    @y.m(1, 2) { |_, _| _ }.should == 1
  end
end`)
}

func TestParseBlockPassChainedMethodCallArgument(t *testing.T) {
	parse(t, "m(*args, &args.pop).should == [[1, nil], nil]")
}

func TestParseBracketAssignmentWithSplatAndPostArgs(t *testing.T) {
	parse(t, "@obj[1,*@ary,123] = 2")
}

func TestParseBareCallHashRocketArgsAcrossNewline(t *testing.T) {
	parse(t, `specs.fooM3 'abc', 456, 'rbx' => 'cool',
      'specs' => 'fail sometimes', 'oh' => 'weh'`)
}

func TestParseCallKeywordArgsWithTrailingComma(t *testing.T) {
	parse(t, "specs.fooM1(rbx: 'cool', specs: :fail_sometimes, non_sym: 1234,).should == []")
}

func TestParseForExpressionWithDestructuredTargets(t *testing.T) {
	parse(t, `for i, *j, k in [[1, 2, 3]]
  i
end`)
}

func TestParseForExpressionWithVariableAndWriterTargets(t *testing.T) {
	parse(t, `for @var in m
end
for arr[1] in m
end
for ofor.target in m
end`)
}

func TestParseForwardArgumentsCall(t *testing.T) {
	parse(t, "bar(...)")
}

func TestParseMatchOperatorMethodDefinition(t *testing.T) {
	parse(t, `class FalseClass
  def =~(o)
    o == false
  end
end`)
}

func TestParseDefinedWithoutParentheses(t *testing.T) {
	parse(t, `(defined? a = 10).should == "assignment"`)
	parse(t, `(not defined? qqq).should == true`)
}

func TestParseKeywordLiteralMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `VariablesSpecs.false`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "false" {
		t.Fatalf("expected false method name, got %#v", call.Method)
	}
}

func TestParseBitwiseAndShiftCompoundAssignments(t *testing.T) {
	parse(t, `a |= b
a &= b
a ^= b
a >>= b
a <<= b`)
}

func TestParseRightwardPatternAssignment(t *testing.T) {
	parse(t, `[0, 1] => [a, b]`)
	parse(t, `{ a: 0, b: 1 } => { a:, b: }`)
}

func TestParseOneLinePatternMatch(t *testing.T) {
	parse(t, `([0, 1] in [a, b]).should == true`)
	parse(t, `({ a: 0, b: 1 } in { a:, b: }).should == true`)
}

func TestParseCaseInPatternClauses(t *testing.T) {
	parse(t, `case [0, 1, 2, 3]
in [*pre, 2, 3]
  pre
else
  false
end.should == [0, 1]`)
	parse(t, `case 0
in (
  -1..1)
  true
end.should == true`)
}

// === Function Call (was: infinite loop + panic) ===

func TestParseCallNoArgs(t *testing.T) {
	expr := parseExpr(t, "puts()")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "puts" {
		t.Errorf("expected puts, got %s", call.Method.Value)
	}
	if len(call.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(call.Args))
	}
}

func TestParseCallWithArgs(t *testing.T) {
	expr := parseExpr(t, "puts(1, 2)")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "puts" {
		t.Errorf("expected puts, got %s", call.Method.Value)
	}
	if len(call.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(call.Args))
	}
}

// === Method Call with Args (was: infinite loop + wrong expectPeek) ===

func TestParseMethodCallWithArgs(t *testing.T) {
	expr := parseExpr(t, `"hello".slice(0, 3)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "slice" {
		t.Errorf("expected slice, got %s", call.Method.Value)
	}
	if len(call.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(call.Args))
	}
}

func TestParseMethodCallWithKeywordName(t *testing.T) {
	expr := parseExpr(t, `[2, 3].prepend(1)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "prepend" {
		t.Errorf("expected prepend, got %s", call.Method.Value)
	}
	if len(call.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(call.Args))
	}
}

// === Prefix minus (regression test) ===

func TestParsePrefixMinusExpression(t *testing.T) {
	expr := parseExpr(t, "-5")
	prefix, ok := expr.(*ast.PrefixExpression)
	if !ok {
		t.Fatalf("expected PrefixExpression, got %T", expr)
	}
	if prefix.Operator != "-" {
		t.Errorf("expected -, got %s", prefix.Operator)
	}
	assertIntLit(t, prefix.Right, 5)
}
