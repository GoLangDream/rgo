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

func TestEndMarkerPermanentlyEndsLexing(t *testing.T) {
	tokens := tokenizeClean("before\n__END__\nafter")
	if len(tokens) != 1 || tokens[0].Type != IDENT || tokens[0].Literal != "before" {
		t.Fatalf("expected only the token before __END__, got %v", tokens)
	}

	tokens = tokenizeClean("value__END__ = 1")
	if len(tokens) == 0 || tokens[0].Literal != "value__END__" {
		t.Fatalf("embedded __END__ must remain an identifier, got %v", tokens)
	}
}

func TestIntegerLiterals(t *testing.T) {
	tests := []struct {
		input   string
		tokType TokenType
		literal string
	}{
		{"0", INT, "0"},
		{"02", INT, "02"},
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

func TestBackslashNewlineContinuesLine(t *testing.T) {
	toks := tokenizeClean("left == \\\n  right")
	expected := []TokenType{IDENT, EQUAL, IDENT}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s", i, typ, toks[i].Type)
		}
	}
}

func TestLeftShiftAfterIdentifierIsNotHeredoc(t *testing.T) {
	toks := tokenizeClean("r<<i")
	expected := []TokenType{IDENT, LSHIFT, IDENT}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestSpacedIndentedHeredocAfterBareCall(t *testing.T) {
	toks := tokenizeClean("eval <<-CODE\n  case 4\n  else\n    true\n  when 4; false\n  end\n  CODE\n")
	expected := []TokenType{IDENT, STRING}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
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
		{"escape nul", `"hello\0world"`, "hello\x00world"},
		{"octal escapes are raw bytes", `"\303\202"`, "\xC3\x82"},
		{"unicode escape", `"\u3042"`, "あ"},
		{"escape ordinary hash", `"\#@"`, EscapedHashInterpolation + "@"},
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

func TestDoubleQuotedStringKeepsHashCharacter(t *testing.T) {
	toks := tokenizeClean(`"Fiber#kill"`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != "Fiber#kill" {
		t.Fatalf("expected %q, got %q", "Fiber#kill", toks[0].Literal)
	}
}

func TestSquigglyHeredocToken(t *testing.T) {
	toks := tokenizeClean("code = <<~CODE\n    first\n      second\nCODE\n")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
	if toks[2].Type != STRING {
		t.Fatalf("expected heredoc STRING token, got %s %q", toks[2].Type, toks[2].Literal)
	}
	if toks[2].Literal != "first\n  second\n" {
		t.Fatalf("expected common indentation removed, got %q", toks[2].Literal)
	}
}

func TestSquigglyHeredocJoinsEscapedNewline(t *testing.T) {
	toks := tokenizeClean("code = <<~CODE\n  a\n  b\\\n  c\nCODE\n")
	if len(toks) != 3 || toks[2].Literal != "a\nbc\n" {
		t.Fatalf("expected escaped newline to join lines, got %v", toks)
	}
}

func TestHeredocInterpolationFlag(t *testing.T) {
	toks := tokenizeClean("s = <<HERE\n#{value}\nHERE\nsingle = <<'TEXT'\n#{value}\nTEXT\n")
	if len(toks) != 6 {
		t.Fatalf("expected 6 tokens, got %d: %v", len(toks), toks)
	}
	if !toks[2].AllowsInterpolation {
		t.Fatalf("expected unquoted heredoc to allow interpolation")
	}
	if toks[5].AllowsInterpolation {
		t.Fatalf("expected single-quoted heredoc to disable interpolation")
	}
}

func TestQuotedHeredocIdentifierMustEndOnDeclarationLine(t *testing.T) {
	tokens := tokenizeClean("<<\"HERE\n\"\nbody\nHERE\n")
	if len(tokens) == 0 || tokens[0].Type != ILLEGAL {
		t.Fatalf("expected illegal unterminated quoted heredoc identifier, got %v", tokens)
	}
}

func TestHeredocDecodesDoubleQuotedEscapes(t *testing.T) {
	toks := tokenizeClean("s = <<CODE\n\\t# encoding: UTF-8\nCODE\nsingle = <<'TEXT'\n\\t# encoding: UTF-8\nTEXT\n")
	if len(toks) != 6 {
		t.Fatalf("expected 6 tokens, got %d: %v", len(toks), toks)
	}
	if toks[2].Literal != "\t# encoding: UTF-8\n" {
		t.Fatalf("expected decoded tab in heredoc, got %q", toks[2].Literal)
	}
	if toks[5].Literal != "\\t# encoding: UTF-8\n" {
		t.Fatalf("expected single-quoted heredoc to preserve escape, got %q", toks[5].Literal)
	}
}

func TestSquigglyHeredocPreservesMarkerLineSuffix(t *testing.T) {
	toks := tokenizeClean("eval(<<~CODE).should == nil\n  10\nCODE\n")
	expected := []TokenType{IDENT, LPAREN, STRING, RPAREN, DOT, IDENT, EQUAL, NIL}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestIndentedHeredocPreservesKeywordArgumentSuffix(t *testing.T) {
	toks := tokenizeClean("ruby_exe(<<-CODE, args: \"2>&1\")\n  return 10\n  CODE\n")
	expected := []TokenType{IDENT, LPAREN, STRING, COMMA, IDENT, COLON, STRING, RPAREN}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestHeredocMarkerSuffixPreservesDeclarationLine(t *testing.T) {
	toks := tokenizeClean("\n\neval(<<-CODE, __FILE__, __LINE__ + 1)\n  value\nCODE\n")
	for _, tok := range toks {
		if tok.Literal == "__LINE__" {
			if tok.Line != 3 {
				t.Fatalf("expected __LINE__ on line 3, got line %d", tok.Line)
			}
			return
		}
	}
	t.Fatal("expected __LINE__ token")
}

func TestHeredocMarkerSuffixIsSeparatedFromFollowingStatement(t *testing.T) {
	toks := tokenize("ruby_exe(<<-CODE, args: \"2>&1\")\n  return 10\n  CODE\nnext_call\n")
	for i := 0; i < len(toks)-2; i++ {
		if toks[i].Type == RPAREN && toks[i+1].Type == NEWLINE && toks[i+2].Type == IDENT && toks[i+2].Literal == "next_call" {
			return
		}
	}
	t.Fatalf("expected RPAREN NEWLINE next_call token sequence, got %v", toks)
}

func TestRegexpLiteral(t *testing.T) {
	toks := tokenizeClean(`/foo/i`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != REGEXP {
		t.Fatalf("expected REGEXP, got %s", toks[0].Type)
	}
	if toks[0].Literal != `/foo/i` {
		t.Errorf("expected literal /foo/i, got %q", toks[0].Literal)
	}
}

func TestPercentRegexpAllowsBackslashAsDelimiter(t *testing.T) {
	toks := tokenizeClean(`%r\ foo \`)
	if len(toks) != 1 || toks[0].Type != REGEXP || toks[0].Literal != `%r\ foo \` {
		t.Fatalf("expected backslash-delimited regexp, got %v", toks)
	}
}

func TestRegexpWithBacktickQuoteCharClassAndEscapedGlobal(t *testing.T) {
	toks := tokenizeClean("/warning: global variable [`']\\$specs_uninitialized_global_variable' not initialized/")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != REGEXP {
		t.Fatalf("expected REGEXP, got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestInterpolatedRegexpWithNestedRegexp(t *testing.T) {
	toks := tokenizeClean(`/#{/./}/e.encoding`)
	if len(toks) < 3 {
		t.Fatalf("expected regexp followed by method call tokens, got %v", toks)
	}
	if toks[0].Type != REGEXP || toks[0].Literal != `/#{/./}/e` {
		t.Fatalf("expected full interpolated REGEXP, got %s %q", toks[0].Type, toks[0].Literal)
	}
	if toks[1].Type != DOT || toks[2].Literal != "encoding" {
		t.Fatalf("expected .encoding after regexp, got %v", toks[1:3])
	}
}

func TestRegexpControlEscapeTakesPrecedenceOverInterpolation(t *testing.T) {
	toks := tokenizeClean(`/\c#{str}/`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != REGEXP {
		t.Fatalf("expected REGEXP, got %s", toks[0].Type)
	}
	if toks[0].AllowsInterpolation {
		t.Fatalf("expected control escape regexp not to interpolate, got %q", toks[0].Literal)
	}
}

func TestUnterminatedRegexpDoesNotPanic(t *testing.T) {
	toks := tokenize(`/foo`)
	if len(toks) == 0 {
		t.Fatal("expected at least one token")
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

func TestBracePipeStartsBlockWithParameters(t *testing.T) {
	toks := tokenizeClean("{|v| v }")
	expected := []TokenType{LBRACE, BIT_OR, IDENT, BIT_OR, IDENT, RBRACE}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
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
		{":<=>", ":<=>"},
		{`:"foo"`, ":foo"},
		{`:'bar'`, ":bar"},
		{":@hash", ":@hash"},
		{":@@hash", ":@@hash"},
		{":$value", ":$value"},
		{":m=", ":m="},
		{":`", ":`"},
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

func TestSymbolHashRocketWithoutSpaceAfterBangOrQuestionIsIllegal(t *testing.T) {
	for _, input := range []string{":a!=> 1", ":a?=> 1"} {
		t.Run(input, func(t *testing.T) {
			toks := tokenizeClean(input)
			if len(toks) == 0 {
				t.Fatal("expected tokens")
			}
			if toks[0].Type != ILLEGAL {
				t.Fatalf("expected ILLEGAL, got %s %q", toks[0].Type, toks[0].Literal)
			}
		})
	}
}

func TestKeywordBlockParameterColonBeforePipe(t *testing.T) {
	toks := tokenizeClean("proc { |b:| b }")
	expected := []TokenType{IDENT, LBRACE, BIT_OR, IDENT, COLON, BIT_OR, IDENT, RBRACE}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestSlashAfterExpressionIsDivision(t *testing.T) {
	toks := tokenizeClean("2*1/2")
	expected := []TokenType{INT, MULTIPLY, INT, DIVIDE, INT}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestSlashAfterNewlineCanStartRegexp(t *testing.T) {
	toks := tokenize("value\n/bar/")
	if len(toks) < 3 {
		t.Fatalf("expected at least 3 tokens, got %v", toks)
	}
	if toks[2].Type != REGEXP || toks[2].Literal != "/bar/" {
		t.Fatalf("expected regexp after newline, got %s %q", toks[2].Type, toks[2].Literal)
	}
}

func TestCompoundAssignmentTokens(t *testing.T) {
	toks := tokenizeClean("a %= b; a |= b; a &= b; a ^= b; a >>= b; a <<= b")
	expected := []TokenType{
		IDENT, MOD_ASSIGN, IDENT, SEMICOLON,
		IDENT, BIT_OR_ASSIGN, IDENT, SEMICOLON,
		IDENT, BIT_AND_ASSIGN, IDENT, SEMICOLON,
		IDENT, BIT_XOR_ASSIGN, IDENT, SEMICOLON,
		IDENT, RSHIFT_ASSIGN, IDENT, SEMICOLON,
		IDENT, LSHIFT_ASSIGN, IDENT,
	}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestBarePercentEqualsStringAtExpressionStart(t *testing.T) {
	toks := tokenizeClean(`%=hey=`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING || toks[0].Literal != "hey" {
		t.Fatalf("expected percent string, got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestSlashAfterWhenCanStartRegexp(t *testing.T) {
	toks := tokenizeClean("case value\nwhen /foo/\nend")
	found := false
	for _, tok := range toks {
		if tok.Type == REGEXP && tok.Literal == "/foo/" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected regexp token, got %v", toks)
	}
}

func TestLineStartRegexpCanBeginWithSpace(t *testing.T) {
	toks := tokenizeClean("/ foo (?x)/")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != REGEXP || toks[0].Literal != "/ foo (?x)/" {
		t.Fatalf("expected regexp literal, got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestBarePercentString(t *testing.T) {
	toks := tokenizeClean(`%<"utf_16be \u3042">`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != `"utf_16be あ"` {
		t.Fatalf("unexpected literal %q", toks[0].Literal)
	}
}

func TestBarePercentStringWithPunctuationDelimiter(t *testing.T) {
	toks := tokenizeClean(`%^hey #{@ip}^`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != `hey #{@ip}` {
		t.Fatalf("unexpected literal %q", toks[0].Literal)
	}
}

func TestBarePercentStringWithUnderscoreDelimiter(t *testing.T) {
	toks := tokenizeClean(`%_hey #{@ip}_`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != `hey #{@ip}` {
		t.Fatalf("unexpected literal %q", toks[0].Literal)
	}
}

func TestBarePercentStringDelimiterInsideInterpolation(t *testing.T) {
	toks := tokenizeClean(`%@hey #{@ip}@`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != `hey #{@ip}` {
		t.Fatalf("unexpected literal %q", toks[0].Literal)
	}
}

func TestPercentStringWithNestedInterpolationBraces(t *testing.T) {
	toks := tokenizeClean(`%Q{alias :"#{'a' + ''.to_s}" value}`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != `alias :"#{'a' + ''.to_s}" value` {
		t.Fatalf("unexpected literal %q", toks[0].Literal)
	}
}

func TestPercentQStringDecodesDoubleQuotedEscapes(t *testing.T) {
	toks := tokenizeClean(`%Q[A\nB\tC]`)
	if len(toks) != 1 || toks[0].Type != STRING || toks[0].Literal != "A\nB\tC" {
		t.Fatalf("expected decoded percent-Q string, got %v", toks)
	}
}

func TestSafeNavigatorToken(t *testing.T) {
	toks := tokenizeClean("nil&.to_s")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
	if toks[1].Type != SAFE_NAV || toks[1].Literal != "&." {
		t.Fatalf("expected SAFE_NAV token, got %s %q", toks[1].Type, toks[1].Literal)
	}
}

func TestAndAssignToken(t *testing.T) {
	toks := tokenizeClean("obj&.m &&= false")
	if len(toks) != 5 {
		t.Fatalf("expected 5 tokens, got %d: %v", len(toks), toks)
	}
	if toks[3].Type != AND_ASSIGN || toks[3].Literal != "&&=" {
		t.Fatalf("expected AND_ASSIGN token, got %s %q", toks[3].Type, toks[3].Literal)
	}
}

func TestLeadingDotContinuationDoesNotEmitNewline(t *testing.T) {
	toks := tokenize(`"abc"
  .to_s`)
	for _, tok := range toks {
		if tok.Type == NEWLINE {
			t.Fatalf("did not expect NEWLINE before leading dot: %v", toks)
		}
	}
}

func TestSingleQuotedEscapedBackslash(t *testing.T) {
	toks := tokenizeClean(`['\\']`)
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
	if toks[1].Type != STRING || toks[1].Literal != `\` {
		t.Fatalf("expected escaped backslash string, got %s %q", toks[1].Type, toks[1].Literal)
	}
}

func TestSingleQuotedStringPreservesNonUTF8SourceBytes(t *testing.T) {
	toks := tokenizeClean("'\xa7A\xa6n'")
	if len(toks) != 1 || toks[0].Type != STRING {
		t.Fatalf("expected one STRING token, got %v", toks)
	}
	if toks[0].Literal != "\xa7A\xa6n" {
		t.Fatalf("expected raw Big5 bytes, got % x", []byte(toks[0].Literal))
	}
}

func TestSpecialGlobalVariableComma(t *testing.T) {
	toks := tokenizeClean("$, = '_'")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != DOLLAR || toks[0].Literal != "$," {
		t.Fatalf("expected global $, token, got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestSpecialGlobalVariableDot(t *testing.T) {
	toks := tokenizeClean("$. = 0")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != DOLLAR || toks[0].Literal != "$." {
		t.Fatalf("expected global $. token, got %s %q", toks[0].Type, toks[0].Literal)
	}
}

func TestSpecialGlobalVariableStar(t *testing.T) {
	toks := tokenizeClean("$*")
	if len(toks) != 1 || toks[0].Type != DOLLAR || toks[0].Literal != "$*" {
		t.Fatalf("expected global $* token, got %v", toks)
	}
}

func TestSpecialGlobalVariableDoubleQuote(t *testing.T) {
	toks := tokenizeClean(`$" = []`)
	expected := []TokenType{DOLLAR, ASSIGN, LBRACKET, RBRACKET}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	if toks[0].Literal != `$"` {
		t.Fatalf("expected global literal $\", got %q", toks[0].Literal)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
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

func TestUnicodeInstanceVariable(t *testing.T) {
	toks := tokenizeClean("@💙 :@💙")
	expected := []TokenType{AT, SYMBOL}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
	if toks[0].Literal != "@💙" || toks[1].Literal != ":@💙" {
		t.Fatalf("unexpected literals: %q %q", toks[0].Literal, toks[1].Literal)
	}
}

func TestSpecialGlobalVariableSymbols(t *testing.T) {
	toks := tokenizeClean(":$~ :$_ :$0")
	expected := []string{":$~", ":$_", ":$0"}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, lit := range expected {
		if toks[i].Type != SYMBOL || toks[i].Literal != lit {
			t.Fatalf("token %d: expected SYMBOL %q, got %s %q", i, lit, toks[i].Type, toks[i].Literal)
		}
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

func TestMethodCallPercentMethod(t *testing.T) {
	toks := tokenizeClean("obj.%(1)")
	expected := []struct {
		tokType TokenType
		literal string
	}{
		{IDENT, "obj"},
		{DOT, "."},
		{MOD, "%"},
		{LPAREN, "("},
		{INT, "1"},
		{RPAREN, ")"},
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

func TestPercentLowercaseSCreatesSymbolToken(t *testing.T) {
	toks := tokenizeClean("%s{foo bar}")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != SYMBOL {
		t.Fatalf("expected SYMBOL, got %s", toks[0].Type)
	}
	if toks[0].Literal != ":foo bar" {
		t.Fatalf("expected %q, got %q", ":foo bar", toks[0].Literal)
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

func TestCommentWithHexEscapesKeepsNextLineSeparate(t *testing.T) {
	toks := tokenize(`send(@method, Encoding::EUC_JP, undef: :replace)
    # testing for: "\xA4\xA2?\xA4\xA2"
    xA4xA2 = [0xA4, 0xA2].pack('CC')`)
	for i := 0; i < len(toks)-2; i++ {
		if toks[i].Type == RPAREN && toks[i+1].Type == NEWLINE && toks[i+2].Type == IDENT && toks[i+2].Literal == "xA4xA2" {
			return
		}
	}
	t.Fatalf("expected RPAREN NEWLINE xA4xA2 token sequence, got %v", toks)
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

func TestEscapedInterpolationInString(t *testing.T) {
	toks := tokenizeClean(`"value of \#{$DEBUG}"`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Errorf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != "value of "+EscapedHashInterpolation+"{$DEBUG}" {
		t.Errorf("expected literal with escaped interpolation marker, got %q", toks[0].Literal)
	}
}

func TestEscapedOpeningBraceDoesNotInterpolate(t *testing.T) {
	toks := tokenizeClean(`"!@#\{$\}%^&**()"`)
	if len(toks) != 1 || toks[0].Type != STRING {
		t.Fatalf("expected one STRING token, got %v", toks)
	}
	if toks[0].Literal != "!@"+EscapedHashInterpolation+`{$}%^&**()` {
		t.Fatalf("expected escaped opening brace to suppress interpolation, got %q", toks[0].Literal)
	}
}

func TestHexEscapeInStringPreservesRawByte(t *testing.T) {
	toks := tokenizeClean(`"\xFF\xFE"`)
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != STRING {
		t.Fatalf("expected STRING, got %s", toks[0].Type)
	}
	if toks[0].Literal != "\xff\xfe" {
		t.Fatalf("expected raw bytes ff fe, got % x", []byte(toks[0].Literal))
	}
}

func TestHexEscapeInCommandLiteralPreservesRawByte(t *testing.T) {
	toks := tokenizeClean("`echo \\xC2`")
	if len(toks) != 1 || !toks[0].CommandLiteral || toks[0].Literal != "echo \xc2" {
		t.Fatalf("expected command literal with raw byte c2, got %#v", toks)
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

func TestCharacterLiteral(t *testing.T) {
	toks := tokenizeClean("?V ?\\n")
	expected := []string{"V", "\n"}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, lit := range expected {
		if toks[i].Type != STRING || toks[i].Literal != lit {
			t.Fatalf("token %d: expected STRING %q, got %s %q", i, lit, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestControlAndMetaCharacterLiterals(t *testing.T) {
	toks := tokenizeClean(`?\C-z ?\M-z ?\M-\C-z`)
	expected := []string{"\x1a", "\xfa", "\x9a"}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, want := range expected {
		if toks[i].Type != STRING || toks[i].Literal != want {
			t.Fatalf("token %d: expected STRING %x, got %s %x", i, []byte(want), toks[i].Type, []byte(toks[i].Literal))
		}
	}
}

func TestBacktickAfterDotIsMethodName(t *testing.T) {
	toks := tokenizeClean("Kernel.`(obj)")
	expected := []TokenType{CONSTANT, DOT, IDENT, LPAREN, IDENT, RPAREN}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
	if toks[2].Literal != "`" {
		t.Fatalf("expected backtick method name, got %q", toks[2].Literal)
	}
}

func TestLambdaArrow(t *testing.T) {
	toks := tokenizeClean("->")
	if len(toks) != 1 {
		t.Fatalf("expected 1 token, got %d: %v", len(toks), toks)
	}
	if toks[0].Type != MINUS_ARROW {
		t.Errorf("expected MINUS_ARROW, got %s", toks[0].Type)
	}
}

func TestImaginaryNumericSuffixTokens(t *testing.T) {
	tests := []struct {
		input   string
		typ     TokenType
		literal string
	}{
		{"1i", IMAGINARY, "1"},
		{"0.0i", IMAGINARY, "0.0"},
		{"1.0e2i", IMAGINARY, "1.0e2"},
		{"0x2ai", IMAGINARY, "0x2a"},
		{"0b101i", IMAGINARY, "0b101"},
	}

	for _, tt := range tests {
		toks := tokenizeClean(tt.input)
		if len(toks) != 1 {
			t.Fatalf("%s: expected 1 token, got %d: %v", tt.input, len(toks), toks)
		}
		if toks[0].Type != tt.typ {
			t.Fatalf("%s: expected %s, got %s", tt.input, tt.typ, toks[0].Type)
		}
		if toks[0].Literal != tt.literal {
			t.Fatalf("%s: expected literal %q, got %q", tt.input, tt.literal, toks[0].Literal)
		}
	}
}

func TestEndlessRangeBeforeBracketTokens(t *testing.T) {
	toks := tokenizeClean("3..]")
	expected := []TokenType{INT, DOT2, RBRACKET}
	if len(toks) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(toks), toks)
	}
	for i, typ := range expected {
		if toks[i].Type != typ {
			t.Fatalf("token %d: expected %s, got %s (%q)", i, typ, toks[i].Type, toks[i].Literal)
		}
	}
}

func TestSymbolInspectLiteralTokens(t *testing.T) {
	tests := []struct {
		input   string
		literal string
	}{
		{":$-w", ":$-w"},
		{`:"$\\"`, `:$\\`},
		{`:"\<\<"`, `:\<\<`},
		{`:"\$"`, `:\$`},
		{`:[]`, `:[]`},
		{`:[]=`, `:[]=`},
	}

	for _, tt := range tests {
		toks := tokenizeClean(tt.input)
		if len(toks) == 0 {
			t.Fatalf("%s: expected tokens", tt.input)
		}
		if toks[0].Literal != tt.literal {
			t.Fatalf("%s: expected literal %q, got %q (%s)", tt.input, tt.literal, toks[0].Literal, toks[0].Type)
		}
	}
}

func TestIdentifierBeforeNewlineKeepsStartingLine(t *testing.T) {
	l := New("line = __LINE__\np line\n")
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	if tokens[2].Literal != "__LINE__" || tokens[2].Line != 1 {
		t.Fatalf("expected __LINE__ on line 1, got %q line %d", tokens[2].Literal, tokens[2].Line)
	}
	if tokens[4].Literal != "p" || tokens[4].Line != 2 {
		t.Fatalf("expected p on line 2, got %q line %d", tokens[4].Literal, tokens[4].Line)
	}
}
