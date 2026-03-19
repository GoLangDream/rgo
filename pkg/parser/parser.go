package parser

import (
	"fmt"
	"strconv"

	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

const (
	LOWEST int = iota
	MODIFIER
	ASSIGN
	TERNARY
	BOOL_OR
	BOOL_AND
	EQUAL
	COMPARATOR
	ORDERING
	BIN_SHIFT
	SUM
	PRODUCT
	PREFIX
	CALL
	ACCESSOR
)

var precedences = map[lexer.TokenType]int{
	lexer.OR:                    BOOL_OR,
	lexer.OR2:                   BOOL_OR,
	lexer.AND:                   BOOL_AND,
	lexer.AND2:                  BOOL_AND,
	lexer.DOT2:                  MODIFIER,
	lexer.DOT3:                  MODIFIER,
	lexer.QUESTION:              MODIFIER,
	lexer.COLON2:                CALL,
	lexer.EQUAL:                 EQUAL,
	lexer.EQUAL3:                EQUAL,
	lexer.BANG_EQUAL:            EQUAL,
	lexer.NOT_EQUAL:             EQUAL,
	lexer.MATCH:                 EQUAL,
	lexer.LESS_THAN:             COMPARATOR,
	lexer.LESS_THAN_OR_EQUAL:    COMPARATOR,
	lexer.GREATER_THAN:          COMPARATOR,
	lexer.GREATER_THAN_OR_EQUAL: COMPARATOR,
	lexer.SPACESHIP:             COMPARATOR,
	lexer.PLUS:                  SUM,
	lexer.MINUS:                 SUM,
	lexer.PLUS_ASSIGN:           ASSIGN,
	lexer.MINUS_ASSIGN:          ASSIGN,
	lexer.ASSIGN:                ASSIGN,
	lexer.MULTIPLY:              PRODUCT,
	lexer.DIVIDE:                PRODUCT,
	lexer.MOD:                   PRODUCT,
	lexer.POW:                   PRODUCT,
	lexer.MULTIPLY_ASSIGN:       ASSIGN,
	lexer.DIVIDE_ASSIGN:         ASSIGN,
	lexer.MOD_ASSIGN:            ASSIGN,
	lexer.POW_ASSIGN:            ASSIGN,
	lexer.LSHIFT:                BIN_SHIFT,
	lexer.RSHIFT:                BIN_SHIFT,
	lexer.BIT_AND:               SUM,
	lexer.BIT_OR:                SUM,
	lexer.BIT_XOR:               SUM,
	lexer.LBRACKET:              CALL,
	lexer.DOT:                   CALL,
	lexer.LPAREN:                CALL,
	lexer.COLON:                 CALL,
	lexer.ARROW:                 CALL,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l         *lexer.Lexer
	curToken  lexer.Token
	peekToken lexer.Token

	prefixFns map[lexer.TokenType]prefixParseFn
	infixFns  map[lexer.TokenType]infixParseFn

	errors []string
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:         l,
		errors:    []string{},
		prefixFns: make(map[lexer.TokenType]prefixParseFn),
		infixFns:  make(map[lexer.TokenType]infixParseFn),
	}

	p.registerPrefix(lexer.IDENT, p.parseIdentifier)
	p.registerPrefix(lexer.TRUE, p.parseBoolean)
	p.registerPrefix(lexer.FALSE, p.parseBoolean)
	p.registerPrefix(lexer.NIL, p.parseNil)
	p.registerPrefix(lexer.INT, p.parseIntegerLiteral)
	p.registerPrefix(lexer.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.SYMBOL, p.parseSymbolLiteral)
	p.registerPrefix(lexer.REGEXP, p.parseRegexpLiteral)
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(lexer.LBRACE, p.parseHashLiteral)
	p.registerPrefix(lexer.DEF, p.parseDefExpression)
	p.registerPrefix(lexer.CLASS, p.parseClassExpression)
	p.registerPrefix(lexer.MODULE, p.parseModuleExpression)
	p.registerPrefix(lexer.IF, p.parseIfExpression)
	p.registerPrefix(lexer.UNLESS, p.parseUnlessExpression)
	p.registerPrefix(lexer.CASE, p.parseCaseExpression)
	p.registerPrefix(lexer.WHILE, p.parseWhileExpression)
	p.registerPrefix(lexer.UNTIL, p.parseUntilExpression)
	p.registerPrefix(lexer.FOR, p.parseForExpression)
	p.registerPrefix(lexer.BEGIN, p.parseBeginExpression)
	p.registerPrefix(lexer.RETURN, p.parseReturnExpression)
	p.registerPrefix(lexer.BREAK, p.parseBreakExpression)
	p.registerPrefix(lexer.NEXT, p.parseNextExpression)
	p.registerPrefix(lexer.REDO, p.parseRedoExpression)
	p.registerPrefix(lexer.RETRY, p.parseRetryExpression)
	p.registerPrefix(lexer.YIELD, p.parseYieldExpression)
	p.registerPrefix(lexer.SUPER, p.parseSuperExpression)
	p.registerPrefix(lexer.SELF, p.parseSelfExpression)
	p.registerPrefix(lexer.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(lexer.AT, p.parseInstanceVariable)
	p.registerPrefix(lexer.AT2, p.parseClassVariable)
	p.registerPrefix(lexer.DOLLAR, p.parseGlobalVariable)
	p.registerPrefix(lexer.BANG, p.parsePrefixExpression)
	p.registerPrefix(lexer.MINUS, p.parsePrefixExpression)
	p.registerPrefix(lexer.PLUS, p.parsePrefixExpression)
	p.registerPrefix(lexer.BIT_NOT, p.parsePrefixExpression)
	p.registerPrefix(lexer.MINUS_ARROW, p.parseLambdaExpression)
	p.registerPrefix(lexer.QUESTION, p.parsePrefixExpression)
	p.registerPrefix(lexer.LESS_THAN, p.parsePrefixExpression)
	p.registerPrefix(lexer.MULTIPLY, p.parseSplatExpression)
	p.registerPrefix(lexer.DEFINED, p.parseDefinedExpression)
	p.registerPrefix(lexer.ALIAS, p.parseAliasExpression)
	p.registerPrefix(lexer.UNDEF, p.parseUndefExpression)
	p.registerPrefix(lexer.INCLUDE, p.parseIncludeExpression)
	p.registerPrefix(lexer.EXTEND, p.parseExtendExpression)
	p.registerPrefix(lexer.PREPEND, p.parsePrependExpression)
	p.registerPrefix(lexer.CONSTANT, p.parseConstant)

	p.registerPrefix(lexer.DO, p.parseIdentifier)
	p.registerPrefix(lexer.END, p.parseIdentifier)

	p.registerInfix(lexer.PLUS, p.parseInfixExpression)
	p.registerInfix(lexer.MINUS, p.parseInfixExpression)
	p.registerInfix(lexer.MULTIPLY, p.parseInfixExpression)
	p.registerInfix(lexer.DIVIDE, p.parseInfixExpression)
	p.registerInfix(lexer.MOD, p.parseInfixExpression)
	p.registerInfix(lexer.POW, p.parseInfixExpression)
	p.registerInfix(lexer.PLUS_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.MINUS_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.MULTIPLY_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.DIVIDE_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.MOD_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.POW_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.EQUAL3, p.parseInfixExpression)
	p.registerInfix(lexer.MATCH, p.parseInfixExpression)
	p.registerInfix(lexer.NOT_EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.LESS_THAN, p.parseInfixExpression)
	p.registerInfix(lexer.LESS_THAN_OR_EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.GREATER_THAN, p.parseInfixExpression)
	p.registerInfix(lexer.GREATER_THAN_OR_EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.SPACESHIP, p.parseInfixExpression)
	p.registerInfix(lexer.BANG_EQUAL, p.parseInfixExpression)
	p.registerInfix(lexer.AND2, p.parseInfixExpression)
	p.registerInfix(lexer.OR2, p.parseInfixExpression)
	p.registerInfix(lexer.AND, p.parseInfixExpression)
	p.registerInfix(lexer.OR, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_AND, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_OR, p.parseInfixExpression)
	p.registerInfix(lexer.BIT_XOR, p.parseInfixExpression)
	p.registerInfix(lexer.LSHIFT, p.parseInfixExpression)
	p.registerInfix(lexer.RSHIFT, p.parseInfixExpression)
	p.registerInfix(lexer.DOT, p.parseMethodCall)
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression)
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)
	p.registerInfix(lexer.COLON2, p.parseConstantResolution)
	p.registerInfix(lexer.COLON, p.parseHashRocket)
	p.registerInfix(lexer.ARROW, p.parseHashRocket)
	p.registerInfix(lexer.DOT2, p.parseRangeExpression)
	p.registerInfix(lexer.DOT3, p.parseRangeExpression)

	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) parseError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	p.errors = append(p.errors, msg)
}

func (p *Parser) registerPrefix(tokenType lexer.TokenType, fn prefixParseFn) {
	p.prefixFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType lexer.TokenType, fn infixParseFn) {
	p.infixFns[tokenType] = fn
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{
		Statements: []ast.Statement{},
	}

	count := 0
	for !p.curTokenIs(lexer.EOF) {
		count++
		if count > 1000 {
			panic("infinite loop in ParseProgram")
		}
		// Skip semicolons and newlines at statement start
		for p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.NEWLINE) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.EOF) {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
		// Skip semicolons and newlines after statement
		for p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.NEWLINE) {
			p.nextToken()
		}
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case lexer.SEMICOLON, lexer.NEWLINE:
		// Empty statement
		return nil
	case lexer.RETURN:
		return p.parseReturnStatement()
	case lexer.BREAK:
		return p.parseBreakStatement()
	case lexer.NEXT:
		return p.parseNextStatement()
	case lexer.RAISE:
		return p.parseRaiseStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{
		Token: p.curToken,
	}

	// Don't parse expression if we're at a separator
	if p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.NEWLINE) {
		return stmt
	}

	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseReturnStatement() *ast.ReturnExpression {
	stmt := &ast.ReturnExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON) {
		stmt.ReturnValue = nil
	} else {
		stmt.ReturnValue = p.parseExpression(LOWEST)
	}

	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseBreakStatement() *ast.BreakExpression {
	stmt := &ast.BreakExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) {
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseNextStatement() *ast.NextExpression {
	stmt := &ast.NextExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) {
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseRaiseStatement() *ast.RaiseExpression {
	stmt := &ast.RaiseExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) {
		stmt.Error = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseExpression(prec int) ast.Expression {
	prefix := p.prefixFns[p.curToken.Type]
	if prefix == nil {
		if !p.curTokenIs(lexer.EOF) {
			p.parseError("no prefix parse function for %s found", p.curToken.Type)
		}
		return nil
	}

	leftExp := prefix()

	for !p.peekTokenIs(lexer.NEWLINE) && prec < p.peekPrecedence() {
		infix := p.infixFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curTokenIs(t lexer.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t lexer.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.parseError("expected next token to be %s, got %s instead", t, p.peekToken.Type)
	return false
}

func (p *Parser) parseIdentifier() ast.Expression {
	ident := &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	if p.isArgumentStart(p.peekToken) {
		call := &ast.MethodCall{
			Token:    p.curToken,
			Receiver: nil,
			Method:   ident,
		}

		p.nextToken()
		arg := p.parseExpression(LOWEST)
		if arg != nil {
			call.Args = append(call.Args, arg)
		}

		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			arg := p.parseExpression(LOWEST)
			if arg != nil {
				call.Args = append(call.Args, arg)
			}
		}

		return call
	}

	return ident
}

func (p *Parser) isArgumentStart(token lexer.Token) bool {
	switch token.Type {
	case lexer.STRING, lexer.INT, lexer.FLOAT, lexer.TRUE, lexer.FALSE, lexer.NIL,
		lexer.LBRACE, lexer.IDENT, lexer.MINUS, lexer.BANG,
		lexer.REGEXP, lexer.SYMBOL:
		return true
	default:
		return false
	}
}

func (p *Parser) parseBoolean() ast.Expression {
	return &ast.Boolean{
		Token: p.curToken,
		Value: p.curTokenIs(lexer.TRUE),
	}
}

func (p *Parser) parseNil() ast.Expression {
	return &ast.NilExpression{
		Token: p.curToken,
	}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{
		Token: p.curToken,
	}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		p.parseError("could not parse %q as integer", p.curToken.Literal)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{
		Token: p.curToken,
	}

	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.parseError("could not parse %q as float", p.curToken.Literal)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}
}

func (p *Parser) parseSymbolLiteral() ast.Expression {
	return &ast.SymbolLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}
}

func (p *Parser) parseRegexpLiteral() ast.Expression {
	return &ast.RegexpLiteral{
		Token:   p.curToken,
		Pattern: p.curToken.Literal,
	}
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	arr := &ast.ArrayLiteral{
		Token:    p.curToken,
		Elements: []ast.Expression{},
	}
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	if p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken()
		return arr
	}
	p.nextToken()
	element := p.parseExpression(LOWEST)
	if element != nil {
		arr.Elements = append(arr.Elements, element)
	}
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		for p.peekTokenIs(lexer.NEWLINE) {
			p.nextToken()
		}
		p.nextToken()
		element := p.parseExpression(LOWEST)
		if element != nil {
			arr.Elements = append(arr.Elements, element)
		}
	}
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}
	return arr
}

func (p *Parser) parseHashLiteral() ast.Expression {
	hash := &ast.HashLiteral{
		Token: p.curToken,
		Pairs: make(map[ast.Expression]ast.Expression),
		Order: []ast.Expression{},
	}
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	if p.peekTokenIs(lexer.RBRACE) {
		p.nextToken()
		return hash
	}
	p.nextToken()
	p.parseHashPair(hash)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		for p.peekTokenIs(lexer.NEWLINE) {
			p.nextToken()
		}
		p.nextToken()
		p.parseHashPair(hash)
	}
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	if !p.expectPeek(lexer.RBRACE) {
		return nil
	}
	return hash
}

func (p *Parser) parseHashPair(hash *ast.HashLiteral) {
	// Handle symbol shorthand: {foo: 1} where key is an IDENT followed by COLON
	if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
		key := &ast.SymbolLiteral{
			Token: p.curToken,
			Value: ":" + p.curToken.Literal,
		}
		p.nextToken()
		p.nextToken()
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		hash.Order = append(hash.Order, key)
		return
	}

	// Handle hash rocket: {"foo" => "bar"} or {:a => 1}
	if p.peekTokenIs(lexer.ARROW) {
		// Don't use parseExpression here - it will trigger the ARROW infix handler
		// Just use the current token as the key
		keyToken := p.curToken
		var key ast.Expression
		switch keyToken.Type {
		case lexer.STRING:
			key = &ast.StringLiteral{Token: keyToken, Value: keyToken.Literal}
		case lexer.SYMBOL:
			key = &ast.SymbolLiteral{Token: keyToken, Value: keyToken.Literal}
		case lexer.IDENT:
			key = &ast.Identifier{Token: keyToken, Value: keyToken.Literal}
		case lexer.INT:
			n, _ := strconv.ParseInt(keyToken.Literal, 10, 64)
			key = &ast.IntegerLiteral{Token: keyToken, Value: n}
		}
		p.nextToken() // move to ARROW
		p.nextToken() // move to value
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		hash.Order = append(hash.Order, key)
		return
	}

	if p.peekTokenIs(lexer.COLON) {
		if p.curTokenIs(lexer.IDENT) {
			key := &ast.SymbolLiteral{
				Token: p.curToken,
				Value: ":" + p.curToken.Literal,
			}
			p.nextToken()
			value := p.parseExpression(LOWEST)
			hash.Pairs[key] = value
			hash.Order = append(hash.Order, key)
			return
		}
	}
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{
		Token: p.curToken,
		Left:  left,
	}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RBRACKET) {
		return nil
	}

	return exp
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseSplatExpression() ast.Expression {
	exp := &ast.SplatExpression{
		Token: p.curToken,
	}

	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	prec := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(prec)

	return expression
}

func (p *Parser) parseTernaryExpression(condition ast.Expression) ast.Expression {
	exp := &ast.TernaryExpression{
		Token:     p.curToken,
		Condition: condition,
	}

	p.nextToken()
	exp.Consequent = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.COLON) {
		return nil
	}

	p.nextToken()
	exp.Alternative = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseRangeExpression(left ast.Expression) ast.Expression {
	exp := &ast.RangeExpression{
		Token: p.curToken,
		Left:  left,
	}

	exp.Exclusive = p.curTokenIs(lexer.DOT3)

	p.nextToken()
	exp.Right = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	assign := &ast.AssignExpression{
		Token: p.curToken,
	}

	if ident, ok := left.(*ast.Identifier); ok {
		assign.Name = ident
	} else if ivar, ok := left.(*ast.InstanceVariable); ok {
		assign.Name = &ast.Identifier{
			Token: ivar.Token,
			Value: ivar.Name,
		}
	} else if cvar, ok := left.(*ast.ClassVariable); ok {
		assign.Name = &ast.Identifier{
			Token: cvar.Token,
			Value: cvar.Name,
		}
	} else if gvar, ok := left.(*ast.GlobalVariable); ok {
		assign.Name = &ast.Identifier{
			Token: gvar.Token,
			Value: gvar.Name,
		}
	} else {
		p.parseError("invalid assignment target")
		return nil
	}

	p.nextToken()
	assign.Value = p.parseExpression(LOWEST)

	return assign
}

func (p *Parser) parseMethodCall(left ast.Expression) ast.Expression {
	call := &ast.MethodCall{
		Token:    p.curToken,
		Receiver: left,
	}

	if p.peekTokenIs(lexer.IDENT) {
		p.nextToken()
		call.Method = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}

	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()

		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
		} else {
			p.nextToken()
			p.parseOneCallArg(call)

			for p.peekTokenIs(lexer.COMMA) {
				p.nextToken()
				p.nextToken()
				p.parseOneCallArg(call)
			}

			if !p.expectPeek(lexer.RPAREN) {
				return nil
			}
		}
	}

	// Handle index access: obj[key]
	if p.peekTokenIs(lexer.LBRACKET) {
		return p.parseIndexExpression(left)
	}

	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
	} else if p.peekTokenIs(lexer.DO) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
	}

	return call
}

func (p *Parser) parseCallExpression(fn ast.Expression) ast.Expression {
	call := &ast.MethodCall{
		Token: p.curToken,
	}

	if ident, ok := fn.(*ast.Identifier); ok {
		call.Method = &ast.Identifier{
			Token: ident.Token,
			Value: ident.Value,
		}
	} else {
		p.parseError("expected identifier for function call, got %T", fn)
		return nil
	}

	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return call
	}

	p.nextToken()
	p.parseOneCallArg(call)

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		p.parseOneCallArg(call)
	}

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	return call
}

func (p *Parser) parseOneCallArg(call *ast.MethodCall) {
	if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.COLON) {
		name := p.curToken.Literal
		tok := p.curToken
		p.nextToken() // consume COLON
		p.nextToken() // move to value
		val := p.parseExpression(LOWEST)
		call.KeywordArgs = append(call.KeywordArgs, &ast.KeywordArg{
			Token: tok,
			Name:  name,
			Value: val,
		})
		return
	}
	arg := p.parseExpression(LOWEST)
	if arg != nil {
		call.Args = append(call.Args, arg)
	}
}

func (p *Parser) parseConstant() ast.Expression {
	return &ast.Constant{
		Token: p.curToken,
		Name:  p.curToken.Literal,
	}
}

func (p *Parser) parseConstantResolution(left ast.Expression) ast.Expression {
	res := &ast.ConstantResolution{
		Token: p.curToken,
		Left:  left,
	}

	p.nextToken()

	res.Name = &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	return res
}

func (p *Parser) parseHashRocket(left ast.Expression) ast.Expression {
	hash := &ast.HashLiteral{
		Token: p.curToken,
		Pairs: make(map[ast.Expression]ast.Expression),
		Order: []ast.Expression{},
	}

	// Convert identifier to symbol for symbol shorthand (baz: -> :baz)
	// But only for COLON case, not ARROW case
	key := left
	if p.curTokenIs(lexer.COLON) {
		if ident, ok := left.(*ast.Identifier); ok {
			key = &ast.SymbolLiteral{
				Token: ident.Token,
				Value: ":" + ident.Value,
			}
		}
	}

	p.nextToken()
	value := p.parseExpression(LOWEST)

	hash.Pairs[key] = value
	hash.Order = append(hash.Order, key)

	return hash
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()

	if p.curTokenIs(lexer.RPAREN) {
		return &ast.NilExpression{Token: p.curToken}
	}

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseInstanceVariable() ast.Expression {
	return &ast.InstanceVariable{
		Token: p.curToken,
		Name:  p.curToken.Literal,
	}
}

func (p *Parser) parseClassVariable() ast.Expression {
	return &ast.ClassVariable{
		Token: p.curToken,
		Name:  p.curToken.Literal,
	}
}

func (p *Parser) parseGlobalVariable() ast.Expression {
	return &ast.GlobalVariable{
		Token: p.curToken,
		Name:  p.curToken.Literal,
	}
}

func (p *Parser) parseSelfExpression() ast.Expression {
	return &ast.SelfExpression{
		Token: p.curToken,
	}
}

func (p *Parser) parseYieldExpression() ast.Expression {
	exp := &ast.YieldExpression{
		Token: p.curToken,
	}

	if p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) || p.peekTokenIs(lexer.END) {
		return exp
	}

	p.nextToken()
	arg := p.parseExpression(LOWEST)
	if arg != nil {
		exp.Args = append(exp.Args, arg)
	}

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		arg = p.parseExpression(LOWEST)
		if arg != nil {
			exp.Args = append(exp.Args, arg)
		}
	}

	return exp
}

func (p *Parser) parseSuperExpression() ast.Expression {
	exp := &ast.SuperExpression{
		Token: p.curToken,
	}

	p.nextToken()

	for !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		arg := p.parseExpression(LOWEST)
		if arg != nil {
			exp.Args = append(exp.Args, arg)
		}

		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
	}

	return exp
}

func (p *Parser) parseReturnExpression() ast.Expression {
	return p.parseReturnStatement()
}

func (p *Parser) parseBreakExpression() ast.Expression {
	return p.parseBreakStatement()
}

func (p *Parser) parseNextExpression() ast.Expression {
	return p.parseNextStatement()
}

func (p *Parser) parseRedoExpression() ast.Expression {
	return &ast.RedoExpression{
		Token: p.curToken,
	}
}

func (p *Parser) parseRetryExpression() ast.Expression {
	return &ast.RetryExpression{
		Token: p.curToken,
	}
}

func (p *Parser) parseIfExpression() ast.Expression {
	exp := &ast.IfExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.Condition = p.parseExpression(LOWEST)

	// Accept "then", newline, or "{" after condition
	if p.peekTokenIs(lexer.THEN) {
		p.nextToken() // consume "then"
	} else if !p.peekTokenIs(lexer.NEWLINE) && !p.peekTokenIs(lexer.LBRACE) {
		p.parseError("expected then, newline, or { after if condition, got %s", p.peekToken.Type)
		return nil
	}

	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken()
		exp.Consequent = p.parseBlockExpression()
	} else {
		p.nextToken() // skip newline/then
		p.skipCurNewlines()
		exp.Consequent = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ELSIF) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Consequent.Statements = append(exp.Consequent.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	for p.curTokenIs(lexer.ELSIF) {
		elsif := &ast.ElsIfExpression{
			Token: p.curToken,
		}

		p.nextToken()
		elsif.Condition = p.parseExpression(LOWEST)

		p.nextToken()
		p.skipCurNewlines()
		elsif.Consequent = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ELSIF) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				elsif.Consequent.Statements = append(elsif.Consequent.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}

		exp.ElsIf = append(exp.ElsIf, elsif)
	}

	if p.curTokenIs(lexer.ELSE) {
		p.nextToken()
		p.skipCurNewlines()
		exp.Alternative = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Alternative.Statements = append(exp.Alternative.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseUnlessExpression() ast.Expression {
	exp := &ast.IfExpression{
		Token: p.curToken,
	}

	p.nextToken()
	exp.Condition = p.parseExpression(LOWEST)

	p.nextToken()
	if p.curTokenIs(lexer.LBRACE) {
		exp.Consequent = p.parseBlockExpression()
	} else {
		exp.Consequent = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Consequent.Statements = append(exp.Consequent.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken()
		p.nextToken()
		exp.Alternative = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Alternative.Statements = append(exp.Alternative.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if !p.expectPeek(lexer.END) {
		return nil
	}

	return exp
}

func (p *Parser) parseCaseExpression() ast.Expression {
	exp := &ast.CaseExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.WHEN) && !p.curTokenIs(lexer.ELSE) && !p.peekTokenIs(lexer.NEWLINE) && !p.peekTokenIs(lexer.END) {
		exp.Expression = p.parseExpression(LOWEST)
	}

	p.skipNewlines()

	for p.peekTokenIs(lexer.WHEN) || p.curTokenIs(lexer.WHEN) {
		clause := &ast.CaseClause{
			Token: p.curToken,
		}

		if p.curTokenIs(lexer.WHEN) {
			p.nextToken()
		}

		p.skipNewlines()

		for !p.peekTokenIs(lexer.THEN) && !p.peekTokenIs(lexer.NEWLINE) && !p.peekTokenIs(lexer.END) && !p.peekTokenIs(lexer.EOF) {
			cond := p.parseExpression(LOWEST)
			clause.Conditions = append(clause.Conditions, cond)

			if p.curTokenIs(lexer.COMMA) {
				p.nextToken()
			}
		}

		if p.peekTokenIs(lexer.THEN) {
			p.nextToken()
		}

		p.nextToken()
		clause.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.peekTokenIs(lexer.WHEN) && !p.peekTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				clause.Body.Statements = append(clause.Body.Statements, stmt)
			}
			p.skipNewlines()
		}

		exp.Clauses = append(exp.Clauses, clause)
	}

	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken()
		p.nextToken()
		exp.Else = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Else.Statements = append(exp.Else.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if !p.expectPeek(lexer.END) {
		return nil
	}

	return exp
}

func (p *Parser) parseWhileExpression() ast.Expression {
	exp := &ast.WhileExpression{
		Token: p.curToken,
	}

	p.nextToken()
	exp.Condition = p.parseExpression(LOWEST)

	p.nextToken()
	if p.curTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
		p.nextToken()
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseUntilExpression() ast.Expression {
	exp := &ast.UntilExpression{
		Token: p.curToken,
	}

	p.nextToken()
	exp.Condition = p.parseExpression(LOWEST)

	p.nextToken()
	if p.curTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
		p.nextToken()
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseForExpression() ast.Expression {
	exp := &ast.ForExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.IDENT) {
		exp.Variable = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
		p.nextToken()
	}

	if !p.expectPeek(lexer.IN) {
		return nil
	}

	p.nextToken()
	exp.Collection = p.parseExpression(LOWEST)

	p.nextToken()

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else if p.peekTokenIs(lexer.DO) {
		p.nextToken()
		p.nextToken()
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.skipNewlines()
		}
	} else {
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if !p.expectPeek(lexer.END) {
		return nil
	}

	return exp
}

func (p *Parser) parseDefExpression() ast.Expression {
	exp := &ast.DefExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.SELF) || p.curTokenIs(lexer.IDENT) {
		exp.Receiver = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
		if p.peekTokenIs(lexer.DOT) {
			p.nextToken()
			p.nextToken()
		}
	}

	if !p.curTokenIs(lexer.IDENT) {
		p.parseError("expected method name")
		return nil
	}

	exp.Name = &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	p.nextToken()

	if p.curTokenIs(lexer.LPAREN) {
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken() // skip RPAREN
		} else {
			p.nextToken() // move to first param
			p.parseDefParams(exp)
			if !p.expectPeek(lexer.RPAREN) {
				return nil
			}
		}
	}

	p.nextToken()

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseDefParams(exp *ast.DefExpression) {
	p.parseOneDefParam(exp)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken() // skip comma
		p.nextToken() // move to next param
		p.parseOneDefParam(exp)
	}
}

func (p *Parser) parseOneDefParam(exp *ast.DefExpression) {
	if p.curTokenIs(lexer.MULTIPLY) {
		p.nextToken()
		if p.curTokenIs(lexer.IDENT) {
			exp.RestParam = &ast.Identifier{
				Token: p.curToken,
				Value: p.curToken.Literal,
			}
		}
		return
	}
	if !p.curTokenIs(lexer.IDENT) {
		return
	}
	name := p.curToken.Literal
	if p.peekTokenIs(lexer.COLON) {
		p.nextToken() // consume COLON
		kp := &ast.KeywordParam{Name: name}
		if !p.peekTokenIs(lexer.COMMA) && !p.peekTokenIs(lexer.RPAREN) && !p.peekTokenIs(lexer.NEWLINE) {
			p.nextToken()
			kp.Default = p.parseExpression(LOWEST)
		}
		exp.KeywordParams = append(exp.KeywordParams, kp)
	} else {
		exp.Params = append(exp.Params, &ast.Identifier{
			Token: p.curToken,
			Value: name,
		})
	}
}

func (p *Parser) parseClassExpression() ast.Expression {
	exp := &ast.ClassExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.CONSTANT) {
		p.parseError("expected class name")
		return nil
	}

	exp.Name = &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	p.nextToken()

	if p.curTokenIs(lexer.LESS_THAN) {
		p.nextToken()
		exp.SuperClass = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
		p.nextToken()
	}

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			// Skip semicolons
			if p.curTokenIs(lexer.SEMICOLON) {
				p.nextToken()
				continue
			}
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseModuleExpression() ast.Expression {
	exp := &ast.ModuleExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.CONSTANT) {
		p.parseError("expected module name")
		return nil
	}

	exp.Name = &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	p.nextToken()

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else {
		exp.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Body.Statements = append(exp.Body.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if !p.expectPeek(lexer.END) {
		return nil
	}

	return exp
}

func (p *Parser) parseLambdaExpression() ast.Expression {
	lit := &ast.ProcLiteral{
		Token: p.curToken,
	}

	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()
		p.nextToken()
		for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.IDENT) {
				lit.Params = append(lit.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
			p.nextToken()
			if p.curTokenIs(lexer.COMMA) {
				p.nextToken()
			}
		}
		p.nextToken()
	}

	if p.curTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.LBRACE) {
		if p.peekTokenIs(lexer.LBRACE) {
			p.nextToken()
		}
		lit.Body = p.parseBlockExpression()
	} else if p.curTokenIs(lexer.DO) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
		lit.Body = p.parseBlockExpression()
	}

	return lit
}

func (p *Parser) parseBlockExpression() *ast.BlockExpression {
	block := &ast.BlockExpression{
		Token: p.curToken,
	}

	p.nextToken()
	for p.curTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}

	if p.curTokenIs(lexer.BIT_OR) {
		p.nextToken()
		for !p.curTokenIs(lexer.BIT_OR) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.IDENT) {
				block.Params = append(block.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
			p.nextToken()
			if p.curTokenIs(lexer.COMMA) {
				p.nextToken()
			}
		}
		if p.curTokenIs(lexer.BIT_OR) {
			p.nextToken()
		}
	}

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		for p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.END) || p.curTokenIs(lexer.EOF) {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.skipNewlines()
		if p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
		if !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) {
			if p.peekTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.END) || p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) {
				p.nextToken()
			}
		}
	}

	if p.curTokenIs(lexer.END) {
		return block
	}

	if p.curTokenIs(lexer.RBRACE) {
		return block
	}

	return block
}

func (p *Parser) parseBeginExpression() ast.Expression {
	exp := &ast.BeginExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.Body = &ast.BlockExpression{
		Token: p.curToken,
	}

	for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			exp.Body.Statements = append(exp.Body.Statements, stmt)
		}
		p.skipNewlines()
	}

	for p.peekTokenIs(lexer.RESCUE) {
		rescue := &ast.RescueClause{
			Token: p.curToken,
		}

		p.nextToken()
		p.nextToken()

		if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.ELSE) {
			rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))

			for p.curTokenIs(lexer.COMMA) {
				p.nextToken()
				p.nextToken()
				rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
			}
		}

		p.nextToken()
		rescue.Body = &ast.BlockExpression{
			Token: p.curToken,
		}

		for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				rescue.Body.Statements = append(rescue.Body.Statements, stmt)
			}
			p.skipNewlines()
		}

		exp.Rescue = append(exp.Rescue, rescue)
	}

	if p.peekTokenIs(lexer.ELSE) {
		p.nextToken()
		p.nextToken()
		exp.Else = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Else.Statements = append(exp.Else.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if p.peekTokenIs(lexer.ENSURE) {
		p.nextToken()
		p.nextToken()
		exp.Ensure = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Ensure.Statements = append(exp.Ensure.Statements, stmt)
			}
			p.skipNewlines()
		}
	}

	if !p.expectPeek(lexer.END) {
		return nil
	}

	return exp
}

func (p *Parser) parseDefinedExpression() ast.Expression {
	exp := &ast.DefinedExpression{
		Token: p.curToken,
	}

	if !p.expectPeek(lexer.LPAREN) {
		return nil
	}

	p.nextToken()
	exp.Expression = p.parseExpression(LOWEST)

	if !p.expectPeek(lexer.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseAliasExpression() ast.Expression {
	exp := &ast.AliasExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.New = p.parseExpression(LOWEST)
	p.nextToken()
	exp.Old = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseUndefExpression() ast.Expression {
	exp := &ast.UndefExpression{
		Token: p.curToken,
	}

	p.nextToken()

	for !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.STRING) {
			exp.Methods = append(exp.Methods, &ast.Identifier{
				Token: p.curToken,
				Value: p.curToken.Literal,
			})
		}
		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return exp
}

func (p *Parser) parseIncludeExpression() ast.Expression {
	exp := &ast.IncludeExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.Module = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseExtendExpression() ast.Expression {
	exp := &ast.ExtendExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.Module = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parsePrependExpression() ast.Expression {
	exp := &ast.PrependExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.Module = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) skipNewlines() {
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
}

func (p *Parser) skipCurNewlines() {
	for p.curTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
}
