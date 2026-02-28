package lexer

import (
	"testing"
)

// helper: collect all tokens from input
func tokenize(input string) []Token {
	l := New(input)
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens
}

// helper: collect non-NEWLINE, non-EOF tokens
func tokenizeClean(input string) []Token {
	all := tokenize(input)
	var result []Token
	for _, tok := range all {
		if tok.Type != NEWLINE && tok.Type != EOF {
			result = append(result, tok)
		}
	}
	return result
}

func TestIntegerLiterals(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
		literal string
	}{
		{"0", INT, "0"},
		{"42", INT, "42"},
		{"123456", INT, "123456"},
		{"1_000_000", INT, "1000000"},
		{"0xFF", INT, "0xFF"},
		{"0xDEAD_BEEF", INT, "0xDEADBEEF"},
		{"0b1010", INT, "0b1010"},
		{"0b1111_0000", INT, "0b11110000"},
		{"0o777", INT, "0o777"},
		{"0o755", INT, "0o755"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected type %s, got %s", tt.tokType, toks[0].Type)
			}
			if toks[0].Literal != tt.literal {
				t.Errorf("expected literal %q, got %q", tt.literal, toks[0].Literal)
			}
		})
	}
}

func TestFloatLiterals(t *testing.T) {
	tests := []struct {
		input   string
		literal string
	}{
		{"1.5", "1.5"},
		{"3.14", "3.14"},
		{"0.5", "0.5"},
		{"1_000.5", "1000.5"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != FLOAT {
				t.Errorf("expected FLOAT, got %s", toks[0].Type)
			}
			if toks[0].Literal != tt.literal {
				t.Errorf("expected literal %q, got %q", tt.literal, toks[0].Literal)
			}
		})
	}
}

func TestStringLiterals(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		literal string
	}{
		{"double quoted", `"hello"`, "hello"},
		{"single quoted", `'hello'`, "hello"},
		{"empty double", `""`, ""},
		{"empty single", `''`, ""},
		{"with spaces", `"hello world"`, "hello world"},
		{"escape newline", `"hello\nworld"`, "hello\nworld"},
		{"escape tab", `"hello\tworld"`, "hello\tworld"},
		{"escape quote", `"say \"hi\""`, `say "hi"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != STRING {
				t.Errorf("expected STRING, got %s", toks[0].Type)
			}
			if toks[0].Literal != tt.literal {
				t.Errorf("expected literal %q, got %q", tt.literal, toks[0].Literal)
			}
		})
	}
}

func TestArithmeticOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"+", PLUS},
		{"-", MINUS},
		{"*", MULTIPLY},
		{"%", MOD},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestCompoundAssignment(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"+=", PLUS_ASSIGN},
		{"-=", MINUS_ASSIGN},
		{"*=", MULTIPLY_ASSIGN},
		{"/=", DIVIDE_ASSIGN},
		{"**=", POW_ASSIGN},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestComparisonOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"==", EQUAL},
		{"!=", BANG_EQUAL},
		{"<", LESS_THAN},
		{">", GREATER_THAN},
		{"<=", LESS_THAN_OR_EQUAL},
		{">=", GREATER_THAN_OR_EQUAL},
		{"<=>", SPACESHIP},
		{"===", EQUAL3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestLogicalOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"&&", AND},
		{"||", OR},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestPowerOperator(t *testing.T) {
	toks := tokenizeClean("**")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != POW {
		t.Errorf("expected POW, got %s", toks[0].Type)
	}
}

func TestKeywords(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"if", IF},
		{"unless", UNLESS},
		{"elsif", ELSIF},
		{"else", ELSE},
		{"case", CASE},
		{"when", WHEN},
		{"def", DEF},
		{"end", END},
		{"class", CLASS},
		{"module", MODULE},
		{"return", RETURN},
		{"break", BREAK},
		{"next", NEXT},
		{"while", WHILE},
		{"until", UNTIL},
		{"for", FOR},
		{"do", DO},
		{"in", IN},
		{"begin", BEGIN},
		{"rescue", RESCUE},
		{"ensure", ENSURE},
		{"raise", RAISE},
		{"super", SUPER},
		{"self", SELF},
		{"yield", YIELD},
		{"true", TRUE},
		{"false", FALSE},
		{"nil", NIL},
		{"and", AND2},
		{"or", OR2},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestIdentifiers(t *testing.T) {
	tests := []struct {
		input   string
		literal string
	}{
		{"foo", "foo"},
		{"bar_baz", "bar_baz"},
		{"hello123", "hello123"},
		{"_private", "_private"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != IDENT {
				t.Errorf("expected IDENT, got %s", toks[0].Type)
			}
			if toks[0].Literal != tt.literal {
				t.Errorf("expected literal %q, got %q", tt.literal, toks[0].Literal)
			}
		})
	}
}

func TestBrackets(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"(", LPAREN},
		{")", RPAREN},
		{"{", LBRACE},
		{"}", RBRACE},
		{"[", LBRACKET},
		{"]", RBRACKET},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestDotOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{".", DOT},
		{"..", DOT2},
		{"...", DOT3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

func TestSymbols(t *testing.T) {
	tests := []struct {
		input   string
		literal string
	}{
		{":foo", ":foo"},
		{":bar_baz", ":bar_baz"},
		{":hello123", ":hello123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != SYMBOL {
				t.Errorf("expected SYMBOL, got %s", toks[0].Type)
			}
			if toks[0].Literal != tt.literal {
				t.Errorf("expected literal %q, got %q", tt.literal, toks[0].Literal)
			}
		})
	}
}

func TestDoubleColon(t *testing.T) {
	toks := tokenizeClean("::")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != COLON2 {
		t.Errorf("expected COLON2, got %s", toks[0].Type)
	}
}

func TestInstanceVariable(t *testing.T) {
	toks := tokenizeClean("@name")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != AT {
		t.Errorf("expected AT, got %s", toks[0].Type)
	}
	if toks[0].Literal != "@name" {
		t.Errorf("expected literal %q, got %q", "@name", toks[0].Literal)
	}
}

func TestClassVariable(t *testing.T) {
	toks := tokenizeClean("@@count")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != AT2 {
		t.Errorf("expected AT2, got %s", toks[0].Type)
	}
	if toks[0].Literal != "@@count" {
		t.Errorf("expected literal %q, got %q", "@@count", toks[0].Literal)
	}
}

func TestGlobalVariable(t *testing.T) {
	toks := tokenizeClean("$stdout")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != DOLLAR {
		t.Errorf("expected DOLLAR, got %s", toks[0].Type)
	}
	if toks[0].Literal != "$stdout" {
		t.Errorf("expected literal %q, got %q", "$stdout", toks[0].Literal)
	}
}

func TestArrow(t *testing.T) {
	toks := tokenizeClean("=>")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != ARROW {
		t.Errorf("expected ARROW, got %s", toks[0].Type)
	}
}

func TestMatchOperators(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
	}{
		{"=~", MATCH},
		{"!~", NOT_EQUAL},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			toks := tokenizeClean(tt.input)
			if len(toks) != 1 {
				t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
			}
			if toks[0].Type != tt.tokType {
				t.Errorf("expected %s, got %s", tt.tokType, toks[0].Type)
			}
		})
	}
}

// Test tokenizing a complete expression
func TestSimpleExpression(t *testing.T) {
	toks := tokenizeClean("1 + 2")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}

	expected := []struct {
		tokType TokenType
		literal string
	}{
		{INT, "1"},
		{PLUS, "+"},
		{INT, "2"},
	}

	for i, exp := range expected {
		if toks[i].Type != exp.tokType {
			t.Errorf("token[%d]: expected type %s, got %s", i, exp.tokType, toks[i].Type)
		}
		if toks[i].Literal != exp.literal {
			t.Errorf("token[%d]: expected literal %q, got %q", i, exp.literal, toks[i].Literal)
		}
	}
}

func TestMethodCallExpression(t *testing.T) {
	toks := tokenizeClean(`"hello".upcase`)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{STRING, "hello"},
		{DOT, "."},
		{IDENT, "upcase"},
	}

	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}

	for i, exp := range expected {
		if toks[i].Type != exp.tokType {
			t.Errorf("token[%d]: expected type %s, got %s", i, exp.tokType, toks[i].Type)
		}
		if toks[i].Literal != exp.literal {
			t.Errorf("token[%d]: expected literal %q, got %q", i, exp.literal, toks[i].Literal)
		}
	}
}

func TestVariableAssignment(t *testing.T) {
	toks := tokenizeClean("x = 5")
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{IDENT, "x"},
		{ASSIGN, "="},
		{INT, "5"},
	}

	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}

	for i, exp := range expected {
		if toks[i].Type != exp.tokType {
			t.Errorf("token[%d]: expected type %s, got %s", i, exp.tokType, toks[i].Type)
		}
		if toks[i].Literal != exp.literal {
			t.Errorf("token[%d]: expected literal %q, got %q", i, exp.literal, toks[i].Literal)
		}
	}
}

func TestPutsExpression(t *testing.T) {
	toks := tokenizeClean(`puts "hello"`)
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{IDENT, "puts"},
		{STRING, "hello"},
	}

	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}

	for i, exp := range expected {
		if toks[i].Type != exp.tokType {
			t.Errorf("token[%d]: expected type %s, got %s", i, exp.tokType, toks[i].Type)
		}
		if toks[i].Literal != exp.literal {
			t.Errorf("token[%d]: expected literal %q, got %q", i, exp.literal, toks[i].Literal)
		}
	}
}

func TestCommaAndSemicolon(t *testing.T) {
	toks := tokenizeClean(",;")
	if len(toks) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != COMMA {
		t.Errorf("expected COMMA, got %s", toks[0].Type)
	}
	if toks[1].Type != SEMICOLON {
		t.Errorf("expected SEMICOLON, got %s", toks[1].Type)
	}
}

func TestEOF(t *testing.T) {
	toks := tokenize("")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token (EOF), got %d", len(toks))
	}
	if toks[0].Type != EOF {
		t.Errorf("expected EOF, got %s", toks[0].Type)
	}
}

func TestNewlineHandling(t *testing.T) {
	toks := tokenize("a\nb")
	// Should have: IDENT(a), NEWLINE, IDENT(b), EOF
	types := make([]TokenType, len(toks))
	for i, tok := range toks {
		types[i] = tok.Type
	}

	if len(toks) < 3 {
		t.Fatalf("expected at least 3 tokens, got %d: %v", len(toks), types)
	}

	if toks[0].Type != IDENT {
		t.Errorf("token[0]: expected IDENT, got %s", toks[0].Type)
	}
	if toks[1].Type != NEWLINE {
		t.Errorf("token[1]: expected NEWLINE, got %s", toks[1].Type)
	}
	if toks[2].Type != IDENT {
		t.Errorf("token[2]: expected IDENT, got %s", toks[2].Type)
	}
}

func TestCommentSkipping(t *testing.T) {
	toks := tokenizeClean("a # this is a comment")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != IDENT || toks[0].Literal != "a" {
		t.Errorf("expected IDENT 'a', got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestStringInterpolation(t *testing.T) {
	toks := tokenizeClean(`"hello #{name}"`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Errorf("expected STRING, got %s", toks[0].Type)
	}
	// The interpolation should be preserved in the literal
	if toks[0].Literal != "hello #{name}" {
		t.Errorf("expected literal %q, got %q", "hello #{name}", toks[0].Literal)
	}
}

func TestBangOperator(t *testing.T) {
	toks := tokenizeClean("!")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != BANG {
		t.Errorf("expected BANG, got %s", toks[0].Type)
	}
}

func TestQuestionMark(t *testing.T) {
	toks := tokenizeClean("?")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != QUESTION {
		t.Errorf("expected QUESTION, got %s", toks[0].Type)
	}
}

func TestLambdaArrow(t *testing.T) {
	toks := tokenizeClean("->")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != ARROW {
		t.Errorf("expected ARROW, got %s", toks[0].Type)
	}
}
