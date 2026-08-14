package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var typedArrayLiteralDeadElementEnabled = os.Getenv("RGO_DISABLE_TYPED_ARRAY_LITERAL_DEAD_ELEMENT") == ""

const typedArrayLiteralDeadElementMinElements = 1024

// tryExecuteArrayLiteralIndexDeadElement handles the exact pure callback
// `[x, x + 1][0]`. The existing literal-index fold already removes the
// temporary Array and [] lookup, but the unselected x+1 still gets executed
// and boxed. Once every element is an exact Integer and the builtin addition
// is overflow-safe, that expression has no observable result and the map can
// reuse the original x pointers in the newly materialized result Array.
func (vm *VM) tryExecuteArrayLiteralIndexDeadElement(receiver, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedArrayLiteralDeadElementEnabled || receiver == nil || receiver.Type != object.ValueArray ||
		receiver.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(receiver) != nil || block == nil ||
		block.Type != object.ValueClosure || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedArrayLiteralDeadElementMinElements {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 || closureUsesRefinements(closure) ||
		closure.AutoSplat && blockWantsDestructuring(closure.Fn) || len(closure.Fn.Params) != 1 ||
		len(closure.Fn.ParamLocalIndices) != 1 || closure.Fn.HasRestParam || closure.Fn.HasBlockParam ||
		len(closure.Fn.KeywordParams) != 0 || closure.Fn.KeywordRestParam != "" || closure.Fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(closure.Fn) {
		return nil, false
	}
	for _, defaultValue := range closure.Fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	leaf, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil ||
		!registerIRPlanSafeForFramelessBlock(leaf.registerIR) || !arrayLiteralDeadElementShape(closure.Fn, leaf.registerIR) ||
		!core.ArrayIndexUsesBuiltinImplementation() || !vm.fusedIntegerOperationAvailable(compiler.OpAdd) {
		return nil, false
	}
	integerClass := core.R.Classes["Integer"]
	generation := object.CurrentMethodGeneration()
	for _, elem := range elems {
		if elem == nil || elem.Type != object.ValueInteger || elem.BigIntValue() != nil ||
			elem.Class != nil && elem.Class != integerClass {
			return nil, false
		}
		value, valueOK := elem.Data.(int64)
		if !valueOK {
			return nil, false
		}
		if _, safe := checkedIntegerAdd(value, 1); !safe {
			return nil, false
		}
	}
	if object.CurrentMethodGeneration() != generation || !core.ArrayIndexUsesBuiltinImplementation() ||
		!vm.fusedIntegerOperationAvailable(compiler.OpAdd) {
		return nil, false
	}
	result := make([]*object.EmeraldValue, len(elems))
	copy(result, elems)
	core.LastBlockResult = nil
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: core.R.Classes["Array"]}, true
}

func arrayLiteralDeadElementShape(fn *object.Function, plan *registerIRPlan) bool {
	if fn == nil || plan == nil || !plan.blockReturn || plan.hasBranches || plan.hasImplicitSends ||
		plan.hasExplicitReturn || plan.sendCount != 0 || len(plan.instructions) != 8 || len(fn.Params) != 1 ||
		len(fn.ParamLocalIndices) != 1 {
		return false
	}
	loadX, loadXAgain, literalOne, add, array, literalZero, index, ret :=
		plan.instructions[0], plan.instructions[1], plan.instructions[2], plan.instructions[3],
		plan.instructions[4], plan.instructions[5], plan.instructions[6], plan.instructions[7]
	inputLocal := fn.ParamLocalIndices[0]
	inputLoad := func(instruction registerIRInstruction) bool {
		return instruction.op == registerIRLoadParam && instruction.param == 0 ||
			instruction.op == registerIRLoadLocal && int(instruction.param) == inputLocal
	}
	return inputLoad(loadX) && inputLoad(loadXAgain) &&
		literalOne.op == registerIRLoadLiteral && literalOne.value != nil && literalOne.value.Type == object.ValueInteger &&
		literalOne.value.Data == int64(1) && add.op == registerIRBinary && add.opcode == compiler.OpAdd &&
		add.left == loadXAgain.dst && add.right == literalOne.dst && add.dst == loadXAgain.dst &&
		array.op == registerIRArray && array.dst == loadX.dst && array.argc == 2 &&
		loadXAgain.dst == array.dst+1 &&
		literalZero.op == registerIRLoadLiteral && literalZero.value != nil && literalZero.value.Type == object.ValueInteger &&
		literalZero.value.Data == int64(0) && index.op == registerIRIndex && index.left == array.dst &&
		index.right == literalZero.dst && index.dst == array.dst && ret.op == registerIRReturn && ret.left == index.dst
}
