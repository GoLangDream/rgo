package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedTimesStringStoreEnabled controls the narrow counted-loop lowering for
// `result = index.to_s`.  The block has no observable operation other than a
// terminal captured write, so all intermediate String objects are dead.
var typedTimesStringStoreEnabled = os.Getenv("RGO_DISABLE_TYPED_TIMES_STRING_STORE") == ""

const typedTimesStringStoreMinIterations int64 = 1024

type typedTimesStringStoreShape struct {
	freeIndex        uint8
	integerTypeGuard bool
}

// typedTimesStringStoreShapeFor recognizes exactly the block emitted for
// `n.times { |index| result = index.to_s }` and its exact `is_a?(Integer)` /
// ternary sibling. Keeping the matcher at the Register IR level makes the
// optimization independent of source spelling; any other prefix, send,
// branch, or result producer is rejected.
func typedTimesStringStoreShapeFor(fn *object.Function, plan *registerIRPlan, closure *object.Closure) (typedTimesStringStoreShape, bool) {
	if fn == nil || plan == nil || closure == nil || !plan.blockReturn ||
		plan.hasImplicitSends || plan.hasExplicitReturn ||
		len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" ||
		fn.KeywordRestOnly || !simpleBlockParameterPatterns(fn) {
		return typedTimesStringStoreShape{}, false
	}
	inputLocal := fn.ParamLocalIndices[0]
	if len(plan.instructions) == 4 && !plan.hasBranches && plan.sendCount == 1 {
		load, send, store, ret := plan.instructions[0], plan.instructions[1], plan.instructions[2], plan.instructions[3]
		if (load.op == registerIRLoadParam && load.param == 0 ||
			load.op == registerIRLoadLocal && int(load.param) == inputLocal) &&
			send.op == registerIRSend && send.opcode == compiler.OpSend && send.name == "to_s" &&
			send.argc == 0 && send.blockPresent == false && send.splatIndex == 255 && send.left == load.dst &&
			store.op == registerIRStoreFree && store.left == send.dst &&
			int(store.param) < len(closure.Free) && ret.op == registerIRReturn && ret.left == send.dst {
			return typedTimesStringStoreShape{freeIndex: store.param}, true
		}
	}
	if len(plan.instructions) != 10 || !plan.hasBranches || plan.sendCount != 2 || !plan.hasConstantLoads {
		return typedTimesStringStoreShape{}, false
	}
	loadType, loadInteger, isA, jumpFalse := plan.instructions[0], plan.instructions[1], plan.instructions[2], plan.instructions[3]
	loadValue, toS, jumpEnd, falseValue := plan.instructions[4], plan.instructions[5], plan.instructions[6], plan.instructions[7]
	store, ret := plan.instructions[8], plan.instructions[9]
	if (loadType.op != registerIRLoadParam && loadType.op != registerIRLoadLocal) ||
		loadType.op == registerIRLoadParam && loadType.param != 0 ||
		loadType.op == registerIRLoadLocal && int(loadType.param) != inputLocal ||
		loadValue.op != loadType.op || loadValue.param != loadType.param || loadValue.dst != loadType.dst ||
		loadInteger.op != registerIRLoadConstant || loadInteger.name != "Integer" ||
		isA.op != registerIRSend || isA.opcode != compiler.OpSend || isA.name != "is_a?" || isA.argc != 1 ||
		isA.blockPresent || isA.splatIndex != 255 || isA.left != loadType.dst || isA.args[0] != loadInteger.dst ||
		jumpFalse.op != registerIRJumpNotTruthy || jumpFalse.left != isA.dst || jumpFalse.target != 7 ||
		toS.op != registerIRSend || toS.opcode != compiler.OpSend || toS.name != "to_s" || toS.argc != 0 ||
		toS.blockPresent || toS.splatIndex != 255 || toS.left != loadValue.dst || jumpEnd.op != registerIRJump ||
		jumpEnd.target != 8 || falseValue.op != registerIRLoadConstantValue || falseValue.value == nil ||
		falseValue.dst != toS.dst || falseValue.value.Type != object.ValueString || falseValue.value.Data != "" ||
		store.op != registerIRStoreFree || store.left != toS.dst || int(store.param) >= len(closure.Free) ||
		ret.op != registerIRReturn || ret.left != toS.dst {
		return typedTimesStringStoreShape{}, false
	}
	return typedTimesStringStoreShape{freeIndex: store.param, integerTypeGuard: true}, true
}

// tryExecuteIntegerTimesTypedStringStore collapses a dead-result counted loop
// to its final assignment. Integer#times ignores each block return value, and
// the exact block shape contains no user code besides the built-in to_s edge;
// therefore only the final String can be observed after the loop. The guard
// rejects every condition that could observe intermediate object identity or
// alter the built-in dispatch, leaving the original callback path untouched.
func (vm *VM) tryExecuteIntegerTimesTypedStringStore(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedTimesStringStoreEnabled || receiver == nil || block == nil ||
		block.Type != object.ValueClosure || count < typedTimesStringStoreMinIterations ||
		!registerIRBlockEnabled || !registerIRBatchBlockEnabled || !registerIRNoFrameEnabled ||
		!registerIRDirectNoFrameEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) ||
		closure.AutoSplat && blockWantsDestructuring(closure.Fn) {
		return nil, false
	}
	plan, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || plan.kind != leafMethodRegisterIR || plan.registerIR == nil ||
		!registerIRPlanSafeForDirectNoFrameWithOptions(plan.registerIR, true, true, true) ||
		!registerIRDirectConstantsSafe(vm, closure, plan.registerIR) {
		return nil, false
	}
	shape, ok := typedTimesStringStoreShapeFor(closure.Fn, plan.registerIR, closure)
	if !ok || !core.IntegerToSUsesBuiltinImplementation() || int(shape.freeIndex) >= len(closure.Free) {
		return nil, false
	}
	constantGeneration := uint64(0)
	if shape.integerTypeGuard {
		if !core.IntegerIsAUsesBuiltinImplementation() {
			return nil, false
		}
		constant, constantOK := vm.topLevelConstantValue("Integer")
		integerClass, classOK := core.R.Classes["Integer"]
		if !constantOK || !classOK || constant == nil || constant.Type != object.ValueClass {
			return nil, false
		}
		constantClass, valueOK := constant.Data.(*object.Class)
		if !valueOK || constantClass != integerClass {
			return nil, false
		}
		constantGeneration = object.CurrentConstantGeneration()
	}
	generation := object.CurrentMethodGeneration()
	if generation != object.CurrentMethodGeneration() {
		return nil, false
	}

	// No callback operation can observe the block between iterations, so the
	// only Ruby value that needs to be materialized is the final index's fresh
	// String. Keep the ordinary closure cell/update path so the final result has
	// the same identity and mutability as a regular Integer#to_s call.
	result := core.NewStringValue(core.IntegerToSRawBuiltin(count - 1))
	if object.CurrentMethodGeneration() != generation || !core.IntegerToSUsesBuiltinImplementation() ||
		shape.integerTypeGuard && (object.CurrentConstantGeneration() != constantGeneration || !core.IntegerIsAUsesBuiltinImplementation()) {
		return nil, false
	}
	setClosureValue(&closure.Free[shape.freeIndex], result)
	core.LastBlockResult = nil
	return receiver, true
}
