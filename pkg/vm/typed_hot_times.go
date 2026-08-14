package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedHotTimesCallEnabled enables the side-effect-safe Integer#times callback
// graph. The ordinary direct fallback keeps a fresh boxed index because a Ruby
// callee may retain that argument; the narrower typed callee proof keeps the
// exact Integer index raw until a Ruby object is required at the boundary.
var typedHotTimesCallEnabled = os.Getenv("RGO_DISABLE_TYPED_HOT_TIMES_CALL") == ""

const typedHotTimesCallMinIterations int64 = 1024

func registerIRPlanHasRubyTerminalSend(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRSend && !registerIRTrustedNativeNoEscapeName(instruction.name) {
			return true
		}
	}
	return false
}

func typedSSAEffectfulIntegerPlanSafe(plan *typedSSAPlan) bool {
	if plan == nil {
		return false
	}
	if plan.effectfulIntegerSafeChecked {
		return plan.effectfulIntegerSafe
	}
	safe := false
	defer func() {
		plan.effectfulIntegerSafeChecked = true
		plan.effectfulIntegerSafe = safe
	}()
	if plan.blockReturn || !plan.hasInstanceStore || len(plan.ops) < 2 ||
		plan.ops[len(plan.ops)-2].kind != typedSSAOpStoreInstanceVar ||
		plan.ops[len(plan.ops)-1].kind != typedSSAOpReturn {
		return false
	}
	storeCount := 0
	for _, instruction := range plan.ops {
		switch instruction.kind {
		case typedSSAOpLoadParam, typedSSAOpLoadLiteral, typedSSAOpLoadLocal, typedSSAOpLoadInstanceVar,
			typedSSAOpMove, typedSSAOpCompare, typedSSAOpBinary,
			typedSSAOpJump, typedSSAOpJumpTruthy, typedSSAOpJumpNotTruthy,
			typedSSAOpJumpNotNil, typedSSAOpReturn:
		case typedSSAOpCall:
			if instruction.name != "to_s" || instruction.argc != 0 || instruction.implicit {
				return false
			}
		case typedSSAOpStoreInstanceVar:
			storeCount++
		default:
			return false
		}
	}
	safe = storeCount == 1
	return safe
}

type typedHotTimesCallee struct {
	receiver          *object.EmeraldValue
	fn                *object.Function
	plan              *typedSSAPlan
	free              []*object.EmeraldValue
	class             *object.Class
	integerClass      *object.Class
	receiverFromSelf  bool
	receiverFromFree  bool
	receiverFreeIndex uint8
	receiverCell      *closureCell
	rawInteger        bool
	outerStoreName    string
}

func resolveTypedHotTimesReceiver(block *object.EmeraldValue, expected *object.EmeraldValue) *object.EmeraldValue {
	if block == nil || expected == nil {
		return nil
	}
	if self := blockBindingSelf(block); self == expected {
		return self
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil {
		return nil
	}
	for _, free := range closure.Free {
		if value := derefClosureValue(free); value == expected {
			return value
		}
	}
	return nil
}

func typedHotTimesReceiverMatches(block *object.EmeraldValue, callee *typedHotTimesCallee) bool {
	if block == nil || callee == nil || callee.receiver == nil {
		return false
	}
	if callee.receiverFromSelf {
		return blockBindingSelf(block) == callee.receiver
	}
	if callee.receiverFromFree {
		closure, ok := block.Data.(*object.Closure)
		if !ok || closure == nil || int(callee.receiverFreeIndex) >= len(closure.Free) {
			return false
		}
		captured := closure.Free[callee.receiverFreeIndex]
		if callee.receiverCell != nil {
			if captured != nil {
				if cell, ok := captured.Data.(*closureCell); ok && cell == callee.receiverCell {
					if cell.slot != nil {
						return *cell.slot == callee.receiver
					}
					return cell.value == callee.receiver
				}
			}
			return false
		}
		return captured == callee.receiver
	}
	return resolveTypedHotTimesReceiver(block, callee.receiver) == callee.receiver
}

func typedHotTimesCacheLeaf(cache *registerIRSendCache, receiver *object.EmeraldValue) (*object.Method, *object.Function, *leafMethodPlan, bool) {
	if cache == nil || receiver == nil || cache.generation != object.CurrentMethodGeneration() {
		return nil, nil, nil, false
	}
	identity := registerIRCacheableClassReceiver(receiver)
	if identity && cache.receiver == receiver || !identity && cache.receiver == nil && cache.class == receiver.Class {
		return cache.method, cache.inlineFn, cache.inlineLeaf, true
	}
	if registerIRPolymorphicSendCacheEnabled && (identity && cache.secondReceiver == receiver || !identity && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		return cache.secondMethod, cache.secondFn, cache.secondLeaf, true
	}
	return nil, nil, nil, false
}

// prepareTypedHotTimesCallee recognizes the stable outer callback shape and
// returns a typed effectful callee after the ordinary direct path has warmed
// the send cache. Restricting the receiver/argument producers keeps the raw
// loop from skipping any observable prefix work in a more complex block.
func (vm *VM) prepareTypedHotTimesCallee(block *object.EmeraldValue, outer *registerIRPlan, generation uint64) (*typedHotTimesCallee, bool) {
	if vm == nil || block == nil || outer == nil || outer.sendCount != 1 {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || len(closure.Fn.Params) != 1 {
		return nil, false
	}
	var send *registerIRInstruction
	for index := range outer.instructions {
		instruction := &outer.instructions[index]
		if instruction.op != registerIRSend || registerIRTrustedNativeNoEscapeName(instruction.name) {
			continue
		}
		if send != nil || instruction.argc != 1 {
			return nil, false
		}
		send = instruction
	}
	if send == nil {
		return nil, false
	}
	outerStoreName := ""
	if len(outer.instructions) >= 2 {
		store := outer.instructions[len(outer.instructions)-2]
		ret := outer.instructions[len(outer.instructions)-1]
		if store.op == registerIRStoreInstanceVar && store.left == send.dst &&
			ret.op == registerIRReturn && ret.left == store.left {
			outerStoreName = store.name
		}
	}
	var receiver *object.EmeraldValue
	var receiverFromSelf, receiverFromFree bool
	var receiverFreeIndex uint8
	var receiverCell *closureCell
	argumentOK := false
	for _, instruction := range outer.instructions {
		if instruction.dst == send.left {
			switch instruction.op {
			case registerIRLoadSelf:
				receiver = blockBindingSelf(block)
				receiverFromSelf = true
				receiverFromFree = false
			case registerIRLoadFree:
				if int(instruction.param) < len(closure.Free) {
					captured := closure.Free[instruction.param]
					receiver = derefClosureValue(captured)
					if captured != nil {
						receiverCell, _ = captured.Data.(*closureCell)
					}
					receiverFromFree = true
					receiverFromSelf = false
					receiverFreeIndex = instruction.param
				}
			}
		}
		if instruction.dst == send.args[0] {
			switch instruction.op {
			case registerIRLoadParam:
				argumentOK = instruction.param == 0
			case registerIRLoadLocal:
				argumentOK = int(instruction.param) == closure.Fn.ParamLocalIndices[0]
			}
		}
	}
	if receiver == nil || !argumentOK || receiver.Class == nil {
		return nil, false
	}
	methodObj, fn, leaf, ok := typedHotTimesCacheLeaf(send.cache, receiver)
	if !ok {
		// The outer direct executor can have proved the send through the plain
		// VM method cache without publishing its leaf plan into this particular
		// Register IR cache slot.  Resolve that already-warmed edge once here;
		// the generation/receiver guards below still make the typed callee
		// speculative, while avoiding a per-element IR interpreter fallback.
		resolved, _, fallback := vm.lookupMethodForSend(receiver, send.name, nil, false, true)
		if fallback != nil || resolved == nil {
			return nil, false
		}
		methodObj = resolved
		fn, ok = methodObj.Fn.(*object.Function)
		if !ok || fn == nil {
			return nil, false
		}
		resolvedLeaf, foundLeaf := vm.cachedBlockLeafPlan(fn)
		if !foundLeaf {
			return nil, false
		}
		leaf = &resolvedLeaf
	}
	if !ok || methodObj == nil || fn == nil || leaf == nil || leaf.kind != leafMethodRegisterIR ||
		methodObj.Closure == nil || methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" {
		return nil, false
	}
	if _, native := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); native {
		return nil, false
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(fn)
	if !ok || calleePlan == nil {
		return nil, false
	}
	rawInteger := false
	if typedSSAEffectfulIntegerPlanSafe(calleePlan) {
		if kernel := calleePlan.effectfulIntegerKernel; kernel.kind == typedSSAEffectfulIntegerKernelCompareToSStore &&
			(!vm.fusedIntegerOperationAvailable(kernel.compare) || !core.IntegerToSUsesBuiltinImplementation()) {
			return nil, false
		}
	} else {
		// A pure primitive callee such as `value > 3 ? value.to_s : ""`
		// does not need the boxed argument identity.  Reuse the typed SSA raw
		// ABI here; if the outer callback has a terminal store, the caller
		// applies that store after this pure result is produced.
		if calleePlan.blockReturn || calleePlan.hasReference || calleePlan.hasFloat ||
			len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
			len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
			!simpleBlockParameterPatterns(fn) || !vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) {
			return nil, false
		}
		for _, operation := range calleePlan.ops {
			if operation.kind == typedSSAOpCall && operation.name == "to_s" && !core.IntegerToSUsesBuiltinImplementation() {
				return nil, false
			}
		}
		rawInteger = true
	}
	return &typedHotTimesCallee{
		receiver:         receiver,
		fn:               fn,
		plan:             calleePlan,
		free:             methodObj.Closure.Free,
		class:            receiver.Class,
		integerClass:     core.R.Classes["Integer"],
		receiverFromSelf: receiverFromSelf, receiverFromFree: receiverFromFree,
		receiverFreeIndex: receiverFreeIndex, receiverCell: receiverCell,
		rawInteger: rawInteger, outerStoreName: outerStoreName,
	}, true
}

// tryExecuteIntegerTimesTypedHotCall removes the per-iteration block Frame
// and outer send admission for a proven `n.times { |index| object.update(index) }`
// graph. The block proof requires any Ruby-defined send to be the terminal
// callback operation, so a guard miss can replay only the current suffix and
// cannot duplicate an earlier observable mutation.
func (vm *VM) tryExecuteIntegerTimesTypedHotCall(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedHotTimesCallEnabled || receiver == nil || block == nil ||
		count < typedHotTimesCallMinIterations || !registerIRBatchBlockEnabled ||
		!registerIRNoFrameEnabled || !registerIRDirectNoFrameEnabled || vm.instructionLimit != 0 ||
		DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.threadDepth > 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	if block.Type != object.ValueClosure {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || closure.AutoSplat && blockWantsDestructuring(closure.Fn) ||
		!registerIRClosureControlFlowSafe(closure.Fn) {
		return nil, false
	}
	fn := closure.Fn
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" ||
		fn.KeywordRestOnly || !simpleBlockParameterPatterns(fn) || fn.NumLocals > 64 {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	plan, found := vm.cachedBlockLeafPlan(fn)
	if !found || plan.kind != leafMethodRegisterIR || plan.registerIR == nil ||
		!registerIRPlanHasRubyTerminalSend(plan.registerIR) ||
		!registerIRPlanSafeForTypedHotArrayCall(plan.registerIR) ||
		!registerIRDirectConstantsSafe(vm, closure, plan.registerIR) {
		return nil, false
	}

	prevBlock := vm.currentBlock
	prevClassStack := vm.classStack
	prevStringBatch := vm.typedStringValueBatch
	prevStringScratch := vm.typedStringValueScratch
	prevStringScratchStored := vm.typedStringScratchStored
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack
		vm.typedStringValueBatch = prevStringBatch
		vm.typedStringValueScratch = prevStringScratch
		vm.typedStringScratchStored = prevStringScratchStored
	}
	fallback := func(start int64) (*object.EmeraldValue, bool) {
		cleanup()
		var fallbackArgs [1]*object.EmeraldValue
		for index := start; index < count; index++ {
			fallbackArgs[0] = core.NewIntegerValue(index)
			core.LastBlockResult = nil
			result := vm.callBlockWithSelfArgs(block, blockBindingSelf(block), fallbackArgs[:])
			if core.LastBlockResult != nil {
				breakResult := core.LastBlockResult
				core.LastBlockResult = nil
				return breakResult, true
			}
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		}
		core.LastBlockResult = nil
		return receiver, true
	}

	self := blockBindingSelf(block)
	if self == nil {
		cleanup()
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	trustedPlan := false
	var typedCallee *typedHotTimesCallee
	var args [1]*object.EmeraldValue
	for index := int64(0); index < count; index++ {
		if object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				core.LastBlockResult = nil
				return nil, false
			}
			return fallback(index)
		}
		core.LastBlockResult = nil
		var result *object.EmeraldValue
		var executed bool
		if typedCallee != nil {
			if !typedHotTimesReceiverMatches(block, typedCallee) ||
				typedCallee.receiver.Class != typedCallee.class || object.CurrentMethodGeneration() != generation {
				return fallback(index)
			}
			result, executed = vm.executeTypedSSAEffectfulIntegerPlan(
				typedCallee.plan, typedCallee.fn, typedCallee.receiver, index, typedCallee.free,
			)
		} else {
			// Keep a distinct boxed index until the typed callee has been proved.
			// A normal Ruby callee may retain its argument in an ivar or closure.
			args[0] = core.NewIntegerValue(index)
			result, executed = vm.executeRegisterIRDirectNoFrameWithFreeMode(
				plan.registerIR, fn, self, args[:], generation, closure.Free, trustedPlan,
				true, true, true, false, trustedPlan,
			)
		}
		if !executed {
			if index == 0 {
				cleanup()
				core.LastBlockResult = nil
				return nil, false
			}
			return fallback(index)
		}
		if result == nil {
			result = core.R.NilVal
		}
		if result.Type == object.ValueException {
			cleanup()
			return result, true
		}
		if core.LastBlockResult != nil {
			breakResult := core.LastBlockResult
			core.LastBlockResult = nil
			cleanup()
			return breakResult, true
		}
		if !trustedPlan {
			trustedPlan = registerIRTrustedArrayCallbackReady(plan.registerIR, generation)
			if trustedPlan {
				typedCallee, _ = vm.prepareTypedHotTimesCallee(block, plan.registerIR, generation)
				if typedStringBatchEnabled && typedCallee != nil && vm.typedStringValueBatch == nil &&
					typedCallee.plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelCompareToSStore {
					vm.typedStringValueScratch = core.NewStringValueScratch()
					vm.typedStringScratchStored = false
				}
			}
		}
	}
	cleanup()
	core.LastBlockResult = nil
	return receiver, true
}
