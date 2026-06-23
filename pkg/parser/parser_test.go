package parser

import (
	"os"
	"strings"
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

func parseWithErrors(input string) []string {
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()
	return p.Errors()
}

func assertContainsError(t *testing.T, errs []string, substr string) {
	t.Helper()
	for _, err := range errs {
		if strings.Contains(err, substr) {
			return
		}
	}
	t.Fatalf("expected parse error containing %q, got %v", substr, errs)
}

func TestParseLambdaRejectsDuplicateNamedParameters(t *testing.T) {
	errs := parseWithErrors("->(x, x) {}")
	assertContainsError(t, errs, "duplicate")
}

func TestParseLambdaAllowsDuplicateUnderscoreParameters(t *testing.T) {
	errs := parseWithErrors("->(_, _) {}")
	if len(errs) > 0 {
		t.Fatalf("expected duplicate underscore lambda parameters to parse, got %v", errs)
	}
}

func TestParseBlockRequiredKeywordParameters(t *testing.T) {
	errs := parseWithErrors(`-> {
  m([1, 2]) { |a, b:, c:| [a, b, c] }
}.should raise_error(ArgumentError, "missing keywords: :b, :c")`)
	if len(errs) > 0 {
		t.Fatalf("expected block required keyword parameters to parse, got %v", errs)
	}
}

func TestParseBlockKeywordParametersAfterDefaultKeywordParameters(t *testing.T) {
	errs := parseWithErrors(`m([1, 2]) { |a, b: :b, c: :c| [a, b, c] }.should == [[1, 2], :b, :c]
-> {
  m([1, 2]) { |a, b:, c:| [a, b, c] }
}.should raise_error(ArgumentError, "missing keywords: :b, :c")`)
	if len(errs) > 0 {
		t.Fatalf("expected consecutive block keyword parameters to parse, got %v", errs)
	}
}

func TestParseHashSyntaxRejectsInvalidBangAndQuestionLabels(t *testing.T) {
	for _, input := range []string{
		`{:a!=> 1}`,
		`{:a?=> 1}`,
		`{a!:}`,
		`{a?:}`,
	} {
		t.Run(input, func(t *testing.T) {
			errs := parseWithErrors(input)
			if len(errs) == 0 {
				t.Fatal("expected parse errors")
			}
		})
	}
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

func TestParseCatchWithBraceBlockAndTrailingNewline(t *testing.T) {
	parse(t, "catch(:out) { 1 }\n")
}

func TestParseCatchWithoutBlockInLambdaCallChain(t *testing.T) {
	parse(t, `-> { catch :blah }.should raise_error(LocalJumpError)`)
}

func TestParseDefinedWithThrowCallChain(t *testing.T) {
	parse(t, `defined?(throw(:out, 42).foo).should == :unreachable`)
}

func TestParseThrowWithBareSecondArgument(t *testing.T) {
	parse(t, `throw :exit, :msg`)
	parse(t, `catch(1) { throw 1, 2 }.should == 2`)
}

func TestParseThrowWithTooManyBareArguments(t *testing.T) {
	parse(t, `throw :exit, :msg, 1`)
	parse(t, `catch(:exit) { throw :exit, :msg, 1, 2, 3 }`)
}

func TestParseKeywordMatcherWithMultilineArguments(t *testing.T) {
	parse(t, `list.should include(
  :a, :b, :c)`)
}

func TestParseShouldWithParenthesizedBlockMatcher(t *testing.T) {
	parse(t, `-> {
  require_relative(path)
}.should(raise_error(LoadError) { |e|
  e.path.should == File.expand_path(path, @abs_dir)
})`)
}

func TestParseBlockWithShouldParenthesizedBlockMatcher(t *testing.T) {
	parse(t, `it "sets path" do
  -> {
    require_relative(path)
  }.should(raise_error(LoadError) { |e|
    e.path.should == File.expand_path(path, @abs_dir)
  })
end`)
}

func TestParseBlockCallArgumentWithBlockAndOuterCallChain(t *testing.T) {
	parse(t, `it "respects Hash default" do
  @method.call("%{foo}", Hash.new { nil }).should == "123"
end`)
}

func TestParseParenthesizedChainedCallAssignment(t *testing.T) {
	parse(t, `a = ("".encode(Encoding::US_ASCII).send(@method, 128))`)
}

func TestParseParenthesizedBlockCallReceiverChain(t *testing.T) {
	parse(t, `(s.each_byte {}).should equal(s)`)
}

func TestParseParenthesizedBlockCallReceiverChainInsideBlock(t *testing.T) {
	parse(t, `it "returns self" do
  s = "hello"
  (s.each_byte {}).should equal(s)
end`)
}

func TestParseParenthesizedSendBlockCallReceiverChainInsideBlock(t *testing.T) {
	parse(t, `it "returns self" do
  s = "hello"
  (s.send(@method) {}).should equal(s)
end`)
}

func TestParseLambdaWithCallArgumentAndEmptyBlock(t *testing.T) {
	parse(t, `-> { "hello world".send(@method, false) {} }.should raise_error(TypeError)`)
}

func TestParseNestedBlockCallChainWithSymbolProcArgument(t *testing.T) {
	parse(t, `10.times.map { Thread.new { x = nil; 100.times { x = p.call }; x } }.map(&:value)`)
}

func TestParseBareBlockCallChain(t *testing.T) {
	parse(t, `loop { e.next }.should == :stopped`)
}

func TestParseYieldAsBareCallArgument(t *testing.T) {
	program := parse(t, `def yielding
  note yield
end`)
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
	if def.Body == nil || len(def.Body.Statements) != 1 {
		t.Fatalf("expected one body statement, got %#v", def.Body)
	}
	bodyStmt, ok := def.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected body ExpressionStatement, got %T", def.Body.Statements[0])
	}
	call, ok := bodyStmt.Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", bodyStmt.Expression)
	}
	if call.Method == nil || call.Method.Value != "note" || len(call.Args) != 1 {
		t.Fatalf("expected note(yield), got %s", call.String())
	}
	if _, ok := call.Args[0].(*ast.YieldExpression); !ok {
		t.Fatalf("expected YieldExpression argument, got %T", call.Args[0])
	}
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

func TestParseGroupedMethodChainAfterNestedParenArgument(t *testing.T) {
	parse(t, "(~(-bignum_value(21))).should == 2")
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

func TestParseTernaryWithNewlineAlternative(t *testing.T) {
	parse(t, "true ? 1 :\n 2")
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

func TestParseConstantBracketCallWithNestedArrayArguments(t *testing.T) {
	parse(t, `Set[[:b, 2], [:a, 1]]`)
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

func TestParseRaiseWithPostfixUnless(t *testing.T) {
	parse(t, `raise "subprocesses leaked" unless leaked.empty?`)
}

func TestParseRaiseUnlessModifierDoesNotConsumeFollowingBlockStatements(t *testing.T) {
	program := parse(t, `enum.each do |*args|
  raise "bad" unless args.empty?
  cnt += 1
  break cnt if cnt >= 42
end`)

	stmt := program.Statements[0].(*ast.ExpressionStatement)
	call := stmt.Expression.(*ast.MethodCall)
	if call.Block == nil {
		t.Fatal("expected method call block")
	}
	if got := len(call.Block.Statements); got != 3 {
		t.Fatalf("expected 3 block statements, got %d: %s", got, call.Block.String())
	}
	if _, ok := call.Block.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.IfExpression); !ok {
		t.Fatalf("expected first block statement to be IfExpression, got %T", call.Block.Statements[0])
	}
}

func TestParseIfWithBeginEnsureBeforeElse(t *testing.T) {
	parse(t, `if io
  begin
    io.gets
  ensure
    io.close
  end
else
  puts "child"
end`)
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

func TestParseThreadCurrentStorageIndexAssignment(t *testing.T) {
	parse(t, `Thread.current[:wait_for] = t2`)
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

func TestParseAnonymousParameterSymbolArrays(t *testing.T) {
	parse(t, `[[:rest, :*], [:keyrest, :**], [:block, :&]]`)
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

func TestParseSymbolInspectLiteralHash(t *testing.T) {
	parse(t, `{
  :$-w => ":$-w",
  :"\<\<" => ":\<\<",
  :"\$" => ":\"$\"",
  :[] => ":[]",
  :[]= => ":[]=",
  :"ê" => [":ê", ":\"\\u00EA\""]
}`)
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

func TestParseUnicodeInstanceVariableSymbolArgument(t *testing.T) {
	parse(t, `obj.instance_variable_set(:@💙, 42)`)
	parse(t, `obj.instance_variable_get(:@💙).should == 42`)
}

func TestParseSpecialGlobalVariableSymbolArgument(t *testing.T) {
	parse(t, `bind.local_variable_get(:$~)`)
	parse(t, `bind.local_variable_set(:$_, "")`)
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

func TestParseSingletonEqualityMethodDefinitionWithLiteralBody(t *testing.T) {
	parse(t, `def x.==(o) false end`)
}

func TestParseMethodDefinitionWithBareParameter(t *testing.T) {
	parse(t, `def eql? other
  ByValueKey === other and @n == other.n
end`)
	parse(t, `class ByValueKey
  def eql? other
    ByValueKey === other and @n == other.n
  end
end`)
}

func TestParseHashFixturesClasses(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/hash/fixtures/classes.rb")
	if err != nil {
		t.Fatal(err)
	}

	parse(t, string(input))
}

func TestParseHashEqualValueSpecWithRequires(t *testing.T) {
	paths := []string{
		"../../vendor/ruby/spec/spec_helper.rb",
		"../../vendor/ruby/spec/core/hash/fixtures/classes.rb",
		"../../vendor/ruby/spec/core/hash/shared/eql.rb",
		"../../vendor/ruby/spec/core/hash/equal_value_spec.rb",
	}
	var input string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		input += string(data) + "\n"
	}

	parse(t, input)
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

func TestParseBacktickMethodCall(t *testing.T) {
	expr := parseExpr(t, "Kernel.`(obj)")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "`" {
		t.Fatalf("expected backtick method name, got %#v", call.Method)
	}
}

func TestParseTernaryAlternativeSuper(t *testing.T) {
	parse(t, `def m
  cond ? [cond] : super
end`)
}

func TestParseConstantMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, "Kernel.Integer(10)")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "Integer" {
		t.Fatalf("expected Integer method name, got %#v", call.Method)
	}
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

func TestParseUnparenthesizedMethodParameters(t *testing.T) {
	input := "class A\n  def respond_to_missing? method, bool\n    method.should == true\n    bool.should == false\n  end\nend"

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

func TestParseInstanceVariableSingletonKeywordMethodDefinition(t *testing.T) {
	parse(t, `def @obj.class
  Integer
end`)
}

func TestParseConstantLikeMethodDefinition(t *testing.T) {
	parse(t, `def Data?
  !self.Data.nil?
end`)
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

func TestParseLambdaWithInlineSingletonClassExpressionCallChain(t *testing.T) {
	parse(t, `-> { class << dup; CLONE; end }.should raise_error(NameError)`)
}

func TestParseLambdaWithInlineSingletonMethodDefinitionCallChain(t *testing.T) {
	parse(t, `-> { def obj.foo; end }.should raise_error(FrozenError)`)
}

func TestParseEvalHeredocCaseElseBeforeWhen(t *testing.T) {
	parse(t, `-> {
  eval <<-CODE
  case 4
  else
    true
  when 4; false
  end
  CODE
}.should raise_error(SyntaxError)`)
}

func TestParseComplainMatcherWithBacktickRegexp(t *testing.T) {
	parse(t, "-> { obj.foobar }.should complain(/warning: global variable [`']\\$specs_uninitialized_global_variable' not initialized/, verbose: true)")
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

func TestParseEndlessRangeIndexArgument(t *testing.T) {
	parse(t, `obj[3..].should == []`)
	parse(t, `/a/.match("a")[3..].should == []`)
}

func TestParseRangeWithMethodCallBounds(t *testing.T) {
	parse(t, `(RangeSpecs::TenfoldSucc.new(0)..RangeSpecs::TenfoldSucc.new(100)).should be_false`)
}

func TestParseEndlessRangeAssignment(t *testing.T) {
	parse(t, `range = (@x..)`)
}

func TestParseExclusiveEndlessRangeAssignment(t *testing.T) {
	parse(t, `range = (@x...)`)
}

func TestParseBeginlessRangeAssignment(t *testing.T) {
	parse(t, `range = (..@x)`)
}

func TestParseExclusiveBeginlessRangeAssignment(t *testing.T) {
	parse(t, `range = (...@x)`)
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

func TestParsePercentWordArrayLiteral(t *testing.T) {
	expr := parseExpr(t, `%w[alpha beta
gamma]`)
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		str, ok := arr.Elements[i].(*ast.StringLiteral)
		if !ok {
			t.Fatalf("element %d: expected StringLiteral, got %T", i, arr.Elements[i])
		}
		if str.Value != want {
			t.Fatalf("element %d: expected %q, got %q", i, want, str.Value)
		}
	}
}

func TestParseArrayLiteralWithBlockCallAndLambdaElements(t *testing.T) {
	parse(t, `[Proc.new{}, -> {}, proc {}]`)
	parse(t, `[Proc.new{}, -> {}, proc {}].each { |p| p.binding }`)
}

func TestParseMultilineNestedArrayLiteral(t *testing.T) {
	parse(t, `[[:TEXT, "Posts: "],
  [:OPEN, "<%="],
  [:CODE, " @post.length "],
]`)
	parse(t, `assert_equal [[:TEXT, "Posts: "],
              [:OPEN, "<%="],
              [:CODE, " @post.length "],
], actual_tokens`)
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

func TestParseImaginaryNumericSuffix(t *testing.T) {
	parse(t, `(1+1i).should.finite?`)
	parse(t, `Complex.polar(1.0+0.0i, Math::PI/2+0.0i)`)
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

func TestParseStringLiteralWithHash(t *testing.T) {
	parse(t, "describe \"Fiber#kill\" do\nend")
}

func TestParseMatrixLikeMultilineIndexCall(t *testing.T) {
	parse(t, `Matrix[
	  [1, 2],
	  [3, 4],
	]`)
	parse(t, `@a.should == Matrix[
	 [16, 19],
	 [36, 43],
]`)
}

func TestParseMultilineMatrixSendChain(t *testing.T) {
	expr := parseExpr(t, `Matrix[
  [1, 3, 3],
  [1, 4, 3],
  [1, 3, 4]
].send(:det)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "send" {
		t.Fatalf("expected send method call, got %#v", call.Method)
	}
	if _, ok := call.Receiver.(*ast.MethodCall); !ok {
		t.Fatalf("expected method call receiver, got %T", call.Receiver)
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

func TestParseCaseWithElseWithoutWhenIsError(t *testing.T) {
	errs := parseWithErrors("case 4\nelse\n  true\nend")
	assertContainsError(t, errs, "expected at least one when clause in case statement")
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

func TestParsePercentRegexpLiteral(t *testing.T) {
	expr := parseExpr(t, `%r{runner/mspec.rb}i`)
	re, ok := expr.(*ast.RegexpLiteral)
	if !ok {
		t.Fatalf("expected RegexpLiteral, got %T", expr)
	}
	if re.Pattern != "runner/mspec.rb" {
		t.Errorf("expected pattern runner/mspec.rb, got %s", re.Pattern)
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

func TestParseLambdaWithAnonymousRestParameter(t *testing.T) {
	program := parse(t, `class BlockDefinitionScope
  def block_to_assign(value)
    -> * { @@cvar = value }
  end
end`)
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	classExpr, ok := stmt.Expression.(*ast.ClassExpression)
	if !ok {
		t.Fatalf("expected ClassExpression, got %T", stmt.Expression)
	}
	if len(classExpr.Body.Statements) != 1 {
		t.Fatalf("expected 1 class body statement, got %d", len(classExpr.Body.Statements))
	}
	defExpr, ok := classExpr.Body.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", classExpr.Body.Statements[0])
	}
	if len(defExpr.Body.Statements) != 1 {
		t.Fatalf("expected lambda in method body, got %d", len(defExpr.Body.Statements))
	}
	lambdaExpr, ok := defExpr.Body.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.ProcLiteral)
	if !ok {
		t.Fatalf("expected ProcLiteral, got %T", defExpr.Body.Statements[0])
	}
	if lambdaExpr.RestParam == nil || lambdaExpr.RestParam.Value != "_" {
		t.Fatalf("expected anonymous rest parameter, got %#v", lambdaExpr.RestParam)
	}
}

func TestParseRaiseIfModifierInsideSemicolonLambda(t *testing.T) {
	expr := parseExpr(t, `-> { times += 1; raise if times > 1; "done" }`)
	proc, ok := expr.(*ast.ProcLiteral)
	if !ok {
		t.Fatalf("expected ProcLiteral, got %T", expr)
	}
	if proc.Body == nil {
		t.Fatal("expected lambda body")
	}
	if len(proc.Body.Statements) != 3 {
		t.Fatalf("expected 3 lambda statements, got %d: %s", len(proc.Body.Statements), proc.Body.String())
	}
	if _, ok := proc.Body.Statements[1].(*ast.ExpressionStatement).Expression.(*ast.IfExpression); !ok {
		t.Fatalf("expected second statement to be if modifier, got %T", proc.Body.Statements[1])
	}
}

func TestParseReturnIfModifierInsideMethod(t *testing.T) {
	program := parse(t, `def obj.eql?(o)
  return true if self.equal?(o)
  false
end

describe "consumer" do
  it "runs" do
  end
end`)
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 top-level statements, got %d: %s", len(program.Statements), program.String())
	}
	def, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected first statement to be DefExpression, got %T", program.Statements[0])
	}
	if def.Body == nil || len(def.Body.Statements) != 2 {
		t.Fatalf("expected method body with 2 statements, got %#v", def.Body)
	}
	if _, ok := def.Body.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.IfExpression); !ok {
		t.Fatalf("expected first method statement to be if modifier, got %T", def.Body.Statements[0])
	}
}

func TestParseBareReturnIfModifierDoesNotConsumeFollowingStatement(t *testing.T) {
	program := parse(t, `describe :shared, shared: true do
  it "runs" do
    return if done?
    ScratchPad << :ran
  end
end

describe "consumer" do
end`)
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 top-level statements, got %d: %s", len(program.Statements), program.String())
	}
}

func TestParseIfModifierDoesNotCrossNewline(t *testing.T) {
	program := parse(t, `x = 1
if x
  2
end`)
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 top-level statements, got %d: %s", len(program.Statements), program.String())
	}
	if _, ok := program.Statements[0].(*ast.ExpressionStatement); !ok {
		t.Fatalf("expected first statement to be expression statement, got %T", program.Statements[0])
	}
	if exprStmt, ok := program.Statements[1].(*ast.ExpressionStatement); !ok || exprStmt.Expression == nil {
		t.Fatalf("expected second statement to be if expression statement, got %T", program.Statements[1])
	} else if _, ok := exprStmt.Expression.(*ast.IfExpression); !ok {
		t.Fatalf("expected second statement expression to be if expression, got %T", exprStmt.Expression)
	}
}

func TestParseUntilModifierInsideBraceBlockDoesNotConsumeFollowingStatement(t *testing.T) {
	expr := parseExpr(t, `Thread.new { Thread.pass until go; foo }`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Block == nil {
		t.Fatal("expected block")
	}
	if len(call.Block.Statements) != 2 {
		t.Fatalf("expected 2 block statements, got %d: %s", len(call.Block.Statements), call.Block.String())
	}
	if _, ok := call.Block.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.UntilExpression); !ok {
		t.Fatalf("expected first statement to be until modifier, got %T", call.Block.Statements[0])
	}
}

func TestParseIfElseWithAssignedLambdaConsequents(t *testing.T) {
	parse(t, `if RUBY_ENGINE == 'truffleruby'
  sclass = -> io { Primitive.singleton_class(io) }
else
  sclass = -> io { io.singleton_class }
end`)
}

func TestParseSingletonClassSpecIoContextSnippet(t *testing.T) {
	parse(t, `it "looks up singleton methods" do
  proxy = -> io { io.foo }
  if RUBY_ENGINE == 'truffleruby'
    sclass = -> io { Primitive.singleton_class(io) }
  else
    sclass = -> io { io.singleton_class }
  end

  io = File.new(__FILE__)
  io.define_singleton_method(:foo) { "old" }
ensure
  io.close
end`)
}

func TestParseBlockWithIfElseAssignedLambda(t *testing.T) {
	parse(t, `it "x" do
  if RUBY_ENGINE == 'truffleruby'
    sclass = -> io { Primitive.singleton_class(io) }
  else
    sclass = -> io { io.singleton_class }
  end
end`)
}

func TestParseBlockWithLeadingLambdaThenIfElseAssignedLambda(t *testing.T) {
	parse(t, `it "x" do
  proxy = -> io { io.foo }
  if RUBY_ENGINE == 'truffleruby'
    sclass = -> io { Primitive.singleton_class(io) }
  else
    sclass = -> io { io.singleton_class }
  end
end`)
}

func TestParseLeadingLambdaThenIfElseAssignedLambda(t *testing.T) {
	parse(t, `proxy = -> io { io.foo }
if RUBY_ENGINE == 'truffleruby'
  sclass = -> io { Primitive.singleton_class(io) }
else
  sclass = -> io { io.singleton_class }
end`)
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

func TestParseIndexArgumentArrayLiteralMethodCall(t *testing.T) {
	expr := parseExpr(t, `@h[[1].dup].should be_nil`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected outer MethodCall, got %T", expr)
	}
	idx, ok := call.Receiver.(*ast.IndexExpression)
	if !ok {
		t.Fatalf("expected receiver IndexExpression, got %T", call.Receiver)
	}
	arg, ok := idx.Index.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected index argument MethodCall, got %T", idx.Index)
	}
	if arg.Method == nil || arg.Method.Value != "dup" {
		t.Fatalf("expected dup index argument call, got %#v", arg.Method)
	}
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

func TestParseKeywordLogicalAnd(t *testing.T) {
	expr := parseExpr(t, "true and false")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "and" {
		t.Errorf("expected and, got %s", infix.Operator)
	}
}

func TestParseKeywordLogicalOr(t *testing.T) {
	expr := parseExpr(t, "true or false")
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "or" {
		t.Errorf("expected or, got %s", infix.Operator)
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

func TestParseAssignmentAsRightHandSideOfKeywordBooleanAnd(t *testing.T) {
	expr := parseExpr(t, "true and false and x = 1")
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

func TestParseKeywordAssignmentValueOr(t *testing.T) {
	expr := parseExpr(t, "x = true or false or y = 1")
	assign, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected AssignExpression, got %T", expr)
	}
	if assign.Name.Value != "x" {
		t.Fatalf("expected assignment to x, got %s", assign.Name.Value)
	}
	infix, ok := assign.Value.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected assignment value to be InfixExpression, got %T", assign.Value)
	}
	if infix.Operator != "or" {
		t.Errorf("expected or at top, got %s", infix.Operator)
	}

	assignExpr, ok := infix.Right.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected right side assignment, got %T", infix.Right)
	}
	if assignExpr.Name.Value != "y" {
		t.Fatalf("expected assignment to y, got %s", assignExpr.Name.Value)
	}
	assertIntLit(t, assignExpr.Value, 1)
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

func TestParseBareMethodCallWithArrayArgumentAndDoBlock(t *testing.T) {
	expr := parseExpr(t, `argf ["input.txt"] do
  value = true
end`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "argf" {
		t.Fatalf("expected argf, got %s", call.Method.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected ArrayLiteral argument, got %T", call.Args[0])
	}
	if call.Block == nil {
		t.Fatal("expected do block")
	}
}

func TestParseDottedBareMethodCallWithIdentifierArgumentAndDoBlock(t *testing.T) {
	expr := parseExpr(t, `Dir.chdir dir1 do
  Dir.chdir(dir2) { Dir.unlink dir1 }
end`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "chdir" {
		t.Fatalf("expected chdir, got %s", call.Method.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	if ident, ok := call.Args[0].(*ast.Identifier); !ok || ident.Value != "dir1" {
		t.Fatalf("expected dir1 identifier argument, got %T %#v", call.Args[0], call.Args[0])
	}
	if call.Block == nil {
		t.Fatal("expected do block")
	}
}

func TestParseDottedSendWithSymbolAndLocalArguments(t *testing.T) {
	expr := parseExpr(t, `Dir.send(:glob, pattern)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method.Value != "send" {
		t.Fatalf("expected send, got %s", call.Method.Value)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.SymbolLiteral); !ok {
		t.Fatalf("expected symbol first arg, got %T", call.Args[0])
	}
	if ident, ok := call.Args[1].(*ast.Identifier); !ok || ident.Value != "pattern" {
		t.Fatalf("expected pattern identifier second arg, got %T %#v", call.Args[1], call.Args[1])
	}
}

func TestParseDottedNoArgCallBeforeMinusAsInfix(t *testing.T) {
	expr := parseExpr(t, `Time.now - "1"`)
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", expr)
	}
	if infix.Operator != "-" {
		t.Fatalf("expected '-' operator, got %q", infix.Operator)
	}
	call, ok := infix.Left.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected left MethodCall, got %T", infix.Left)
	}
	if call.Method == nil || call.Method.Value != "now" || len(call.Args) != 0 {
		t.Fatalf("expected Time.now with no args, got %s args=%d", call.String(), len(call.Args))
	}
}

func TestParseCallSplatWithInfixArrayExpression(t *testing.T) {
	expr := parseExpr(t, `Time.send(:gm, *[0]*8)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(call.Args))
	}
	splat, ok := call.Args[1].(*ast.SplatExpression)
	if !ok {
		t.Fatalf("expected splat arg, got %T", call.Args[1])
	}
	if _, ok := splat.Value.(*ast.InfixExpression); !ok {
		t.Fatalf("expected splat value infix expression, got %T", splat.Value)
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

func TestParseHashShorthandReceiverWithMultiplePairs(t *testing.T) {
	parse(t, `{a: 1, b: 2}.all?(pattern).should == true`)
	parse(t, `pattern = EnumerableSpecs::Pattern.new { |x| Array === x }
{a: 1, b: 2}.all?(pattern).should == true`)
	parse(t, `it "x" do
  pattern = EnumerableSpecs::Pattern.new { |x| Array === x }
  {a: 1, b: 2}.all?(pattern).should == true
end`)
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

func TestParseHashRocketValueOnFollowingLine(t *testing.T) {
	parse(t, `{
	  @class_file =>
	    {lines: [1, 2, 3]}
	}`)
}

func TestParseGroupedHashEqualityExpression(t *testing.T) {
	parse(t, `({ 1 => l_val } == { 1 => r_val }).should be_true`)
}

func TestParseHashWithSignedNumericArrowKeys(t *testing.T) {
	parse(t, `{ -2.2 => -2, -0.1 => 0, 5.5 => 5 }`)
}

func TestParseHashRocketWithConstantResolutionCallKey(t *testing.T) {
	parse(t, `{ObjectSpaceFixtures::ObjectToBeFound.new(:hash_key) => :value}`)
}

func TestParseHashRocketWithMethodCallKey(t *testing.T) {
	parse(t, `{
  1.minute => 60,
  1.hour + 15.minutes => 4500
}`)
}

func TestParseMethodCallBlockWithUnaryMinusExpression(t *testing.T) {
	parse(t, `(@x...@y).minmax { |x, y| - (x <=> y) }.should == [@x, @x]`)
}

func TestParseInclusiveMethodCallBlockWithUnaryMinusExpression(t *testing.T) {
	parse(t, `(@x..@y).minmax { |x, y| - (x <=> y) }.should == [@y, @x]`)
}

func TestParseCallWithRegexpArgumentAfterCommaNewline(t *testing.T) {
	parse(t, `-> { range.minmax }.should raise_error(RangeError,
  /cannot get the maximum of beginless range with custom comparison method|cannot get the minimum of beginless range/)`)
}

func TestParseBareDoBlockWithEndlessRangeAssignment(t *testing.T) {
	parse(t, `foo do
  range = (@x..)
end`)
}

func TestParseRangeMinmaxSpec(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/range/minmax_spec.rb")
	if err != nil {
		t.Fatal(err)
	}

	parse(t, string(input))
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

func TestParsePercentMethodCall(t *testing.T) {
	expr := parseExpr(t, `obj.%(1)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "%" {
		t.Fatalf("expected %% method, got %v", call.Method)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	assertIntLit(t, call.Args[0], 1)
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

func TestParseSafeNavigatorCompoundAssignmentReadsCurrentValue(t *testing.T) {
	expr := parseExpr(t, "obj&.foo += 3")
	setter, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected setter MethodCall, got %T", expr)
	}
	if !setter.Safe || setter.Method.Value != "foo=" || len(setter.Args) != 1 {
		t.Fatalf("expected safe foo= call with one arg, got %#v", setter)
	}
	infix, ok := setter.Args[0].(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected setter arg to read and add current value, got %T", setter.Args[0])
	}
	getter, ok := infix.Left.(*ast.MethodCall)
	if !ok || !getter.Safe || getter.Method.Value != "foo" || infix.Operator != "+" {
		t.Fatalf("expected safe foo getter plus RHS, got %#v", infix)
	}
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

func TestParseSuperWithEmptyParenthesesTerminates(t *testing.T) {
	result := make(chan []string, 1)

	go func() {
		l := lexer.New("super()")
		p := New(l)
		p.ParseProgram()
		result <- p.Errors()
	}()

	select {
	case errors := <-result:
		if len(errors) > 0 {
			t.Fatalf("parse errors: %v", errors)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("super() parse did not terminate")
	}
}

func TestParseSuperWithParenthesizedArgumentsTerminates(t *testing.T) {
	result := make(chan []string, 1)

	go func() {
		l := lexer.New("super(1 + 2)")
		p := New(l)
		p.ParseProgram()
		result <- p.Errors()
	}()

	select {
	case errors := <-result:
		if len(errors) > 0 {
			t.Fatalf("parse errors: %v", errors)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("super with parenthesized arguments parse did not terminate")
	}
}

func TestParseReturnWithoutValueBeforeBrace(t *testing.T) {
	program := parse(t, `class A
  1.times { return }
end`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
}

func TestParseSuperWithBareArgumentTerminates(t *testing.T) {
	result := make(chan *ast.Program, 1)
	errors := make(chan []string, 1)

	go func() {
		l := lexer.New("def require(name)\n  super name\nend")
		p := New(l)
		program := p.ParseProgram()
		errors <- p.Errors()
		result <- program
	}()

	select {
	case parseErrors := <-errors:
		if len(parseErrors) > 0 {
			t.Fatalf("parse errors: %v", parseErrors)
		}
		program := <-result
		if len(program.Statements) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(program.Statements))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("super with bare argument parse did not terminate")
	}
}

func TestParseBareSuperWithInfixOperatorTerminates(t *testing.T) {
	expr := parseExpr(t, `super + 1`)
	if _, ok := expr.(*ast.InfixExpression); !ok {
		t.Fatalf("expected bare super operator call to parse as InfixExpression, got %T", expr)
	}
}

func TestParseBareSuperAtArrayElementEndTerminates(t *testing.T) {
	parse(t, `["m", super]`)
}

func TestParseBareSuperWithShiftOperatorTerminates(t *testing.T) {
	expr := parseExpr(t, `super << :m1`)
	if _, ok := expr.(*ast.InfixExpression); !ok {
		t.Fatalf("expected bare super shift call to parse as InfixExpression, got %T", expr)
	}
}

func TestParseBareYieldWithInfixOperator(t *testing.T) {
	program := parse(t, `while yield == :retry
  :ok
end`)
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}
	whileExpr, ok := stmt.Expression.(*ast.WhileExpression)
	if !ok {
		t.Fatalf("expected WhileExpression, got %T", stmt.Expression)
	}
	if _, ok := whileExpr.Condition.(*ast.InfixExpression); !ok {
		t.Fatalf("expected yield comparison condition, got %T", whileExpr.Condition)
	}
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

func TestParseRaiseAsDottedMethodName(t *testing.T) {
	expr := parseExpr(t, `Object.new.raise("message", {cause: RuntimeError.new})`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "raise" {
		t.Fatalf("expected dotted method raise, got %#v", call.Method)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(call.Args))
	}
}

func TestParseRaiseAsDottedMethodNameInLambda(t *testing.T) {
	expr := parseExpr(t, `-> { Object.new.raise("message", {cause: RuntimeError.new}) }`)
	lambda, ok := expr.(*ast.ProcLiteral)
	if !ok {
		t.Fatalf("expected ProcLiteral, got %T", expr)
	}
	if len(lambda.Body.Statements) != 1 {
		t.Fatalf("expected 1 lambda body statement, got %d", len(lambda.Body.Statements))
	}
	stmt, ok := lambda.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", lambda.Body.Statements[0])
	}
	call, ok := stmt.Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", stmt.Expression)
	}
	if call.Method == nil || call.Method.Value != "raise" {
		t.Fatalf("expected dotted method raise, got %#v", call.Method)
	}
}

func TestParseRaiseAsDottedMethodNameOnInstanceVariableInLambda(t *testing.T) {
	expr := parseExpr(t, `-> { @object.raise("message", {cause: RuntimeError.new}) }`)
	lambda, ok := expr.(*ast.ProcLiteral)
	if !ok {
		t.Fatalf("expected ProcLiteral, got %T", expr)
	}
	if len(lambda.Body.Statements) != 1 {
		t.Fatalf("expected 1 lambda body statement, got %d", len(lambda.Body.Statements))
	}
	stmt, ok := lambda.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", lambda.Body.Statements[0])
	}
	call, ok := stmt.Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", stmt.Expression)
	}
	if call.Method == nil || call.Method.Value != "raise" {
		t.Fatalf("expected dotted method raise, got %#v", call.Method)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(call.Args))
	}
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

func TestParseMultiAssignWithNestedGroupedTargets(t *testing.T) {
	expr := parseExpr(t, `((a, b), c), (d, (e,), (f, (g, h))) = 1`)
	if _, ok := expr.(*ast.MultiAssignExpression); !ok {
		t.Fatalf("expected MultiAssignExpression, got %T", expr)
	}
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

func TestParseNestedGroupedAnonymousSplatAssignmentCallChain(t *testing.T) {
	parse(t, `((*) = *1).should == [1]`)
}

func TestParseGroupedRescueModifierCallChain(t *testing.T) {
	program := parse(t, `((raise until true and false) rescue 10).should == 10`)

	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}

	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}

	infix, ok := stmt.Expression.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", stmt.Expression)
	}

	if infix.Operator != "==" {
		t.Fatalf("expected infix operator ==, got %q", infix.Operator)
	}

	call, ok := infix.Left.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected left side to be MethodCall, got %T", infix.Left)
	}

	if call.Method == nil || call.Method.Value != "should" {
		t.Fatalf("expected method call to `should`, got %#v", call.Method)
	}
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

func TestParseRescueWithLiteralSplatArray(t *testing.T) {
	parse(t, `begin
  raise FirstError
rescue FirstError, *[SecondError]
  :caught
end`)
}

func TestParseRescueExceptionWithVariableBinding(t *testing.T) {
	expr := parseExpr(t, `begin
  raise "boom"
rescue RuntimeError => e
  e.message
end`)
	beginExpr, ok := expr.(*ast.BeginExpression)
	if !ok {
		t.Fatalf("expected BeginExpression, got %T", expr)
	}
	if len(beginExpr.Rescue) != 1 {
		t.Fatalf("expected one rescue clause, got %d", len(beginExpr.Rescue))
	}
	rescue := beginExpr.Rescue[0]
	if len(rescue.Exceptions) != 1 {
		t.Fatalf("expected one rescue exception, got %d", len(rescue.Exceptions))
	}
	if _, ok := rescue.Exceptions[0].(*ast.Constant); !ok {
		t.Fatalf("expected rescue exception constant, got %T", rescue.Exceptions[0])
	}
	if rescue.Variable == nil || rescue.Variable.Value != "e" {
		t.Fatalf("expected rescue variable e, got %#v", rescue.Variable)
	}
}

func TestParseMultiAssignWithInlineRescueValue(t *testing.T) {
	parse(t, `a, b = raise rescue [1, 2]`)
}

func TestParseMultiAssignValueWithBlockCall(t *testing.T) {
	parse(t, `thread, line = Thread.new { "hello" }, __LINE__`)
	parse(t, `describe :thread_to_s, shared: true do
  it "returns a description including file and line number" do
    thread, line = Thread.new { "hello" }, __LINE__
    thread.join
  end
end`)
}

func TestParseBareCallArgumentsAcrossNewlineAfterComma(t *testing.T) {
	parse(t, `assert_equal "__#{safe_char}_",
             ERB::Util.xml_name_escape("#{unsafe_char * 2}#{safe_char}#{unsafe_char}")`)
}

func TestParseReceiverBareCallWithMultipleArguments(t *testing.T) {
	parse(t, `Regexp.send @method, /hi/, Regexp::IGNORECASE`)
}

func TestParseReceiverBareCallWithArrayFirstArgumentAndMoreArguments(t *testing.T) {
	expr := parseExpr(t, `IO.select [rd], nil, nil, 0`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "select" {
		t.Fatalf("expected select method call, got %v", call.Method)
	}
	if len(call.Args) != 4 {
		t.Fatalf("expected 4 args, got %d: %s", len(call.Args), call.String())
	}
	if _, ok := call.Args[0].(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected first arg ArrayLiteral, got %T", call.Args[0])
	}
	if _, ok := call.Args[3].(*ast.IntegerLiteral); !ok {
		t.Fatalf("expected fourth arg IntegerLiteral, got %T", call.Args[3])
	}
}

func TestParseBareCallArgumentStartingWithSelf(t *testing.T) {
	parse(t, `assert_equal self, exc.receiver`)
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

func TestParseCallArgumentProcBlockBeforeKeywordArgument(t *testing.T) {
	parse(t, `string = Marshal.send(@method, Marshal.dump("foo"), proc { |o| o.upcase }, freeze: true)`)
}

func TestParseCallArgumentParenthesizedCallWithBlock(t *testing.T) {
	parse(t, `assert_equal SummablePayment.new(20), payments.sum(SummablePayment.new(0)) { |p| p }`)
}

func TestParseBangMethodCallWithParenthesizedArgsAndBlock(t *testing.T) {
	parse(t, `hash_1.deep_merge!(hash_2) { |k, o, n| [k, o, n] }`)
}

func TestParseBlockCallAsParenthesizedCallArgumentBeforeNextStatement(t *testing.T) {
	parse(t, `assert_equal(expected, hash_1.deep_merge(hash_2) { |k, o, n| [k, o, n] })

hash_1.deep_merge!(hash_2) { |k, o, n| [k, o, n] }`)
}

func TestParseMethodCallWithNestedParenthesizedArgsAndDoBlock(t *testing.T) {
	parse(t, `DateTime.stub(:current, DateTime.civil(2005, 2, 10, 15, 30, 45, Rational(-18000, 86400))) do
  assert_equal true, DateTime.civil(2005, 2, 10, 15, 30, 44, Rational(-18000, 86400)).past?
end`)
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

func TestParseBareMethodCallWithLambdaArgumentAndDoBlock(t *testing.T) {
	program := parse(t, `guard -> { true } do
  it "x" do
    1
  end
end`)
	call, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", program.Statements[0])
	}
	if call.Method.Value != "guard" || len(call.Args) != 1 || call.Block == nil {
		t.Fatalf("expected guard with lambda argument and trailing block, got %s", call.String())
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

func TestParseInfixAfterBraceBlockCall(t *testing.T) {
	parse(t, "`id -G`.scan(/\\d+/).map {|i| i.to_i} << gid")
	parse(t, "augmented_groups = `id -G`.scan(/\\d+/).map {|i| i.to_i} << gid")
	parse(t, `describe "Process.initgroups" do
  as_user do
    augmented_groups = `+"`id -G`"+`.scan(/\d+/).map {|i| i.to_i} << gid
  end
end`)
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

func TestParseReturnWithOrKeywordValueError(t *testing.T) {
	errs := parseWithErrors("return true or false")
	assertContainsError(t, errs, "void value expression")
}

func TestParseBreakWithOrKeywordValueError(t *testing.T) {
	errs := parseWithErrors("break true or false")
	assertContainsError(t, errs, "void value expression")
}

func TestParseNextWithOrKeywordValueError(t *testing.T) {
	errs := parseWithErrors("next true or false")
	assertContainsError(t, errs, "void value expression")
}

func TestParseReturnValueAssignmentIsVoidValueError(t *testing.T) {
	assertContainsError(t, parseWithErrors("x = return"), "void value expression")
	assertContainsError(t, parseWithErrors("x = break"), "void value expression")
	assertContainsError(t, parseWithErrors("x = next"), "void value expression")
	assertContainsError(t, parseWithErrors("x = redo"), "void value expression")
	assertContainsError(t, parseWithErrors("x = retry"), "void value expression")
}

func TestParseIfValueAssignmentIsVoidValueError(t *testing.T) {
	assertContainsError(t, parseWithErrors(`x = if false
      return
    else
      return
    end`), "void value expression")
}

func TestParseSpecialVariableAssignmentIsError(t *testing.T) {
	assertContainsError(t, parseWithErrors("__LINE__ = 1"), "can't assign to __LINE__")
	assertContainsError(t, parseWithErrors("__FILE__ = 1"), "can't assign to __FILE__")
	assertContainsError(t, parseWithErrors("__ENCODING__ = 1"), "can't assign to __ENCODING__")
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

func TestParseKeywordArgNamedIn(t *testing.T) {
	parse(t, `Time.now(in: "W").utc_offset.should == -36000`)
}

func TestParseKeywordArgNamedPrepend(t *testing.T) {
	parse(t, `concerning(:Foo, prepend: true) { }`)
}

func TestParseCommentWithHexEscapesAfterKeywordArgCall(t *testing.T) {
	parse(t, `describe "when passed to, options" do
  it "x" do
    result = "あ?あ".send(@method, Encoding::EUC_JP, undef: :replace)
    # testing for: "\xA4\xA2?\xA4\xA2"
    xA4xA2 = [0xA4, 0xA2].pack('CC')
  end
end`)
}

func TestParseCallKeywordShorthandArgs(t *testing.T) {
	expr := parseExpr(t, "foo(a:, b:)")
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.KeywordArgs) != 2 {
		t.Fatalf("expected 2 keyword args, got %d", len(call.KeywordArgs))
	}
	for _, kw := range call.KeywordArgs {
		ident, ok := kw.Value.(*ast.Identifier)
		if !ok {
			t.Fatalf("expected shorthand value for %s to be Identifier, got %T", kw.Name, kw.Value)
		}
		if ident.Value != kw.Name {
			t.Fatalf("expected shorthand %s value to reference %s, got %s", kw.Name, kw.Name, ident.Value)
		}
	}
}

func TestParseForExpressionWithDestructuredTargets(t *testing.T) {
	parse(t, `for i, *j, k in [[1, 2, 3]]
  i
end`)
}

func TestParseForExpressionWithMultipleTargets(t *testing.T) {
	expr := parseExpr(t, `for i, j in [[1, 2], [3, 4]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 2 {
		t.Fatalf("expected 2 for variables, got %d", len(forExpr.Variable))
	}
	first, ok := forExpr.Variable[0].(*ast.Identifier)
	if !ok || first.Value != "i" {
		t.Fatalf("unexpected first for variable: %#v", forExpr.Variable[0])
	}
	second, ok := forExpr.Variable[1].(*ast.Identifier)
	if !ok || second.Value != "j" {
		t.Fatalf("unexpected second for variable: %#v", forExpr.Variable[1])
	}
}

func TestParseForExpressionWithGroupedTargets(t *testing.T) {
	expr := parseExpr(t, `for (i, j) in [[1, 2], [3, 4]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 2 {
		t.Fatalf("expected 2 for variables, got %d", len(forExpr.Variable))
	}
	first, ok := forExpr.Variable[0].(*ast.Identifier)
	if !ok || first.Value != "i" {
		t.Fatalf("unexpected first for variable: %#v", forExpr.Variable[0])
	}
	second, ok := forExpr.Variable[1].(*ast.Identifier)
	if !ok || second.Value != "j" {
		t.Fatalf("unexpected second for variable: %#v", forExpr.Variable[1])
	}
}

func TestParseForExpressionWithArrayTargets(t *testing.T) {
	expr := parseExpr(t, `for [i, j] in [[1, 2], [3, 4]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 2 {
		t.Fatalf("expected 2 for variables, got %d", len(forExpr.Variable))
	}
	first, ok := forExpr.Variable[0].(*ast.Identifier)
	if !ok || first.Value != "i" {
		t.Fatalf("unexpected first for variable: %#v", forExpr.Variable[0])
	}
	second, ok := forExpr.Variable[1].(*ast.Identifier)
	if !ok || second.Value != "j" {
		t.Fatalf("unexpected second for variable: %#v", forExpr.Variable[1])
	}
}

func TestParseForExpressionWithSingleGroupedTarget(t *testing.T) {
	expr := parseExpr(t, `for (i) in [1, 2]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 1 {
		t.Fatalf("expected 1 for variable, got %d", len(forExpr.Variable))
	}
	ident, ok := forExpr.Variable[0].(*ast.Identifier)
	if !ok || ident.Value != "i" {
		t.Fatalf("unexpected for variable: %#v", forExpr.Variable[0])
	}
}

func TestParseForExpressionWithEmptyCommaTarget(t *testing.T) {
	parse(t, `for i, in [[1, 2], [3, 4]]
end`)
}

func TestParseForExpressionWithEmptySplatTarget(t *testing.T) {
	parse(t, `for i, * in [[1, 2], [3, 4]]
end`)
}

func TestParseForExpressionWithMiddleSplatTarget(t *testing.T) {
	expr := parseExpr(t, `for a, *rest, z in [[1, 2, 3], [4, 5], [6]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(forExpr.Variable))
	}
	if _, ok := forExpr.Variable[1].(*ast.SplatExpression); !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", forExpr.Variable[1])
	}
	rest, ok := forExpr.Variable[1].(*ast.SplatExpression)
	if !ok || rest.Value == nil {
		t.Fatal("expected splat expression with value")
	}
	if ident, ok := rest.Value.(*ast.Identifier); !ok || ident.Value != "rest" {
		t.Fatalf("expected splat value rest, got %T", rest.Value)
	}
}

func TestParseForExpressionWithMiddleSplatInGroupedTarget(t *testing.T) {
	expr := parseExpr(t, `for (a, *rest, z) in [[1, 2, 3], [4, 5], [6]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(forExpr.Variable))
	}
	if _, ok := forExpr.Variable[1].(*ast.SplatExpression); !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", forExpr.Variable[1])
	}
	rest, ok := forExpr.Variable[1].(*ast.SplatExpression)
	if !ok || rest.Value == nil {
		t.Fatal("expected splat expression with value")
	}
	if ident, ok := rest.Value.(*ast.Identifier); !ok || ident.Value != "rest" {
		t.Fatalf("expected splat value rest, got %T", rest.Value)
	}
}

func TestParseForExpressionWithMiddleSplatInGroupedTargetAndHashSource(t *testing.T) {
	expr := parseExpr(t, `for (a, *rest, z) in {1 => 2, 3 => 4}
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(forExpr.Variable))
	}
	if _, ok := forExpr.Variable[1].(*ast.SplatExpression); !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", forExpr.Variable[1])
	}
}

func TestParseForExpressionWithGroupedMiddleEmptySplat(t *testing.T) {
	expr := parseExpr(t, `for (a, *, z) in [[1, 2, 3], [4], [5, 6]]
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(forExpr.Variable))
	}
	rest, ok := forExpr.Variable[1].(*ast.SplatExpression)
	if !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", forExpr.Variable[1])
	}
	if rest.Value == nil {
		t.Fatal("expected splat expression with implicit value")
	}
	ident, ok := rest.Value.(*ast.Identifier)
	if !ok || ident.Value != "_" {
		t.Fatalf("expected implicit splat value '_', got %T", rest.Value)
	}
}

func TestParseForExpressionWithGroupedMiddleEmptySplatAndHashSource(t *testing.T) {
	expr := parseExpr(t, `for (a, *, z) in { 1 => 2, 3 => 4 }
end`)
	forExpr, ok := expr.(*ast.ForExpression)
	if !ok {
		t.Fatalf("expected ForExpression, got %T", expr)
	}
	if len(forExpr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(forExpr.Variable))
	}
	rest, ok := forExpr.Variable[1].(*ast.SplatExpression)
	if !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", forExpr.Variable[1])
	}
	if rest.Value == nil {
		t.Fatal("expected splat expression with implicit value")
	}
	ident, ok := rest.Value.(*ast.Identifier)
	if !ok || ident.Value != "_" {
		t.Fatalf("expected implicit splat value '_', got %T", rest.Value)
	}
}

func TestParseForExpressionWithGroupedMiddleSplatInEmptyHash(t *testing.T) {
	forExpr := parseExpr(t, `for (a, *rest, z) in {}
end`)
	if forExpr == nil {
		t.Fatal("expected for expression")
	}
	expr := forExpr.(*ast.ForExpression)
	if len(expr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(expr.Variable))
	}
	if _, ok := expr.Variable[1].(*ast.SplatExpression); !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", expr.Variable[1])
	}
	if _, ok := expr.Collection.(*ast.HashLiteral); !ok {
		t.Fatalf("expected hash collection, got %T", expr.Collection)
	}
}

func TestParseForExpressionWithGroupedTargetAndEmptyCollection(t *testing.T) {
	forExpr := parseExpr(t, `for (a, *rest, z) in []
end`)
	if forExpr == nil {
		t.Fatal("expected for expression")
	}
	expr := forExpr.(*ast.ForExpression)
	if len(expr.Variable) != 3 {
		t.Fatalf("expected 3 for variables, got %d", len(expr.Variable))
	}
	if _, ok := expr.Variable[1].(*ast.SplatExpression); !ok {
		t.Fatalf("expected middle target to be SplatExpression, got %T", expr.Variable[1])
	}
	if _, ok := expr.Collection.(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected array collection, got %T", expr.Collection)
	}
}

func TestParseForExpressionWithDoTarget(t *testing.T) {
	parse(t, `for i in 1..3 do
	i
end`)
}

func TestParseForExpressionWithDoTargetAndSameLineBody(t *testing.T) {
	parse(t, `for i in 1..3 do i += 1
end`)
}

func TestParseForExpressionTargetsForHashEach(t *testing.T) {
	parse(t, `for key, value in {1 => 2}
end`)
}

func TestParseForExpressionTargetForHashEachSingleVariable(t *testing.T) {
	parse(t, `for pair in {1 => 2}
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

func TestParseEndlessMethodDefinition(t *testing.T) {
	parse(t, `def greet(person) = "Hi, ".dup.concat person`)
}

func TestParsePrivateEndlessMethodDefinition(t *testing.T) {
	parse(t, `private def instance_variables_to_inspect = []`)
}

func TestParseSuperForwardArguments(t *testing.T) {
	parse(t, `def m(...)
  super(...)
end`)
}

func TestParseDefReceiverOnlyForSingletonMethod(t *testing.T) {
	regular := parseExpr(t, `def regular
end`)
	regularDef, ok := regular.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", regular)
	}
	if regularDef.Receiver != nil {
		t.Fatalf("expected regular method receiver to be nil, got %T", regularDef.Receiver)
	}

	singleton := parseExpr(t, `def obj.singleton
end`)
	singletonDef, ok := singleton.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", singleton)
	}
	if singletonDef.Receiver == nil {
		t.Fatal("expected singleton method receiver")
	}
}

func TestParseSingletonMethodNameAsKeyword(t *testing.T) {
	expr := parseExpr(t, `def self.raise(*args, **kwargs, &block)
  1
end`)
	defExpr, ok := expr.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", expr)
	}
	if defExpr.Name == nil || defExpr.Name.Value != "raise" {
		t.Fatalf("expected method name raise, got %#v", defExpr.Name)
	}
	if defExpr.Receiver == nil {
		t.Fatal("expected singleton receiver")
	}
}

func TestParseParenthesizedLiteralSingletonMethodDefinition(t *testing.T) {
	parse(t, `def (false).foo; end`)
	parse(t, `def (true).foo; end`)
	parse(t, `def (nil).foo; end`)
}

func TestParseSingletonMethodDefinitionWithEmptyParamsAndInlineBody(t *testing.T) {
	parse(t, `def source_code.to_str() "1" end`)
	parse(t, `def source_code.to_str() :symbol end`)
	parse(t, `def lineno.to_int() 15 end`)
	parse(t, `def lineno.to_int() :symbol end`)
}

func TestParseBareHeredocCallArgumentAssignment(t *testing.T) {
	parse(t, `prc = instance_eval <<-CODE
  proc do |x, prc|
    x
  end
CODE

prc.call`)
}

func TestParseBasicObjectInstanceEvalSpec(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/basicobject/instance_eval_spec.rb")
	if err != nil {
		t.Fatal(err)
	}
	parse(t, string(input))
}

func TestParseBasicObjectInstanceExecSpec(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/basicobject/instance_exec_spec.rb")
	if err != nil {
		t.Fatal(err)
	}
	parse(t, string(input))
}

func TestParseProcessGroupsSpec(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/process/groups_spec.rb")
	if err != nil {
		t.Fatal(err)
	}
	parse(t, string(input))
}

func TestParseTimeComparisonSpec(t *testing.T) {
	input, err := os.ReadFile("../../vendor/ruby/spec/core/time/comparison_spec.rb")
	if err != nil {
		t.Fatal(err)
	}
	parse(t, string(input))
}

func TestParseSingletonSpaceshipMethodDefinition(t *testing.T) {
	parse(t, `def r.<=>(other); other <=> self; end`)
}

func TestParseGroupedSpaceshipFollowedByBareCall(t *testing.T) {
	parse(t, `(t <=> r).should be_nil`)
}

func TestParseTimeInverseComparisonExample(t *testing.T) {
	parse(t, `it "returns nil if argument also uses an inverse comparison for <=>" do
  t = Time.now
  r = mock('r')
  def r.<=>(other); other <=> self; end
  r.should_receive(:<=>).once

  (t <=> r).should be_nil
end`)
}

func TestParseTimeIntegerComparisonLambdaExample(t *testing.T) {
	parse(t, `it "returns nil when compared to an Integer because Time does not respond to #coerce" do
  time = Time.at(1)
  time.respond_to?(:coerce).should == false
  time.should_receive(:respond_to?).exactly(2).and_return(false)
  -> {
    (time <=> 2).should == nil
    (2 <=> time).should == nil
  }.should_not complain
end`)
}

func TestParseTimeNonTimeArgumentContext(t *testing.T) {
	parse(t, `describe "given a non-Time argument" do
  it "returns nil if argument <=> self returns nil" do
    t = Time.now
    obj = mock('time')
    obj.should_receive(:<=>).with(t).and_return(nil)
    (t <=> obj).should == nil
  end

  it "returns -1 if argument <=> self is greater than 0" do
    t = Time.now
    r = mock('r')
    r.should_receive(:>).with(0).and_return(true)
    obj = mock('time')
    obj.should_receive(:<=>).with(t).and_return(r)
    (t <=> obj).should == -1
  end

  it "returns 1 if argument <=> self is not greater than 0 and is less than 0" do
    t = Time.now
    r = mock('r')
    r.should_receive(:>).with(0).and_return(false)
    r.should_receive(:<).with(0).and_return(true)
    obj = mock('time')
    obj.should_receive(:<=>).with(t).and_return(r)
    (t <=> obj).should == 1
  end

  it "returns 0 if argument <=> self is neither greater than 0 nor less than 0" do
    t = Time.now
    r = mock('r')
    r.should_receive(:>).with(0).and_return(false)
    r.should_receive(:<).with(0).and_return(false)
    obj = mock('time')
    obj.should_receive(:<=>).with(t).and_return(r)
    (t <=> obj).should == 0
  end

  it "returns nil if argument also uses an inverse comparison for <=>" do
    t = Time.now
    r = mock('r')
    def r.<=>(other); other <=> self; end
    r.should_receive(:<=>).once

    (t <=> r).should be_nil
  end
end`)
}

func TestParseTimeComparisonLeadingExamples(t *testing.T) {
	parse(t, `describe "Time#<=>" do
  it "returns 1 if the first argument is a point in time after the second argument" do
    (Time.now <=> Time.at(0)).should == 1
  end

  it "returns 1 if the first argument is a fraction of a microsecond after the second argument" do
    (Time.at(100, Rational(1,1000)) <=> Time.at(100, 0)).should == 1
  end

  context "given different timezones" do
    it "returns 0 if time is the same as other" do
      time_utc = Time.new(2000, 1, 1, 0, 0, 0, 0)
      time_cet = Time.new(2000, 1, 1, 1, 0, 0, '+01:00')
      time_brt = Time.new(1999, 12, 31, 21, 0, 0, '-03:00')
      (time_utc <=> time_cet).should == 0
      (time_utc <=> time_brt).should == 0
      (time_cet <=> time_brt).should == 0
    end
  end
end`)
}

func TestParseGroupedSpaceshipWithRationalCallOnRight(t *testing.T) {
	parse(t, `(Time.at(100, Rational(1,1000)) <=> Time.at(100, Rational(1,1000))).should == 0`)
	parse(t, `(Time.at(100, 0) <=> Time.at(100, Rational(1,1000))).should == -1`)
}

func TestParseBareCallWithMultipleArgsAndDoBlock(t *testing.T) {
	parse(t, `platform_is_not :windows, :android do
  as_superuser do
    Process.groups = []
  end
end`)
}

func TestParseGroupedCompoundAssignmentWithRescueModifier(t *testing.T) {
	parse(t, "(groups |= `/usr/bin/id -G`.scan(/\\d+/).map { |i| i.to_i }) rescue nil")
}

func TestParseCallArgumentRegexpWithApostrophe(t *testing.T) {
	parse(t, `-> { a.instance_eval(source_code) }.should raise_consistent_error(TypeError, /can't convert Object into String/)`)
}

func TestParsePercentBraceStringArgument(t *testing.T) {
	parse(t, `obj.instance_eval %{
  class B; end
  B
}`)
}

func TestParseGroupedBlockCallEqualityReceiver(t *testing.T) {
	parse(t, `(s == s.instance_eval { self }).should be_true`)
	parse(t, `(o == o.instance_eval("self")).should be_true`)
}

func TestParseGroupedRangeMethodCallInGroupedEquality(t *testing.T) {
	parse(t, `((1..10).step == (1..11).step).should == false`)
	parse(t, `((1..10).step == (1...10).step).should == false`)
	parse(t, `((1..10).step == (1..10).step(2)).should == false`)
	parse(t, `((1..10).step.hash == (1..10).step(2).hash).should == false`)
}

func TestParseGroupedPrefixCallReceiverChainAsArgument(t *testing.T) {
	parse(t, `@bignum.div((-bignum_value(88)).to_f).should eql(-1)`)
	parse(t, `(-(10**50)).div(-(10**40 + 1)).should == 9999999999`)
	parse(t, `@bignum.div(-(@bignum+1)).should == (@bignum / -(@bignum+1)).floor`)
}

func TestParseEndlessRangeStepEndMethodChain(t *testing.T) {
	parse(t, `(1..).step(1).end.should == nil`)
	parse(t, `(1...).step(1).end.should == nil`)
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

func TestParseForKeywordMethodNameAfterDot(t *testing.T) {
	parse(t, `IO::Buffer.for(+"12345") do |buffer|
  buffer.size
end`)
}

func TestParseUntilKeywordMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `1.month.until(@now)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "until" {
		t.Fatalf("expected until method name, got %#v", call.Method)
	}
}

func TestParseDoKeywordMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `obj.do`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "do" {
		t.Fatalf("expected do method name, got %#v", call.Method)
	}
}

func TestParseInKeywordMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `@twz.in(1)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "in" {
		t.Fatalf("expected in method name, got %#v", call.Method)
	}
}

func TestParseLShiftOperatorMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `y.<<(1)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "<<" {
		t.Fatalf("expected << method name, got %#v", call.Method)
	}
}

func TestParseSelfMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `tp.self`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "self" {
		t.Fatalf("expected self method name, got %#v", call.Method)
	}
}

func TestParseAliasKeywordMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `c.new.alias.should`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected outer MethodCall, got %T", expr)
	}
	inner, ok := call.Receiver.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected inner MethodCall, got %T", call.Receiver)
	}
	if inner.Method == nil || inner.Method.Value != "alias" {
		t.Fatalf("expected alias method name, got %#v", inner.Method)
	}
}

func TestParseMethodNameOnLineAfterDot(t *testing.T) {
	parse(t, `Encoding::Converter.
  asciicompat_encoding(Encoding.find("ISO-2022-JP")).
  should == Encoding::Converter.asciicompat_encoding("ISO-2022-JP")`)
}

func TestParseOperatorMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `1.+(2)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "+" {
		t.Fatalf("expected + method name, got %#v", call.Method)
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

func TestParseBareIncludeWithMultipleArguments(t *testing.T) {
	parse(t, `class B < A; include U, V, W; end`)
	parse(t, `module V; include X, U, Y; end`)
}

func TestParseBareIncludeWithNoArguments(t *testing.T) {
	parse(t, `Module.new do
  include
end`)
}

func TestParseBlockCallFollowedByIndex(t *testing.T) {
	expr := parseExpr(t, `proc { |a| a }["sometext"]`)
	if _, ok := expr.(*ast.IndexExpression); !ok {
		t.Fatalf("expected IndexExpression, got %T", expr)
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
