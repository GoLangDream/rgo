package lexer

type TokenType string

const (
	ILLEGAL TokenType = "ILLEGAL"
	EOF     TokenType = "EOF"

	IDENT  TokenType = "IDENT"
	INT    TokenType = "INT"
	FLOAT  TokenType = "FLOAT"
	STRING TokenType = "STRING"
	SYMBOL TokenType = "SYMBOL"
	REGEXP TokenType = "REGEXP"

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

	AND  TokenType = "&&"
	OR   TokenType = "||"
	AND2 TokenType = "and"
	OR2  TokenType = "or"

	DOT  TokenType = "."
	DOT2 TokenType = ".."
	DOT3 TokenType = "..."

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
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func (t Token) String() string {
	return string(t.Type) + ":" + t.Literal
}

var keywords = map[string]TokenType{
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
	"super":     SUPER,
	"self":      SELF,
	"yield":     YIELD,
	"true":      TRUE,
	"false":     FALSE,
	"nil":       NIL,
	"and":       AND2,
	"or":        OR2,
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
	// Constants start with uppercase letter
	if len(ident) > 0 && ident[0] >= 'A' && ident[0] <= 'Z' {
		return CONSTANT
	}
	return IDENT
}
