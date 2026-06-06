package lexer

import (
	"unicode"
	"unicode/utf8"
)

type TokenType string

const (
	ILLEGAL TokenType = "ILLEGAL"
	EOF     TokenType = "EOF"

	EscapedHashInterpolation = "\x00#"

	IDENT    TokenType = "IDENT"
	INT      TokenType = "INT"
	FLOAT    TokenType = "FLOAT"
	RATIONAL TokenType = "RATIONAL"
	STRING   TokenType = "STRING"
	WORDS    TokenType = "WORDS"
	SYMBOL   TokenType = "SYMBOL"
	REGEXP   TokenType = "REGEXP"

	ASSIGN   TokenType = "="
	PLUS     TokenType = "+"
	MINUS    TokenType = "-"
	MULTIPLY TokenType = "*"
	DIVIDE   TokenType = "/"
	MOD      TokenType = "%"
	POW      TokenType = "**"

	PLUS_ASSIGN     TokenType = "+="
	MINUS_ASSIGN    TokenType = "-="
	MULTIPLY_ASSIGN TokenType = "*="
	DIVIDE_ASSIGN   TokenType = "/="
	MOD_ASSIGN      TokenType = "%="
	POW_ASSIGN      TokenType = "**="
	OR_ASSIGN       TokenType = "||="
	AND_ASSIGN      TokenType = "&&="
	BIT_OR_ASSIGN   TokenType = "|="
	BIT_AND_ASSIGN  TokenType = "&="
	BIT_XOR_ASSIGN  TokenType = "^="
	LSHIFT_ASSIGN   TokenType = "<<="
	RSHIFT_ASSIGN   TokenType = ">>="

	BANG       TokenType = "!"
	BANG_EQUAL TokenType = "!="

	EQUAL     TokenType = "=="
	EQUAL3    TokenType = "==="
	NOT_EQUAL TokenType = "!~"
	MATCH     TokenType = "=~"

	LESS_THAN             TokenType = "<"
	LESS_THAN_OR_EQUAL    TokenType = "<="
	GREATER_THAN          TokenType = ">"
	GREATER_THAN_OR_EQUAL TokenType = ">="

	LSHIFT TokenType = "<<"
	RSHIFT TokenType = ">>"

	SPACESHIP TokenType = "<=>"

	TERNARY TokenType = "?"
	THEN    TokenType = "then"

	AND     TokenType = "&&"
	OR      TokenType = "||"
	AND2    TokenType = "and"
	OR2     TokenType = "or"
	BIT_AND TokenType = "&"
	BIT_OR  TokenType = "|"
	BIT_XOR TokenType = "^"
	BIT_NOT TokenType = "~"

	DOT      TokenType = "."
	DOT2     TokenType = ".."
	DOT3     TokenType = "..."
	SAFE_NAV TokenType = "&."

	COMMA       TokenType = ","
	COLON       TokenType = ":"
	COLON2      TokenType = "::"
	SEMICOLON   TokenType = ";"
	ARROW       TokenType = "=>"
	MINUS_ARROW TokenType = "->"

	LPAREN   TokenType = "("
	RPAREN   TokenType = ")"
	LBRACE   TokenType = "{"
	RBRACE   TokenType = "}"
	LBRACKET TokenType = "["
	RBRACKET TokenType = "]"

	QUESTION   TokenType = "?"
	UNDERSCORE TokenType = "_"

	AT     TokenType = "@"
	AT2    TokenType = "@@"
	DOLLAR TokenType = "$"

	BACKSLASH TokenType = "\\"
	PERCENT   TokenType = "%"

	NEWLINE TokenType = "NEWLINE"
	COMMENT TokenType = "COMMENT"

	TRUE  TokenType = "true"
	FALSE TokenType = "false"
	NIL   TokenType = "nil"

	IF     TokenType = "if"
	UNLESS TokenType = "unless"
	ELSIF  TokenType = "elsif"
	ELSE   TokenType = "else"
	CASE   TokenType = "case"
	WHEN   TokenType = "when"

	DEF    TokenType = "def"
	END    TokenType = "end"
	CLASS  TokenType = "class"
	MODULE TokenType = "module"

	RETURN TokenType = "return"
	BREAK  TokenType = "break"
	NEXT   TokenType = "next"
	REDO   TokenType = "redo"
	RETRY  TokenType = "retry"

	WHILE TokenType = "while"
	UNTIL TokenType = "until"
	FOR   TokenType = "for"
	DO    TokenType = "do"
	IN    TokenType = "in"

	BEGIN  TokenType = "begin"
	RESCUE TokenType = "rescue"
	ENSURE TokenType = "ensure"
	RAISE  TokenType = "raise"
	CATCH  TokenType = "catch"
	THROW  TokenType = "throw"

	SUPER TokenType = "super"
	SELF  TokenType = "self"
	YIELD TokenType = "yield"

	DEFINED TokenType = "defined?"
	ALIAS   TokenType = "alias"
	UNDEF   TokenType = "undef"
	INCLUDE TokenType = "include"
	EXTEND  TokenType = "extend"
	PREPEND TokenType = "prepend"

	PUBLIC    TokenType = "public"
	PRIVATE   TokenType = "private"
	PROTECTED TokenType = "protected"

	NIL_METHOD TokenType = "nil?"

	CONSTANT  TokenType = "CONSTANT"
	BLOCK_END TokenType = "END"
	LPIPE     TokenType = "|"
	RPIPE     TokenType = "|"
)

type Token struct {
	Type    TokenType
	Literal string
	// AllowsInterpolation indicates whether this literal should be interpreted as an
	// interpolated string (double-quoted style).
	AllowsInterpolation bool
	CommandLiteral      bool
	Line                int
	Column              int
}

func (t Token) String() string {
	return string(t.Type) + ":" + t.Literal
}

var keywords = map[string]TokenType{
	"|":         LPIPE,
	"if":        IF,
	"unless":    UNLESS,
	"elsif":     ELSIF,
	"else":      ELSE,
	"then":      THEN,
	"case":      CASE,
	"when":      WHEN,
	"def":       DEF,
	"end":       END,
	"class":     CLASS,
	"module":    MODULE,
	"return":    RETURN,
	"break":     BREAK,
	"next":      NEXT,
	"redo":      REDO,
	"retry":     RETRY,
	"while":     WHILE,
	"until":     UNTIL,
	"for":       FOR,
	"do":        DO,
	"in":        IN,
	"begin":     BEGIN,
	"rescue":    RESCUE,
	"ensure":    ENSURE,
	"raise":     RAISE,
	"catch":     CATCH,
	"throw":     THROW,
	"super":     SUPER,
	"self":      SELF,
	"yield":     YIELD,
	"true":      TRUE,
	"false":     FALSE,
	"nil":       NIL,
	"and":       AND2,
	"or":        OR2,
	"not":       BANG,
	"defined?":  DEFINED,
	"END":       END,
	"alias":     ALIAS,
	"undef":     UNDEF,
	"include":   INCLUDE,
	"extend":    EXTEND,
	"prepend":   PREPEND,
	"public":    PUBLIC,
	"private":   PRIVATE,
	"protected": PROTECTED,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	// Constants start with an uppercase letter, including non-ASCII identifiers.
	if startsWithUpper(ident) {
		return CONSTANT
	}
	return IDENT
}

func startsWithUpper(ident string) bool {
	if ident == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(ident)
	return unicode.IsUpper(r)
}
