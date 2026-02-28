package lexer

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           rune
	line         int
	column       int

	templateNesting uint8
}

func New(input string) *Lexer {
	l := &Lexer{
		input: input,
		line:  1,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
		l.position = l.readPosition
	} else {
		r, w := utf8.DecodeRuneInString(l.input[l.readPosition:])
		l.ch = r
		l.position = l.readPosition
		l.readPosition += w
	}

	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPosition:])
	return r
}

func (l *Lexer) peekCharN(n int) rune {
	pos := l.readPosition
	for i := 0; i < n-1; i++ {
		if pos >= len(l.input) {
			return 0
		}
		_, w := utf8.DecodeRuneInString(l.input[pos:])
		pos += w
	}
	if pos >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[pos:])
	return r
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
		if l.ch == '\n' {
			return
		}
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	for l.ch == '#' {
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	}
}

func (l *Lexer) NewLine() Token {
	l.readChar()
	l.skipWhitespace()
	l.skipComment()
	return Token{
		Type:    NEWLINE,
		Literal: "\n",
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	// Skip inline comments
	if l.ch == '#' {
		l.skipComment()
	}

	var tok Token
	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case 0:
		tok.Type = EOF
		tok.Literal = ""
	case '\n':
		tok.Type = NEWLINE
		tok.Literal = "\n"
		l.readChar()
		l.skipWhitespace()
		l.skipComment()
		return tok
	case '"':
		tok = l.readString(false)
		// readString stops at closing quote; readChar() below consumes it
	case '\'':
		tok = l.readString(true)
		// readString stops at closing quote; readChar() below consumes it
	case '`':
		tok = l.readRawString()
		return tok // readRawString already consumed closing backtick
	case '[':
		tok = newToken(LBRACKET, l.ch)
	case ']':
		tok = newToken(RBRACKET, l.ch)
	case '(':
		tok = newToken(LPAREN, l.ch)
	case ')':
		tok = newToken(RPAREN, l.ch)
	case '{':
		if l.peekChar() == '|' {
			tok = l.makeTwoCharToken(LBRACE, RBRACE)
		} else if l.peekChar() == '-' {
			tok = l.readHashArrow()
			return tok // readHashArrow already advanced
		} else {
			tok = newToken(LBRACE, l.ch)
		}
	case '}':
		tok = newToken(RBRACE, l.ch)
	case ',':
		tok = newToken(COMMA, l.ch)
	case ';':
		tok = newToken(SEMICOLON, l.ch)
	case ':':
		tok = l.readSymbolOrColon()
		return tok // readSymbolOrColon already advanced past content
	case '.':
		if l.peekChar() == '.' {
			if l.peekCharN(2) == '.' {
				tok = l.makeThreeCharToken(DOT3)
			} else {
				tok = l.makeTwoCharToken(DOT, DOT2)
			}
		} else {
			tok = newToken(DOT, l.ch)
		}
	case '/':
		tok = l.readSlashOrRegexp()
		return tok // readSlashOrRegexp already advanced past content
	case '+':
		if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(PLUS, PLUS_ASSIGN)
		} else {
			tok = newToken(PLUS, l.ch)
		}
	case '-':
		if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(MINUS, MINUS_ASSIGN)
		} else if l.peekChar() == '>' {
			tok = l.makeTwoCharToken(MINUS, ARROW)
		} else {
			tok = newToken(MINUS, l.ch)
		}
	case '*':
		if l.peekChar() == '*' {
			if l.peekCharN(2) == '=' {
				tok = l.makeThreeCharToken(POW_ASSIGN)
			} else {
				tok = l.makeTwoCharToken(MULTIPLY, POW)
			}
		} else if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(MULTIPLY, MULTIPLY_ASSIGN)
		} else {
			tok = newToken(MULTIPLY, l.ch)
		}
	case '%':
		if l.peekChar() == 'q' || l.peekChar() == 'Q' || l.peekChar() == 'w' || l.peekChar() == 'W' ||
			l.peekChar() == 'i' || l.peekChar() == 'I' || l.peekChar() == 'r' || l.peekChar() == 's' ||
			l.peekChar() == 'x' || l.peekChar() == 's' {
			tok = l.readPercentString()
			return tok // readPercentString already advanced past content
		} else {
			tok = newToken(MOD, l.ch)
		}
	case '=':
		if l.peekChar() == '=' {
			if l.peekCharN(2) == '=' {
				tok = l.makeThreeCharToken(EQUAL3)
			} else {
				tok = l.makeTwoCharToken(ASSIGN, EQUAL)
			}
		} else if l.peekChar() == '~' {
			tok = l.makeTwoCharToken(ASSIGN, MATCH)
		} else if l.peekChar() == '>' {
			tok = l.makeTwoCharToken(ASSIGN, ARROW)
		} else {
			tok = newToken(ASSIGN, l.ch)
		}
	case '!':
		if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(BANG, BANG_EQUAL)
		} else if l.peekChar() == '~' {
			tok = l.makeTwoCharToken(BANG, NOT_EQUAL)
		} else {
			tok = newToken(BANG, l.ch)
		}
	case '<':
		if l.peekChar() == '=' {
			if l.peekCharN(2) == '>' {
				tok = l.makeThreeCharToken(SPACESHIP)
			} else {
				tok = l.makeTwoCharToken(LESS_THAN, LESS_THAN_OR_EQUAL)
			}
		} else if l.peekChar() == '<' {
			tok = l.readLeftShift()
			return tok // readLeftShift already advanced past content
		} else {
			tok = newToken(LESS_THAN, l.ch)
		}
	case '>':
		if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(GREATER_THAN, GREATER_THAN_OR_EQUAL)
		} else {
			tok = newToken(GREATER_THAN, l.ch)
		}
	case '&':
		if l.peekChar() == '&' {
			tok = l.makeTwoCharToken(BANG, AND)
		} else if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(BANG, BANG)
		} else {
			tok = newToken(BANG, l.ch)
		}
	case '|':
		if l.peekChar() == '|' {
			tok = l.makeTwoCharToken(BANG, OR)
		} else if l.peekChar() == '=' {
			tok = l.makeTwoCharToken(BANG, BANG)
		} else {
			tok = newToken(BANG, l.ch)
		}
	case '?':
		tok = newToken(QUESTION, l.ch)
	case '@':
		tok = l.readVariable()
		return tok // readVariable already advanced past content
	case '$':
		tok = l.readGlobalVariable()
		return tok // readGlobalVariable already advanced past content
	case '_':
		if len(l.input[l.position:]) >= 5 && l.input[l.position:l.position+5] == "__END__" {
			tok.Type = EOF
			tok.Literal = ""
		} else {
			tok = l.readIdentifier()
			return tok // readIdentifier already advanced past content
		}
	default:
		if isLetter(l.ch) {
			tok = l.readIdentifier()
			return tok
		} else if isDigit(l.ch) {
			tok = l.readNumber()
			return tok
		} else {
			tok = newToken(ILLEGAL, l.ch)
		}
	}

	l.readChar()
	return tok
}

func newToken(tokenType TokenType, ch rune) Token {
	return Token{
		Type:    tokenType,
		Literal: string(ch),
	}
}

func (l *Lexer) makeTwoCharToken(t1, t2 TokenType) Token {
	ch := l.ch
	l.readChar()
	return Token{
		Type:    t2,
		Literal: string(ch) + string(l.ch),
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) makeThreeCharToken(t TokenType) Token {
	ch := l.ch
	l.readChar()
	ch2 := l.ch
	l.readChar()
	return Token{
		Type:    t,
		Literal: string(ch) + string(ch2) + string(l.ch),
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readHashArrow() Token {
	ch := l.ch
	l.readChar()
	if l.ch == '>' {
		l.readChar()
		return Token{
			Type:    ARROW,
			Literal: string(ch) + string(l.ch),
			Line:    l.line,
			Column:  l.column,
		}
	}
	return Token{
		Type:    LBRACE,
		Literal: string(ch),
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readIdentifier() Token {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}

	lit := l.input[position:l.position]
	return Token{
		Type:    LookupIdent(lit),
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readVariable() Token {
	l.readChar()

	if l.ch == '@' {
		l.readChar()
		position := l.position
		for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}

		lit := l.input[position:l.position]
		if len(lit) == 0 {
			return newToken(AT, '@')
		}

		return Token{
			Type:    AT2,
			Literal: "@@" + lit,
			Line:    l.line,
			Column:  l.column,
		}
	}

	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[position:l.position]
	if len(lit) == 0 {
		return newToken(AT, '@')
	}

	return Token{
		Type:    AT,
		Literal: "@" + lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readGlobalVariable() Token {
	l.readChar() // skip '$'
	position := l.position

	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[position:l.position]
	if len(lit) == 0 {
		// Special global variables like $-, $!, etc.
		if l.ch == '-' {
			l.readChar()
			position = l.position
			for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
				l.readChar()
			}
			return Token{
				Type:    IDENT,
				Literal: "$-" + l.input[position:l.position],
				Line:    l.line,
				Column:  l.column,
			}
		}
		return newToken(DOLLAR, '$')
	}

	return Token{
		Type:    DOLLAR,
		Literal: "$" + lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readNumber() Token {
	position := l.position

	tok := Token{
		Type:    INT,
		Literal: "",
		Line:    l.line,
		Column:  l.column,
	}

	if l.ch == '0' {
		l.readChar()
		switch l.ch {
		case 'x', 'X':
			return l.readHexNumber(position)
		case 'b', 'B':
			return l.readBinaryNumber(position)
		case 'o', 'O':
			return l.readOctalNumber(position)
		case 'd', 'D':
			l.readChar()
			return l.readDecimalNumber(position)
		case '.':
			if isDigit(l.peekChar()) {
				l.readChar()
				return l.readFloat(position)
			}
		case '_':
			if isDigit(l.peekChar()) {
				return l.readDecimalNumber(position)
			}
		}

		tok.Literal = "0"
		return tok
	}

	return l.readDecimalNumber(position)
}

func (l *Lexer) readDecimalNumber(position int) Token {
	for isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar()
		return l.readFloat(position)
	}

	if l.ch == 'e' || l.ch == 'E' {
		return l.readExponent(position)
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    INT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readHexNumber(position int) Token {
	l.readChar()
	for isHexDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    INT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readBinaryNumber(position int) Token {
	l.readChar()
	for isBinaryDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    INT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readOctalNumber(position int) Token {
	l.readChar()
	for isOctalDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    INT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readFloat(position int) Token {
	for isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	if l.ch == 'e' || l.ch == 'E' {
		return l.readExponent(position)
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    FLOAT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readExponent(position int) Token {
	l.readChar()
	if l.ch == '+' || l.ch == '-' {
		l.readChar()
	}

	start := l.position
	for isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	if start == l.position {
		l.position = start
		l.ch = 'e'
		lit := l.input[position:l.position]
		return Token{
			Type:    FLOAT,
			Literal: lit,
			Line:    l.line,
			Column:  l.column,
		}
	}

	lit := l.input[position:l.position]
	lit = removeUnderscores(lit)

	return Token{
		Type:    FLOAT,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readString(singleQuote bool) Token {
	quote := l.ch
	l.readChar()

	position := l.position

	if singleQuote {
		lit := l.readSingleQuotedString(position, quote)
		return Token{
			Type:    STRING,
			Literal: lit,
			Line:    l.line,
			Column:  l.column,
		}
	} else {
		return l.readDoubleQuotedString(position, quote)
	}
}

func (l *Lexer) readSingleQuotedString(position int, quote rune) string {
	for l.ch != quote && l.ch != 0 {
		if l.ch == '\\' && l.peekChar() == quote {
			l.readChar()
		}
		l.readChar()
	}

	lit := l.input[position:l.position]
	// 不在这里调用 l.readChar()，让 NextToken 函数处理

	return lit
}

func (l *Lexer) readDoubleQuotedString(position int, quote rune) Token {
	var lit string

	for l.ch != quote && l.ch != 0 {
		if l.ch == '\\' {
			// Flush raw text before the backslash
			lit += l.input[position:l.position]
			l.readChar() // skip '\'
			lit += l.readEscapeSequence()
			position = l.position // skip past the escape in raw input
		} else if l.ch == '#' && l.peekChar() == '{' {
			lit += l.input[position:l.position]
			lit += l.readStringInterpolation()
			position = l.position
		} else if l.ch == '#' && l.peekChar() == '$' {
			lit += l.input[position:l.position]
			lit += l.readVarInterpolation()
			position = l.position
		} else if l.ch == '#' && isLetter(l.peekChar()) {
			// # 后面的字母不是 { 或 $，说明不是插值，直接添加到字符串
			lit += l.input[position:l.position]
			l.readChar()
			position = l.position
		} else if l.ch == quote {
			// 遇到结束引号，退出循环
			break
		} else {
			l.readChar()
		}
	}

	lit += l.input[position:l.position]

	// 不在这里调用 l.readChar()，让 NextToken 函数处理

	return Token{
		Type:    STRING,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readEscapeSequence() string {
	switch l.ch {
	case 'n':
		l.readChar()
		return "\n"
	case 't':
		l.readChar()
		return "\t"
	case 'r':
		l.readChar()
		return "\r"
	case 'v':
		l.readChar()
		return "\v"
	case 'f':
		l.readChar()
		return "\f"
	case 'a':
		l.readChar()
		return "\a"
	case 'b':
		l.readChar()
		return "\b"
	case 'e':
		l.readChar()
		return "\033"
	case 's':
		l.readChar()
		return " "
	case '\\':
		l.readChar()
		return "\\"
	case '\'':
		l.readChar()
		return "'"
	case '"':
		l.readChar()
		return "\""
	case '$':
		l.readChar()
		return "$"
	case '0', '1', '2', '3', '4', '5', '6', '7':
		return l.readOctalEscape()
	case 'x':
		return l.readHexEscape()
	case 'u':
		return l.readUnicodeEscape()
	case 'c', 'C':
		return l.readControlEscape()
	case 'M':
		return l.readMetaEscape()
	default:
		l.readChar()
		return string(l.ch)
	}
}

func (l *Lexer) readOctalEscape() string {
	var seq string
	for i := 0; i < 3 && isOctalDigit(l.ch); i++ {
		seq += string(l.ch)
		l.readChar()
	}
	return seq
}

func (l *Lexer) readHexEscape() string {
	var seq string
	for i := 0; i < 2 && isHexDigit(l.ch); i++ {
		seq += string(l.ch)
		l.readChar()
	}
	return seq
}

func (l *Lexer) readUnicodeEscape() string {
	l.readChar()
	if l.ch == '{' {
		l.readChar()
		var seq string
		for l.ch != '}' && l.ch != 0 {
			seq += string(l.ch)
			l.readChar()
		}
		l.readChar()
		return fmt.Sprintf("\\u%s", seq)
	}

	var seq string
	for i := 0; i < 4 && isHexDigit(l.ch); i++ {
		seq += string(l.ch)
		l.readChar()
	}
	return seq
}

func (l *Lexer) readControlEscape() string {
	l.readChar()
	if l.ch == '-' {
		l.readChar()
	}

	if unicode.IsLetter(l.ch) || (l.ch >= ' ' && l.ch <= '~') {
		r := unicode.ToUpper(l.ch) - 64
		l.readChar()
		return string(r)
	}

	return ""
}

func (l *Lexer) readMetaEscape() string {
	l.readChar()
	if l.ch == '-' {
		l.readChar()
	}

	if l.ch == '\\' {
		l.readChar()
		seq := l.readEscapeSequence()
		return "\x1b" + seq
	}

	r := l.ch + 128
	l.readChar()
	return string(r)
}

func (l *Lexer) readStringInterpolation() string {
	l.readChar() // skip '#'
	l.readChar() // skip '{'

	depth := 1
	start := l.position

	for depth > 0 && l.ch != 0 {
		if l.ch == '{' {
			depth++
		} else if l.ch == '}' {
			depth--
			if depth == 0 {
				break
			}
		}
		l.readChar()
	}

	lit := l.input[start:l.position]
	l.readChar() // skip closing '}'

	return "#{" + lit + "}"
}

func (l *Lexer) readVarInterpolation() string {
	l.readChar()

	start := l.position

	if l.ch == '@' || l.ch == '$' {
		l.readChar()
	}

	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	lit := l.input[start:l.position]
	return "#$" + lit
}

func (l *Lexer) readRawString() Token {
	l.readChar()

	position := l.position

	for l.ch != '`' && l.ch != 0 {
		l.readChar()
	}

	lit := l.input[position:l.position]
	l.readChar()

	return Token{
		Type:    STRING,
		Literal: "`" + lit + "`",
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readPercentString() Token {
	l.readChar()

	delimiter := l.ch

	switch delimiter {
	case 'q':
		delimiter = l.peekChar()
		l.readChar()
	case 'Q':
		delimiter = l.peekChar()
		l.readChar()
	case 'w':
		delimiter = l.peekChar()
		l.readChar()
	case 'W':
		delimiter = l.peekChar()
		l.readChar()
	case 'i':
		delimiter = l.peekChar()
		l.readChar()
	case 'I':
		delimiter = l.peekChar()
		l.readChar()
	case 'r':
		delimiter = l.peekChar()
		l.readChar()
	case 's':
		delimiter = l.peekChar()
		l.readChar()
	}

	if delimiter == '(' {
		delimiter = ')'
	} else if delimiter == '[' {
		delimiter = ']'
	} else if delimiter == '{' {
		delimiter = '}'
	} else if delimiter == '<' {
		delimiter = '>'
	}

	l.readChar()

	position := l.position

	for l.ch != delimiter && l.ch != 0 {
		if l.ch == '\\' && l.peekChar() == delimiter {
			l.readChar()
		}
		l.readChar()
	}

	lit := l.input[position:l.position]
	l.readChar()

	tok := Token{
		Type:    STRING,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}

	return tok
}

func (l *Lexer) readSlashOrRegexp() Token {
	l.readChar()

	if l.ch == '=' {
		l.readChar()
		return Token{
			Type:    DIVIDE_ASSIGN,
			Literal: "/=",
			Line:    l.line,
			Column:  l.column,
		}
	}

	if l.ch == ' ' {
		return newToken(DIVIDE, '/')
	}

	// Regexp
	position := l.position - 1

	for l.ch != '/' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
		}
		l.readChar()
	}

	lit := l.input[position:l.position]
	l.readChar()

	// Check for modifiers
	for l.ch == 'i' || l.ch == 'm' || l.ch == 'x' || l.ch == 'o' || l.ch == 'e' || l.ch == 's' || l.ch == 'u' || l.ch == 'n' {
		lit += string(l.ch)
		l.readChar()
	}

	return Token{
		Type:    REGEXP,
		Literal: lit,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readLeftShift() Token {
	ch := l.ch
	l.readChar()

	if l.ch == '-' {
		l.readChar()
		position := l.position

		// Check for heredoc
		if l.ch == '\n' {
			return l.readHeredoc(position)
		}

		if isLetter(l.ch) {
			return l.readHeredoc(position)
		}

		l.position = position
		l.ch = '-'
	}

	if l.peekChar() == '=' {
		l.readChar()
		return Token{
			Type:    ASSIGN,
			Literal: string(ch) + string(l.ch),
			Line:    l.line,
			Column:  l.column,
		}
	}

	return Token{
		Type:    LESS_THAN,
		Literal: "<<",
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readHeredoc(position int) Token {
	l.readChar()

	start := l.position

	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}

	delimiter := l.input[start:l.position]

	if l.ch == '-' {
		l.readChar()
	}

	l.readChar()

	position = l.position
	var lit string

	for !endsWith(l.input[position:l.position], delimiter) && l.ch != 0 {
		l.readChar()
	}

	lit = l.input[position:l.position]
	l.readChar()

	return Token{
		Type:    STRING,
		Literal: "<<" + delimiter + lit + delimiter,
		Line:    l.line,
		Column:  l.column,
	}
}

func (l *Lexer) readSymbolOrColon() Token {
	l.readChar()

	if l.ch == ':' {
		l.readChar()
		return Token{
			Type:    COLON2,
			Literal: "::",
			Line:    l.line,
			Column:  l.column,
		}
	}

	if isLetter(l.ch) || l.ch == '_' {
		position := l.position
		for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}

		lit := l.input[position:l.position]

		if l.ch == '=' && l.peekChar() != '>' {
			l.readChar()
			return Token{
				Type:    ASSIGN,
				Literal: ":" + lit + "=",
				Line:    l.line,
				Column:  l.column,
			}
		}

		if l.ch == '?' || l.ch == '!' {
			lit += string(l.ch)
			l.readChar()
		}

		return Token{
			Type:    SYMBOL,
			Literal: ":" + lit,
			Line:    l.line,
			Column:  l.column,
		}
	}

	return newToken(COLON, ':')
}

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func isLetter(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isBinaryDigit(ch rune) bool {
	return ch == '0' || ch == '1'
}

func isOctalDigit(ch rune) bool {
	return ch >= '0' && ch <= '7'
}

func removeUnderscores(lit string) string {
	result := ""
	for _, ch := range lit {
		if ch != '_' {
			result += string(ch)
		}
	}
	return result
}
