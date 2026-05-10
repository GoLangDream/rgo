package parser

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

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
	RANGE
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
	lexer.DOT2:                  RANGE,
	lexer.DOT3:                  RANGE,
	lexer.QUESTION:              MODIFIER,
	lexer.RESCUE:                MODIFIER,
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
	lexer.OR_ASSIGN:             ASSIGN,
	lexer.AND_ASSIGN:            ASSIGN,
	lexer.BIT_OR_ASSIGN:         ASSIGN,
	lexer.BIT_AND_ASSIGN:        ASSIGN,
	lexer.BIT_XOR_ASSIGN:        ASSIGN,
	lexer.LSHIFT_ASSIGN:         ASSIGN,
	lexer.RSHIFT_ASSIGN:         ASSIGN,
	lexer.LSHIFT:                BIN_SHIFT,
	lexer.RSHIFT:                BIN_SHIFT,
	lexer.BIT_AND:               SUM,
	lexer.BIT_OR:                SUM,
	lexer.BIT_XOR:               SUM,
	lexer.LBRACKET:              CALL,
	lexer.DOT:                   CALL,
	lexer.SAFE_NAV:              CALL,
	lexer.LPAREN:                CALL,
	lexer.COLON:                 CALL,
	lexer.ARROW:                 CALL,
	lexer.IN:                    EQUAL,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l         *lexer.Lexer
	prevToken lexer.Token
	curToken  lexer.Token
	peekToken lexer.Token

	prefixFns map[lexer.TokenType]prefixParseFn
	infixFns  map[lexer.TokenType]infixParseFn

	errors                     []string
	stopAtColon                bool
	stopAtRParen               bool
	stopAtRBracket             bool
	stopAtCallArgRParen        bool
	stopAtHashRocket           bool
	allowGroupedRParenDotChain bool
	groupDepth                 int
	lastClosedGroupDepth       int
	stopAtDo                   bool
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
	p.registerPrefix(lexer.RATIONAL, p.parseRationalLiteral)
	p.registerPrefix(lexer.STRING, p.parseStringLiteral)
	p.registerPrefix(lexer.SYMBOL, p.parseSymbolLiteral)
	p.registerPrefix(lexer.REGEXP, p.parseRegexpLiteral)
	p.registerPrefix(lexer.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(lexer.LBRACE, p.parseHashLiteral)
	p.registerPrefix(lexer.RBRACE, p.parseTerminatorExpression)
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
	p.registerPrefix(lexer.RAISE, p.parseRaiseExpression)
	p.registerPrefix(lexer.THROW, p.parseThrowExpression)
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
	p.registerPrefix(lexer.POW, p.parseDoubleSplatExpression)
	p.registerPrefix(lexer.BIT_AND, p.parseBlockPassExpression)
	p.registerPrefix(lexer.DOT2, p.parseBeginlessRangeExpression)
	p.registerPrefix(lexer.DOT3, p.parseBeginlessRangeExpression)
	p.registerPrefix(lexer.DEFINED, p.parseDefinedExpression)
	p.registerPrefix(lexer.ALIAS, p.parseAliasExpression)
	p.registerPrefix(lexer.UNDEF, p.parseUndefExpression)
	p.registerPrefix(lexer.INCLUDE, p.parseIncludeExpression)
	p.registerPrefix(lexer.EXTEND, p.parseExtendExpression)
	p.registerPrefix(lexer.PREPEND, p.parsePrependExpression)
	p.registerPrefix(lexer.PUBLIC, p.parseIdentifier)
	p.registerPrefix(lexer.PRIVATE, p.parseIdentifier)
	p.registerPrefix(lexer.PROTECTED, p.parseIdentifier)
	p.registerPrefix(lexer.CONSTANT, p.parseConstant)
	p.registerPrefix(lexer.COLON2, p.parseTopLevelConstantResolution)
	p.registerPrefix(lexer.CATCH, p.parseCatchExpression)

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
	p.registerInfix(lexer.OR_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.AND_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.BIT_OR_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.BIT_AND_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.BIT_XOR_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.LSHIFT_ASSIGN, p.parseAssignExpression)
	p.registerInfix(lexer.RSHIFT_ASSIGN, p.parseAssignExpression)
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
	p.registerInfix(lexer.SAFE_NAV, p.parseMethodCall)
	p.registerInfix(lexer.LBRACKET, p.parseIndexExpression)
	p.registerInfix(lexer.LPAREN, p.parseCallExpression)
	p.registerInfix(lexer.COLON2, p.parseConstantResolution)
	p.registerInfix(lexer.COLON, p.parseHashRocket)
	p.registerInfix(lexer.ARROW, p.parseHashRocket)
	p.registerInfix(lexer.IN, p.parsePatternMatchExpression)
	p.registerInfix(lexer.DOT2, p.parseRangeExpression)
	p.registerInfix(lexer.DOT3, p.parseRangeExpression)
	p.registerInfix(lexer.QUESTION, p.parseTernaryExpression)
	p.registerInfix(lexer.RESCUE, p.parseRescueModifier)

	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) nextToken() {
	p.prevToken = p.curToken
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) parseError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if p.curToken.Line > 0 || p.curToken.Column > 0 {
		msg = fmt.Sprintf("line %d:%d: %s", p.curToken.Line, p.curToken.Column, msg)
	}
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
	case lexer.SEMICOLON, lexer.NEWLINE, lexer.RBRACE, lexer.RPAREN:
		return nil
	case lexer.RETURN:
		ret := p.parseReturnStatement()
		if p.curTokenIs(lexer.IF) || p.peekTokenIs(lexer.IF) {
			if !p.curTokenIs(lexer.IF) {
				p.nextToken()
			}
			return &ast.ExpressionStatement{Token: ret.Token, Expression: p.parseIfModifier(ret)}
		}
		if p.curTokenIs(lexer.UNLESS) || p.peekTokenIs(lexer.UNLESS) {
			if !p.curTokenIs(lexer.UNLESS) {
				p.nextToken()
			}
			return &ast.ExpressionStatement{Token: ret.Token, Expression: p.parseUnlessModifier(ret)}
		}
		return ret
	case lexer.RAISE:
		raise := p.parseRaiseStatement()
		if p.curTokenIs(lexer.IF) || p.peekTokenIs(lexer.IF) {
			if !p.curTokenIs(lexer.IF) {
				p.nextToken()
			}
			return &ast.ExpressionStatement{Token: raise.Token, Expression: p.parseIfModifier(raise)}
		}
		if p.curTokenIs(lexer.UNLESS) || p.peekTokenIs(lexer.UNLESS) {
			if !p.curTokenIs(lexer.UNLESS) {
				p.nextToken()
			}
			return &ast.ExpressionStatement{Token: raise.Token, Expression: p.parseUnlessModifier(raise)}
		}
		return raise
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{
		Token: p.curToken,
	}

	if p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.NEWLINE) {
		return stmt
	}

	expr := p.parseExpression(LOWEST)

	if p.peekTokenIs(lexer.COMMA) {
		if multiAssign := p.tryParseMultiAssign(expr); multiAssign != nil {
			stmt.Expression = multiAssign
			return stmt
		}
	}

	if p.curTokenIs(lexer.IF) || p.peekTokenIs(lexer.IF) {
		if !p.curTokenIs(lexer.IF) {
			p.nextToken()
		}
		stmt.Expression = p.parseIfModifier(expr)
	} else if p.curTokenIs(lexer.UNLESS) || p.peekTokenIs(lexer.UNLESS) {
		if !p.curTokenIs(lexer.UNLESS) {
			p.nextToken()
		}
		stmt.Expression = p.parseUnlessModifier(expr)
	} else if p.curTokenIs(lexer.WHILE) || p.peekTokenIs(lexer.WHILE) {
		if !p.curTokenIs(lexer.WHILE) {
			p.nextToken()
		}
		stmt.Expression = p.parseWhileModifier(expr)
	} else if p.curTokenIs(lexer.UNTIL) || p.peekTokenIs(lexer.UNTIL) {
		if !p.curTokenIs(lexer.UNTIL) {
			p.nextToken()
		}
		stmt.Expression = p.parseUntilModifier(expr)
	} else {
		stmt.Expression = expr
	}

	if (p.peekTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON)) || (p.peekTokenIs(lexer.DO) && !p.stopAtDo) {
		var block *ast.BlockExpression
		if p.peekTokenIs(lexer.LBRACE) {
			p.nextToken()
			block = p.parseBlockExpression()
		} else {
			p.nextToken()
			block = p.parseBlockExpression()
		}
		stmt.Expression = p.attachTrailingBlock(stmt.Expression, block)
		if p.shouldConsumeBlockTerminator() {
			p.consumeBlockTerminator()
		}
	}

	if !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && (p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON)) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) attachTrailingBlock(expr ast.Expression, block *ast.BlockExpression) ast.Expression {
	switch e := expr.(type) {
	case *ast.MethodCall:
		if len(e.Args) > 0 {
			lastArg := e.Args[len(e.Args)-1]
			switch la := lastArg.(type) {
			case *ast.MethodCall:
				if la.Block == nil {
					la.Block = block
					return expr
				}
			case *ast.Identifier:
				e.Args[len(e.Args)-1] = &ast.MethodCall{
					Token:  la.Token,
					Method: &ast.Identifier{Token: la.Token, Value: la.Value},
					Block:  block,
				}
				return expr
			}
		}
		e.Block = block
	case *ast.Identifier:
		return &ast.MethodCall{
			Token:  e.Token,
			Method: &ast.Identifier{Token: e.Token, Value: e.Value},
			Block:  block,
		}
	}
	return expr
}

func (p *Parser) parseIfModifier(expr ast.Expression) ast.Expression {
	modifier := &ast.IfExpression{
		Token: p.curToken,
	}

	p.nextToken()
	modifier.Condition = p.parseExpression(LOWEST)
	modifier.Consequent = &ast.BlockExpression{
		Token:      p.curToken,
		Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: expr}},
	}

	return modifier
}

func (p *Parser) parseUnlessModifier(expr ast.Expression) ast.Expression {
	modifier := &ast.IfExpression{
		Token: p.curToken,
	}

	p.nextToken()
	condition := p.parseExpression(LOWEST)
	modifier.Condition = &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: "!",
		Right:    condition,
	}
	modifier.Consequent = &ast.BlockExpression{
		Token:      p.curToken,
		Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: expr}},
	}

	return modifier
}

func (p *Parser) parseWhileModifier(expr ast.Expression) ast.Expression {
	modifier := &ast.WhileExpression{
		Token: p.curToken,
	}

	p.nextToken()
	modifier.Condition = p.parseExpression(LOWEST)
	modifier.Body = &ast.BlockExpression{
		Token:      p.curToken,
		Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: expr}},
	}

	return modifier
}

func (p *Parser) parseUntilModifier(expr ast.Expression) ast.Expression {
	modifier := &ast.UntilExpression{
		Token: p.curToken,
	}

	p.nextToken()
	modifier.Condition = p.parseExpression(LOWEST)
	modifier.Body = &ast.BlockExpression{
		Token:      p.curToken,
		Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: expr}},
	}

	return modifier
}

func (p *Parser) tryParseMultiAssign(first ast.Expression) *ast.MultiAssignExpression {
	firstName := assignmentName(first)
	if firstName == nil {
		return nil
	}
	names := []*ast.Identifier{firstName}

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		if p.peekTokenIs(lexer.ASSIGN) {
			break
		}
		p.nextToken()
		p.skipCurSeparators()

		target := p.parseExpression(ASSIGN)
		name := assignmentName(target)
		if name == nil {
			return nil
		}
		names = append(names, name)
	}

	if !p.peekTokenIs(lexer.ASSIGN) {
		return nil
	}

	p.nextToken()
	p.nextToken()

	values := []ast.Expression{}
	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.EOF) {
		val := p.parseExpression(LOWEST)
		if val != nil {
			values = append(values, val)
		}

		for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
			if !p.curTokenIs(lexer.COMMA) {
				p.nextToken()
			}
			p.nextToken()
			val := p.parseExpression(LOWEST)
			if val != nil {
				values = append(values, val)
			}
		}
	}

	return &ast.MultiAssignExpression{
		Token:  firstName.Token,
		Names:  names,
		Values: values,
	}
}

func (p *Parser) parseReturnStatement() *ast.ReturnExpression {
	stmt := &ast.ReturnExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.RPAREN) || p.curTokenIs(lexer.IF) || p.curTokenIs(lexer.UNLESS) || p.curTokenIs(lexer.WHILE) || p.curTokenIs(lexer.UNTIL) {
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

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.COLON) && !p.curTokenIs(lexer.IF) && !p.curTokenIs(lexer.UNLESS) && !p.curTokenIs(lexer.WHILE) && !p.curTokenIs(lexer.UNTIL) {
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseNextStatement() *ast.NextExpression {
	stmt := &ast.NextExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.COLON) && !p.curTokenIs(lexer.IF) && !p.curTokenIs(lexer.UNLESS) && !p.curTokenIs(lexer.WHILE) && !p.curTokenIs(lexer.UNTIL) {
		stmt.Value = p.parseCommaSeparatedValue()
	}

	return stmt
}

func (p *Parser) parseCommaSeparatedValue() ast.Expression {
	first := p.parseExpression(LOWEST)
	if !p.peekTokenIs(lexer.COMMA) {
		return first
	}

	values := []ast.Expression{}
	if first != nil {
		values = append(values, first)
	}
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		if p.assignmentValueEndsAfterComma() {
			break
		}
		p.nextToken()
		value := p.parseExpression(LOWEST)
		if value != nil {
			values = append(values, value)
		}
	}

	return &ast.ArrayLiteral{
		Token:    lexer.Token{Type: lexer.LBRACKET, Literal: "["},
		Elements: values,
	}
}

func (p *Parser) parseRaiseStatement() *ast.RaiseExpression {
	stmt := &ast.RaiseExpression{
		Token: p.curToken,
	}

	if p.peekTokenIs(lexer.RESCUE) {
		return stmt
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.IF) && !p.curTokenIs(lexer.UNLESS) && !p.curTokenIs(lexer.WHILE) && !p.curTokenIs(lexer.UNTIL) {
		stmt.Error = p.parseExpression(LOWEST)
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
			stmt.Error = p.parseExpression(LOWEST)
		}
	}

	return stmt
}

func (p *Parser) parseRaiseExpression() ast.Expression {
	return p.parseRaiseStatement()
}

func (p *Parser) parseThrowExpression() ast.Expression {
	return p.parseThrowStatement()
}

func (p *Parser) parseThrowStatement() *ast.ThrowExpression {
	stmt := &ast.ThrowExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) {
		stmt.Label = p.parseExpression(LOWEST)
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
			stmt.Value = p.parseExpression(LOWEST)
		}
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			stmt.ExtraArgs = append(stmt.ExtraArgs, p.parseExpression(LOWEST))
		}
	}

	return stmt
}

func (p *Parser) parseBlockParams(block *ast.BlockExpression) {
	if p.curTokenIs(lexer.OR) {
		p.nextToken()
		return
	}
	if p.curTokenIs(lexer.BIT_OR) {
		p.nextToken()
		if p.curTokenIs(lexer.BIT_OR) {
			p.nextToken()
			return
		}
		for !p.curTokenIs(lexer.BIT_OR) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.CONSTANT) {
				block.Params = append(block.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			} else if p.curTokenIs(lexer.MULTIPLY) || p.curTokenIs(lexer.POW) {
				paramTok := p.curToken
				p.nextToken()
				if p.curTokenIs(lexer.IDENT) {
					block.Params = append(block.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
				} else {
					block.Params = append(block.Params, &ast.Identifier{Token: paramTok, Value: paramTok.Literal})
					continue
				}
			} else if p.curTokenIs(lexer.BIT_AND) {
				p.nextToken()
				if p.curTokenIs(lexer.IDENT) {
					block.Params = append(block.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
				}
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
}

func (p *Parser) parseCatchExpression() ast.Expression {
	exp := &ast.CatchExpression{
		Token: p.curToken,
	}
	endToken := lexer.END

	p.nextToken()

	if !p.curTokenIs(lexer.DO) && !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.NEWLINE) {
		exp.Label = p.parseExpression(LOWEST)
	}

	if !p.curTokenIs(lexer.DO) && !p.curTokenIs(lexer.LBRACE) && !p.peekTokenIs(lexer.DO) && !p.peekTokenIs(lexer.LBRACE) {
		exp.Body = &ast.BlockExpression{Token: p.curToken}
		return exp
	}

	if p.curTokenIs(lexer.DO) {
		p.nextToken()
		exp.Body = &ast.BlockExpression{Token: p.curToken}
		p.parseBlockParams(exp.Body)
	} else if p.curTokenIs(lexer.LBRACE) {
		endToken = lexer.RBRACE
		p.nextToken()
		exp.Body = &ast.BlockExpression{Token: p.curToken}
		p.parseBlockParams(exp.Body)
	} else {
		if p.peekTokenIs(lexer.DO) || p.peekTokenIs(lexer.LBRACE) {
			p.nextToken()
			if p.curTokenIs(lexer.LBRACE) {
				endToken = lexer.RBRACE
			}
			if p.curTokenIs(lexer.DO) || p.curTokenIs(lexer.LBRACE) {
				p.nextToken()
			}
			exp.Body = &ast.BlockExpression{Token: p.curToken}
			p.parseBlockParams(exp.Body)
		} else {
			exp.Body = &ast.BlockExpression{Token: p.curToken}
		}
	}

	for !p.curTokenIs(endToken) && !p.curTokenIs(lexer.EOF) {
		before := p.curToken
		stmt := p.parseStatement()
		if stmt != nil {
			exp.Body.Statements = append(exp.Body.Statements, stmt)
		}
		if p.curToken == before && !p.curTokenIs(endToken) && !p.curTokenIs(lexer.EOF) {
			p.nextToken()
		}
		if p.curTokenIs(endToken) {
			break
		}
		p.skipNewlines()
		if !p.curTokenIs(endToken) && !p.curTokenIs(lexer.EOF) {
			if p.peekTokenIs(endToken) || p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) {
				p.nextToken()
			}
		}
	}

	if !p.curTokenIs(endToken) && !p.expectPeek(endToken) {
		return nil
	}

	return exp
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
	for !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.IF) && !p.curTokenIs(lexer.UNLESS) && !p.curTokenIs(lexer.WHILE) && !p.curTokenIs(lexer.UNTIL) && (!p.curTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.ARROW) || p.peekTokenIs(lexer.IN) || p.peekTokenIs(lexer.LBRACKET) || isHashLiteral(leftExp) || isBlockMethodCall(leftExp)) && (!p.curTokenIs(lexer.END) || p.peekTokenIs(lexer.DOT)) && !p.shouldStopGroupedRParenDotChain(leftExp) && !(p.stopAtRBracket && p.curTokenIs(lexer.RBRACKET) && !(p.peekTokenIs(lexer.DOT) && isArrayLiteral(leftExp))) && !(p.stopAtHashRocket && p.peekTokenIs(lexer.ARROW)) && !p.peekTokenIs(lexer.NEWLINE) && !(p.stopAtColon && p.peekTokenIs(lexer.COLON)) && prec < p.peekPrecedence() {
		infix := p.infixFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.allowGroupedRParenDotChain = false
		p.lastClosedGroupDepth = 0
		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) shouldStopGroupedRParenDotChain(expr ast.Expression) bool {
	if !p.stopAtRParen || !p.curTokenIs(lexer.RPAREN) || !p.peekTokenIs(lexer.DOT) || p.allowGroupedRParenDotChain {
		return false
	}
	if isAssignmentExpression(expr) {
		return true
	}
	return !p.currentTokenClosedChildGroup() && (isRangeExpression(expr) || isPatternMatchExpression(expr) || isBlockMethodCall(expr) || isInfixExpression(expr))
}

func isRangeExpression(expr ast.Expression) bool {
	_, ok := expr.(*ast.RangeExpression)
	return ok
}

func isPatternMatchExpression(expr ast.Expression) bool {
	_, ok := expr.(*ast.PatternMatchExpression)
	return ok
}

func isAssignmentExpression(expr ast.Expression) bool {
	_, ok := expr.(*ast.AssignExpression)
	return ok
}

func isHashLiteral(expr ast.Expression) bool {
	_, ok := expr.(*ast.HashLiteral)
	return ok
}

func isArrayLiteral(expr ast.Expression) bool {
	_, ok := expr.(*ast.ArrayLiteral)
	return ok
}

func isBlockMethodCall(expr ast.Expression) bool {
	call, ok := expr.(*ast.MethodCall)
	return ok && call.Block != nil
}

func isInfixExpression(expr ast.Expression) bool {
	_, ok := expr.(*ast.InfixExpression)
	return ok
}

func (p *Parser) currentTokenClosedChildGroup() bool {
	return p.curTokenIs(lexer.RPAREN) && p.lastClosedGroupDepth > p.groupDepth
}

func isUnparenthesizedMethodCall(expr ast.Expression) bool {
	call, ok := expr.(*ast.MethodCall)
	return ok && !call.ParenthesizedArgs
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

func (p *Parser) curTokenIsAny(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.curTokenIs(t) {
			return true
		}
	}
	return false
}

func (p *Parser) peekTokenIsAny(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.peekTokenIs(t) {
			return true
		}
	}
	return false
}

func (p *Parser) isHashLabelKey() bool {
	switch p.curToken.Type {
	case lexer.IDENT, lexer.STRING, lexer.CONSTANT,
		lexer.TRUE, lexer.FALSE, lexer.NIL,
		lexer.SELF, lexer.SUPER, lexer.YIELD,
		lexer.IF, lexer.UNLESS, lexer.WHILE, lexer.UNTIL,
		lexer.FOR, lexer.DO, lexer.BEGIN, lexer.END,
		lexer.DEF, lexer.CLASS, lexer.MODULE,
		lexer.RETURN, lexer.BREAK, lexer.NEXT,
		lexer.CASE, lexer.WHEN, lexer.THEN,
		lexer.ELSE, lexer.ELSIF,
		lexer.RESCUE, lexer.ENSURE, lexer.RAISE,
		lexer.IN, lexer.REDO, lexer.RETRY,
		lexer.CATCH, lexer.THROW,
		lexer.ALIAS, lexer.UNDEF,
		lexer.DEFINED:
		return true
	}
	return false
}

func (p *Parser) expectPeek(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.parseError("expected next token to be %s, got %s instead", t, p.peekToken.Type)
	return false
}

func (p *Parser) consumeExpectedRParen() bool {
	if p.stopAtCallArgRParen && p.curTokenIs(lexer.RPAREN) {
		return true
	}
	if p.curTokenIs(lexer.RPAREN) && !p.peekTokenIs(lexer.RPAREN) {
		return true
	}
	if p.curTokenIs(lexer.NEWLINE) && p.prevToken.Type == lexer.RPAREN && !p.peekTokenIs(lexer.RPAREN) {
		return true
	}
	return p.expectPeek(lexer.RPAREN)
}

func (p *Parser) parseIdentifier() ast.Expression {
	ident := &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	if p.peekTokenIs(lexer.LBRACE) || (p.peekTokenIs(lexer.DO) && !p.stopAtDo) {
		call := &ast.MethodCall{
			Token:  p.curToken,
			Method: ident,
		}
		p.nextToken()
		call.Block = p.parseBlockExpression()
		if !p.peekTokenIs(lexer.DOT) {
			p.consumeBlockTerminator()
		}
		return call
	}

	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()
		return p.parseCallExpression(ident)
	}

	if p.isArgumentStart(p.peekToken) && (!p.peekTokenIs(lexer.LBRACKET) || p.methodCanTakeBareArrayArgument(ident.Value)) {
		call := &ast.MethodCall{
			Token:    p.curToken,
			Receiver: nil,
			Method:   ident,
		}

		p.nextToken()
		p.parseOneCallArg(call)

		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.skipPeekNewlines()
			p.nextToken()
			p.parseOneCallArgStoppingAtRParen(call)
		}

		if len(call.Args) == 1 {
			if arr, ok := call.Args[0].(*ast.ArrayLiteral); ok && len(arr.Elements) == 1 {
				return &ast.IndexExpression{
					Token: call.Token,
					Left:  call.Method,
					Index: arr.Elements[0],
				}
			}
		}

		return call
	}

	return ident
}

func (p *Parser) isArgumentStart(token lexer.Token) bool {
	switch token.Type {
	case lexer.STRING, lexer.INT, lexer.FLOAT, lexer.TRUE, lexer.FALSE, lexer.NIL,
		lexer.LBRACKET, lexer.IDENT, lexer.CONSTANT, lexer.SELF, lexer.MINUS, lexer.BANG, lexer.BIT_AND,
		lexer.REGEXP, lexer.SYMBOL, lexer.INCLUDE, lexer.AT, lexer.AT2, lexer.DOLLAR:
		return true
	default:
		return false
	}
}

func (p *Parser) methodCanTakeBareArrayArgument(name string) bool {
	return name == "puts" || name == "print" || name == "p" || strings.HasPrefix(name, "assert")
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

func (p *Parser) parseTerminatorExpression() ast.Expression {
	return &ast.NilExpression{
		Token: lexer.Token{Type: lexer.NIL, Literal: "nil"},
	}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{
		Token: p.curToken,
	}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		unsigned, unsignedErr := strconv.ParseUint(p.curToken.Literal, 0, 64)
		if unsignedErr != nil {
			bigValue, ok := new(big.Int).SetString(strings.ReplaceAll(p.curToken.Literal, "_", ""), 0)
			if !ok {
				p.parseError("could not parse %q as integer", p.curToken.Literal)
				return nil
			}
			value = int64(bigValue.Uint64())
		} else {
			value = int64(unsigned)
		}
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
		if strings.Contains(err.Error(), "too large") || math.IsInf(value, 1) {
			value = math.Inf(1)
		} else {
			p.parseError("could not parse %q as float", p.curToken.Literal)
			return nil
		}
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseRationalLiteral() ast.Expression {
	return &ast.RationalLiteral{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}
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
	literal := p.curToken.Literal
	pattern := literal
	options := ""
	if strings.HasPrefix(literal, "/") {
		lastSlash := strings.LastIndex(literal, "/")
		if lastSlash > 0 {
			pattern = literal[1:lastSlash]
			options = literal[lastSlash+1:]
		}
	}
	return &ast.RegexpLiteral{
		Token:   p.curToken,
		Pattern: pattern,
		Options: options,
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
	previousStopAtRBracket := p.stopAtRBracket
	p.stopAtRBracket = true
	element := p.parseExpression(LOWEST)
	p.stopAtRBracket = previousStopAtRBracket
	if element != nil {
		arr.Elements = append(arr.Elements, element)
	}
	for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
		if !p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		for p.peekTokenIs(lexer.NEWLINE) {
			p.nextToken()
		}
		if p.peekTokenIs(lexer.RBRACKET) {
			break
		}
		p.nextToken()
		previousStopAtRBracket := p.stopAtRBracket
		p.stopAtRBracket = true
		element := p.parseExpression(LOWEST)
		p.stopAtRBracket = previousStopAtRBracket
		if element != nil {
			arr.Elements = append(arr.Elements, element)
		}
	}
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	if p.curTokenIs(lexer.RBRACKET) && !p.peekTokenIs(lexer.RBRACKET) {
		return arr
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
		if p.peekTokenIs(lexer.RBRACE) {
			break
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
	if p.curTokenIs(lexer.POW) {
		key := p.parseDoubleSplatExpression()
		hash.Pairs[key] = &ast.NilExpression{Token: p.curToken}
		hash.Order = append(hash.Order, key)
		return
	}

	if p.curTokenIs(lexer.LPAREN) && p.peekTokenIs(lexer.RPAREN) {
		key := p.parseGroupedExpression()
		if p.peekTokenIs(lexer.ARROW) {
			p.nextToken()
			p.nextToken()
			value := p.parseExpression(LOWEST)
			hash.Pairs[key] = value
			hash.Order = append(hash.Order, key)
		}
		return
	}

	// Handle complex key expressions like [] or [1,2] as hash keys
	if p.curTokenIs(lexer.LBRACKET) {
		key := p.parseArrayLiteral()
		if p.peekTokenIs(lexer.ARROW) {
			p.nextToken()
			p.nextToken()
			value := p.parseExpression(LOWEST)
			hash.Pairs[key] = value
			hash.Order = append(hash.Order, key)
		}
		return
	}

	// Handle expression keys that are still syntactically simple enough to parse
	// before the hash rocket, e.g. { a[0] => 1 }.
	if (p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.CONSTANT) || p.curTokenIs(lexer.INT) || p.curTokenIs(lexer.FLOAT)) &&
		(p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.COLON2) || p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.LPAREN)) {
		p.parseHashRocketExpressionPair(hash, LOWEST)
		return
	}

	if p.curTokenIs(lexer.MINUS) && (p.peekTokenIs(lexer.INT) || p.peekTokenIs(lexer.FLOAT)) {
		p.parseHashRocketExpressionPair(hash, PREFIX)
		return
	}

	// Handle symbol shorthand: {foo: 1}, {false: false}, {nil: nil}, or quoted labels like {"foo": 1}.
	if p.peekTokenIs(lexer.COLON) && p.isHashLabelKey() {
		label := p.curToken.Literal
		key := &ast.SymbolLiteral{
			Token: p.curToken,
			Value: ":" + label,
		}
		p.nextToken()
		if p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RBRACE) {
			value := &ast.Identifier{Token: key.Token, Value: label}
			hash.Pairs[key] = value
			hash.Order = append(hash.Order, key)
			return
		}
		p.nextToken()
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		hash.Order = append(hash.Order, key)
		return
	}

	// Handle hash rocket: {"foo" => "bar"}, {:a => 1}, {nil => true}, etc.
	if p.peekTokenIs(lexer.ARROW) {
		keyToken := p.curToken
		var key ast.Expression
		switch keyToken.Type {
		case lexer.STRING:
			key = &ast.StringLiteral{Token: keyToken, Value: keyToken.Literal}
		case lexer.SYMBOL:
			key = &ast.SymbolLiteral{Token: keyToken, Value: keyToken.Literal}
		case lexer.IDENT:
			key = &ast.Identifier{Token: keyToken, Value: keyToken.Literal}
		case lexer.CONSTANT:
			key = &ast.Constant{Token: keyToken, Name: keyToken.Literal}
		case lexer.INT:
			n, _ := strconv.ParseInt(keyToken.Literal, 10, 64)
			key = &ast.IntegerLiteral{Token: keyToken, Value: n}
		case lexer.FLOAT:
			f, _ := strconv.ParseFloat(keyToken.Literal, 64)
			key = &ast.FloatLiteral{Token: keyToken, Value: f}
		case lexer.TRUE:
			key = &ast.Boolean{Token: keyToken, Value: true}
		case lexer.FALSE:
			key = &ast.Boolean{Token: keyToken, Value: false}
		case lexer.NIL:
			key = &ast.NilExpression{Token: keyToken}
		case lexer.SELF:
			key = &ast.SelfExpression{Token: keyToken}
		default:
			key = &ast.Identifier{Token: keyToken, Value: keyToken.Literal}
		}
		p.nextToken() // move to ARROW
		p.nextToken() // move to value
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		hash.Order = append(hash.Order, key)
		return
	}

	if p.peekTokenIs(lexer.COLON) && p.isHashLabelKey() {
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

func (p *Parser) parseHashRocketExpressionPair(hash *ast.HashLiteral, precedence int) {
	previousStopAtHashRocket := p.stopAtHashRocket
	p.stopAtHashRocket = true
	key := p.parseExpression(precedence)
	p.stopAtHashRocket = previousStopAtHashRocket
	if p.peekTokenIs(lexer.ARROW) {
		p.nextToken()
		p.nextToken()
		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value
		hash.Order = append(hash.Order, key)
	}
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{
		Token: p.curToken,
		Left:  left,
	}

	p.nextToken()
	args := []ast.Expression{}
	if p.curTokenIs(lexer.RBRACKET) {
		if _, ok := left.(*ast.ConstantResolution); ok {
			return &ast.MethodCall{
				Token:    exp.Token,
				Receiver: left,
				Method:   &ast.Identifier{Token: exp.Token, Value: "[]"},
				Args:     args,
			}
		}
		exp.Index = &ast.NilExpression{Token: p.curToken}
		return exp
	}
	previousStopAtRBracket := p.stopAtRBracket
	p.stopAtRBracket = true
	args = append(args, p.parseExpression(LOWEST))
	p.stopAtRBracket = previousStopAtRBracket

	if p.peekTokenIs(lexer.COMMA) {
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			previousStopAtRBracket := p.stopAtRBracket
			p.stopAtRBracket = true
			args = append(args, p.parseExpression(LOWEST))
			p.stopAtRBracket = previousStopAtRBracket
		}
	}

	if !(p.curTokenIs(lexer.RBRACKET) && !p.peekTokenIs(lexer.RBRACKET)) && !p.expectPeek(lexer.RBRACKET) {
		return nil
	}

	if _, ok := left.(*ast.ConstantResolution); ok || len(args) > 2 {
		return &ast.MethodCall{
			Token:    exp.Token,
			Receiver: left,
			Method:   &ast.Identifier{Token: exp.Token, Value: "[]"},
			Args:     args,
		}
	}

	exp.Index = args[0]
	if len(args) == 2 {
		exp.End = args[1]
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
	if p.curTokenIs(lexer.ASSIGN) {
		p.nextToken()
		return &ast.AssignExpression{
			Token: exp.Token,
			Name:  &ast.Identifier{Token: exp.Token, Value: "_"},
			Value: p.parseExpression(LOWEST),
		}
	}
	if p.curTokenIs(lexer.RPAREN) || p.curTokenIs(lexer.COMMA) {
		exp.Value = &ast.Identifier{Token: exp.Token, Value: "_"}
		return exp
	}
	if p.curTokenIs(lexer.MINUS_ARROW) {
		exp.Value = p.parseExpression(CALL)
		return exp
	}
	exp.Value = p.parseExpression(PREFIX)

	return exp
}

func (p *Parser) parseDoubleSplatExpression() ast.Expression {
	return p.parseSplatExpression()
}

func (p *Parser) parseBlockPassExpression() ast.Expression {
	exp := &ast.SplatExpression{
		Token: p.curToken,
	}

	if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.COMMA) {
		exp.Value = &ast.Identifier{Token: p.curToken, Value: "_"}
		return exp
	}

	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	if p.curTokenIs(lexer.PLUS) && p.peekTokenIs(lexer.PLUS) {
		if ident, ok := left.(*ast.Identifier); ok {
			p.nextToken()
			return &ast.AssignExpression{
				Token: p.curToken,
				Name:  ident,
				Value: &ast.InfixExpression{
					Token:    p.curToken,
					Left:     ident,
					Operator: "+",
					Right: &ast.IntegerLiteral{
						Token: lexer.Token{Type: lexer.INT, Literal: "1"},
						Value: 1,
					},
				},
			}
		}
	}

	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	prec := p.curPrecedence()
	p.nextToken()
	for p.curTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
	expression.Right = p.parseExpression(prec)

	return expression
}

func (p *Parser) parseTernaryExpression(condition ast.Expression) ast.Expression {
	exp := &ast.TernaryExpression{
		Token:     p.curToken,
		Condition: condition,
	}

	p.nextToken()
	previousStopAtColon := p.stopAtColon
	p.stopAtColon = true
	exp.Consequent = p.parseExpression(LOWEST)
	p.stopAtColon = previousStopAtColon

	if !p.consumeExpectedColon() {
		return nil
	}

	p.nextToken()
	exp.Alternative = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) consumeExpectedColon() bool {
	if p.curTokenIs(lexer.COLON) {
		return true
	}
	return p.expectPeek(lexer.COLON)
}

func (p *Parser) parseRangeExpression(left ast.Expression) ast.Expression {
	exp := &ast.RangeExpression{
		Token: p.curToken,
		Left:  left,
	}

	exp.Exclusive = p.curTokenIs(lexer.DOT3)

	p.nextToken()
	if p.curTokenEndsRange() {
		exp.Right = p.missingRangeBound()
		return exp
	}
	exp.Right = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseBeginlessRangeExpression() ast.Expression {
	exp := &ast.RangeExpression{
		Token:     p.curToken,
		Left:      p.missingRangeBound(),
		Exclusive: p.curTokenIs(lexer.DOT3),
	}

	p.nextToken()
	exp.Right = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) curTokenEndsRange() bool {
	switch p.curToken.Type {
	case lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE, lexer.COMMA, lexer.NEWLINE, lexer.SEMICOLON, lexer.EOF:
		return true
	default:
		return false
	}
}

func (p *Parser) missingRangeBound() *ast.NilExpression {
	return &ast.NilExpression{
		Token: lexer.Token{Type: lexer.NIL, Literal: "nil"},
	}
}

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	assign := &ast.AssignExpression{
		Token: p.curToken,
	}

	if ident, ok := left.(*ast.Identifier); ok {
		assign.Name = ident
	} else if constant, ok := left.(*ast.Constant); ok {
		assign.Name = &ast.Identifier{
			Token: constant.Token,
			Value: constant.Name,
		}
	} else if constantResolution, ok := left.(*ast.ConstantResolution); ok {
		assign.Name = constantResolution.Name
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
	} else if arr, ok := left.(*ast.ArrayLiteral); ok {
		assign.Name = &ast.Identifier{
			Token: arr.Token,
			Value: arr.String(),
		}
	} else if splat, ok := left.(*ast.SplatExpression); ok {
		name := assignmentName(splat.Value)
		if name == nil {
			p.parseError("invalid assignment target %T", left)
			return nil
		}
		assign.Name = name
	} else if idx, ok := left.(*ast.IndexExpression); ok {
		assign.Target = idx.Left
		assign.Index = idx.Index
		assign.End = idx.End
		switch target := idx.Left.(type) {
		case *ast.Identifier:
			assign.Name = target
		case *ast.InstanceVariable:
			assign.Name = &ast.Identifier{Token: target.Token, Value: target.Name}
		case *ast.ClassVariable:
			assign.Name = &ast.Identifier{Token: target.Token, Value: target.Name}
		case *ast.GlobalVariable:
			assign.Name = &ast.Identifier{Token: target.Token, Value: target.Name}
		default:
			assign.Name = &ast.Identifier{Token: idx.Token, Value: idx.Left.String()}
		}
	} else if call, ok := left.(*ast.MethodCall); ok && call.Receiver == nil && len(call.Args) == 1 {
		if arr, ok := call.Args[0].(*ast.ArrayLiteral); ok && len(arr.Elements) == 1 {
			assign.Name = call.Method
			assign.Index = arr.Elements[0]
		} else {
			p.parseError("invalid assignment target")
			return nil
		}
	} else if call, ok := left.(*ast.MethodCall); ok && call.Receiver != nil && len(call.Args) == 0 && len(call.KeywordArgs) == 0 {
		p.nextToken()
		call.Method = &ast.Identifier{
			Token: call.Method.Token,
			Value: call.Method.Value + "=",
		}
		value := p.parseAssignmentValue()
		call.Args = append(call.Args, value)
		return call
	} else if call, ok := left.(*ast.MethodCall); ok && call.Receiver != nil && call.Method != nil && call.Method.Value == "[]" && len(call.KeywordArgs) == 0 {
		p.nextToken()
		call.Method = &ast.Identifier{
			Token: call.Method.Token,
			Value: "[]=",
		}
		value := p.parseAssignmentValue()
		call.Args = append(call.Args, value)
		return call
	} else if infix, ok := left.(*ast.InfixExpression); ok {
		if assign := p.assignRightHandSideOfInfix(infix); assign != nil {
			return infix
		}
		p.parseError("invalid assignment target %T", left)
		return nil
	} else {
		p.parseError("invalid assignment target %T", left)
		return nil
	}

	p.nextToken()
	assign.Value = p.parseAssignmentValue()
	if p.curTokenIs(lexer.RPAREN) {
		p.lastClosedGroupDepth = p.groupDepth
	}

	return assign
}

func (p *Parser) assignRightHandSideOfInfix(infix *ast.InfixExpression) *ast.AssignExpression {
	name := assignmentName(infix.Right)
	if name == nil {
		if nested, ok := infix.Right.(*ast.InfixExpression); ok {
			return p.assignRightHandSideOfInfix(nested)
		}
		return nil
	}

	assign := &ast.AssignExpression{
		Token: p.curToken,
		Name:  name,
	}
	p.nextToken()
	assign.Value = p.parseAssignmentValue()
	infix.Right = assign
	return assign
}

func (p *Parser) currentAssignmentName() *ast.Identifier {
	switch p.curToken.Type {
	case lexer.IDENT, lexer.CONSTANT, lexer.AT, lexer.AT2, lexer.DOLLAR:
		return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	case lexer.MULTIPLY:
		if p.peekTokenIs(lexer.IDENT) {
			p.nextToken()
			return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		}
		return &ast.Identifier{Token: p.curToken, Value: "_"}
	default:
		return nil
	}
}

func assignmentName(expr ast.Expression) *ast.Identifier {
	switch right := expr.(type) {
	case *ast.Identifier:
		return right
	case *ast.Constant:
		return &ast.Identifier{Token: right.Token, Value: right.Name}
	case *ast.ConstantResolution:
		return right.Name
	case *ast.InstanceVariable:
		return &ast.Identifier{Token: right.Token, Value: right.Name}
	case *ast.ClassVariable:
		return &ast.Identifier{Token: right.Token, Value: right.Name}
	case *ast.GlobalVariable:
		return &ast.Identifier{Token: right.Token, Value: right.Name}
	case *ast.IndexExpression:
		return &ast.Identifier{Token: right.Token, Value: right.String()}
	case *ast.MethodCall:
		return &ast.Identifier{Token: right.Token, Value: right.String()}
	case *ast.ArrayLiteral:
		return &ast.Identifier{Token: right.Token, Value: right.String()}
	case *ast.SplatExpression:
		if name := assignmentName(right.Value); name != nil {
			return name
		}
		return &ast.Identifier{Token: right.Token, Value: "_"}
	default:
		return nil
	}
}

func (p *Parser) parseAssignmentValue() ast.Expression {
	p.skipCurSeparators()
	first := p.parseExpression(LOWEST)
	if !p.curTokenIs(lexer.COMMA) && !p.peekTokenIs(lexer.COMMA) {
		return first
	}

	values := []ast.Expression{}
	if first != nil {
		values = append(values, first)
	}
	for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
		if !p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		if p.assignmentValueEndsAfterComma() {
			break
		}
		p.nextToken()
		value := p.parseExpression(LOWEST)
		if value != nil {
			values = append(values, value)
		}
	}

	return &ast.ArrayLiteral{
		Token:    lexer.Token{Type: lexer.LBRACKET, Literal: "["},
		Elements: values,
	}
}

func (p *Parser) assignmentValueEndsAfterComma() bool {
	switch p.peekToken.Type {
	case lexer.NEWLINE, lexer.SEMICOLON, lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE, lexer.EOF:
		return true
	default:
		return false
	}
}

func (p *Parser) parseMethodCall(left ast.Expression) ast.Expression {
	call := &ast.MethodCall{
		Token:    p.curToken,
		Receiver: left,
		Safe:     p.curTokenIs(lexer.SAFE_NAV),
	}
	p.skipPeekNewlines()

	if p.peekTokenIs(lexer.LBRACKET) {
		p.nextToken()
		tok := p.curToken
		if !p.expectPeek(lexer.RBRACKET) {
			return nil
		}
		methodName := "[]"
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken()
			methodName = "[]="
		}
		call.Method = &ast.Identifier{
			Token: tok,
			Value: methodName,
		}
	} else if p.peekTokenCanBeMethodName() {
		p.nextToken()
		call.Method = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}

	if p.peekTokenIs(lexer.LPAREN) {
		call.ParenthesizedArgs = true
		if call.Method == nil {
			call.Method = &ast.Identifier{
				Token: p.peekToken,
				Value: "call",
			}
		}
		p.nextToken()
		p.skipPeekNewlines()

		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
		} else {
			p.nextToken()
			p.parseOneCallArgStoppingAtRParen(call)

			for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
				if !p.curTokenIs(lexer.COMMA) {
					p.nextToken()
				}
				p.skipPeekNewlines()
				if p.peekTokenIs(lexer.RPAREN) {
					break
				}
				p.nextToken()
				p.parseOneCallArgStoppingAtRParen(call)
			}

			if p.curTokenIs(lexer.RPAREN) && p.peekTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
			if p.curTokenIs(lexer.RBRACE) && p.peekTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
			if p.curTokenIs(lexer.RBRACE) && p.peekTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
			p.parseSingleArgumentDotChain(call)
			if !p.curTokenIs(lexer.RPAREN) {
				p.skipPeekNewlines()
			}
			if !p.consumeExpectedRParen() {
				return nil
			}
		}
	}

	if p.peekTokenIs(lexer.LBRACKET) && call.Method != nil && !call.ParenthesizedArgs && p.hasSpaceBetween(call.Method.Token, p.peekToken) {
		p.nextToken()
		if arg := p.parseArrayLiteral(); arg != nil {
			call.Args = append(call.Args, arg)
		}
		return call
	}

	// Handle index access: obj.method[key]
	if p.peekTokenIs(lexer.LBRACKET) {
		p.nextToken()
		return p.parseIndexExpression(call)
	}

	if len(call.Args) == 0 && len(call.KeywordArgs) == 0 && p.isArgumentStart(p.peekToken) {
		p.nextToken()
		p.parseOneCallArg(call)
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.skipPeekNewlines()
			p.nextToken()
			p.parseOneCallArgStoppingAtRParen(call)
		}
	}

	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
		if p.shouldConsumeBlockTerminator() {
			p.consumeBlockTerminator()
		}
	} else if p.peekTokenIs(lexer.DO) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
		if p.shouldConsumeBlockTerminator() {
			p.consumeBlockTerminator()
		}
	}

	return call
}

func (p *Parser) hasSpaceBetween(left, right lexer.Token) bool {
	if left.Line == 0 || right.Line == 0 || left.Line != right.Line {
		return false
	}
	return right.Column > left.Column
}

func (p *Parser) peekTokenCanBeMethodName() bool {
	switch p.peekToken.Type {
	case lexer.IDENT, lexer.CONSTANT, lexer.SELF, lexer.CLASS, lexer.BEGIN, lexer.DO, lexer.END, lexer.FOR, lexer.IN, lexer.PREPEND, lexer.THEN, lexer.UNTIL, lexer.YIELD, lexer.MATCH, lexer.NOT_EQUAL,
		lexer.TRUE, lexer.FALSE, lexer.NIL, lexer.EXTEND, lexer.INCLUDE, lexer.RAISE, lexer.THROW, lexer.CATCH,
		lexer.BREAK, lexer.NEXT, lexer.REDO, lexer.RETRY,
		lexer.EQUAL, lexer.EQUAL3, lexer.SPACESHIP, lexer.LESS_THAN, lexer.LESS_THAN_OR_EQUAL,
		lexer.GREATER_THAN, lexer.GREATER_THAN_OR_EQUAL, lexer.PLUS, lexer.MINUS, lexer.MULTIPLY,
		lexer.DIVIDE, lexer.MOD, lexer.BIT_AND, lexer.BIT_OR, lexer.BIT_XOR, lexer.LSHIFT:
		return true
	default:
		return false
	}
}

func (p *Parser) parseCallExpression(fn ast.Expression) ast.Expression {
	call := &ast.MethodCall{
		Token:             p.curToken,
		ParenthesizedArgs: true,
	}

	if ident, ok := fn.(*ast.Identifier); ok {
		call.Method = &ast.Identifier{
			Token: ident.Token,
			Value: ident.Value,
		}
	} else if constant, ok := fn.(*ast.Constant); ok {
		call.Method = &ast.Identifier{
			Token: constant.Token,
			Value: constant.Name,
		}
	} else {
		p.parseError("expected identifier for function call, got %T", fn)
		return nil
	}

	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
	} else {
		p.skipPeekNewlines()
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
		} else {
			p.nextToken()
			p.parseOneCallArgStoppingAtRParen(call)

			for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
				if !p.curTokenIs(lexer.COMMA) {
					p.nextToken()
				}
				p.skipPeekNewlines()
				if p.peekTokenIs(lexer.RPAREN) {
					break
				}
				p.nextToken()
				p.parseOneCallArgStoppingAtRParen(call)
			}

			p.parseSingleArgumentDotChain(call)
			if !p.curTokenIs(lexer.RPAREN) {
				p.skipPeekNewlines()
			}
			if !p.consumeExpectedRParen() {
				return nil
			}
		}
	}

	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
		if p.shouldConsumeBlockTerminator() {
			p.consumeBlockTerminator()
		}
	} else if p.peekTokenIs(lexer.DO) {
		p.nextToken()
		call.Block = p.parseBlockExpression()
		if p.shouldConsumeBlockTerminator() {
			p.consumeBlockTerminator()
		}
	}

	return call
}

func (p *Parser) parseSingleArgumentDotChain(call *ast.MethodCall) {
	if len(call.Args) != 1 || !p.curTokenIs(lexer.RPAREN) || !p.peekTokenIs(lexer.DOT) {
		return
	}
	if !p.currentTokenClosedChildGroup() {
		return
	}
	next := p.tokenAfterPeek()
	if next.Literal == "should" || next.Literal == "should_not" {
		return
	}
	if !p.dotChainCanStayWithinArgument() {
		return
	}
	arg := call.Args[0]
	if !argumentCanContinueWithDot(arg) {
		return
	}
	for p.peekTokenIs(lexer.DOT) {
		p.nextToken()
		arg = p.parseMethodCall(arg)
	}
	call.Args[0] = arg
}

func (p *Parser) tokenAfterPeek() lexer.Token {
	lexerCopy := *p.l
	return lexerCopy.NextToken()
}

func (p *Parser) dotChainCanStayWithinArgument() bool {
	lexerCopy := *p.l
	for {
		method := lexerCopy.NextToken()
		if method.Type == lexer.EOF {
			return false
		}
		next := lexerCopy.NextToken()
		if next.Type == lexer.LPAREN {
			next = scanAfterBalancedParen(&lexerCopy)
		}
		switch next.Type {
		case lexer.RPAREN, lexer.COMMA:
			return true
		case lexer.DOT:
			continue
		default:
			return false
		}
	}
}

func scanAfterBalancedParen(l *lexer.Lexer) lexer.Token {
	depth := 1
	for depth > 0 {
		tok := l.NextToken()
		switch tok.Type {
		case lexer.EOF:
			return tok
		case lexer.LPAREN:
			depth++
		case lexer.RPAREN:
			depth--
		}
	}
	return l.NextToken()
}

func argumentCanContinueWithDot(expr ast.Expression) bool {
	switch expr.(type) {
	case *ast.PrefixExpression, *ast.InfixExpression, *ast.AssignExpression, *ast.RangeExpression, *ast.PatternMatchExpression, *ast.BeginExpression:
		return true
	default:
		return false
	}
}

func (p *Parser) parseOneCallArg(call *ast.MethodCall) {
	if p.curTokenIs(lexer.INCLUDE) && p.peekTokenIs(lexer.LPAREN) {
		call.Args = append(call.Args, p.parseKeywordCallArgument())
		return
	}
	if p.curTokenIs(lexer.DOT3) && p.peekTokenIs(lexer.RPAREN) {
		call.Args = append(call.Args, &ast.SplatExpression{
			Token: p.curToken,
			Value: &ast.Identifier{Token: p.curToken, Value: "..."},
		})
		return
	}
	if p.curTokenIs(lexer.BIT_AND) {
		if arg := p.parseBlockPassExpression(); arg != nil {
			call.Args = append(call.Args, arg)
		}
		return
	}
	if p.curTokenCanBeKeywordArgName() && p.peekTokenIs(lexer.COLON) {
		name := p.curToken.Literal
		tok := p.curToken
		p.nextToken() // consume COLON
		if p.peekTokenEndsArgument() {
			call.KeywordArgs = append(call.KeywordArgs, &ast.KeywordArg{
				Token: tok,
				Name:  name,
				Value: &ast.Identifier{Token: tok, Value: name},
			})
			return
		}
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

func (p *Parser) parseOneCallArgStoppingAtRParen(call *ast.MethodCall) {
	if p.curTokenIs(lexer.LPAREN) {
		previousStopAtCallArgRParen := p.stopAtCallArgRParen
		p.stopAtCallArgRParen = true
		p.parseOneCallArg(call)
		p.stopAtCallArgRParen = previousStopAtCallArgRParen
		return
	}
	previousStopAtRParen := p.stopAtRParen
	previousStopAtCallArgRParen := p.stopAtCallArgRParen
	p.stopAtRParen = true
	p.stopAtCallArgRParen = true
	p.parseOneCallArg(call)
	p.stopAtRParen = previousStopAtRParen
	p.stopAtCallArgRParen = previousStopAtCallArgRParen
}

func (p *Parser) curTokenCanBeKeywordArgName() bool {
	switch p.curToken.Type {
	case lexer.IDENT, lexer.IN, lexer.PREPEND:
		return true
	default:
		return false
	}
}

func (p *Parser) parseKeywordCallArgument() ast.Expression {
	argCall := &ast.MethodCall{
		Token: p.curToken,
		Method: &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		},
	}

	p.nextToken() // consume (
	p.skipPeekNewlines()
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return argCall
	}

	p.nextToken()
	p.parseOneCallArg(argCall)
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.skipPeekNewlines()
		if p.peekTokenIs(lexer.RPAREN) {
			break
		}
		p.nextToken()
		p.parseOneCallArg(argCall)
	}

	if !p.curTokenIs(lexer.RPAREN) {
		p.skipPeekNewlines()
	}
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
	}

	return argCall
}

func (p *Parser) skipPeekNewlines() {
	for p.peekTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}
}

func (p *Parser) parseConstant() ast.Expression {
	return &ast.Constant{
		Token: p.curToken,
		Name:  p.curToken.Literal,
	}
}

func (p *Parser) parseTopLevelConstantResolution() ast.Expression {
	res := &ast.ConstantResolution{
		Token: p.curToken,
	}

	if !p.expectPeek(lexer.CONSTANT) {
		return nil
	}

	res.Name = &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	return res
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
	if p.curTokenIs(lexer.ARROW) {
		switch left.(type) {
		case *ast.ArrayLiteral, *ast.HashLiteral:
			return p.parsePatternMatchExpression(left)
		}
	}

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

func (p *Parser) parsePatternMatchExpression(left ast.Expression) ast.Expression {
	exp := &ast.PatternMatchExpression{
		Token: p.curToken,
		Left:  left,
	}
	p.nextToken()
	p.skipPatternTokens(lexer.NEWLINE, lexer.SEMICOLON, lexer.RPAREN, lexer.RBRACE, lexer.EOF)
	return exp
}

func (p *Parser) skipPatternTokens(stops ...lexer.TokenType) {
	depth := 0
	for !p.curTokenIs(lexer.EOF) {
		if depth == 0 && p.curTokenIsAny(stops...) {
			return
		}
		switch p.curToken.Type {
		case lexer.LPAREN, lexer.LBRACKET, lexer.LBRACE:
			depth++
		case lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE:
			if depth == 0 {
				return
			}
			depth--
		}
		if depth == 0 && p.peekTokenIsAny(stops...) {
			p.nextToken()
			return
		}
		p.nextToken()
	}
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.groupDepth++
	defer func() {
		p.groupDepth--
	}()

	p.nextToken()
	p.skipCurSeparators()

	if p.curTokenIs(lexer.RPAREN) {
		return &ast.NilExpression{Token: p.curToken}
	}

	previousStopAtRParen := p.stopAtRParen
	p.stopAtRParen = true
	exp := p.parseExpression(LOWEST)
	exp = p.parseGroupedPostfixModifier(exp)
	exp = p.parseGroupedCommaExpression(exp)
	var consumedTerminator bool
	exp, consumedTerminator = p.parseGroupedSequenceExpression(exp)
	p.stopAtRParen = previousStopAtRParen

	if consumedTerminator {
		return exp
	}

	if !p.consumeExpectedRParen() {
		return nil
	}
	if p.curTokenIs(lexer.RPAREN) {
		p.lastClosedGroupDepth = p.groupDepth
	}
	return exp
}

func (p *Parser) parseGroupedCommaExpression(first ast.Expression) ast.Expression {
	if first == nil || !p.peekTokenIs(lexer.COMMA) {
		return first
	}

	values := []ast.Expression{first}
	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.skipPeekNewlines()
		if p.peekTokenIs(lexer.RPAREN) {
			break
		}
		p.nextToken()
		p.skipCurSeparators()
		value := p.parseExpression(LOWEST)
		if value != nil {
			values = append(values, value)
		}
	}

	return &ast.ArrayLiteral{
		Token:    lexer.Token{Type: lexer.LBRACKET, Literal: "["},
		Elements: values,
	}
}

func (p *Parser) parseGroupedSequenceExpression(first ast.Expression) (ast.Expression, bool) {
	if first == nil {
		return nil, false
	}
	if p.curTokenIs(lexer.RPAREN) && !p.peekTokenIs(lexer.RPAREN) {
		p.lastClosedGroupDepth = p.groupDepth
		if p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.SAFE_NAV) {
			p.allowGroupedRParenDotChain = true
		}
		return first, true
	}
	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.peekTokenIs(lexer.NEWLINE) && !p.peekTokenIs(lexer.SEMICOLON) {
		return first, false
	}

	body := &ast.BlockExpression{
		Token: p.curToken,
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Token: p.curToken, Expression: first},
		},
	}

	if !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && (p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON)) {
		p.nextToken()
	}
	p.skipCurSeparators()

	for !p.curTokenIs(lexer.RPAREN) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			body.Statements = append(body.Statements, stmt)
		}
		if p.curTokenIs(lexer.RPAREN) {
			break
		}
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
			break
		}
		p.nextToken()
		p.skipCurSeparators()
	}

	return &ast.BeginExpression{Token: body.Token, Body: body}, p.curTokenIs(lexer.RPAREN)
}

func (p *Parser) parseGroupedPostfixModifier(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}
	if p.curTokenIs(lexer.IF) || p.peekTokenIs(lexer.IF) {
		if !p.curTokenIs(lexer.IF) {
			p.nextToken()
		}
		return p.parseIfModifier(expr)
	}
	if p.curTokenIs(lexer.UNLESS) || p.peekTokenIs(lexer.UNLESS) {
		if !p.curTokenIs(lexer.UNLESS) {
			p.nextToken()
		}
		return p.parseUnlessModifier(expr)
	}
	if p.curTokenIs(lexer.WHILE) || p.peekTokenIs(lexer.WHILE) {
		if !p.curTokenIs(lexer.WHILE) {
			p.nextToken()
		}
		return p.parseWhileModifier(expr)
	}
	if p.curTokenIs(lexer.UNTIL) || p.peekTokenIs(lexer.UNTIL) {
		if !p.curTokenIs(lexer.UNTIL) {
			p.nextToken()
		}
		return p.parseUntilModifier(expr)
	}
	return expr
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

	if p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) || p.peekTokenIs(lexer.END) || p.peekTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.DOT) || p.peekTokenIs(lexer.SAFE_NAV) {
		return exp
	}

	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()
		p.skipPeekNewlines()

		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
			return exp
		}

		p.nextToken()
		p.parseOneYieldArg(exp)
		for p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.skipPeekNewlines()
			if p.peekTokenIs(lexer.RPAREN) {
				break
			}
			p.nextToken()
			p.parseOneYieldArg(exp)
		}

		if !p.curTokenIs(lexer.RPAREN) {
			p.skipPeekNewlines()
		}
		if !p.consumeExpectedRParen() {
			return nil
		}
		return exp
	}

	p.nextToken()
	p.parseOneYieldArg(exp)

	for p.peekTokenIs(lexer.COMMA) {
		p.nextToken()
		p.nextToken()
		p.parseOneYieldArg(exp)
	}

	return exp
}

func (p *Parser) parseOneYieldArg(exp *ast.YieldExpression) {
	if p.curTokenCanBeKeywordArgName() && p.peekTokenIs(lexer.COLON) {
		name := p.curToken.Literal
		tok := p.curToken
		p.nextToken()
		if p.peekTokenEndsArgument() {
			exp.KeywordArgs = append(exp.KeywordArgs, &ast.KeywordArg{
				Token: tok,
				Name:  name,
				Value: &ast.Identifier{Token: tok, Value: name},
			})
			return
		}
		p.nextToken()
		exp.KeywordArgs = append(exp.KeywordArgs, &ast.KeywordArg{
			Token: tok,
			Name:  name,
			Value: p.parseExpression(LOWEST),
		})
		return
	}

	arg := p.parseExpression(LOWEST)
	if arg != nil {
		exp.Args = append(exp.Args, arg)
	}
}

func (p *Parser) peekTokenEndsArgument() bool {
	switch p.peekToken.Type {
	case lexer.COMMA, lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE, lexer.NEWLINE, lexer.SEMICOLON, lexer.EOF:
		return true
	default:
		return false
	}
}

func (p *Parser) parseSuperExpression() ast.Expression {
	exp := &ast.SuperExpression{
		Token:        p.curToken,
		ImplicitArgs: true,
	}

	if p.peekTokenContinuesBareSuperExpression() {
		return exp
	}
	if p.peekTokenEndsArgument() || p.peekTokenIs(lexer.END) {
		return exp
	}

	p.nextToken()
	if p.curTokenIs(lexer.LPAREN) {
		exp.ImplicitArgs = false
		p.skipPeekNewlines()
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
		} else {
			p.nextToken()
			if p.curTokenIs(lexer.DOT3) && p.peekTokenIs(lexer.RPAREN) {
				exp.Args = append(exp.Args, &ast.SplatExpression{
					Token: p.curToken,
					Value: &ast.Identifier{Token: p.curToken, Value: "..."},
				})
			} else {
				arg := p.parseExpression(LOWEST)
				if arg != nil {
					exp.Args = append(exp.Args, arg)
				}
			}

			for p.peekTokenIs(lexer.COMMA) {
				p.nextToken()
				p.skipPeekNewlines()
				if p.peekTokenIs(lexer.RPAREN) {
					break
				}
				p.nextToken()
				arg := p.parseExpression(LOWEST)
				if arg != nil {
					exp.Args = append(exp.Args, arg)
				}
			}

			if !p.curTokenIs(lexer.RPAREN) {
				p.skipPeekNewlines()
			}
			if !p.consumeExpectedRParen() {
				return nil
			}
		}
		if p.peekTokenIs(lexer.LBRACE) || (p.peekTokenIs(lexer.DO) && !p.stopAtDo) {
			p.nextToken()
			exp.Block = p.parseBlockExpression()
		}
		return exp
	}
	if p.curTokenIs(lexer.LBRACE) || (p.curTokenIs(lexer.DO) && !p.stopAtDo) {
		exp.Block = p.parseBlockExpression()
		return exp
	}

	for !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.DO) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		exp.ImplicitArgs = false
		arg := p.parseExpression(LOWEST)
		if arg != nil {
			exp.Args = append(exp.Args, arg)
		}

		if p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
	}
	if p.curTokenIs(lexer.LBRACE) || (p.curTokenIs(lexer.DO) && !p.stopAtDo) {
		exp.Block = p.parseBlockExpression()
	}

	return exp
}

func (p *Parser) peekTokenContinuesBareSuperExpression() bool {
	switch p.peekToken.Type {
	case lexer.PLUS, lexer.MINUS, lexer.MULTIPLY, lexer.DIVIDE, lexer.MOD, lexer.POW,
		lexer.EQUAL, lexer.EQUAL3, lexer.BANG_EQUAL, lexer.NOT_EQUAL, lexer.MATCH,
		lexer.LESS_THAN, lexer.GREATER_THAN, lexer.LESS_THAN_OR_EQUAL,
		lexer.GREATER_THAN_OR_EQUAL, lexer.SPACESHIP,
		lexer.AND, lexer.OR, lexer.AND2, lexer.OR2,
		lexer.LSHIFT, lexer.RSHIFT, lexer.BIT_AND, lexer.BIT_OR, lexer.BIT_XOR:
		return true
	default:
		return false
	}
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

	// Accept "then", newline, semicolon, or "{" after condition
	if p.peekTokenIs(lexer.THEN) {
		p.nextToken() // consume "then"
	} else if !p.peekTokenIs(lexer.NEWLINE) && !p.peekTokenIs(lexer.SEMICOLON) && !p.peekTokenIs(lexer.LBRACE) {
		p.parseError("expected then, newline, ;, or { after if condition, got %s", p.peekToken.Type)
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
			if p.curTokenIs(lexer.END) && statementIsBeginExpression(stmt) && !p.peekTokenIs(lexer.EOF) {
				p.nextToken()
				p.skipCurNewlines()
				continue
			}
			if p.curTokenIs(lexer.END) || p.curTokenIs(lexer.ELSIF) || p.curTokenIs(lexer.ELSE) || p.curTokenIs(lexer.EOF) {
				break
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
			if p.curTokenIs(lexer.END) && statementIsBeginExpression(stmt) && !p.peekTokenIs(lexer.EOF) {
				p.nextToken()
				p.skipCurNewlines()
				continue
			}
			if p.curTokenIs(lexer.END) || p.curTokenIs(lexer.ELSIF) || p.curTokenIs(lexer.ELSE) || p.curTokenIs(lexer.EOF) {
				break
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
			if p.curTokenIs(lexer.END) && statementIsBeginExpression(stmt) && !p.peekTokenIs(lexer.EOF) {
				p.nextToken()
				p.skipCurNewlines()
				continue
			}
			if p.curTokenIs(lexer.END) || p.curTokenIs(lexer.EOF) {
				break
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

	if p.peekTokenIs(lexer.THEN) {
		p.nextToken()
	}

	if p.peekTokenIs(lexer.LBRACE) {
		p.nextToken()
		exp.Consequent = p.parseBlockExpression()
	} else {
		p.nextToken()
		p.skipCurNewlines()
		exp.Consequent = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Consequent.Statements = append(exp.Consequent.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
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

	exp.IsUnless = true
	return exp
}

func (p *Parser) parseCaseExpression() ast.Expression {
	exp := &ast.CaseExpression{
		Token: p.curToken,
	}

	p.nextToken()
	p.skipCurNewlines()

	if !p.curTokenIs(lexer.WHEN) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		exp.Expression = p.parseExpression(LOWEST)
		p.nextToken()
	}

	p.skipCurNewlines()
	for p.curTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
	p.skipCurNewlines()

	hasPatternClause := false
	for p.curTokenIs(lexer.WHEN) || p.peekTokenIs(lexer.WHEN) || p.curTokenIs(lexer.IN) || p.peekTokenIs(lexer.IN) {
		if !p.curTokenIs(lexer.WHEN) && !p.curTokenIs(lexer.IN) {
			p.nextToken()
		}
		clause := &ast.CaseClause{
			Token: p.curToken,
		}

		p.nextToken()
		if clause.Token.Type == lexer.IN {
			p.skipCurNewlines()
		} else {
			p.skipCurNewlines()
		}

		if clause.Token.Type == lexer.IN {
			hasPatternClause = true
			clause.Conditions = append(clause.Conditions, &ast.PatternMatchExpression{Token: clause.Token})
			p.skipPatternTokens(lexer.THEN, lexer.NEWLINE, lexer.SEMICOLON, lexer.END, lexer.ELSE, lexer.EOF)
			if p.curTokenIs(lexer.RPAREN) {
				p.nextToken()
			}
		} else {
			for !p.curTokenIs(lexer.THEN) && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
				cond := p.parseExpression(LOWEST)
				if cond != nil {
					clause.Conditions = append(clause.Conditions, cond)
				}

				if p.peekTokenIs(lexer.COMMA) {
					p.nextToken()
					p.nextToken()
					continue
				}
				if p.curTokenIs(lexer.COMMA) {
					p.nextToken()
					continue
				}
				if p.peekTokenIs(lexer.THEN) || p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.END) || p.peekTokenIs(lexer.EOF) {
					break
				}
				p.nextToken()
			}
		}

		if p.peekTokenIs(lexer.THEN) {
			p.nextToken()
		}

		p.nextToken()
		p.skipCurNewlines()
		clause.Body = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.WHEN) && !p.curTokenIs(lexer.IN) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.SEMICOLON) {
				p.nextToken()
				continue
			}
			stmt := p.parseStatement()
			if stmt != nil {
				clause.Body.Statements = append(clause.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}

		exp.Clauses = append(exp.Clauses, clause)
	}

	if p.curTokenIs(lexer.ELSE) || p.peekTokenIs(lexer.ELSE) {
		if !p.curTokenIs(lexer.ELSE) {
			p.nextToken()
		}
		p.nextToken()
		p.skipCurNewlines()
		exp.Else = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.SEMICOLON) {
				p.nextToken()
				continue
			}
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Else.Statements = append(exp.Else.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
	}
	if !p.curTokenIs(lexer.END) {
		if hasPatternClause && p.curTokenIs(lexer.EOF) {
			return exp
		}
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseWhileExpression() ast.Expression {
	exp := &ast.WhileExpression{
		Token: p.curToken,
	}

	p.nextToken()
	previousStopAtDo := p.stopAtDo
	p.stopAtDo = true
	exp.Condition = p.parseExpression(LOWEST)
	p.stopAtDo = previousStopAtDo

	p.nextToken()
	if p.curTokenIs(lexer.LBRACE) || p.curTokenIs(lexer.DO) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
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
	previousStopAtDo := p.stopAtDo
	p.stopAtDo = true
	exp.Condition = p.parseExpression(LOWEST)
	p.stopAtDo = previousStopAtDo

	p.nextToken()
	if p.curTokenIs(lexer.LBRACE) || p.curTokenIs(lexer.DO) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
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
	exp.Variable = p.parseForTarget()
	if exp.Variable == nil {
		return nil
	}
	if !p.curTokenIs(lexer.IN) {
		p.parseError("expected in, got %s", p.curToken.Type)
		return nil
	}
	p.nextToken()

	exp.Collection = p.parseExpression(LOWEST)

	p.nextToken()
	p.skipCurNewlines()

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else if p.curTokenIs(lexer.DO) {
		p.nextToken()
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
	} else {
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

func (p *Parser) parseForTarget() *ast.Identifier {
	var target *ast.Identifier
	for !p.curTokenIs(lexer.IN) && !p.curTokenIs(lexer.EOF) {
		if target == nil {
			target = p.currentAssignmentName()
		}
		p.nextToken()
	}
	if target == nil {
		return &ast.Identifier{
			Token: lexer.Token{Type: lexer.IDENT, Literal: "_"},
			Value: "_",
		}
	}
	return target
}

func (p *Parser) parseDefExpression() ast.Expression {
	exp := &ast.DefExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if (p.curTokenIs(lexer.SELF) || p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.CONSTANT)) && p.peekTokenIs(lexer.DOT) {
		exp.Receiver = &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
		p.nextToken()
		p.nextToken()
	} else if (p.curTokenIs(lexer.AT) || p.curTokenIs(lexer.AT2) || p.curTokenIs(lexer.DOLLAR)) && p.peekTokenIs(lexer.DOT) {
		switch p.curToken.Type {
		case lexer.AT:
			exp.Receiver = p.parseInstanceVariable()
		case lexer.AT2:
			exp.Receiver = p.parseClassVariable()
		case lexer.DOLLAR:
			exp.Receiver = p.parseGlobalVariable()
		}
		p.nextToken()
		p.nextToken()
	} else if p.curTokenIs(lexer.LPAREN) {
		p.nextToken()
		exp.Receiver = p.parseExpression(LOWEST)
		if exp.Receiver == nil {
			return nil
		}
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
		if !p.expectPeek(lexer.DOT) {
			return nil
		}
		p.nextToken()
	}

	if !p.curTokenCanBeMethodName() {
		p.parseError("expected method name")
		return nil
	}

	exp.Name = p.parseDefMethodName()
	if exp.Name == nil {
		return nil
	}

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

	if !p.curTokenIs(lexer.ASSIGN) {
		p.nextToken()
	}

	p.skipCurNewlines()
	if p.curTokenIs(lexer.ASSIGN) {
		p.nextToken()
		p.skipCurNewlines()
		body := &ast.BlockExpression{Token: p.curToken}
		if expr := p.parseExpression(LOWEST); expr != nil {
			body.Statements = append(body.Statements, &ast.ExpressionStatement{
				Token:      p.curToken,
				Expression: expr,
			})
		}
		exp.Body = body
		return exp
	}

	body := &ast.BlockExpression{
		Token: p.curToken,
	}
	for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			body.Statements = append(body.Statements, stmt)
		}
		p.nextToken()
		p.skipCurNewlines()
	}
	exp.Body = p.parseImplicitBeginClauses(body)

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseImplicitBeginClauses(body *ast.BlockExpression) *ast.BlockExpression {
	if !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) {
		return body
	}

	begin := &ast.BeginExpression{
		Token: body.Token,
		Body:  body,
	}

	for p.curTokenIs(lexer.RESCUE) {
		rescue := &ast.RescueClause{Token: p.curToken}
		p.nextToken()
		hadSeparator := p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON)
		p.skipCurSeparators()

		if p.curTokenIs(lexer.ARROW) {
			p.nextToken()
			if p.curTokenIs(lexer.IDENT) {
				rescue.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
				p.nextToken()
			}
		} else if !hadSeparator && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.ELSE) {
			rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
			for p.curTokenIs(lexer.COMMA) {
				p.nextToken()
				p.nextToken()
				rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
			}
			if p.curTokenIs(lexer.ARROW) {
				p.nextToken()
				if p.curTokenIs(lexer.IDENT) {
					rescue.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
					p.nextToken()
				}
			}
		}

		p.skipCurSeparators()
		rescue.Body = &ast.BlockExpression{Token: p.curToken}
		for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				rescue.Body.Statements = append(rescue.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
		begin.Rescue = append(begin.Rescue, rescue)
	}

	if p.curTokenIs(lexer.ELSE) {
		p.nextToken()
		p.skipCurSeparators()
		begin.Else = &ast.BlockExpression{Token: p.curToken}
		for !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				begin.Else.Statements = append(begin.Else.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
	}

	if p.curTokenIs(lexer.ENSURE) {
		p.nextToken()
		p.skipCurSeparators()
		begin.Ensure = &ast.BlockExpression{Token: p.curToken}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				begin.Ensure.Statements = append(begin.Ensure.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
	}

	return &ast.BlockExpression{
		Token: body.Token,
		Statements: []ast.Statement{
			&ast.ExpressionStatement{Token: begin.Token, Expression: begin},
		},
	}
}

func (p *Parser) curTokenCanBeMethodName() bool {
	switch p.curToken.Type {
	case lexer.IDENT, lexer.CONSTANT, lexer.CLASS, lexer.EQUAL, lexer.EQUAL3, lexer.SPACESHIP, lexer.LESS_THAN, lexer.LESS_THAN_OR_EQUAL,
		lexer.GREATER_THAN, lexer.GREATER_THAN_OR_EQUAL, lexer.PLUS, lexer.MINUS,
		lexer.MULTIPLY, lexer.DIVIDE, lexer.MOD, lexer.BIT_AND, lexer.BIT_OR, lexer.BIT_XOR,
		lexer.MATCH, lexer.NOT_EQUAL, lexer.LBRACKET, lexer.TRUE, lexer.FALSE, lexer.NIL:
		return true
	default:
		return false
	}
}

func (p *Parser) parseDefMethodName() *ast.Identifier {
	name := &ast.Identifier{
		Token: p.curToken,
		Value: p.curToken.Literal,
	}

	if p.curTokenIs(lexer.LBRACKET) && p.peekTokenIs(lexer.RBRACKET) {
		p.nextToken() // consume ]
		name.Value = "[]"
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // consume =
			name.Value = "[]="
		}
	} else if p.curTokenIs(lexer.IDENT) && p.peekTokenIs(lexer.ASSIGN) && !p.hasSpaceBetween(p.curToken, p.peekToken) {
		p.nextToken()
		name.Value += "="
	}

	p.nextToken()
	return name
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
		if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.COMMA) {
			exp.RestParam = &ast.Identifier{
				Token: p.curToken,
				Value: "_",
			}
			return
		}
		p.nextToken()
		if p.curTokenIs(lexer.IDENT) {
			exp.RestParam = &ast.Identifier{
				Token: p.curToken,
				Value: p.curToken.Literal,
			}
		}
		return
	}
	if p.curTokenIs(lexer.POW) {
		if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.COMMA) {
			return
		}
		p.nextToken()
		return
	}
	if p.curTokenIs(lexer.BIT_AND) {
		if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.COMMA) {
			return
		}
		p.nextToken()
		if p.curTokenIs(lexer.IDENT) {
			exp.BlockParam = &ast.Identifier{
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
		if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken()
			p.nextToken()
			exp.ParamDefaults = append(exp.ParamDefaults, p.parseExpression(LOWEST))
		} else {
			exp.ParamDefaults = append(exp.ParamDefaults, nil)
		}
	}
}

func (p *Parser) parseClassExpression() ast.Expression {
	exp := &ast.ClassExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.LSHIFT) || (p.curTokenIs(lexer.LESS_THAN) && p.curToken.Literal == "<<") {
		exp.Name = &ast.Identifier{
			Token: p.curToken,
			Value: "__singleton_class__",
		}
		p.nextToken()
		_ = p.parseExpression(LOWEST)
		p.nextToken()
	} else {
		name := p.parseClassName()
		if name == nil {
			p.parseError("expected class name")
			return nil
		}
		exp.Name = name
	}

	if p.curTokenIs(lexer.LESS_THAN) {
		p.nextToken()
		superClass := p.parseExpression(LOWEST)
		switch sc := superClass.(type) {
		case *ast.Identifier:
			exp.SuperClass = sc
		case *ast.Constant:
			exp.SuperClass = &ast.Identifier{Token: sc.Token, Value: sc.Name}
		default:
			if superClass != nil {
				exp.SuperClass = &ast.Identifier{Token: p.curToken, Value: superClass.String()}
			}
		}
		p.nextToken()
	}

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		body := &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			// Skip semicolons
			if p.curTokenIs(lexer.SEMICOLON) {
				p.nextToken()
				continue
			}
			stmt := p.parseStatement()
			if stmt != nil {
				body.Statements = append(body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
		exp.Body = p.parseImplicitBeginClauses(body)
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseClassName() *ast.Identifier {
	if p.curTokenIs(lexer.COLON2) {
		p.nextToken()
	}

	if !p.curTokenCanStartClassName() {
		return nil
	}

	name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	for p.peekTokenIs(lexer.COLON2) {
		p.nextToken()
		p.nextToken()
		if !p.curTokenCanStartClassName() {
			return name
		}
		name = &ast.Identifier{Token: p.curToken, Value: name.Value + "::" + p.curToken.Literal}
	}
	p.nextToken()
	return name
}

func (p *Parser) curTokenCanStartClassName() bool {
	return p.curTokenIs(lexer.CONSTANT) || p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.NIL) || p.curTokenIs(lexer.SELF)
}

func (p *Parser) parseModuleExpression() ast.Expression {
	exp := &ast.ModuleExpression{
		Token: p.curToken,
	}

	p.nextToken()

	if p.curTokenIs(lexer.COLON2) {
		p.nextToken()
	}

	name := p.parseClassName()
	if name == nil {
		p.parseError("expected module name")
		return nil
	}
	exp.Name = name

	if p.curTokenIs(lexer.LBRACE) {
		exp.Body = p.parseBlockExpression()
	} else {
		p.skipCurNewlines()
		body := &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				body.Statements = append(body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurNewlines()
		}
		exp.Body = p.parseImplicitBeginClauses(body)
	}

	if !p.curTokenIs(lexer.END) && !p.expectPeek(lexer.END) {
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
	} else if !p.peekTokenIs(lexer.LBRACE) && !p.peekTokenIs(lexer.DO) {
		p.nextToken()
		for !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.DO) && !p.curTokenIs(lexer.EOF) {
			if p.curTokenIs(lexer.IDENT) {
				lit.Params = append(lit.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
			if p.peekTokenIs(lexer.COMMA) {
				p.nextToken()
				p.nextToken()
				continue
			}
			if p.peekTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.DO) {
				p.nextToken()
				break
			}
			p.nextToken()
		}
	}

	if p.curTokenIs(lexer.LBRACE) || p.peekTokenIs(lexer.LBRACE) {
		if p.peekTokenIs(lexer.LBRACE) {
			p.nextToken()
		}
		lit.Body = p.parseBlockExpression()
		if p.curTokenIs(lexer.RBRACE) && p.peekTokenIs(lexer.RBRACE) {
			p.nextToken()
		}
	} else if p.curTokenIs(lexer.DO) || p.peekTokenIs(lexer.DO) {
		if p.peekTokenIs(lexer.DO) {
			p.nextToken()
		}
		lit.Body = p.parseBlockExpression()
	}

	return lit
}

func (p *Parser) parseBlockExpression() *ast.BlockExpression {
	previousStopAtRParen := p.stopAtRParen
	p.stopAtRParen = false
	defer func() {
		p.stopAtRParen = previousStopAtRParen
	}()

	block := &ast.BlockExpression{
		Token: p.curToken,
	}

	p.nextToken()
	for p.curTokenIs(lexer.NEWLINE) {
		p.nextToken()
	}

	p.parseBlockParams(block)

	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.EOF) {
		for p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.END) || p.curTokenIs(lexer.RESCUE) || p.curTokenIs(lexer.ENSURE) || p.curTokenIs(lexer.EOF) {
			break
		}
		before := p.curToken
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		if p.curTokenIs(lexer.RBRACE) && (p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON)) && p.statementCanConsumeNestedBrace(stmt) {
			p.nextToken()
			p.skipCurSeparators()
			continue
		}
		if p.curTokenIs(lexer.RBRACE) && p.peekTokenIs(lexer.RBRACE) && p.statementCanConsumeNestedBrace(stmt) {
			p.nextToken()
			continue
		}
		if p.curTokenIs(lexer.END) && p.peekTokenIs(lexer.RBRACE) && p.statementCanConsumeNestedEnd(stmt) {
			p.nextToken()
			continue
		}
		if p.curTokenIs(lexer.END) && (p.peekTokenIs(lexer.NEWLINE) || p.peekTokenIs(lexer.SEMICOLON)) {
			p.nextToken()
			p.skipCurNewlines()
			continue
		}
		if p.curToken == before && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			p.nextToken()
		}
		if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.END) {
			break
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

	if p.curTokenIs(lexer.RESCUE) || p.curTokenIs(lexer.ENSURE) {
		body := &ast.BlockExpression{
			Token:      block.Token,
			Params:     block.Params,
			Statements: block.Statements,
		}
		begin := &ast.BeginExpression{
			Token: block.Token,
			Body:  body,
		}

		for p.curTokenIs(lexer.RESCUE) {
			rescue := &ast.RescueClause{Token: p.curToken}
			p.nextToken()
			hadSeparator := p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON)
			p.skipCurSeparators()

			if p.curTokenIs(lexer.ARROW) {
				p.nextToken()
				if p.curTokenIs(lexer.IDENT) {
					rescue.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
					p.nextToken()
				}
			} else if !hadSeparator && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.ELSE) {
				rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
				for p.curTokenIs(lexer.COMMA) {
					p.nextToken()
					p.nextToken()
					rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
				}
				if p.curTokenIs(lexer.ARROW) {
					p.nextToken()
					if p.curTokenIs(lexer.IDENT) {
						rescue.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
						p.nextToken()
					}
				}
			}

			p.skipCurSeparators()
			rescue.Body = &ast.BlockExpression{Token: p.curToken}
			for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
				stmt := p.parseStatement()
				if stmt != nil {
					rescue.Body.Statements = append(rescue.Body.Statements, stmt)
				}
				p.nextToken()
				p.skipCurSeparators()
			}
			begin.Rescue = append(begin.Rescue, rescue)
		}

		if p.curTokenIs(lexer.ENSURE) {
			begin.Ensure = &ast.BlockExpression{Token: p.curToken}
			p.nextToken()
			p.skipCurSeparators()
			begin.Ensure.Token = p.curToken
		}
		for begin.Ensure != nil && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				begin.Ensure.Statements = append(begin.Ensure.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
		block.Statements = []ast.Statement{&ast.ExpressionStatement{Token: begin.Token, Expression: begin}}
	}

	if p.curTokenIs(lexer.END) {
		return block
	}

	if p.curTokenIs(lexer.RBRACE) {
		return block
	}

	return block
}

func (p *Parser) statementCanConsumeNestedBrace(stmt ast.Statement) bool {
	exprStmt, ok := stmt.(*ast.ExpressionStatement)
	if !ok || exprStmt.Expression == nil {
		return false
	}
	switch exprStmt.Expression.(type) {
	case *ast.BreakExpression, *ast.NextExpression, *ast.ReturnExpression:
		return false
	default:
		return true
	}
}

func (p *Parser) statementCanConsumeNestedEnd(stmt ast.Statement) bool {
	exprStmt, ok := stmt.(*ast.ExpressionStatement)
	if !ok || exprStmt.Expression == nil {
		return false
	}
	switch exprStmt.Expression.(type) {
	case *ast.ClassExpression, *ast.ModuleExpression, *ast.DefExpression, *ast.IfExpression, *ast.BeginExpression:
		return true
	default:
		return false
	}
}

func statementIsBeginExpression(stmt ast.Statement) bool {
	exprStmt, ok := stmt.(*ast.ExpressionStatement)
	if !ok || exprStmt.Expression == nil {
		return false
	}
	_, ok = exprStmt.Expression.(*ast.BeginExpression)
	return ok
}

func (p *Parser) consumeBlockTerminator() {
	if p.curTokenIs(lexer.RBRACE) && p.peekTokenIs(lexer.RBRACE) {
		return
	}
	if p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.END) {
		p.nextToken()
	}
}

func (p *Parser) shouldConsumeBlockTerminator() bool {
	if p.peekTokenIs(lexer.DOT) {
		return false
	}
	return p.peekPrecedence() == LOWEST
}

func (p *Parser) parseBeginExpression() ast.Expression {
	exp := &ast.BeginExpression{
		Token: p.curToken,
	}

	p.nextToken()
	p.skipCurSeparators()

	exp.Body = &ast.BlockExpression{
		Token: p.curToken,
	}

	for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			exp.Body.Statements = append(exp.Body.Statements, stmt)
		}
		p.nextToken()
		p.skipCurSeparators()
	}

	for p.curTokenIs(lexer.RESCUE) || p.peekTokenIs(lexer.RESCUE) {
		if !p.curTokenIs(lexer.RESCUE) {
			p.nextToken()
		}
		rescue := &ast.RescueClause{
			Token: p.curToken,
		}

		p.nextToken()
		hadSeparator := p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON)
		p.skipCurSeparators()

		if p.curTokenIs(lexer.ARROW) {
			p.nextToken()
			if p.curTokenIs(lexer.IDENT) {
				rescue.Variable = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
				p.nextToken()
			}
		} else if !hadSeparator && !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.LBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.ELSE) {
			rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))

			for p.curTokenIs(lexer.COMMA) {
				p.nextToken()
				p.nextToken()
				rescue.Exceptions = append(rescue.Exceptions, p.parseExpression(LOWEST))
			}
		}

		if p.curTokenIs(lexer.RPAREN) {
			p.nextToken()
		}
		p.skipCurSeparators()
		rescue.Body = &ast.BlockExpression{
			Token: p.curToken,
		}

		for !p.curTokenIs(lexer.RESCUE) && !p.curTokenIs(lexer.ELSE) && !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				rescue.Body.Statements = append(rescue.Body.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}

		exp.Rescue = append(exp.Rescue, rescue)
	}

	if p.curTokenIs(lexer.ELSE) || p.peekTokenIs(lexer.ELSE) {
		if !p.curTokenIs(lexer.ELSE) {
			p.nextToken()
		}
		p.nextToken()
		p.skipCurSeparators()
		exp.Else = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.ENSURE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Else.Statements = append(exp.Else.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
	}

	if p.curTokenIs(lexer.ENSURE) || p.peekTokenIs(lexer.ENSURE) {
		if !p.curTokenIs(lexer.ENSURE) {
			p.nextToken()
		}
		p.nextToken()
		p.skipCurSeparators()
		exp.Ensure = &ast.BlockExpression{
			Token: p.curToken,
		}
		for !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
			stmt := p.parseStatement()
			if stmt != nil {
				exp.Ensure.Statements = append(exp.Ensure.Statements, stmt)
			}
			p.nextToken()
			p.skipCurSeparators()
		}
	}

	if !p.curTokenIs(lexer.END) {
		p.parseError("expected end, got %s", p.curToken.Type)
		return nil
	}

	return exp
}

func (p *Parser) parseDefinedExpression() ast.Expression {
	exp := &ast.DefinedExpression{
		Token: p.curToken,
	}

	if !p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()
		exp.Expression = p.parseExpression(LOWEST)
		return exp
	}
	p.nextToken()
	p.nextToken()
	if p.isBareDefinedControlFlowExpression() {
		prefix := p.prefixFns[p.curToken.Type]
		if prefix == nil {
			return nil
		}
		exp.Expression = prefix()
	} else {
		previousStopAtRParen := p.stopAtRParen
		p.stopAtRParen = true
		exp.Expression = p.parseExpression(LOWEST)
		p.stopAtRParen = previousStopAtRParen
	}

	if !p.consumeExpectedRParen() {
		return nil
	}

	return exp
}

func (p *Parser) isBareDefinedControlFlowExpression() bool {
	if !p.peekTokenIs(lexer.RPAREN) {
		return false
	}
	switch p.curToken.Type {
	case lexer.BREAK, lexer.NEXT, lexer.RETURN, lexer.YIELD, lexer.REDO, lexer.RETRY:
		return true
	default:
		return false
	}
}

func (p *Parser) parseAliasExpression() ast.Expression {
	exp := &ast.AliasExpression{
		Token: p.curToken,
	}

	p.nextToken()

	exp.New = p.parseAliasName()
	p.nextToken()
	exp.Old = p.parseAliasName()

	return exp
}

func (p *Parser) parseAliasName() ast.Expression {
	if p.curTokenIs(lexer.DOLLAR) {
		return p.parseGlobalVariable()
	}
	if p.curTokenIs(lexer.LBRACKET) && p.peekTokenIs(lexer.RBRACKET) {
		return p.parseDefMethodName()
	}
	if p.curTokenCanBeMethodName() || p.curTokenIs(lexer.SYMBOL) {
		return &ast.Identifier{
			Token: p.curToken,
			Value: p.curToken.Literal,
		}
	}
	p.parseError("expected alias method name, got %s", p.curToken.Type)
	return nil
}

func (p *Parser) parseUndefExpression() ast.Expression {
	exp := &ast.UndefExpression{
		Token: p.curToken,
	}

	p.nextToken()

	for !p.curTokenIs(lexer.NEWLINE) && !p.curTokenIs(lexer.SEMICOLON) && !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.END) && !p.curTokenIs(lexer.EOF) {
		if p.curTokenIs(lexer.IDENT) || p.curTokenIs(lexer.STRING) || p.curTokenIs(lexer.SYMBOL) {
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
	return p.parseMixinMethodCall("include")
}

func (p *Parser) parseExtendExpression() ast.Expression {
	return p.parseMixinMethodCall("extend")
}

func (p *Parser) parsePrependExpression() ast.Expression {
	return p.parseMixinMethodCall("prepend")
}

func (p *Parser) parseMixinMethodCall(name string) ast.Expression {
	call := &ast.MethodCall{
		Token: p.curToken,
		Method: &ast.Identifier{
			Token: p.curToken,
			Value: name,
		},
	}

	if p.peekTokenIs(lexer.LPAREN) {
		p.nextToken()
		return p.parseCallExpression(call.Method)
	}

	if p.peekTokenEndsStatement() {
		return call
	}

	p.nextToken()
	p.parseOneCallArg(call)

	for p.curTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.COMMA) {
		if !p.curTokenIs(lexer.COMMA) {
			p.nextToken()
		}
		p.skipPeekNewlines()
		if p.peekTokenEndsStatement() {
			break
		}
		p.nextToken()
		p.parseOneCallArg(call)
	}

	return call
}

func (p *Parser) peekTokenEndsStatement() bool {
	switch p.peekToken.Type {
	case lexer.NEWLINE, lexer.SEMICOLON, lexer.RBRACE, lexer.END, lexer.EOF:
		return true
	default:
		return false
	}
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

func (p *Parser) skipCurSeparators() {
	for p.curTokenIs(lexer.NEWLINE) || p.curTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}
}

func (p *Parser) parseRescueModifier(left ast.Expression) ast.Expression {
	begin := &ast.BeginExpression{
		Token: p.curToken,
		Body: &ast.BlockExpression{
			Token:      p.curToken,
			Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: left}},
		},
	}

	p.nextToken()
	rescueValue := p.parseExpression(LOWEST)

	begin.Rescue = []*ast.RescueClause{
		{
			Token: p.curToken,
			Body: &ast.BlockExpression{
				Token:      p.curToken,
				Statements: []ast.Statement{&ast.ExpressionStatement{Token: p.curToken, Expression: rescueValue}},
			},
		},
	}

	return begin
}
