package ast

import (
	"fmt"

	"github.com/GoLangDream/rgo/pkg/lexer"
)

type Node interface {
	TokenLiteral() string
	String() string
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	out := ""
	for _, s := range p.Statements {
		out += s.String()
	}
	return out
}

type Identifier struct {
	Token lexer.Token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

type Boolean struct {
	Token lexer.Token
	Value bool
}

func (b *Boolean) expressionNode()      {}
func (b *Boolean) TokenLiteral() string { return b.Token.Literal }
func (b *Boolean) String() string       { return b.Token.Literal }

type IntegerLiteral struct {
	Token lexer.Token
	Value int64
}

func (i *IntegerLiteral) expressionNode()      {}
func (i *IntegerLiteral) TokenLiteral() string { return i.Token.Literal }
func (i *IntegerLiteral) String() string       { return i.Token.Literal }

type FloatLiteral struct {
	Token lexer.Token
	Value float64
}

func (f *FloatLiteral) expressionNode()      {}
func (f *FloatLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FloatLiteral) String() string       { return f.Token.Literal }

type StringLiteral struct {
	Token lexer.Token
	Value string
}

func (s *StringLiteral) expressionNode()      {}
func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) String() string       { return s.Token.Literal }

type SymbolLiteral struct {
	Token lexer.Token
	Value string
}

func (s *SymbolLiteral) expressionNode()      {}
func (s *SymbolLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *SymbolLiteral) String() string       { return s.Token.Literal }

type RegexpLiteral struct {
	Token   lexer.Token
	Pattern string
	Options string
}

func (r *RegexpLiteral) expressionNode()      {}
func (r *RegexpLiteral) TokenLiteral() string { return r.Token.Literal }
func (r *RegexpLiteral) String() string       { return r.Token.Literal }

type ArrayLiteral struct {
	Token    lexer.Token
	Elements []Expression
}

func (a *ArrayLiteral) expressionNode()      {}
func (a *ArrayLiteral) TokenLiteral() string { return a.Token.Literal }
func (a *ArrayLiteral) String() string {
	out := "["
	for i, e := range a.Elements {
		out += e.String()
		if i < len(a.Elements)-1 {
			out += ", "
		}
	}
	out += "]"
	return out
}

type HashLiteral struct {
	Token lexer.Token
	Pairs map[Expression]Expression
	Order []Expression
}

func (h *HashLiteral) expressionNode()      {}
func (h *HashLiteral) TokenLiteral() string { return h.Token.Literal }
func (h *HashLiteral) String() string {
	out := "{"
	for i, k := range h.Order {
		out += k.String() + ": " + h.Pairs[k].String()
		if i < len(h.Order)-1 {
			out += ", "
		}
	}
	out += "}"
	return out
}

type IndexExpression struct {
	Token lexer.Token
	Left  Expression
	Index Expression
}

func (i *IndexExpression) expressionNode()      {}
func (i *IndexExpression) TokenLiteral() string { return i.Token.Literal }
func (i *IndexExpression) String() string {
	return fmt.Sprintf("(%s[%s])", i.Left.String(), i.Index.String())
}

type PrefixExpression struct {
	Token    lexer.Token
	Operator string
	Right    Expression
}

func (p *PrefixExpression) expressionNode()      {}
func (p *PrefixExpression) TokenLiteral() string { return p.Token.Literal }
func (p *PrefixExpression) String() string {
	return fmt.Sprintf("(%s%s)", p.Operator, p.Right.String())
}

type InfixExpression struct {
	Token    lexer.Token
	Left     Expression
	Operator string
	Right    Expression
}

func (i *InfixExpression) expressionNode()      {}
func (i *InfixExpression) TokenLiteral() string { return i.Token.Literal }
func (i *InfixExpression) String() string {
	return fmt.Sprintf("(%s %s %s)", i.Left.String(), i.Operator, i.Right.String())
}

type TernaryExpression struct {
	Token       lexer.Token
	Condition   Expression
	Consequent  Expression
	Alternative Expression
}

func (t *TernaryExpression) expressionNode()      {}
func (t *TernaryExpression) TokenLiteral() string { return t.Token.Literal }
func (t *TernaryExpression) String() string {
	return fmt.Sprintf("(%s ? %s : %s)", t.Condition.String(), t.Consequent.String(), t.Alternative.String())
}

type RangeExpression struct {
	Token     lexer.Token
	Left      Expression
	Right     Expression
	Exclusive bool
}

func (r *RangeExpression) expressionNode()      {}
func (r *RangeExpression) TokenLiteral() string { return r.Token.Literal }
func (r *RangeExpression) String() string {
	op := ".."
	if r.Exclusive {
		op = "..."
	}
	return fmt.Sprintf("%s %s %s", r.Left.String(), op, r.Right.String())
}

type BlockExpression struct {
	Token      lexer.Token
	Statements []Statement
}

func (b *BlockExpression) expressionNode()      {}
func (b *BlockExpression) TokenLiteral() string { return b.Token.Literal }
func (b *BlockExpression) String() string {
	out := "{\n"
	for _, s := range b.Statements {
		out += s.String() + "\n"
	}
	out += "}"
	return out
}

type IfExpression struct {
	Token       lexer.Token
	Condition   Expression
	Consequent  *BlockExpression
	Alternative *BlockExpression
	ElsIf       []*ElsIfExpression
}

type ElsIfExpression struct {
	Token      lexer.Token
	Condition  Expression
	Consequent *BlockExpression
}

func (i *IfExpression) expressionNode()      {}
func (i *IfExpression) TokenLiteral() string { return i.Token.Literal }
func (i *IfExpression) String() string {
	out := "if " + i.Condition.String() + "\n"
	out += i.Consequent.String()
	for _, elsif := range i.ElsIf {
		out += "elsif " + elsif.Condition.String() + "\n"
		out += elsif.Consequent.String()
	}
	if i.Alternative != nil {
		out += "else\n" + i.Alternative.String()
	}
	out += "end"
	return out
}

type CaseExpression struct {
	Token      lexer.Token
	Expression Expression
	Clauses    []*CaseClause
	Else       *BlockExpression
}

type CaseClause struct {
	Token      lexer.Token
	Conditions []Expression
	Body       *BlockExpression
}

func (c *CaseExpression) expressionNode()      {}
func (c *CaseExpression) TokenLiteral() string { return c.Token.Literal }
func (c *CaseExpression) String() string {
	out := "case"
	if c.Expression != nil {
		out += " " + c.Expression.String()
	}
	out += "\n"
	for _, clause := range c.Clauses {
		for _, cond := range clause.Conditions {
			out += "when " + cond.String() + "\n"
		}
		out += clause.Body.String() + "\n"
	}
	if c.Else != nil {
		out += "else\n" + c.Else.String() + "\n"
	}
	out += "end"
	return out
}

type WhileExpression struct {
	Token     lexer.Token
	Condition Expression
	Body      *BlockExpression
}

func (w *WhileExpression) expressionNode()      {}
func (w *WhileExpression) TokenLiteral() string { return w.Token.Literal }
func (w *WhileExpression) String() string {
	return "while " + w.Condition.String() + "\n" + w.Body.String() + "\nend"
}

type UntilExpression struct {
	Token     lexer.Token
	Condition Expression
	Body      *BlockExpression
}

func (u *UntilExpression) expressionNode()      {}
func (u *UntilExpression) TokenLiteral() string { return u.Token.Literal }
func (u *UntilExpression) String() string {
	return "until " + u.Condition.String() + "\n" + u.Body.String() + "\nend"
}

type ForExpression struct {
	Token      lexer.Token
	Variable   *Identifier
	Collection Expression
	Body       *BlockExpression
}

func (f *ForExpression) expressionNode()      {}
func (f *ForExpression) TokenLiteral() string { return f.Token.Literal }
func (f *ForExpression) String() string {
	return "for " + f.Variable.String() + " in " + f.Collection.String() + "\n" + f.Body.String() + "\nend"
}

type DefExpression struct {
	Token    lexer.Token
	Name     *Identifier
	Params   []*Identifier
	Body     *BlockExpression
	Receiver Expression
}

func (d *DefExpression) expressionNode()      {}
func (d *DefExpression) TokenLiteral() string { return d.Token.Literal }
func (d *DefExpression) String() string {
	out := "def "
	if d.Receiver != nil {
		out += d.Receiver.String() + "."
	}
	out += d.Name.String()
	out += "("
	for i, p := range d.Params {
		out += p.String()
		if i < len(d.Params)-1 {
			out += ", "
		}
	}
	out += ")\n"
	out += d.Body.String()
	out += "\nend"
	return out
}

type ClassExpression struct {
	Token      lexer.Token
	Name       *Identifier
	SuperClass *Identifier
	Body       *BlockExpression
}

func (c *ClassExpression) expressionNode()      {}
func (c *ClassExpression) TokenLiteral() string { return c.Token.Literal }
func (c *ClassExpression) String() string {
	out := "class " + c.Name.String()
	if c.SuperClass != nil {
		out += " < " + c.SuperClass.String()
	}
	out += "\n"
	out += c.Body.String()
	out += "\nend"
	return out
}

type ModuleExpression struct {
	Token lexer.Token
	Name  *Identifier
	Body  *BlockExpression
}

func (m *ModuleExpression) expressionNode()      {}
func (m *ModuleExpression) TokenLiteral() string { return m.Token.Literal }
func (m *ModuleExpression) String() string {
	out := "module " + m.Name.String() + "\n"
	out += m.Body.String()
	out += "\nend"
	return out
}

type ReturnExpression struct {
	Token       lexer.Token
	ReturnValue Expression
}

func (r *ReturnExpression) statementNode()       {}
func (r *ReturnExpression) expressionNode()      {}
func (r *ReturnExpression) TokenLiteral() string { return r.Token.Literal }
func (r *ReturnExpression) String() string {
	if r.ReturnValue != nil {
		return "return " + r.ReturnValue.String()
	}
	return "return"
}

type BreakExpression struct {
	Token lexer.Token
	Value Expression
}

func (b *BreakExpression) statementNode()       {}
func (b *BreakExpression) expressionNode()      {}
func (b *BreakExpression) TokenLiteral() string { return b.Token.Literal }
func (b *BreakExpression) String() string {
	if b.Value != nil {
		return "break " + b.Value.String()
	}
	return "break"
}

type NextExpression struct {
	Token lexer.Token
	Value Expression
}

func (n *NextExpression) statementNode()       {}
func (n *NextExpression) expressionNode()      {}
func (n *NextExpression) TokenLiteral() string { return n.Token.Literal }
func (n *NextExpression) String() string {
	if n.Value != nil {
		return "next " + n.Value.String()
	}
	return "next"
}

type RedoExpression struct {
	Token lexer.Token
}

func (r *RedoExpression) statementNode()       {}
func (r *RedoExpression) expressionNode()      {}
func (r *RedoExpression) TokenLiteral() string { return r.Token.Literal }
func (r *RedoExpression) String() string       { return "redo" }

type RetryExpression struct {
	Token lexer.Token
}

func (r *RetryExpression) statementNode()       {}
func (r *RetryExpression) expressionNode()      {}
func (r *RetryExpression) TokenLiteral() string { return r.Token.Literal }
func (r *RetryExpression) String() string       { return "retry" }

type YieldExpression struct {
	Token lexer.Token
	Args  []Expression
}

func (y *YieldExpression) expressionNode()      {}
func (y *YieldExpression) TokenLiteral() string { return y.Token.Literal }
func (y *YieldExpression) String() string {
	out := "yield"
	if len(y.Args) > 0 {
		out += "("
		for i, arg := range y.Args {
			out += arg.String()
			if i < len(y.Args)-1 {
				out += ", "
			}
		}
		out += ")"
	}
	return out
}

type SuperExpression struct {
	Token lexer.Token
	Args  []Expression
}

func (s *SuperExpression) expressionNode()      {}
func (s *SuperExpression) TokenLiteral() string { return s.Token.Literal }
func (s *SuperExpression) String() string {
	out := "super"
	if len(s.Args) > 0 {
		out += "("
		for i, arg := range s.Args {
			out += arg.String()
			if i < len(s.Args)-1 {
				out += ", "
			}
		}
		out += ")"
	}
	return out
}

type SelfExpression struct {
	Token lexer.Token
}

func (s *SelfExpression) expressionNode()      {}
func (s *SelfExpression) TokenLiteral() string { return s.Token.Literal }
func (s *SelfExpression) String() string       { return "self" }

type NilExpression struct {
	Token lexer.Token
}

func (n *NilExpression) expressionNode()      {}
func (n *NilExpression) TokenLiteral() string { return n.Token.Literal }
func (n *NilExpression) String() string       { return "nil" }

type InstanceVariable struct {
	Token lexer.Token
	Name  string
}

func (i *InstanceVariable) expressionNode()      {}
func (i *InstanceVariable) TokenLiteral() string { return i.Token.Literal }
func (i *InstanceVariable) String() string       { return i.Name }

type ClassVariable struct {
	Token lexer.Token
	Name  string
}

func (c *ClassVariable) expressionNode()      {}
func (c *ClassVariable) TokenLiteral() string { return c.Token.Literal }
func (c *ClassVariable) String() string       { return c.Name }

type GlobalVariable struct {
	Token lexer.Token
	Name  string
}

func (g *GlobalVariable) expressionNode()      {}
func (g *GlobalVariable) TokenLiteral() string { return g.Token.Literal }
func (g *GlobalVariable) String() string       { return g.Name }

type Constant struct {
	Token lexer.Token
	Name  string
}

func (c *Constant) expressionNode()      {}
func (c *Constant) TokenLiteral() string { return c.Token.Literal }
func (c *Constant) String() string       { return c.Name }

type ConstantResolution struct {
	Token lexer.Token
	Left  Expression
	Name  *Identifier
}

func (c *ConstantResolution) expressionNode()      {}
func (c *ConstantResolution) TokenLiteral() string { return c.Token.Literal }
func (c *ConstantResolution) String() string {
	if c.Left != nil {
		return c.Left.String() + "::" + c.Name.String()
	}
	return c.Name.String()
}

type AssignExpression struct {
	Token lexer.Token
	Name  *Identifier
	Value Expression
}

func (a *AssignExpression) expressionNode()      {}
func (a *AssignExpression) TokenLiteral() string { return a.Token.Literal }
func (a *AssignExpression) String() string       { return a.Name.String() + " = " + a.Value.String() }

type InstanceVarAssign struct {
	Token lexer.Token
	Name  string
	Value Expression
}

func (i *InstanceVarAssign) expressionNode()      {}
func (i *InstanceVarAssign) TokenLiteral() string { return i.Token.Literal }
func (i *InstanceVarAssign) String() string       { return i.Name + " = " + i.Value.String() }

type ClassVarAssign struct {
	Token lexer.Token
	Name  string
	Value Expression
}

func (c *ClassVarAssign) expressionNode()      {}
func (c *ClassVarAssign) TokenLiteral() string { return c.Token.Literal }
func (c *ClassVarAssign) String() string       { return c.Name + " = " + c.Value.String() }

type GlobalVarAssign struct {
	Token lexer.Token
	Name  string
	Value Expression
}

func (g *GlobalVarAssign) expressionNode()      {}
func (g *GlobalVarAssign) TokenLiteral() string { return g.Token.Literal }
func (g *GlobalVarAssign) String() string       { return g.Name + " = " + g.Value.String() }

type MethodCall struct {
	Token    lexer.Token
	Receiver Expression
	Method   *Identifier
	Args     []Expression
	Block    *BlockExpression
}

func (m *MethodCall) expressionNode()      {}
func (m *MethodCall) TokenLiteral() string { return m.Token.Literal }
func (m *MethodCall) String() string {
	out := ""
	if m.Receiver != nil {
		out += m.Receiver.String() + "."
	}
	out += m.Method.String()
	out += "("
	for i, arg := range m.Args {
		out += arg.String()
		if i < len(m.Args)-1 {
			out += ", "
		}
	}
	out += ")"
	if m.Block != nil {
		out += " " + m.Block.String()
	}
	return out
}

type UndefExpression struct {
	Token   lexer.Token
	Methods []*Identifier
}

func (u *UndefExpression) expressionNode()      {}
func (u *UndefExpression) TokenLiteral() string { return u.Token.Literal }
func (u *UndefExpression) String() string {
	out := "undef "
	for i, method := range u.Methods {
		out += method.String()
		if i < len(u.Methods)-1 {
			out += ", "
		}
	}
	return out
}

type AliasExpression struct {
	Token lexer.Token
	Old   Expression
	New   Expression
}

func (a *AliasExpression) expressionNode()      {}
func (a *AliasExpression) TokenLiteral() string { return a.Token.Literal }
func (a *AliasExpression) String() string       { return "alias " + a.New.String() + " " + a.Old.String() }

type BeginExpression struct {
	Token  lexer.Token
	Body   *BlockExpression
	Rescue []*RescueClause
	Else   *BlockExpression
	Ensure *BlockExpression
}

type RescueClause struct {
	Token      lexer.Token
	Exceptions []Expression
	Variable   *Identifier
	Body       *BlockExpression
}

func (b *BeginExpression) expressionNode()      {}
func (b *BeginExpression) TokenLiteral() string { return b.Token.Literal }
func (b *BeginExpression) String() string {
	out := "begin\n"
	out += b.Body.String()
	for _, rescue := range b.Rescue {
		out += "rescue\n"
		out += rescue.Body.String()
	}
	if b.Else != nil {
		out += "else\n" + b.Else.String()
	}
	if b.Ensure != nil {
		out += "ensure\n" + b.Ensure.String()
	}
	out += "end"
	return out
}

type RaiseExpression struct {
	Token lexer.Token
	Error Expression
}

func (r *RaiseExpression) statementNode()       {}
func (r *RaiseExpression) TokenLiteral() string { return r.Token.Literal }
func (r *RaiseExpression) String() string {
	if r.Error != nil {
		return "raise " + r.Error.String()
	}
	return "raise"
}

type ExpressionStatement struct {
	Token      lexer.Token
	Expression Expression
}

func (e *ExpressionStatement) statementNode()       {}
func (e *ExpressionStatement) TokenLiteral() string { return e.Token.Literal }
func (e *ExpressionStatement) String() string       { return e.Expression.String() }

type IncludeExpression struct {
	Token  lexer.Token
	Module Expression
}

func (i *IncludeExpression) expressionNode()      {}
func (i *IncludeExpression) TokenLiteral() string { return i.Token.Literal }
func (i *IncludeExpression) String() string       { return "include " + i.Module.String() }

type ExtendExpression struct {
	Token  lexer.Token
	Module Expression
}

func (e *ExtendExpression) expressionNode()      {}
func (e *ExtendExpression) TokenLiteral() string { return e.Token.Literal }
func (e *ExtendExpression) String() string       { return "extend " + e.Module.String() }

type PrependExpression struct {
	Token  lexer.Token
	Module Expression
}

func (p *PrependExpression) expressionNode()      {}
func (p *PrependExpression) TokenLiteral() string { return p.Token.Literal }
func (p *PrependExpression) String() string       { return "prepend " + p.Module.String() }

type DefinedExpression struct {
	Token      lexer.Token
	Expression Expression
}

func (d *DefinedExpression) expressionNode()      {}
func (d *DefinedExpression) TokenLiteral() string { return d.Token.Literal }
func (d *DefinedExpression) String() string       { return "defined?(" + d.Expression.String() + ")" }

type ProcLiteral struct {
	Token  lexer.Token
	Params []*Identifier
	Body   *BlockExpression
}

func (p *ProcLiteral) expressionNode()      {}
func (p *ProcLiteral) TokenLiteral() string { return p.Token.Literal }
func (p *ProcLiteral) String() string       { return "-> " + p.Body.String() }
