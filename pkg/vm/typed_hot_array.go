package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedHotArrayCallEnabled enables the side-effect-safe Array callback graph.
// It is deliberately separate from the pure typed-SSA batch: a Ruby callee
// may update its receiver, but only after the complete callback has crossed a
// direct-call proof. A miss therefore deopts the current element before the
// callee's terminal mutation and can replay only the unexecuted suffix.
var typedHotArrayCallEnabled = os.Getenv("RGO_DISABLE_TYPED_HOT_ARRAY_CALL") == ""
var typedStringBatchEnabled = os.Getenv("RGO_DISABLE_TYPED_STRING_BATCH") == ""
var typedSSAEffectfulStringMapLazyResultEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_EFFECTFUL_STRING_MAP_LAZY_RESULT") == ""

type typedHotArrayEffectfulIntegerCache struct {
	key               blockLeafPlanCacheKey
	generation        uint64
	callbackFn        *object.Function
	calleeReceiver    *object.EmeraldValue
	receiverFromSelf  bool
	receiverFreeIndex uint8
	receiverClass     *object.Class
	methodName        string
	method            *object.Method
	fn                *object.Function
	plan              *typedSSAPlan
	integerClass      *object.Class
	ready             bool
}

const typedHotArrayCallMinElements = 1024

// registerIRPlanSafeForTypedHotArrayCall admits the common Array callback
// shape `items.map { |item| receiver.update(item) }`. The outer callback may
// branch and allocate values, but every non-query Ruby send must either be the
// final value-producing operation or be followed only by a terminal instance
// variable/capture write and BlockReturn. That makes a nested direct callee an
// atomic callback from the outer loop's point of view: a failed guard cannot
// replay a prior mutation in the same element.
func registerIRPlanSafeForTypedHotArrayCall(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if plan.typedHotArrayCallSafeChecked && plan.typedHotArrayCallSafeGeneration == generation {
		return plan.typedHotArrayCallSafe
	}
	safe := false
	defer func() {
		plan.typedHotArrayCallSafeChecked = true
		plan.typedHotArrayCallSafe = safe
		plan.typedHotArrayCallSafeGeneration = generation
	}()
	if !plan.blockReturn || plan.hasImplicitSends || plan.hasExplicitReturn ||
		!registerIRPlanSafeForDirectNoFrameWithOptions(plan, true, true, true) {
		return false
	}
	for index, instruction := range plan.instructions {
		if instruction.op != registerIRSend {
			continue
		}
		if instruction.opcode != compiler.OpSend || instruction.blockPresent || instruction.splatIndex != 255 {
			return false
		}
		if registerIRTrustedNativeNoEscapeName(instruction.name) {
			if instruction.name == "length" && instruction.argc == 0 && !core.StringLengthUsesBuiltinImplementation() {
				return false
			}
			continue
		}
		// A user-defined direct callee may commit a receiver/capture mutation.
		// It is safe as the final value-producing operation, or immediately
		// before a terminal outer mutation that cannot deopt after the send.
		if index == len(plan.instructions)-2 && plan.instructions[index+1].op == registerIRReturn &&
			plan.instructions[index+1].left == instruction.dst {
			continue
		}
		if !registerIRTypedHotArrayTerminalSuffixSafe(plan, index) {
			return false
		}
	}
	safe = true
	return true
}

// registerIRTypedHotArrayTerminalSuffixSafe keeps the direct callback atomic
// when a Ruby send feeds an outer terminal mutation, for example
// `items.map { |item| @last = helper.convert(item) }`. After the send only
// operations that cannot fail or invoke user code are admitted. The general
// direct-plan proof still checks that the final store is immediately followed
// by the callback return and that its result register is preserved.
func registerIRTypedHotArrayTerminalSuffixSafe(plan *registerIRPlan, sendIndex int) bool {
	if plan == nil || sendIndex < 0 || sendIndex >= len(plan.instructions) {
		return false
	}
	for index := sendIndex + 1; index < len(plan.instructions); index++ {
		switch plan.instructions[index].op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadFrozenString, registerIRLoadInstanceVar, registerIRLoadSelf,
			registerIRLoadFree, registerIRMove, registerIRSwap, registerIRBang,
			registerIRStoreLocal, registerIRStoreInstanceVar, registerIRStoreFree:
		case registerIRReturn:
			return index == len(plan.instructions)-1
		default:
			return false
		}
	}
	return false
}

// tryExecuteArrayTypedSSAEffectfulIntegerUpdate handles the small-array form
// `items.each/map { |value| receiver.update(value) }` when update is the
// already-proven stateful integer kernel `@field = @field <op> value`.
//
// The existing Array typed batch intentionally starts at 1024 elements, while
// Gem code commonly repeats a short four- or eight-element Array inside an
// outer loop. This entry keeps the proof narrow enough for those calls: the
// callback is exactly one receiver load, one parameter load, one ordinary send
// and BlockReturn; the callee has no user-code edge after its typed plan is
// admitted. Overflow, redefinition, heterogeneous input and frozen writes
// side-exit before the current element's mutation and replay only the suffix.
func (vm *VM) tryExecuteArrayTypedSSAEffectfulIntegerUpdate(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || receiver.Class != core.R.ArrayClass ||
		block == nil || block.Type != object.ValueClosure || len(elems) < registerIRBatchBlockMinElements ||
		!typedHotArrayCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive ||
		vm.pendingReturnTargetID > 0 || vm.pendingBreakTargetID > 0 {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || closure.AutoSplat && blockWantsDestructuring(closure.Fn) {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	callCache := &vm.typedHotArrayEffectfulIntegerCache
	var method *object.Method
	var calleeFn *object.Function
	var calleePlan *typedSSAPlan
	var calleeReceiver *object.EmeraldValue
	fastCacheHit := false
	if callCache.ready && callCache.callbackFn == closure.Fn && callCache.generation == generation {
		if callCache.receiverFromSelf {
			calleeReceiver = blockBindingSelf(block)
		} else if int(callCache.receiverFreeIndex) < len(closure.Free) {
			calleeReceiver = derefClosureValue(closure.Free[callCache.receiverFreeIndex])
		}
		fastCacheHit = calleeReceiver == callCache.calleeReceiver
		if fastCacheHit {
			method = callCache.method
			calleeFn = callCache.fn
			calleePlan = callCache.plan
		}
	}
	if !fastCacheHit {
		fn := closure.Fn
		if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
			len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
			!simpleBlockParameterPatterns(fn) {
			return nil, false
		}
		for _, defaultValue := range fn.ParamDefaults {
			if defaultValue != nil {
				return nil, false
			}
		}
		leaf, found := vm.cachedTypedHotArrayLeafPlan(fn)
		if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil ||
			!registerIRPlanSafeForTypedHotArrayCall(leaf.registerIR) || len(leaf.registerIR.instructions) != 4 {
			return nil, false
		}
		outer := leaf.registerIR.instructions
		receiverReg := uint8(255)
		argumentReg := uint8(255)
		var send *registerIRInstruction
		for index := range outer {
			instruction := &outer[index]
			switch index {
			case 0:
				switch instruction.op {
				case registerIRLoadSelf, registerIRLoadFree:
					receiverReg = instruction.dst
				default:
					return nil, false
				}
			case 1:
				switch instruction.op {
				case registerIRLoadParam:
					if instruction.param != 0 {
						return nil, false
					}
				case registerIRLoadLocal:
					if int(instruction.param) != fn.ParamLocalIndices[0] {
						return nil, false
					}
				default:
					return nil, false
				}
				argumentReg = instruction.dst
			case 2:
				if instruction.op != registerIRSend || instruction.opcode != compiler.OpSend || instruction.blockPresent ||
					instruction.splatIndex != 255 || instruction.argc != 1 || instruction.left != receiverReg ||
					instruction.args[0] != argumentReg || instruction.name == "" {
					return nil, false
				}
				send = instruction
			case 3:
				if instruction.op != registerIRReturn || send == nil || instruction.left != send.dst {
					return nil, false
				}
			}
		}
		if send == nil {
			return nil, false
		}
		if outer[0].op == registerIRLoadSelf {
			calleeReceiver = blockBindingSelf(block)
		} else if int(outer[0].param) < len(closure.Free) {
			calleeReceiver = derefClosureValue(closure.Free[outer[0].param])
		}
		if calleeReceiver == nil || calleeReceiver.Type != object.ValueObject || calleeReceiver.Class == nil ||
			core.AttachedSingletonClass(calleeReceiver) != nil {
			return nil, false
		}
		callKey := makeBlockLeafPlanCacheKey(fn)
		var cacheHit bool
		method, _, _, cacheHit = typedHotTimesCacheLeaf(send.cache, calleeReceiver)
		var fallback *object.EmeraldValue
		if !cacheHit || method == nil {
			if send.cache != nil && vm.populateRegisterIRNoFrameCache(*send, calleeReceiver) {
				method, _, _, cacheHit = typedHotTimesCacheLeaf(send.cache, calleeReceiver)
			}
		}
		if !cacheHit || method == nil {
			method, _, fallback = vm.lookupMethodForSend(calleeReceiver, send.name, nil, false, true)
		}
		if fallback != nil || method == nil || method.DispatchOwner != nil || method.Ruby2Keywords ||
			methodUsesRefinements(method) || method.Visibility != "" && method.Visibility != "public" {
			return nil, false
		}
		var ok bool
		calleeFn, ok = method.Fn.(*object.Function)
		if !ok || calleeFn == nil || len(calleeFn.Params) != 1 || len(calleeFn.ParamLocalIndices) != 1 ||
			calleeFn.HasRestParam || calleeFn.HasBlockParam || len(calleeFn.KeywordParams) != 0 ||
			calleeFn.KeywordRestParam != "" || calleeFn.KeywordRestOnly || calleeFn.RejectKeywords || calleeFn.RejectBlock {
			return nil, false
		}
		calleePlan, ok = vm.cachedTypedSSAPlan(calleeFn)
		if !ok || calleePlan == nil || !typedSSAEffectfulIntegerPlanSafe(calleePlan) ||
			calleePlan.effectfulIntegerKernel.kind != typedSSAEffectfulIntegerKernelInstanceBinary ||
			!vm.fusedIntegerOperationAvailable(calleePlan.effectfulIntegerKernel.binary) {
			return nil, false
		}
		*callCache = typedHotArrayEffectfulIntegerCache{
			key: callKey, generation: generation, callbackFn: fn, calleeReceiver: calleeReceiver,
			receiverFromSelf: outer[0].op == registerIRLoadSelf, receiverFreeIndex: outer[0].param,
			receiverClass: calleeReceiver.Class, methodName: send.name, method: method, fn: calleeFn,
			plan: calleePlan, integerClass: core.R.IntegerClass, ready: true,
		}
	}
	if calleeReceiver == nil || calleeReceiver.Type != object.ValueObject || calleeReceiver.Class == nil ||
		core.AttachedSingletonClass(calleeReceiver) != nil {
		return nil, false
	}
	if method == nil || calleeFn == nil || calleePlan == nil {
		return nil, false
	}
	integerClass := callCache.integerClass
	if integerClass == nil {
		integerClass = core.R.IntegerClass
	}
	if integerClass == nil {
		return nil, false
	}
	var smallArguments [4]int64
	arguments := smallArguments[:]
	if len(elems) > len(smallArguments) {
		arguments = make([]int64, len(elems))
	} else {
		arguments = arguments[:len(elems)]
	}
	for index, elem := range elems {
		argument, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact {
			return nil, false
		}
		arguments[index] = argument
	}
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	prevBlock := vm.currentBlock
	prevClassStack := vm.classStack
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack
	}
	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, 0, len(arguments))
	}
	kernel := calleePlan.effectfulIntegerKernel
	var hotIntegerObject *object.Object
	hotIntegerSlot := -1
	if !collect && !calleeReceiver.Frozen && calleeReceiver.Type == object.ValueObject {
		if candidate, ok := calleeReceiver.Data.(*object.Object); ok {
			if slot, prepared := candidate.PrepareHotIntegerInstanceVar(kernel.instanceVar); prepared {
				hotIntegerObject = candidate
				hotIntegerSlot = slot
			}
		}
	}
	fallbackSuffix := func(start int) (*object.EmeraldValue, bool) {
		if hotIntegerObject != nil {
			hotIntegerObject.FlushHotIntegerInstanceVars()
		}
		cleanup()
		var arg [1]*object.EmeraldValue
		for index := start; index < len(elems); index++ {
			arg[0] = elems[index]
			core.LastBlockResult = nil
			result := vm.callBlockWithSelfArgs(block, blockBindingSelf(block), arg[:])
			if result == nil {
				result = core.R.NilVal
			}
			if result.Type == object.ValueException {
				return result, true
			}
			if core.LastBlockResult != nil {
				breakResult := core.LastBlockResult
				core.LastBlockResult = nil
				return breakResult, true
			}
			if collect {
				results = append(results, result)
			}
		}
		core.LastBlockResult = nil
		if collect {
			return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
		}
		return receiver, true
	}
	if !collect && !calleeReceiver.Frozen {
		var current int64
		var currentOK bool
		if hotIntegerObject != nil {
			current, currentOK = hotIntegerObject.HotIntegerInstanceVar(hotIntegerSlot)
			if !currentOK {
				current, currentOK = typedSSAExactIntegerValueForClass(hotIntegerObject.GetInstanceVar(kernel.instanceVar), integerClass)
			}
		} else {
			current, currentOK = typedSSAExactIntegerValueForClass(core.DynamicInstanceVar(calleeReceiver, kernel.instanceVar), integerClass)
		}
		if !currentOK {
			return fallbackSuffix(0)
		}
		commit := func() (*object.EmeraldValue, bool) {
			if hotIntegerObject != nil && hotIntegerObject.SetHotIntegerInstanceVar(hotIntegerSlot, current, integerClass) {
				return nil, false
			}
			result := core.NewIntegerValue(current)
			if errorValue := core.SetDynamicInstanceVar(calleeReceiver, kernel.instanceVar, result); errorValue != nil {
				cleanup()
				return errorValue, true
			}
			return nil, false
		}
		for index, argument := range arguments {
			if object.CurrentMethodGeneration() != generation {
				if result, handled := commit(); handled {
					return result, true
				}
				return fallbackSuffix(index)
			}
			left, right := current, argument
			if kernel.parameterLeft {
				left, right = right, left
			}
			var value int64
			var operationOK bool
			switch kernel.binary {
			case compiler.OpAdd:
				value, operationOK = checkedIntegerAdd(left, right)
			case compiler.OpSub:
				value, operationOK = checkedIntegerSub(left, right)
			case compiler.OpMul:
				value, operationOK = checkedIntegerMul(left, right)
			case compiler.OpMod:
				value, operationOK = checkedIntegerMod(left, right)
			case compiler.OpBitAnd:
				value, operationOK = left&right, true
			}
			if !operationOK {
				if result, handled := commit(); handled {
					return result, true
				}
				return fallbackSuffix(index)
			}
			current = value
		}
		if result, handled := commit(); handled {
			return result, true
		}
		core.LastBlockResult = nil
		return receiver, true
	}
	for index, argument := range arguments {
		if object.CurrentMethodGeneration() != generation {
			return fallbackSuffix(index)
		}
		current, currentOK := typedSSAExactIntegerValueForClass(core.DynamicInstanceVar(calleeReceiver, kernel.instanceVar), integerClass)
		if !currentOK {
			return fallbackSuffix(index)
		}
		left, right := current, argument
		if kernel.parameterLeft {
			left, right = right, left
		}
		var value int64
		var operationOK bool
		switch kernel.binary {
		case compiler.OpAdd:
			value, operationOK = checkedIntegerAdd(left, right)
		case compiler.OpSub:
			value, operationOK = checkedIntegerSub(left, right)
		case compiler.OpMul:
			value, operationOK = checkedIntegerMul(left, right)
		case compiler.OpMod:
			value, operationOK = checkedIntegerMod(left, right)
		case compiler.OpBitAnd:
			value, operationOK = left&right, true
		}
		if !operationOK {
			return fallbackSuffix(index)
		}
		result := core.NewIntegerValue(value)
		if errorValue := core.SetDynamicInstanceVar(calleeReceiver, kernel.instanceVar, result); errorValue != nil {
			cleanup()
			return errorValue, true
		}
		if collect {
			results = append(results, result)
		}
	}
	cleanup()
	core.LastBlockResult = nil
	if collect {
		return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
	}
	return receiver, true
}

func typedSSAEffectfulIntegerKernelFromRegisterIR(plan *registerIRPlan) (typedSSAEffectfulIntegerKernel, bool) {
	if plan == nil || len(plan.instructions) != 10 || plan.sendCount != 1 {
		return typedSSAEffectfulIntegerKernel{}, false
	}
	ops := plan.instructions
	if ops[0].op != registerIRLoadParam || ops[0].param != 0 ||
		ops[1].op != registerIRLoadLiteral || !smallIntegerValue(ops[1].value) ||
		ops[2].op != registerIRCompare || ops[2].left != ops[0].dst || ops[2].right != ops[1].dst ||
		ops[3].op != registerIRJumpNotTruthy || ops[3].left != ops[2].dst || ops[3].target != 7 ||
		ops[4].op != registerIRLoadParam || ops[4].param != 0 ||
		ops[5].op != registerIRSend || ops[5].name != "to_s" || ops[5].argc != 0 || ops[5].blockPresent ||
		ops[5].splatIndex != 255 || ops[5].left != ops[4].dst || ops[5].dst != ops[4].dst ||
		ops[6].op != registerIRJump || ops[6].target != 8 ||
		ops[7].op != registerIRLoadLiteral && ops[7].op != registerIRLoadConstantValue || ops[7].value == nil || ops[7].value.Type != object.ValueString ||
		ops[7].dst != ops[5].dst ||
		ops[8].op != registerIRStoreInstanceVar || ops[8].left != ops[5].dst || ops[8].name == "" ||
		ops[9].op != registerIRReturn || ops[9].left != ops[8].left {
		return typedSSAEffectfulIntegerKernel{}, false
	}
	switch ops[2].opcode {
	case compiler.OpLessThan, compiler.OpLessThanOrEqual, compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual:
	default:
		return typedSSAEffectfulIntegerKernel{}, false
	}
	return typedSSAEffectfulIntegerKernel{
		kind:         typedSSAEffectfulIntegerKernelCompareToSStore,
		compare:      ops[2].opcode,
		compareValue: ops[1].value.Data.(int64),
		falseString:  ops[7].value.Data.(string),
		instanceVar:  ops[8].name,
	}, true
}

type typedEffectfulIntegerStringMapLazyPayload struct {
	inputs    []int64
	input     int64
	length    int
	constant  bool
	values    []*object.EmeraldValue
	lastIndex int
	lastValue *object.EmeraldValue
	kernel    typedSSAEffectfulIntegerKernel
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) lazyLength() int {
	if payload == nil {
		return 0
	}
	return payload.length
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) stringLengthAt(index int) (int64, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return 0, false
	}
	input := payload.input
	if !payload.constant {
		input = payload.inputs[index]
	}
	condition, ok := typedSSAEffectfulIntegerKernelCondition(payload.kernel, input)
	if !ok {
		return 0, false
	}
	if condition {
		return int64(core.IntegerToSLengthRawBuiltin(input)), true
	}
	if !typedSSAASCIIString(payload.kernel.falseString) {
		return 0, false
	}
	return int64(len(payload.kernel.falseString)), true
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) constantStringLength() (int64, bool) {
	if payload == nil || !payload.constant || payload.length == 0 {
		return 0, false
	}
	return payload.stringLengthAt(0)
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	if index == payload.lastIndex && payload.lastValue != nil {
		return payload.lastValue, true
	}
	if index != payload.lastIndex {
		if payload.values != nil {
			if value := payload.values[index]; value != nil {
				return value, true
			}
		} else {
			payload.values = make([]*object.EmeraldValue, payload.length)
		}
	}
	input := payload.input
	if !payload.constant {
		input = payload.inputs[index]
	}
	condition, ok := typedSSAEffectfulIntegerKernelCondition(payload.kernel, input)
	if !ok {
		return nil, false
	}
	var value *object.EmeraldValue
	if condition {
		value = core.NewStringValue(core.IntegerToSRawBuiltin(input))
	} else {
		value = core.NewStringValue(payload.kernel.falseString)
	}
	if index == payload.lastIndex {
		payload.lastValue = value
	} else {
		payload.values[index] = value
	}
	return value, value != nil
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) valueAtBatch(index int, batch *core.StringValueBatch) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	input := payload.input
	if !payload.constant {
		input = payload.inputs[index]
	}
	condition, ok := typedSSAEffectfulIntegerKernelCondition(payload.kernel, input)
	if !ok {
		return nil, false
	}
	if batch != nil {
		if condition {
			return batch.NewInteger(input), true
		}
		return batch.New(payload.kernel.falseString), true
	}
	if condition {
		return core.NewStringValue(core.IntegerToSRawBuiltin(input)), true
	}
	return core.NewStringValue(payload.kernel.falseString), true
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, payload.length)
	}
	if payload.lastValue != nil && payload.lastIndex >= 0 && payload.lastIndex < len(payload.values) {
		payload.values[payload.lastIndex] = payload.lastValue
	}
	batch := core.NewStringValueBatch(payload.length)
	for index := 0; index < payload.length; index++ {
		if payload.values[index] != nil {
			continue
		}
		value, ok := payload.valueAtBatch(index, batch)
		if !ok {
			payload.values[index] = core.R.NilVal
			continue
		}
		payload.values[index] = value
	}
	return payload.values
}

func (payload *typedEffectfulIntegerStringMapLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

func (vm *VM) prepareTypedHotEffectfulCallee(block *object.EmeraldValue, outer *registerIRPlan, generation uint64) (*typedHotTimesCallee, bool) {
	if vm == nil || block == nil || outer == nil || outer.sendCount != 1 || !outer.blockReturn || outer.hasBranches ||
		outer.hasImplicitSends || outer.hasExplicitReturn || len(outer.instructions) != 4 {
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
		if store.op == registerIRStoreInstanceVar && store.left == send.dst && ret.op == registerIRReturn && ret.left == store.left {
			outerStoreName = store.name
		}
	}
	var calleeReceiver *object.EmeraldValue
	var receiverFromSelf, receiverFromFree bool
	var receiverFreeIndex uint8
	var receiverCell *closureCell
	argumentOK := false
	var blockReturnLeft uint8
	blockReturnSeen := false
	for _, instruction := range outer.instructions {
		switch instruction.op {
		case registerIRLoadSelf, registerIRLoadFree, registerIRLoadParam, registerIRLoadLocal:
		case registerIRSend:
			if instruction.opcode != compiler.OpSend || instruction.blockPresent || instruction.splatIndex != 255 || instruction.argc != 1 {
				return nil, false
			}
		case registerIRReturn:
			if blockReturnSeen {
				return nil, false
			}
			blockReturnSeen = true
			blockReturnLeft = instruction.left
		default:
			return nil, false
		}
		if instruction.dst == send.left {
			switch instruction.op {
			case registerIRLoadSelf:
				calleeReceiver = blockBindingSelf(block)
				receiverFromSelf = true
			case registerIRLoadFree:
				if int(instruction.param) < len(closure.Free) {
					captured := closure.Free[instruction.param]
					calleeReceiver = derefClosureValue(captured)
					receiverFromFree = true
					receiverFreeIndex = instruction.param
					if captured != nil {
						receiverCell, _ = captured.Data.(*closureCell)
					}
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
	if send == nil || !blockReturnSeen || blockReturnLeft != send.dst || calleeReceiver == nil || calleeReceiver.Class == nil || !argumentOK {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(calleeReceiver, send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil || method.Closure == nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords || methodUsesRefinements(method) {
		return nil, false
	}
	calleeFn, ok := method.Fn.(*object.Function)
	if !ok || calleeFn == nil || len(calleeFn.Params) != 1 || len(calleeFn.ParamLocalIndices) != 1 ||
		calleeFn.HasRestParam || calleeFn.HasBlockParam || len(calleeFn.KeywordParams) != 0 || calleeFn.KeywordRestParam != "" ||
		calleeFn.KeywordRestOnly || calleeFn.RejectKeywords || calleeFn.RejectBlock ||
		!simpleBlockParameterPatterns(calleeFn) || registerIRFunctionNeedsDefaultEvaluation(calleeFn, 1) {
		return nil, false
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(calleeFn)
	if !ok || calleePlan == nil || !typedSSAEffectfulIntegerPlanSafe(calleePlan) ||
		calleePlan.effectfulIntegerKernel.kind != typedSSAEffectfulIntegerKernelCompareToSStore ||
		!vm.fusedIntegerOperationAvailable(calleePlan.effectfulIntegerKernel.compare) || !core.IntegerToSUsesBuiltinImplementation() ||
		object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	return &typedHotTimesCallee{
		receiver: calleeReceiver, fn: calleeFn, plan: calleePlan, free: method.Closure.Free,
		class: calleeReceiver.Class, integerClass: core.R.IntegerClass,
		receiverFromSelf: receiverFromSelf, receiverFromFree: receiverFromFree,
		receiverFreeIndex: receiverFreeIndex, receiverCell: receiverCell,
		outerStoreName: outerStoreName,
	}, true
}

// tryExecuteArrayTypedHotEffectfulStringMapLazyResult handles the narrow
// `items.map { |item| receiver.update(item) }` graph when update is the
// compare/to_s/instance-store kernel. The callee has no user-code edge after
// admission, so the complete input snapshot can be preflighted before the
// result Array is exposed; only the final receiver ivar write and final String
// object are committed eagerly.
func (vm *VM) tryExecuteArrayTypedHotEffectfulStringMapLazyResult(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool, outer *registerIRPlan) (*object.EmeraldValue, bool) {
	if vm == nil || !typedSSAEffectfulStringMapLazyResultEnabled || !collect || receiver == nil || block == nil || outer == nil ||
		len(elems) < typedHotArrayCallMinElements || vm.typedStringValueBatch != nil || vm.typedStringValueScratch != nil ||
		core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	callee, ok := vm.prepareTypedHotEffectfulCallee(block, outer, generation)
	if !ok || callee == nil || callee.outerStoreName != "" || callee.rawInteger || callee.plan == nil ||
		callee.plan.effectfulIntegerKernel.kind != typedSSAEffectfulIntegerKernelCompareToSStore ||
		callee.receiver == nil || callee.receiver.Type != object.ValueObject || callee.receiver.Frozen {
		return nil, false
	}
	if _, ok := callee.receiver.Data.(*object.Object); !ok || !typedHotTimesReceiverMatches(block, callee) || callee.receiver.Class != callee.class {
		return nil, false
	}
	kernel := callee.plan.effectfulIntegerKernel
	if _, ok := typedSSAEffectfulIntegerKernelCondition(kernel, 0); !ok {
		return nil, false
	}
	integerClass := callee.integerClass
	if integerClass == nil {
		return nil, false
	}
	firstInput := int64(0)
	constantInput := true
	var inputs []int64
	for index, elem := range elems {
		// This loop only snapshots already materialized Integer inputs and
		// evaluates a pure kernel predicate; it cannot execute Ruby code that
		// changes the method generation or the captured receiver. The initial
		// guard and the final guard below therefore cover the whole preflight,
		// avoiding a class/free-cell probe for every element.
		value, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact || core.AttachedSingletonClass(elem) != nil {
			return nil, false
		}
		if index == 0 {
			firstInput = value
		} else if inputs == nil && value != firstInput {
			inputs = make([]int64, len(elems))
			for previousIndex := 0; previousIndex < index; previousIndex++ {
				previous, previousExact := typedSSAExactIntegerValueForClass(elems[previousIndex], integerClass)
				if !previousExact {
					return nil, false
				}
				inputs[previousIndex] = previous
			}
			inputs[index] = value
			constantInput = false
		} else if inputs != nil {
			inputs[index] = value
		}
		if _, ok := typedSSAEffectfulIntegerKernelCondition(kernel, value); !ok {
			return nil, false
		}
	}
	if object.CurrentMethodGeneration() != generation || !typedHotTimesReceiverMatches(block, callee) || callee.receiver.Class != callee.class {
		return nil, false
	}
	payload := &typedEffectfulIntegerStringMapLazyPayload{length: len(elems), lastIndex: len(elems) - 1, kernel: kernel}
	if constantInput {
		payload.input = firstInput
		payload.constant = true
	} else {
		payload.inputs = inputs
	}
	lastValue, ok := payload.valueAt(len(elems) - 1)
	if !ok {
		return nil, false
	}
	if result := core.SetDynamicInstanceVar(callee.receiver, kernel.instanceVar, lastValue); result != nil {
		return result, true
	}
	core.LastBlockResult = nil
	result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
	result.SetLazyArrayRegion(&object.LazyArrayRegion{
		Length: len(elems), Payload: payload, Materialize: payload.materialize,
		ElementAt: payload.elementAt, MethodGeneration: generation,
	})
	core.RegisterLazyArrayRegion(result)
	return result, true
}

func typedSSAPureIntegerStoreOuterShape(plan *registerIRPlan, fn *object.Function) bool {
	if plan == nil || fn == nil || plan.sendCount != 1 || plan.hasBranches || len(plan.instructions) != 5 {
		return false
	}
	var receiverLoad, parameterLoad, send, store, result *registerIRInstruction
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		switch instruction.op {
		case registerIRLoadSelf, registerIRLoadFree:
			if receiverLoad != nil {
				return false
			}
			receiverLoad = instruction
		case registerIRLoadParam:
			if instruction.param != 0 || parameterLoad != nil {
				return false
			}
			parameterLoad = instruction
		case registerIRLoadLocal:
			if int(instruction.param) != fn.ParamLocalIndices[0] || parameterLoad != nil {
				return false
			}
			parameterLoad = instruction
		case registerIRSend:
			if send != nil || instruction.opcode != compiler.OpSend || instruction.argc != 1 ||
				instruction.blockPresent || instruction.splatIndex != 255 || instruction.name == "" {
				return false
			}
			send = instruction
		case registerIRStoreInstanceVar:
			if store != nil || instruction.name == "" {
				return false
			}
			store = instruction
		case registerIRReturn:
			if result != nil {
				return false
			}
			result = instruction
		default:
			return false
		}
	}
	return receiverLoad != nil && parameterLoad != nil && send != nil && store != nil && result != nil &&
		send.left == receiverLoad.dst && send.args[0] == parameterLoad.dst && store.left == send.dst && result.left == store.left
}

// tryExecuteArrayTypedSSAPureIntegerStoreBatch fuses
// `items.map { |item| @ivar = helper.convert(item) }` when convert is the
// pure Integer compare/to_s kernel. The complete input preflight means the
// only visible mutation can be committed once after all values are produced.
func (vm *VM) tryExecuteArrayTypedSSAPureIntegerStoreBatch(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool, fn *object.Function, self *object.EmeraldValue, registerPlan *registerIRPlan) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || block == nil || !collect || fn == nil || self == nil ||
		len(elems) < typedHotArrayCallMinElements || vm.typedStringValueBatch != nil ||
		!typedSSAPureIntegerStoreOuterShape(registerPlan, fn) || self.Type != object.ValueObject || self.Frozen {
		return nil, false
	}
	if _, ok := self.Data.(*object.Object); !ok {
		return nil, false
	}
	var receiverLoad, send, store *registerIRInstruction
	for index := range registerPlan.instructions {
		instruction := &registerPlan.instructions[index]
		switch instruction.op {
		case registerIRLoadSelf, registerIRLoadFree:
			receiverLoad = instruction
		case registerIRSend:
			send = instruction
		case registerIRStoreInstanceVar:
			store = instruction
		}
	}
	if receiverLoad == nil || send == nil || store == nil {
		return nil, false
	}
	calleeReceiver := self
	if receiverLoad.op == registerIRLoadFree {
		closure, ok := block.Data.(*object.Closure)
		if !ok || int(receiverLoad.param) >= len(closure.Free) {
			return nil, false
		}
		calleeReceiver = derefClosureValue(closure.Free[receiverLoad.param])
	}
	if calleeReceiver == nil || calleeReceiver.Class == nil ||
		!vm.registerIRCacheableReceiverForMethod(calleeReceiver, send.name) {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(calleeReceiver, send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords ||
		methodUsesRefinements(method) || method.Closure == nil {
		return nil, false
	}
	calleeFn, ok := method.Fn.(*object.Function)
	if !ok || calleeFn == nil || len(calleeFn.Params) != 1 || len(calleeFn.ParamLocalIndices) != 1 ||
		calleeFn.HasRestParam || calleeFn.HasBlockParam || len(calleeFn.KeywordParams) != 0 ||
		calleeFn.KeywordRestParam != "" || calleeFn.KeywordRestOnly || !simpleBlockParameterPatterns(calleeFn) ||
		registerIRFunctionNeedsDefaultEvaluation(calleeFn, 1) {
		return nil, false
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(calleeFn)
	if !ok || calleePlan == nil || calleePlan.primitiveIntegerStringKernel.kind != typedSSAPrimitiveIntegerStringKernelCompareToS ||
		!vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) || !core.IntegerToSUsesBuiltinImplementation() {
		return nil, false
	}
	kernel := calleePlan.primitiveIntegerStringKernel
	integerClass := core.R.Classes["Integer"]
	if integerClass == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	arguments := make([]int64, len(elems))
	lazyResult := typedSSAEffectfulStringMapLazyResultEnabled && collect && vm.typedStringValueBatch == nil &&
		vm.typedStringValueScratch == nil && !core.ObjectSpaceAllocationTracing()
	maxInt := int64(^uint(0) >> 1)
	batchByteCapacity := int64(0)
	for index, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		argument, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact {
			return nil, false
		}
		arguments[index] = argument
		if !lazyResult {
			condition := false
			switch kernel.compare {
			case compiler.OpLessThan:
				condition = argument < kernel.compareValue
			case compiler.OpLessThanOrEqual:
				condition = argument <= kernel.compareValue
			case compiler.OpGreaterThan:
				condition = argument > kernel.compareValue
			case compiler.OpGreaterThanOrEqual:
				condition = argument >= kernel.compareValue
			default:
				return nil, false
			}
			additionalBytes := int64(len(kernel.falseString))
			if condition {
				additionalBytes = int64(core.IntegerToSLengthRawBuiltin(argument))
			}
			if additionalBytes > maxInt-batchByteCapacity {
				return nil, false
			}
			batchByteCapacity += additionalBytes
		}
	}
	if !lazyResult && batchByteCapacity < int64(len(elems)) {
		return nil, false
	}
	if lazyResult {
		effectfulKernel := typedSSAEffectfulIntegerKernel{
			kind:         typedSSAEffectfulIntegerKernelCompareToSStore,
			compare:      kernel.compare,
			compareValue: kernel.compareValue,
			falseString:  kernel.falseString,
			instanceVar:  store.name,
		}
		payload := &typedEffectfulIntegerStringMapLazyPayload{
			length:    len(arguments),
			lastIndex: len(arguments) - 1,
			kernel:    effectfulKernel,
		}
		constantInput := true
		if len(arguments) > 0 {
			payload.input = arguments[0]
			for _, argument := range arguments[1:] {
				if argument != payload.input {
					constantInput = false
					break
				}
			}
		}
		if constantInput {
			payload.constant = true
		} else {
			payload.inputs = arguments
		}
		lastValue, ok := payload.valueAt(len(arguments) - 1)
		if !ok {
			return nil, false
		}
		if result := core.SetDynamicInstanceVar(self, store.name, lastValue); result != nil {
			return result, true
		}
		core.LastBlockResult = nil
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(arguments), Payload: payload, Materialize: payload.materialize,
			ElementAt: payload.elementAt, MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		return result, true
	}
	previousBatch := vm.typedStringValueBatch
	if typedStringBatchEnabled {
		vm.typedStringValueBatch = core.NewStringValueBatchWithByteCapacity(len(elems), int(batchByteCapacity))
	}
	defer func() { vm.typedStringValueBatch = previousBatch }()
	results := make([]*object.EmeraldValue, 0, len(elems))
	var lastValue *object.EmeraldValue
	for _, argument := range arguments {
		value, executed := vm.executeTypedSSAPrimitiveIntegerStringKernel(kernel, argument)
		if !executed || value == nil {
			return nil, false
		}
		lastValue = value
		results = append(results, value)
	}
	if result := core.SetDynamicInstanceVar(self, store.name, lastValue); result != nil {
		return result, true
	}
	core.LastBlockResult = nil
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

// tryExecuteArrayTypedSSAEffectfulIntegerStoreBatch handles a callback whose
// complete body is the predecoded compare/to_s/instance-store kernel. The
// callback has a visible mutation, so it cannot use the pure preflight batch;
// all input and builtin guards are checked before the first write instead.
// There is no user Ruby code in the committed loop, therefore a generation
// change cannot occur between elements and no partial replay is needed.
func (vm *VM) tryExecuteArrayTypedSSAEffectfulIntegerStoreBatch(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool, fn *object.Function, self *object.EmeraldValue, registerPlan *registerIRPlan) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || block == nil || fn == nil || self == nil || len(elems) < typedHotArrayCallMinElements ||
		vm.typedStringValueScratch != nil {
		return nil, false
	}
	kernel, ok := typedSSAEffectfulIntegerKernelFromRegisterIR(registerPlan)
	if !ok {
		return nil, false
	}
	if !vm.fusedIntegerOperationAvailable(kernel.compare) || !core.IntegerToSUsesBuiltinImplementation() ||
		self.Type != object.ValueObject || self.Frozen {
		return nil, false
	}
	if _, ok := self.Data.(*object.Object); !ok {
		return nil, false
	}
	integerClass := core.R.Classes["Integer"]
	if integerClass == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	lazyResult := typedSSAEffectfulStringMapLazyResultEnabled && collect && vm.typedStringValueBatch == nil &&
		vm.typedStringValueScratch == nil && !core.ObjectSpaceAllocationTracing()
	var lazyInputs []int64
	lazyFirstInput := int64(0)
	lazyConstantInput := true
	if lazyResult {
		lazyInputs = make([]int64, len(elems))
	}
	previousBatch := vm.typedStringValueBatch
	batchEnabled := !lazyResult && collect && typedStringBatchEnabled && previousBatch == nil && len(elems) <= typedStringBatchMaxValues
	batchByteCapacity := int64(0)
	batchByteCapacityOK := batchEnabled
	if batchEnabled {
		maxInt := int64(^uint(0) >> 1)
		falseBytes := int64(len(kernel.falseString))
		count := int64(len(elems))
		if falseBytes > 0 && count > maxInt/falseBytes {
			batchByteCapacityOK = false
		} else {
			batchByteCapacity = falseBytes * count
		}
	}
	for index, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		argument, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact {
			return nil, false
		}
		if lazyResult {
			lazyInputs[index] = argument
			if index == 0 {
				lazyFirstInput = argument
			} else if argument != lazyFirstInput {
				lazyConstantInput = false
			}
		}
		if batchByteCapacityOK {
			maxInt := int64(^uint(0) >> 1)
			digits := int64(core.IntegerToSLengthRawBuiltin(argument))
			if digits > maxInt-batchByteCapacity {
				batchByteCapacityOK = false
			} else {
				batchByteCapacity += digits
			}
		}
	}
	if lazyResult {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		payload := &typedEffectfulIntegerStringMapLazyPayload{
			length: len(elems), lastIndex: len(elems) - 1, values: nil, kernel: kernel,
		}
		if lazyConstantInput {
			payload.input = lazyFirstInput
			payload.constant = true
		} else {
			payload.inputs = lazyInputs
		}
		lastValue, ok := payload.valueAt(len(elems) - 1)
		if !ok {
			return nil, false
		}
		if result := core.SetDynamicInstanceVar(self, kernel.instanceVar, lastValue); result != nil {
			return result, true
		}
		core.LastBlockResult = nil
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(elems), Payload: payload, Materialize: payload.materialize,
			ElementAt: payload.elementAt, MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		return result, true
	}

	if batchEnabled {
		if batchByteCapacityOK {
			vm.typedStringValueBatch = core.NewStringValueBatchWithByteCapacity(len(elems), int(batchByteCapacity))
		} else {
			vm.typedStringValueBatch = core.NewStringValueBatch(len(elems))
		}
	}
	defer func() { vm.typedStringValueBatch = previousBatch }()

	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, 0, len(elems))
	}
	var lastValue *object.EmeraldValue
	for _, elem := range elems {
		argument, _ := typedSSAExactIntegerValueForClass(elem, integerClass)
		value, executed := vm.executeTypedSSAEffectfulIntegerKernelValue(kernel, argument)
		if !executed {
			// The complete input/generation/receiver proof above makes this
			// unreachable under the current kernel shape. Keep the ordinary
			// callback as a conservative fallback if that shape grows later.
			return nil, false
		}
		lastValue = value
		if collect {
			results = append(results, value)
		}
	}
	if result := core.SetDynamicInstanceVar(self, kernel.instanceVar, lastValue); result != nil {
		return result, true
	}
	core.LastBlockResult = nil
	if !collect {
		return receiver, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

// tryExecuteArrayTypedHotCall removes the per-element block Frame and outer
// send protocol for a proven callback-to-Ruby-method edge. Unlike the pure
// typed-SSA batch, this path commits each direct callback result immediately;
// a later generation/type miss replays only the current suffix through the
// ordinary block protocol.
func (vm *VM) tryExecuteArrayTypedHotCall(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || !typedHotArrayCallEnabled || receiver == nil || receiver.Type != object.ValueArray ||
		block == nil || block.Type != object.ValueClosure || !registerIRBatchBlockEnabled ||
		!registerIRNoFrameEnabled || !registerIRDirectNoFrameEnabled || vm.instructionLimit != 0 ||
		DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok {
		return nil, false
	}
	if result, handled := vm.tryExecuteArrayTypedSSAEffectfulIntegerUpdate(receiver, block, elems, collect); handled {
		return result, true
	}
	if len(elems) < typedHotArrayCallMinElements {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 || closureUsesRefinements(closure) ||
		closure.AutoSplat && blockWantsDestructuring(closure.Fn) || !registerIRClosureControlFlowSafe(closure.Fn) {
		return nil, false
	}
	fn := closure.Fn
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	plan, found := vm.cachedBlockLeafPlan(fn)
	if !found || plan.kind != leafMethodRegisterIR || plan.registerIR == nil ||
		!registerIRPlanSafeForTypedHotArrayCall(plan.registerIR) ||
		!registerIRDirectConstantsSafe(vm, closure, plan.registerIR) {
		return nil, false
	}
	// The dedicated Array Register IR tier already executes this exact
	// straight-line arithmetic shape with raw int64 values. Letting the more
	// general callback-to-callee tier claim it first rebuilds a per-element
	// direct-plan register path and is substantially slower for map/each.
	if plan.registerIR.integerOnly && plan.registerIR.integerLinear &&
		registerIRIntegerLinearInputMatchesParameter(plan.registerIR, fn) {
		return nil, false
	}
	self := blockBindingSelf(block)
	if self == nil || fn.NumLocals > 64 {
		return nil, false
	}
	if result, handled := vm.tryExecuteArrayTypedSSAPureIntegerStoreBatch(receiver, block, elems, collect, fn, self, plan.registerIR); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSAEffectfulIntegerStoreBatch(receiver, block, elems, collect, fn, self, plan.registerIR); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedHotEffectfulStringMapLazyResult(receiver, block, elems, collect, plan.registerIR); handled {
		return result, true
	}

	prevBlock := vm.currentBlock
	prevClassStack := vm.classStack
	prevStringBatch := vm.typedStringValueBatch
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack
		vm.typedStringValueBatch = prevStringBatch
	}
	var collected []*object.EmeraldValue
	if collect {
		collected = make([]*object.EmeraldValue, 0, len(elems))
	}
	finish := func() (*object.EmeraldValue, bool) {
		cleanup()
		core.LastBlockResult = nil
		if collect {
			return &object.EmeraldValue{Type: object.ValueArray, Data: collected, Class: core.R.Classes["Array"]}, true
		}
		return receiver, true
	}
	fallback := func(start int) (*object.EmeraldValue, bool) {
		cleanup()
		var fallbackArgs [1]*object.EmeraldValue
		for index := start; index < len(elems); index++ {
			value := elems[index]
			if value == nil {
				value = core.R.NilVal
			}
			fallbackArgs[0] = value
			core.LastBlockResult = nil
			if !collect {
				core.ForEachClearNext()
			}
			result := vm.callBlockWithSelfArgs(block, self, fallbackArgs[:])
			if core.LastBlockResult != nil {
				breakResult := core.LastBlockResult
				core.LastBlockResult = nil
				return breakResult, true
			}
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
			if !collect && core.ForEachConsumeNext() {
				continue
			}
			if collect {
				collected = append(collected, result)
			}
		}
		return finish()
	}

	generation := object.CurrentMethodGeneration()
	trustedPlan := false
	var typedCallee *typedHotTimesCallee
	typedCalleeReceiverStable := false
	var args [1]*object.EmeraldValue
	for index, value := range elems {
		if object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				core.LastBlockResult = nil
				return nil, false
			}
			return fallback(index)
		}
		if value == nil {
			value = core.R.NilVal
		}
		args[0] = value
		core.LastBlockResult = nil
		if !collect {
			core.ForEachClearNext()
		}
		var result *object.EmeraldValue
		var executed bool
		if typedCallee != nil {
			if !typedCalleeReceiverStable {
				if !typedHotTimesReceiverMatches(block, typedCallee) || typedCallee.receiver.Class != typedCallee.class {
					return fallback(index)
				}
				typedCalleeReceiverStable = true
			}
			if typedCallee.rawInteger {
				raw, exact := typedSSAExactIntegerValueForClass(value, typedCallee.integerClass)
				if !exact {
					return fallback(index)
				}
				result, executed = vm.executeTypedSSAEffectfulIntegerPlan(
					typedCallee.plan, typedCallee.fn, typedCallee.receiver, raw, typedCallee.free,
				)
			} else {
				result, executed = vm.executeTypedSSAEffectfulIntegerObjectPlan(
					typedCallee.plan, typedCallee.fn, typedCallee.receiver, value, typedCallee.integerClass, typedCallee.free,
				)
			}
		} else {
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
		if !trustedPlan {
			trustedPlan = registerIRTrustedArrayCallbackReady(plan.registerIR, generation)
			if trustedPlan {
				typedCallee, _ = vm.prepareTypedHotTimesCallee(block, plan.registerIR, generation)
				if collect && typedStringBatchEnabled && len(elems) <= typedStringBatchMaxValues && typedCallee != nil && vm.typedStringValueBatch == nil &&
					(typedCallee.rawInteger && typedSSAPlanCanUseUnboxedStringResult(typedCallee.plan) ||
						typedCallee.plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelCompareToSStore) {
					vm.typedStringValueBatch = core.NewStringValueBatch(len(elems))
				}
			}
		}
		if result == nil {
			result = core.R.NilVal
		}
		if result.Type == object.ValueException {
			cleanup()
			return result, true
		}
		if typedCallee != nil && typedCallee.outerStoreName != "" {
			if storeResult := core.SetDynamicInstanceVar(self, typedCallee.outerStoreName, result); storeResult != nil {
				cleanup()
				return storeResult, true
			}
		}
		if core.LastBlockResult != nil {
			breakResult := core.LastBlockResult
			core.LastBlockResult = nil
			cleanup()
			return breakResult, true
		}
		if !collect && core.ForEachConsumeNext() {
			continue
		}
		if collect {
			collected = append(collected, result)
		}
		if collect && typedCallee != nil && typedCallee.rawInteger &&
			typedCallee.plan.primitiveIntegerStringKernel.kind == typedSSAPrimitiveIntegerStringKernelCompareToS {
			// The first element has already crossed the ordinary direct path and
			// validated the callback receiver, method generation, Integer class,
			// and builtin to_s guard. This pure kernel cannot execute Ruby code or
			// mutate anything except the outer terminal ivar, so the remaining
			// suffix needs only one generation/type check per element. A mismatch
			// replays the unexecuted suffix through the semantic fallback.
			for tailIndex := index + 1; tailIndex < len(elems); tailIndex++ {
				if object.CurrentMethodGeneration() != generation {
					return fallback(tailIndex)
				}
				value := elems[tailIndex]
				if value == nil {
					value = core.R.NilVal
				}
				raw, exact := typedSSAExactIntegerValueForClass(value, typedCallee.integerClass)
				if !exact {
					return fallback(tailIndex)
				}
				result, executed := vm.executeTypedSSAPrimitiveIntegerStringKernel(
					typedCallee.plan.primitiveIntegerStringKernel, raw,
				)
				if !executed || result == nil {
					return fallback(tailIndex)
				}
				if typedCallee.outerStoreName != "" {
					if storeResult := setTypedHotOuterInstanceVar(self, typedCallee.outerStoreName, result); storeResult != nil {
						cleanup()
						return storeResult, true
					}
				}
				collected = append(collected, result)
			}
			return finish()
		}
	}
	return finish()
}

// setTypedHotOuterInstanceVar is used only after the first ordinary write has
// succeeded and the callback has been proven to contain no user Ruby code.
// The compatibility object layout is map-backed, so updating the known ivar
// directly avoids repeating the dynamic receiver/type/frozen dispatch on every
// element. Other ValueObject payloads and compact layouts retain the normal
// setter path.
func setTypedHotOuterInstanceVar(receiver *object.EmeraldValue, name string, value *object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil && receiver.Type == object.ValueObject && !receiver.Frozen {
		if data, ok := receiver.Data.(*object.Object); ok && data != nil {
			if data.InstanceVars != nil {
				data.InstanceVars[name] = value
				return nil
			}
			data.SetInstanceVar(name, value)
			return nil
		}
	}
	return core.SetDynamicInstanceVar(receiver, name, value)
}
