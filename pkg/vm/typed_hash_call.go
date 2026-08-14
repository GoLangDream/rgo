package vm

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedHashCallEnabled controls the narrow Hash#each call-graph tier. The
// normal Hash loop and the integer reduction tier remain independent so this
// path can be bisected without changing their admission rules.
var typedHashCallEnabled = os.Getenv("RGO_DISABLE_TYPED_HASH_CALL") == ""
var typedHashDirectCallEnabled = os.Getenv("RGO_DISABLE_TYPED_HASH_DIRECT_CALL") == ""
var typedIntegerStringBatchEnabled = os.Getenv("RGO_DISABLE_TYPED_INTEGER_STRING_BATCH") == ""
var typedHashArrayBatchAppendEnabled = os.Getenv("RGO_DISABLE_TYPED_HASH_ARRAY_BATCH_APPEND") == ""
var typedHashLazyTargetEnabled = os.Getenv("RGO_DISABLE_TYPED_HASH_LAZY_TARGET") == ""

const typedHashCallMinEntries = 1024

type typedHashCallShape struct {
	targetFree   uint8
	helperFree   uint8
	target       *object.EmeraldValue
	helper       *object.EmeraldValue
	send         *registerIRInstruction
	callee       *object.Function
	calleePlan   *typedSSAPlan
	directPlan   *registerIRPlan
	integerClass *object.Class
	kernel       typedHashCallIntegerStringKernel
}

type typedHashCallIntegerStringKernel struct {
	valid       bool
	multiplyArg uint8
	addArg      uint8
	multiplier  int64
}

// typedHashIntegerStringLazyPayload is the deferred result side of the
// proven `target << helper.render(key, value)` Hash#each graph. The callback
// has no Ruby-visible effects beyond appending the formatter result, so the
// target Array can retain raw integer pairs until an operation actually reads
// one of the strings. The raw pair snapshot is important: Hash mutation,
// helper ivar mutation, and method redefinition after each returns must not
// change an already-completed Ruby iteration.
type typedHashIntegerStringLazyPayload struct {
	keys        []int64
	values      []int64
	linear      bool
	start       int64
	step        int64
	valueOffset int64
	length      int
	results     []*object.EmeraldValue
	lastIndex   int
	lastValue   *object.EmeraldValue

	// Generic typed-SSA kernel: result = raw[MultiplyArg] * Multiplier +
	// raw[AddArg], then Integer#to_s.
	multiplyArg uint8
	addArg      uint8
	multiplier  int64

	// Direct formatter kernel adds a guarded prefix and a cold fallback branch.
	direct       bool
	conditionArg uint8
	threshold    int64
	prefix       string
	fallback     string
}

func (payload *typedHashIntegerStringLazyPayload) rawPairAt(index int) (int64, int64, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return 0, 0, false
	}
	if payload.linear {
		key := payload.start + payload.step*int64(index)
		value, ok := checkedIntegerAdd(key, payload.valueOffset)
		return key, value, ok
	}
	if index >= len(payload.keys) || index >= len(payload.values) {
		return 0, 0, false
	}
	return payload.keys[index], payload.values[index], true
}

func (payload *typedHashIntegerStringLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	if index == payload.lastIndex && payload.lastValue != nil {
		return payload.lastValue, true
	}
	if index != payload.lastIndex {
		if payload.results != nil {
			if value := payload.results[index]; value != nil {
				return value, true
			}
		} else {
			payload.results = make([]*object.EmeraldValue, payload.length)
		}
	}
	key, value, ok := payload.rawPairAt(index)
	if !ok {
		return nil, false
	}
	left, right := key, value
	if payload.multiplyArg == 1 {
		left = value
	}
	if payload.addArg == 0 {
		right = key
	} else {
		right = value
	}
	condition := key
	if payload.conditionArg == 1 {
		condition = value
	}
	if payload.direct && condition <= payload.threshold {
		result := core.NewStringValue(payload.fallback)
		if index == payload.lastIndex {
			payload.lastValue = result
		} else {
			payload.results[index] = result
		}
		return result, result != nil
	}
	product, ok := checkedIntegerMul(left, payload.multiplier)
	if !ok {
		return nil, false
	}
	total, ok := checkedIntegerAdd(product, right)
	if !ok {
		return nil, false
	}
	text := core.IntegerToSRawBuiltin(total)
	if payload.direct {
		text = payload.prefix + text
	}
	result := core.NewStringValue(text)
	if index == payload.lastIndex {
		payload.lastValue = result
	} else {
		payload.results[index] = result
	}
	return result, result != nil
}

// valueAtBatch is used only while committing a full lazy region. Keeping the
// single-index path allocation-minimal is what makes the region useful for a
// partial read; full materialization can instead reuse the same StringValue-
// Batch ABI as the eager Hash kernel and avoid one EmeraldValue allocation per
// result.
func (payload *typedHashIntegerStringLazyPayload) valueAtBatch(index int, batch *core.StringValueBatch) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	key, value, ok := payload.rawPairAt(index)
	if !ok {
		return nil, false
	}
	condition := key
	if payload.conditionArg == 1 {
		condition = value
	}
	if payload.direct && condition <= payload.threshold {
		if batch != nil {
			return batch.New(payload.fallback), true
		}
		return core.NewStringValue(payload.fallback), true
	}
	left, right := key, value
	if payload.multiplyArg == 1 {
		left = value
	}
	if payload.addArg == 0 {
		right = key
	}
	product, ok := checkedIntegerMul(left, payload.multiplier)
	if !ok {
		return nil, false
	}
	total, ok := checkedIntegerAdd(product, right)
	if !ok {
		return nil, false
	}
	if batch != nil {
		if payload.direct {
			return batch.NewPrefixInteger(payload.prefix, total), true
		}
		return batch.NewInteger(total), true
	}
	text := core.IntegerToSRawBuiltin(total)
	if payload.direct {
		text = payload.prefix + text
	}
	return core.NewStringValue(text), true
}

func (payload *typedHashIntegerStringLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	if payload.results == nil {
		payload.results = make([]*object.EmeraldValue, payload.length)
	}
	if payload.lastValue != nil && payload.lastIndex >= 0 && payload.lastIndex < len(payload.results) {
		payload.results[payload.lastIndex] = payload.lastValue
	}
	batch := core.NewStringValueBatch(payload.length)
	for index := 0; index < payload.length; index++ {
		if payload.results[index] != nil {
			continue
		}
		value, ok := payload.valueAtBatch(index, batch)
		if !ok {
			payload.results[index] = core.R.NilVal
			continue
		}
		payload.results[index] = value
	}
	return payload.results
}

func (payload *typedHashIntegerStringLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

func detectTypedHashCallIntegerStringKernel(plan *typedSSAPlan) typedHashCallIntegerStringKernel {
	if plan == nil || len(plan.ops) != 7 || plan.hasReference || plan.hasFloat || plan.hasInstanceStore || plan.hasYield {
		return typedHashCallIntegerStringKernel{}
	}
	ops := plan.ops
	if ops[0].kind != typedSSAOpLoadParam || ops[1].kind != typedSSAOpLoadLiteral ||
		ops[1].literal.kind != typedSSAInteger || ops[2].kind != typedSSAOpBinary ||
		ops[2].opcode != compiler.OpMul || ops[2].left != ops[0].dst || ops[2].right != ops[1].dst ||
		ops[3].kind != typedSSAOpLoadParam || ops[3].param == ops[0].param ||
		ops[4].kind != typedSSAOpBinary || ops[4].opcode != compiler.OpAdd ||
		ops[4].left != ops[2].dst || ops[4].right != ops[3].dst ||
		ops[5].kind != typedSSAOpCall || ops[5].name != "to_s" || ops[5].argc != 0 || ops[5].implicit ||
		ops[5].left != ops[4].dst || ops[6].kind != typedSSAOpReturn || ops[6].left != ops[5].dst {
		return typedHashCallIntegerStringKernel{}
	}
	if ops[0].param > 1 || ops[3].param > 1 {
		return typedHashCallIntegerStringKernel{}
	}
	return typedHashCallIntegerStringKernel{
		valid: true, multiplyArg: ops[0].param, addArg: ops[3].param, multiplier: ops[1].literal.int,
	}
}

func (vm *VM) cachedTypedHashDirectPlan(fn *object.Function) *registerIRPlan {
	if vm == nil || fn == nil {
		return nil
	}
	if vm.typedHashDirectPlans == nil {
		vm.typedHashDirectPlans = make(map[*object.Function]*registerIRPlan)
	}
	if plan, found := vm.typedHashDirectPlans[fn]; found {
		return plan
	}
	options := defaultRegisterIRCompileOptions()
	options.allowStringLiterals = true
	var plan *registerIRPlan
	if compiled, compiledOK := compileRegisterIRWithOptions(fn, options); compiledOK &&
		compiled != nil && compiled.directFastKind == registerIRDirectFastIntegerStringConcat &&
		registerIRPlanSafeForDirectNoFrameWithOptions(compiled, false, true) {
		plan = compiled
	}
	vm.typedHashDirectPlans[fn] = plan
	return plan
}

// prepareTypedHashCallShape recognizes exactly:
//
//	values.each { |key, value| mapped << helper.render(key, value) }
//
// The outer block is intentionally matched as a seven-instruction graph. Any
// extra operation, second send, control-flow edge, or unrecognized producer
// stays on the ordinary block/frame path.
func (vm *VM) prepareTypedHashCallShape(block *object.EmeraldValue) (typedHashCallShape, bool) {
	invalid := typedHashCallShape{}
	if vm == nil || !typedHashCallEnabled || block == nil || block.Type != object.ValueClosure ||
		!typedSSABatchCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 ||
		DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() {
		return invalid, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || len(closure.Free) != 2 ||
		closure.BreakOwnerID > 0 || closureUsesRefinements(closure) ||
		!registerIRClosureControlFlowSafe(closure.Fn) {
		return invalid, false
	}
	fn := closure.Fn
	if len(fn.Params) != 2 || len(fn.ParamLocalIndices) != 2 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.RejectKeywords || fn.RejectBlock || !simpleBlockParameterPatterns(fn) {
		return invalid, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}
	leaf, found := vm.cachedBlockLeafPlan(fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil {
		return invalid, false
	}
	outer := leaf.registerIR
	if !outer.blockReturn || outer.hasBranches || outer.hasImplicitSends || outer.hasExplicitReturn ||
		outer.sendCount != 1 || len(outer.instructions) != 7 {
		return invalid, false
	}

	var parameterDst [2]uint8
	var parameterLoaded [2]bool
	var freeLoads [2]registerIRInstruction
	freeCount := 0
	var send *registerIRInstruction
	var appendOp *registerIRInstruction
	var blockReturn *registerIRInstruction
	for index := range outer.instructions {
		instruction := &outer.instructions[index]
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param >= 2 || parameterLoaded[instruction.param] {
				return invalid, false
			}
			parameterLoaded[instruction.param] = true
			parameterDst[instruction.param] = instruction.dst
		case registerIRLoadLocal:
			parameter := -1
			for candidate, local := range fn.ParamLocalIndices {
				if int(instruction.param) == local {
					parameter = candidate
					break
				}
			}
			if parameter < 0 || parameterLoaded[parameter] {
				return invalid, false
			}
			parameterLoaded[parameter] = true
			parameterDst[parameter] = instruction.dst
		case registerIRLoadFree:
			if freeCount >= len(freeLoads) || instruction.param >= uint8(len(closure.Free)) {
				return invalid, false
			}
			for previous := 0; previous < freeCount; previous++ {
				if freeLoads[previous].param == instruction.param {
					return invalid, false
				}
			}
			freeLoads[freeCount] = *instruction
			freeCount++
		case registerIRSend:
			if send != nil || instruction.opcode != compiler.OpSend || instruction.blockPresent ||
				instruction.splatIndex != 255 || instruction.argc != 2 || instruction.name == "" {
				return invalid, false
			}
			send = instruction
		case registerIRBinary:
			if appendOp != nil || instruction.opcode != compiler.OpBitLeftShift {
				return invalid, false
			}
			appendOp = instruction
		case registerIRReturn:
			if blockReturn != nil {
				return invalid, false
			}
			blockReturn = instruction
		default:
			return invalid, false
		}
	}
	if send == nil || appendOp == nil || blockReturn == nil || freeCount != 2 ||
		!parameterLoaded[0] || !parameterLoaded[1] || send.args[0] != parameterDst[0] ||
		send.args[1] != parameterDst[1] || appendOp.right != send.dst ||
		blockReturn.left != appendOp.dst {
		return invalid, false
	}

	targetFree := uint8(255)
	helperFree := uint8(255)
	for index := 0; index < freeCount; index++ {
		load := freeLoads[index]
		if load.dst == appendOp.left {
			targetFree = load.param
		}
		if load.dst == send.left {
			helperFree = load.param
		}
	}
	if targetFree == 255 || helperFree == 255 || targetFree == helperFree {
		return invalid, false
	}
	target := derefClosureValue(closure.Free[targetFree])
	helper := derefClosureValue(closure.Free[helperFree])
	if target == nil || target.Type != object.ValueArray || target.Class != core.R.Classes["Array"] ||
		target.Frozen || core.AttachedSingletonClass(target) != nil || helper == nil || helper.Class == nil ||
		!vm.registerIRCacheableReceiverForMethod(helper, send.name) ||
		!core.ArrayAppendUsesBuiltinImplementation() || !core.HashEachUsesBuiltinImplementation() {
		return invalid, false
	}

	methodObj, _, fallback := vm.lookupMethodForSend(helper, send.name, nil, false, true)
	if fallback != nil || methodObj == nil || methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodUsesRefinements(methodObj) {
		return invalid, false
	}
	callee, ok := methodObj.Fn.(*object.Function)
	if !ok || callee == nil || methodObj.Closure == nil || len(callee.Params) != 2 ||
		len(callee.ParamLocalIndices) != 2 || callee.HasRestParam || callee.HasBlockParam ||
		len(callee.KeywordParams) != 0 || callee.KeywordRestParam != "" || callee.KeywordRestOnly ||
		callee.RejectKeywords || callee.RejectBlock || !simpleBlockParameterPatterns(callee) ||
		registerIRFunctionNeedsDefaultEvaluation(callee, 2) {
		return invalid, false
	}
	for _, defaultValue := range callee.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}
	if typedHashDirectCallEnabled {
		var directPlan *registerIRPlan
		if calleeLeaf, leafFound := vm.cachedBlockLeafPlan(callee); leafFound &&
			calleeLeaf.kind == leafMethodRegisterIR && calleeLeaf.registerIR != nil &&
			calleeLeaf.registerIR.directFastKind == registerIRDirectFastIntegerStringConcat {
			directPlan = calleeLeaf.registerIR
		}
		// The normal leaf cache deliberately rejects mutable String literals so
		// the broad Register IR tier keeps its compatibility default. This exact
		// direct Hash callback has a stronger structural proof and the executor
		// already clones its cold fallback literal per invocation. Compile only
		// this candidate with literals enabled; do not widen the global cache.
		if directPlan == nil {
			directPlan = vm.cachedTypedHashDirectPlan(callee)
		}
		if directPlan != nil {
			return typedHashCallShape{
				targetFree: targetFree, helperFree: helperFree, target: target, helper: helper,
				send: send, callee: callee, directPlan: directPlan,
				integerClass: core.R.Classes["Integer"],
			}, true
		}
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(callee)
	if !ok || calleePlan == nil || calleePlan.blockReturn || calleePlan.hasReference || calleePlan.hasFloat ||
		calleePlan.hasInstanceStore || calleePlan.hasYield || !typedSSAPlanCanUseUnboxedStringResult(calleePlan) ||
		!vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) {
		return invalid, false
	}
	for _, operation := range calleePlan.ops {
		if operation.kind != typedSSAOpCall {
			continue
		}
		if operation.name != "to_s" || operation.argc != 0 || operation.implicit {
			return invalid, false
		}
	}
	if !core.IntegerToSUsesBuiltinImplementation() && typedSSAPlanHasIntegerToS(calleePlan) {
		return invalid, false
	}

	return typedHashCallShape{
		targetFree: targetFree, helperFree: helperFree, target: target, helper: helper,
		send: send, callee: callee, calleePlan: calleePlan, integerClass: core.R.Classes["Integer"],
		kernel: detectTypedHashCallIntegerStringKernel(calleePlan),
	}, true
}

func typedSSAPlanHasIntegerToS(plan *typedSSAPlan) bool {
	if plan == nil {
		return false
	}
	for _, operation := range plan.ops {
		if operation.kind == typedSSAOpCall && operation.name == "to_s" {
			return true
		}
	}
	return false
}

func typedHashCallFreeMatches(closure *object.Closure, index uint8, expected *object.EmeraldValue) bool {
	return closure != nil && int(index) < len(closure.Free) && derefClosureValue(closure.Free[index]) == expected
}

func typedHashCallReceiverStable(vm *VM, shape typedHashCallShape, closure *object.Closure) bool {
	if vm == nil || closure == nil || shape.target == nil || shape.helper == nil ||
		!typedHashCallFreeMatches(closure, shape.targetFree, shape.target) ||
		!typedHashCallFreeMatches(closure, shape.helperFree, shape.helper) {
		return false
	}
	return shape.target.Type == object.ValueArray && shape.target.Class == core.R.Classes["Array"] &&
		!shape.target.Frozen && core.AttachedSingletonClass(shape.target) == nil &&
		shape.helper.Class != nil && vm.registerIRCacheableReceiverForMethod(shape.helper, shape.send.name)
}

// tryExecuteHashIntegerStringLazyTarget turns the narrow formatter callback
// into a lazy Array region. The outer block is already proven to contain only
// one exact Array#<< and one pure, fixed-arity formatter call. We therefore
// execute the complete integer/generation preflight before publishing the
// lazy target, then defer only String allocation and boxing. A target that is
// non-empty, aliased through a pre-existing lazy region, or otherwise has a
// non-canonical layout stays on the existing eager path.
func (vm *VM) tryExecuteHashIntegerStringLazyTarget(shape typedHashCallShape, receiver *object.EmeraldValue, keys []*object.EmeraldValue, hash map[*object.EmeraldValue]*object.EmeraldValue, linearRegion *object.RHashLinearRegion, linearRegionOK bool, generation uint64) bool {
	if vm == nil || !typedHashLazyTargetEnabled || shape.target == nil || shape.helper == nil ||
		generation != object.CurrentMethodGeneration() || len(keys) < typedHashCallMinEntries ||
		shape.target.LazyArrayRegionValue() != nil || !core.ArrayAppendUsesBuiltinImplementation() {
		return false
	}
	if data := shape.target.Data; data != nil {
		elements, ok := data.([]*object.EmeraldValue)
		if !ok || len(elements) != 0 {
			return false
		}
	}

	payload := &typedHashIntegerStringLazyPayload{
		length:    0,
		lastIndex: -1,
	}
	switch {
	case shape.directPlan != nil && shape.directPlan.directFastKind == registerIRDirectFastIntegerStringConcat:
		plan := shape.directPlan
		if !vm.fusedIntegerOperationAvailable(compiler.OpGreaterThan) ||
			!vm.fusedIntegerOperationAvailable(compiler.OpMul) ||
			!vm.fusedIntegerOperationAvailable(compiler.OpAdd) ||
			!vm.fusedIntegerToSAvailable() || !vm.fusedStringOperationAvailable(compiler.OpAdd) {
			return false
		}
		prefix := core.DynamicInstanceVar(shape.helper, plan.directFastIntegerStringPrefixIvar)
		if prefix == nil || prefix.Type != object.ValueString || prefix.Class != core.R.Classes["String"] ||
			core.AttachedSingletonClass(prefix) != nil {
			return false
		}
		prefixRaw, ok := prefix.Data.(string)
		if !ok || prefix.Encoding != "" && !strings.EqualFold(prefix.Encoding, core.DefaultExternalEncoding()) ||
			stringHasNonASCIIByteForVM(prefixRaw) {
			return false
		}
		fallbackIndex := int(plan.directFastIntegerStringFallback)
		if fallbackIndex < 0 || fallbackIndex >= len(plan.instructions) {
			return false
		}
		fallbackValue := plan.instructions[fallbackIndex].value
		if fallbackValue == nil || fallbackValue.Type != object.ValueString || fallbackValue.Frozen ||
			plan.instructions[fallbackIndex].op == registerIRLoadFrozenString {
			return false
		}
		fallbackRaw, ok := fallbackValue.Data.(string)
		if !ok {
			return false
		}
		payload.direct = true
		payload.conditionArg = plan.directFastIntegerStringValueParam
		payload.multiplyArg = plan.directFastIntegerStringKeyParam
		payload.addArg = plan.directFastIntegerStringValueParam
		payload.multiplier = plan.directFastIntegerStringMultiplier
		payload.threshold = plan.directFastIntegerStringThreshold
		payload.prefix = strings.Clone(prefixRaw)
		payload.fallback = strings.Clone(fallbackRaw)
	case shape.kernel.valid:
		if !vm.fusedIntegerOperationAvailable(compiler.OpMul) ||
			!vm.fusedIntegerOperationAvailable(compiler.OpAdd) || !vm.fusedIntegerToSAvailable() {
			return false
		}
		payload.multiplyArg = shape.kernel.multiplyArg
		payload.addArg = shape.kernel.addArg
		payload.multiplier = shape.kernel.multiplier
	default:
		return false
	}

	if linearRegionOK && hash == nil {
		if linearRegion == nil || linearRegion.Count != len(keys) || linearRegion.Count <= 0 {
			return false
		}
		// Copy the affine metadata, not the region pointer. A later Hash API
		// clears the source region when it materializes or mutates the Hash;
		// the completed iteration must remain a snapshot.
		payload.linear = true
		payload.start = linearRegion.Start
		payload.step = linearRegion.Step
		payload.valueOffset = linearRegion.ValueOffset
		payload.length = linearRegion.Count
	} else {
		if hash == nil {
			return false
		}
		payload.keys = make([]int64, 0, len(keys))
		payload.values = make([]int64, 0, len(keys))
		integerClass := shape.integerClass
		if integerClass == nil {
			integerClass = core.R.IntegerClass
		}
		for _, key := range keys {
			if object.CurrentMethodGeneration() != generation {
				return false
			}
			keyInteger, keyOK := typedSSAExactIntegerValueForClass(key, integerClass)
			if !keyOK {
				return false
			}
			value, exists := hash[key]
			if !exists {
				continue
			}
			valueInteger, valueOK := typedSSAExactIntegerValueForClass(value, integerClass)
			if !valueOK {
				return false
			}
			payload.keys = append(payload.keys, keyInteger)
			payload.values = append(payload.values, valueInteger)
		}
		payload.length = len(payload.keys)
	}
	if payload.length <= 0 || object.CurrentMethodGeneration() != generation {
		return false
	}
	// Complete arithmetic/branch preflight before publishing the region. This
	// guarantees that a later ElementAt cannot discover an overflow after the
	// Ruby callback has already been logically committed.
	for index := 0; index < payload.length; index++ {
		key, value, ok := payload.rawPairAt(index)
		if !ok {
			return false
		}
		if payload.direct {
			condition := key
			if payload.conditionArg == 1 {
				condition = value
			}
			if condition <= payload.threshold {
				continue
			}
		}
		left, right := key, value
		if payload.multiplyArg == 1 {
			left = value
		}
		if payload.addArg == 0 {
			right = key
		}
		product, ok := checkedIntegerMul(left, payload.multiplier)
		if !ok {
			return false
		}
		if _, ok = checkedIntegerAdd(product, right); !ok {
			return false
		}
	}
	if object.CurrentMethodGeneration() != generation {
		return false
	}
	payload.lastIndex = payload.length - 1
	shape.target.SetLazyArrayRegion(&object.LazyArrayRegion{
		Length:           payload.length,
		Payload:          payload,
		Materialize:      payload.materialize,
		ElementAt:        payload.elementAt,
		MethodGeneration: generation,
	})
	core.RegisterLazyArrayRegion(shape.target)
	core.LastBlockResult = nil
	return true
}

// tryExecuteHashTypedCall removes the per-entry block/frame protocol for the
// exact helper-plus-array-append graph. A guard miss replays only the current
// suffix through Hash#each's normal callback ABI, preserving already appended
// prefix values without duplicating them.
func (vm *VM) tryExecuteHashTypedCall(receiver, block *object.EmeraldValue, keys []*object.EmeraldValue, hash map[*object.EmeraldValue]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	linearRegion, linearRegionOK := core.DirectHashLinearRegion(receiver)
	if vm == nil || !typedHashCallEnabled || receiver == nil || receiver.Type != object.ValueHash ||
		receiver.Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(receiver) != nil ||
		block == nil || block.Type != object.ValueClosure || len(keys) < typedHashCallMinEntries || hash == nil && !linearRegionOK ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.threadDepth > 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	if core.CallBlockWithArgs == nil && core.CallBlock == nil {
		return nil, false
	}
	shape, ok := vm.prepareTypedHashCallShape(block)
	if !ok {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if !typedHashCallReceiverStable(vm, shape, block.Data.(*object.Closure)) {
		return nil, false
	}
	if tryLazy := vm.tryExecuteHashIntegerStringLazyTarget(shape, receiver, keys, hash, linearRegion, linearRegionOK, generation); tryLazy {
		return receiver, true
	}
	if shape.directPlan != nil {
		return vm.executeTypedHashDirectCall(shape, receiver, block, keys, hash, generation)
	}
	previousBlock := vm.currentBlock
	previousClassStack := vm.classStack
	previousStringBatch := vm.typedStringValueBatch
	vm.currentBlock = nil
	if closure, ok := block.Data.(*object.Closure); ok && closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = previousBlock
		vm.classStack = previousClassStack
		vm.typedStringValueBatch = previousStringBatch
	}
	fallback := func(start int) (*object.EmeraldValue, bool) {
		cleanup()
		if hash == nil {
			hash = core.DirectHashMaterialize(receiver)
		}
		for index := start; index < len(keys); index++ {
			key := keys[index]
			value, exists := hash[key]
			if !exists {
				continue
			}
			core.LastBlockResult = nil
			core.ForEachClearNext()
			var result *object.EmeraldValue
			if core.CallBlockWithArgs != nil {
				result = core.CallBlockWithArgs(block, key, value)
			} else {
				result = core.CallBlock(key, value)
			}
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
			if core.LastBlockResult != nil {
				control := core.LastBlockResult
				core.LastBlockResult = nil
				return control, true
			}
			if core.ForEachConsumeNext() {
				continue
			}
		}
		core.LastBlockResult = nil
		return receiver, true
	}

	batchByteCapacity := len(keys) * 16
	if shape.kernel.valid && typedIntegerStringBatchEnabled {
		batchByteCapacity = len(keys) * 8
	}
	vm.typedStringValueBatch = core.NewStringValueBatchWithByteCapacity(len(keys), batchByteCapacity)
	var pending []*object.EmeraldValue
	if typedHashArrayBatchAppendEnabled {
		pending = make([]*object.EmeraldValue, 0, len(keys))
	}
	flushPending := func() bool {
		if !typedHashArrayBatchAppendEnabled || len(pending) == 0 {
			return true
		}
		if !core.AppendArrayValues(shape.target, pending) {
			return false
		}
		pending = pending[:0]
		return true
	}
	fallbackWithPending := func(start int) (*object.EmeraldValue, bool) {
		if typedHashArrayBatchAppendEnabled && !flushPending() {
			cleanup()
			return nil, false
		}
		return fallback(start)
	}
	var rawArgs [2]int64
	for index, key := range keys {
		if object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		keyInteger, keyOK := typedSSAExactIntegerValueForClass(key, shape.integerClass)
		var valueInteger int64
		var valueOK bool
		if linearRegionOK && index < linearRegion.Count {
			if keyOK {
				valueInteger, valueOK = checkedIntegerAdd(keyInteger, linearRegion.ValueOffset)
			}
		} else {
			value, exists := hash[key]
			if !exists {
				continue
			}
			valueInteger, valueOK = typedSSAExactIntegerValueForClass(value, shape.integerClass)
		}
		if !keyOK || !valueOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		rawArgs[0], rawArgs[1] = keyInteger, valueInteger
		var result typedSSAValue
		var executed bool
		if shape.kernel.valid {
			product, productOK := checkedIntegerMul(rawArgs[shape.kernel.multiplyArg], shape.kernel.multiplier)
			if productOK {
				total, totalOK := checkedIntegerAdd(product, rawArgs[shape.kernel.addArg])
				if totalOK {
					if typedIntegerStringBatchEnabled && vm.typedStringValueBatch != nil {
						boxed := vm.typedStringValueBatch.NewInteger(total)
						if boxed == nil {
							if index == 0 {
								cleanup()
								return nil, false
							}
							return fallbackWithPending(index)
						}
						if typedHashArrayBatchAppendEnabled {
							pending = append(pending, boxed)
						} else if !core.AppendArrayValue(shape.target, boxed) {
							if index == 0 {
								cleanup()
								return nil, false
							}
							return fallbackWithPending(index)
						}
						continue
					}
					result = typedSSAValue{kind: typedSSAString, str: core.IntegerToSRawBuiltin(total)}
					executed = true
				}
			}
		} else {
			result, executed = vm.executeTypedSSAUnboxedArgsPlanTrusted(shape.calleePlan, shape.callee, rawArgs[:])
		}
		if !executed || object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		boxed := vm.typedSSAValueToObjectForVM(result)
		if boxed == nil {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		if typedHashArrayBatchAppendEnabled {
			pending = append(pending, boxed)
		} else if !core.AppendArrayValue(shape.target, boxed) {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
	}
	if !flushPending() {
		cleanup()
		return nil, false
	}
	core.LastBlockResult = nil
	cleanup()
	return receiver, true
}

// executeTypedHashDirectCall removes both the outer block IR interpreter and
// the nested Ruby frame for a callee whose direct fast plan is already a
// complete, side-effect-free proof. The only observable mutation in the
// admitted graph is the exact Array#<< target. A miss before the first append
// returns false; a later miss replays only the unexecuted Hash suffix through
// the ordinary callback ABI.
func (vm *VM) executeTypedHashDirectCall(shape typedHashCallShape, receiver, block *object.EmeraldValue, keys []*object.EmeraldValue, hash map[*object.EmeraldValue]*object.EmeraldValue, generation uint64) (*object.EmeraldValue, bool) {
	if vm == nil || shape.directPlan == nil || shape.callee == nil || receiver == nil || block == nil ||
		!typedHashDirectCallEnabled || !core.ArrayAppendUsesBuiltinImplementation() ||
		!core.HashEachUsesBuiltinImplementation() || generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || !typedHashCallReceiverStable(vm, shape, closure) {
		return nil, false
	}
	previousBlock := vm.currentBlock
	previousClassStack := vm.classStack
	previousStringBatch := vm.typedStringValueBatch
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	batchByteCapacity := len(keys) * 16
	if shape.directPlan != nil {
		prefix := core.DynamicInstanceVar(shape.helper, shape.directPlan.directFastIntegerStringPrefixIvar)
		if prefix != nil && prefix.Type == object.ValueString {
			if rawPrefix, ok := prefix.Data.(string); ok {
				perValue := len(rawPrefix) + 8
				if perValue > 0 && len(keys) <= int(^uint(0)>>1)/perValue {
					batchByteCapacity = len(keys) * perValue
				}
			}
		}
	}
	vm.typedStringValueBatch = core.NewStringValueBatchWithByteCapacity(len(keys), batchByteCapacity)
	cleanup := func() {
		vm.currentBlock = previousBlock
		vm.classStack = previousClassStack
		vm.typedStringValueBatch = previousStringBatch
	}
	fallback := func(start int) (*object.EmeraldValue, bool) {
		cleanup()
		if hash == nil {
			hash = core.DirectHashMaterialize(receiver)
		}
		for index := start; index < len(keys); index++ {
			key := keys[index]
			value, exists := hash[key]
			if !exists {
				continue
			}
			core.LastBlockResult = nil
			core.ForEachClearNext()
			var result *object.EmeraldValue
			if core.CallBlockWithArgs != nil {
				result = core.CallBlockWithArgs(block, key, value)
			} else if core.CallBlock != nil {
				result = core.CallBlock(key, value)
			}
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
			if core.LastBlockResult != nil {
				control := core.LastBlockResult
				core.LastBlockResult = nil
				return control, true
			}
			if core.ForEachConsumeNext() {
				continue
			}
		}
		core.LastBlockResult = nil
		return receiver, true
	}
	if result, executed := vm.executeTypedHashIntegerStringBatch(shape, receiver, keys, hash, generation, cleanup, fallback); executed {
		return result, true
	}
	if hash == nil {
		hash = core.DirectHashMaterialize(receiver)
	}

	var args [2]*object.EmeraldValue
	for index, key := range keys {
		if object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		value, exists := hash[key]
		if !exists {
			continue
		}
		args[0], args[1] = key, value
		result, executed := vm.executeRegisterIRDirectFast(shape.directPlan, shape.callee, shape.helper, args[:], generation)
		if !executed || result == nil {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		if result.Type == object.ValueException {
			cleanup()
			return result, true
		}
		if !core.AppendArrayValue(shape.target, result) {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
	}
	core.LastBlockResult = nil
	cleanup()
	return receiver, true
}

// executeTypedHashIntegerStringBatch is the hoisted steady-state form of the
// direct integer/string callee. It checks generation, builtin methods, the
// helper prefix and result encoding once, then keeps the per-entry loop to a
// Hash lookup, checked int64 arithmetic and one batched String/Array append.
// A non-ASCII or otherwise incompatible prefix returns false before mutation
// so the caller can use the more general direct executor instead.
func (vm *VM) executeTypedHashIntegerStringBatch(shape typedHashCallShape, receiver *object.EmeraldValue, keys []*object.EmeraldValue, hash map[*object.EmeraldValue]*object.EmeraldValue, generation uint64, cleanup func(), fallback func(int) (*object.EmeraldValue, bool)) (*object.EmeraldValue, bool) {
	plan := shape.directPlan
	if vm == nil || plan == nil || plan.directFastKind != registerIRDirectFastIntegerStringConcat ||
		generation != object.CurrentMethodGeneration() || !vm.fusedIntegerOperationAvailable(compiler.OpGreaterThan) ||
		!vm.fusedIntegerOperationAvailable(compiler.OpMul) || !vm.fusedIntegerOperationAvailable(compiler.OpAdd) ||
		!vm.fusedIntegerToSAvailable() || !vm.fusedStringOperationAvailable(compiler.OpAdd) {
		return nil, false
	}
	prefix := core.DynamicInstanceVar(shape.helper, plan.directFastIntegerStringPrefixIvar)
	if prefix == nil || prefix.Type != object.ValueString || prefix.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(prefix) != nil {
		return nil, false
	}
	prefixRaw, ok := prefix.Data.(string)
	if !ok || prefix.Encoding != "" && !strings.EqualFold(prefix.Encoding, core.DefaultExternalEncoding()) || stringHasNonASCIIByteForVM(prefixRaw) {
		return nil, false
	}
	batch := vm.typedStringValueBatch
	if batch == nil {
		return nil, false
	}
	linearRegion, linearRegionOK := core.DirectHashLinearRegion(receiver)
	if hash == nil && !linearRegionOK {
		return nil, false
	}
	linearKey := int64(0)
	linearValue := int64(0)
	if linearRegionOK && hash == nil {
		if linearRegion.Count != len(keys) || len(keys) == 0 {
			return nil, false
		}
		linearKey = linearRegion.Start
		var valid bool
		linearValue, valid = checkedIntegerAdd(linearKey, linearRegion.ValueOffset)
		if !valid {
			return nil, false
		}
		lastKey, lastKeyOK := registerIRDirectFastExactInteger(keys[len(keys)-1])
		if !lastKeyOK {
			return nil, false
		}
		if _, valid = checkedIntegerAdd(lastKey, linearRegion.ValueOffset); !valid {
			return nil, false
		}
	}
	var pending []*object.EmeraldValue
	if typedHashArrayBatchAppendEnabled {
		pending = make([]*object.EmeraldValue, 0, len(keys))
	}
	flushPending := func() bool {
		if !typedHashArrayBatchAppendEnabled {
			return true
		}
		if len(pending) == 0 {
			return true
		}
		if !core.AppendArrayValues(shape.target, pending) {
			return false
		}
		pending = pending[:0]
		return true
	}
	fallbackWithPending := func(start int) (*object.EmeraldValue, bool) {
		if !typedHashArrayBatchAppendEnabled {
			return fallback(start)
		}
		if !flushPending() {
			cleanup()
			return nil, false
		}
		return fallback(start)
	}
	for index, key := range keys {
		if object.CurrentMethodGeneration() != generation {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		var valueInteger int64
		var valueOK bool
		var keyInteger int64
		var keyOK bool
		if linearRegionOK && hash == nil {
			// DirectHashLinearRegion proves ordered, contiguous affine keys and
			// values while its pointer map is still absent. Reuse the raw affine
			// counters instead of decoding/boxing each key from hash.Keys.
			keyInteger, keyOK = linearKey, true
			valueInteger, valueOK = linearValue, true
			linearKey += linearRegion.Step
			linearValue += linearRegion.Step
		} else {
			keyInteger, keyOK = registerIRDirectFastExactInteger(key)
			value, exists := hash[key]
			if !exists {
				continue
			}
			valueInteger, valueOK = registerIRDirectFastExactInteger(value)
		}
		if !valueOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		if valueInteger <= plan.directFastIntegerStringThreshold {
			result, executed := vm.executeRegisterIRDirectIntegerStringFallback(plan, shape.callee)
			if !executed || result == nil {
				if index == 0 {
					cleanup()
					return nil, false
				}
				return fallbackWithPending(index)
			}
			if typedHashArrayBatchAppendEnabled {
				pending = append(pending, result)
			} else if !core.AppendArrayValue(shape.target, result) {
				if index == 0 {
					cleanup()
					return nil, false
				}
				return fallbackWithPending(index)
			}
			continue
		}
		if !keyOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		product, productOK := checkedIntegerMul(keyInteger, plan.directFastIntegerStringMultiplier)
		total, totalOK := checkedIntegerAdd(product, valueInteger)
		if !productOK || !totalOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		result := batch.NewPrefixInteger(prefixRaw, total)
		if result == nil {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
		if typedHashArrayBatchAppendEnabled {
			pending = append(pending, result)
		} else if !core.AppendArrayValue(shape.target, result) {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallbackWithPending(index)
		}
	}
	if !flushPending() {
		cleanup()
		return nil, false
	}
	core.LastBlockResult = nil
	cleanup()
	return receiver, true
}

func stringHasNonASCIIByteForVM(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= 0x80 {
			return true
		}
	}
	return false
}
