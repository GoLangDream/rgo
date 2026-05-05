package compiler

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

const (
	ScopeGlobal  = "global"
	ScopeLocal   = "local"
	ScopeBuiltin = "builtin"
	ScopeFree    = "free"
	ScopeOuter   = "outer"
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
			return Symbol{
				Name:       outerObj.Name,
				Index:      outerObj.Index,
				Scope:      ScopeOuter,
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
	lastInstruction     EmittedInstruction
	previousInstruction EmittedInstruction
	breakTarget         int
	nextPatchPos        []int
	redoTarget          int
	breakValuePatchPos  []int
	retryTarget         int
}

type Compiler struct {
	constants   []*object.EmeraldValue
	scopes      []CompilationScope
	scopeIndex  int
	symbolTable *SymbolTable
}

func New() *Compiler {
	mainScope := CompilationScope{
		instructions: Instructions{},
		breakTarget:  -1,
		redoTarget:   -1,
		retryTarget:  -1,
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

func (c *Compiler) globalSymbolIndex(name string) int {
	if sym, ok := c.symbolTable.Resolve(name); ok && sym.Scope == ScopeGlobal {
		return sym.Index
	}
	sym := c.symbolTable.DefineGlobal(name)
	return sym.Index
}

func (c *Compiler) Compile(node interface{}) error {
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
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  node.Value,
			Class: core.R.Classes["Integer"],
		})
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
		val := node.Value
		if !strings.Contains(val, "#{") {
			c.EmitConstant(&object.EmeraldValue{
				Type:  object.ValueString,
				Data:  val,
				Class: core.R.Classes["String"],
			})
		} else {
			if err := c.compileStringInterpolation(val); err != nil {
				return err
			}
		}
	case *ast.SymbolLiteral:
		val := node.Value
		if len(val) > 0 && val[0] == ':' {
			val = val[1:]
		}
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  val,
			Class: core.R.Classes["Symbol"],
		})
	case *ast.RegexpLiteral:
		c.EmitConstant(&object.EmeraldValue{
			Type: object.ValueRegexp,
			Data: &object.RRegexp{
				Pattern: node.Pattern,
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
			}), 0, 0)
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
			c.Emit(OpDup)
			jumpPos := c.emit(OpJumpNotTruthy, 9999)
			c.Emit(OpPop) // pop the duplicated left value
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			afterRight := len(c.currentInstructions())
			c.changeOperand(jumpPos, afterRight)
			return nil
		}
		if node.Operator == "||" || node.Operator == "or" {
			if err := c.Compile(node.Left); err != nil {
				return err
			}
			c.Emit(OpDup)
			jumpPos := c.emit(OpJumpTruthy, 9999)
			c.Emit(OpPop) // pop the duplicated left value
			if err := c.Compile(node.Right); err != nil {
				return err
			}
			afterRight := len(c.currentInstructions())
			c.changeOperand(jumpPos, afterRight)
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
			c.emit(OpSend, methodNameIdx, 0, 1)
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
			c.emit(OpSend, methodNameIdx, 0, 1)
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
					c.emit(OpSend, methodNameIdx, 0, 1)
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
			c.Emit(OpPop)
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

		if len(node.Name.Value) > 0 && node.Name.Value[0] >= 'A' && node.Name.Value[0] <= 'Z' {
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
		case ScopeFree:
			c.emit(OpSetFree, sym.Index)
		}
	case *ast.MultiAssignExpression:
		if len(node.Values) == 1 && len(node.Names) > 1 {
			if err := c.Compile(node.Values[0]); err != nil {
				return err
			}
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
					sym, _ := c.symbolTable.Resolve(name.Value)
					c.symbolTable.DefineGlobal(name.Value)
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
			for _, val := range node.Values {
				if err := c.Compile(val); err != nil {
					return err
				}
			}
			for i := len(node.Names) - 1; i >= 0; i-- {
				name := node.Names[i]
				if len(name.Value) > 0 && name.Value[0] == '$' {
					sym, _ := c.symbolTable.Resolve(name.Value)
					c.symbolTable.DefineGlobal(name.Value)
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
		if node.Receiver != nil {
			if err := c.Compile(node.Receiver); err != nil {
				return err
			}
		} else {
			c.Emit(OpSelf)
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

		for _, arg := range node.Args {
			if err := c.Compile(arg); err != nil {
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
					Type:  object.ValueString,
					Data:  ":" + kwa.Name,
					Class: core.R.Classes["String"],
				})
			}
			c.emit(OpHash, len(node.KeywordArgs))
			argCount++
		}

		blockArg := 0
		if node.Block != nil {
			if err := c.compileBlockAsClosure(node.Block); err != nil {
				return err
			}
			blockArg = 1
		}
		c.emit(OpSend, methodNameIdx, blockArg, argCount)
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
		c.emit(OpSend, methodNameIdx, 0, 1)
	case *ast.UndefExpression:
		c.Emit(OpNil)
	case *ast.AliasExpression:
		c.Emit(OpNil)
	case *ast.ReturnExpression:
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

		if node.BlockParam != nil {
			c.symbolTable.Define(node.BlockParam.Value)
		}

		if err := c.compileBlockAsValue(node.Body); err != nil {
			return err
		}

		c.replaceLastPopWithReturn()

		free := c.symbolTable.FreeSymbols
		numLocals := c.symbolTable.MaxSymbols

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

		fnObj := &object.Function{
			Name:          node.Name.Value,
			Instructions:  instructions,
			NumLocals:     numLocals,
			ParamDefaults: paramDefaults,
			KeywordParams: kwParams,
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

		for _, s := range free {
			c.emitCaptureSymbol(s)
		}
		c.emit(OpClosure, fnIdx, len(free))

		c.emit(OpDefineMethod, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))
	case *ast.ClassExpression:
		if node.SuperClass != nil {
			if node.SuperClass.Token.Type == lexer.CONSTANT || strings.Contains(node.SuperClass.Value, "::") {
				c.emit(OpGetConstant, c.addConstant(&object.EmeraldValue{
					Type:  object.ValueString,
					Data:  node.SuperClass.Value,
					Class: core.R.Classes["String"],
				}))
			} else {
				if err := c.Compile(node.SuperClass); err != nil {
					return err
				}
			}
		}

		c.emit(OpClass, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))

		if node.SuperClass != nil {
			c.Emit(OpInherited)
		}

		c.EnterScope()

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.Emit(OpReturnValue)

		instructions := c.LeaveScope()

		bodyFn := &object.EmeraldValue{
			Type: object.ValueFunction,
			Data: &object.Function{
				Name:         node.Name.Value + "#body",
				Instructions: instructions,
				NumLocals:    0,
			},
			Class: core.R.Classes["Class"],
		}
		fnIdx := c.addConstant(bodyFn)
		c.emit(OpClosure, fnIdx, 0)
		c.emit(OpSend, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__exec_class_body__",
			Class: core.R.Classes["String"],
		}), 1, 0)
		c.emit(OpSetConstant, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))
		c.Emit(OpPop)
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

		c.Emit(OpReturn)
		c.LeaveScope()
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
			instructions := c.LeaveScope()

			fnObj := &object.Function{
				Name:         "__block__",
				Instructions: instructions,
				NumLocals:    numLocals,
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
		c.scopes[c.scopeIndex].redoTarget = bodyStart

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
		c.scopes[c.scopeIndex].redoTarget = bodyStart

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.emit(OpJump, loopStart)

		afterBody := len(c.currentInstructions())
		c.changeOperand(jumpTruthyPos, afterBody)

		// until returns nil in Ruby
		c.Emit(OpNil)
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
		pos := c.emit(OpJump, 0)
		c.scopes[c.scopeIndex].nextPatchPos = append(c.scopes[c.scopeIndex].nextPatchPos, pos)
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
			for _, arg := range node.Args {
				if err := c.Compile(arg); err != nil {
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
						Type:  object.ValueString,
						Data:  ":" + kwa.Name,
						Class: core.R.Classes["String"],
					})
				}
				c.emit(OpHash, len(node.KeywordArgs))
				argCount++
			}
			c.emit(OpYieldWithValue, argCount)
		} else {
			c.Emit(OpYield)
		}
	case *ast.SelfExpression:
		c.Emit(OpSelf)
	case *ast.RaiseExpression:
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
		c.emit(OpSendSuper, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "__super__",
			Class: core.R.Classes["String"],
		}), blockArg, len(node.Args))
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
		Constants:    c.constants,
		NumLocals:    c.symbolTable.MaxSymbols,
	}
}

func (c *Compiler) currentInstructions() Instructions {
	return c.scopes[c.scopeIndex].instructions
}

// compileBlockAsValue compiles a BlockExpression.
// For blocks with params, this is called within an EnterScope/LeavaScope pair
// so the block body's instructions are in the block scope.
// For blocks without params, the statements are compiled inline in the parent scope.
func (c *Compiler) compileBlockAsClosure(block *ast.BlockExpression) error {
	c.EnterScope()

	for _, param := range block.Params {
		c.symbolTable.Define(param.Value)
	}

	if err := c.compileBlockAsValue(block); err != nil {
		return err
	}

	c.replaceLastPopWithReturn()

	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	instructions := c.LeaveScope()

	fnObj := &object.Function{
		Name:         "__block__",
		Instructions: instructions,
		NumLocals:    numLocals,
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
	for _, patchPos := range c.scopes[c.scopeIndex].nextPatchPos {
		c.changeOperand(patchPos, endPos)
	}
	c.scopes[c.scopeIndex].nextPatchPos = []int{}
	return nil
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
	for i, rescue := range node.Rescue {
		rescueOffsets[i] = len(c.currentInstructions())
		if i == 0 {
			rescueStart = rescueOffsets[i]
		}

		for _, exc := range rescue.Exceptions {
			if err := c.Compile(exc); err != nil {
				return err
			}
		}
		c.Emit(OpRescue)
		if rescue.Variable != nil {
			if _, ok := c.symbolTable.Resolve(rescue.Variable.Value); !ok {
				c.symbolTable.Define(rescue.Variable.Value)
			}
			sym, _ := c.symbolTable.Resolve(rescue.Variable.Value)
			if sym.Scope == ScopeLocal {
				c.emit(OpSetLocal, sym.Index)
			}
		} else {
			c.Emit(OpPop)
		}

		if err := c.compileBlockAsValue(rescue.Body); err != nil {
			return err
		}

		if i < len(node.Rescue)-1 {
			c.emit(OpJump, 0)
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

	if hasElse {
		if err := c.compileBlockAsValue(node.Else); err != nil {
			return err
		}
	}

	endStart := len(c.currentInstructions())

	if hasEnsure {
		c.changeOperand(jumpToEnd, ensureStart)
	} else {
		c.changeOperand(jumpToEnd, endStart)
	}
	c.changeOperandAt(beginPos, 0, rescueStart)
	c.changeOperandAt(beginPos, 1, ensureStart)
	c.changeOperandAt(beginPos, 2, endStart)

	return nil
}

func (c *Compiler) compileCatchExpression(node *ast.CatchExpression) error {
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

func (c *Compiler) EnterScope() {
	scope := CompilationScope{
		instructions:       Instructions{},
		breakTarget:        -1,
		nextPatchPos:       []int{},
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

type Bytecode struct {
	Instructions Instructions
	Constants    []*object.EmeraldValue
	NumLocals    int
}

func (c *Compiler) compileDefaultValue(expr ast.Expression) *object.EmeraldValue {
	switch node := expr.(type) {
	case *ast.IntegerLiteral:
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  node.Value,
			Class: core.R.Classes["Integer"],
		}
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
	case *ast.Boolean:
		if node.Value {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	case *ast.NilExpression:
		return core.R.NilVal
	default:
		return core.R.NilVal
	}
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
					Data:  "#{" + part.text + "}",
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
				c.emit(OpSend, methodIdx, 0, 0)
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
				Data:  part.text,
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
		if i+1 < len(s) && s[i] == '#' && s[i+1] == '{' {
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
	if len(name.Value) > 0 && name.Value[0] >= 'A' && name.Value[0] <= 'Z' {
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
		c.emit(OpGetLocal, sym.Index)
	case ScopeOuter:
		c.emit(OpGetOuter, sym.ScopeIndex)
	case ScopeFree:
		c.emit(OpGetFree, sym.Index)
	case ScopeGlobal:
		c.emit(OpGetGlobal, sym.Index)
	default:
		c.Emit(OpNil)
	}
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
	c.EnterScope()
	c.symbolTable.Define(node.Variable.Value)

	if err := c.compileBlockAsValue(node.Body); err != nil {
		return err
	}

	c.replaceLastPopWithReturn()

	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	instructions := c.LeaveScope()

	fnObj := &object.Function{
		Name:         "__for_block__",
		Instructions: instructions,
		NumLocals:    numLocals,
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

	if err := c.Compile(node.Collection); err != nil {
		return err
	}

	eachIdx := c.addConstant(&object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "each",
		Class: core.R.Classes["String"],
	})
	c.emit(OpSend, eachIdx, 1, 0)

	return nil
}

func (c *Compiler) compileProcLiteral(node *ast.ProcLiteral) error {
	c.EnterScope()
	for _, param := range node.Params {
		c.symbolTable.Define(param.Value)
	}

	if node.Body != nil {
		for _, s := range node.Body.Statements {
			if err := c.Compile(s); err != nil {
				return err
			}
		}
	}

	c.replaceLastPopWithReturn()

	free := c.symbolTable.FreeSymbols
	numLocals := c.symbolTable.MaxSymbols
	instructions := c.LeaveScope()

	fnObj := &object.Function{
		Name:         "__lambda__",
		Instructions: instructions,
		NumLocals:    numLocals,
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
