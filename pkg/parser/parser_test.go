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

func TestParseBareRescueInsideBlock(t *testing.T) {
	parse(t, `module Example
  def self.clear
    [1].each do |value|
      begin
        puts value
      rescue
      end
    end
  end
end`)
}

func TestSplitPercentWordsPreservesOrdinaryBackslash(t *testing.T) {
	words := splitPercentLiteralWords(`special/\a escaped\ space`)
	if len(words) != 2 || words[0] != `special/\a` || words[1] != "escaped space" {
		t.Fatalf("unexpected words: %#v", words)
	}
}

func TestParseBareArrayArgumentDotChain(t *testing.T) {
	expr := parseExpr(t, `Marshal.load [args].pack("H*")`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "load" || len(call.Args) != 1 {
		t.Fatalf("expected Marshal.load with one argument, got %T %#v", expr, expr)
	}
	pack, ok := call.Args[0].(*ast.MethodCall)
	if !ok || pack.Method == nil || pack.Method.Value != "pack" {
		t.Fatalf("expected array pack call as argument, got %T %#v", call.Args[0], call.Args[0])
	}
	if _, ok := pack.Receiver.(*ast.ArrayLiteral); !ok {
		t.Fatalf("expected pack receiver to be an array, got %T", pack.Receiver)
	}
}

func parseWithErrors(input string) []string {
	l := lexer.New(input)
	p := New(l)
	p.ParseProgram()
	return p.Errors()
}

func TestParseQualifiedUppercaseMethodCall(t *testing.T) {
	expr := parseExpr(t, `UpperMethod::Build(42)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "Build" {
		t.Fatalf("expected Build MethodCall, got %T %#v", expr, expr)
	}
	receiver, ok := call.Receiver.(*ast.Constant)
	if !ok || receiver.Name != "UpperMethod" {
		t.Fatalf("expected UpperMethod receiver, got %T %#v", call.Receiver, call.Receiver)
	}
}

func TestParsePercentSymbolArrayAsBareCallArgument(t *testing.T) {
	expr := parseExpr(t, `Set.new %i[get post]`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "new" || len(call.Args) != 1 {
		t.Fatalf("expected Set.new with one argument, got %T %#v", expr, expr)
	}
	array, ok := call.Args[0].(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 2 {
		t.Fatalf("expected two-element symbol array argument, got %T %#v", call.Args[0], call.Args[0])
	}
}

func TestParseMultilineHashValueOmissionBeforeClosingBrace(t *testing.T) {
	parse(t, `def options(type, optional, default)
  {
    type:,
    optional:,
    default:
  }.compact
end`)
}

func TestPercentWordsDecodeEscapedBackslash(t *testing.T) {
	expr := parseExpr(t, `%w[\\]`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 1 {
		t.Fatalf("expected one-element ArrayLiteral, got %T (%v)", expr, expr)
	}
	word, ok := array.Elements[0].(*ast.StringLiteral)
	if !ok || word.Value != `\` {
		t.Fatalf("expected one backslash word, got %T (%v)", array.Elements[0], array.Elements[0])
	}
}

func TestArrayLiteralCombinesAdjacentUnbracedLabelPairs(t *testing.T) {
	expr := parseExpr(t, `[args: [1, 2, 3], kw: {a: "b"}]`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 1 {
		t.Fatalf("expected one-element ArrayLiteral, got %T (%v)", expr, expr)
	}
	hash, ok := array.Elements[0].(*ast.HashLiteral)
	if !ok || len(hash.Order) != 2 {
		t.Fatalf("expected one HashLiteral with two ordered pairs, got %T (%v)", array.Elements[0], array.Elements[0])
	}
}

func TestArrayLiteralCombinesAdjacentMixedUnbracedHashPairs(t *testing.T) {
	expr := parseExpr(t, `[1, "foo" => :bar, baz: 42, 3 => 6]`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 2 {
		t.Fatalf("expected scalar and one HashLiteral, got %T (%v)", expr, expr)
	}
	hash, ok := array.Elements[1].(*ast.HashLiteral)
	if !ok || len(hash.Order) != 3 {
		t.Fatalf("expected one HashLiteral with three ordered pairs, got %T (%v)", array.Elements[1], array.Elements[1])
	}
}

func TestPercentCapitalWordsPreserveInterpolationAndEscapedWhitespace(t *testing.T) {
	expr := parseExpr(t, `%W(a\  b\tc #{value})`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 3 {
		t.Fatalf("expected three words, got %T (%v)", expr, expr)
	}
	wants := []string{"a ", "b\tc", "#{value}"}
	for index, want := range wants {
		word, ok := array.Elements[index].(*ast.StringLiteral)
		if !ok || word.Value != want || !word.Interpolates {
			t.Fatalf("word %d: expected interpolating %q, got %T (%v)", index, want, array.Elements[index], array.Elements[index])
		}
	}
}

func TestPercentWordsKeepEscapedWhitespaceInsideWord(t *testing.T) {
	expr := parseExpr(t, `%w[one two\ words]`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 2 {
		t.Fatalf("expected two-element ArrayLiteral, got %T (%v)", expr, expr)
	}
	word, ok := array.Elements[1].(*ast.StringLiteral)
	if !ok || word.Value != "two words" {
		t.Fatalf("expected escaped space inside second word, got %T (%v)", array.Elements[1], array.Elements[1])
	}
}

func TestWindows31JHashLabelTreatsTrailByteAsIdentifierContent(t *testing.T) {
	input := "{\x87]: 1}"
	p := New(lexer.NewWithEncoding(input, "Windows-31J"))
	p.ParseProgram()
	if errors := p.Errors(); len(errors) != 0 {
		t.Fatalf("expected Windows-31J label to parse, got %v", errors)
	}
}

func TestPercentRegexpParsesBackslashDelimiter(t *testing.T) {
	expr := parseExpr(t, `%r\ foo \`)
	literal, ok := expr.(*ast.RegexpLiteral)
	if !ok || literal.Pattern != " foo " {
		t.Fatalf("expected backslash-delimited regexp body, got %T (%v)", expr, expr)
	}
}

func TestRescueModifierInSpacedMethodArgumentStaysInsideArgument(t *testing.T) {
	expr := parseExpr(t, `1.+ (raise("Error") rescue 1)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected one argument, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.BeginExpression); !ok {
		t.Fatalf("expected rescued argument, got %T", call.Args[0])
	}
}

func TestMethodParameterDestructuringPattern(t *testing.T) {
	expr := parseExpr(t, `def unpack((a, *middle, z)); [a, middle, z]; end`)
	definition, ok := expr.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", expr)
	}
	if len(definition.Params) != 1 || len(definition.ParamPatterns) != 1 {
		t.Fatalf("expected one physical parameter and pattern, got %d and %d", len(definition.Params), len(definition.ParamPatterns))
	}
	pattern := definition.ParamPatterns[0]
	if pattern == nil || len(pattern.Children) != 2 || pattern.Rest == nil || pattern.RestIndex != 1 {
		t.Fatalf("unexpected pattern: %#v", pattern)
	}
	if pattern.Children[0].Name.Value != "a" || pattern.Rest.Name.Value != "middle" || pattern.Children[1].Name.Value != "z" {
		t.Fatalf("unexpected pattern names: %#v", pattern)
	}
}

func TestMethodRestParameterPreservesPostParameterPosition(t *testing.T) {
	expr := parseExpr(t, `def last(*, value); value; end`)
	definition, ok := expr.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", expr)
	}
	if definition.RestParam == nil || definition.RestParamIndex != 0 {
		t.Fatalf("expected anonymous rest before post parameter, got %#v at %d", definition.RestParam, definition.RestParamIndex)
	}
	if len(definition.Params) != 1 || definition.Params[0].Value != "value" {
		t.Fatalf("unexpected post parameters: %#v", definition.Params)
	}
}

func TestLambdaParameterDestructuringPattern(t *testing.T) {
	expr := parseExpr(t, `-> ((a, (b, *tail))) { [a, b, tail] }`)
	lambda := expr.(*ast.ProcLiteral)
	if len(lambda.Params) != 1 || len(lambda.ParamPatterns) != 1 {
		t.Fatalf("expected one lambda parameter pattern, got %d and %d", len(lambda.Params), len(lambda.ParamPatterns))
	}
	pattern := lambda.ParamPatterns[0]
	if pattern == nil || len(pattern.Children) != 2 || len(pattern.Children[1].Children) != 1 || pattern.Children[1].Rest == nil {
		t.Fatalf("unexpected nested lambda pattern: %#v", pattern)
	}
}

func TestArrowLambdaCountsEachParenthesizedParameter(t *testing.T) {
	expr := parseExpr(t, `-> (a, (*b, c), d, (*e), (*)) { }`)
	lambda := expr.(*ast.ProcLiteral)
	if len(lambda.Params) != 5 || len(lambda.ParamPatterns) != 5 {
		t.Fatalf("expected five physical parameters and patterns, got %d and %d", len(lambda.Params), len(lambda.ParamPatterns))
	}
}

func TestArrowLambdaPreservesAnonymousRestParameter(t *testing.T) {
	expr := parseExpr(t, `-> (*) { }`)
	lambda := expr.(*ast.ProcLiteral)
	if lambda.RestParam == nil || lambda.RestParamIndex != 0 {
		t.Fatalf("expected anonymous rest parameter, got %#v at %d", lambda.RestParam, lambda.RestParamIndex)
	}
}

func TestArrowLambdaKeywordParameters(t *testing.T) {
	expr := parseExpr(t, `-> (required:, optional: 1, **rest) { [required, optional, rest] }`)
	lambda := expr.(*ast.ProcLiteral)
	if len(lambda.KeywordParams) != 2 || lambda.KeywordParams[0].Name != "required" || lambda.KeywordParams[0].Default != nil {
		t.Fatalf("unexpected required keyword metadata: %#v", lambda.KeywordParams)
	}
	if lambda.KeywordParams[1].Name != "optional" || lambda.KeywordParams[1].Default == nil {
		t.Fatalf("unexpected optional keyword metadata: %#v", lambda.KeywordParams[1])
	}
	if lambda.KeywordRestParam == nil || lambda.KeywordRestParam.Value != "rest" {
		t.Fatalf("unexpected keyword rest metadata: %#v", lambda.KeywordRestParam)
	}
}

func TestLambdaKeywordDefaultCanDefineNestedLambda(t *testing.T) {
	parse(t, `@a = -> (a: @a = -> (a: 1) { a }, b:) do
  [a, b]
end`)
	parse(t, `@a = lambda do |a: (@a = -> (a: 1) { a }), b:|
  [a, b]
end`)
}

func TestBlockParameterDestructuringPattern(t *testing.T) {
	expr := parseExpr(t, `target { |(a, *tail)| [a, tail] }`)
	call := expr.(*ast.MethodCall)
	if call.Block == nil || len(call.Block.Params) != 1 || len(call.Block.ParamPatterns) != 1 || call.Block.ParamPatterns[0].Rest == nil {
		t.Fatalf("unexpected block parameter pattern: %#v", call.Block)
	}
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

func TestEndlessRangeRemainsOneArgumentBeforeComma(t *testing.T) {
	expr := parseExpr(t, `Enumerator.product(1.., ["A", "B"])`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 arguments, got %d: %s", len(call.Args), expr.String())
	}
	rangeExpr, ok := call.Args[0].(*ast.RangeExpression)
	if !ok {
		t.Fatalf("expected first argument RangeExpression, got %T", call.Args[0])
	}
	if _, ok := rangeExpr.Right.(*ast.NilExpression); !ok {
		t.Fatalf("expected missing range end, got %T", rangeExpr.Right)
	}
}

func TestParenthesizedSuperHasExplicitSingleExpressionArgument(t *testing.T) {
	program := parse(t, `def value(x, y); super(x + 3 * y); end`)
	statement := program.Statements[0].(*ast.ExpressionStatement)
	definition := statement.Expression.(*ast.DefExpression)
	bodyStatement := definition.Body.Statements[0].(*ast.ExpressionStatement)
	superExpression := bodyStatement.Expression.(*ast.SuperExpression)
	if superExpression.ImplicitArgs || len(superExpression.Args) != 1 {
		t.Fatalf("expected one explicit super argument, implicit=%v args=%d", superExpression.ImplicitArgs, len(superExpression.Args))
	}
}

func TestSuperSeparatesKeywordSplatAndStaticKeywordArguments(t *testing.T) {
	program := parse(t, `def value(x, **options); super(x, **options, enabled: true); end`)
	statement := program.Statements[0].(*ast.ExpressionStatement)
	definition := statement.Expression.(*ast.DefExpression)
	bodyStatement := definition.Body.Statements[0].(*ast.ExpressionStatement)
	superExpression := bodyStatement.Expression.(*ast.SuperExpression)
	if superExpression.ImplicitArgs || len(superExpression.Args) != 2 || len(superExpression.KeywordArgs) != 1 {
		t.Fatalf("expected positional, keyword splat and static keyword, got implicit=%v args=%d keywords=%d",
			superExpression.ImplicitArgs, len(superExpression.Args), len(superExpression.KeywordArgs))
	}
}

func TestBareSuperContinuesKeywordArgumentsAfterCommaAndNewline(t *testing.T) {
	program := parse(t, `def initialize(config_path:, key_path:, env_key:)
  super content_path: config_path, key_path: key_path,
    env_key: env_key
end`)
	statement := program.Statements[0].(*ast.ExpressionStatement)
	definition := statement.Expression.(*ast.DefExpression)
	bodyStatement := definition.Body.Statements[0].(*ast.ExpressionStatement)
	superExpression := bodyStatement.Expression.(*ast.SuperExpression)
	if superExpression.ImplicitArgs || len(superExpression.KeywordArgs) != 3 {
		t.Fatalf("expected three explicit super keyword arguments, implicit=%v keywords=%d",
			superExpression.ImplicitArgs, len(superExpression.KeywordArgs))
	}
}

func TestParseSuperDoBlockInsideIfBranch(t *testing.T) {
	parse(t, `def call(*args)
  if args.empty?
    :empty
  else
    super(*args) do
      :value
    end
  end
end`)
}

func TestKeywordAndHasLowerPrecedenceThanAssignment(t *testing.T) {
	expr := parseExpr(t, `x = 1 and y = 2`)
	logical, ok := expr.(*ast.InfixExpression)
	if !ok || logical.Operator != "and" {
		t.Fatalf("expected top-level and expression, got %T: %s", expr, expr.String())
	}
	if _, ok := logical.Left.(*ast.AssignExpression); !ok {
		t.Fatalf("expected assignment on left, got %T", logical.Left)
	}
	if _, ok := logical.Right.(*ast.AssignExpression); !ok {
		t.Fatalf("expected assignment on right, got %T", logical.Right)
	}
}

func TestCompoundAssignmentsDoNotConsumeArgumentCommas(t *testing.T) {
	array, ok := parseExpr(t, `[a += 1, a += 1]`).(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 2 {
		t.Fatalf("expected two array elements, got %T: %s", array, array.String())
	}
	call, ok := parseExpr(t, `target(a += 1, a += 1)`).(*ast.MethodCall)
	if !ok || len(call.Args) != 2 {
		t.Fatalf("expected two call arguments, got %T: %s", call, call.String())
	}
}

func TestUndefParsesEveryCommaSeparatedMethod(t *testing.T) {
	undef, ok := parseExpr(t, `undef :first, :second`).(*ast.UndefExpression)
	if !ok || len(undef.Methods) != 2 {
		t.Fatalf("expected two undef methods, got %T: %s", undef, undef.String())
	}
}

func TestUndefPreservesPostfixIf(t *testing.T) {
	expr := parseExpr(t, `undef :missing if private_method_defined? :missing`)
	conditional, ok := expr.(*ast.IfExpression)
	if !ok || conditional.Consequent == nil || len(conditional.Consequent.Statements) != 1 {
		t.Fatalf("expected postfix IfExpression, got %T %#v", expr, expr)
	}
	statement := conditional.Consequent.Statements[0].(*ast.ExpressionStatement)
	undef, ok := statement.Expression.(*ast.UndefExpression)
	if !ok || len(undef.Methods) != 1 || undef.Methods[0].Value != ":missing" {
		t.Fatalf("expected one conditional undef method, got %T %#v", statement.Expression, statement.Expression)
	}
}

func TestUndefPreservesSetterMethodName(t *testing.T) {
	undef, ok := parseExpr(t, `undef context=`).(*ast.UndefExpression)
	if !ok || len(undef.Methods) != 1 || undef.Methods[0].Value != "context=" {
		t.Fatalf("expected setter method name, got %T %#v", undef, undef)
	}
}

func TestParseArraySplatEndingWithMultilineDoBlock(t *testing.T) {
	array, ok := parseExpr(t, `[:html, :attrs, *attrs.sort_by do |attr|
  [attr[2].to_s, n += 1]
end]`).(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 3 {
		t.Fatalf("expected three array elements, got %T: %v", array, array)
	}
	splat, ok := array.Elements[2].(*ast.SplatExpression)
	if !ok || !isBlockMethodCall(splat) {
		t.Fatalf("expected splatted block call, got %T: %v", array.Elements[2], array.Elements[2])
	}
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

func TestParsePostfixUntilConditionWithBraceBlockAndBooleanTail(t *testing.T) {
	parse(t, `Thread.pass until t.backtrace && t.backtrace.any? { |call| call.include? 'require' } && t.stop?`)
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
	l := lexer.New(`defined?(throw(:out, 42).foo)`)
	p := New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	expr := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefinedExpression).Expression
	if _, ok := expr.(*ast.MethodCall); !ok {
		if thrown, isThrow := expr.(*ast.ThrowExpression); isThrow {
			t.Fatalf("expected MethodCall, got ThrowExpression label=%T value=%T extra=%d", thrown.Label, thrown.Value, len(thrown.ExtraArgs))
		}
		t.Fatalf("expected MethodCall, got %T", expr)
	}
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

func TestParseNestedThreadNewRetainsInnerBlock(t *testing.T) {
	expr := parseExpr(t, `2.times.map { Thread.new { 1 } }`)
	outer, ok := expr.(*ast.MethodCall)
	if !ok || outer.Block == nil || len(outer.Block.Statements) != 1 {
		t.Fatalf("expected outer map block, got %T: %v", expr, expr)
	}
	statement := outer.Block.Statements[0].(*ast.ExpressionStatement)
	inner, ok := statement.Expression.(*ast.MethodCall)
	if !ok || inner.Block == nil {
		t.Fatalf("expected Thread.new inner block, got %T: %v", statement.Expression, statement.Expression)
	}
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

func TestParseTernaryWithNewlineConsequent(t *testing.T) {
	parse(t, "true ?\n [1, 2] : [3]")
}

func TestParseArrayElementTernaryConditionEndingInArrayLiteral(t *testing.T) {
	expr := parseExpr(t, `[
  @seconds == [0] ? nil : (@seconds || ["*"]).join(","),
  2
].compact`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "compact" {
		t.Fatalf("expected compact call on array literal, got %T (%v)", expr, expr)
	}
	array, ok := call.Receiver.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 2 {
		t.Fatalf("expected two array elements, got %T (%v)", call.Receiver, call.Receiver)
	}
	if _, ok := array.Elements[0].(*ast.TernaryExpression); !ok {
		t.Fatalf("expected first element to be ternary, got %T (%v)", array.Elements[0], array.Elements[0])
	}
}

func TestParseTernaryWithColonOnFollowingLine(t *testing.T) {
	parse(t, `formatter = inline ? HTMLInline.new(theme)
                   : HTML.new`)
}

func TestParseMultilineTernaryWithProcBlocks(t *testing.T) {
	parse(t, `proc { |a, p| unbound_method.bind(a).call(*p) }`)
	parse(t, `wrapper = block.arity.zero? ?
  proc { |a, _p| unbound_method.bind(a).call } :
  proc { |a, p| unbound_method.bind(a).call(*p) }`)
}

func TestParseKeywordNamedMethodAndLineLeadingBooleanOr(t *testing.T) {
	parse(t, `class Pager
  def next
    42
  end

  def skip?(options)
    options == {} ||
      options[:except] ||
      options[:only]
  end
end`)
}

func TestParseReturnNamedMethod(t *testing.T) {
	parse(t, `class Clock
  def return(&block)
    block.call
  end
end`)
}

func TestParseBareYieldAsFirstCommandArgument(t *testing.T) {
	parse(t, `assert yield, message`)
}

func TestParseMultilineGroupedExpressionFollowedByModulo(t *testing.T) {
	parse(t, `checksum = (
  7 * (values[0] + values[3]) +
    3 * (values[1] + values[4])
) % 10`)
}

func TestParseMultiplicationByGroupedTernaryWithNestedAssignment(t *testing.T) {
	expr := parseExpr(t, `25 * ((page = page - 1) < 0 ? 0 : page)`)
	product, ok := expr.(*ast.InfixExpression)
	if !ok || product.Operator != "*" {
		t.Fatalf("expected multiplication, got %T (%v)", expr, expr)
	}
	if _, ok := product.Right.(*ast.TernaryExpression); !ok {
		t.Fatalf("expected grouped ternary on the right, got %T (%v)", product.Right, product.Right)
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

func TestParseConstantBracketCallWithNestedArrayArguments(t *testing.T) {
	parse(t, `Set[[:b, 2], [:a, 1]]`)
}

func TestParseConstantBracketCallWithCharacterLiteralArgument(t *testing.T) {
	program := parse(t, `Set[?c, "b", :a]`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	stmt, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected ExpressionStatement, got %T", program.Statements[0])
	}
	call, ok := stmt.Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", stmt.Expression)
	}
	if len(call.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(call.Args))
	}
}

func TestParseConstantBracketCallWithStringArguments(t *testing.T) {
	program := parse(t, `Set["c", "b", :a]`)
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	call, ok := stmt.Expression.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", stmt.Expression)
	}
	if len(call.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(call.Args))
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

func TestParseRaiseWithScopedExceptionAndKeywordOptions(t *testing.T) {
	program := parse(t, `raise Pundit::NotAuthorizedError, query: :update?, record: record, policy: policy`)
	raise, ok := program.Statements[0].(*ast.RaiseExpression)
	if !ok {
		t.Fatalf("expected RaiseExpression, got %T", program.Statements[0])
	}
	if _, ok := raise.Error.(*ast.ConstantResolution); !ok {
		t.Fatalf("expected scoped exception class, got %T %#v", raise.Error, raise.Error)
	}
	options, ok := raise.Message.(*ast.HashLiteral)
	if !ok || len(options.Order) != 3 || !raise.MessageIsKeyword {
		t.Fatalf("expected three keyword options as the message, got %T %#v", raise.Message, raise)
	}
	if raise.Backtrace != nil || raise.Keyword != nil {
		t.Fatalf("unexpected extra raise arguments: %#v", raise)
	}
}

func TestParseForInsideProcInsideCaseKeepsCaseElse(t *testing.T) {
	parse(t, `callback = case next_state
when Array
  proc do
    for item in next_state do
      next values.pop if item == :pop
      values << item
    end
  end
else
  raise "invalid"
end`)
}

func TestParsePercentRegexpDelimiterInsideInterpolationString(t *testing.T) {
	expr := parseExpr(t, `pattern = %r'\b(?:#{keywords.join('|')})\b'`)
	assignment, ok := expr.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected assignment, got %T", expr)
	}
	regexp, ok := assignment.Value.(*ast.RegexpLiteral)
	if !ok {
		t.Fatalf("expected regexp value, got %T", assignment.Value)
	}
	if regexp.Pattern != `\b(?:#{keywords.join('|')})\b` || !regexp.Interpolates {
		t.Fatalf("unexpected interpolated regexp: %#v", regexp)
	}
}

func TestParseRaiseWithPostfixUnless(t *testing.T) {
	parse(t, `raise "subprocesses leaked" unless leaked.empty?`)
}

func TestParseRaiseClassAndMessageWithPostfixIf(t *testing.T) {
	expr := parseExpr(t, `raise RuntimeError, "blocked" if frozen?`)
	conditional, ok := expr.(*ast.IfExpression)
	if !ok || conditional.Consequent == nil || len(conditional.Consequent.Statements) != 1 {
		t.Fatalf("expected postfix IfExpression, got %T %#v", expr, expr)
	}
	statement, ok := conditional.Consequent.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected consequent ExpressionStatement, got %T", conditional.Consequent.Statements[0])
	}
	raise, ok := statement.Expression.(*ast.RaiseExpression)
	if !ok || raise.Error == nil || raise.Message == nil {
		t.Fatalf("expected raise class and message, got %T %#v", statement.Expression, statement.Expression)
	}
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
	input := `ArraySpecs::MyArray.[](5, true, nil, "a")`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}
	statement := program.Statements[0].(*ast.ExpressionStatement)
	call := statement.Expression.(*ast.MethodCall)
	if len(call.Args) != 4 {
		t.Fatalf("expected 4 arguments, got %d: %s", len(call.Args), call.String())
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

func TestParseModuleKeywordAsExplicitReceiverMethodName(t *testing.T) {
	parse(t, `def self.module(*namespaces, default: :nominal, **aliases)
  [namespaces, default, aliases]
end`)
}

func TestParseKeywordArgumentInConstantIndexCall(t *testing.T) {
	call, ok := parseExpr(t, `TypeClass[value, fn: callable]`).(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "[]" || len(call.Args) != 2 {
		t.Fatalf("expected constant [] call with two arguments, got %T %#v", call, call)
	}
	hash, ok := call.Args[1].(*ast.HashLiteral)
	if !ok || hash.Token.Type != lexer.COLON || len(hash.Pairs) != 1 {
		t.Fatalf("expected implicit keyword hash, got %T %#v", call.Args[1], call.Args[1])
	}
}

func TestParseKeywordArgumentInDynamicIndexCall(t *testing.T) {
	call, ok := parseExpr(t, `constructor_type[value, fn: callable]`).(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "[]" || len(call.Args) != 2 {
		t.Fatalf("expected dynamic [] call with keyword hash, got %T %#v", call, call)
	}
}

func TestParseRaiseWithPostfixIfInGroupedExpression(t *testing.T) {
	parse(t, `(raise if 2 + 2 == 3; /a/)`)
}

func TestParseThenAsMethodName(t *testing.T) {
	parse(t, `self.then { value }`)
}

func TestParseYieldAsMethodName(t *testing.T) {
	parse(t, `def yield(source = nil); source; end`)
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

func TestKeywordParameterNamedIfDoesNotShadowControlFlowIf(t *testing.T) {
	parse(t, `class State
  def initialize(values, if: nil)
    filtered = values.select do |value|
      value
    end

    if (found = filtered.detect do |value|
      value
    end)
      found
    elsif filtered.empty?
      nil
    end
  end
end`)
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

func TestParseEndlessRangeIndexAsKeywordArgumentValue(t *testing.T) {
	parse(t, `new(value: content.to_s[2..], private: private)`)
	parse(t, `def parts
  @value.split(DOT)[1..]
end`)
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

func TestParseRangeWithCompoundAssignmentEndpoint(t *testing.T) {
	parse(t, `values = (@index...@index += count).to_a`)
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

func TestNestedBreakCallKeepsFollowingOuterBlockStatements(t *testing.T) {
	expr := parseExpr(t, `invoke do
  ["a"].bsearch { |value| break }.should be_nil
  events << nil
  events << :after
end`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Block == nil {
		t.Fatalf("expected method call with block, got %T", expr)
	}
	if len(call.Block.Statements) != 3 {
		t.Fatalf("expected 3 outer block statements, got %d: %s", len(call.Block.Statements), call.Block.String())
	}
}

func TestParsePercentSymbolArrayLiteral(t *testing.T) {
	expr := parseExpr(t, `%i[alpha beta
gamma]`)
	arr, ok := expr.(*ast.ArrayLiteral)
	if !ok {
		t.Fatalf("expected ArrayLiteral, got %T", expr)
	}
	if len(arr.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr.Elements))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		sym, ok := arr.Elements[i].(*ast.SymbolLiteral)
		if !ok {
			t.Fatalf("element %d: expected SymbolLiteral, got %T", i, arr.Elements[i])
		}
		if sym.Value != want {
			t.Fatalf("element %d: expected %q, got %q", i, want, sym.Value)
		}
	}
}

func TestParseArrayLiteralWithBlockCallAndLambdaElements(t *testing.T) {
	parse(t, `[Proc.new{}, -> {}, proc {}]`)
	parse(t, `[Proc.new{}, -> {}, proc {}].each { |p| p.binding }`)
}

func TestParseArrayEndingWithBlockCallBeforeCaseElse(t *testing.T) {
	parse(t, `case value
when 1
  [true, items.any? { |item| item }]
else
  [false, false]
end`)
}

func TestParseMultilineArrayEndingWithNestedArray(t *testing.T) {
	parse(t, `[
  first,
  value == [3, 0.25]
]`)
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

func TestParseHeredocAssignmentBeforeConditional(t *testing.T) {
	parse(t, `def file_collision_help(block_given)
  help = <<-HELP
text
  HELP
  if block_given
    help << <<-HELP
more
    HELP
  end
  help
end`)
}

func TestParseEmbeddedDocumentBetweenStatements(t *testing.T) {
	parse(t, `before
=begin
This is documentation, not Ruby = ]
=end
after`)
}

func TestParseMultilineRescueExceptionListWithTernary(t *testing.T) {
	parse(t, `begin
  work
rescue IOError, EOFError,
       Errno::ECONNRESET,
       defined?(OpenSSL::SSL) ? OpenSSL::SSL::SSLError : IOError,
       Timeout::Error => exception
  exception
end`)
}

func TestParseSingleQuotedHeredocWithPunctuationDelimiter(t *testing.T) {
	parse(t, "target.class_eval <<-'end;', __FILE__, __LINE__ + 1\n  def value\n    1\n  end\nend;\n")
}

func TestParseTernaryWithBareYieldConsequent(t *testing.T) {
	parse(t, `value = (block_given? && default.nil?) ? yield : default`)
}

func TestParseBlockCallAsIfCondition(t *testing.T) {
	parse(t, `if wait_until(timeout) { generation.status != :waiting }
  generation.status == :fulfilled
else
  false
end`)
}

func TestParseConstantResolutionBareKeywordArguments(t *testing.T) {
	parse(t, `DEBUGGER__::start no_sigint_hook: true, nonstop: true`)
}

func TestParsePercentCommandLiteralDotChain(t *testing.T) {
	parse(t, `%x{printf 42}.to_i`)
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

func TestParsePercentRegexpLiteralRemovesEscapingForNonMetaDelimiter(t *testing.T) {
	expr := parseExpr(t, `%r@\@@`)
	re, ok := expr.(*ast.RegexpLiteral)
	if !ok {
		t.Fatalf("expected RegexpLiteral, got %T", expr)
	}
	if re.Pattern != "@" {
		t.Fatalf("expected @, got %q", re.Pattern)
	}
}

func TestParsePercentRegexpLiteralPreservesEscapedGreaterThanDelimiter(t *testing.T) {
	expr := parseExpr(t, `%r>\>>`)
	re, ok := expr.(*ast.RegexpLiteral)
	if !ok {
		t.Fatalf("expected RegexpLiteral, got %T", expr)
	}
	if re.Pattern != `\>` {
		t.Fatalf("expected escaped greater-than, got %q", re.Pattern)
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

func TestParsePowerBindsBeforeUnaryMinusAndAssociatesRight(t *testing.T) {
	expr := parseExpr(t, "-2 ** 3 ** 2")
	prefix, ok := expr.(*ast.PrefixExpression)
	if !ok || prefix.Operator != "-" {
		t.Fatalf("expected outer unary minus, got %T (%v)", expr, expr)
	}
	outerPower, ok := prefix.Right.(*ast.InfixExpression)
	if !ok || outerPower.Operator != "**" {
		t.Fatalf("expected power under unary minus, got %T", prefix.Right)
	}
	innerPower, ok := outerPower.Right.(*ast.InfixExpression)
	if !ok || innerPower.Operator != "**" {
		t.Fatalf("expected right-associated power, got %T", outerPower.Right)
	}
}

func TestRangeBindsBeforeTernary(t *testing.T) {
	expr := parseExpr(t, "from..to ? 3 : 4")
	ternary, ok := expr.(*ast.TernaryExpression)
	if !ok {
		t.Fatalf("expected TernaryExpression, got %T (%v)", expr, expr)
	}
	rangeCondition, ok := ternary.Condition.(*ast.RangeExpression)
	if !ok || rangeCondition.Exclusive {
		t.Fatalf("expected inclusive range condition, got %T (%v)", ternary.Condition, ternary.Condition)
	}

	expr = parseExpr(t, "from...to ? 3 : 4")
	ternary, ok = expr.(*ast.TernaryExpression)
	if !ok {
		t.Fatalf("expected TernaryExpression, got %T (%v)", expr, expr)
	}
	rangeCondition, ok = ternary.Condition.(*ast.RangeExpression)
	if !ok || !rangeCondition.Exclusive {
		t.Fatalf("expected exclusive range condition, got %T (%v)", ternary.Condition, ternary.Condition)
	}
}

func TestRangeBindsBeforeKeywordOr(t *testing.T) {
	expr := parseExpr(t, "(a == 1)...(a == 2) or (a == 3)...(a == 4)")
	logical, ok := expr.(*ast.InfixExpression)
	if !ok || logical.Operator != "or" {
		t.Fatalf("expected keyword or outside both ranges, got %T (%v)", expr, expr)
	}
	if _, ok := logical.Left.(*ast.RangeExpression); !ok {
		t.Fatalf("expected left range, got %T", logical.Left)
	}
	if _, ok := logical.Right.(*ast.RangeExpression); !ok {
		t.Fatalf("expected right range, got %T", logical.Right)
	}
}

func TestChainedDefaultParameterStopsBeforeNextParameter(t *testing.T) {
	program := parse(t, `def bar(a=b=c=1, d=2); [a, b, c, d]; end`)
	statement, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[0])
	}
	definition, ok := statement.Expression.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected method definition, got %T", statement.Expression)
	}
	if len(definition.Params) != 2 || len(definition.ParamDefaults) != 2 {
		t.Fatalf("expected two parameters with defaults, got %d/%d", len(definition.Params), len(definition.ParamDefaults))
	}
}

func TestParseBitwiseOperatorsBelowArithmeticWithRubyPrecedence(t *testing.T) {
	expr := parseExpr(t, "1 | 2 ^ 3 & 4 + 5")
	bitOr, ok := expr.(*ast.InfixExpression)
	if !ok || bitOr.Operator != "|" {
		t.Fatalf("expected outer bitwise OR, got %T (%v)", expr, expr)
	}
	bitXor, ok := bitOr.Right.(*ast.InfixExpression)
	if !ok || bitXor.Operator != "^" {
		t.Fatalf("expected XOR under OR, got %T", bitOr.Right)
	}
	bitAnd, ok := bitXor.Right.(*ast.InfixExpression)
	if !ok || bitAnd.Operator != "&" {
		t.Fatalf("expected AND under XOR, got %T", bitXor.Right)
	}
	sum, ok := bitAnd.Right.(*ast.InfixExpression)
	if !ok || sum.Operator != "+" {
		t.Fatalf("expected arithmetic under AND, got %T", bitAnd.Right)
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

func TestParseMethodCallOnIndexInsideArrayLiteral(t *testing.T) {
	expr := parseExpr(t, `[value[-1].to_s]`)
	array, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(array.Elements) != 1 {
		t.Fatalf("expected one-element ArrayLiteral, got %T (%v)", expr, expr)
	}
	call, ok := array.Elements[0].(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "to_s" {
		t.Fatalf("expected to_s MethodCall element, got %T (%v)", array.Elements[0], array.Elements[0])
	}
	if _, ok := call.Receiver.(*ast.IndexExpression); !ok {
		t.Fatalf("expected IndexExpression receiver, got %T", call.Receiver)
	}
}

func TestParseMethodCallOnQualifiedConstantIndexInsideArrayLiteral(t *testing.T) {
	expr := parseExpr(t, `[[key, Schema::Path[key].to_a.join(DOT)]]`)
	outer, ok := expr.(*ast.ArrayLiteral)
	if !ok || len(outer.Elements) != 1 {
		t.Fatalf("expected one-element outer ArrayLiteral, got %T (%v)", expr, expr)
	}
	inner, ok := outer.Elements[0].(*ast.ArrayLiteral)
	if !ok || len(inner.Elements) != 2 {
		t.Fatalf("expected two-element inner ArrayLiteral, got %T (%v)", outer.Elements[0], outer.Elements[0])
	}
	call, ok := inner.Elements[1].(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "join" {
		t.Fatalf("expected chained join call, got %T (%v)", inner.Elements[1], inner.Elements[1])
	}
}

func TestParseDefaultParameterWithParenthesizedCallChain(t *testing.T) {
	program := parse(t, `def build(value=Factory.new(1).build); value; end`)
	definition, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[0])
	}
	method, ok := definition.Expression.(*ast.DefExpression)
	if !ok || len(method.ParamDefaults) != 1 {
		t.Fatalf("expected method with one default, got %T", definition.Expression)
	}
	call, ok := method.ParamDefaults[0].(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "build" {
		t.Fatalf("expected chained build default, got %T (%v)", method.ParamDefaults[0], method.ParamDefaults[0])
	}
}

func TestParseHashRocketValuesWithBraceBlocks(t *testing.T) {
	parse(t, `HOOKS = {
  :before => Hash.new { BeforeHook },
  :after => Hash.new { AfterHook }
}`)
}

func TestParseInfixAfterIfExpressionEnd(t *testing.T) {
	expr := parseExpr(t, `(if true; 1; else; 2; end) || 3`)
	if _, ok := expr.(*ast.InfixExpression); !ok {
		t.Fatalf("expected infix expression, got %T", expr)
	}
	parse(t, `def value; if true; 1; else; 2; end || 3; end`)
}

func TestParseImplicitHashRocketValueOnNextLine(t *testing.T) {
	parse(t, `deprecate(
  "old",
  :message =>
    "use " \
    "new"
)`)
}

func TestParseGroupedParenthesizedCallWithBraceBlock(t *testing.T) {
	parse(t, `def ascending(metadata)
  return unless (group = metadata.fetch(:group) { metadata[:parent] })
  group
end`)
}

func TestParseDefaultParameterWithNestedBraceBlockCall(t *testing.T) {
	parse(t, `def register(name, strategy=Custom.new(Proc.new { |value| yield value }))
  return if name == :global
  strategy
end`)
}

func TestParseExtendAsMethodName(t *testing.T) {
	parse(t, `def extend(mod, *filters)
  [mod, filters]
end`)
}

func TestParseCaseAndExponentiationAsMethodNames(t *testing.T) {
	parse(t, `def case(*args)
  args
end

def **(a, b)
  [a, b]
end

def %(other)
  other
end`)
}

func TestParseIndexWithBraceBlockArgumentBeforeNewline(t *testing.T) {
	parse(t, `def sorted(input)
  if input.empty?
    Hash[input.sort_by { |key, _value| key.to_s }]
  else
    input
  end
end`)
}

func TestParseConstantIndexWithSplattedMappedAndInjectedBlock(t *testing.T) {
	parse(t, `def stringify(arg)
  case arg
  when Hash
    Hash[
      *arg.map { |key, value|
        [key, value]
      }.inject([]) { |result, pair| result + pair }]
  else
    arg
  end
end`)
}

func TestParseMultilineGroupedBooleanWithParenthesizedCalls(t *testing.T) {
	parse(t, `def compatible?(matcher)
  (
    !matcher.respond_to?(:failure_message) &&
    matcher.respond_to?(:legacy_failure_message)
  ) || (
    !matcher.respond_to?(:failure_message_when_negated) &&
    matcher.respond_to?(:legacy_negative_failure_message)
  )
end`)
}

func TestParseCallWithGroupedBlockPassBeforeNewline(t *testing.T) {
	parse(t, `def substitute(host, method, block, *args)
  expectation = host.__send__(method, *args, &(@implementation || block))
  @customizations.each { |customization| customization.call(expectation) }
end`)
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

func TestKeywordNotParenthesizedArgumentEndsBeforeCallChain(t *testing.T) {
	expr := parseExpr(t, "not(true).should be_false")
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "should" {
		t.Fatalf("expected should call, got %T (%v)", expr, expr)
	}
	prefix, ok := call.Receiver.(*ast.PrefixExpression)
	if !ok || prefix.Operator != "not" {
		t.Fatalf("expected not result as receiver, got %T", call.Receiver)
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
	top, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected low-precedence or at the root, got %T", expr)
	}
	if top.Operator != "or" {
		t.Fatalf("expected or at the root, got %s", top.Operator)
	}
	left, ok := top.Left.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected left-associated or expression, got %T", top.Left)
	}
	assign, ok := left.Left.(*ast.AssignExpression)
	if !ok || assign.Name.Value != "x" {
		t.Fatalf("expected assignment to x on the left, got %T", left.Left)
	}
	assignedValue, ok := assign.Value.(*ast.Boolean)
	if !ok || !assignedValue.Value {
		t.Fatalf("expected x to receive true, got %T", assign.Value)
	}
	leftRight, ok := left.Right.(*ast.Boolean)
	if !ok || leftRight.Value {
		t.Fatalf("expected false as the middle operand, got %T", left.Right)
	}
	assignExpr, ok := top.Right.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected right side assignment, got %T", top.Right)
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

func TestParseDottedMethodCallWithNestedCallArgumentAndDoBlock(t *testing.T) {
	expr := parseExpr(t, `Dir.chdir(fixture(__FILE__)) do
  ruby_exe("script.rb")
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
	nested, ok := call.Args[0].(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected nested MethodCall argument, got %T", call.Args[0])
	}
	if nested.Method.Value != "fixture" || len(nested.Args) != 1 {
		t.Fatalf("expected fixture(__FILE__), got %#v", nested)
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

func TestParseArrayKeyHashRocketAsCallArgument(t *testing.T) {
	expr := parseExpr(t, `spawn("cmd", [:out, :err] => "file")`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(call.Args))
	}
	if _, ok := call.Args[1].(*ast.HashLiteral); !ok {
		t.Fatalf("expected second argument to be HashLiteral, got %T", call.Args[1])
	}
}

func TestParseArrayKeyHashRocketAsBareCallArgument(t *testing.T) {
	expr := parseExpr(t, `receiver.delegate [:start, :clean] => :strategy`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		if match, isMatch := expr.(*ast.PatternMatchExpression); isMatch {
			t.Fatalf("expected MethodCall, got PatternMatchExpression with left %T", match.Left)
		}
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected one argument, got %d", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.HashLiteral); !ok {
		t.Fatalf("expected argument to be HashLiteral, got %T", call.Args[0])
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

func TestParseHashRocketWithTopLevelConstantKey(t *testing.T) {
	parse(t, `{::String => :str, ::Array => :array}`)
}

func TestParseHashRocketWithMethodCallKey(t *testing.T) {
	parse(t, `{
  1.minute => 60,
  1.hour + 15.minutes => 4500
}`)
}

func TestParseHashRocketWithStringMethodCallKey(t *testing.T) {
	parse(t, `{ ">".b => '\u003e'.b }`)
}

func TestParseHashRocketWithRegexpKey(t *testing.T) {
	parse(t, `{/features/ => "Cucumber Features"}`)
}

func TestParseBareRaiseAsKeywordParameterDefault(t *testing.T) {
	parse(t, `def trigger(event, range: raise, conflict: nil, **arguments); end`)
}

func TestParseSelfAsMethodDefinitionName(t *testing.T) {
	parse(t, `def self(token); token; end`)
}

func TestParseThenOnLineAfterIfCondition(t *testing.T) {
	parse(t, "value = Array.new.push(if true\nthen :yes\nelse :no\nend)")
}

func TestParseGroupedOrBareRaiseFollowedByCall(t *testing.T) {
	parse(t, `def character_offset(source, byte_offset); (source.byteslice(0, byte_offset) or raise).length; end`)
}

func TestParseGroupedSequenceEndingInNextWithOuterModifier(t *testing.T) {
	parse(t, `[1].each do |value|; (count += 1; next) if value; end`)
}

func TestParseGroupedSequenceEndingInNextInsideLogicalExpression(t *testing.T) {
	parse(t, `loop do
  ready || (advance; next)
  break
end`)
}

func TestParseHashValueBlockCallFollowedByPair(t *testing.T) {
	parse(t, `{ a: lambda { |x| x }, b: 1 }`)
}

func TestParseBareArrowLambdaArgumentInsideBlock(t *testing.T) {
	parse(t, `Builder.new { run ->(env) { env } }`)
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

func TestParseHashLabelsWithoutSpaces(t *testing.T) {
	parse(t, `h = {a:1,text:"x",items:[true,nil]}`)
}

func TestParseHashLiteralWithEmptyGroupedKeyAndValue(t *testing.T) {
	parse(t, "h = {() => ()}")
}

func TestParseHashLiteralWithArrayCallChainKey(t *testing.T) {
	parse(t, `h = {[224, 71].pack("U*") => :home}`)
}

func TestParseCaseProcConditionsKeepClauseBodies(t *testing.T) {
	expr := parseExpr(t, `case "a"
when proc { |value| value == "a" }
  :matched
when proc { |value| value == "b" }
  :other
else
  :missing
end`)
	caseExpr, ok := expr.(*ast.CaseExpression)
	if !ok {
		t.Fatalf("expected CaseExpression, got %T", expr)
	}
	if len(caseExpr.Clauses) != 2 ||
		caseExpr.Clauses[0].Body == nil || len(caseExpr.Clauses[0].Body.Statements) != 1 ||
		caseExpr.Clauses[1].Body == nil || len(caseExpr.Clauses[1].Body.Statements) != 1 {
		t.Fatalf("unexpected Proc case clause bodies: %s", caseExpr.String())
	}
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

func TestGroupedBraceBlockCallCanHavePostfixIfOutsideParentheses(t *testing.T) {
	parse(t, `machines << (owner.state_machine(name) {}) if enabled`)
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

func TestParseImplicitEnsureBraceBlockWithModifierInsideClass(t *testing.T) {
	parse(t, `class EnsureModifierSpec
  def run(call_method)
    Symbol.class_eval { undef :call } if call_method
    yield
  ensure
    Symbol.instance_eval { define_method(:call, call_method) } if call_method
  end
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

func TestParseBareSuperAsFirstBareCallArgument(t *testing.T) {
	expr := parseExpr(t, `Regexp.union super, Regexp.union(%w(one two))`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "union" || len(call.Args) != 2 {
		t.Fatalf("expected Regexp.union with two arguments, got %T %#v", expr, expr)
	}
	superArg, ok := call.Args[0].(*ast.SuperExpression)
	if !ok || !superArg.ImplicitArgs {
		t.Fatalf("expected implicit super first argument, got %T %#v", call.Args[0], call.Args[0])
	}
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

func TestParseMethodDefinitionWithDefaultAfterNewline(t *testing.T) {
	parse(t, `def normalize_component(component, character_class=
    CharacterClassesRegexps::RESERVED_AND_UNRESERVED,
    leave_encoded='')
end`)
}

func TestParseMethodDefinitionWithConstantReceiver(t *testing.T) {
	program := parse(t, `def TARGET.defs_method
  self
end
after_definition`)
	if len(program.Statements) != 2 {
		t.Fatalf("expected definition and following statement, got %d", len(program.Statements))
	}
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	if _, ok := defn.Receiver.(*ast.Constant); !ok {
		t.Fatalf("expected constant receiver, got %T", defn.Receiver)
	}
}

func TestParseMultiAssignWithIndexTargets(t *testing.T) {
	parse(t, `object[:a], object[:b] = :a, :b`)
}

func TestParseMultiAssignWithGroupedAccessorTargets(t *testing.T) {
	parse(t, `(object.a, object.b), c = [:a, :b], nil`)
}

func TestParseParenthesizedMultiAssignTargets(t *testing.T) {
	expr := parseExpr(t, `(before, after) = [:first, :last]`)
	assignment, ok := expr.(*ast.MultiAssignExpression)
	if !ok {
		t.Fatalf("expected MultiAssignExpression, got %T", expr)
	}
	if len(assignment.Targets) != 2 {
		t.Fatalf("expected two targets, got %d", len(assignment.Targets))
	}
	for i, target := range assignment.Targets {
		if _, ok := target.(*ast.Identifier); !ok {
			t.Fatalf("target %d should be an identifier, got %T", i, target)
		}
	}
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

func TestParseBareCallStringArgumentsAcrossNewlinesAfterCommas(t *testing.T) {
	expr := parseExpr(t, `fail ::Concurrent::ConcurrentUpdateError,
     "update failed",
     "retry the update"`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "fail" {
		t.Fatalf("expected fail MethodCall, got %T: %s", expr, expr.String())
	}
	if len(call.Args) != 3 {
		t.Fatalf("expected 3 args, got %d: %s", len(call.Args), call.String())
	}
}

func TestParseExtendWithScopedConstantArgument(t *testing.T) {
	expr := parseExpr(t, `extend Utility::EngineDetector`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "extend" || len(call.Args) != 1 {
		t.Fatalf("expected one-argument extend call, got %T: %s", expr, expr.String())
	}
	if _, ok := call.Args[0].(*ast.ConstantResolution); !ok {
		t.Fatalf("expected scoped constant argument, got %T: %s", call.Args[0], call.Args[0].String())
	}
}

func TestParseBareCallWithTopLevelConstantArgument(t *testing.T) {
	expr := parseExpr(t, `left.merge ::Net::HTTPResponse::CODE_TO_OBJ`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "merge" || len(call.Args) != 1 {
		t.Fatalf("expected one-argument merge call, got %T: %s", expr, expr.String())
	}
	resolution, ok := call.Args[0].(*ast.ConstantResolution)
	if !ok || resolution.Name == nil || resolution.Name.Value != "CODE_TO_OBJ" {
		t.Fatalf("expected top-level constant argument, got %T: %s", call.Args[0], call.Args[0].String())
	}
}

func TestParseConstantResolutionAfterMethodCallWithoutSpace(t *testing.T) {
	expr := parseExpr(t, `factory.build::Result`)
	resolution, ok := expr.(*ast.ConstantResolution)
	if !ok || resolution.Name == nil || resolution.Name.Value != "Result" {
		t.Fatalf("expected constant resolution, got %T: %s", expr, expr.String())
	}
	if _, ok := resolution.Left.(*ast.MethodCall); !ok {
		t.Fatalf("expected method call receiver, got %T: %s", resolution.Left, resolution.Left.String())
	}
}

func TestParseEndMethodCallInIfModifierDoesNotCloseDefinition(t *testing.T) {
	program := parse(t, `class LengthLike
  def initialize(options)
    if range = true
      options[:maximum] = 1 if range.end
    end
    options[:minimum] = 1
  end
end`)
	stmt := program.Statements[0].(*ast.ExpressionStatement)
	classExpr := stmt.Expression.(*ast.ClassExpression)
	if len(classExpr.Body.Statements) != 1 {
		t.Fatalf("expected only the method definition in class body, got %d statements", len(classExpr.Body.Statements))
	}
}

func TestParseScopedSingletonDefinitionWithKeywordMethodName(t *testing.T) {
	expr := parseExpr(t, `def Flags::true()
  true
end`)
	definition, ok := expr.(*ast.DefExpression)
	if !ok || definition.Receiver == nil || definition.Name == nil || definition.Name.Value != "true" {
		t.Fatalf("expected Flags::true singleton definition, got %T: %s", expr, expr.String())
	}
}

func TestParseMultilineWhenConditionsAfterComma(t *testing.T) {
	expr := parseExpr(t, `case :ancestor
when :following, :following_sibling,
     :ancestor, :ancestor_or_self
  :matched
else
  :other
end`)
	caseExpr, ok := expr.(*ast.CaseExpression)
	if !ok || len(caseExpr.Clauses) != 1 || len(caseExpr.Clauses[0].Conditions) != 4 {
		t.Fatalf("expected four multiline when conditions, got %T: %s", expr, expr.String())
	}
}

func TestParseBareCallArgumentEndsBeforeLogicalAnd(t *testing.T) {
	expr := parseExpr(t, `arg.respond_to? :read and arg.respond_to? :eof?`)
	infix, ok := expr.(*ast.InfixExpression)
	if !ok || infix.Operator != "and" {
		t.Fatalf("expected top-level logical and, got %T: %s", expr, expr.String())
	}
	left, ok := infix.Left.(*ast.MethodCall)
	if !ok || left.Method == nil || left.Method.Value != "respond_to?" || len(left.Args) != 1 {
		t.Fatalf("expected one-argument respond_to? call, got %T: %s", infix.Left, infix.Left.String())
	}
}

func TestParseBareDefinedEndsBeforeLogicalAnd(t *testing.T) {
	expr := parseExpr(t, `used if defined? parent and parent`)
	modifier, ok := expr.(*ast.IfExpression)
	if !ok || !modifier.Modifier {
		t.Fatalf("expected if modifier, got %T: %s", expr, expr.String())
	}
	infix, ok := modifier.Condition.(*ast.InfixExpression)
	if !ok || infix.Operator != "and" {
		t.Fatalf("expected logical and outside defined?, got %T: %s", modifier.Condition, modifier.Condition.String())
	}
	if _, ok := infix.Left.(*ast.DefinedExpression); !ok {
		t.Fatalf("expected defined? on left side, got %T: %s", infix.Left, infix.Left.String())
	}
}

func TestParseKeywordNamedMethodDefinitionAndCall(t *testing.T) {
	parse(t, `class MaybeValue
  def or(other)
    other
  end

  def then(*args, &block)
    args
  end

  def rescue(&block)
    self.then(block)
  end
end

value = MaybeValue.new
[value.or(1), value.then(2), value.rescue {}]`)
}

func TestParseMethodDefinitionPassedToBareCallKeepsNestedDoBlock(t *testing.T) {
	parse(t, `internal def collect(items)
  items.each do |item|
    consume(item)
  end
end`)
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

func TestParseImplicitBeginRescueElseEnsureInNestedBlock(t *testing.T) {
	program := parse(t, `1.times do
  Object.new do
    :body
  rescue StandardError
    :rescued
  else
    :successful
  ensure
    :ensured
  end
end`)
	outerCall := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	innerCall := outerCall.Block.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	beginExpr := innerCall.Block.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.BeginExpression)
	if beginExpr.Else == nil || len(beginExpr.Else.Statements) != 1 {
		t.Fatalf("expected one implicit rescue else statement, got %#v", beginExpr.Else)
	}
	if beginExpr.Ensure == nil || len(beginExpr.Ensure.Statements) != 1 {
		t.Fatalf("expected one implicit rescue ensure statement, got %#v", beginExpr.Ensure)
	}
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

func TestParseYieldWithAnonymousRestForwarding(t *testing.T) {
	program := parse(t, `def capture(*, **)
  yield(*, **)
end`)
	defn := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefExpression)
	stmt := defn.Body.Statements[0].(*ast.ExpressionStatement)
	yield := stmt.Expression.(*ast.YieldExpression)

	if len(yield.Args) != 2 {
		t.Fatalf("expected positional and keyword splats, got %d args", len(yield.Args))
	}
	first, ok := yield.Args[0].(*ast.SplatExpression)
	if !ok || first.Token.Type != lexer.MULTIPLY {
		t.Fatalf("expected anonymous positional splat, got %#v", yield.Args[0])
	}
	second, ok := yield.Args[1].(*ast.SplatExpression)
	if !ok || second.Token.Type != lexer.POW {
		t.Fatalf("expected anonymous keyword splat, got %#v", yield.Args[1])
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

func TestParseBareCallKeepsDoBlockOutsideDottedArgument(t *testing.T) {
	program := parse(t, `refine Array.singleton_class do
  def ===(value)
    true
  end
end`)
	call, ok := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "refine" || call.Block == nil || len(call.Args) != 1 {
		t.Fatalf("expected refine call with one argument and trailing block, got %T: %v", program.Statements[0], program.Statements[0])
	}
	argument, ok := call.Args[0].(*ast.MethodCall)
	if !ok || argument.Method == nil || argument.Method.Value != "singleton_class" || argument.Block != nil {
		t.Fatalf("expected singleton_class argument without block, got %T: %v", call.Args[0], call.Args[0])
	}
}

func TestParsePinnedRangeExpressionInHashPattern(t *testing.T) {
	expr := parseExpr(t, `case {released_at: Time.new(2018, 12, 25)}
in {released_at: ^(Time.new(2010)..Time.new(2020))}
  true
end`)
	caseExpr, ok := expr.(*ast.CaseExpression)
	if !ok || len(caseExpr.Clauses) != 1 || len(caseExpr.Clauses[0].Conditions) != 1 {
		t.Fatalf("expected one case pattern clause, got %T: %s", expr, expr.String())
	}
	match, ok := caseExpr.Clauses[0].Conditions[0].(*ast.PatternMatchExpression)
	if !ok {
		t.Fatalf("expected pattern match expression, got %T", caseExpr.Clauses[0].Conditions[0])
	}
	want := "{ released_at : ^ ( Time.new( 2010 ) .. Time.new( 2020 ) ) }"
	if match.Pattern != want {
		t.Fatalf("expected pattern %q, got %q", want, match.Pattern)
	}
}

func TestParsePatternGuardEndingWithBraceBlock(t *testing.T) {
	expr := parseExpr(t, `case []
in String then :string
in Array if [].all? { _1 in String | Integer }
  :array
else
  :other
end`)
	caseExpr, ok := expr.(*ast.CaseExpression)
	if !ok || len(caseExpr.Clauses) != 2 || caseExpr.Else == nil {
		t.Fatalf("expected two guarded case clauses and an else, got %T: %s", expr, expr.String())
	}
}

func TestParseMethodContinuesAfterDoBlockEndingWithOrAssignHash(t *testing.T) {
	expr := parseExpr(t, `module M
  def self.capture(defaults: nil, **kw)
    defaults.each_pair do |key, value|
      @values[key] ||= {}
    end
    kw
  end
end`)
	moduleExpr, ok := expr.(*ast.ModuleExpression)
	if !ok || len(moduleExpr.Body.Statements) != 1 {
		t.Fatalf("expected module containing only the method, got %T: %s", expr, expr.String())
	}
	stmt, ok := moduleExpr.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected method expression statement, got %T", moduleExpr.Body.Statements[0])
	}
	method, ok := stmt.Expression.(*ast.DefExpression)
	if !ok || len(method.Body.Statements) != 2 {
		t.Fatalf("expected method to continue after do block, got %T: %s", stmt.Expression, stmt.Expression.String())
	}
}

func TestParseSafeNavigationChainStartingOnNextLine(t *testing.T) {
	expr := parseExpr(t, `value
  &.strip
  &.upcase`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || !call.Safe || call.Method.Value != "upcase" {
		t.Fatalf("expected multiline safe-navigation chain, got %T: %s", expr, expr.String())
	}
	receiver, ok := call.Receiver.(*ast.MethodCall)
	if !ok || !receiver.Safe || receiver.Method.Value != "strip" {
		t.Fatalf("expected safe strip receiver, got %T: %s", call.Receiver, call.Receiver.String())
	}
}

func TestParseNestedEndHookStaysInsideOuterBlock(t *testing.T) {
	program := parse(t, `END { puts :first }; END { puts :before; END { puts :nested }; puts :after }; END { puts :last }`)
	if len(program.Statements) != 3 {
		t.Fatalf("expected three top-level END calls, got %d: %s", len(program.Statements), program.String())
	}
	stmt, ok := program.Statements[1].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[1])
	}
	outer, ok := stmt.Expression.(*ast.MethodCall)
	if !ok || outer.Method == nil || outer.Method.Value != "END" || outer.Block == nil {
		t.Fatalf("expected second top-level END call with block, got %T: %s", stmt.Expression, stmt.Expression.String())
	}
	if len(outer.Block.Statements) != 3 {
		t.Fatalf("expected nested END between two puts calls, got %d statements: %s", len(outer.Block.Statements), outer.Block.String())
	}
	innerStmt, ok := outer.Block.Statements[1].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected nested expression statement, got %T", outer.Block.Statements[1])
	}
	inner, ok := innerStmt.Expression.(*ast.MethodCall)
	if !ok || inner.Method == nil || inner.Method.Value != "END" || inner.Block == nil {
		t.Fatalf("expected nested END call with block, got %T: %s", innerStmt.Expression, innerStmt.Expression.String())
	}
}

func TestParseSpacedGroupedArgumentKeepsDotChainInsideArgument(t *testing.T) {
	expr := parseExpr(t, `double (5).to_s`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "double" || len(call.Args) != 1 {
		t.Fatalf("expected double call with one argument, got %T: %s", expr, expr.String())
	}
	argument, ok := call.Args[0].(*ast.MethodCall)
	if !ok || argument.Method == nil || argument.Method.Value != "to_s" {
		t.Fatalf("expected .to_s inside the argument, got %T: %s", call.Args[0], call.Args[0].String())
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

func TestParseNestedDoBlockInsideLambdaKeepsTrailingBlockOnCall(t *testing.T) {
	program := parse(t, `describe :shared, shared: true do
  guard -> {
    with_timezone "UTC" do
      true
    end
  } do
    it "x" do
      1
    end
  end
end`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected one top-level statement, got %d: %s", len(program.Statements), program.String())
	}
	describeCall := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	if describeCall.Block == nil || len(describeCall.Block.Statements) != 1 {
		t.Fatalf("expected guard inside describe block, got %s", describeCall.String())
	}
	guardCall, ok := describeCall.Block.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.MethodCall)
	if !ok || guardCall.Method.Value != "guard" || guardCall.Block == nil {
		t.Fatalf("expected guard with trailing block, got %T: %s", describeCall.Block.Statements[0], describeCall.Block.String())
	}
}

func TestParseBlockWithEmptyPipes(t *testing.T) {
	parse(t, "@y.z { || 1 }")
}

func TestParseCatchBraceBlockWithSemicolonExpressions(t *testing.T) {
	expr := parseExpr(t, "catch(:tag) { 1; :last }")
	catchExpr, ok := expr.(*ast.CatchExpression)
	if !ok {
		t.Fatalf("expected CatchExpression, got %T", expr)
	}
	if catchExpr.Body == nil || len(catchExpr.Body.Statements) != 2 {
		t.Fatalf("expected two catch body statements, got %#v", catchExpr.Body)
	}
}

func TestParseCatchWithExplicitBlockPass(t *testing.T) {
	expr := parseExpr(t, "catch(:halt, &block)")
	catchExpr, ok := expr.(*ast.CatchExpression)
	if !ok {
		t.Fatalf("expected CatchExpression, got %T", expr)
	}
	if !catchExpr.HasBlock || catchExpr.BlockPass == nil || catchExpr.Label == nil || catchExpr.Label.String() != ":halt" {
		t.Fatalf("expected catch label and explicit block pass, got %#v", catchExpr)
	}
}

func TestParseCatchDoBlockWithSingleIdentifier(t *testing.T) {
	expr := parseExpr(t, "catch :tag do\nwork\nend")
	catchExpr, ok := expr.(*ast.CatchExpression)
	if !ok {
		t.Fatalf("expected CatchExpression, got %T", expr)
	}
	if catchExpr.Body == nil || len(catchExpr.Body.Statements) != 1 {
		t.Fatalf("expected one catch body statement, got %#v", catchExpr.Body)
	}
}

func TestParseCatchInsideMethodBlockKeepsFollowingStatementInOrder(t *testing.T) {
	expr := parseExpr(t, `it "clears" do
  catch :exit do
    begin
      raise "exception"
    rescue
      throw :exit
    end
  end
  $!.should be_nil
end`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Block == nil {
		t.Fatalf("expected method call with block, got %T", expr)
	}
	if len(call.Block.Statements) != 2 {
		t.Fatalf("expected catch followed by assertion, got %d statements: %s", len(call.Block.Statements), call.Block.String())
	}
	first := call.Block.Statements[0].(*ast.ExpressionStatement).Expression
	if _, ok := first.(*ast.CatchExpression); !ok {
		t.Fatalf("expected first statement to be catch, got %T", first)
	}
}

func TestParseBlockLocalAfterSemicolonIsNotAParameter(t *testing.T) {
	expr := parseExpr(t, `[1].each { |; glark| glark }`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Block == nil {
		t.Fatalf("expected method block, got %T", expr)
	}
	if len(call.Block.Params) != 0 || len(call.Block.BlockLocals) != 1 || call.Block.BlockLocals[0] != "glark" {
		t.Fatalf("unexpected block parameter metadata: params=%v locals=%v", call.Block.Params, call.Block.BlockLocals)
	}
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

func TestParseMultiAssignmentWithTrailingAnonymousSplatTarget(t *testing.T) {
	expr := parseExpr(t, "other, * = node")
	assignment, ok := expr.(*ast.MultiAssignExpression)
	if !ok {
		t.Fatalf("expected MultiAssignExpression, got %T", expr)
	}
	if len(assignment.Targets) != 2 {
		t.Fatalf("expected two targets, got %d", len(assignment.Targets))
	}
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

func TestParsePositionalParameterNamedPrepend(t *testing.T) {
	parse(t, `def fold(prepend = 0)
  prepend = 1
  prepend
end`)
}

func TestParseDefaultParameterContinuesAfterGroupedOperand(t *testing.T) {
	parse(t, `def detailed(highlight_no_color: (ENV["NO_COLOR"] || "") != "")
  highlight_no_color
end`)
}

func TestParseSingleLineMethodEndingWithBraceBlock(t *testing.T) {
	parse(t, `class Config
  def to_h; data.to_h { [_1, send(_1)] } end
end`)
}

func TestParseConstantAssignmentWithSingleLineDoBlock(t *testing.T) {
	parse(t, `module Types
  TABLE = Hash.new do |hash, type| type => Proc | nil; safe { type } end
end`)
}

func TestParseRationalComparisonInModifierCondition(t *testing.T) {
	parse(t, `value = (hash.fetch(key.to_r, nil) if key.respond_to?(:to_r) && key.to_r != 0r) || nil`)
}

func TestParseRationalHashRocketKey(t *testing.T) {
	parse(t, `{0.4r => true}`)
}

func TestParseScalarRightwardPatternAssignment(t *testing.T) {
	expr := parseExpr(t, `type => Proc | nil`)
	if _, ok := expr.(*ast.PatternMatchExpression); !ok {
		t.Fatalf("expected rightward PatternMatchExpression, got %T", expr)
	}
}

func TestParseBareUppercasePredicateAsMethodCall(t *testing.T) {
	expr := parseExpr(t, `Integer?`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "Integer?" {
		t.Fatalf("expected uppercase predicate method call, got %T %#v", expr, expr)
	}
}

func TestParseReservedInWhenAndElseMethodDefinitions(t *testing.T) {
	parse(t, `class ReservedMethods
  def in(value)
    value
  end

  def when(value)
    value
  end

  def else(value)
    value
  end
end`)
}

func TestParseKeywordArgNamedUndef(t *testing.T) {
	expr := parseExpr(t, `value.encode(undef: :replace)`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.KeywordArgs) != 1 || call.KeywordArgs[0].Name != "undef" {
		t.Fatalf("expected undef keyword argument, got %#v", call.KeywordArgs)
	}
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

func TestParseIndexKeywordShorthandArgs(t *testing.T) {
	parse(t, `Command[tag:, name: "IDLE"]`)
}

func TestParseIndexMergesMultipleKeywordArguments(t *testing.T) {
	expr := parseExpr(t, `registry[value, complete: complete, registered: registered]`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected positional value and one keyword hash, got %d args", len(call.Args))
	}
	hash, ok := call.Args[1].(*ast.HashLiteral)
	if !ok || len(hash.Pairs) != 2 {
		t.Fatalf("expected merged two-entry keyword hash, got %#v", call.Args[1])
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

func TestParseMultilineAnonymousBlockForwardingArgument(t *testing.T) {
	parse(t, `def forward(&)
  target(
    :value,
    &
  )
end`)
}

func TestParseMultilineKeywordOnlyMethodParameters(t *testing.T) {
	parse(t, `def initialize(
  namespace_separator: ".",
  resolver: Object.new,
  registry: Object.new
)
end`)
}

func TestParseForwardArgumentsMethodCapturesAllArgumentKinds(t *testing.T) {
	expr := parseExpr(t, `def forward(...); target(...); end`)
	definition, ok := expr.(*ast.DefExpression)
	if !ok {
		t.Fatalf("expected DefExpression, got %T", expr)
	}
	if definition.RestParam == nil || definition.RestParam.Value != "__rgo_forward_args" {
		t.Fatalf("unexpected forwarded positional capture: %#v", definition.RestParam)
	}
	if definition.KeywordRestParam == nil || definition.KeywordRestParam.Value != "__rgo_forward_kwargs" {
		t.Fatalf("unexpected forwarded keyword capture: %#v", definition.KeywordRestParam)
	}
	if definition.BlockParam == nil || definition.BlockParam.Value != "__rgo_forward_block" {
		t.Fatalf("unexpected forwarded block capture: %#v", definition.BlockParam)
	}
}

func TestParseMatchOperatorMethodDefinition(t *testing.T) {
	parse(t, `class FalseClass
  def =~(o)
    o == false
  end
end`)
}

func TestParseUnaryBooleanOperatorMethodDefinitions(t *testing.T) {
	parse(t, `class Predicate
  def !
    :negated
  end

  def ~
    :inverted
  end
end`)
}

func TestParseEndlessMethodDefinition(t *testing.T) {
	parse(t, `def greet(person) = "Hi, ".dup.concat person`)
}

func TestParseKeywordMethodNamesUsedByGems(t *testing.T) {
	parse(t, `def defined?; true; end`)
	parse(t, `def options.alias(from, to) = [from, to]`)
	parse(t, `def options.alias(from, to) = (dict = topdict(from) ; dict[to] = dict[from])`)
	parse(t, `def options.topdict(name) = (name.length > 1 ? top.long : top.short)
def options.alias(from, to) = (dict = topdict(from) ; dict[to] = dict[from])`)
}

func TestParseReservedBooleanOperatorKeywordLabels(t *testing.T) {
	parse(t, `visitor.(not: true, and: false, or: true)`)
	parse(t, `{not: true, and: false, or: true}`)
}

func TestParseMultilineCallChainAfterNestedFinalDoubleSplatArgument(t *testing.T) {
	parse(t, `options = opts.update(
  path: path, **tokens, **lookup_options(input: input)
).to_h`)
}

func TestParseSafeNavigationCallShorthand(t *testing.T) {
	parse(t, `before_steps[name]&.each { |step| step&.(result) }`)
	parse(t, `steps[name]&.(result)`)
}

func TestParseAliasSetterAndBracketMethodNames(t *testing.T) {
	parse(t, `alias multipart_part_limit= multipart_file_limit=`)
	parse(t, `alias [] fetch`)
	parse(t, `alias []= store`)
}

func TestParseReservedKeywordParameterNames(t *testing.T) {
	parse(t, `def delegate(to: nil, private: nil); [to, private]; end`)
	parse(t, `->(private: nil) { private }`)
	parse(t, `proc { |private: nil| private }`)
	parse(t, `def translate(raise: false); translate_key(:key, raise); raise Disabled if false; end`)
	parse(t, `def translate_key(key, throw, raise); handle_exception((throw && :throw || raise && :raise), key); end`)
	parse(t, `def call_class(method, public, safe); [method, public, safe]; end`)
}

func TestParseRequiredPositionalParametersAfterOptionalParameter(t *testing.T) {
	parse(t, `def ruby_version(version = RUBY_VERSION, comparison, major, minor, patch); end`)
}

func TestParseRaiseWithExplicitBacktrace(t *testing.T) {
	parse(t, `raise error, "message", error.backtrace`)
	parse(t, `raise UnexpectedError, "message", error.backtrace, cause: error`)
	parse(t, `raise Error,
  "first line" \
  " second line"`)
}

func TestParseMultilinePostfixConditionsUsedByMinitest(t *testing.T) {
	parse(t, `opts.on "--bisect" if
  File.basename($0).match?(/minitest/)`)
	parse(t, `filtered_results.reject!(&:skipped?) unless
  options[:verbose] or options[:show_skips]`)
	parse(t, `opts.on "--bisect" if
  File.basename($0).match?(/minitest/)

opts.on "--include PATTERN" do |value|
  options[:include] = value
end

def opts.topdict(name) = (name.length > 1 ? top.long : top.short)
def opts.alias(from, to) = (dict = topdict(from) ; dict[to] = dict[from])`)
	parse(t, `def self.process_args args = []
  OptionParser.new do |opts|
    def opts.alias(from, to) = (dict = topdict(from) ; dict[to] = dict[from])
  end
end`)
	parse(t, `def self.process_args args = []
  options = { :io => $stdout }
  OptionParser.new do |opts|
    opts.on "-s", "--seed SEED", Integer, "seed" do |m|
      options[:seed] = m
    end
    opts.on "--bisect" if
      File.basename($0).match?(/minitest/)
    opts.on "-i", "--include PATTERN" do |a|
      options[:include] = a
    end
    opts.on "-e", "--exclude PATTERN" do |a|
      options[:exclude] = a
    end
    def opts.topdict(name) = (name.length > 1 ? top.long : top.short)
    def opts.alias(from, to) = (dict = topdict(from) ; dict[to] = dict[from])
  end
end`)
	parse(t, `extra << "skipped" if
  results.any?(&:skipped?) unless
  options[:verbose]    or
  options[:show_skips] or
  ENV["MT_NO_SKIP_MSG"]`)
	parse(t, `class SummaryReporter
  def summary
    extra << "skipped" if
      results.any?(&:skipped?) unless
      options[:verbose]    or
      options[:show_skips] or
      ENV["MT_NO_SKIP_MSG"]
    "summary" % [count, extra.join]
  end
end
class CompositeReporter
end`)
	parse(t, `class SummaryReporter < StatisticsReporter
  def start
    super
    self.sync = io.respond_to? :"sync="
    self.old_sync, io.sync = io.sync, true if self.sync
  end
end`)
	parse(t, `class SummaryReporter < StatisticsReporter
  def aggregated_results io
    filtered_results = results.dup
    filtered_results.reject!(&:skipped?) unless
      options[:verbose] or options[:show_skips]
    filtered_results.each_with_index { |result, i|
      next if skip.include? result.result_code
      io.puts "%s" % [result]
    }
    io
  end
end`)
	parse(t, `class SummaryReporter < StatisticsReporter
  def start
    super
    self.sync = io.respond_to? :"sync="
    self.old_sync, io.sync = io.sync, true if self.sync
  end

  def report
    super
    io.puts unless options[:verbose]
    aggregated_results io
  end

  def statistics
    "Finished in %.6fs" % [total_time]
  end

  def aggregated_results io
    filtered_results = results.dup
    filtered_results.reject!(&:skipped?) unless
      options[:verbose] or options[:show_skips]
    filtered_results.each_with_index { |result, i|
      next if skip.include? result.result_code
      io.puts "%s" % [result]
    }
    io
  end

  def summary
    extra = []
    extra << "warnings" if options[:Werror]
    extra << "skipped" if
      results.any?(&:skipped?) unless
      options[:verbose] or
      options[:show_skips] or
      ENV["MT_NO_SKIP_MSG"]
    "summary" % [extra.join]
  end
end

class CompositeReporter
end`)
	parse(t, `value = if system "gdiff", __FILE__, __FILE__ then
  "gdiff -u"
elsif system "diff", __FILE__, __FILE__ then
  "diff -u"
else
  nil
end`)
}

func TestParseNestedIfAssignmentBeforeOuterElsif(t *testing.T) {
	parse(t, `if values
  entry = if deep
    transform(entry)
  else
    interpolate(entry)
  end
elsif entry.is_a?(String)
  raise Error
end`)
}

func TestParseWhileConditionEndingInNegatedParenthesizedCall(t *testing.T) {
	parse(t, `class Queue
  def swim(k)
    while k > 1 && ! ordered?(k / 2, k) do
      k = k / 2
    end
  end
end`)
}

func TestParsePrivateEndlessMethodDefinition(t *testing.T) {
	parse(t, `private def instance_variables_to_inspect = []`)
}

func TestParsePrivateMethodDefinitionContainingBlockParameters(t *testing.T) {
	parse(t, `private def define_autoloads_for_dir(dir, mod, external:)
  fs.ls(dir) do |basename, abspath, ftype|
    [basename, abspath, ftype]
  end
end`)
	parse(t, `private def visit_file(cref, file)
  if autoload_path = cref.autoload? || registered?(cref)
    if extension?(autoload_path)
      shadowed_files << file
    else
      promote(file)
    end
  elsif cref.defined?
    shadowed_files << file
    if location = cref.location
      log(location)
    else
      log(file)
    end
  else
    define_autoload(cref, file)
  end
end`)
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

func TestParseForAsSingletonMethodName(t *testing.T) {
	parse(t, `class Timestamp
  def self.for(value, offset = :preserve)
    value
  end
end`)
}

func TestParseMultilineDefaultMethodParameters(t *testing.T) {
	parse(t, `def timezone(identifier, latitude = nil,
             longitude = nil, description = nil)
  [identifier, latitude, longitude, description]
end`)
}

func TestParseCommandWrappedMethodDefinitionInsideConditional(t *testing.T) {
	parse(t, `if RUBY_VERSION >= "3.0.0"
  def method_missing(name, *args, **kwargs, &block)
  end
elsif RUBY_VERSION >= "2.7.0"
  ruby2_keywords def method_missing(name, *args, &block)
  end
else
  def method_missing(name, *args, &block)
  end
end`)
}

func TestParseMultilineMultiAssignmentValue(t *testing.T) {
	parse(t, `magic, version, count =
  ["TZif", "2", 1]`)
}

func TestParseMultilineMultiAssignmentValuesSeparatedAfterComma(t *testing.T) {
	parse(t, `inflated,
plain =
  entries.select { |_, value| value[:inflated] }.freeze,
  entries.select { |_, value| !value[:inflated] }.freeze`)
}

func TestParseIncludeAsMethodName(t *testing.T) {
	parse(t, `def include(*filenames)
  filenames.each { |filename| include(filename.to_s) }
end`)
	parse(t, `def prepend(*args, &block)
  resolve
  @items.send(:prepend, *args, &block)
end`)
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

func TestParseBeginKeywordMethodNameBeforeComparison(t *testing.T) {
	parse(t, `if result[0].begin == 0 && result[0].end == -1
  raise "invalid range"
end`)
}

func TestParseDefinedWithoutParentheses(t *testing.T) {
	parse(t, `(defined? a = 10).should == "assignment"`)
	parse(t, `(not defined? qqq).should == true`)
}

func TestParseGroupedDefinedMultipleAssignment(t *testing.T) {
	l := lexer.New(`defined?((left, right = 1, 2))`)
	p := New(l)
	program := p.ParseProgram()
	expr := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.DefinedExpression).Expression
	if _, ok := expr.(*ast.MultiAssignExpression); !ok {
		t.Fatalf("expected MultiAssignExpression, got %T", expr)
	}
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

func TestParseCaseKeywordMethodNameAfterDot(t *testing.T) {
	parse(t, `Sequel.case({filter => 1}, nil)`)
	parse(t, `Sequel.case({{expression => nil} => null_order}, 1)`)
	parse(t, `Sequel./(left, Sequel.*(right, 2))`)
}

func TestParseIfWithAssignedCallsInGroupedBooleanOperand(t *testing.T) {
	parse(t, `if (v0 = values[0]).is_a?(Array) && ((v1 = values[1]).is_a?(Array) || v1.is_a?(Dataset) || v1.is_a?(LiteralString))
  [v0, v1]
end`)
}

func TestPostfixIfDoesNotConsumeOuterIfEnd(t *testing.T) {
	program := parse(t, `def adjust(sub)
  if sub.length == 1 && (range = sub.first).is_a?(Range)
    value = range.end
    value -= 1 if range.exclude_end? && value.is_a?(Integer)
    value
  else
    2
  end
end`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected one definition, got %d statements", len(program.Statements))
	}
	expr := program.Statements[0].(*ast.ExpressionStatement).Expression
	definition := expr.(*ast.DefExpression)
	outer := definition.Body.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.IfExpression)
	if outer.Alternative == nil {
		t.Fatal("expected postfix if to preserve outer if alternative")
	}
}

func TestParseNegatedBraceBlockCallBeforeBooleanInfix(t *testing.T) {
	parse(t, `if !keys.any?{|key| opts[key]} && opts[:uri]
  opts
end`)
}

func TestParseBraceBlockCallBeforeBooleanOr(t *testing.T) {
	expr := parseExpr(t, `yield_value{1} || []`)
	infix, ok := expr.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T (%v)", expr, expr)
	}
	if call, ok := infix.Left.(*ast.MethodCall); !ok || call.Block == nil {
		t.Fatalf("expected block call on left, got %T (%v)", infix.Left, infix.Left)
	}
}

func TestParseParenthesizedRaiseArguments(t *testing.T) {
	program := parse(t, `raise(ArgumentError, "bad")`)
	raise, ok := program.Statements[0].(*ast.RaiseExpression)
	if !ok {
		t.Fatalf("expected RaiseExpression, got %T", program.Statements[0])
	}
	if raise.Error == nil || raise.Message == nil {
		t.Fatalf("expected error and message arguments, got %#v", raise)
	}
}

func TestParseParenthesizedRaiseArgumentsWithTrailingComma(t *testing.T) {
	program := parse(t, `options[:bounds] || raise(
  ArgumentError,
  "missing bounds",
)

next_value`)
	statement := program.Statements[0].(*ast.ExpressionStatement)
	infix, ok := statement.Expression.(*ast.InfixExpression)
	if !ok {
		t.Fatalf("expected InfixExpression, got %T", statement.Expression)
	}
	raise, ok := infix.Right.(*ast.RaiseExpression)
	if !ok || raise.Error == nil || raise.Message == nil || raise.Backtrace != nil {
		t.Fatalf("expected two raise arguments and no backtrace, got %T (%#v)", infix.Right, raise)
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected following statement after blank line, got %d", len(program.Statements))
	}
}

func TestParseMultilineParenthesizedRaiseArgument(t *testing.T) {
	program := parse(t, `raise(
  "unexpected value "\
    "type is #{value.class}"
)`)
	raise, ok := program.Statements[0].(*ast.RaiseExpression)
	if !ok || raise.Error == nil {
		t.Fatalf("expected raise with a multiline argument, got %T (%#v)", program.Statements[0], raise)
	}
}

func TestParseParenthesizedRaiseWithNestedCallBeforeMethodEnd(t *testing.T) {
	parse(t, `def fail_with(message)
  raise(ArgumentError.new(message).tap { |error| error.set_backtrace(caller(1)) })
end`)
}

func TestParseParenthesizedRaiseWithNestedCallAndPostfixUnless(t *testing.T) {
	parse(t, `def validate(args)
  raise(ArgumentError.new("wrong arguments")) unless args.empty?
end`)
}

func TestParseTripleNestedParenthesizedCallBeforeDotChain(t *testing.T) {
	parse(t, `pluralize(underscore(demodulize(name))).to_sym`)
}

func TestParseBareRaiseAsTernaryBranch(t *testing.T) {
	parse(t, `raise_on_failure ? raise : value`)
}

func TestParseRegexpAfterAndKeyword(t *testing.T) {
	parse(t, `if parameters and /\Aq=([\d.]+)/ =~ parameters
  quality = 1
end`)
}

func TestParseRegexpAfterElseOnSameLine(t *testing.T) {
	parse(t, `case value; when 1 then /one/; else /other/o; end`)
}

func TestIndexAssignmentStringKeyMatchingParameterRemainsLiteral(t *testing.T) {
	standalone := parse(t, `hash["href"]`)
	standaloneStatement := standalone.Statements[0].(*ast.ExpressionStatement)
	standaloneIndex, ok := standaloneStatement.Expression.(*ast.IndexExpression)
	if !ok {
		t.Fatalf("expected standalone index expression, got %T", standaloneStatement.Expression)
	}
	if literal, ok := standaloneIndex.Index.(*ast.StringLiteral); !ok || literal.Value != "href" {
		t.Fatalf("expected standalone string literal index, got %T: %v", standaloneIndex.Index, standaloneIndex.Index)
	}

	source := `def assign_link(hash, href); hash["href"] = href; end`
	l := lexer.New(source)
	p := New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	definition, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected definition expression statement, got %T", program.Statements[0])
	}
	def, ok := definition.Expression.(*ast.DefExpression)
	if !ok || len(def.Body.Statements) == 0 {
		t.Fatalf("expected method definition body, got %T", definition.Expression)
	}
	statement, ok := def.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected assignment expression statement, got %T", def.Body.Statements[0])
	}
	assign, ok := statement.Expression.(*ast.AssignExpression)
	if !ok {
		t.Fatalf("expected index assignment, got %T", statement.Expression)
	}
	if literal, ok := assign.Index.(*ast.StringLiteral); !ok || literal.Value != "href" {
		t.Fatalf("expected string literal index, got %T: %v", assign.Index, assign.Index)
	}
}

func TestParseLowPrecedenceNotAssignmentCondition(t *testing.T) {
	parse(t, `if not data = write_session(request)
  warn(data)
elsif deferred
  defer
end`)
}

func TestParseVisibilityKeywordSetterMethodName(t *testing.T) {
	parse(t, `def public=(value)
  value
end`)
}

func TestInfixRightHandIfConsumesItsOwnEndBeforeOuterElsif(t *testing.T) {
	parse(t, `if protocol == 3
  prelude << if pass
    ["HELLO", pass]
  else
    ["HELLO"]
  end
elsif pass
  prelude << ["AUTH", pass]
end`)
}

func TestParseIndexArgumentEndingInDoBlock(t *testing.T) {
	parse(t, `Hash[reply.map do |key, value|
  [key, value]
end]`)
	parse(t, `reply = Hash[reply.map do |key, value|
  [key, value]
end]`)
	parse(t, `def redis_info(reply, command)
  send_command([]) do |reply|
    if reply.is_a?(String)
      if command && command.to_s == "commandstats"
        reply = Hash[reply.map do |key, value|
          [key, Hash[value]]
        end]
      end
    end
    reply
  end
end`)
	parse(t, `Hash[reply.map do |key, value|
  value = value.split(",").map { |entry| entry.split("=") }
  [key[/^prefix_(.*)$/, 1], Hash[value]]
end]`)
}

func TestParseParenthesizedCallWithAddedZipMapExpressions(t *testing.T) {
	parse(t, `_join_table_dataset(opts).where(left_keys.zip(left_primary_keys.map{|key| get_column_value(key)}) + right_keys.zip(opts.right_primary_key_methods.map{|key| object.get_column_value(key)})).delete`)
}

func TestParseNestedParenthesizedCallAtEndOfHashArgument(t *testing.T) {
	parse(t, `clone(:with=>((options[:with]||EMPTY_ARRAY) + [Hash[opts].merge!(:recursive=>true, :name=>name, :dataset=>nonrecursive.union(recursive, {:all=>opts[:union_all] != false, :from_self=>false}))]).freeze)`)
}

func TestParseGroupedTernaryHashRocketKey(t *testing.T) {
	parse(t, `{(keys.length == 1 ? keys.first : keys)=>object.select(*methods).exclude(source.from_value_pairs(methods.zip([]), :OR))}`)
}

func TestParseCaseBranchWithAssignedBraceBlockCallCondition(t *testing.T) {
	parse(t, `records.each do |record|
	  case key
	  when Array
	    if (value = key.map{|part| record.get_column_value(part)}) && value.all?
	      id_map[value] << record
	    end
	  when Symbol
	    if value = record.get_column_value(key)
	      id_map[value] << record
	    end
	  else
	    raise Error, "unsupported"
	  end
	end`)
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

func TestParseSuperKeywordMethodNameAfterDot(t *testing.T) {
	expr := parseExpr(t, `method.super`)
	call, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if call.Method == nil || call.Method.Value != "super" {
		t.Fatalf("expected super method name, got %#v", call.Method)
	}
}

func TestParseSuperKeywordMethodDefinition(t *testing.T) {
	parse(t, `class MethodWrapper
  def super(times = 1)
    times
  end
end`)
}

func TestParsePrivateRedoKeywordMethodDefinition(t *testing.T) {
	parse(t, `class LineEditor
  private def redo(key)
    key
  end
end`)
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

func TestParseAliasUnaryOperatorMethodName(t *testing.T) {
	expr := parseExpr(t, `alias -@ opposite`)
	aliasExpr, ok := expr.(*ast.AliasExpression)
	if !ok {
		t.Fatalf("expected AliasExpression, got %T", expr)
	}
	if aliasExpr.New.String() != "-@" || aliasExpr.Old.String() != "opposite" {
		t.Fatalf("unexpected unary alias: %v", aliasExpr)
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

func TestBraceBlockBindsToBareCallArgument(t *testing.T) {
	expr := parseExpr(t, `using Module.new { refine Array do; alias_method :original_count, :count; end }`)
	usingCall, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected using MethodCall, got %T", expr)
	}
	if usingCall.Block != nil {
		t.Fatal("expected brace block to bind to Module.new argument, not using")
	}
	if len(usingCall.Args) != 1 {
		t.Fatalf("expected one using argument, got %d", len(usingCall.Args))
	}
	newCall, ok := usingCall.Args[0].(*ast.MethodCall)
	if !ok || newCall.Method == nil || newCall.Method.Value != "new" || newCall.Block == nil {
		t.Fatalf("expected Module.new argument with block, got %#v", usingCall.Args[0])
	}
	if len(newCall.Block.Statements) != 1 {
		t.Fatalf("expected one statement in Module.new block, got %d", len(newCall.Block.Statements))
	}
	statement, ok := newCall.Block.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", newCall.Block.Statements[0])
	}
	refineCall, ok := statement.Expression.(*ast.MethodCall)
	if !ok || refineCall.Method == nil || refineCall.Method.Value != "refine" || refineCall.Block == nil {
		t.Fatalf("expected refine call with do block, got %#v", statement.Expression)
	}
}

func TestBraceBlockContainingDoBlockKeepsFollowingStatement(t *testing.T) {
	program := parse(t, `refinery = Module.new { refine Array do; end }; refinery`)
	if len(program.Statements) != 2 {
		t.Fatalf("expected two statements, got %d", len(program.Statements))
	}
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

func TestNestedWhileInLambdaMatcherDoesNotLeaveOuterEnd(t *testing.T) {
	program := parse(t, `
describe "outer" do
  argf [] do
    -> { while source.readchar; end }.should raise_error(EOFError)
  end
end
	`)
	if len(program.Statements) != 1 {
		t.Fatalf("expected one outer describe statement, got %d", len(program.Statements))
	}
}

func TestKeywordNotCanBeCalledAfterDot(t *testing.T) {
	expr := parseExpr(t, `relation.where.not(discarded_at: nil)`)
	notCall, ok := expr.(*ast.MethodCall)
	if !ok || notCall.Method == nil || notCall.Method.Value != "not" {
		t.Fatalf("expected dotted not method call, got %#v", expr)
	}
	whereCall, ok := notCall.Receiver.(*ast.MethodCall)
	if !ok || whereCall.Method == nil || whereCall.Method.Value != "where" {
		t.Fatalf("expected where receiver, got %#v", notCall.Receiver)
	}
}

func TestInequalityOperatorCanBeCalledAfterSafeNavigation(t *testing.T) {
	expr := parseExpr(t, `table_name&.!= node.left.relation.name`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || !call.Safe || call.Method == nil || call.Method.Value != "!=" {
		t.Fatalf("expected safe-navigation inequality call, got %#v", expr)
	}
}

func TestBareCallSplatCanBeFollowedByKeywordArgument(t *testing.T) {
	expr := parseExpr(t, `delegate *Counter::OPTIONS, to: :counter`)
	call, ok := expr.(*ast.MethodCall)
	if !ok || call.Method == nil || call.Method.Value != "delegate" {
		t.Fatalf("expected delegate call, got %#v", expr)
	}
	if len(call.Args) != 1 || len(call.KeywordArgs) != 1 || call.KeywordArgs[0].Name != "to" {
		t.Fatalf("unexpected delegate arguments: %#v", call)
	}
}
