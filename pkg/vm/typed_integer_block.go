package vm

import (
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

type typedIntegerArrayReduceShape struct {
	ivar  string
	valid bool
}

// tryExecuteIntegerArrayReduceBlock handles the general two-argument shape
// `|sum, index| sum + @table[index]`. It is a typed block ABI, not a Gem-name
// special case: the exact bytecode sequence, builtin Array#/Integer#+
// generations, and concrete Array contents are checked on every admission.
// A miss is side-effect free and returns to the ordinary block binder.
func (vm *VM) tryExecuteIntegerArrayReduceBlock(block *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedIntegerMethodEnabled || len(args) != 2 || block == nil ||
		core.ObjectSpaceAllocationTracing() || !core.ArrayIndexUsesBuiltinImplementation() ||
		!vm.fusedIntegerOperationAvailable(compiler.OpAdd) {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if block.Type != object.ValueClosure || !ok || closure == nil || closure.Fn == nil ||
		closure.BreakOwnerID > 0 || closure.ReturnOwnerID > 0 || closureUsesRefinements(closure) {
		return nil, false
	}
	fn := closure.Fn
	if len(fn.Params) != 2 || len(fn.ParamLocalIndices) != 2 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || !simpleBlockParameterPatterns(fn) {
		return nil, false
	}
	shape, found := vm.typedIntegerArrayReduceShapes[fn]
	if !found {
		name, valid := typedIntegerArrayReduceShapeForFunction(fn)
		shape = typedIntegerArrayReduceShape{ivar: name, valid: valid}
		vm.typedIntegerArrayReduceShapes[fn] = shape
	}
	if !shape.valid {
		return nil, false
	}
	if !smallIntegerValue(args[0]) || !smallIntegerValue(args[1]) {
		return nil, false
	}
	self := blockBindingSelf(block)
	if self == nil || self.Type != object.ValueObject || self.Class == nil ||
		core.AttachedSingletonClass(self) != nil {
		return nil, false
	}
	table := core.DynamicInstanceVar(self, shape.ivar)
	if table == nil || table.Type != object.ValueArray || table.Class != core.R.Classes["Array"] {
		return nil, false
	}
	index := args[1].Data.(int64)
	items, ok := typedIntegerArrayItems(table)
	if !ok || index < 0 || index >= int64(len(items)) {
		return nil, false
	}
	item := items[index]
	if !smallIntegerValue(item) {
		return nil, false
	}
	result, ok := checkedIntegerAdd(args[0].Data.(int64), item.Data.(int64))
	if !ok {
		return nil, false
	}
	return core.NewIntegerValue(result), true
}

func typedIntegerArrayReduceShapeForFunction(fn *object.Function) (string, bool) {
	if fn == nil {
		return "", false
	}
	if registerIROpcodeSequence(fn) != "OpGetLocal>OpGetInstanceVar>OpGetLocal>OpIndex>OpAdd>OpBlockReturn" {
		return "", false
	}
	for position := 0; position+2 < len(fn.Instructions); position++ {
		if compiler.Opcode(fn.Instructions[position]) != compiler.OpGetInstanceVar {
			continue
		}
		index := int(fn.Instructions[position+1])<<8 | int(fn.Instructions[position+2])
		if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
			return "", false
		}
		name, ok := fn.Constants[index].Data.(string)
		return name, ok && strings.HasPrefix(name, "@") && name != "@"
	}
	return "", false
}

func typedIntegerArrayItems(value *object.EmeraldValue) ([]*object.EmeraldValue, bool) {
	if value == nil {
		return nil, false
	}
	switch data := value.Data.(type) {
	case []*object.EmeraldValue:
		return data, true
	case *object.RArray:
		if data == nil {
			return nil, false
		}
		return data.Elements, true
	default:
		return nil, false
	}
}
