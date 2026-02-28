package parser

import (
	"testing"

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
