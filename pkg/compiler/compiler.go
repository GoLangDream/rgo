package compiler

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

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
	Outer       *SymbolTable
	store       map[string]Symbol
	FreeSymbols []Symbol
	MaxSymbols  int
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
	if !ok && s.Outer != nil {
		if outerObj, found := s.Outer.store[name]; found && outerObj.Scope == ScopeLocal {
			free := s.DefineFree(outerObj)
			return free, true
		}
		if outerObj, found := s.Outer.store[name]; found && outerObj.Scope == ScopeFree {
			return Symbol{
				Name:       outerObj.Name,
				Index:      outerObj.Index,
				Scope:      ScopeOuterFree,
				ScopeIndex: outerObj.Index,
			}, true
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
	constants    []*object.EmeraldValue
	scopes       []CompilationScope
	scopeIndex   int
	symbolTable  *SymbolTable
	methodDepth  int
	currentLine  int
	forEachDepth int
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
		constants:   []*object.EmeraldValue{},
		scopes:      []CompilationScope{mainScope},
		symbolTable: symbolTable,
	}
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
		for _, s := range node.Statements {
			if err := c.Compile(s); err != nil {
				return err
			}
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
		if err := c.Compile(node.Condition); err != nil {
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
		parts := strings.Split(node.Value, ".")
		num := int64(0)
		den := int64(1)
		if len(parts) == 1 {
			n, _ := strconv.ParseInt(parts[0], 10, 64)
			num = n
		} else if len(parts) == 2 {
			n, _ := strconv.ParseInt(parts[0], 10, 64)
			d, _ := strconv.ParseInt(parts[1], 10, 64)
			if d == 0 {
				num = n
			} else {
				places := len(parts[1])
				mul := int64(1)
				for i := 0; i < places; i++ {
					mul *= 10
				}
				num = n*mul + d
				den = mul
			}
		}
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  num,
			Class: core.R.Classes["Integer"],
		})
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  den,
			Class: core.R.Classes["Integer"],
		})
		c.emit(OpRationalNew)
	case *ast.StringLiteral:
		if node.Command {
			c.Emit(OpSelf)
		}
		if err := c.compileStringLiteralValue(node); err != nil {
			return err
		}
		if node.Command {
			methodNameIdx := c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "`",
				Class: core.R.Classes["String"],
			})
			c.emit(OpSend, methodNameIdx, 0, 1, 255)
		}
	case *ast.SymbolLiteral:
		val := normalizedStaticSymbolName(node.Value)
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  val,
			Class: core.R.Classes["Symbol"],
		})
	case *ast.RegexpLiteral:
		if node.Interpolates && strings.Contains(node.Pattern, "#{") {
			c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "Regexp",
				Class: core.R.Classes["String"],
			}))
			if err := c.compileStringInterpolation(node.Pattern); err != nil {
				return err
			}
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "new",
				Class: core.R.Classes["String"],
			}), 0, 1, 255)
			return nil
		}
		c.EmitConstant(&object.EmeraldValue{
			Type: object.ValueRegexp,
			Data: &object.RRegexp{
				Pattern: strings.ReplaceAll(node.Pattern, lexer.EscapedHashInterpolation, "#"),
				Options: node.Options,
			},
			Class: core.R.Classes["Regexp"],
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
			}), 0, 0, 255)
			return nil
		}
		switch sym.Scope {
		case ScopeGlobal:
			c.emit(OpGetGlobal, sym.Index)
		case ScopeLocal:
			c.emit(OpGetLocal, sym.Index)
		case ScopeBuiltin:
			c.Emit(OpNil)
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
			c.emit(OpGetScopedConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
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
			jumpPos := c.emit(OpJumpNotTruthy, 9999)
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			jumpToEndPos := c.emit(OpJump, 9999)
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			c.emit(OpFalse)
			afterFalse := len(c.currentInstructions())
			c.changeOperand(jumpToEndPos, afterFalse)
			return nil
		}
		if node.Operator == "||" || node.Operator == "or" {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			jumpPos := c.emit(OpJumpTruthy, 9999)
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			jumpToEndPos := c.emit(OpJump, 9999)
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			c.emit(OpTrue)
			afterTrue := len(c.currentInstructions())
			c.changeOperand(jumpToEndPos, afterTrue)
			return nil
		}

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
				switch receiver := call.Receiver.(type) {
				case *ast.IntegerLiteral:
					copyCall := *call
					copyReceiver := *receiver
					copyReceiver.Value = -copyReceiver.Value
					copyCall.Receiver = &copyReceiver
					return c.Compile(&copyCall)
				case *ast.FloatLiteral:
					copyCall := *call
					copyReceiver := *receiver
					copyReceiver.Value = -copyReceiver.Value
					copyCall.Receiver = &copyReceiver
					return c.Compile(&copyCall)
				}
			}
		}
		if err := c.Compile(node.Right); err != nil {
			return err
		}

		switch node.Operator {
		case "!", "not":
			c.Emit(OpBang)
		case "-":
			c.Emit(OpNeg)
		case "~":
			c.Emit(OpBitNot)
		}
	case *ast.IfExpression:
		if err := c.Compile(node.Condition); err != nil {
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
				if err := c.Compile(elsif.Condition); err != nil {
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
					if err := c.Compile(cond); err != nil {
						return err
					}
					methodNameIdx := c.addConstant(&object.EmeraldValue{
						Type:  object.ValueString,
						Data:  "===",
						Class: core.R.Classes["String"],
					})
					c.emit(OpSend, methodNameIdx, 0, 1, 255)
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
				c.Emit(OpPop)
			}
			c.Emit(OpNil)
		}
		afterAll := len(c.currentInstructions())
		for _, pos := range jumpToEndPositions {
			c.changeOperand(pos, afterAll)
		}
	case *ast.PatternMatchExpression:
		if node.Left != nil {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			if node.Pattern != "" {
				c.emit(OpPatternCheck, c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  node.Pattern,
					Class: core.R.Classes["String"],
				}))
			}
			c.Emit(OpPop)
		} else if node.Pattern != "" {
			c.emit(OpPatternCheck, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Pattern,
				Class: core.R.Classes["String"],
			}))
		}
		c.Emit(OpTrue)
	case *ast.ArrayLiteral:
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
				c.emit(OpSend, methodNameIdx, 0, 3, 255)
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

		if op, ok := compoundAssignmentOpcode(node.Token.Type); ok {
			if err := c.compileAssignmentCurrentValue(node.Name); err != nil {
				return err
			}
			if err := c.Compile(node.Value); err != nil {
				return err
			}
			c.Emit(op)
		} else if err := c.Compile(node.Value); err != nil {
			return err
		}
		if isSplatMultiAssignTarget(node.Target) {
			c.Emit(OpMultiAssignPrepare)
		}

		// Check if the name is a global variable (starts with $)
		if len(node.Name.Value) > 0 && node.Name.Value[0] == '$' {
			c.emit(OpSetGlobal, c.globalSymbolIndex(node.Name.Value))
			return nil
		}

		// Check if the name is a class variable (starts with @@)
		if len(node.Name.Value) > 1 && node.Name.Value[0] == '@' && node.Name.Value[1] == '@' {
			c.emit(OpSetClassVar, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			return nil
		}

		// Check if the name is an instance variable (starts with @)
		if len(node.Name.Value) > 0 && node.Name.Value[0] == '@' {
			c.emit(OpSetInstanceVar, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
			return nil
		}

		if isConstantName(node.Name.Value) {
			c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
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
	case *ast.MultiAssignExpression:
		if len(node.Values) == 1 && len(node.Names) > 1 {
			if err := c.Compile(node.Values[0]); err != nil {
				return err
			}
			c.Emit(OpMultiAssignPrepare)
			for i := 0; i < len(node.Names); i++ {
				c.Emit(OpDup)
				c.EmitConstant(&object.EmeraldValue{
					Type:  object.ValueInteger,
					Data:  int64(i),
					Class: core.R.Classes["Integer"],
				})
				c.Emit(OpIndex)
				name := node.Names[i]
				if len(name.Value) > 0 && name.Value[0] == '$' {
					sym := c.symbolTable.DefineGlobal(name.Value)
					c.emit(OpSetGlobal, sym.Index)
				} else if len(name.Value) > 1 && name.Value[0] == '@' && name.Value[1] == '@' {
					c.emit(OpSetClassVar, c.addConstant(&object.EmeraldValue{
						Type:  object.ValueString,
						Data:  name.Value,
						Class: core.R.Classes["String"],
					}))
				} else if len(name.Value) > 0 && name.Value[0] == '@' {
					c.emit(OpSetInstanceVar, c.addConstant(&object.EmeraldValue{
						Type:  object.ValueString,
						Data:  name.Value,
						Class: core.R.Classes["String"],
					}))
				} else {
					if _, ok := c.symbolTable.Resolve(name.Value); !ok {
						c.symbolTable.Define(name.Value)
					}
					sym, _ := c.symbolTable.Resolve(name.Value)
					switch sym.Scope {
					case ScopeGlobal:
						c.emit(OpSetGlobal, sym.Index)
					case ScopeLocal:
						c.emit(OpSetLocal, sym.Index)
					}
				}
				c.Emit(OpPop)
			}
			c.Emit(OpPop)
		} else {
			hasNestedTarget := multiAssignHasNestedTarget(node)
			for i, val := range node.Values {
				if splat, ok := val.(*ast.SplatExpression); ok {
					if err := c.Compile(splat.Value); err != nil {
						return err
					}
					if hasNestedTarget {
						c.Emit(OpDup)
						c.Emit(OpMultiAssignCheckToAry)
					}
					c.Emit(OpSplatToA)
				} else {
					if err := c.Compile(val); err != nil {
						return err
					}
					if i < len(node.Targets) && isNestedMultiAssignTarget(node.Targets[i]) {
						c.Emit(OpMultiAssignPrepare)
					} else if len(node.Values) == 1 && i < len(node.Targets) && isSplatMultiAssignTarget(node.Targets[i]) {
						c.Emit(OpMultiAssignPrepare)
					}
				}
			}
			for i := len(node.Names) - 1; i >= 0; i-- {
				name := node.Names[i]
				if len(name.Value) > 0 && name.Value[0] == '$' {
					sym := c.symbolTable.DefineGlobal(name.Value)
					c.emit(OpSetGlobal, sym.Index)
				} else if len(name.Value) > 1 && name.Value[0] == '@' && name.Value[1] == '@' {
					c.emit(OpSetClassVar, c.addConstant(&object.EmeraldValue{
						Type:  object.ValueString,
						Data:  name.Value,
						Class: core.R.Classes["String"],
					}))
				} else if len(name.Value) > 0 && name.Value[0] == '@' {
					c.emit(OpSetInstanceVar, c.addConstant(&object.EmeraldValue{
						Type:  object.ValueString,
						Data:  name.Value,
						Class: core.R.Classes["String"],
					}))
				} else {
					if _, ok := c.symbolTable.Resolve(name.Value); !ok {
						c.symbolTable.Define(name.Value)
					}
					sym, _ := c.symbolTable.Resolve(name.Value)
					switch sym.Scope {
					case ScopeGlobal:
						c.emit(OpSetGlobal, sym.Index)
					case ScopeLocal:
						c.emit(OpSetLocal, sym.Index)
					}
				}
				c.Emit(OpPop)
			}
		}
	case *ast.MethodCall:
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

		args := node.Args
		var blockPass ast.Expression
		var blockPassAnonymous bool
		if len(args) > 0 {
			if splat, ok := args[len(args)-1].(*ast.SplatExpression); ok && splat.Token.Type == lexer.BIT_AND {
				blockPass = splat.Value
				blockPassAnonymous = splat.AnonymousBlockPass
				args = args[:len(args)-1]
			}
		}

		splatIndex := 255
		for i, arg := range args {
			compileArg := arg
			if splat, ok := arg.(*ast.SplatExpression); ok && splat.Token.Type != lexer.BIT_AND {
				compileArg = splat.Value
				if splat.Token.Type == lexer.MULTIPLY && splatIndex == 255 {
					splatIndex = i
				}
			}
			if err := c.Compile(compileArg); err != nil {
				return err
			}
		}

		argCount := len(args)

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
					Type:  object.ValueString,
					Data:  ":" + kwa.Name,
					Class: core.R.Classes["String"],
				})
			}
			c.emit(OpHash, len(node.KeywordArgs))
			argCount++
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
			Data:  node.New.String(),
			Class: core.R.Classes["Symbol"],
		})
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  node.Old.String(),
			Class: core.R.Classes["Symbol"],
		})
		c.emit(OpAlias)
	case *ast.ReturnExpression:
		if c.scopeIndex == 0 && c.methodDepth == 0 && node.ReturnValue != nil {
			fmt.Fprintln(os.Stderr, "warning: argument of top-level return is ignored")
		}
		if node.ReturnValue != nil {
			if err := c.Compile(node.ReturnValue); err != nil {
				return err
			}
		} else {
			c.Emit(OpNil)
		}
		c.Emit(OpReturnValue)
	case *ast.DefExpression:
		c.EnterScope()

		for _, param := range node.Params {
			c.symbolTable.Define(param.Value)
		}

		if node.RestParam != nil {
			c.symbolTable.Define(node.RestParam.Value)
		}

		for _, kp := range node.KeywordParams {
			c.symbolTable.Define(kp.Name)
		}
		if node.KeywordRestParam != nil {
			c.symbolTable.Define(node.KeywordRestParam.Value)
		}

		if node.BlockParam != nil {
			c.symbolTable.Define(node.BlockParam.Value)
		}

		c.methodDepth++
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
			Name:            node.Name.Value,
			Params:          params,
			Instructions:    instructions,
			LineMap:         lineMap,
			NumLocals:       numLocals,
			GlobalNames:     c.globalNamesCopy(),
			LocalNames:      localNames,
			ParamDefaults:   paramDefaults,
			KeywordParams:   kwParams,
			RejectKeywords:  node.RejectKeywords,
			KeywordRestOnly: node.KeywordRestOnly,
			RejectBlock:     node.RejectBlock,
			FreeVarNames:    freeVarNames(free),
		}
		if node.KeywordRestParam != nil {
			fnObj.KeywordRestParam = node.KeywordRestParam.Value
		}
		if node.RestParam != nil {
			fnObj.HasRestParam = true
			fnObj.RestParamIndex = len(node.Params)
		}
		if node.BlockParam != nil {
			fnObj.HasBlockParam = true
			fnObj.BlockParamIndex = numLocals - 1
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
		if node.SingletonReceiver != nil {
			if err := c.Compile(node.SingletonReceiver); err != nil {
				return err
			}
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "singleton_class",
				Class: core.R.Classes["String"],
			}), 0, 0, 255)
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
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}), hasSuperclass)

			if node.SuperClass != nil {
				c.Emit(OpInherited)
			}
		}

		c.EnterScope()

		if err := c.Compile(node.Body); err != nil {
			return err
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
				Name:         node.Name.Value + "#body",
				Instructions: instructions,
				LineMap:      lineMap,
				NumLocals:    numLocals,
				GlobalNames:  c.globalNamesCopy(),
				LocalNames:   localNames,
				FreeVarNames: freeVarNames(c.symbolTable.FreeSymbols),
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
				Data:  node.Name.Value,
				Class: core.R.Classes["String"],
			}))
		}
	case *ast.ModuleExpression:
		c.emit(OpModule, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))

		c.EnterScope()
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
				Name:         node.Name.Value + "#body",
				Instructions: instructions,
				LineMap:      lineMap,
				NumLocals:    numLocals,
				GlobalNames:  c.globalNamesCopy(),
				LocalNames:   localNames,
				FreeVarNames: freeVarNames(c.symbolTable.FreeSymbols),
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
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))
		c.Emit(OpPop)
	case *ast.BlockExpression:
		// If block has params, compile as closure
		if len(node.Params) > 0 {
			c.EnterScope()

			for _, param := range node.Params {
				c.symbolTable.Define(param.Value)
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
				Name:         "__block__",
				Instructions: instructions,
				LineMap:      lineMap,
				NumLocals:    numLocals,
				LocalNames:   localNames,
				FreeVarNames: freeVarNames(free),
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
		loopStart := len(c.currentInstructions())

		if err := c.Compile(node.Condition); err != nil {
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
		// until is like while with negated condition
		loopStart := len(c.currentInstructions())

		if err := c.Compile(node.Condition); err != nil {
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
			if err := c.Compile(node.Value); err != nil {
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
		pos := c.emit(OpJump, 0)
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
					if splatIndex < 0 {
						splatIndex = i
					}
				}
				if err := c.Compile(compileArg); err != nil {
					return err
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
			c.emit(OpSend, c.addConstant(&object.EmeraldValue{
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
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "RuntimeError",
				Class: core.R.Classes["String"],
			})
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
		for _, arg := range node.Args {
			if err := c.Compile(arg); err != nil {
				return err
			}
		}
		blockArg := 0
		if node.Block != nil {
			if err := c.compileBlockAsClosure(node.Block); err != nil {
				return err
			}
			blockArg = 1
		}
		argCount := len(node.Args)
		if node.ImplicitArgs {
			argCount = 255
		}
		c.emit(OpSendSuper, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__super__",
			Class: core.R.Classes["String"],
		}), blockArg, argCount)
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
	implicitIt := c.blockUsesImplicitItParameter(block)
	c.EnterScope()

	params := block.Params
	paramDefaults := block.ParamDefaults
	if implicitIt {
		params = []*ast.Identifier{{Value: "it"}}
		paramDefaults = []ast.Expression{nil}
	}
	restIndex := block.RestParamIndex
	if restIndex < 0 || restIndex > len(params) {
		restIndex = len(params)
	}
	for _, param := range params[:restIndex] {
		c.symbolTable.Define(param.Value)
	}
	if block.RestParam != nil {
		c.symbolTable.Define(block.RestParam.Value)
	}
	for _, param := range params[restIndex:] {
		c.symbolTable.Define(param.Value)
	}
	if block.BlockParam != nil {
		c.symbolTable.Define(block.BlockParam.Value)
	}
	for _, kp := range block.KeywordParams {
		c.symbolTable.Define(kp.Name)
	}
	for _, localName := range block.BlockLocals {
		c.symbolTable.Define(localName)
	}

	if err := c.compileBlockAsValue(block); err != nil {
		return err
	}

	c.replaceLastPopWithReturn()

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
		Name:                 "__block__",
		Params:               identifierNames(params),
		ParamDefaults:        compiledParamDefaults,
		BlockLocals:          append([]string(nil), block.BlockLocals...),
		Instructions:         instructions,
		LineMap:              lineMap,
		NumLocals:            numLocals,
		GlobalNames:          c.globalNamesCopy(),
		LocalNames:           localNames,
		KeywordParams:        kwParams,
		RejectKeywords:       block.RejectKeywords,
		SingleDestructure:    block.SingleDestructure,
		KeywordRestOnly:      block.KeywordRestOnly,
		RejectBlock:          block.RejectBlock,
		FreeVarNames:         freeVarNames(free),
		TrailingCommaParam:   block.TrailingComma,
		ForLoopCollectAsPair: forLoopCollectAsPair,
	}
	if block.RestParam != nil {
		fnObj.HasRestParam = true
		fnObj.RestParamIndex = restIndex
	}
	if block.BlockParam != nil {
		fnObj.HasBlockParam = true
		fnObj.BlockParamIndex = numLocals - 1
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
	for i, s := range block.Statements {
		if i == len(block.Statements)-1 {
			if exprStmt, ok := s.(*ast.ExpressionStatement); ok {
				if err := c.Compile(exprStmt.Expression); err != nil {
					return err
				}
				break
			}
		}
		if err := c.Compile(s); err != nil {
			return err
		}
	}
	c.removeLastPop()
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
		if node.Name != nil && node.Name.Value == "it" {
			return true
		}
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

	beginPos := c.emit(OpBeginRescue, 0, 0, 0)

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
			if sym.Scope == ScopeLocal {
				c.emit(OpSetLocal, sym.Index)
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
		if hasEnsure {
			c.Emit(OpEnsure)
			if err := c.compileBlockAsValue(node.Ensure); err != nil {
				return err
			}
			c.Emit(OpPop)
		}
		c.Emit(OpReraise)
	}

	elseStart := 0
	if hasElse {
		elseStart = len(c.currentInstructions())
		if err := c.compileBlockAsValue(node.Else); err != nil {
			return err
		}
	}

	ensureStart := len(c.currentInstructions())
	if hasEnsure {
		c.Emit(OpEnsure)
		if err := c.compileBlockAsValue(node.Ensure); err != nil {
			return err
		}
		c.Emit(OpPop)
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
			c.changeOperand(jump, ensureStart)
		} else {
			c.changeOperand(jump, endRescueStart)
		}
	}
	c.changeOperandAt(beginPos, 0, rescueStart)
	c.changeOperandAt(beginPos, 1, ensureStart)
	c.changeOperandAt(beginPos, 2, endStart)

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
	if !node.Interpolates || !strings.Contains(val, "#{") {
		val = strings.ReplaceAll(val, lexer.EscapedHashInterpolation, "#")
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  val,
			Class: core.R.Classes["String"],
		})
		return nil
	}
	return c.compileStringInterpolation(val)
}

func (c *Compiler) compileStringInterpolation(s string) error {
	parts := splitStringInterpolation(s)
	if len(parts) == 0 {
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "",
			Class: core.R.Classes["String"],
		})
		return nil
	}

	first := true
	for _, part := range parts {
		if part.isExpr {
			l := lexer.New(part.text)
			p := parser.New(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				c.EmitConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  strings.ReplaceAll("#{"+part.text+"}", lexer.EscapedHashInterpolation, "#"),
					Class: core.R.Classes["String"],
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
					Type:  object.ValueString,
					Data:  "",
					Class: core.R.Classes["String"],
				})
			}
		} else {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  strings.ReplaceAll(part.text, lexer.EscapedHashInterpolation, "#"),
				Class: core.R.Classes["String"],
			})
		}
		if !first {
			c.Emit(OpAdd)
		}
		first = false
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
		} else {
			i++
		}
	}
	if start < len(s) {
		parts = append(parts, interpPart{text: s[start:], isExpr: false})
	}
	return parts
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
			Name:         "__scoped_const_rhs__",
			Instructions: instructions,
			LineMap:      lineMap,
			NumLocals:    numLocals,
			GlobalNames:  c.globalNamesCopy(),
			LocalNames:   localNames,
			FreeVarNames: freeVarNames(free),
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
	c.emit(OpRange, exclusive)
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
	implicitIt := c.procLiteralUsesImplicitItParameter(node)
	c.EnterScope()

	params := node.Params
	paramDefaults := node.ParamDefaults
	if implicitIt {
		params = []*ast.Identifier{{Value: "it"}}
		paramDefaults = []ast.Expression{nil}
	}
	restIndex := node.RestParamIndex
	if restIndex < 0 || restIndex > len(params) {
		restIndex = len(params)
	}
	for _, param := range params[:restIndex] {
		c.symbolTable.Define(param.Value)
	}
	if node.RestParam != nil {
		c.symbolTable.Define(node.RestParam.Value)
	}
	for _, param := range params[restIndex:] {
		c.symbolTable.Define(param.Value)
	}
	if node.BlockParam != nil {
		c.symbolTable.Define(node.BlockParam.Value)
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

	fnObj := &object.Function{
		Name:           "__lambda__",
		Params:         identifierNames(params),
		ParamDefaults:  compiledParamDefaults,
		Instructions:   instructions,
		LineMap:        lineMap,
		NumLocals:      numLocals,
		GlobalNames:    c.globalNamesCopy(),
		LocalNames:     localNames,
		RejectKeywords: node.RejectKeywords,
		RejectBlock:    node.RejectBlock,
		FreeVarNames:   freeVarNames(free),
	}
	if node.RestParam != nil {
		fnObj.HasRestParam = true
		fnObj.RestParamIndex = restIndex
	}
	if node.BlockParam != nil {
		fnObj.HasBlockParam = true
		fnObj.BlockParamIndex = numLocals - 1
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

func isNestedMultiAssignTarget(target ast.Expression) bool {
	switch target.(type) {
	case *ast.ArrayLiteral:
		return true
	default:
		return false
	}
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
