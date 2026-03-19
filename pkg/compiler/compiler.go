package compiler

import (
	"fmt"
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
	symbol := Symbol{Name: name, Index: len(s.store), Scope: ScopeLocal}
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
	symbol := Symbol{Name: name, Index: len(s.store), Scope: ScopeGlobal}
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
		obj, ok = s.Outer.Resolve(name)
		if !ok {
			return obj, ok
		}

		if obj.Scope == ScopeLocal || obj.Scope == ScopeBuiltin {
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
		c.Emit(OpNil)
	case *ast.FloatLiteral:
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  node.Value,
			Class: core.R.Classes["Float"],
		})
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
		c.EmitConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Value,
			Class: core.R.Classes["String"],
		})
	case *ast.Boolean:
		if node.Value {
			c.Emit(OpTrue)
		} else {
			c.Emit(OpFalse)
		}
	case *ast.NilExpression:
		c.Emit(OpNil)
	case *ast.Identifier:
		if node.Value == "self" {
			c.Emit(OpSelf)
			return nil
		}
		sym, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			c.Emit(OpNil)
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
		c.Emit(OpNil)
	case *ast.InstanceVariable:
		c.emit(OpGetInstanceVar, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name,
			Class: core.R.Classes["String"],
		}))
	case *ast.GlobalVariable:
		c.emit(OpGetGlobal, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name,
			Class: core.R.Classes["String"],
		}))
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
		if err := c.Compile(node.Right); err != nil {
			return err
		}

		switch node.Operator {
		case "!":
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

		jumpNotTruthyPos := c.emit(OpJumpNotTruthy, 9999)

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
					if err := c.Compile(clause.Body); err != nil {
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
					if err := c.Compile(clause.Body); err != nil {
						return err
					}
					jumpToEndPositions = append(jumpToEndPositions, c.emit(OpJump, 9999))
					afterCond := len(c.currentInstructions())
					c.changeOperand(condJumpPos, afterCond)
				}
			}
		}
		if node.Else != nil {
			if err := c.Compile(node.Else); err != nil {
				return err
			}
		} else {
			c.Emit(OpNil)
		}
		afterAll := len(c.currentInstructions())
		for _, pos := range jumpToEndPositions {
			c.changeOperand(pos, afterAll)
		}
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
		c.Emit(OpIndex)
	case *ast.AssignExpression:
		if err := c.Compile(node.Value); err != nil {
			return err
		}

		// Check if the name is a global variable (starts with $)
		if len(node.Name.Value) > 0 && node.Name.Value[0] == '$' {
			// Define in symbol table and use index
			if _, ok := c.symbolTable.Resolve(node.Name.Value); !ok {
				c.symbolTable.Define(node.Name.Value)
			}
			sym, _ := c.symbolTable.Resolve(node.Name.Value)
			c.symbolTable.DefineGlobal(node.Name.Value)
			c.emit(OpSetGlobal, sym.Index)
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

		sym, ok := c.symbolTable.Resolve(node.Name.Value)
		if !ok {
			c.symbolTable.Define(node.Name.Value)
			sym, _ = c.symbolTable.Resolve(node.Name.Value)
		}

		switch sym.Scope {
		case ScopeGlobal:
			c.emit(OpSetGlobal, sym.Index)
		case ScopeLocal:
			c.emit(OpSetLocal, sym.Index)
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
			if err := c.Compile(node.Block); err != nil {
				return err
			}
			blockArg = 1
		}
		c.emit(OpSend, methodNameIdx, blockArg, argCount)
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

		if err := c.compileBlockAsValue(node.Body); err != nil {
			return err
		}

		c.Emit(OpReturnValue)

		free := c.symbolTable.FreeSymbols

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

		numLocals := len(node.Params) + len(node.KeywordParams)
		if node.RestParam != nil {
			numLocals++
		}

		fnObj := &object.Function{
			Name:          node.Name.Value,
			Instructions:  instructions,
			NumLocals:     numLocals,
			KeywordParams: kwParams,
		}
		if node.RestParam != nil {
			fnObj.HasRestParam = true
			fnObj.RestParamIndex = len(node.Params)
		}

		fn := &object.EmeraldValue{
			Type:  object.ValueFunction,
			Data:  fnObj,
			Class: core.R.Classes["Class"],
		}
		fnIdx := c.addConstant(fn)

		c.emit(OpClosure, fnIdx, len(free))
		for _, s := range free {
			if s.Scope == ScopeLocal {
				c.emit(OpGetLocal, s.Index)
			} else {
				c.emit(OpGetFree, s.Index)
			}
		}

		c.emit(OpDefineMethod, c.addConstant(&object.EmeraldValue{
			Type:  object.ValueString,
			Data:  node.Name.Value,
			Class: core.R.Classes["String"],
		}))
	case *ast.ClassExpression:
		if node.SuperClass != nil {
			if err := c.Compile(node.SuperClass); err != nil {
				return err
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
			instructions := c.LeaveScope()

			fnObj := &object.Function{
				Name:         "__block__",
				Instructions: instructions,
				NumLocals:    len(node.Params),
			}

			fn := &object.EmeraldValue{
				Type:  object.ValueFunction,
				Data:  fnObj,
				Class: core.R.Classes["Class"],
			}
			fnIdx := c.addConstant(fn)

			c.emit(OpClosure, fnIdx, len(free))
			for _, s := range free {
				if s.Scope == ScopeLocal {
					c.emit(OpGetLocal, s.Index)
				} else {
					c.emit(OpGetFree, s.Index)
				}
			}
		} else {
			// No params - compile inline (for if/while bodies)
			for _, s := range node.Statements {
				if err := c.Compile(s); err != nil {
					return err
				}
			}
		}
	case *ast.ProcLiteral:
		if node.Body != nil {
			for _, s := range node.Body.Statements {
				if err := c.Compile(s); err != nil {
					return err
				}
			}
		}
		c.Emit(OpNil)
	case *ast.WhileExpression:
		loopStart := len(c.currentInstructions())

		if err := c.Compile(node.Condition); err != nil {
			return err
		}

		jumpNotTruthyPos := c.emit(OpJumpNotTruthy, 9999)

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.emit(OpJump, loopStart)

		afterBody := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBody)

		// while returns nil in Ruby
		c.Emit(OpNil)
	case *ast.UntilExpression:
		// until is like while with negated condition
		loopStart := len(c.currentInstructions())

		if err := c.Compile(node.Condition); err != nil {
			return err
		}

		// Jump out if condition is TRUE (opposite of while)
		jumpTruthyPos := c.emit(OpJumpTruthy, 9999)

		if err := c.Compile(node.Body); err != nil {
			return err
		}

		c.emit(OpJump, loopStart)

		afterBody := len(c.currentInstructions())
		c.changeOperand(jumpTruthyPos, afterBody)

		// until returns nil in Ruby
		c.Emit(OpNil)
	case *ast.BreakExpression:
		c.Emit(OpBreak)
	case *ast.NextExpression:
		c.Emit(OpJump)
	case *ast.YieldExpression:
		if len(node.Args) > 0 {
			for _, arg := range node.Args {
				if err := c.Compile(arg); err != nil {
					return err
				}
			}
			c.emit(OpYieldWithValue, len(node.Args))
		} else {
			c.Emit(OpYield)
		}
	case *ast.SelfExpression:
		c.Emit(OpSelf)
	case *ast.SplatExpression:
		if err := c.Compile(node.Value); err != nil {
			return err
		}
		c.Emit(OpSplat)
	default:
		return fmt.Errorf("unknown node type: %T", node)
	}

	return nil
}

func (c *Compiler) Bytecode() *Bytecode {
	return &Bytecode{
		Instructions: c.currentInstructions(),
		Constants:    c.constants,
	}
}

func (c *Compiler) currentInstructions() Instructions {
	return c.scopes[c.scopeIndex].instructions
}

// compileBlockAsValue compiles a BlockExpression but removes the last OpPop
// so the block's last value stays on the stack (used for if/elsif/else branches)
func (c *Compiler) compileBlockAsValue(block *ast.BlockExpression) error {
	if block == nil || len(block.Statements) == 0 {
		c.Emit(OpNil)
		return nil
	}
	for _, s := range block.Statements {
		if err := c.Compile(s); err != nil {
			return err
		}
	}
	// Remove the last OpPop so the value remains on the stack
	last := c.scopes[c.scopeIndex].lastInstruction
	if last.Opcode == OpPop {
		c.scopes[c.scopeIndex].instructions = c.scopes[c.scopeIndex].instructions[:last.Position]
		c.scopes[c.scopeIndex].lastInstruction = c.scopes[c.scopeIndex].previousInstruction
	}
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

func (c *Compiler) EnterScope() {
	scope := CompilationScope{
		instructions: Instructions{},
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
