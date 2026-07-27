package compiler

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

func negateNumericLiteralCallReceiver(call *ast.MethodCall) (*ast.MethodCall, bool) {
	if call == nil || call.Receiver == nil {
		return nil, false
	}
	copyCall := *call
	switch receiver := call.Receiver.(type) {
	case *ast.IntegerLiteral:
		copyReceiver := *receiver
		copyReceiver.Value = -copyReceiver.Value
		copyCall.Receiver = &copyReceiver
		return &copyCall, true
	case *ast.FloatLiteral:
		copyReceiver := *receiver
		copyReceiver.Value = -copyReceiver.Value
		copyCall.Receiver = &copyReceiver
		return &copyCall, true
	case *ast.MethodCall:
		negativeReceiver, ok := negateNumericLiteralCallReceiver(receiver)
		if !ok {
			return nil, false
		}
		copyCall.Receiver = negativeReceiver
		return &copyCall, true
	default:
		return nil, false
	}
}

const (
	ScopeGlobal    = "global"
	ScopeLocal     = "local"
	ScopeBuiltin   = "builtin"
	ScopeFree      = "free"
	ScopeOuter     = "outer"
	ScopeOuterFree = "outer_free"
)

type Symbol struct {
	Name       string
	Index      int
	Scope      string
	ScopeIndex int
}

var builtinVariables = []string{
	"puts", "print", "p", "gets", "chomp",
	"to_s", "to_i", "to_f", "to_a", "to_h",
	"length", "size", "first", "last", "push",
	"pop", "shift", "unshift", "each", "map",
	"select", "reject", "reduce", "inject", "find",
	"detect", "find_all", "compact", "flatten", "join",
	"split", "reverse", "sort", "sort_by", "max", "min",
	"abs", "ceil", "floor", "round", "chr", "ord",
	"upcase", "downcase", "capitalize", "strip", "lstrip", "rstrip",
}

type SymbolTable struct {
	Outer          *SymbolTable
	store          map[string]Symbol
	FreeSymbols    []Symbol
	MaxSymbols     int
	MethodBoundary bool
}

func NewSymbolTable() *SymbolTable {
	s := &SymbolTable{
		store: make(map[string]Symbol),
	}
	return s
}

func NewEnclosedSymbolTable(outer *SymbolTable) *SymbolTable {
	s := NewSymbolTable()
	s.Outer = outer
	return s
}

func (s *SymbolTable) Define(name string) Symbol {
	symbol := Symbol{Name: name, Index: s.MaxSymbols, Scope: ScopeLocal}
	s.store[name] = symbol
	s.MaxSymbols++
	return symbol
}

func (s *SymbolTable) DefineParameter(name string) Symbol {
	if name == "_" {
		if existing, ok := s.store[name]; ok && existing.Scope == ScopeLocal {
			symbol := Symbol{Name: name, Index: s.MaxSymbols, Scope: ScopeLocal}
			s.MaxSymbols++
			return symbol
		}
	}
	return s.Define(name)
}

func (s *SymbolTable) DefineBuiltin(index int, name string) Symbol {
	symbol := Symbol{Name: name, Index: index, Scope: ScopeBuiltin}
	s.store[name] = symbol
	return symbol
}

func (s *SymbolTable) DefineGlobal(name string) Symbol {
	symbol := Symbol{Name: name, Index: s.MaxSymbols, Scope: ScopeGlobal}
	s.store[name] = symbol
	s.MaxSymbols++
	return symbol
}

func (s *SymbolTable) DefineFree(original Symbol) Symbol {
	s.FreeSymbols = append(s.FreeSymbols, original)

	symbol := Symbol{
		Name:       original.Name,
		Index:      len(s.FreeSymbols) - 1,
		Scope:      ScopeFree,
		ScopeIndex: original.Index,
	}

	s.store[original.Name] = symbol

	return symbol
}

func (s *SymbolTable) Resolve(name string) (Symbol, bool) {
	obj, ok := s.store[name]
	if !ok && s.MethodBoundary {
		for outer := s.Outer; outer != nil; outer = outer.Outer {
			if outerObj, found := outer.store[name]; found && (outerObj.Scope == ScopeGlobal || outerObj.Scope == ScopeBuiltin) {
				return outerObj, true
			}
		}
		return Symbol{}, false
	}
	if !ok && s.Outer != nil {
		if outerObj, found := s.Outer.store[name]; found && outerObj.Scope == ScopeLocal {
			free := s.DefineFree(outerObj)
			return free, true
		}
		if outerObj, found := s.Outer.store[name]; found && outerObj.Scope == ScopeFree {
			free := s.DefineFree(outerObj)
			return free, true
		}
	}
	if !ok && s.Outer != nil {
		obj, ok = s.Outer.Resolve(name)
		if !ok {
			return obj, ok
		}

		if obj.Scope == ScopeGlobal || obj.Scope == ScopeBuiltin {
			return obj, ok
		}

		free := s.DefineFree(obj)

		return free, true
	}

	return obj, ok
}

type EmittedInstruction struct {
	Opcode   Opcode
	Position int
}

type CompilationScope struct {
	instructions        Instructions
	lineMap             map[int]int
	lastInstruction     EmittedInstruction
	previousInstruction EmittedInstruction
	breakTarget         int
	nextPatchPos        []int
	nextPatchDepth      int
	nextPatchTarget     int
	redoTarget          int
	breakValuePatchPos  []int
	retryTarget         int
}

type Compiler struct {
	constants          []*object.EmeraldValue
	scopes             []CompilationScope
	scopeIndex         int
	symbolTable        *SymbolTable
	methodDepth        int
	currentLine        int
	forEachDepth       int
	implicitIt         map[*SymbolTable]bool
	tempCounter        int
	voidContext        bool
	evalTopLevelReturn bool
	sourceEncoding     string
}

func New() *Compiler {
	mainScope := CompilationScope{
		instructions:    Instructions{},
		lineMap:         map[int]int{},
		breakTarget:     -1,
		nextPatchTarget: -1,
		redoTarget:      -1,
		retryTarget:     -1,
	}

	symbolTable := NewSymbolTable()

	for i, v := range builtinVariables {
		symbolTable.DefineBuiltin(i, v)
	}

	return &Compiler{
		constants:      []*object.EmeraldValue{},
		scopes:         []CompilationScope{mainScope},
		symbolTable:    symbolTable,
		implicitIt:     map[*SymbolTable]bool{},
		sourceEncoding: core.CurrentEvalSourceEncoding,
	}
}

func NewWithSourceEncoding(encoding string) *Compiler {
	c := New()
	c.sourceEncoding = encoding
	return c
}

func NewWithLocalNames(localNames []string) *Compiler {
	c := New()
	seen := map[string]struct{}{}
	for _, name := range localNames {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		c.symbolTable.Define(name)
	}
	return c
}

func (c *Compiler) SetEvalTopLevelReturn(enabled bool) {
	c.evalTopLevelReturn = enabled
}

func compileNodeLine(node interface{}) int {
	switch n := node.(type) {
	case *ast.ExpressionStatement:
		return compileNodeLine(n.Expression)
	case *ast.IntegerLiteral:
		return n.Token.Line
	case *ast.FloatLiteral:
		return n.Token.Line
	case *ast.StringLiteral:
		return n.Token.Line
	case *ast.StringConcatExpression:
		return n.Token.Line
	case *ast.SymbolLiteral:
		return n.Token.Line
	case *ast.Identifier:
		return n.Token.Line
	case *ast.Constant:
		return n.Token.Line
	case *ast.ConstantResolution:
		return n.Token.Line
	case *ast.InstanceVariable:
		return n.Token.Line
	case *ast.ClassVariable:
		return n.Token.Line
	case *ast.GlobalVariable:
		return n.Token.Line
	case *ast.AssignExpression:
		return n.Token.Line
	case *ast.MultiAssignExpression:
		return n.Token.Line
	case *ast.InfixExpression:
		return n.Token.Line
	case *ast.PrefixExpression:
		return n.Token.Line
	case *ast.Boolean:
		return n.Token.Line
	case *ast.NilExpression:
		return n.Token.Line
	case *ast.IfExpression:
		return n.Token.Line
	case *ast.WhileExpression:
		return n.Token.Line
	case *ast.UntilExpression:
		return n.Token.Line
	case *ast.ForExpression:
		return n.Token.Line
	case *ast.MethodCall:
		return n.Token.Line
	case *ast.DefExpression:
		return n.Token.Line
	case *ast.ClassExpression:
		return n.Token.Line
	case *ast.ModuleExpression:
		return n.Token.Line
	case *ast.BlockExpression:
		return n.Token.Line
	case *ast.ProcLiteral:
		return n.Token.Line
	case *ast.ArrayLiteral:
		return n.Token.Line
	case *ast.HashLiteral:
		return n.Token.Line
	case *ast.IndexExpression:
		return n.Token.Line
	case *ast.ReturnExpression:
		return n.Token.Line
	case *ast.BreakExpression:
		return n.Token.Line
	case *ast.NextExpression:
		return n.Token.Line
	case *ast.RaiseExpression:
		return n.Token.Line
	case *ast.BeginExpression:
		return n.Token.Line
	case *ast.DefinedExpression:
		return n.Token.Line
	case *ast.SelfExpression:
		return n.Token.Line
	}
	return 0
}

func (c *Compiler) globalSymbolIndex(name string) int {
	root := c.symbolTable
	for root.Outer != nil {
		root = root.Outer
	}
	if sym, ok := root.Resolve(name); ok && sym.Scope == ScopeGlobal {
		return sym.Index
	}
	sym := root.DefineGlobal(name)
	return sym.Index
}

func (c *Compiler) Compile(node interface{}) error {
	prevLine := c.currentLine
	if line := compileNodeLine(node); line > 0 {
		c.currentLine = line
	}
	defer func() {
		c.currentLine = prevLine
	}()

	switch node := node.(type) {
	case *ast.Program:
		for i, s := range node.Statements {
			previousVoidContext := c.voidContext
			if expressionStatement, ok := s.(*ast.ExpressionStatement); ok && i < len(node.Statements)-1 {
				_, c.voidContext = expressionStatement.Expression.(*ast.DefinedExpression)
			}
			if err := c.Compile(s); err != nil {
				c.voidContext = previousVoidContext
				return err
			}
			c.voidContext = previousVoidContext
		}
	case *ast.ExpressionStatement:
		if err := c.Compile(node.Expression); err != nil {
			return err
		}
		c.Emit(OpPop)
	case *ast.IntegerLiteral:
		value := &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  node.Value,
			Class: core.R.Classes["Integer"],
		}
		c.EmitConstant(core.RememberULEBPackIntegerLiteral(value, node.Token.Literal))
	case *ast.RangeExpression:
		if err := c.compileRangeExpression(node); err != nil {
			return err
		}
	case *ast.TernaryExpression:
		if err := c.compileCondition(node.Condition); err != nil {
			return err
		}
		jumpNotTruthyPos := c.emit(OpJumpNotTruthy, 9999)
		if err := c.Compile(node.Consequent); err != nil {
			return err
		}
		jumpPos := c.emit(OpJump, 9999)
		afterConsequent := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterConsequent)
		if err := c.Compile(node.Alternative); err != nil {
			return err
		}
		afterAlternative := len(c.currentInstructions())
		c.changeOperand(jumpPos, afterAlternative)
	case *ast.FloatLiteral:
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  node.Value,
			Class: core.R.Classes["Float"],
		})
	case *ast.RationalLiteral:
		numerator, denominator, ok := rationalLiteralParts(node.Value)
		if !ok {
			return fmt.Errorf("invalid rational literal %q", node.Value)
		}
		c.EmitConstant(core.NewIntegerFromBigInt(numerator))
		c.EmitConstant(core.NewIntegerFromBigInt(denominator))
		c.emit(OpRationalNew)
	case *ast.ImaginaryLiteral:
		if node.Numeric == nil {
			return fmt.Errorf("invalid imaginary literal %q", node.Token.Literal)
		}
		if err := c.Compile(node.Numeric); err != nil {
			return err
		}
		c.emit(OpSend, c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: "i", Class: core.R.Classes["String"]}), 0, 0, 255)
	case *ast.StringLiteral:
		if node.Command {
			c.Emit(OpSelf)
		}
		if err := c.compileStringLiteralValue(node); err != nil {
			return err
		}
		if node.Command {
			if !node.Interpolates || !strings.Contains(node.Value, "#{") {
				c.emit(OpFreeze)
			}
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "`",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
		}
	case *ast.StringConcatExpression:
		for i, part := range node.Parts {
			if err := c.compileStringLiteralValue(part); err != nil {
				return err
			}
			if i > 0 {
				c.Emit(OpAdd)
			}
		}
	case *ast.SymbolLiteral:
		val := normalizedStaticSymbolName(node.Value)
		if node.Token.AllowsInterpolation && stringContainsInterpolation(val) {
			if err := c.compileStringInterpolationAtLine(val, node.Token.Line); err != nil {
				return err
			}
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "to_sym",
				Class: core.R.Classes["String"],
			}), 0, 0, 255)
			return nil
		}
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  val,
			Class: core.R.Classes["Symbol"],
		})
	case *ast.RegexpLiteral:
		if node.Interpolates && strings.Contains(node.Pattern, "#{") {
			cacheIndex := -1
			cacheJump := -1
			if strings.Contains(node.Options, "o") {
				cacheIndex = c.addConstant(&object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: core.R.Classes["Array"]})
				cacheJump = c.emit(OpRegexpOnceGet, cacheIndex, 9999)
			}
			c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "Regexp",
				Class: core.R.Classes["String"],
			}))
			if err := c.compileStringInterpolationPreservingEncodingAtLine(node.Pattern, node.Token.Line); err != nil {
				return err
			}
			c.EmitConstant(&object.EmeraldValue{Type: object.ValueSymbol, Data: "__rgo_regexp_options:" + node.Options, Class: core.R.Classes["Symbol"]})
			argCount := 2
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "new",
				Class: core.R.Classes["String"],
			}), 0, argCount, 255)
			if cacheIndex >= 0 {
				c.emit(OpRegexpOnceSet, cacheIndex)
				c.changeOperandAt(cacheJump, 1, len(c.currentInstructions()))
			}
			return nil
		}
		c.EmitConstant(&object.EmeraldValue{
			Type: object.ValueRegexp,
			Data: &object.RRegexp{
				Pattern: strings.ReplaceAll(node.Pattern, lexer.EscapedHashInterpolation, "#"),
				Options: node.Options,
			},
			Class:   core.R.Classes["Regexp"],
			Frozen:  true,
			Literal: true,
		})
	case *ast.Boolean:
		if node.Value {
			c.Emit(OpTrue)
		} else {
			c.Emit(OpFalse)
		}
	case *ast.NilExpression:
		c.Emit(OpNil)
	case *ast.DefinedExpression:
		c.compileDefinedExpression(node)
	case *ast.Identifier:
		if node.Value == "self" {
			c.Emit(OpSelf)
			return nil
		}
		if node.Value == "__FILE__" {
			c.EmitConstant(&object.EmeraldValue{
				Type:     object.ValueString,
				Data:     core.CurrentSpecFile,
				Class:    core.R.Classes["String"],
				Encoding: c.sourceEncoding,
			})
			return nil
		}
		if node.Value == "__dir__" {
			path := core.CurrentSpecFile
			if core.CurrentSpecFileAbsolute != "" {
				path = core.CurrentSpecFileAbsolute
			}
			if abs, err := filepath.Abs(path); err == nil {
				path = abs
			}
			c.EmitConstant(&object.EmeraldValue{
				Type:     object.ValueString,
				Data:     filepath.Dir(path),
				Class:    core.R.Classes["String"],
				Encoding: c.sourceEncoding,
			})
			return nil
		}
		if node.Value == "__LINE__" {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(node.Token.Line),
				Class: core.R.Classes["Integer"],
			})
			return nil
		}
		if node.Value == "block_given?" {
			c.Emit(OpBlockGiven)
			return nil
		}
		sym, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			c.Emit(OpSelf)
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Value,
				Class: core.R.Classes["String"],
			}), 3, 0, 255)
			return nil
		}
		switch sym.Scope {
		case ScopeGlobal:
			c.emit(OpGetGlobal, sym.Index)
		case ScopeLocal:
			c.emit(OpGetLocal, sym.Index)
		case ScopeBuiltin:
			c.Emit(OpSelf)
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Value,
				Class: core.R.Classes["String"],
			}), 0, 0, 255)
		case ScopeFree:
			c.emit(OpGetFree, sym.Index)
		case ScopeOuter:
			c.emit(OpGetOuter, sym.ScopeIndex)
		case ScopeOuterFree:
			c.emit(OpGetOuterFree, sym.ScopeIndex)
		}
	case *ast.Constant:
		c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name,
			Class: core.R.Classes["String"],
		}))
	case *ast.ConstantResolution:
		if left, ok := node.Left.(*ast.Constant); ok && left.Name == "Float" && node.Name.Value == "INFINITY" {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueFloat,
				Data:  math.Inf(1),
				Class: core.R.Classes["Float"],
			})
			return nil
		}
		if node.Left != nil {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			if !isConstantName(node.Name.Value) {
				c.emit(OpSend, c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  node.Name.Value,
					Class: core.R.Classes["String"],
				}), 0, 0, 255)
				return nil
			}
			c.emit(OpGetScopedConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			return nil
		}
		if node.Token.Type == lexer.COLON2 {
			c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "::" + node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			return nil
		}
		c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.String(),
			Class: core.R.Classes["String"],
		}))
	case *ast.InstanceVariable:
		c.emit(OpGetInstanceVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name,
			Class: core.R.Classes["String"],
		}))
	case *ast.GlobalVariable:
		c.emit(OpGetGlobal, c.globalSymbolIndex(node.Name))
	case *ast.ClassVariable:
		c.emit(OpGetClassVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name,
			Class: core.R.Classes["String"],
		}))
	case *ast.InfixExpression:
		// Short-circuit operators need special handling
		if node.Operator == "&&" || node.Operator == "and" {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			c.emit(OpDup)
			jumpPos := c.emit(OpJumpNotTruthy, 9999)
			c.emit(OpPop)
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			return nil
		}
		if node.Operator == "||" || node.Operator == "or" {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			c.emit(OpDup)
			jumpPos := c.emit(OpJumpTruthy, 9999)
			c.emit(OpPop)
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			return nil
		}

		namedCaptures := c.prepareNamedRegexpCaptureBindings(node)
		if err := c.Compile(node.Left); err != nil {
			return err
		}
		if err := c.Compile(node.Right); err != nil {
			return err
		}

		switch node.Operator {
		case "+":
			c.Emit(OpAdd)
		case "-":
			c.Emit(OpSub)
		case "*":
			c.Emit(OpMul)
		case "/":
			c.Emit(OpDiv)
		case "%":
			c.Emit(OpMod)
		case "**":
			c.Emit(OpPow)
		case "==":
			c.Emit(OpEqual)
		case "!=":
			c.Emit(OpNotEqual)
		case "===":
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "===",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
		case ">":
			c.Emit(OpGreaterThan)
		case ">=":
			c.Emit(OpGreaterThanOrEqual)
		case "<":
			c.Emit(OpLessThan)
		case "<=":
			c.Emit(OpLessThanOrEqual)
		case "<=>":
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "<=>",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
		case "=~":
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "=~",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
			c.emitNamedRegexpCaptureBindings(namedCaptures)
		case "!~":
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "!~",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
		case "&":
			c.Emit(OpBitAnd)
		case "|":
			c.Emit(OpBitOr)
		case "^":
			c.Emit(OpBitXor)
		case "~":
			c.Emit(OpBitNot)
		case "<<":
			c.Emit(OpBitLeftShift)
		case ">>":
			c.Emit(OpBitRightShift)
		}
	case *ast.PrefixExpression:
		if node.Operator == "-" {
			if call, ok := node.Right.(*ast.MethodCall); ok && call.Receiver != nil {
				if negativeCall, ok := negateNumericLiteralCallReceiver(call); ok {
					return c.Compile(negativeCall)
				}
			}
		}
		if err := c.Compile(node.Right); err != nil {
			return err
		}

		switch node.Operator {
		case "!", "not":
			c.Emit(OpBang)
		case "+":
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "+@",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 0, 255)
		case "-":
			c.Emit(OpNeg)
		case "~":
			c.Emit(OpBitNot)
		}
	case *ast.IfExpression:
		if err := c.compileCondition(node.Condition); err != nil {
			return err
		}

		jumpOp := OpJumpNotTruthy
		if node.IsUnless {
			jumpOp = OpJumpTruthy
		}
		jumpNotTruthyPos := c.emit(jumpOp, 9999)

		if err := c.compileBlockAsValue(node.Consequent); err != nil {
			return err
		}

		if len(node.ElsIf) == 0 && node.Alternative == nil {
			// Simple if without else — push nil when condition is false
			jumpToEnd := c.emit(OpJump, 9999)
			afterConsequent := len(c.currentInstructions())
			c.changeOperand(jumpNotTruthyPos, afterConsequent)
			c.Emit(OpNil)
			afterNil := len(c.currentInstructions())
			c.changeOperand(jumpToEnd, afterNil)
		} else {
			// if with elsif/else — need jump over remaining branches
			jumpToEndPositions := []int{}
			jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))

			afterConsequent := len(c.currentInstructions())
			c.changeOperand(jumpNotTruthyPos, afterConsequent)

			// Compile elsif branches
			for _, elsif := range node.ElsIf {
				if err := c.compileCondition(elsif.Condition); err != nil {
					return err
				}
				elsifJumpPos := c.emit(OpJumpNotTruthy, 9999)
				if err := c.compileBlockAsValue(elsif.Consequent); err != nil {
					return err
				}
				jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))
				afterElsif := len(c.currentInstructions())
				c.changeOperand(elsifJumpPos, afterElsif)
			}

			// Compile else branch
			if node.Alternative != nil {
				if err := c.compileBlockAsValue(node.Alternative); err != nil {
					return err
				}
			} else {
				c.Emit(OpNil)
			}

			// Patch all jump-to-end positions
			afterAll := len(c.currentInstructions())
			for _, pos := range jumpToEndPositions {
				c.changeOperand(pos, afterAll)
			}
		}
	case *ast.CaseExpression:
		patternCase := false
		for _, clause := range node.Clauses {
			for _, condition := range clause.Conditions {
				if _, ok := condition.(*ast.PatternMatchExpression); ok {
					patternCase = true
				}
			}
		}
		if patternCase {
			c.emit(OpPatternCacheClear, 1)
		}
		if node.Expression != nil {
			if err := c.Compile(node.Expression); err != nil {
				return err
			}
		}
		jumpToEndPositions := []int{}
		for _, clause := range node.Clauses {
			for _, cond := range clause.Conditions {
				if node.Expression != nil {
					c.Emit(OpDup)
					if _, isPattern := cond.(*ast.PatternMatchExpression); isPattern {
						patternCase = true
						if err := c.Compile(cond); err != nil {
							return err
						}
						condJumpPositions := []int{c.emit(OpJumpNotTruthy, 9999)}
						match := cond.(*ast.PatternMatchExpression)
						if match.Guard != nil {
							if err := c.Compile(match.Guard); err != nil {
								return err
							}
							if match.GuardUnless {
								c.Emit(OpBang)
							}
							condJumpPositions = append(condJumpPositions, c.emit(OpJumpNotTruthy, 9999))
						}
						c.Emit(OpPop)
						if err := c.compileBlockAsValue(clause.Body); err != nil {
							return err
						}
						jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))
						afterCond := len(c.currentInstructions())
						for _, pos := range condJumpPositions {
							c.changeOperand(pos, afterCond)
						}
						continue
					}
					if splat, ok := cond.(*ast.SplatExpression); ok {
						if err := c.Compile(splat.Value); err != nil {
							return err
						}
						c.Emit(OpSplatToArray)
						c.Emit(OpCaseSplatMatch)
					} else {
						if err := c.Compile(cond); err != nil {
							return err
						}
						c.Emit(OpSwap)
						methodNameIdx := c.addConstant(&object.EmeraldValue{
							Type:  object.ValueString,
							Data:  "===",
							Class: core.R.Classes["String"],
						})
						c.emit(OpSend, methodNameIdx, 0, 1, 255)
					}
					condJumpPos := c.emit(OpJumpNotTruthy, 9999)
					c.Emit(OpPop)
					if err := c.compileBlockAsValue(clause.Body); err != nil {
						return err
					}
					jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))
					afterCond := len(c.currentInstructions())
					c.changeOperand(condJumpPos, afterCond)
				} else {
					if err := c.Compile(cond); err != nil {
						return err
					}
					condJumpPos := c.emit(OpJumpNotTruthy, 9999)
					if err := c.compileBlockAsValue(clause.Body); err != nil {
						return err
					}
					jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))
					afterCond := len(c.currentInstructions())
					c.changeOperand(condJumpPos, afterCond)
				}
			}
		}
		if node.Else != nil {
			if node.Expression != nil {
				c.Emit(OpPop)
			}
			if err := c.compileBlockAsValue(node.Else); err != nil {
				return err
			}
		} else {
			if node.Expression != nil {
				if patternCase {
					c.Emit(OpRaiseNoMatchingPattern)
				} else {
					c.Emit(OpPop)
					c.Emit(OpNil)
				}
			} else {
				c.Emit(OpNil)
			}
		}
		endTarget := len(c.currentInstructions())
		if patternCase {
			c.emit(OpPatternCacheClear, 0)
		}
		afterAll := len(c.currentInstructions())
		for _, pos := range jumpToEndPositions {
			if patternCase {
				c.changeOperand(pos, endTarget)
			} else {
				c.changeOperand(pos, afterAll)
			}
		}
	case *ast.PatternMatchExpression:
		if node.Left != nil {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
		}
		if node.Pattern != "" {
			c.definePatternLocals(node.Pattern)
			c.emit(OpPatternCheck, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Pattern,
				Class: core.R.Classes["String"],
			}))
		} else {
			c.Emit(OpFalse)
		}
	case *ast.ArrayLiteral:
		hasSplat := false
		for _, element := range node.Elements {
			if _, ok := element.(*ast.SplatExpression); ok {
				hasSplat = true
				break
			}
		}
		if hasSplat {
			c.emit(OpArray, 0)
			for _, element := range node.Elements {
				splat := 0
				compileElement := element
				if expression, ok := element.(*ast.SplatExpression); ok {
					splat = 1
					compileElement = expression.Value
				}
				if err := c.Compile(compileElement); err != nil {
					return err
				}
				c.emit(OpArrayAppend, splat)
			}
			break
		}
		for _, e := range node.Elements {
			if err := c.Compile(e); err != nil {
				return err
			}
		}
		c.emit(OpArray, len(node.Elements))
	case *ast.HashLiteral:
		keys := node.Order
		for i := len(keys) - 1; i >= 0; i-- {
			if err := c.Compile(node.Pairs[keys[i]]); err != nil {
				return err
			}
			if err := c.Compile(keys[i]); err != nil {
				return err
			}
		}
		c.emit(OpHash, len(node.Pairs))
	case *ast.IndexExpression:
		if err := c.Compile(node.Left); err != nil {
			return err
		}
		if err := c.Compile(node.Index); err != nil {
			return err
		}
		if node.End != nil {
			if err := c.Compile(node.End); err != nil {
				return err
			}
			c.emit(OpSliceIndex)
		} else {
			c.Emit(OpIndex)
		}
	case *ast.AssignExpression:
		if node.Index != nil {
			target := ast.Expression(node.Name)
			if node.Target != nil {
				target = node.Target
			}
			if (node.Token.Type == lexer.OR_ASSIGN || node.Token.Type == lexer.AND_ASSIGN) && !expressionContainsSplat(node.Index) && !expressionContainsSplat(node.End) {
				args := []ast.Expression{node.Index}
				if node.End != nil {
					args = append(args, node.End)
				}
				return c.compileLogicalSendAssignment(target, "[]", "[]=", args, node.Value, node.Token.Type, false)
			}
			if node.End != nil {
				if err := c.Compile(target); err != nil {
					return err
				}
				if err := c.Compile(node.Index); err != nil {
					return err
				}
				if err := c.Compile(node.End); err != nil {
					return err
				}
				if err := c.Compile(node.Value); err != nil {
					return err
				}
				methodNameIdx := c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  "[]=",
					Class: core.R.Classes["String"],
				})
				c.emit(OpSendSetter, methodNameIdx, 0, 3, 255)
				return nil
			}
			if err := c.Compile(target); err != nil {
				return err
			}
			if err := c.Compile(node.Index); err != nil {
				return err
			}
			if err := c.Compile(node.Value); err != nil {
				return err
			}
			if method, ok := compoundAssignmentMethod(node.Token.Type); ok {
				methodIdx := c.addConstant(&object.EmeraldValue{
					Type: object.ValueString, Data: method, Class: core.R.Classes["String"],
				})
				c.emit(OpIndexCompoundAssign, methodIdx)
				return nil
			}
			c.Emit(OpIndexAssign)
			return nil
		}

		if node.Target != nil && isConstantName(node.Name.Value) {
			if err := c.Compile(node.Target); err != nil {
				return err
			}
			name := &object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}
			mode := ScopedConstAssignPlain
			switch node.Token.Type {
			case lexer.OR_ASSIGN:
				mode = ScopedConstAssignOr
			case lexer.AND_ASSIGN:
				mode = ScopedConstAssignAnd
			case lexer.PLUS_ASSIGN:
				mode = ScopedConstAssignAdd
			}
			if mode == ScopedConstAssignPlain {
				if err := c.Compile(node.Value); err != nil {
					return err
				}
				c.emit(OpSetScopedConstant, c.addConstant(name), mode)
				return nil
			}
			if mode == ScopedConstAssignOr || mode == ScopedConstAssignAnd {
				if err := c.compileExpressionThunk(node.Value); err != nil {
					return err
				}
			} else if err := c.Compile(node.Value); err != nil {
				return err
			}
			c.emit(OpSetScopedConstant, c.addConstant(name), mode)
			return nil
		}

		// Ruby treats a local assignment target as declared throughout the
		// surrounding scope, including closures in the assignment RHS.
		if node.Target == nil && isValidLocalNameLikeRuby(node.Name.Value) {
			if symbol, ok := c.symbolTable.Resolve(node.Name.Value); !ok || symbol.Scope == ScopeBuiltin {
				c.symbolTable.Define(node.Name.Value)
			}
		}

		if (node.Token.Type == lexer.OR_ASSIGN || node.Token.Type == lexer.AND_ASSIGN) && !isConstantName(node.Name.Value) {
			if err := c.compileAssignmentCurrentValue(node.Name); err != nil {
				return err
			}
			c.Emit(OpDup)
			jumpOp := OpJumpTruthy
			if node.Token.Type == lexer.AND_ASSIGN {
				jumpOp = OpJumpNotTruthy
			}
			jumpSkip := c.emit(jumpOp, 9999)
			c.Emit(OpPop)
			if err := c.Compile(node.Value); err != nil {
				return err
			}
			if err := c.emitVariableAssignmentStore(node.Name); err != nil {
				return err
			}
			jumpEnd := c.emit(OpJump, 9999)
			c.changeOperand(jumpSkip, len(c.currentInstructions()))
			c.changeOperand(jumpEnd, len(c.currentInstructions()))
			return nil
		}

		if op, ok := compoundAssignmentOpcode(node.Token.Type); ok {
			if err := c.compileAssignmentCurrentValue(node.Name); err != nil {
				return err
			}
			if err := c.Compile(node.Value); err != nil {
				return err
			}
			c.Emit(op)
		} else if splat, ok := node.Value.(*ast.SplatExpression); ok {
			if err := c.Compile(splat.Value); err != nil {
				return err
			}
			c.Emit(OpSplatToArray)
		} else if err := c.Compile(node.Value); err != nil {
			return err
		}
		preserveSplatRHS := isSplatMultiAssignTarget(node.Target)
		if preserveSplatRHS {
			c.Emit(OpDup)
			c.Emit(OpMultiAssignPrepare)
		}

		// Check if the name is a global variable (starts with $)
		if len(node.Name.Value) > 0 && node.Name.Value[0] == '$' {
			c.emit(OpSetGlobal, c.globalSymbolIndex(node.Name.Value))
			if preserveSplatRHS {
				c.Emit(OpPop)
			}
			return nil
		}

		// Check if the name is a class variable (starts with @@)
		if len(node.Name.Value) > 1 && node.Name.Value[0] == '@' && node.Name.Value[1] == '@' {
			c.emit(OpSetClassVar, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			if preserveSplatRHS {
				c.Emit(OpPop)
			}
			return nil
		}

		// Check if the name is an instance variable (starts with @)
		if len(node.Name.Value) > 0 && node.Name.Value[0] == '@' {
			c.emit(OpSetInstanceVar, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			if preserveSplatRHS {
				c.Emit(OpPop)
			}
			return nil
		}

		if isConstantName(node.Name.Value) {
			c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}), 0)
			if preserveSplatRHS {
				c.Emit(OpPop)
			}
			return nil
		}

		sym, ok := c.symbolTable.Resolve(node.Name.Value)
		if !ok || sym.Scope == ScopeBuiltin {
			c.symbolTable.Define(node.Name.Value)
			sym, _ = c.symbolTable.Resolve(node.Name.Value)
		}

		switch sym.Scope {
		case ScopeGlobal:
			c.emit(OpSetGlobal, sym.Index)
		case ScopeLocal:
			c.emit(OpSetLocal, sym.Index)
		case ScopeOuter:
			c.emit(OpSetOuter, 0, sym.ScopeIndex)
		case ScopeOuterFree:
			c.emit(OpSetOuterFree, sym.ScopeIndex)
		case ScopeFree:
			c.emit(OpSetFree, sym.Index)
		}
		if preserveSplatRHS {
			c.Emit(OpPop)
		}
	case *ast.MultiAssignExpression:
		contexts := make(map[ast.Expression]*multiAssignTargetContext)
		for _, target := range node.Targets {
			if err := c.prepareMultiAssignTarget(target, contexts); err != nil {
				return err
			}
		}
		if len(node.Values) == 1 {
			if splat, ok := node.Values[0].(*ast.SplatExpression); ok {
				if err := c.Compile(splat.Value); err != nil {
					return err
				}
				c.Emit(OpSplatToArray)
			} else {
				if err := c.Compile(node.Values[0]); err != nil {
					return err
				}
			}
			c.Emit(OpDup)
			c.Emit(OpMultiAssignPrepare)
		} else {
			c.emit(OpArray, 0)
			for _, val := range node.Values {
				splatMode := 0
				if splat, ok := val.(*ast.SplatExpression); ok {
					if err := c.Compile(splat.Value); err != nil {
						return err
					}
					splatMode = 2
				} else {
					if err := c.Compile(val); err != nil {
						return err
					}
				}
				c.emit(OpArrayAppend, splatMode)
			}
			c.Emit(OpDup)
		}
		if err := c.compileMultiAssignTargets(node.Targets, contexts); err != nil {
			return err
		}
		c.Emit(OpPop)
	case *ast.MethodCall:
		if node.Assignment && node.Receiver != nil && len(node.Args) >= 1 {
			if compound, ok := node.Args[len(node.Args)-1].(*ast.InfixExpression); ok {
				if getter, ok := compound.Left.(*ast.MethodCall); ok && getter.Receiver == node.Receiver && getter.Method != nil && getter.Method.Value == "[]" && len(getter.Args) == 1 {
					if splat, ok := getter.Args[0].(*ast.SplatExpression); ok {
						if err := c.Compile(node.Receiver); err != nil {
							return err
						}
						jumpEnd := -1
						if node.Safe {
							c.Emit(OpDup)
							jumpCall := c.emit(OpJumpNotNil, 9999)
							jumpEnd = c.emit(OpJump, 9999)
							c.changeOperand(jumpCall, len(c.currentInstructions()))
						}
						if err := c.Compile(splat.Value); err != nil {
							return err
						}
						if err := c.Compile(compound.Right); err != nil {
							return err
						}
						methodIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: compound.Operator, Class: core.R.Classes["String"]})
						c.emit(OpIndexSplatCompoundAssign, methodIdx)
						if jumpEnd >= 0 {
							c.changeOperand(jumpEnd, len(c.currentInstructions()))
						}
						return nil
					}
				}
			}
		}
		if node.Assignment && node.Receiver != nil && len(node.Args) == 1 {
			if compound, ok := node.Args[0].(*ast.InfixExpression); ok {
				if getter, ok := compound.Left.(*ast.MethodCall); ok && getter.Receiver == node.Receiver && len(getter.Args) == 0 && len(getter.KeywordArgs) == 0 {
					if err := c.Compile(node.Receiver); err != nil {
						return err
					}
					jumpEnd := -1
					if node.Safe {
						c.Emit(OpDup)
						jumpCall := c.emit(OpJumpNotNil, 9999)
						jumpEnd = c.emit(OpJump, 9999)
						c.changeOperand(jumpCall, len(c.currentInstructions()))
					}
					c.Emit(OpDup)
					getterIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: getter.Method.Value, Class: core.R.Classes["String"]})
					c.emit(OpSend, getterIdx, 0, 0, 255)
					if err := c.Compile(compound.Right); err != nil {
						return err
					}
					if !c.emitCompoundOperator(compound.Operator) {
						return fmt.Errorf("unsupported compound assignment operator %s", compound.Operator)
					}
					setterIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: node.Method.Value, Class: core.R.Classes["String"]})
					c.emit(OpSendSetter, setterIdx, 0, 1, 255)
					if jumpEnd >= 0 {
						c.changeOperand(jumpEnd, len(c.currentInstructions()))
					}
					return nil
				}
			}
		}
		if (node.LogicalAssignment == lexer.OR_ASSIGN || node.LogicalAssignment == lexer.AND_ASSIGN) && len(node.Args) > 0 {
			args := node.Args[:len(node.Args)-1]
			if !expressionsContainSplat(args) {
				setter := node.Method.Value
				getter := strings.TrimSuffix(setter, "=")
				return c.compileLogicalSendAssignment(node.Receiver, getter, setter, args, node.Args[len(node.Args)-1], node.LogicalAssignment, node.Safe)
			}
		}
		if c.methodDepth > 0 && node.Receiver == nil && node.Method != nil && node.Method.Value == "END" && node.Block != nil {
			fmt.Fprintln(os.Stderr, "warning: END in method; use at_exit")
		}
		if node.Receiver == nil && node.Method != nil && len(node.Args) == 1 && len(node.KeywordArgs) == 0 && node.Block == nil {
			if sym, ok := c.symbolTable.Resolve(node.Method.Value); ok && sym.Scope == ScopeLocal {
				if prefix, ok := node.Args[0].(*ast.PrefixExpression); ok && prefix.Operator == "-" {
					if err := c.Compile(node.Method); err != nil {
						return err
					}
					if err := c.Compile(prefix.Right); err != nil {
						return err
					}
					c.Emit(OpSub)
					return nil
				}
			}
		}

		if node.Receiver != nil {
			if err := c.Compile(node.Receiver); err != nil {
				return err
			}
		} else {
			c.Emit(OpSelf)
		}

		if node.Method == nil {
			return fmt.Errorf("method call missing method name at line %d column %d after %q (receiver %T)", node.Token.Line, node.Token.Column, node.Token.Literal, node.Receiver)
		}

		methodNameIdx := c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Method.Value,
			Class: core.R.Classes["String"],
		})

		var jumpEnd int
		if node.Safe {
			c.Emit(OpDup)
			jumpCall := c.emit(OpJumpNotNil, 9999)
			jumpEnd = c.emit(OpJump, 9999)
			c.changeOperand(jumpCall, len(c.currentInstructions()))
		}

		args := expandForwardArguments(node.Args)
		var blockPass ast.Expression
		var blockPassAnonymous bool
		for i, arg := range args {
			if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type == lexer.BIT_AND {
				blockPass = splat.Value
				blockPassAnonymous = splat.AnonymousBlockPass
				args = append(append([]ast.Expression(nil), args[:i]...), args[i+1:]...)
				break
			}
		}

		splatCount := 0
		for i, arg := range args {
			if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type == lexer.MULTIPLY {
				if node.Assignment && i == len(args)-1 {
					continue
				}
				splatCount++
			}
		}
		splatIndex := 255
		argCount := len(args)
		if splatCount > 1 {
			for i, arg := range args {
				if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type == lexer.MULTIPLY {
					if err := c.Compile(splat.Value); err != nil {
						return err
					}
					c.Emit(OpSplatToArray)
					if node.Assignment && i == len(args)-1 {
						c.emit(OpArray, 1)
					}
				} else {
					if err := c.Compile(arg); err != nil {
						return err
					}
					c.emit(OpArray, 1)
				}
				if i > 0 {
					c.Emit(OpAdd)
				}
			}
			splatIndex = 0
			argCount = 1
		} else {
			for i, arg := range args {
				compileArg := arg
				assignmentSplat := false
				if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type != lexer.BIT_AND {
					compileArg = splat.Value
					if splat.Token.Type == lexer.MULTIPLY && node.Assignment && i == len(args)-1 {
						assignmentSplat = true
					} else if splat.Token.Type == lexer.MULTIPLY && splatIndex == 255 {
						splatIndex = i
						assignmentSplat = true
					}
				}
				if err := c.Compile(compileArg); err != nil {
					return err
				}
				if hash, ok := arg.(*ast.HashLiteral); ok && hash.Token.Type == lexer.ARROW && len(node.KeywordArgs) > 0 {
					c.Emit(OpMarkKeywordHash)
				}
				if assignmentSplat {
					c.Emit(OpSplatToArray)
				}
			}
		}

		sendOp := OpSend
		if len(args) > 0 {
			if hash, ok := args[len(args)-1].(*ast.HashLiteral); ok && hash.Token.Type == lexer.ARROW {
				sendOp = OpSendWithKeywords
			}
			if splat, ok := args[len(args)-1].(*ast.SplatExpression); ok && splat.Token.Type == lexer.POW {
				sendOp = OpSendWithKeywords
			}
		}
		if len(node.KeywordArgs) > 0 {
			sendOp = OpSendWithKeywords
			for i := len(node.KeywordArgs) - 1; i >= 0; i-- {
				kwa := node.KeywordArgs[i]
				if err := c.Compile(kwa.Value); err != nil {
					return err
				}
				c.EmitConstant(&object.EmeraldValue{
					Type:  object.ValueSymbol,
					Data:  normalizedStaticSymbolName(kwa.Name),
					Class: core.R.Classes["Symbol"],
				})
			}
			c.emit(OpHash, len(node.KeywordArgs))
			argCount++
		}
		if node.Assignment {
			sendOp = OpSendSetter
		}

		blockArg := 0
		if blockPass != nil {
			if blockPassAnonymous {
				blockArg = 2
			} else {
				if err := c.Compile(blockPass); err != nil {
					return err
				}
				blockArg = 1
			}
		} else if node.Block != nil {
			if err := c.compileBlockAsClosure(node.Block); err != nil {
				return err
			}
			blockArg = 1
		}
		c.emit(sendOp, methodNameIdx, blockArg, argCount, splatIndex)
		if node.Safe {
			c.changeOperand(jumpEnd, len(c.currentInstructions()))
		}
	case *ast.IncludeExpression:
		c.Emit(OpSelf)
		if err := c.Compile(node.Module); err != nil {
			return err
		}
		methodNameIdx := c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "include",
			Class: core.R.Classes["String"],
		})
		c.emit(OpSend, methodNameIdx, 0, 1, 255)
	case *ast.ExtendExpression:
		c.Emit(OpSelf)
		if err := c.Compile(node.Module); err != nil {
			return err
		}
		methodNameIdx := c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "extend",
			Class: core.R.Classes["String"],
		})
		c.emit(OpSend, methodNameIdx, 0, 1, 255)
	case *ast.PrependExpression:
		c.Emit(OpSelf)
		if err := c.Compile(node.Module); err != nil {
			return err
		}
		methodNameIdx := c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "prepend",
			Class: core.R.Classes["String"],
		})
		c.emit(OpSend, methodNameIdx, 0, 1, 255)
	case *ast.UndefExpression:
		c.Emit(OpSelf)
		for _, method := range node.Methods {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueSymbol,
				Data:  normalizedStaticSymbolName(method.String()),
				Class: core.R.Classes["Symbol"],
			})
		}
		methodNameIdx := c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "undef_method",
			Class: core.R.Classes["String"],
		})
		c.emit(OpSend, methodNameIdx, 0, len(node.Methods), 255)
	case *ast.AliasExpression:
		c.Emit(OpSelf)
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  normalizedStaticSymbolName(node.New.String()),
			Class: core.R.Classes["Symbol"],
		})
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  normalizedStaticSymbolName(node.Old.String()),
			Class: core.R.Classes["Symbol"],
		})
		c.emit(OpAlias)
	case *ast.ReturnExpression:
		topLevelEvalReturn := c.scopeIndex == 0 && c.methodDepth == 0 && c.evalTopLevelReturn
		if c.scopeIndex == 0 && c.methodDepth == 0 && node.ReturnValue != nil && !topLevelEvalReturn {
			fmt.Fprintln(os.Stderr, "warning: argument of top-level return is ignored")
		}
		if node.ReturnValue != nil {
			if splat, ok := node.ReturnValue.(*ast.SplatExpression); ok {
				if err := c.Compile(splat.Value); err != nil {
					return err
				}
				c.Emit(OpSplatToArray)
			} else if err := c.Compile(node.ReturnValue); err != nil {
				return err
			}
		} else {
			c.Emit(OpNil)
		}
		if topLevelEvalReturn {
			c.Emit(OpNonLocalReturnValue)
		} else {
			c.Emit(OpReturnValue)
		}
	case *ast.DefExpression:
		c.EnterScope()
		c.symbolTable.MethodBoundary = true

		paramIndices := make([]int, len(node.Params))
		for i, param := range node.Params {
			paramIndices[i] = c.symbolTable.DefineParameter(param.Value).Index
		}
		c.defineParameterPatternLocals(node.ParamPatterns)

		anonymousRestParam := node.RestParam != nil && node.RestParam.Value == "_" && node.RestParam.Token.Literal == "*"
		if node.RestParam != nil && !anonymousRestParam {
			c.symbolTable.Define(node.RestParam.Value)
		}

		for _, kp := range node.KeywordParams {
			c.symbolTable.Define(kp.Name)
		}
		if node.KeywordRestParam != nil {
			c.symbolTable.Define(node.KeywordRestParam.Value)
		}

		blockParamIndex := -1
		if node.BlockParam != nil {
			blockParamIndex = c.symbolTable.Define(node.BlockParam.Value).Index
		}

		c.methodDepth++
		for i, defaultExpr := range node.ParamDefaults {
			if defaultExpr == nil || i >= len(paramIndices) {
				continue
			}
			jump := c.emit(OpJumpLocalPresent, paramIndices[i], 0)
			if err := c.Compile(defaultExpr); err != nil {
				c.methodDepth--
				return err
			}
			c.emit(OpSetLocal, paramIndices[i])
			c.Emit(OpPop)
			c.changeOperandAt(jump, 1, len(c.currentInstructions()))
		}
		for _, keyword := range node.KeywordParams {
			if err := c.compileParameterDefault(keyword.Name, keyword.Default); err != nil {
				c.methodDepth--
				return err
			}
		}
		if err := c.compileBlockAsValue(node.Body); err != nil {
			c.methodDepth--
			return err
		}
		c.methodDepth--

		c.replaceLastPopWithReturn()

		free := c.symbolTable.FreeSymbols
		numLocals := c.symbolTable.MaxSymbols
		localNames := c.localNames()

		lineMap := c.currentLineMapCopy()
		instructions := c.LeaveScope()

		kwParams := make([]object.KeywordParamInfo, len(node.KeywordParams))
		for i, kp := range node.KeywordParams {
			info := object.KeywordParamInfo{
				Name:       kp.Name,
				HasDefault: kp.Default != nil,
			}
			if kp.Default != nil {
				info.Default = c.compileDefaultValue(kp.Default)
			}
			kwParams[i] = info
		}
		paramDefaults := make([]*object.EmeraldValue, len(node.Params))
		for i, defaultExpr := range node.ParamDefaults {
			if i >= len(paramDefaults) {
				break
			}
			if defaultExpr != nil {
				paramDefaults[i] = c.compileDefaultValue(defaultExpr)
			}
		}
		params := make([]string, len(node.Params))
		for i, param := range node.Params {
			params[i] = param.Value
		}

		fnObj := &object.Function{
			Name:                  node.Name.Value,
			SourcePath:            core.CurrentSpecFile,
			EvalSource:            core.CurrentEvalSource,
			SourceEncoding:        c.sourceEncoding,
			DefinitionLine:        int64(node.Token.Line),
			Params:                params,
			ParamLocalIndices:     paramIndices,
			ParamPatterns:         compileParameterPatterns(node.ParamPatterns, len(params)),
			Instructions:          instructions,
			LineMap:               lineMap,
			NumLocals:             numLocals,
			GlobalNames:           c.globalNamesCopy(),
			LocalNames:            localNames,
			ParamDefaults:         paramDefaults,
			EvaluateParamDefaults: true,
			MethodBody:            true,
			KeywordParams:         kwParams,
			RejectKeywords:        node.RejectKeywords,
			KeywordRestOnly:       node.KeywordRestOnly,
			RejectBlock:           node.RejectBlock,
			FreeVarNames:          freeVarNames(free),
		}
		if node.KeywordRestParam != nil {
			fnObj.KeywordRestParam = node.KeywordRestParam.Value
		}
		if node.RestParam != nil {
			fnObj.HasRestParam = true
			fnObj.AnonymousRestParam = anonymousRestParam
			fnObj.RestParamIndex = node.RestParamIndex
			if !anonymousRestParam {
				fnObj.RestParamName = node.RestParam.Value
			}
		}
		if node.BlockParam != nil {
			fnObj.HasBlockParam = true
			fnObj.AnonymousBlockParam = node.BlockParam.Value == "_" && node.BlockParam.Token.Literal == "&"
			fnObj.BlockParamIndex = blockParamIndex
		}

		fn := &object.EmeraldValue{
			Type:  object.ValueFunction,
			Data:  fnObj,
			Class: core.R.Classes["Class"],
		}
		fnIdx := c.addConstant(fn)

		if node.Receiver != nil {
			if err := c.Compile(node.Receiver); err != nil {
				return err
			}
		}

		for _, s := range free {
			c.emitCaptureSymbol(s)
		}
		c.emit(OpClosure, fnIdx, len(free))

		op := OpDefineMethod
		if node.Receiver != nil {
			op = OpDefineSingletonMethod
		}
		c.emit(op, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))
	case *ast.ClassExpression:
		definitionName := node.Name.Value
		if node.Absolute {
			definitionName = "::" + definitionName
		}
		if node.SingletonReceiver != nil {
			if err := c.Compile(node.SingletonReceiver); err != nil {
				return err
			}
			c.Emit(OpSingletonClass)
		} else if node.SuperClass != nil {
			if ident, ok := node.SuperClass.(*ast.Identifier); ok && (ident.Token.Type == lexer.CONSTANT || strings.Contains(ident.Value, "::")) {
				c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  ident.Value,
					Class: core.R.Classes["String"],
				}))
			} else {
				if err := c.Compile(node.SuperClass); err != nil {
					return err
				}
			}
		}

		if node.SingletonReceiver == nil {
			hasSuperclass := 0
			if node.SuperClass != nil {
				hasSuperclass = 1
			}
			c.emit(OpClass, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  definitionName,
				Class: core.R.Classes["String"],
			}), hasSuperclass)

			if node.SuperClass != nil {
				c.Emit(OpInherited)
			}
		}

		c.EnterScope()
		c.symbolTable.MethodBoundary = true

		if err := c.Compile(node.Body); err != nil {
			return err
		}
		if node.SingletonReceiver != nil {
			c.replaceOpcodes(OpReturnValue, OpNonLocalReturnValue)
		}

		if len(c.currentInstructions()) == 0 {
			c.Emit(OpNil)
			c.Emit(OpReturnValue)
		} else {
			c.replaceLastPopWithReturn()
		}

		numLocals := c.symbolTable.MaxSymbols
		localNames := c.localNames()
		lineMap := c.currentLineMapCopy()
		instructions := c.LeaveScope()

		bodyFn := &object.EmeraldValue{
			Type: object.ValueFunction,
			Data: &object.Function{
				Name:               node.Name.Value + "#body",
				SourcePath:         core.CurrentSpecFile,
				EvalSource:         core.CurrentEvalSource,
				SourceEncoding:     c.sourceEncoding,
				Instructions:       instructions,
				LineMap:            lineMap,
				NumLocals:          numLocals,
				GlobalNames:        c.globalNamesCopy(),
				LocalNames:         localNames,
				FreeVarNames:       freeVarNames(c.symbolTable.FreeSymbols),
				SingletonClassBody: node.SingletonReceiver != nil,
			},
			Class: core.R.Classes["Class"],
		}
		fnIdx := c.addConstant(bodyFn)
		c.emit(OpClosure, fnIdx, 0)
		c.emit(OpSend, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__exec_class_body__",
			Class: core.R.Classes["String"],
		}), 1, 0, 255)
		if node.SingletonReceiver == nil {
			c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  definitionName,
				Class: core.R.Classes["String"],
			}), 1)
		}
	case *ast.ModuleExpression:
		definitionName := node.Name.Value
		if node.Absolute {
			definitionName = "::" + definitionName
		}
		c.emit(OpModule, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  definitionName,
			Class: core.R.Classes["String"],
		}))

		c.EnterScope()
		c.symbolTable.MethodBoundary = true
		c.Emit(OpPop)

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.Emit(OpReturnValue)
		numLocals := c.symbolTable.MaxSymbols
		localNames := c.localNames()
		lineMap := c.currentLineMapCopy()
		instructions := c.LeaveScope()

		bodyFn := &object.EmeraldValue{
			Type: object.ValueFunction,
			Data: &object.Function{
				Name:           node.Name.Value + "#body",
				SourcePath:     core.CurrentSpecFile,
				EvalSource:     core.CurrentEvalSource,
				SourceEncoding: c.sourceEncoding,
				Instructions:   instructions,
				LineMap:        lineMap,
				NumLocals:      numLocals,
				GlobalNames:    c.globalNamesCopy(),
				LocalNames:     localNames,
				FreeVarNames:   freeVarNames(c.symbolTable.FreeSymbols),
			},
			Class: core.R.Classes["Class"],
		}
		fnIdx := c.addConstant(bodyFn)
		c.emit(OpClosure, fnIdx, 0)
		c.emit(OpSend, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__exec_class_body__",
			Class: core.R.Classes["String"],
		}), 1, 0, 255)
		c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  definitionName,
			Class: core.R.Classes["String"],
		}), 1)
		c.Emit(OpPop)
	case *ast.BlockExpression:
		// If block has params, compile as closure
		if len(node.Params) > 0 {
			c.EnterScope()

			paramIndices := make([]int, len(node.Params))
			for i, param := range node.Params {
				paramIndices[i] = c.symbolTable.DefineParameter(param.Value).Index
			}

			// Compile block body - use compileBlockAsValue to keep last value on stack
			if err := c.compileBlockAsValue(node); err != nil {
				return err
			}

			c.Emit(OpReturnValue)

			free := c.symbolTable.FreeSymbols
			numLocals := c.symbolTable.MaxSymbols
			localNames := c.localNames()
			lineMap := c.currentLineMapCopy()
			instructions := c.LeaveScope()

			fnObj := &object.Function{
				Name:              "__block__",
				SourcePath:        core.CurrentSpecFile,
				EvalSource:        core.CurrentEvalSource,
				SourceEncoding:    c.sourceEncoding,
				Params:            identifierNames(node.Params),
				ParamLocalIndices: paramIndices,
				Instructions:      instructions,
				LineMap:           lineMap,
				NumLocals:         numLocals,
				LocalNames:        localNames,
				FreeVarNames:      freeVarNames(free),
			}

			fn := &object.EmeraldValue{
				Type:  object.ValueFunction,
				Data:  fnObj,
				Class: core.R.Classes["Class"],
			}
			fnIdx := c.addConstant(fn)

			for _, s := range free {
				c.emitCaptureSymbol(s)
			}
			c.emit(OpClosure, fnIdx, len(free))
		} else {
			// No params - compile inline (for if/while bodies)
			for _, s := range node.Statements {
				if err := c.Compile(s); err != nil {
					return err
				}
			}
		}
	case *ast.ProcLiteral:
		if err := c.compileProcLiteral(node); err != nil {
			return err
		}
	case *ast.WhileExpression:
		if node.Post {
			return c.compilePostWhileExpression(node)
		}
		loopStart := len(c.currentInstructions())

		if err := c.compileCondition(node.Condition); err != nil {
			return err
		}

		jumpNotTruthyPos := c.emit(OpJumpNotTruthy, 9999)

		c.scopes[c.scopeIndex].breakTarget = -1
		c.scopes[c.scopeIndex].nextPatchPos = []int{}
		c.scopes[c.scopeIndex].breakValuePatchPos = []int{}

		setWhileEndPos := c.emit(OpSetWhileEnd, 0)
		bodyStart := len(c.currentInstructions())
		previousRedoTarget := c.scopes[c.scopeIndex].redoTarget
		previousNextPatchTarget := c.scopes[c.scopeIndex].nextPatchTarget
		c.scopes[c.scopeIndex].redoTarget = bodyStart
		c.scopes[c.scopeIndex].nextPatchTarget = loopStart

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.emit(OpJump, loopStart)

		afterBody := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBody)
		c.changeOperand(setWhileEndPos, afterBody)

		c.scopes[c.scopeIndex].breakTarget = afterBody

		c.Emit(OpNil)

		endOfWhile := len(c.currentInstructions())

		for _, patchPos := range c.scopes[c.scopeIndex].nextPatchPos {
			c.changeOperand(patchPos, loopStart)
		}
		for _, patchPos := range c.scopes[c.scopeIndex].breakValuePatchPos {
			c.changeOperand(patchPos, endOfWhile)
		}
		c.scopes[c.scopeIndex].breakTarget = -1
		c.scopes[c.scopeIndex].nextPatchPos = []int{}
		c.scopes[c.scopeIndex].breakValuePatchPos = []int{}
		c.scopes[c.scopeIndex].nextPatchTarget = previousNextPatchTarget
		c.scopes[c.scopeIndex].redoTarget = previousRedoTarget
	case *ast.UntilExpression:
		if node.Post {
			return c.compilePostUntilExpression(node)
		}
		// until is like while with negated condition
		loopStart := len(c.currentInstructions())

		if err := c.compileCondition(node.Condition); err != nil {
			return err
		}

		// Jump out if condition is TRUE (opposite of while)
		jumpTruthyPos := c.emit(OpJumpTruthy, 9999)
		bodyStart := len(c.currentInstructions())
		previousRedoTarget := c.scopes[c.scopeIndex].redoTarget
		previousNextPatchTarget := c.scopes[c.scopeIndex].nextPatchTarget
		c.scopes[c.scopeIndex].redoTarget = bodyStart
		c.scopes[c.scopeIndex].nextPatchTarget = loopStart

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.emit(OpJump, loopStart)

		afterBody := len(c.currentInstructions())
		c.changeOperand(jumpTruthyPos, afterBody)

		// until returns nil in Ruby
		c.Emit(OpNil)
		c.scopes[c.scopeIndex].nextPatchTarget = previousNextPatchTarget
		c.scopes[c.scopeIndex].redoTarget = previousRedoTarget
	case *ast.ForExpression:
		if err := c.compileForExpression(node); err != nil {
			return err
		}
	case *ast.BreakExpression:
		if node.Value != nil {
			if splat, ok := node.Value.(*ast.SplatExpression); ok {
				if err := c.Compile(splat.Value); err != nil {
					return err
				}
				c.Emit(OpSplatToArray)
			} else if err := c.Compile(node.Value); err != nil {
				return err
			}
			pos := c.emit(OpBreakValue, 0)
			c.scopes[c.scopeIndex].breakValuePatchPos = append(c.scopes[c.scopeIndex].breakValuePatchPos, pos)
		} else {
			c.Emit(OpBreak)
		}
	case *ast.NextExpression:
		if node.Value != nil {
			if err := c.Compile(node.Value); err != nil {
				return err
			}
		} else {
			c.Emit(OpNil)
		}
		if c.forEachDepth > 0 {
			c.emit(OpNext, 0)
			return nil
		}
		pos := c.emit(OpNext, 0)
		if c.scopes[c.scopeIndex].nextPatchTarget >= 0 {
			c.changeOperand(pos, c.scopes[c.scopeIndex].nextPatchTarget)
		} else {
			c.scopes[c.scopeIndex].nextPatchPos = append(c.scopes[c.scopeIndex].nextPatchPos, pos)
		}
	case *ast.RedoExpression:
		if c.scopes[c.scopeIndex].redoTarget >= 0 {
			c.emit(OpJump, c.scopes[c.scopeIndex].redoTarget)
		} else {
			c.Emit(OpRedo)
		}
	case *ast.RetryExpression:
		if c.scopes[c.scopeIndex].retryTarget >= 0 {
			c.emit(OpJump, c.scopes[c.scopeIndex].retryTarget)
		} else {
			c.Emit(OpRetry)
		}
	case *ast.YieldExpression:
		if len(node.Args) > 0 || len(node.KeywordArgs) > 0 {
			splatIndex := -1
			for i, arg := range node.Args {
				compileArg := arg
				if splat, ok := arg.(*ast.SplatExpression); ok {
					compileArg = splat.Value
					if splat.Token.Type == lexer.MULTIPLY && splatIndex < 0 {
						splatIndex = i
					}
				}
				if err := c.Compile(compileArg); err != nil {
					return err
				}
				if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type == lexer.POW {
					c.Emit(OpMarkKeywordHash)
				}
			}
			argCount := len(node.Args)
			if len(node.KeywordArgs) > 0 {
				for i := len(node.KeywordArgs) - 1; i >= 0; i-- {
					kwa := node.KeywordArgs[i]
					if err := c.Compile(kwa.Value); err != nil {
						return err
					}
					c.EmitConstant(&object.EmeraldValue{
						Type:  object.ValueSymbol,
						Data:  normalizedStaticSymbolName(kwa.Name),
						Class: core.R.Classes["Symbol"],
					})
				}
				c.emit(OpHash, len(node.KeywordArgs))
				c.Emit(OpMarkKeywordHash)
				argCount++
			}
			if splatIndex >= 0 {
				c.emit(OpYieldWithSplat, argCount, splatIndex)
			} else {
				c.emit(OpYieldWithValue, argCount)
			}
		} else {
			c.Emit(OpYield)
		}
	case *ast.SelfExpression:
		c.Emit(OpSelf)
	case *ast.RaiseExpression:
		if node.Message != nil {
			c.Emit(OpSelf)
			if err := c.Compile(node.Error); err != nil {
				return err
			}
			if err := c.Compile(node.Message); err != nil {
				return err
			}
			op := OpSend
			if node.MessageIsKeyword {
				op = OpSendWithKeywords
			}
			c.emit(op, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "raise",
				Class: core.R.Classes["String"],
			}), 0, 2, 255)
			return nil
		}
		if node.Error != nil {
			if err := c.Compile(node.Error); err != nil {
				return err
			}
		} else {
			c.Emit(OpReraise)
			return nil
		}
		c.Emit(OpRaise)
	case *ast.ThrowExpression:
		if len(node.ExtraArgs) > 0 {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueClass,
				Data:  core.R.Classes["ArgumentError"],
				Class: core.R.Classes["Class"],
			})
			c.Emit(OpRaise)
			return nil
		}
		if node.Label != nil {
			if err := c.Compile(node.Label); err != nil {
				return err
			}
		} else {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "RuntimeError",
				Class: core.R.Classes["String"],
			})
		}
		if node.Value != nil {
			if err := c.Compile(node.Value); err != nil {
				return err
			}
		} else {
			c.Emit(OpNil)
		}
		c.Emit(OpThrow)
	case *ast.CatchExpression:
		if err := c.compileCatchExpression(node); err != nil {
			return err
		}
	case *ast.BeginExpression:
		if err := c.compileBeginExpression(node); err != nil {
			return err
		}
	case *ast.SplatExpression:
		if err := c.Compile(node.Value); err != nil {
			return err
		}
		c.Emit(OpSplat)
	case *ast.SuperExpression:
		c.Emit(OpSelf)
		args := expandForwardArguments(node.Args)
		var blockPass ast.Expression
		for i, arg := range args {
			if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type == lexer.BIT_AND {
				blockPass = splat.Value
				args = append(append([]ast.Expression(nil), args[:i]...), args[i+1:]...)
				break
			}
		}
		splatIndex := 255
		hasKeywords := false
		for i, arg := range args {
			compileArg := arg
			if splat, ok := arg.(*ast.SplatExpression); ok {
				if splat.Token.Type == lexer.MULTIPLY {
					compileArg = splat.Value
					if splatIndex == 255 {
						splatIndex = i
					}
				} else if splat.Token.Type == lexer.POW {
					compileArg = splat.Value
					hasKeywords = true
				}
			}
			if err := c.Compile(compileArg); err != nil {
				return err
			}
		}
		blockArg := 0
		if blockPass != nil {
			if err := c.Compile(blockPass); err != nil {
				return err
			}
			blockArg = 1
		} else if node.Block != nil {
			if err := c.compileBlockAsClosure(node.Block); err != nil {
				return err
			}
			blockArg = 1
		}
		if hasKeywords {
			blockArg |= 2
		}
		argCount := len(args)
		if node.ImplicitArgs {
			argCount = 255
		}
		c.emit(OpSendSuper, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__super__",
			Class: core.R.Classes["String"],
		}), blockArg, argCount, splatIndex)
	default:
		return fmt.Errorf("unknown node type: %T", node)
	}

	return nil
}

func (c *Compiler) compileDefinedExpression(node *ast.DefinedExpression) {
	if node == nil || node.Expression == nil {
		c.Emit(OpNil)
		return
	}
	if c.voidContext {
		call, explicitReceiverCall := node.Expression.(*ast.MethodCall)
		if !explicitReceiverCall || call.Receiver == nil {
			result, ok := c.definedDescription(node.Expression)
			if !ok {
				c.Emit(OpNil)
				return
			}
			c.emitString(result)
			c.Emit(OpFreeze)
			return
		}
	}
	if identifier, ok := node.Expression.(*ast.Identifier); ok {
		switch identifier.Value {
		case "self", "nil", "true", "false", "__FILE__", "__LINE__", "__ENCODING__":
			// These have static descriptions handled below.
		default:
			if symbol, found := c.symbolTable.Resolve(identifier.Value); found && symbol.Scope != ScopeBuiltin {
				c.emitString("local-variable")
				c.Emit(OpFreeze)
				return
			}
			c.Emit(OpSelf)
			nameIdx := c.addConstant(&object.EmeraldValue{
				Type: object.ValueString, Data: identifier.Value, Class: core.R.Classes["String"],
			})
			c.emit(OpDefinedMethod, nameIdx, 1)
			return
		}
	}
	if _, ok := node.Expression.(*ast.YieldExpression); ok {
		c.Emit(OpDefinedYield)
		return
	}
	if _, ok := node.Expression.(*ast.SuperExpression); ok {
		c.Emit(OpDefinedSuper)
		return
	}
	if instanceVar, ok := node.Expression.(*ast.InstanceVariable); ok {
		c.emit(OpDefinedInstanceVar, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: instanceVar.Name, Class: core.R.Classes["String"],
		}))
		return
	}
	if globalVar, ok := node.Expression.(*ast.GlobalVariable); ok {
		c.emit(OpDefinedGlobal, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: globalVar.Name, Class: core.R.Classes["String"],
		}))
		return
	}
	if classVar, ok := node.Expression.(*ast.ClassVariable); ok {
		c.emit(OpDefinedClassVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  classVar.Name,
			Class: core.R.Classes["String"],
		}))
		return
	}
	if constant, ok := node.Expression.(*ast.Constant); ok {
		c.emit(OpDefinedConstant, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: constant.Name, Class: core.R.Classes["String"],
		}))
		return
	}
	if call, ok := node.Expression.(*ast.MethodCall); ok && call.Assignment {
		c.emitString("assignment")
		c.Emit(OpFreeze)
		return
	}
	if assignment, ok := node.Expression.(*ast.AssignExpression); ok && assignment.Index != nil && assignment.Token.Type == lexer.ASSIGN {
		c.emitString("method")
		c.Emit(OpFreeze)
		return
	}
	if prefix, ok := node.Expression.(*ast.PrefixExpression); ok && (prefix.Operator == "!" || prefix.Operator == "not") {
		if c.compileDefinedNegatedVariable(prefix.Right) {
			return
		}
		_, methodCall := prefix.Right.(*ast.MethodCall)
		_, identifier := prefix.Right.(*ast.Identifier)
		if methodCall || identifier {
			c.compileDefinedGuarded(func() error {
				if err := c.Compile(prefix.Right); err != nil {
					return err
				}
				c.Emit(OpPop)
				c.emitString("method")
				c.Emit(OpFreeze)
				return nil
			})
			return
		}
	}
	if infix, ok := node.Expression.(*ast.InfixExpression); ok {
		_, methodCall := infix.Left.(*ast.MethodCall)
		_, identifier := infix.Left.(*ast.Identifier)
		if (methodCall || identifier) && infix.Operator != "&&" && infix.Operator != "and" && infix.Operator != "||" && infix.Operator != "or" {
			c.compileDefinedGuarded(func() error {
				if err := c.Compile(infix.Left); err != nil {
					return err
				}
				methodName := infix.Operator
				if methodName == "!=" {
					methodName = "=="
				}
				nameIdx := c.addConstant(&object.EmeraldValue{
					Type: object.ValueString, Data: methodName, Class: core.R.Classes["String"],
				})
				c.emit(OpDefinedMethod, nameIdx, 0)
				return nil
			})
			return
		}
	}
	if infix, ok := node.Expression.(*ast.InfixExpression); ok && (infix.Operator == "==" || infix.Operator == "!=" || infix.Operator == "!~") {
		if identifier, ok := infix.Left.(*ast.Identifier); ok {
			if symbol, found := c.symbolTable.Resolve(identifier.Value); found && symbol.Scope != ScopeBuiltin {
				if err := c.Compile(identifier); err != nil {
					c.Emit(OpNil)
					return
				}
				methodName := infix.Operator
				if methodName == "!=" {
					methodName = "=="
				}
				nameIdx := c.addConstant(&object.EmeraldValue{
					Type: object.ValueString, Data: methodName, Class: core.R.Classes["String"],
				})
				c.emit(OpDefinedMethod, nameIdx, 0)
				return
			}
			c.Emit(OpNil)
			return
		}
	}
	if call, ok := node.Expression.(*ast.MethodCall); ok && call.Method != nil {
		nameIdx := c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: call.Method.Value, Class: core.R.Classes["String"],
		})
		if call.Receiver != nil {
			c.compileDefinedGuarded(func() error {
				if err := c.Compile(call.Receiver); err != nil {
					return err
				}
				c.emit(OpDefinedMethod, nameIdx, 0)
				return nil
			})
			return
		}
		c.Emit(OpSelf)
		c.emit(OpDefinedMethod, nameIdx, 1)
		return
	}
	if resolution, ok := node.Expression.(*ast.ConstantResolution); ok {
		if resolution.Left != nil {
			if err := c.Compile(resolution.Left); err != nil {
				c.Emit(OpNil)
				return
			}
		} else {
			c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "Object",
				Class: core.R.Classes["String"],
			}))
		}
		c.emit(OpDefined, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  resolution.Name.Value,
			Class: core.R.Classes["String"],
		}))
		return
	}
	result, ok := c.definedDescription(node.Expression)
	if !ok {
		c.Emit(OpNil)
		return
	}
	c.emitString(result)
	c.Emit(OpFreeze)
}

func (c *Compiler) compileDefinedNegatedVariable(exp ast.Expression) bool {
	switch value := exp.(type) {
	case *ast.InstanceVariable:
		c.emit(OpDefinedInstanceVar, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: value.Name, Class: core.R.Classes["String"],
		}))
	case *ast.GlobalVariable:
		c.emit(OpDefinedGlobal, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: value.Name, Class: core.R.Classes["String"],
		}))
	case *ast.ClassVariable:
		c.emit(OpDefinedClassVar, c.addConstant(&object.EmeraldValue{
			Type: object.ValueString, Data: value.Name, Class: core.R.Classes["String"],
		}))
	default:
		return false
	}
	c.Emit(OpDup)
	defined := c.emit(OpJumpNotNil, 0)
	done := c.emit(OpJump, 0)
	c.changeOperand(defined, len(c.currentInstructions()))
	c.Emit(OpPop)
	c.emitString("expression")
	c.Emit(OpFreeze)
	c.changeOperand(done, len(c.currentInstructions()))
	return true
}

func (c *Compiler) compileDefinedGuarded(body func() error) {
	beginPos := c.emit(OpBeginRescue, 0, 0, 0, 0)
	if err := body(); err != nil {
		c.Emit(OpNil)
	}
	jumpToEnd := c.emit(OpJump, 0)

	rescueStart := len(c.currentInstructions())
	c.emit(OpRescueMatch, 0, 0)
	jumpReraise := c.emit(OpJumpNotTruthy, 0)
	c.Emit(OpRescue)
	c.Emit(OpPop)
	c.Emit(OpNil)
	jumpRescueEnd := c.emit(OpJump, 0)

	reraiseStart := len(c.currentInstructions())
	c.Emit(OpReraise)
	endRescueStart := len(c.currentInstructions())
	c.Emit(OpEndRescue)
	endStart := len(c.currentInstructions())

	c.changeOperand(jumpToEnd, endStart)
	c.changeOperand(jumpReraise, reraiseStart)
	c.changeOperand(jumpRescueEnd, endRescueStart)
	c.changeOperandAt(beginPos, 0, rescueStart)
	c.changeOperandAt(beginPos, 1, endRescueStart)
	c.changeOperandAt(beginPos, 2, endStart)
	c.changeOperandAt(beginPos, 3, 0)
}

func (c *Compiler) definedDescription(exp ast.Expression) (string, bool) {
	switch node := exp.(type) {
	case *ast.SelfExpression:
		return "self", true
	case *ast.Identifier:
		switch node.Value {
		case "self":
			return "self", true
		case "nil":
			return "nil", true
		case "true":
			return "true", true
		case "false":
			return "false", true
		case "__FILE__", "__LINE__", "__ENCODING__":
			return "expression", true
		}
		sym, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			return "", false
		}
		if sym.Scope == ScopeBuiltin {
			return "method", true
		}
		return "local-variable", true
	case *ast.NilExpression:
		return "nil", true
	case *ast.Boolean:
		if node.Value {
			return "true", true
		}
		return "false", true
	case *ast.AssignExpression, *ast.MultiAssignExpression:
		return "assignment", true
	case *ast.Constant:
		if _, ok := core.R.Classes[node.Name]; ok {
			return "constant", true
		}
		return "", false
	case *ast.ConstantResolution:
		return "constant", true
	case *ast.MethodCall:
		if node.Receiver == nil {
			if _, ok := c.symbolTable.Resolve(node.Method.Value); ok {
				return "method", true
			}
			return "", false
		}
		if _, ok := c.definedDescription(node.Receiver); !ok {
			return "", false
		}
		return "method", true
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			if _, ok := c.definedDescription(element); !ok {
				return "", false
			}
		}
		return "expression", true
	case *ast.HashLiteral:
		for _, key := range node.Order {
			if _, ok := c.definedDescription(key); !ok {
				return "", false
			}
			if _, ok := c.definedDescription(node.Pairs[key]); !ok {
				return "", false
			}
		}
		return "expression", true
	default:
		return "expression", true
	}
}

func (c *Compiler) emitString(value string) {
	c.EmitConstant(&object.EmeraldValue{
		Type:  object.ValueString,
		Data:  value,
		Class: core.R.Classes["String"],
	})
}

func (c *Compiler) definePatternLocals(pattern string) {
	for i := 0; i < len(pattern); {
		ch := pattern[i]
		if !((ch >= 'a' && ch <= 'z') || ch == '_') {
			i++
			continue
		}
		start := i
		i++
		for i < len(pattern) {
			ch = pattern[i]
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
				break
			}
			i++
		}
		name := pattern[start:i]
		switch name {
		case "_", "true", "false", "nil", "if", "unless", "then", "in":
			continue
		}
		if _, ok := c.symbolTable.Resolve(name); !ok {
			c.symbolTable.Define(name)
		}
	}
}

func (c *Compiler) Bytecode() *Bytecode {
	return &Bytecode{
		Instructions: c.currentInstructions(),
		LineMap:      c.currentLineMapCopy(),
		Constants:    c.constants,
		NumLocals:    c.symbolTable.MaxSymbols,
		GlobalNames:  c.globalNamesCopy(),
		LocalNames:   c.localNames(),
	}
}

func (c *Compiler) globalNamesCopy() map[string]int {
	root := c.symbolTable
	for root.Outer != nil {
		root = root.Outer
	}
	globalNames := make(map[string]int)
	for name, symbol := range root.store {
		if symbol.Scope == ScopeGlobal {
			globalNames[name] = symbol.Index
		}
	}
	return globalNames
}

func (c *Compiler) localNames() map[string]int {
	localNames := make(map[string]int)
	for name, symbol := range c.symbolTable.store {
		if symbol.Scope == ScopeLocal {
			localNames[name] = symbol.Index
		}
	}
	return localNames
}

func freeVarNames(free []Symbol) []string {
	names := make([]string, len(free))
	for i, s := range free {
		names[i] = s.Name
	}
	return names
}

func (c *Compiler) currentInstructions() Instructions {
	return c.scopes[c.scopeIndex].instructions
}

// compileBlockAsValue compiles a BlockExpression.
// For blocks with params, this is called within an EnterScope/LeavaScope pair
// so the block body's instructions are in the block scope.
// For blocks without params, the statements are compiled inline in the parent scope.
func (c *Compiler) compileBlockAsClosure(block *ast.BlockExpression) error {
	return c.compileBlockAsClosureWithLocalNames(block, nil)
}

func (c *Compiler) compileBlockAsClosureWithLocalNames(block *ast.BlockExpression, localNameOverrides map[string]int) error {
	return c.compileBlockAsClosureWithLocalNamesInternal(block, localNameOverrides, false)
}

func (c *Compiler) compileBlockAsClosureWithLocalNamesInternal(block *ast.BlockExpression, localNameOverrides map[string]int, forLoopCollectAsPair bool) error {
	numberedParamCount := blockNumberedParameterCount(block)
	implicitIt := numberedParamCount == 0 && c.blockUsesImplicitItParameter(block)
	c.EnterScope()
	if implicitIt {
		c.implicitIt[c.symbolTable] = true
	}

	params := block.Params
	paramDefaults := block.ParamDefaults
	if numberedParamCount > 0 && len(params) == 0 && block.RestParam == nil && block.BlockParam == nil && len(block.KeywordParams) == 0 {
		params = numberedParameterIdentifiers(numberedParamCount)
		paramDefaults = make([]ast.Expression, numberedParamCount)
	} else if implicitIt {
		params = []*ast.Identifier{{Value: "it"}}
		paramDefaults = []ast.Expression{nil}
	}
	restIndex := block.RestParamIndex
	if restIndex < 0 || restIndex > len(params) {
		restIndex = len(params)
	}
	paramLocalIndices := make([]int, len(params))
	for i, param := range params[:restIndex] {
		paramLocalIndices[i] = c.symbolTable.DefineParameter(param.Value).Index
	}
	anonymousRestParam := block.RestParam != nil && block.RestParam.Value == "_" && block.RestParam.Token.Literal == "*"
	if block.RestParam != nil && !anonymousRestParam {
		c.symbolTable.Define(block.RestParam.Value)
	}
	for i, param := range params[restIndex:] {
		paramLocalIndices[restIndex+i] = c.symbolTable.DefineParameter(param.Value).Index
	}
	if paramsMatchAST(params, block.Params) {
		c.defineParameterPatternLocals(block.ParamPatterns)
	}
	blockParamIndex := -1
	if block.BlockParam != nil {
		blockParamIndex = c.symbolTable.Define(block.BlockParam.Value).Index
	}
	for _, kp := range block.KeywordParams {
		c.symbolTable.Define(kp.Name)
	}
	if block.KeywordRestParam != nil {
		c.symbolTable.Define(block.KeywordRestParam.Value)
	}
	for _, localName := range block.BlockLocals {
		c.symbolTable.Define(localName)
	}
	for i, defaultExpr := range paramDefaults {
		if i < len(params) {
			if err := c.compileParameterDefault(params[i].Value, defaultExpr); err != nil {
				return err
			}
		}
	}
	for _, keyword := range block.KeywordParams {
		if err := c.compileParameterDefault(keyword.Name, keyword.Default); err != nil {
			return err
		}
	}

	if err := c.compileBlockAsValue(block); err != nil {
		return err
	}

	c.replaceLastPopWithBlockReturn()

	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	localNames := c.localNames()
	maxIndex := -1
	for _, idx := range localNames {
		if idx > maxIndex {
			maxIndex = idx
		}
	}
	for name, idx := range localNameOverrides {
		if idx < 0 {
			maxIndex++
			idx = maxIndex
		} else if idx > maxIndex {
			maxIndex = idx
		}
		localNames[name] = idx
	}
	if maxIndex+1 > numLocals {
		numLocals = maxIndex + 1
	}

	lineMap := c.currentLineMapCopy()
	instructions := c.LeaveScope()

	compiledParamDefaults := make([]*object.EmeraldValue, len(params))
	for i, defaultExpr := range paramDefaults {
		if i >= len(compiledParamDefaults) {
			break
		}
		if defaultExpr != nil {
			compiledParamDefaults[i] = c.compileDefaultValue(defaultExpr)
		}
	}
	kwParams := make([]object.KeywordParamInfo, len(block.KeywordParams))
	for i, kp := range block.KeywordParams {
		info := object.KeywordParamInfo{
			Name:       kp.Name,
			HasDefault: kp.Default != nil,
		}
		if kp.Default != nil {
			info.Default = c.compileDefaultValue(kp.Default)
		}
		kwParams[i] = info
	}

	fnObj := &object.Function{
		Name:                  "__block__",
		SourcePath:            core.CurrentSpecFile,
		EvalSource:            core.CurrentEvalSource,
		SourceEncoding:        c.sourceEncoding,
		DefinitionLine:        int64(block.Token.Line),
		Params:                identifierNames(params),
		ParamLocalIndices:     paramLocalIndices,
		ParamPatterns:         compileParameterPatternsIfAligned(block.ParamPatterns, params, block.Params),
		ParamDefaults:         compiledParamDefaults,
		EvaluateParamDefaults: true,
		BlockLocals:           append([]string(nil), block.BlockLocals...),
		Instructions:          instructions,
		LineMap:               lineMap,
		NumLocals:             numLocals,
		GlobalNames:           c.globalNamesCopy(),
		LocalNames:            localNames,
		KeywordParams:         kwParams,
		RejectKeywords:        block.RejectKeywords,
		SingleDestructure:     block.SingleDestructure,
		KeywordRestOnly:       block.KeywordRestOnly,
		RejectBlock:           block.RejectBlock,
		FreeVarNames:          freeVarNames(free),
		TrailingCommaParam:    block.TrailingComma,
		ForLoopCollectAsPair:  forLoopCollectAsPair,
		ImplicitItParameter:   implicitIt,
		NumberedParameters:    numberedParamCount > 0,
	}
	if block.RestParam != nil {
		fnObj.HasRestParam = true
		fnObj.AnonymousRestParam = anonymousRestParam
		fnObj.RestParamIndex = restIndex
		if !anonymousRestParam {
			fnObj.RestParamName = block.RestParam.Value
		}
	}
	if block.KeywordRestParam != nil {
		fnObj.KeywordRestParam = block.KeywordRestParam.Value
	}
	if block.BlockParam != nil {
		fnObj.HasBlockParam = true
		fnObj.AnonymousBlockParam = block.BlockParam.Value == "_" && block.BlockParam.Token.Literal == "&"
		fnObj.BlockParamIndex = blockParamIndex
	}

	fn := &object.EmeraldValue{
		Type:  object.ValueFunction,
		Data:  fnObj,
		Class: core.R.Classes["Class"],
	}

	fnIdx := c.addConstant(fn)

	for _, s := range free {
		c.emitCaptureSymbol(s)
	}
	c.emit(OpClosure, fnIdx, len(free))
	return nil
}

func (c *Compiler) compileBlockAsValue(block *ast.BlockExpression) error {
	if block == nil || len(block.Statements) == 0 {
		c.Emit(OpNil)
		return nil
	}
	scopeIndex := c.scopeIndex
	c.scopes[scopeIndex].nextPatchDepth++
	defer func() {
		c.scopes[scopeIndex].nextPatchDepth--
	}()
	lastExpressionDirect := false
	for i, s := range block.Statements {
		if i == len(block.Statements)-1 {
			if exprStmt, ok := s.(*ast.ExpressionStatement); ok {
				if err := c.Compile(exprStmt.Expression); err != nil {
					return err
				}
				lastExpressionDirect = true
				break
			}
		}
		previousVoidContext := c.voidContext
		if expressionStatement, ok := s.(*ast.ExpressionStatement); ok {
			_, c.voidContext = expressionStatement.Expression.(*ast.DefinedExpression)
		}
		if err := c.Compile(s); err != nil {
			c.voidContext = previousVoidContext
			return err
		}
		c.voidContext = previousVoidContext
	}
	if !lastExpressionDirect {
		c.removeLastPop()
	}
	endPos := len(c.currentInstructions())
	scope := &c.scopes[c.scopeIndex]
	targetPos := endPos
	if scope.nextPatchTarget >= 0 {
		targetPos = scope.nextPatchTarget
	}
	if scope.nextPatchDepth == 1 {
		for _, patchPos := range scope.nextPatchPos {
			c.changeOperand(patchPos, targetPos)
		}
		scope.nextPatchPos = []int{}
	}
	return nil
}

func (c *Compiler) blockUsesImplicitItParameter(block *ast.BlockExpression) bool {
	if block == nil || len(block.Params) > 0 || block.RestParam != nil || block.BlockParam != nil || len(block.KeywordParams) > 0 {
		return false
	}
	if c.symbolTableHasLexicalBinding("it") {
		return false
	}
	return statementsUseBareIt(block.Statements)
}

func numberedParameterIdentifiers(count int) []*ast.Identifier {
	params := make([]*ast.Identifier, count)
	for i := range params {
		params[i] = &ast.Identifier{Value: fmt.Sprintf("_%d", i+1)}
	}
	return params
}

func blockNumberedParameterCount(block *ast.BlockExpression) int {
	if block == nil {
		return 0
	}
	max := 0
	for _, stmt := range block.Statements {
		switch node := stmt.(type) {
		case *ast.ExpressionStatement:
			max = maxInt(max, expressionNumberedParameterCount(node.Expression))
		case ast.Expression:
			max = maxInt(max, expressionNumberedParameterCount(node))
		}
	}
	return max
}

func expressionNumberedParameterCount(expr ast.Expression) int {
	switch node := expr.(type) {
	case nil:
		return 0
	case *ast.Identifier:
		if len(node.Value) == 2 && node.Value[0] == '_' && node.Value[1] >= '1' && node.Value[1] <= '9' {
			return int(node.Value[1] - '0')
		}
	case *ast.MethodCall:
		max := expressionNumberedParameterCount(node.Receiver)
		if node.Receiver == nil && node.Method != nil {
			max = maxInt(max, expressionNumberedParameterCount(node.Method))
		}
		for _, arg := range node.Args {
			max = maxInt(max, expressionNumberedParameterCount(arg))
		}
		for _, arg := range node.KeywordArgs {
			max = maxInt(max, expressionNumberedParameterCount(arg.Value))
		}
		return max
	case *ast.AssignExpression:
		return maxInt(expressionNumberedParameterCount(node.Target), maxInt(expressionNumberedParameterCount(node.Index), expressionNumberedParameterCount(node.Value)))
	case *ast.InfixExpression:
		return maxInt(expressionNumberedParameterCount(node.Left), expressionNumberedParameterCount(node.Right))
	case *ast.PrefixExpression:
		return expressionNumberedParameterCount(node.Right)
	case *ast.ArrayLiteral:
		max := 0
		for _, elem := range node.Elements {
			max = maxInt(max, expressionNumberedParameterCount(elem))
		}
		return max
	case *ast.HashLiteral:
		max := 0
		for _, key := range node.Order {
			max = maxInt(max, expressionNumberedParameterCount(key))
			max = maxInt(max, expressionNumberedParameterCount(node.Pairs[key]))
		}
		return max
	case *ast.IfExpression:
		return maxInt(expressionNumberedParameterCount(node.Condition), maxInt(blockNumberedParameterCount(node.Consequent), blockNumberedParameterCount(node.Alternative)))
	case *ast.BeginExpression:
		return maxInt(blockNumberedParameterCount(node.Body), maxInt(blockNumberedParameterCount(node.Else), blockNumberedParameterCount(node.Ensure)))
	case *ast.TernaryExpression:
		return maxInt(expressionNumberedParameterCount(node.Condition), maxInt(expressionNumberedParameterCount(node.Consequent), expressionNumberedParameterCount(node.Alternative)))
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (c *Compiler) procLiteralUsesImplicitItParameter(node *ast.ProcLiteral) bool {
	if node == nil || len(node.Params) > 0 || node.RestParam != nil || node.BlockParam != nil {
		return false
	}
	if c.symbolTableHasLexicalBinding("it") {
		return false
	}
	if node.Body == nil {
		return false
	}
	return statementsUseBareIt(node.Body.Statements)
}

func (c *Compiler) symbolTableHasLexicalBinding(name string) bool {
	for table := c.symbolTable; table != nil; table = table.Outer {
		if name == "it" && c.implicitIt[table] {
			continue
		}
		if _, ok := table.store[name]; ok {
			return true
		}
	}
	return false
}

func statementsUseBareIt(stmts []ast.Statement) bool {
	for _, stmt := range stmts {
		switch node := stmt.(type) {
		case *ast.ExpressionStatement:
			if expressionUsesBareIt(node.Expression) {
				return true
			}
		case ast.Expression:
			if expressionUsesBareIt(node) {
				return true
			}
		}
	}
	return false
}

func expressionUsesBareIt(expr ast.Expression) bool {
	switch node := expr.(type) {
	case nil:
		return false
	case *ast.Identifier:
		return node.Value == "it"
	case *ast.MethodCall:
		if node.Receiver == nil && node.Method != nil && node.Method.Value == "it" && len(node.Args) == 0 && len(node.KeywordArgs) == 0 && node.Block == nil {
			return true
		}
		if expressionUsesBareIt(node.Receiver) {
			return true
		}
		for _, arg := range node.Args {
			if expressionUsesBareIt(arg) {
				return true
			}
		}
		for _, arg := range node.KeywordArgs {
			if expressionUsesBareIt(arg.Value) {
				return true
			}
		}
		return false
	case *ast.AssignExpression:
		return expressionUsesBareIt(node.Target) || expressionUsesBareIt(node.Index) || expressionUsesBareIt(node.Value)
	case *ast.InfixExpression:
		return expressionUsesBareIt(node.Left) || expressionUsesBareIt(node.Right)
	case *ast.PrefixExpression:
		return expressionUsesBareIt(node.Right)
	case *ast.ArrayLiteral:
		for _, elem := range node.Elements {
			if expressionUsesBareIt(elem) {
				return true
			}
		}
	case *ast.HashLiteral:
		for _, key := range node.Order {
			if expressionUsesBareIt(key) || expressionUsesBareIt(node.Pairs[key]) {
				return true
			}
		}
	case *ast.IfExpression:
		return expressionUsesBareIt(node.Condition) || statementsUseBareIt(blockStatements(node.Consequent)) || statementsUseBareIt(blockStatements(node.Alternative))
	case *ast.BeginExpression:
		return statementsUseBareIt(blockStatements(node.Body)) || statementsUseBareIt(blockStatements(node.Else)) || statementsUseBareIt(blockStatements(node.Ensure))
	case *ast.TernaryExpression:
		return expressionUsesBareIt(node.Condition) || expressionUsesBareIt(node.Consequent) || expressionUsesBareIt(node.Alternative)
	}
	return false
}

func blockStatements(block *ast.BlockExpression) []ast.Statement {
	if block == nil {
		return nil
	}
	return block.Statements
}

func (c *Compiler) compileBeginExpression(node *ast.BeginExpression) error {
	hasRescue := len(node.Rescue) > 0
	hasElse := node.Else != nil
	hasEnsure := node.Ensure != nil

	if !hasRescue && !hasElse && !hasEnsure {
		return c.compileBlockAsValue(node.Body)
	}

	beginPos := c.emit(OpBeginRescue, 0, 0, 0, 0)

	if err := c.compileBlockAsValue(node.Body); err != nil {
		return err
	}

	jumpToEnd := c.emit(OpJump, 0)

	rescueStart := 0
	rescueOffsets := make([]int, len(node.Rescue))
	var pendingNoMatchJump int
	rescueEndJumps := []int{}
	for i, rescue := range node.Rescue {
		rescueOffsets[i] = len(c.currentInstructions())
		if pendingNoMatchJump > 0 {
			c.changeOperand(pendingNoMatchJump, rescueOffsets[i])
		}
		if i == 0 {
			rescueStart = rescueOffsets[i]
		}

		splatMask := 0
		for excIndex, exc := range rescue.Exceptions {
			if splat, ok := exc.(*ast.SplatExpression); ok {
				splatMask |= 1 << excIndex
				if err := c.Compile(splat.Value); err != nil {
					return err
				}
				continue
			}
			if err := c.Compile(exc); err != nil {
				return err
			}
		}
		c.emit(OpRescueMatch, len(rescue.Exceptions), splatMask)
		pendingNoMatchJump = c.emit(OpJumpNotTruthy, 0)
		c.Emit(OpRescue)
		if rescue.Variable != nil {
			if _, ok := c.symbolTable.Resolve(rescue.Variable.Value); !ok {
				c.symbolTable.Define(rescue.Variable.Value)
			}
			sym, _ := c.symbolTable.Resolve(rescue.Variable.Value)
			switch sym.Scope {
			case ScopeGlobal:
				c.emit(OpSetGlobal, sym.Index)
			case ScopeLocal:
				c.emit(OpSetLocal, sym.Index)
			case ScopeFree:
				c.emit(OpSetFree, sym.Index)
			}
			if rescue.Target != nil {
				if err := c.Compile(rescue.Target); err != nil {
					return err
				}
				c.Emit(OpPop)
			}
		} else {
			c.Emit(OpPop)
		}

		if err := c.compileBlockAsValue(rescue.Body); err != nil {
			return err
		}

		rescueEndJumps = append(rescueEndJumps, c.emit(OpJump, 0))
	}

	unmatchedReraiseStart := 0
	if pendingNoMatchJump > 0 {
		unmatchedReraiseStart = len(c.currentInstructions())
		c.Emit(OpReraise)
	}

	elseStart := 0
	if hasElse {
		elseStart = len(c.currentInstructions())
		if err := c.compileBlockAsValue(node.Else); err != nil {
			return err
		}
	}

	rescuedEnsureStart := len(c.currentInstructions())
	if hasRescue && hasEnsure {
		c.Emit(OpEndRescue)
	}
	ensureStart := len(c.currentInstructions())
	ensureEnd := 0
	if hasEnsure {
		c.Emit(OpEnsure)
		if err := c.compileBlockAsValue(node.Ensure); err != nil {
			return err
		}
		c.Emit(OpPop)
		c.Emit(OpEndEnsure)
		ensureEnd = len(c.currentInstructions())
	}

	endRescueStart := len(c.currentInstructions())
	if hasRescue {
		c.Emit(OpEndRescue)
	}
	endStart := len(c.currentInstructions())

	if hasElse {
		c.changeOperand(jumpToEnd, elseStart)
	} else if hasEnsure {
		c.changeOperand(jumpToEnd, ensureStart)
	} else {
		c.changeOperand(jumpToEnd, endStart)
	}
	if pendingNoMatchJump > 0 {
		c.changeOperand(pendingNoMatchJump, unmatchedReraiseStart)
	}
	for _, jump := range rescueEndJumps {
		if hasEnsure {
			c.changeOperand(jump, rescuedEnsureStart)
		} else {
			c.changeOperand(jump, endRescueStart)
		}
	}
	c.changeOperandAt(beginPos, 0, rescueStart)
	c.changeOperandAt(beginPos, 1, ensureStart)
	c.changeOperandAt(beginPos, 2, endStart)
	c.changeOperandAt(beginPos, 3, ensureEnd)

	return nil
}

func (c *Compiler) compileCatchExpression(node *ast.CatchExpression) error {
	if !node.HasBlock {
		c.EmitConstant(core.NewLocalJumpError("no block given"))
		c.Emit(OpRaise)
		return nil
	}
	if node.Label != nil {
		if err := c.Compile(node.Label); err != nil {
			return err
		}
	}

	c.emit(OpCatch, 0)
	afterBody := len(c.currentInstructions())
	c.changeOperand(afterBody-3, afterBody)

	if err := c.compileBlockAsValue(node.Body); err != nil {
		return err
	}

	endPos := len(c.currentInstructions())
	c.changeOperand(afterBody-3, endPos)

	return nil
}

func (c *Compiler) emit(op Opcode, operands ...int) int {
	ins := Make(op, operands...)
	pos := c.addInstruction(ins)
	c.setLastInstruction(op, pos)
	return pos
}

func (c *Compiler) Emit(op Opcode) int {
	return c.emit(op)
}

func (c *Compiler) EmitConstant(v *object.EmeraldValue) int {
	return c.emit(OpConstant, c.addConstant(v))
}

func (c *Compiler) addConstant(v *object.EmeraldValue) int {
	c.constants = append(c.constants, v)
	return len(c.constants) - 1
}

func (c *Compiler) addInstruction(ins Instructions) int {
	pos := len(c.currentInstructions())
	updated := append(c.currentInstructions(), ins...)
	c.scopes[c.scopeIndex].instructions = updated
	if c.currentLine > 0 {
		if c.scopes[c.scopeIndex].lineMap == nil {
			c.scopes[c.scopeIndex].lineMap = map[int]int{}
		}
		c.scopes[c.scopeIndex].lineMap[pos] = c.currentLine
	}
	return pos
}

func (c *Compiler) setLastInstruction(op Opcode, pos int) {
	prev := c.scopes[c.scopeIndex].lastInstruction
	c.scopes[c.scopeIndex].previousInstruction = prev
	c.scopes[c.scopeIndex].lastInstruction = EmittedInstruction{Opcode: op, Position: pos}
}

func (c *Compiler) replaceLastPopWithReturn() {
	last := c.scopes[c.scopeIndex].lastInstruction
	if last.Opcode == OpPop {
		c.scopes[c.scopeIndex].instructions[last.Position] = byte(OpReturnValue)
		c.scopes[c.scopeIndex].lastInstruction.Opcode = OpReturnValue
		return
	}
	if last.Opcode != OpReturnValue {
		c.Emit(OpReturnValue)
	}
}

func (c *Compiler) replaceOpcodes(from, to Opcode) {
	instructions := c.currentInstructions()
	for pos := 0; pos < len(instructions); {
		op := Opcode(instructions[pos])
		definition, ok := Lookup(byte(op))
		if !ok {
			pos++
			continue
		}
		if op == from {
			instructions[pos] = byte(to)
			if last := &c.scopes[c.scopeIndex].lastInstruction; last.Position == pos && last.Opcode == from {
				last.Opcode = to
			}
			if previous := &c.scopes[c.scopeIndex].previousInstruction; previous.Position == pos && previous.Opcode == from {
				previous.Opcode = to
			}
		}
		pos++
		for _, width := range definition.OperandWidths {
			pos += width
		}
	}
}

func (c *Compiler) replaceLastPopWithBlockReturn() {
	last := c.scopes[c.scopeIndex].lastInstruction
	if last.Opcode == OpPop {
		c.scopes[c.scopeIndex].instructions[last.Position] = byte(OpBlockReturn)
		c.scopes[c.scopeIndex].lastInstruction.Opcode = OpBlockReturn
		return
	}
	if last.Opcode != OpBlockReturn && last.Opcode != OpReturnValue {
		c.Emit(OpBlockReturn)
	}
}

func (c *Compiler) removeLastPop() {
	last := c.scopes[c.scopeIndex].lastInstruction
	if last.Opcode != OpPop {
		return
	}
	c.scopes[c.scopeIndex].instructions = c.scopes[c.scopeIndex].instructions[:last.Position]
	delete(c.scopes[c.scopeIndex].lineMap, last.Position)
	c.scopes[c.scopeIndex].lastInstruction = c.scopes[c.scopeIndex].previousInstruction
	c.scopes[c.scopeIndex].previousInstruction = EmittedInstruction{}
}

func (c *Compiler) changeOperand(opPos int, operand int) {
	op := c.currentInstructions()[opPos]
	def, _ := Lookup(byte(op))
	read := 0

	for _, w := range def.OperandWidths {
		if w == 2 {
			c.currentInstructions()[opPos+1+read] = byte(operand >> 8)
			c.currentInstructions()[opPos+2+read] = byte(operand)
		}
		read += w
	}
}

func (c *Compiler) changeOperandAt(opPos int, operandIndex int, operand int) {
	op := c.currentInstructions()[opPos]
	def, _ := Lookup(byte(op))
	offset := 1
	for i, width := range def.OperandWidths {
		if i == operandIndex {
			if width == 2 {
				c.currentInstructions()[opPos+offset] = byte(operand >> 8)
				c.currentInstructions()[opPos+offset+1] = byte(operand)
			} else if width == 1 {
				c.currentInstructions()[opPos+offset] = byte(operand)
			}
			return
		}
		offset += width
	}
}

func normalizedStaticSymbolName(value string) string {
	if len(value) > 0 && value[0] == ':' {
		value = value[1:]
	}
	value = strings.ReplaceAll(value, lexer.EscapedHashInterpolation, "#")
	if len(value) < 3 || !strings.HasPrefix(value, "#{") || !strings.HasSuffix(value, "}") {
		return value
	}
	if resolved, ok := staticQuotedInterpolationValue(value[2 : len(value)-1]); ok {
		return resolved
	}
	return value
}

type namedRegexpCaptureBinding struct {
	captureIndex int
	symbol       Symbol
}

func (c *Compiler) compileCondition(condition ast.Expression) error {
	if flipFlop, ok := condition.(*ast.RangeExpression); ok && !flipFlop.StartMissing && !flipFlop.EndMissing {
		return c.compileFlipFlopCondition(flipFlop)
	}
	if logical, ok := condition.(*ast.InfixExpression); ok && (logical.Operator == "&&" || logical.Operator == "and" || logical.Operator == "||" || logical.Operator == "or") {
		if err := c.compileCondition(logical.Left); err != nil {
			return err
		}
		c.Emit(OpDup)
		jumpOp := OpJumpNotTruthy
		if logical.Operator == "||" || logical.Operator == "or" {
			jumpOp = OpJumpTruthy
		}
		jumpPos := c.emit(jumpOp, 9999)
		c.Emit(OpPop)
		if err := c.compileCondition(logical.Right); err != nil {
			return err
		}
		c.changeOperand(jumpPos, len(c.currentInstructions()))
		return nil
	}
	if _, ok := condition.(*ast.RegexpLiteral); !ok {
		return c.Compile(condition)
	}
	fmt.Fprintln(os.Stderr, "warning: regex literal in condition")
	if err := c.Compile(condition); err != nil {
		return err
	}
	c.emit(OpGetGlobal, c.globalSymbolIndex("$_"))
	methodNameIdx := c.addConstant(&object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "=~",
		Class: core.R.Classes["String"],
	})
	c.emit(OpSend, methodNameIdx, 0, 1, 255)
	return nil
}

func (c *Compiler) compileFlipFlopCondition(node *ast.RangeExpression) error {
	stateID := c.tempCounter
	c.tempCounter++

	c.emit(OpFlipFlopGet, stateID)
	jumpActive := c.emit(OpJumpTruthy, 9999)
	if err := c.compileFlipFlopEndpoint(node.Left); err != nil {
		return err
	}
	jumpFalse := c.emit(OpJumpNotTruthy, 9999)
	c.emit(OpFlipFlopSet, stateID, 1)
	if node.Exclusive {
		c.Emit(OpTrue)
		jumpEnd := c.emit(OpJump, 9999)
		activePos := len(c.currentInstructions())
		c.changeOperand(jumpActive, activePos)
		if err := c.compileFlipFlopEndpoint(node.Right); err != nil {
			return err
		}
		jumpKeepActive := c.emit(OpJumpNotTruthy, 9999)
		c.emit(OpFlipFlopSet, stateID, 0)
		truePos := len(c.currentInstructions())
		c.changeOperand(jumpKeepActive, truePos)
		c.Emit(OpTrue)
		jumpAfterActive := c.emit(OpJump, 9999)
		falsePos := len(c.currentInstructions())
		c.changeOperand(jumpFalse, falsePos)
		c.Emit(OpFalse)
		endPos := len(c.currentInstructions())
		c.changeOperand(jumpEnd, endPos)
		c.changeOperand(jumpAfterActive, endPos)
		return nil
	}

	activePos := len(c.currentInstructions())
	c.changeOperand(jumpActive, activePos)
	if err := c.compileFlipFlopEndpoint(node.Right); err != nil {
		return err
	}
	jumpKeepActive := c.emit(OpJumpNotTruthy, 9999)
	c.emit(OpFlipFlopSet, stateID, 0)
	truePos := len(c.currentInstructions())
	c.changeOperand(jumpKeepActive, truePos)
	c.Emit(OpTrue)
	jumpEnd := c.emit(OpJump, 9999)
	falsePos := len(c.currentInstructions())
	c.changeOperand(jumpFalse, falsePos)
	c.Emit(OpFalse)
	endPos := len(c.currentInstructions())
	c.changeOperand(jumpEnd, endPos)
	return nil
}

func (c *Compiler) compileFlipFlopEndpoint(expression ast.Expression) error {
	if _, ok := expression.(*ast.IntegerLiteral); ok {
		fmt.Fprintln(os.Stderr, "warning: integer literal in flip-flop")
		c.emit(OpGetGlobal, c.globalSymbolIndex("$."))
		if err := c.Compile(expression); err != nil {
			return err
		}
		c.Emit(OpEqual)
		return nil
	}
	return c.compileCondition(expression)
}

func (c *Compiler) prepareNamedRegexpCaptureBindings(node *ast.InfixExpression) []namedRegexpCaptureBinding {
	if node == nil || node.Operator != "=~" {
		return nil
	}
	literal, ok := node.Left.(*ast.RegexpLiteral)
	if !ok {
		return nil
	}
	captures := regexpNamedCaptures(literal.Pattern)
	bindings := make([]namedRegexpCaptureBinding, 0, len(captures))
	for _, capture := range captures {
		symbol, found := c.symbolTable.Resolve(capture.name)
		if !found || symbol.Scope == ScopeBuiltin {
			symbol = c.symbolTable.Define(capture.name)
		}
		bindings = append(bindings, namedRegexpCaptureBinding{captureIndex: capture.index, symbol: symbol})
	}
	return bindings
}

func (c *Compiler) emitNamedRegexpCaptureBindings(bindings []namedRegexpCaptureBinding) {
	for _, binding := range bindings {
		c.emit(OpGetMatchCapture, binding.captureIndex)
		switch binding.symbol.Scope {
		case ScopeLocal:
			c.emit(OpSetLocal, binding.symbol.Index)
		case ScopeFree:
			c.emit(OpSetFree, binding.symbol.Index)
		case ScopeOuter:
			c.emit(OpSetOuter, 0, binding.symbol.ScopeIndex)
		case ScopeOuterFree:
			c.emit(OpSetOuterFree, binding.symbol.ScopeIndex)
		}
		c.Emit(OpPop)
	}
}

type regexpNamedCapture struct {
	name  string
	index int
}

func regexpNamedCaptures(pattern string) []regexpNamedCapture {
	result := []regexpNamedCapture{}
	captureIndex := 0
	inClass := false
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '\\':
			index++
			continue
		case '[':
			inClass = true
			continue
		case ']':
			inClass = false
			continue
		case '(':
			if inClass {
				continue
			}
		}
		if index+3 < len(pattern) && strings.HasPrefix(pattern[index:], "(?<") && pattern[index+3] != '=' && pattern[index+3] != '!' {
			end := strings.IndexByte(pattern[index+3:], '>')
			if end >= 0 {
				name := pattern[index+3 : index+3+end]
				captureIndex++
				result = append(result, regexpNamedCapture{name: name, index: captureIndex})
				continue
			}
		}
		if index+1 >= len(pattern) || pattern[index+1] != '?' {
			captureIndex++
		}
	}
	return result
}

func staticQuotedInterpolationValue(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if strings.HasSuffix(expr, ".to_sym") {
		expr = strings.TrimSpace(strings.TrimSuffix(expr, ".to_sym"))
	}
	if len(expr) < 2 {
		return "", false
	}
	quote := expr[0]
	if (quote != '\'' && quote != '"') || expr[len(expr)-1] != quote {
		return "", false
	}
	return expr[1 : len(expr)-1], true
}

func (c *Compiler) EnterScope() {
	scope := CompilationScope{
		instructions:       Instructions{},
		lineMap:            map[int]int{},
		breakTarget:        -1,
		nextPatchPos:       []int{},
		nextPatchTarget:    -1,
		redoTarget:         -1,
		breakValuePatchPos: []int{},
		retryTarget:        -1,
	}
	c.scopes = append(c.scopes, scope)
	c.scopeIndex++
	c.symbolTable = NewEnclosedSymbolTable(c.symbolTable)
}

func (c *Compiler) LeaveScope() Instructions {
	instructions := c.currentInstructions()
	c.scopes = c.scopes[:len(c.scopes)-1]
	c.scopeIndex--
	c.symbolTable = c.symbolTable.Outer

	return instructions
}

func (c *Compiler) currentLineMapCopy() map[int]int {
	source := c.scopes[c.scopeIndex].lineMap
	if len(source) == 0 {
		return nil
	}
	copied := make(map[int]int, len(source))
	for pos, line := range source {
		copied[pos] = line
	}
	return copied
}

type Bytecode struct {
	Instructions Instructions
	LineMap      map[int]int
	Constants    []*object.EmeraldValue
	NumLocals    int
	GlobalNames  map[string]int
	LocalNames   map[string]int
}

func (c *Compiler) compileParameterDefault(name string, expr ast.Expression) error {
	if expr == nil {
		return nil
	}
	symbol, ok := c.symbolTable.Resolve(name)
	if !ok || symbol.Scope != ScopeLocal {
		return fmt.Errorf("parameter default local %q is not defined", name)
	}
	jump := c.emit(OpJumpLocalPresent, symbol.Index, 0)
	if err := c.Compile(expr); err != nil {
		return err
	}
	c.emit(OpSetLocal, symbol.Index)
	c.Emit(OpPop)
	c.changeOperandAt(jump, 1, len(c.currentInstructions()))
	return nil
}

func (c *Compiler) compileDefaultValue(expr ast.Expression) *object.EmeraldValue {
	switch node := expr.(type) {
	case *ast.IntegerLiteral:
		value := &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  node.Value,
			Class: core.R.Classes["Integer"],
		}
		return core.RememberULEBPackIntegerLiteral(value, node.Token.Literal)
	case *ast.FloatLiteral:
		return &object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  node.Value,
			Class: core.R.Classes["Float"],
		}
	case *ast.StringLiteral:
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Value,
			Class: core.R.Classes["String"],
		}
	case *ast.SymbolLiteral:
		return &object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  strings.TrimPrefix(node.Value, ":"),
			Class: core.R.Classes["Symbol"],
		}
	case *ast.Boolean:
		if node.Value {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	case *ast.NilExpression:
		return core.R.NilVal
	case *ast.ArrayLiteral:
		elements := make([]*object.EmeraldValue, 0, len(node.Elements))
		for _, element := range node.Elements {
			elements = append(elements, c.compileDefaultValue(element))
		}
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  elements,
			Class: core.R.Classes["Array"],
		}
	default:
		return core.R.NilVal
	}
}

func (c *Compiler) compileStringLiteralValue(node *ast.StringLiteral) error {
	val := node.Value
	encoding := c.sourceEncoding
	if node.Token.HasUnicodeEscape && stringHasNonASCII(val) {
		encoding = "UTF-8"
	}
	if !node.Interpolates || !stringContainsInterpolation(val) {
		val = strings.ReplaceAll(val, lexer.EscapedHashInterpolation, "#")
		c.EmitConstant(&object.EmeraldValue{
			Type:     object.ValueString,
			Data:     val,
			Class:    core.R.Classes["String"],
			Encoding: encoding,
		})
		return nil
	}
	return c.compileStringInterpolationAtLineWithEncoding(val, node.Token.Line, encoding)
}

func stringHasNonASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] >= 0x80 {
			return true
		}
	}
	return false
}

func rationalLiteralParts(literal string) (*big.Int, *big.Int, bool) {
	text := strings.TrimSuffix(strings.ReplaceAll(literal, "_", ""), "r")
	if strings.ContainsAny(text, ".eE") {
		ratio, ok := new(big.Rat).SetString(text)
		if !ok {
			return nil, nil, false
		}
		return new(big.Int).Set(ratio.Num()), new(big.Int).Set(ratio.Denom()), true
	}
	integer, ok := new(big.Int).SetString(text, 0)
	if !ok {
		return nil, nil, false
	}
	return integer, big.NewInt(1), true
}

func (c *Compiler) compileStringInterpolation(s string) error {
	return c.compileStringInterpolationAtLine(s, 1)
}

func (c *Compiler) compileStringInterpolationAtLine(s string, startLine int) error {
	return c.compileStringInterpolationAtLineWithEncoding(s, startLine, c.sourceEncoding)
}

func (c *Compiler) compileStringInterpolationAtLineWithEncoding(s string, startLine int, encoding string) error {
	return c.compileStringInterpolationParts(s, startLine, encoding, true)
}

func (c *Compiler) compileStringInterpolationPreservingEncodingAtLine(s string, startLine int) error {
	return c.compileStringInterpolationParts(s, startLine, c.sourceEncoding, false)
}

func (c *Compiler) compileStringInterpolationParts(s string, startLine int, encoding string, forceEncoding bool) error {
	parts := splitStringInterpolation(s)
	if len(parts) == 0 {
		c.EmitConstant(&object.EmeraldValue{
			Type:     object.ValueString,
			Data:     "",
			Class:    core.R.Classes["String"],
			Encoding: encoding,
		})
		return nil
	}

	c.EmitConstant(&object.EmeraldValue{
		Type: object.ValueString, Data: "", Class: core.R.Classes["String"], Encoding: encoding,
	})
	partLine := startLine
	for _, part := range parts {
		if part.isExpr {
			expressionSource := part.text
			if partLine > 1 {
				expressionSource = strings.Repeat("\n", partLine-1) + expressionSource
			}
			l := lexer.New(expressionSource)
			p := parser.New(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				c.EmitConstant(&object.EmeraldValue{
					Type:     object.ValueString,
					Data:     strings.ReplaceAll("#{"+part.text+"}", lexer.EscapedHashInterpolation, "#"),
					Class:    core.R.Classes["String"],
					Encoding: encoding,
				})
			} else if len(prog.Statements) > 0 {
				stmt := prog.Statements[0]
				if exprStmt, ok := stmt.(*ast.ExpressionStatement); ok {
					if err := c.Compile(exprStmt.Expression); err != nil {
						return err
					}
				} else {
					if err := c.Compile(stmt); err != nil {
						return err
					}
				}
				methodIdx := c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  "to_s",
					Class: core.R.Classes["String"],
				})
				c.emit(OpSend, methodIdx, 0, 0, 255)
			} else {
				c.EmitConstant(&object.EmeraldValue{
					Type:     object.ValueString,
					Data:     "",
					Class:    core.R.Classes["String"],
					Encoding: encoding,
				})
			}
		} else {
			c.EmitConstant(&object.EmeraldValue{
				Type:     object.ValueString,
				Data:     strings.ReplaceAll(part.text, lexer.EscapedHashInterpolation, "#"),
				Class:    core.R.Classes["String"],
				Encoding: encoding,
			})
		}
		c.Emit(OpAdd)
		partLine += strings.Count(part.text, "\n")
	}
	if forceEncoding {
		encodingIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: encoding, Class: core.R.Classes["String"]})
		c.emit(OpSetStringEncoding, encodingIdx)
	}
	return nil
}

type interpPart struct {
	text   string
	isExpr bool
}

func splitStringInterpolation(s string) []interpPart {
	var parts []interpPart
	i := 0
	start := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '#' && s[i+1] == '{' && (i == 0 || s[i-1] != lexer.EscapedHashInterpolation[0]) {
			if i > start {
				parts = append(parts, interpPart{text: s[start:i], isExpr: false})
			}
			depth := 1
			j := i + 2
			for j < len(s) && depth > 0 {
				if s[j] == '{' {
					depth++
				} else if s[j] == '}' {
					depth--
				}
				j++
			}
			parts = append(parts, interpPart{text: s[i+2 : j-1], isExpr: true})
			start = j
			i = j
		} else if end, ok := simpleVariableInterpolationEnd(s, i); ok {
			if i > start {
				parts = append(parts, interpPart{text: s[start:i]})
			}
			parts = append(parts, interpPart{text: s[i+1 : end], isExpr: true})
			start = end
			i = end
		} else {
			i++
		}
	}
	if start < len(s) {
		parts = append(parts, interpPart{text: s[start:], isExpr: false})
	}
	return parts
}

func stringContainsInterpolation(value string) bool {
	if strings.Contains(value, "#{") {
		return true
	}
	for i := 0; i < len(value); i++ {
		if _, ok := simpleVariableInterpolationEnd(value, i); ok {
			return true
		}
	}
	return false
}

func simpleVariableInterpolationEnd(value string, start int) (int, bool) {
	if start < 0 || start+2 >= len(value) || value[start] != '#' || (start > 0 && value[start-1] == lexer.EscapedHashInterpolation[0]) {
		return 0, false
	}
	index := start + 1
	if value[index] == '@' {
		index++
		if index < len(value) && value[index] == '@' {
			index++
		}
	} else if value[index] == '$' {
		index++
	} else {
		return 0, false
	}
	nameStart := index
	for index < len(value) && ((value[index] >= 'a' && value[index] <= 'z') || (value[index] >= 'A' && value[index] <= 'Z') || (value[index] >= '0' && value[index] <= '9') || value[index] == '_') {
		index++
	}
	return index, index > nameStart
}

func compoundAssignmentOpcode(token lexer.TokenType) (Opcode, bool) {
	switch token {
	case lexer.PLUS_ASSIGN:
		return OpAdd, true
	case lexer.MINUS_ASSIGN:
		return OpSub, true
	case lexer.MULTIPLY_ASSIGN:
		return OpMul, true
	case lexer.DIVIDE_ASSIGN:
		return OpDiv, true
	case lexer.MOD_ASSIGN:
		return OpMod, true
	case lexer.POW_ASSIGN:
		return OpPow, true
	case lexer.BIT_AND_ASSIGN:
		return OpBitAnd, true
	case lexer.BIT_OR_ASSIGN:
		return OpBitOr, true
	case lexer.BIT_XOR_ASSIGN:
		return OpBitXor, true
	case lexer.LSHIFT_ASSIGN:
		return OpBitLeftShift, true
	case lexer.RSHIFT_ASSIGN:
		return OpBitRightShift, true
	default:
		return 0, false
	}
}

func compoundAssignmentMethod(token lexer.TokenType) (string, bool) {
	switch token {
	case lexer.PLUS_ASSIGN:
		return "+", true
	case lexer.MINUS_ASSIGN:
		return "-", true
	case lexer.MULTIPLY_ASSIGN:
		return "*", true
	case lexer.DIVIDE_ASSIGN:
		return "/", true
	case lexer.MOD_ASSIGN:
		return "%", true
	case lexer.POW_ASSIGN:
		return "**", true
	case lexer.BIT_AND_ASSIGN:
		return "&", true
	case lexer.BIT_OR_ASSIGN:
		return "|", true
	case lexer.BIT_XOR_ASSIGN:
		return "^", true
	case lexer.LSHIFT_ASSIGN:
		return "<<", true
	case lexer.RSHIFT_ASSIGN:
		return ">>", true
	default:
		return "", false
	}
}

func expressionContainsSplat(expr ast.Expression) bool {
	if expr == nil {
		return false
	}
	_, ok := expr.(*ast.SplatExpression)
	return ok
}

func expressionsContainSplat(expressions []ast.Expression) bool {
	for _, expression := range expressions {
		if expressionContainsSplat(expression) {
			return true
		}
	}
	return false
}

func expandForwardArguments(args []ast.Expression) []ast.Expression {
	if len(args) != 1 {
		return args
	}
	forward, ok := args[0].(*ast.SplatExpression)
	if !ok || forward.Token.Type != lexer.DOT3 {
		return args
	}
	positionalToken := forward.Token
	positionalToken.Type = lexer.MULTIPLY
	positionalToken.Literal = "*"
	keywordToken := forward.Token
	keywordToken.Type = lexer.POW
	keywordToken.Literal = "**"
	blockToken := forward.Token
	blockToken.Type = lexer.BIT_AND
	blockToken.Literal = "&"
	return []ast.Expression{
		&ast.SplatExpression{Token: positionalToken, Value: &ast.Identifier{Token: positionalToken, Value: "__rgo_forward_args"}},
		&ast.SplatExpression{Token: keywordToken, Value: &ast.Identifier{Token: keywordToken, Value: "__rgo_forward_kwargs"}},
		&ast.SplatExpression{Token: blockToken, Value: &ast.Identifier{Token: blockToken, Value: "__rgo_forward_block"}},
	}
}

func (c *Compiler) compileLogicalSendAssignment(receiver ast.Expression, getter, setter string, args []ast.Expression, value ast.Expression, operator lexer.TokenType, safe bool) error {
	if receiver == nil {
		c.Emit(OpSelf)
	} else if err := c.Compile(receiver); err != nil {
		return err
	}
	jumpEnd := -1
	if safe {
		c.Emit(OpDup)
		jumpCall := c.emit(OpJumpNotNil, 0)
		jumpEnd = c.emit(OpJump, 0)
		c.changeOperand(jumpCall, len(c.currentInstructions()))
	}
	for _, arg := range args {
		if err := c.Compile(arg); err != nil {
			return err
		}
	}
	if err := c.compileExpressionThunk(value); err != nil {
		return err
	}
	mode := 0
	if operator == lexer.AND_ASSIGN {
		mode = 1
	}
	getterIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: getter, Class: core.R.Classes["String"]})
	setterIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: setter, Class: core.R.Classes["String"]})
	c.emit(OpLogicalSendAssignment, getterIdx, setterIdx, len(args), mode)
	if jumpEnd >= 0 {
		c.changeOperand(jumpEnd, len(c.currentInstructions()))
	}
	return nil
}

func (c *Compiler) emitCompoundOperator(operator string) bool {
	switch operator {
	case "+":
		c.Emit(OpAdd)
	case "-":
		c.Emit(OpSub)
	case "*":
		c.Emit(OpMul)
	case "/":
		c.Emit(OpDiv)
	case "%":
		c.Emit(OpMod)
	case "**":
		c.Emit(OpPow)
	case "&":
		c.Emit(OpBitAnd)
	case "|":
		c.Emit(OpBitOr)
	case "^":
		c.Emit(OpBitXor)
	case "<<":
		c.Emit(OpBitLeftShift)
	case ">>":
		c.Emit(OpBitRightShift)
	default:
		return false
	}
	return true
}

func (c *Compiler) emitVariableAssignmentStore(name *ast.Identifier) error {
	if name == nil {
		return fmt.Errorf("missing assignment target")
	}
	if strings.HasPrefix(name.Value, "$") {
		c.emit(OpSetGlobal, c.globalSymbolIndex(name.Value))
		return nil
	}
	if strings.HasPrefix(name.Value, "@@") {
		c.emit(OpSetClassVar, c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}))
		return nil
	}
	if strings.HasPrefix(name.Value, "@") {
		c.emit(OpSetInstanceVar, c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}))
		return nil
	}
	if isConstantName(name.Value) {
		c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}), 0)
		return nil
	}
	sym, ok := c.symbolTable.Resolve(name.Value)
	if !ok || sym.Scope == ScopeBuiltin {
		c.symbolTable.Define(name.Value)
		sym, _ = c.symbolTable.Resolve(name.Value)
	}
	switch sym.Scope {
	case ScopeGlobal:
		c.emit(OpSetGlobal, sym.Index)
	case ScopeLocal:
		c.emit(OpSetLocal, sym.Index)
	case ScopeOuter:
		c.emit(OpSetOuter, 0, sym.ScopeIndex)
	case ScopeOuterFree:
		c.emit(OpSetOuterFree, sym.ScopeIndex)
	case ScopeFree:
		c.emit(OpSetFree, sym.Index)
	default:
		return fmt.Errorf("unsupported assignment scope for %s", name.Value)
	}
	return nil
}

func (c *Compiler) compileAssignmentCurrentValue(name *ast.Identifier) error {
	if name == nil {
		c.Emit(OpNil)
		return nil
	}
	if len(name.Value) > 0 && name.Value[0] == '$' {
		c.emit(OpGetGlobal, c.globalSymbolIndex(name.Value))
		return nil
	}
	if len(name.Value) > 1 && name.Value[0] == '@' && name.Value[1] == '@' {
		c.emit(OpGetClassVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  name.Value,
			Class: core.R.Classes["String"],
		}))
		return nil
	}
	if len(name.Value) > 0 && name.Value[0] == '@' {
		c.emit(OpGetInstanceVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  name.Value,
			Class: core.R.Classes["String"],
		}))
		return nil
	}
	if isConstantName(name.Value) {
		c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  name.Value,
			Class: core.R.Classes["String"],
		}))
		return nil
	}

	sym, ok := c.symbolTable.Resolve(name.Value)
	if !ok || sym.Scope == ScopeBuiltin {
		c.symbolTable.Define(name.Value)
		sym, _ = c.symbolTable.Resolve(name.Value)
	}
	switch sym.Scope {
	case ScopeGlobal:
		c.emit(OpGetGlobal, sym.Index)
	case ScopeLocal:
		c.emit(OpGetLocal, sym.Index)
	case ScopeOuter:
		c.emit(OpGetOuter, sym.ScopeIndex)
	case ScopeOuterFree:
		c.emit(OpGetOuterFree, sym.ScopeIndex)
	case ScopeFree:
		c.emit(OpGetFree, sym.Index)
	default:
		c.Emit(OpNil)
	}
	return nil
}

func (c *Compiler) emitCaptureSymbol(sym Symbol) {
	switch sym.Scope {
	case ScopeLocal:
		c.emit(OpGetLocalCell, sym.Index)
	case ScopeOuter:
		c.emit(OpGetOuterCell, sym.ScopeIndex)
	case ScopeOuterFree:
		c.emit(OpGetOuterFreeCell, sym.ScopeIndex)
	case ScopeFree:
		if localSym, ok := c.symbolTable.store[sym.Name]; ok && localSym.Scope == ScopeLocal {
			c.emit(OpGetLocalCell, localSym.Index)
			return
		}
		c.emit(OpGetFreeCell, sym.Index)
	case ScopeGlobal:
		c.emit(OpGetGlobal, sym.Index)
	default:
		c.Emit(OpNil)
	}
}

func (c *Compiler) compileExpressionThunk(expr ast.Expression) error {
	c.EnterScope()
	if err := c.Compile(expr); err != nil {
		return err
	}
	c.Emit(OpReturnValue)
	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	localNames := c.localNames()
	lineMap := c.currentLineMapCopy()
	instructions := c.LeaveScope()

	fn := &object.EmeraldValue{
		Type: object.ValueFunction,
		Data: &object.Function{
			Name:           "__scoped_const_rhs__",
			SourcePath:     core.CurrentSpecFile,
			EvalSource:     core.CurrentEvalSource,
			SourceEncoding: c.sourceEncoding,
			Instructions:   instructions,
			LineMap:        lineMap,
			NumLocals:      numLocals,
			GlobalNames:    c.globalNamesCopy(),
			LocalNames:     localNames,
			FreeVarNames:   freeVarNames(free),
		},
		Class: core.R.Classes["Class"],
	}
	fnIdx := c.addConstant(fn)
	for _, s := range free {
		c.emitCaptureSymbol(s)
	}
	c.emit(OpClosure, fnIdx, len(free))
	return nil
}

func (c *Compiler) compileRangeExpression(node *ast.RangeExpression) error {
	if err := c.Compile(node.Left); err != nil {
		return err
	}
	if err := c.Compile(node.Right); err != nil {
		return err
	}
	exclusive := 0
	if node.Exclusive {
		exclusive = 1
	}
	startMissing := 0
	if node.StartMissing {
		startMissing = 1
	}
	endMissing := 0
	if node.EndMissing {
		endMissing = 1
	}
	// Ruby treats the ambiguous forms `..nil` and `nil...` as ranges
	// whose two endpoints are explicitly nil, not as open ranges.
	if _, ok := node.Right.(*ast.NilExpression); ok && node.StartMissing {
		startMissing = 0
	}
	if _, ok := node.Left.(*ast.NilExpression); ok && node.EndMissing {
		endMissing = 0
	}
	c.emit(OpRange, exclusive, startMissing, endMissing)
	return nil
}

func (c *Compiler) compilePostUntilExpression(node *ast.UntilExpression) error {
	scope := &c.scopes[c.scopeIndex]
	previousRedoTarget := scope.redoTarget
	previousNextPatchTarget := scope.nextPatchTarget
	previousBreakTarget := scope.breakTarget
	previousNextPatches := scope.nextPatchPos
	previousBreakValuePatches := scope.breakValuePatchPos

	scope.breakTarget = -1
	scope.nextPatchPos = nil
	scope.breakValuePatchPos = nil
	setWhileEndPos := c.emit(OpSetWhileEnd, 0)
	bodyStart := len(c.currentInstructions())
	scope.redoTarget = bodyStart
	scope.nextPatchTarget = -1
	if err := c.Compile(node.Body); err != nil {
		return err
	}
	conditionStart := len(c.currentInstructions())
	for _, patchPos := range scope.nextPatchPos {
		c.changeOperand(patchPos, conditionStart)
	}
	if err := c.compileCondition(node.Condition); err != nil {
		return err
	}
	jumpTruthyPos := c.emit(OpJumpTruthy, 0)
	c.emit(OpJump, bodyStart)
	afterBody := len(c.currentInstructions())
	c.changeOperand(jumpTruthyPos, afterBody)
	c.changeOperand(setWhileEndPos, afterBody)
	scope.breakTarget = afterBody
	c.Emit(OpNil)
	endOfLoop := len(c.currentInstructions())
	for _, patchPos := range scope.breakValuePatchPos {
		c.changeOperand(patchPos, endOfLoop)
	}

	scope.redoTarget = previousRedoTarget
	scope.nextPatchTarget = previousNextPatchTarget
	scope.breakTarget = previousBreakTarget
	scope.nextPatchPos = previousNextPatches
	scope.breakValuePatchPos = previousBreakValuePatches
	return nil
}

func (c *Compiler) compilePostWhileExpression(node *ast.WhileExpression) error {
	scope := &c.scopes[c.scopeIndex]
	previousRedoTarget := scope.redoTarget
	previousNextPatchTarget := scope.nextPatchTarget
	previousBreakTarget := scope.breakTarget
	previousNextPatches := scope.nextPatchPos
	previousBreakValuePatches := scope.breakValuePatchPos

	scope.breakTarget = -1
	scope.nextPatchPos = nil
	scope.breakValuePatchPos = nil
	setWhileEndPos := c.emit(OpSetWhileEnd, 0)
	bodyStart := len(c.currentInstructions())
	scope.redoTarget = bodyStart
	scope.nextPatchTarget = -1
	if err := c.Compile(node.Body); err != nil {
		return err
	}
	conditionStart := len(c.currentInstructions())
	for _, patchPos := range scope.nextPatchPos {
		c.changeOperand(patchPos, conditionStart)
	}
	if err := c.compileCondition(node.Condition); err != nil {
		return err
	}
	jumpNotTruthyPos := c.emit(OpJumpNotTruthy, 0)
	c.emit(OpJump, bodyStart)
	afterBody := len(c.currentInstructions())
	c.changeOperand(jumpNotTruthyPos, afterBody)
	c.changeOperand(setWhileEndPos, afterBody)
	scope.breakTarget = afterBody
	c.Emit(OpNil)
	endOfLoop := len(c.currentInstructions())
	for _, patchPos := range scope.breakValuePatchPos {
		c.changeOperand(patchPos, endOfLoop)
	}

	scope.redoTarget = previousRedoTarget
	scope.nextPatchTarget = previousNextPatchTarget
	scope.breakTarget = previousBreakTarget
	scope.nextPatchPos = previousNextPatches
	scope.breakValuePatchPos = previousBreakValuePatches
	return nil
}

func (c *Compiler) compileForExpression(node *ast.ForExpression) error {
	collectForResults := len(node.Body.Statements) > 0
	forBody := *node.Body

	// for loop loop variables should be assigned back into the outer scope.
	// Capture all variables first in the outer frame (if not already present),
	// then pass temporary iterator values into the block body and rebind.
	targetLocalAliases := map[string]int{}
	assignedVarCount := 1
	tupleUnpackTarget := c.forTargetNeedsTupleUnpack(node.Variable)
	hasSplatTarget := false
	nonNilTargetCount := 0
	for _, name := range node.Variable {
		if name != nil {
			nonNilTargetCount++
		}
		if _, ok := name.(*ast.SplatExpression); ok {
			hasSplatTarget = true
			break
		}
	}
	if hasSplatTarget || node.TupleTarget {
		assignedVarCount = 1
	} else if tupleUnpackTarget {
		assignedVarCount = 1
	}
	for _, name := range node.Variable {
		targetName := c.forTargetCaptureName(name)
		if targetName == "" {
			continue
		}
		if !isValidLocalNameLikeRuby(targetName) {
			continue
		}
		sym, ok := c.symbolTable.Resolve(targetName)
		if ok && sym.Scope != ScopeBuiltin {
			targetLocalAliases[targetName] = sym.Index
			continue
		}
		c.symbolTable.Define(targetName)
		if sym, ok := c.symbolTable.Resolve(targetName); ok && sym.Scope != ScopeBuiltin {
			targetLocalAliases[targetName] = sym.Index
		}
	}
	bodyAssignedLocals := c.forBodyAssignedLocalNames(&forBody)
	for bodyName := range bodyAssignedLocals {
		if _, exists := targetLocalAliases[bodyName]; exists {
			continue
		}
		if _, ok := c.symbolTable.Resolve(bodyName); ok {
			sym, _ := c.symbolTable.Resolve(bodyName)
			if sym.Scope != ScopeBuiltin {
				continue
			}
		}
		c.symbolTable.Define(bodyName)
		if sym, ok := c.symbolTable.Resolve(bodyName); ok && sym.Scope != ScopeBuiltin {
			targetLocalAliases[bodyName] = sym.Index
		}
	}

	// Temporary parameters receive each yielded value(s). Assign into outer targets explicitly.
	forBody.Params = make([]*ast.Identifier, assignedVarCount)
	for i := 0; i < assignedVarCount; i++ {
		forBody.Params[i] = &ast.Identifier{
			Token: node.Token,
			Value: fmt.Sprintf("__rgo_for_value_%d", i),
		}
	}

	if assignedVarCount > 0 {
		prefix := make([]ast.Statement, 0, assignedVarCount)
		targetValue := forBody.Params[0]
		for i, name := range node.Variable {
			var valueExpr ast.Expression
			var err error
			switch {
			case hasSplatTarget:
				valueExpr, err = c.forTargetSplatValue(node.Token, i, node.Variable, targetValue)
				if err != nil {
					return err
				}
			case node.TupleTarget:
				valueExpr = c.forTargetArrayValue(node.Token, i, targetValue)
			case tupleUnpackTarget:
				valueExpr = c.forTargetFirstArrayOrSelfValue(node.Token, targetValue)
			case !hasSplatTarget && !tupleUnpackTarget:
				if len(node.Variable) == 1 {
					flattenForHash := &ast.MethodCall{
						Token:    node.Token,
						Receiver: forBody.Params[0],
						Method: &ast.Identifier{
							Token: node.Token,
							Value: "to_a",
						},
						Args: []ast.Expression{},
					}
					flattenForHash = &ast.MethodCall{
						Token:    node.Token,
						Receiver: flattenForHash,
						Method: &ast.Identifier{
							Token: node.Token,
							Value: "flatten",
						},
						Args: []ast.Expression{},
					}
					isHashValue := &ast.MethodCall{
						Token:    node.Token,
						Receiver: targetValue,
						Method: &ast.Identifier{
							Token: node.Token,
							Value: "is_a?",
						},
						Args: []ast.Expression{&ast.Constant{
							Token: node.Token,
							Name:  "Hash",
						}},
					}
					valueExpr = &ast.TernaryExpression{
						Token: node.Token,
						Condition: &ast.InfixExpression{
							Token:    node.Token,
							Left:     isHashValue,
							Operator: "==",
							Right:    &ast.Boolean{Token: node.Token, Value: true},
						},
						Consequent:  flattenForHash,
						Alternative: forBody.Params[0],
					}
					break
				}
				valueExpr = c.forTargetArrayValue(node.Token, i, targetValue)
				break
			}

			assignExpr, err := c.forTargetAssignmentExpression(name, valueExpr)
			if err != nil {
				return err
			}
			if assignExpr != nil {
				prefix = append(prefix, &ast.ExpressionStatement{
					Token:      node.Token,
					Expression: assignExpr,
				})
			}
		}

		if !hasSplatTarget && !tupleUnpackTarget && node.TupleTarget && nonNilTargetCount > 1 {
			isArrayValue := &ast.MethodCall{
				Token:    node.Token,
				Receiver: targetValue,
				Method: &ast.Identifier{
					Token: node.Token,
					Value: "is_a?",
				},
				Args: []ast.Expression{&ast.Constant{Token: node.Token, Name: "Array"}},
			}
			arrayLength := c.forTargetLengthExpression(targetValue, node.Token)
			shortArray := &ast.InfixExpression{
				Token:    node.Token,
				Left:     arrayLength,
				Operator: "<",
				Right:    c.integerLiteral(node.Token, int64(len(node.Variable))),
			}
			skipShortSource := &ast.InfixExpression{
				Token:    node.Token,
				Left:     isArrayValue,
				Operator: "&&",
				Right:    shortArray,
			}
			guardBlock := &ast.BlockExpression{Token: node.Token}
			guardBlock.Statements = append(guardBlock.Statements, prefix...)
			guardBlock.Statements = append(guardBlock.Statements, forBody.Statements...)
			forBody.Statements = []ast.Statement{
				&ast.ExpressionStatement{
					Token: node.Token,
					Expression: &ast.IfExpression{
						Token:      node.Token,
						Condition:  skipShortSource,
						Consequent: guardBlock,
						IsUnless:   true,
					},
				},
			}
		} else {
			forBody.Statements = append(prefix, forBody.Statements...)
		}
	}
	eachIdx := c.addConstant(&object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "each",
		Class: core.R.Classes["String"],
	})

	if err := c.Compile(node.Collection); err != nil {
		return err
	}
	c.forEachDepth++
	forLoopCollectAsPair := !hasSplatTarget && !node.TupleTarget && !tupleUnpackTarget
	if err := c.compileBlockAsClosureWithLocalNamesInternal(&forBody, targetLocalAliases, forLoopCollectAsPair); err != nil {
		c.forEachDepth--
		return err
	}
	c.forEachDepth--

	if collectForResults {
		c.emit(OpEnterForEach, 1)
	} else {
		c.emit(OpEnterForEach, 0)
	}
	c.emit(OpSend, eachIdx, 1, 0, 255)
	c.emit(OpExitForEach)

	return nil
}

func (c *Compiler) forBodyAssignedLocalNames(block *ast.BlockExpression) map[string]struct{} {
	names := map[string]struct{}{}
	if block == nil {
		return names
	}
	for _, stmt := range block.Statements {
		c.forBodyCollectAssignedLocalNamesFromStatement(stmt, names)
	}
	return names
}

func (c *Compiler) forBodyCollectAssignedLocalNamesFromStatement(stmt ast.Statement, names map[string]struct{}) {
	if stmt == nil {
		return
	}
	switch node := stmt.(type) {
	case *ast.ExpressionStatement:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Expression, names)
	case *ast.ReturnExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.ReturnValue, names)
	case *ast.BreakExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Value, names)
	case *ast.NextExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Value, names)
	case *ast.ThrowExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Label, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Value, names)
		for _, arg := range node.ExtraArgs {
			c.forBodyCollectAssignedLocalNamesFromExpression(arg, names)
		}
	case *ast.RaiseExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Error, names)
	case *ast.RedoExpression:
	case *ast.RetryExpression:
	}
}

func (c *Compiler) forBodyCollectAssignedLocalNamesFromBlock(block *ast.BlockExpression, names map[string]struct{}) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		c.forBodyCollectAssignedLocalNamesFromStatement(stmt, names)
	}
}

func (c *Compiler) forBodyCollectAssignedLocalNamesFromExpression(expr ast.Expression, names map[string]struct{}) {
	if expr == nil {
		return
	}
	switch node := expr.(type) {
	case *ast.AssignExpression:
		c.forBodyAddAssignedLocalName(node.Name, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Target, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Index, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.End, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Value, names)
	case *ast.MultiAssignExpression:
		for _, name := range node.Names {
			c.forBodyAddAssignedLocalName(name, names)
		}
		for _, value := range node.Values {
			c.forBodyCollectAssignedLocalNamesFromExpression(value, names)
		}
	case *ast.InfixExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Left, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Right, names)
	case *ast.PrefixExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Right, names)
	case *ast.TernaryExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Condition, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Consequent, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Alternative, names)
	case *ast.RangeExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Left, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Right, names)
	case *ast.IndexExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Left, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Index, names)
		c.forBodyCollectAssignedLocalNamesFromExpression(node.End, names)
	case *ast.ArrayLiteral:
		for _, elem := range node.Elements {
			c.forBodyCollectAssignedLocalNamesFromExpression(elem, names)
		}
	case *ast.HashLiteral:
		for _, key := range node.Order {
			c.forBodyCollectAssignedLocalNamesFromExpression(key, names)
			if value, ok := node.Pairs[key]; ok {
				c.forBodyCollectAssignedLocalNamesFromExpression(value, names)
			}
		}
	case *ast.MethodCall:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Receiver, names)
		for _, arg := range node.Args {
			c.forBodyCollectAssignedLocalNamesFromExpression(arg, names)
		}
		for _, kw := range node.KeywordArgs {
			c.forBodyCollectAssignedLocalNamesFromExpression(kw.Value, names)
		}
	case *ast.ConstantResolution:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Left, names)
	case *ast.IfExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Condition, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Consequent, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Alternative, names)
		for _, elsif := range node.ElsIf {
			c.forBodyCollectAssignedLocalNamesFromExpression(elsif.Condition, names)
			c.forBodyCollectAssignedLocalNamesFromBlock(elsif.Consequent, names)
		}
	case *ast.WhileExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Condition, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Body, names)
	case *ast.UntilExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Condition, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Body, names)
	case *ast.ForExpression:
		for _, target := range node.Variable {
			targetName := c.forTargetCaptureName(target)
			if !isValidLocalNameLikeRuby(targetName) {
				continue
			}
			c.forBodyAddAssignedLocalName(&ast.Identifier{Value: targetName}, names)
		}
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Body, names)
	case *ast.BeginExpression:
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Body, names)
		for _, rescueClause := range node.Rescue {
			c.forBodyAddAssignedLocalName(rescueClause.Variable, names)
			c.forBodyCollectAssignedLocalNamesFromBlock(rescueClause.Body, names)
		}
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Else, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Ensure, names)
	case *ast.CaseExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Expression, names)
		for _, clause := range node.Clauses {
			for _, cond := range clause.Conditions {
				c.forBodyCollectAssignedLocalNamesFromExpression(cond, names)
			}
			c.forBodyCollectAssignedLocalNamesFromBlock(clause.Body, names)
		}
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Else, names)
	case *ast.CatchExpression:
		c.forBodyCollectAssignedLocalNamesFromExpression(node.Label, names)
		c.forBodyCollectAssignedLocalNamesFromBlock(node.Body, names)
	case *ast.BlockExpression:
		c.forBodyCollectAssignedLocalNamesFromBlock(node, names)
	case *ast.ProcLiteral, *ast.DefExpression, *ast.ClassExpression, *ast.ModuleExpression:
		return
	}
}

func (c *Compiler) forBodyAddAssignedLocalName(name *ast.Identifier, names map[string]struct{}) {
	if name == nil {
		return
	}
	if !isValidLocalNameLikeRuby(name.Value) {
		return
	}
	names[name.Value] = struct{}{}
}

func (c *Compiler) forTargetArrayValue(token lexer.Token, targetIndex int, source ast.Expression) ast.Expression {
	indexExpr := &ast.IndexExpression{
		Token: token,
		Left:  source,
		Index: &ast.IntegerLiteral{
			Token: token,
			Value: int64(targetIndex),
		},
	}
	return &ast.TernaryExpression{
		Token: token,
		Condition: &ast.InfixExpression{
			Token:    token,
			Left:     c.forTargetLengthExpression(source, token),
			Operator: ">",
			Right:    &ast.IntegerLiteral{Token: token, Value: int64(targetIndex)},
		},
		Consequent:  indexExpr,
		Alternative: &ast.IntegerLiteral{Token: token, Value: 0},
	}
}

func (c *Compiler) forTargetNeedsTupleUnpack(targets []ast.Expression) bool {
	if len(targets) != 1 {
		return false
	}
	if target, ok := targets[0].(*ast.ArrayLiteral); ok {
		return len(target.Elements) == 1
	}
	return false
}

func (c *Compiler) forTargetFirstArrayOrSelfValue(token lexer.Token, source ast.Expression) ast.Expression {
	isArrayValue := &ast.MethodCall{
		Token:    token,
		Receiver: source,
		Method: &ast.Identifier{
			Token: token,
			Value: "is_a?",
		},
		Args: []ast.Expression{&ast.Constant{Token: token, Name: "Array"}},
	}
	return &ast.TernaryExpression{
		Token: token,
		Condition: &ast.InfixExpression{
			Token:    token,
			Left:     isArrayValue,
			Operator: "==",
			Right:    &ast.Boolean{Token: token, Value: true},
		},
		Consequent: &ast.IndexExpression{
			Token: token,
			Left:  source,
			Index: &ast.IntegerLiteral{
				Token: token,
				Value: 0,
			},
		},
		Alternative: source,
	}
}

func (c *Compiler) forTargetSplatValue(token lexer.Token, targetIndex int, targets []ast.Expression, source ast.Expression) (ast.Expression, error) {
	if source == nil {
		return &ast.NilExpression{Token: token}, nil
	}
	splatIndex := -1
	for i, candidate := range targets {
		if _, ok := candidate.(*ast.SplatExpression); ok {
			splatIndex = i
			break
		}
	}
	if splatIndex < 0 {
		return nil, fmt.Errorf("splat assignment expected splat target")
	}
	if targetIndex < splatIndex {
		return &ast.IndexExpression{
			Token: token,
			Left:  source,
			Index: &ast.IntegerLiteral{
				Token: token,
				Value: int64(targetIndex),
			},
		}, nil
	}
	if targetIndex > splatIndex {
		afterCount := len(targets) - splatIndex - 1
		afterOffset := int64(targetIndex - splatIndex)
		length := c.forTargetLengthExpression(source, token)
		minSourceLength := int64(splatIndex + afterCount)
		availableCond := &ast.InfixExpression{
			Token:    token,
			Left:     length,
			Operator: ">=",
			Right:    c.integerLiteral(token, minSourceLength),
		}
		afterIndex := &ast.InfixExpression{
			Token:    token,
			Left:     length,
			Operator: "-",
			Right:    c.integerLiteral(token, int64(afterCount+1)-afterOffset),
		}
		return &ast.TernaryExpression{
			Token:     token,
			Condition: availableCond,
			Consequent: &ast.IndexExpression{
				Token: token,
				Left:  source,
				Index: afterIndex,
			},
			Alternative: &ast.NilExpression{Token: token},
		}, nil
	}

	afterCount := len(targets) - splatIndex - 1
	length := c.forTargetLengthExpression(source, token)
	preCountExpr := c.integerLiteral(token, int64(splatIndex))
	diffExpr := &ast.InfixExpression{
		Token:    token,
		Left:     length,
		Operator: "-",
		Right:    preCountExpr,
	}
	diffExpr2 := &ast.InfixExpression{
		Token:    token,
		Left:     diffExpr,
		Operator: "-",
		Right:    c.integerLiteral(token, int64(afterCount)),
	}
	restLenExpr := &ast.TernaryExpression{
		Token:       token,
		Condition:   &ast.InfixExpression{Token: token, Left: diffExpr2, Operator: ">", Right: c.integerLiteral(token, 0)},
		Consequent:  diffExpr2,
		Alternative: &ast.IntegerLiteral{Token: token, Value: 0},
	}
	startExpr := &ast.TernaryExpression{
		Token:       token,
		Condition:   &ast.InfixExpression{Token: token, Left: preCountExpr, Operator: ">", Right: length},
		Consequent:  length,
		Alternative: preCountExpr,
	}
	return &ast.IndexExpression{
		Token: token,
		Left:  source,
		Index: startExpr,
		End:   restLenExpr,
	}, nil
}

func (c *Compiler) forTargetLengthExpression(source ast.Expression, token lexer.Token) ast.Expression {
	return &ast.MethodCall{
		Token:    token,
		Receiver: source,
		Method: &ast.Identifier{
			Token: token,
			Value: "length",
		},
		Args: []ast.Expression{},
	}
}

func (c *Compiler) integerLiteral(token lexer.Token, value int64) *ast.IntegerLiteral {
	return &ast.IntegerLiteral{
		Token: token,
		Value: value,
	}
}

func isValidLocalNameLikeRuby(name string) bool {
	if len(name) == 0 {
		return false
	}
	first := name[0]
	if first != '_' && (first < 'a' || first > 'z') {
		return false
	}
	for i := 1; i < len(name); i++ {
		ch := name[i]
		if ch == '_' ||
			(ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func (c *Compiler) forTargetCaptureName(target ast.Expression) string {
	switch node := target.(type) {
	case *ast.Identifier:
		return node.Value
	case *ast.ArrayLiteral:
		if len(node.Elements) != 1 {
			return ""
		}
		return c.forTargetCaptureName(node.Elements[0])
	case *ast.InstanceVariable:
		return node.Name
	case *ast.ClassVariable:
		return node.Name
	case *ast.GlobalVariable:
		return node.Name
	case *ast.SplatExpression:
		return c.forTargetCaptureName(node.Value)
	default:
		return ""
	}
}

func (c *Compiler) forTargetAssignmentExpression(target ast.Expression, value ast.Expression) (ast.Expression, error) {
	if target == nil {
		return nil, nil
	}
	switch node := target.(type) {
	case *ast.ArrayLiteral:
		if len(node.Elements) == 1 {
			return c.forTargetAssignmentExpression(node.Elements[0], value)
		}
		return nil, nil
	case *ast.Identifier:
		return &ast.AssignExpression{
			Token: node.Token,
			Name:  node,
			Value: value,
		}, nil
	case *ast.Constant:
		return &ast.AssignExpression{
			Token: node.Token,
			Name: &ast.Identifier{
				Token: node.Token,
				Value: node.Name,
			},
			Value: value,
		}, nil
	case *ast.InstanceVariable:
		return &ast.AssignExpression{
			Token: node.Token,
			Name: &ast.Identifier{
				Token: node.Token,
				Value: node.Name,
			},
			Value: value,
		}, nil
	case *ast.ClassVariable:
		return &ast.AssignExpression{
			Token: node.Token,
			Name: &ast.Identifier{
				Token: node.Token,
				Value: node.Name,
			},
			Value: value,
		}, nil
	case *ast.GlobalVariable:
		return &ast.AssignExpression{
			Token: node.Token,
			Name: &ast.Identifier{
				Token: node.Token,
				Value: node.Name,
			},
			Value: value,
		}, nil
	case *ast.IndexExpression:
		assignName := node.Left
		return &ast.AssignExpression{
			Token: node.Token,
			Name: &ast.Identifier{
				Token: node.Token,
				Value: node.String(),
			},
			Target: assignName,
			Index:  node.Index,
			End:    node.End,
			Value:  value,
		}, nil
	case *ast.MethodCall:
		if node.Receiver == nil || node.Method == nil {
			return nil, fmt.Errorf("invalid for loop target %T", target)
		}
		if len(node.Args) > 0 || len(node.KeywordArgs) > 0 {
			return nil, fmt.Errorf("invalid for loop target %T", target)
		}
		return &ast.MethodCall{
			Token:    node.Token,
			Receiver: node.Receiver,
			Method: &ast.Identifier{
				Token: node.Method.Token,
				Value: node.Method.Value + "=",
			},
			Safe: node.Safe,
			Args: []ast.Expression{value},
		}, nil
	case *ast.SplatExpression:
		return c.forTargetAssignmentExpression(node.Value, value)
	default:
		return nil, fmt.Errorf("invalid for-loop target %T", target)
	}
}

func (c *Compiler) compileProcLiteral(node *ast.ProcLiteral) error {
	numberedParamCount := blockNumberedParameterCount(node.Body)
	implicitIt := numberedParamCount == 0 && c.procLiteralUsesImplicitItParameter(node)
	c.EnterScope()
	if implicitIt {
		c.implicitIt[c.symbolTable] = true
	}

	params := node.Params
	paramDefaults := node.ParamDefaults
	if numberedParamCount > 0 && len(params) == 0 && node.RestParam == nil && node.BlockParam == nil {
		params = numberedParameterIdentifiers(numberedParamCount)
		paramDefaults = make([]ast.Expression, numberedParamCount)
	} else if implicitIt {
		params = []*ast.Identifier{{Value: "it"}}
		paramDefaults = []ast.Expression{nil}
	}
	restIndex := node.RestParamIndex
	if restIndex < 0 || restIndex > len(params) {
		restIndex = len(params)
	}
	paramLocalIndices := make([]int, len(params))
	for i, param := range params[:restIndex] {
		paramLocalIndices[i] = c.symbolTable.DefineParameter(param.Value).Index
	}
	anonymousRestParam := node.RestParam != nil && node.RestParam.Value == "_" && node.RestParam.Token.Literal == "*"
	if node.RestParam != nil && !anonymousRestParam {
		c.symbolTable.Define(node.RestParam.Value)
	}
	for i, param := range params[restIndex:] {
		paramLocalIndices[restIndex+i] = c.symbolTable.DefineParameter(param.Value).Index
	}
	if paramsMatchAST(params, node.Params) {
		c.defineParameterPatternLocals(node.ParamPatterns)
	}
	for _, keyword := range node.KeywordParams {
		c.symbolTable.Define(keyword.Name)
	}
	if node.KeywordRestParam != nil {
		c.symbolTable.Define(node.KeywordRestParam.Value)
	}
	blockParamIndex := -1
	if node.BlockParam != nil {
		blockParamIndex = c.symbolTable.Define(node.BlockParam.Value).Index
	}
	for i, defaultExpr := range paramDefaults {
		if i < len(params) {
			if err := c.compileParameterDefault(params[i].Value, defaultExpr); err != nil {
				return err
			}
		}
	}
	for _, keyword := range node.KeywordParams {
		if err := c.compileParameterDefault(keyword.Name, keyword.Default); err != nil {
			return err
		}
	}

	if node.Body != nil {
		if err := c.compileBlockAsValue(node.Body); err != nil {
			return err
		}
	}

	c.replaceLastPopWithReturn()

	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	localNames := c.localNames()
	lineMap := c.currentLineMapCopy()
	instructions := c.LeaveScope()

	compiledParamDefaults := make([]*object.EmeraldValue, len(params))
	for i, defaultExpr := range paramDefaults {
		if i >= len(compiledParamDefaults) {
			break
		}
		if defaultExpr != nil {
			compiledParamDefaults[i] = c.compileDefaultValue(defaultExpr)
		}
	}
	keywordParams := make([]object.KeywordParamInfo, len(node.KeywordParams))
	for i, keyword := range node.KeywordParams {
		keywordParams[i] = object.KeywordParamInfo{Name: keyword.Name, HasDefault: keyword.Default != nil}
		if keyword.Default != nil {
			keywordParams[i].Default = c.compileDefaultValue(keyword.Default)
		}
	}

	fnObj := &object.Function{
		Name:                  "__lambda__",
		SourcePath:            core.CurrentSpecFile,
		EvalSource:            core.CurrentEvalSource,
		SourceEncoding:        c.sourceEncoding,
		Params:                identifierNames(params),
		ParamLocalIndices:     paramLocalIndices,
		ParamPatterns:         compileParameterPatternsIfAligned(node.ParamPatterns, params, node.Params),
		ParamDefaults:         compiledParamDefaults,
		EvaluateParamDefaults: true,
		Instructions:          instructions,
		LineMap:               lineMap,
		NumLocals:             numLocals,
		GlobalNames:           c.globalNamesCopy(),
		LocalNames:            localNames,
		KeywordParams:         keywordParams,
		RejectKeywords:        node.RejectKeywords,
		KeywordRestOnly:       node.KeywordRestOnly,
		RejectBlock:           node.RejectBlock,
		FreeVarNames:          freeVarNames(free),
		ImplicitItParameter:   implicitIt,
		NumberedParameters:    numberedParamCount > 0,
	}
	if node.KeywordRestParam != nil {
		fnObj.KeywordRestParam = node.KeywordRestParam.Value
	}
	if node.RestParam != nil {
		fnObj.HasRestParam = true
		fnObj.AnonymousRestParam = anonymousRestParam
		fnObj.RestParamIndex = restIndex
		if !anonymousRestParam {
			fnObj.RestParamName = node.RestParam.Value
		}
	}
	if node.BlockParam != nil {
		fnObj.HasBlockParam = true
		fnObj.AnonymousBlockParam = node.BlockParam.Value == "_" && node.BlockParam.Token.Literal == "&"
		fnObj.BlockParamIndex = blockParamIndex
	}

	fn := &object.EmeraldValue{
		Type:  object.ValueFunction,
		Data:  fnObj,
		Class: core.R.Classes["Class"],
	}
	fnIdx := c.addConstant(fn)
	for _, s := range free {
		c.emitCaptureSymbol(s)
	}
	c.emit(OpLambda, fnIdx, len(free))

	return nil
}

func identifierNames(params []*ast.Identifier) []string {
	names := make([]string, len(params))
	for i, param := range params {
		names[i] = param.Value
	}
	return names
}

func paramsMatchAST(params, original []*ast.Identifier) bool {
	if len(params) != len(original) {
		return false
	}
	for i := range params {
		if params[i] != original[i] {
			return false
		}
	}
	return true
}

func compileParameterPatternsIfAligned(patterns []*ast.ParameterPattern, params, original []*ast.Identifier) []*object.ParameterPattern {
	if !paramsMatchAST(params, original) {
		return nil
	}
	return compileParameterPatterns(patterns, len(params))
}

func compileParameterPatterns(patterns []*ast.ParameterPattern, count int) []*object.ParameterPattern {
	if len(patterns) == 0 {
		return nil
	}
	result := make([]*object.ParameterPattern, count)
	for i := 0; i < count && i < len(patterns); i++ {
		result[i] = compileParameterPattern(patterns[i])
	}
	return result
}

func compileParameterPattern(pattern *ast.ParameterPattern) *object.ParameterPattern {
	if pattern == nil {
		return nil
	}
	result := &object.ParameterPattern{RestIndex: pattern.RestIndex}
	if pattern.Name != nil {
		result.Name = pattern.Name.Value
	}
	result.Children = make([]*object.ParameterPattern, len(pattern.Children))
	for i, child := range pattern.Children {
		result.Children[i] = compileParameterPattern(child)
	}
	result.Rest = compileParameterPattern(pattern.Rest)
	return result
}

func (c *Compiler) defineParameterPatternLocals(patterns []*ast.ParameterPattern) {
	for _, pattern := range patterns {
		c.defineParameterPatternLocal(pattern)
	}
}

func (c *Compiler) defineParameterPatternLocal(pattern *ast.ParameterPattern) {
	if pattern == nil {
		return
	}
	if pattern.Name != nil {
		c.symbolTable.Define(pattern.Name.Value)
	}
	for _, child := range pattern.Children {
		c.defineParameterPatternLocal(child)
	}
	c.defineParameterPatternLocal(pattern.Rest)
}

func isNestedMultiAssignTarget(target ast.Expression) bool {
	switch target.(type) {
	case *ast.ArrayLiteral:
		return true
	default:
		return false
	}
}

type multiAssignTargetContext struct {
	receiver *Symbol
	args     []*Symbol
}

func (c *Compiler) newHiddenTemp() Symbol {
	name := fmt.Sprintf("\x00rgo_multi_%d", c.tempCounter)
	c.tempCounter++
	return c.symbolTable.Define(name)
}

func (c *Compiler) storeMultiAssignExpression(expr ast.Expression) (*Symbol, error) {
	if err := c.Compile(expr); err != nil {
		return nil, err
	}
	temp := c.newHiddenTemp()
	c.emit(OpSetLocal, temp.Index)
	c.Emit(OpPop)
	return &temp, nil
}

func (c *Compiler) prepareMultiAssignTarget(target ast.Expression, contexts map[ast.Expression]*multiAssignTargetContext) error {
	switch node := target.(type) {
	case *ast.ArrayLiteral:
		for _, child := range node.Elements {
			if err := c.prepareMultiAssignTarget(child, contexts); err != nil {
				return err
			}
		}
	case *ast.SplatExpression:
		return c.prepareMultiAssignTarget(node.Value, contexts)
	case *ast.MethodCall:
		if node.Receiver == nil {
			return nil
		}
		ctx := &multiAssignTargetContext{}
		var err error
		ctx.receiver, err = c.storeMultiAssignExpression(node.Receiver)
		if err != nil {
			return err
		}
		if node.Method != nil && node.Method.Value == "[]" {
			for _, arg := range node.Args {
				temp, err := c.storeMultiAssignExpression(arg)
				if err != nil {
					return err
				}
				ctx.args = append(ctx.args, temp)
			}
		}
		contexts[target] = ctx
	case *ast.IndexExpression:
		ctx := &multiAssignTargetContext{}
		var err error
		ctx.receiver, err = c.storeMultiAssignExpression(node.Left)
		if err != nil {
			return err
		}
		for _, expr := range []ast.Expression{node.Index, node.End} {
			if expr == nil {
				continue
			}
			temp, err := c.storeMultiAssignExpression(expr)
			if err != nil {
				return err
			}
			ctx.args = append(ctx.args, temp)
		}
		contexts[target] = ctx
	case *ast.ConstantResolution:
		if node.Left != nil {
			receiver, err := c.storeMultiAssignExpression(node.Left)
			if err != nil {
				return err
			}
			contexts[target] = &multiAssignTargetContext{receiver: receiver}
		}
	}
	return nil
}

func (c *Compiler) emitHiddenTemp(temp *Symbol) {
	c.emit(OpGetLocal, temp.Index)
}

func multiAssignSplatLayout(targets []ast.Expression) (int, int, int) {
	for i, target := range targets {
		if _, ok := target.(*ast.SplatExpression); ok {
			return i, i, len(targets) - i - 1
		}
	}
	return -1, len(targets), 0
}

func (c *Compiler) emitMultiAssignExtract(position int, splatIndex int, preCount int, postCount int) {
	kind := 0
	index := position
	if splatIndex >= 0 {
		switch {
		case position == splatIndex:
			kind = 1
			index = 0
		case position > splatIndex:
			kind = 2
			index = position - splatIndex - 1
		}
	}
	c.emit(OpMultiAssignExtract, kind, index, preCount, postCount)
}

func (c *Compiler) compileMultiAssignTargets(targets []ast.Expression, contexts map[ast.Expression]*multiAssignTargetContext) error {
	splatIndex, preCount, postCount := multiAssignSplatLayout(targets)
	for i, target := range targets {
		c.Emit(OpDup)
		c.emitMultiAssignExtract(i, splatIndex, preCount, postCount)
		if err := c.compileMultiAssignTarget(target, contexts); err != nil {
			return err
		}
		c.Emit(OpPop)
	}
	return nil
}

func (c *Compiler) compileMultiAssignTarget(target ast.Expression, contexts map[ast.Expression]*multiAssignTargetContext) error {
	switch node := target.(type) {
	case *ast.ArrayLiteral:
		c.Emit(OpMultiAssignPrepare)
		return c.compileMultiAssignTargets(node.Elements, contexts)
	case *ast.SplatExpression:
		return c.compileMultiAssignTarget(node.Value, contexts)
	case *ast.MethodCall:
		ctx := contexts[target]
		if ctx == nil || ctx.receiver == nil || node.Method == nil {
			return c.compileMultiAssignName(assignmentTargetName(target))
		}
		value := c.newHiddenTemp()
		c.emit(OpSetLocal, value.Index)
		c.Emit(OpPop)
		c.emitHiddenTemp(ctx.receiver)
		for _, arg := range ctx.args {
			c.emitHiddenTemp(arg)
		}
		c.emitHiddenTemp(&value)
		method := node.Method.Value + "="
		if node.Method.Value == "[]" {
			method = "[]="
		}
		methodIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: method, Class: core.R.Classes["String"]})
		c.emit(OpSendSetter, methodIdx, 0, len(ctx.args)+1, 255)
		return nil
	case *ast.IndexExpression:
		ctx := contexts[target]
		if ctx == nil || ctx.receiver == nil {
			return fmt.Errorf("missing multiple assignment index context")
		}
		value := c.newHiddenTemp()
		c.emit(OpSetLocal, value.Index)
		c.Emit(OpPop)
		c.emitHiddenTemp(ctx.receiver)
		for _, arg := range ctx.args {
			c.emitHiddenTemp(arg)
		}
		c.emitHiddenTemp(&value)
		methodIdx := c.addConstant(&object.EmeraldValue{Type: object.ValueString, Data: "[]=", Class: core.R.Classes["String"]})
		c.emit(OpSendSetter, methodIdx, 0, len(ctx.args)+1, 255)
		return nil
	case *ast.ConstantResolution:
		ctx := contexts[target]
		if ctx == nil || ctx.receiver == nil {
			return c.compileMultiAssignName(node.Name)
		}
		value := c.newHiddenTemp()
		c.emit(OpSetLocal, value.Index)
		c.Emit(OpPop)
		c.emitHiddenTemp(ctx.receiver)
		c.emitHiddenTemp(&value)
		name := &object.EmeraldValue{Type: object.ValueString, Data: node.Name.Value, Class: core.R.Classes["String"]}
		c.emit(OpSetScopedConstant, c.addConstant(name), ScopedConstAssignPlain)
		return nil
	default:
		return c.compileMultiAssignName(assignmentTargetName(target))
	}
}

func assignmentTargetName(target ast.Expression) *ast.Identifier {
	switch node := target.(type) {
	case *ast.Identifier:
		return node
	case *ast.InstanceVariable:
		return &ast.Identifier{Token: node.Token, Value: node.Name}
	case *ast.ClassVariable:
		return &ast.Identifier{Token: node.Token, Value: node.Name}
	case *ast.GlobalVariable:
		return &ast.Identifier{Token: node.Token, Value: node.Name}
	case *ast.Constant:
		return &ast.Identifier{Token: node.Token, Value: node.Name}
	}
	return nil
}

func (c *Compiler) compileMultiAssignName(name *ast.Identifier) error {
	if name == nil {
		return fmt.Errorf("invalid multiple assignment target")
	}
	if strings.HasPrefix(name.Value, "$") {
		c.emit(OpSetGlobal, c.globalSymbolIndex(name.Value))
		return nil
	}
	if strings.HasPrefix(name.Value, "@@") {
		value := &object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}
		c.emit(OpSetClassVar, c.addConstant(value))
		return nil
	}
	if strings.HasPrefix(name.Value, "@") {
		value := &object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}
		c.emit(OpSetInstanceVar, c.addConstant(value))
		return nil
	}
	if isConstantName(name.Value) {
		value := &object.EmeraldValue{Type: object.ValueString, Data: name.Value, Class: core.R.Classes["String"]}
		c.emit(OpSetConstant, c.addConstant(value), 0)
		return nil
	}
	if sym, ok := c.symbolTable.Resolve(name.Value); !ok || sym.Scope == ScopeBuiltin {
		c.symbolTable.Define(name.Value)
	}
	sym, _ := c.symbolTable.Resolve(name.Value)
	switch sym.Scope {
	case ScopeGlobal:
		c.emit(OpSetGlobal, sym.Index)
	case ScopeLocal:
		c.emit(OpSetLocal, sym.Index)
	case ScopeOuter:
		c.emit(OpSetOuter, 0, sym.ScopeIndex)
	case ScopeOuterFree:
		c.emit(OpSetOuterFree, sym.ScopeIndex)
	case ScopeFree:
		c.emit(OpSetFree, sym.Index)
	}
	return nil
}

func isSplatMultiAssignTarget(target ast.Expression) bool {
	_, ok := target.(*ast.SplatExpression)
	return ok
}

func multiAssignHasNestedTarget(node *ast.MultiAssignExpression) bool {
	if node == nil {
		return false
	}
	for _, target := range node.Targets {
		if isNestedMultiAssignTarget(target) {
			return true
		}
	}
	return false
}

func isConstantName(name string) bool {
	if name == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}
