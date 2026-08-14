package vm

import (
	"fmt"
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedSSABatchCallEnabled enables the first reusable typed call-graph edge.
// It is intentionally narrower than the ordinary direct Register IR tier:
// the block contains exactly one Ruby send, and the callee must be a pure
// typed-SSA function.  The block itself therefore has no user-visible side
// effect and can be preflighted for every array element before Array#map/each
// commits an observable result.
var typedSSABatchCallEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_BATCH_CALL") == ""
var typedSSABatchProfileEnabled = os.Getenv("RGO_PROFILE_TYPED_SSA_BATCH") != ""
var typedSSACompactGetterEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_COMPACT_GETTER") == ""
var typedSSAStringMapLazyResultEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_STRING_MAP_LAZY_RESULT") == ""
var typedSSAIntegerMapLazyResultEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_INTEGER_MAP_LAZY_RESULT") == ""
var typedSSARepeatedValueLazyResultEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_REPEATED_VALUE_LAZY_RESULT") == ""
var typedSSARescueStringMapLazyResultEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_RESCUE_STRING_MAP_LAZY_RESULT") == ""

// A typed call graph has a small per-batch proof cost.  Keep it away from
// short object-heavy arrays where the existing single-call leaf path is
// cheaper; long arrays amortize the proof and avoid one frame/send protocol
// per element.
const typedSSABatchCallMinElements = 1024

// The field reducer has a slightly higher proof cost than the existing
// framed Array#each tier. Keep it for genuinely hot regions so short loops do
// not pay a speculative shape walk that cannot amortize.
const typedSSAFieldReduceMinElements = 1024

type typedSSABatchCallShape struct {
	plan         *registerIRPlan
	fn           *object.Function
	closure      *object.Closure
	send         *registerIRInstruction
	self         *object.EmeraldValue
	free         []*object.EmeraldValue
	paramLocal   int
	implicitSend bool
	// directParam is the common `|value| value.method` shape. Once the callee
	// is proven on the first element, later iterations can pass the element
	// directly instead of replaying the three Register IR prologue operations.
	directParam bool
	// directSelfParam is the common `|value| helper(value)` shape. The block
	// prologue is exactly self + parameter + send + block return, so the batch
	// can pass the element directly without rebuilding a temporary register
	// file on every callback.
	directSelfParam bool
	// directFreeParam is the captured-object counterpart, e.g.
	// `|value| classifier.classify(value)`. The free receiver is immutable for
	// this pure four-instruction block, so resolve it once per batch.
	directFreeParam bool
	directFreeIndex uint8
}

type typedSSABatchCallTemplate struct {
	plan         *registerIRPlan
	fn           *object.Function
	send         *registerIRInstruction
	paramLocal   int
	implicitSend bool
}

func typedSSAConstantReturnObject(plan *typedSSAPlan) (*object.EmeraldValue, bool) {
	if plan == nil || len(plan.ops) != 2 {
		return nil, false
	}
	load, ret := plan.ops[0], plan.ops[1]
	if load.kind != typedSSAOpLoadLiteral || ret.kind != typedSSAOpReturn || ret.left != load.dst {
		return nil, false
	}
	switch load.literal.kind {
	case typedSSAInteger, typedSSABool, typedSSANil:
		return typedSSAValueToObject(load.literal), true
	default:
		// Float and String results may require a fresh object per invocation;
		// keep those on the ordinary typed executor until their allocation and
		// identity rules have a dedicated batch proof.
		return nil, false
	}
}

// typedSSAPlanCanUseUnboxedStringResult recognizes a pure primitive graph
// whose changing input is an Integer and whose result may be a String. The
// plan compiler already records literal String use; the explicit to_s probe
// covers the common `value.to_s` result even when the graph has no String
// literal. Reference/Float graphs stay on their existing identity-preserving
// path.
func typedSSAPlanCanUseUnboxedStringResult(plan *typedSSAPlan) bool {
	if plan == nil || plan.hasReference || plan.hasFloat {
		return false
	}
	if plan.hasString {
		return true
	}
	for _, instruction := range plan.ops {
		if instruction.kind == typedSSAOpCall && instruction.name == "to_s" && instruction.argc == 0 && !instruction.implicit {
			return true
		}
	}
	return false
}

func typedSSAPlanASCIIStringLiterals(plan *typedSSAPlan) bool {
	if plan == nil {
		return false
	}
	for _, operation := range plan.ops {
		if operation.kind != typedSSAOpLoadLiteral || operation.literal.kind != typedSSAString {
			continue
		}
		for index := 0; index < len(operation.literal.str); index++ {
			if operation.literal.str[index] >= 0x80 {
				return false
			}
		}
	}
	return true
}

func typedSSAPlanHasStringPlus(plan *typedSSAPlan) bool {
	if plan == nil {
		return false
	}
	for _, operation := range plan.ops {
		if operation.kind == typedSSAOpCall && operation.name == "+" && operation.argc == 1 {
			return true
		}
		if operation.kind == typedSSAOpBinary && operation.opcode == compiler.OpAdd {
			return true
		}
	}
	return false
}

// prepareTypedSSABatchCallShape recognizes a block with one ordinary send and
// no other operation that can mutate state or observe a Ruby Frame.  The
// result is a small call graph edge; the block's caller supplies the changing
// parameter for each array element.
func (vm *VM) prepareTypedSSABatchCallShape(block *object.EmeraldValue) (typedSSABatchCallShape, bool) {
	invalid := typedSSABatchCallShape{}
	if vm == nil || !typedSSABatchCallEnabled || block == nil || block.Type != object.ValueClosure ||
		!registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() {
		return invalid, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil ||
		closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) {
		return invalid, false
	}
	fn := closure.Fn
	if closure.AutoSplat && blockWantsDestructuring(fn) {
		return invalid, false
	}
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" ||
		fn.KeywordRestOnly || !simpleBlockParameterPatterns(fn) {
		return invalid, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}
	if vm.typedSSABatchCallTemplates == nil {
		vm.typedSSABatchCallTemplates = make(map[*object.Function]typedSSABatchCallTemplate)
	}
	if template, found := vm.typedSSABatchCallTemplates[fn]; found {
		return typedSSABatchCallShape{
			plan: template.plan, fn: template.fn, closure: closure,
			send: template.send, self: blockBindingSelf(block),
			free: closure.Free, paramLocal: template.paramLocal,
			implicitSend: template.implicitSend,
			directParam: template.send.argc == 0 && len(template.plan.instructions) == 3 &&
				template.plan.instructions[0].op == registerIRLoadParam && template.plan.instructions[0].param == 0 &&
				template.plan.instructions[0].dst == template.send.left &&
				template.plan.instructions[2].op == registerIRReturn && template.plan.instructions[2].left == template.send.dst,
			directSelfParam: typedSSABatchDirectSelfParam(template.plan, template.send),
			directFreeParam: typedSSABatchDirectFreeParam(template.plan, template.send),
			directFreeIndex: typedSSABatchDirectFreeIndex(template.plan, template.send),
		}, true
	}
	plan, found := vm.cachedBlockLeafPlan(fn)
	if !found || plan.kind != leafMethodRegisterIR || plan.registerIR == nil ||
		!plan.registerIR.blockReturn || plan.registerIR.hasBranches ||
		plan.registerIR.sendCount != 1 ||
		closure.ReturnOwnerID > 0 && plan.registerIR.hasExplicitReturn {
		return invalid, false
	}
	var send *registerIRInstruction
	for index := range plan.registerIR.instructions {
		instruction := &plan.registerIR.instructions[index]
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return invalid, false
			}
		case registerIRLoadLocal:
			if int(instruction.param) != fn.ParamLocalIndices[0] {
				return invalid, false
			}
		case registerIRStoreLocal:
			if int(instruction.param) != fn.ParamLocalIndices[0] {
				return invalid, false
			}
		case registerIRLoadLiteral, registerIRLoadSelf, registerIRLoadFree,
			registerIRMove, registerIRReturn:
		case registerIRSend:
			if send != nil || instruction.opcode != compiler.OpSend || instruction.blockPresent ||
				instruction.splatIndex != 255 || instruction.argc > 4 || instruction.name == "" {
				return invalid, false
			}
			send = instruction
		default:
			return invalid, false
		}
	}
	if send == nil {
		return invalid, false
	}
	implicitSend := typedSSABatchImplicitSend(fn, send)
	if plan.registerIR.hasImplicitSends && !implicitSend {
		return invalid, false
	}
	self := blockBindingSelf(block)
	if self == nil {
		return invalid, false
	}
	template := typedSSABatchCallTemplate{
		plan: plan.registerIR, fn: fn, send: send, paramLocal: fn.ParamLocalIndices[0], implicitSend: implicitSend,
	}
	vm.typedSSABatchCallTemplates[fn] = template
	return typedSSABatchCallShape{
		plan: plan.registerIR, fn: fn, closure: closure, send: send,
		self: self, free: closure.Free, paramLocal: fn.ParamLocalIndices[0],
		implicitSend: implicitSend,
		directParam: send.argc == 0 && len(plan.registerIR.instructions) == 3 &&
			plan.registerIR.instructions[0].op == registerIRLoadParam && plan.registerIR.instructions[0].param == 0 &&
			plan.registerIR.instructions[0].dst == send.left &&
			plan.registerIR.instructions[2].op == registerIRReturn && plan.registerIR.instructions[2].left == send.dst,
		directSelfParam: typedSSABatchDirectSelfParam(plan.registerIR, send),
		directFreeParam: typedSSABatchDirectFreeParam(plan.registerIR, send),
		directFreeIndex: typedSSABatchDirectFreeIndex(plan.registerIR, send),
	}, true
}

func typedSSABatchDirectSelfParam(plan *registerIRPlan, send *registerIRInstruction) bool {
	if plan == nil || send == nil || send.argc != 1 || len(plan.instructions) != 4 {
		return false
	}
	loadSelf, loadParam, bodySend, blockReturn := plan.instructions[0], plan.instructions[1], plan.instructions[2], plan.instructions[3]
	return loadSelf.op == registerIRLoadSelf && loadSelf.dst == send.left &&
		loadParam.op == registerIRLoadParam && loadParam.param == 0 && loadParam.dst == send.args[0] &&
		bodySend.op == registerIRSend && bodySend == *send &&
		blockReturn.op == registerIRReturn && blockReturn.left == send.dst
}

func typedSSABatchDirectFreeParam(plan *registerIRPlan, send *registerIRInstruction) bool {
	if plan == nil || send == nil || send.argc != 1 || len(plan.instructions) != 4 {
		return false
	}
	loadFree, loadParam, bodySend, blockReturn := plan.instructions[0], plan.instructions[1], plan.instructions[2], plan.instructions[3]
	return loadFree.op == registerIRLoadFree && loadFree.dst == send.left &&
		loadParam.op == registerIRLoadParam && loadParam.param == 0 && loadParam.dst == send.args[0] &&
		bodySend.op == registerIRSend && bodySend == *send &&
		blockReturn.op == registerIRReturn && blockReturn.left == send.dst
}

func typedSSABatchDirectFreeIndex(plan *registerIRPlan, send *registerIRInstruction) uint8 {
	if !typedSSABatchDirectFreeParam(plan, send) {
		return 255
	}
	return plan.instructions[0].param
}

// typedSSARescueIntegerStringShape recognizes the success path of a narrow
// helper such as:
//
//	begin
//	  value > 3 ? value.to_s : ""
//	rescue
//	  "fallback"
//	end
//
// Register IR intentionally keeps rescue methods on the framed path.  This
// parser does not weaken that general rule: it admits only the compiler's
// exact bytecode layout, and callers still require exact Integer inputs and
// generation-scoped builtin guards before executing the raw branch.
func typedSSARescueIntegerStringShape(fn *object.Function) (typedSSAPrimitiveIntegerStringKernel, bool) {
	if fn == nil || len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || len(fn.ParamDefaults) != 1 || fn.ParamDefaults[0] != nil ||
		fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.FreezeStringLiterals || len(fn.Instructions) < 20 {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	instructions := fn.Instructions
	if compiler.Opcode(instructions[0]) != compiler.OpBeginRescue || len(instructions) < 9 {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	rescueStart, rescueOK := typedSSABytecodeWord(instructions, 1)
	endRescue, endRescueOK := typedSSABytecodeWord(instructions, 3)
	end, endOK := typedSSABytecodeWord(instructions, 5)
	ensureStart, ensureOK := typedSSABytecodeWord(instructions, 7)
	bodyStart := 9
	if !rescueOK || !endRescueOK || !endOK || !ensureOK || ensureStart != 0 || rescueStart <= bodyStart ||
		rescueStart >= endRescue || endRescue >= end || end != len(instructions)-1 {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}

	position := bodyStart
	var ok bool
	position, ok = typedSSABytecodeLocalLoad(instructions, position, fn.ParamLocalIndices[0])
	if !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	compareValue, position, ok := typedSSABytecodeConstant(fn, instructions, position)
	if !ok || !smallIntegerValue(compareValue) {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	compare := compiler.Opcode(instructions[position])
	switch compare {
	case compiler.OpLessThan, compiler.OpLessThanOrEqual, compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual:
	default:
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position++
	falseTarget, next, ok := typedSSABytecodeJump(instructions, position, compiler.OpJumpNotTruthy)
	if !ok || falseTarget <= next || falseTarget >= rescueStart {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position = next
	position, ok = typedSSABytecodeLocalLoad(instructions, position, fn.ParamLocalIndices[0])
	if !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	if _, position, ok = leafSendNameArity(fn, position, "to_s", 0); !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	trueTarget, next, ok := typedSSABytecodeJump(instructions, position, compiler.OpJump)
	if !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position = next
	falseLiteral, position, ok := typedSSABytecodeConstant(fn, instructions, falseTarget)
	if !ok || falseLiteral == nil || falseLiteral.Type != object.ValueString {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	falseString, stringOK := falseLiteral.Data.(string)
	if !stringOK || falseLiteral.Frozen || !typedSSAASCIIString(falseString) || falseLiteral.Encoding != "" && falseLiteral.Encoding != "UTF-8" {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	falseJumpTarget, falseJumpEnd, ok := typedSSABytecodeJump(instructions, position, compiler.OpJump)
	if !ok || falseJumpTarget != end || falseJumpEnd != rescueStart || trueTarget != position {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}

	position = rescueStart
	if position+3 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpRescueMatch ||
		instructions[position+1] != 0 || instructions[position+2] != 0 || instructions[position+3] != 0 {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position += 4
	reraiseTarget, next, ok := typedSSABytecodeJump(instructions, position, compiler.OpJumpNotTruthy)
	if !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position = next
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpRescue {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position++
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpPop {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position++
	fallback, position, ok := typedSSABytecodeConstant(fn, instructions, position)
	if !ok || fallback == nil || fallback.Type != object.ValueString {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	fallbackString, fallbackOK := fallback.Data.(string)
	if !fallbackOK || fallback.Frozen || !typedSSAASCIIString(fallbackString) || fallback.Encoding != "" && fallback.Encoding != "UTF-8" {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	rescueJumpTarget, next, ok := typedSSABytecodeJump(instructions, position, compiler.OpJump)
	if !ok {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position = next
	if position != reraiseTarget || position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpReraise {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position++
	if rescueJumpTarget != endRescue || position != endRescue || compiler.Opcode(instructions[position]) != compiler.OpEndRescue {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}
	position++
	if position != end || compiler.Opcode(instructions[position]) != compiler.OpReturnValue {
		return typedSSAPrimitiveIntegerStringKernel{}, false
	}

	return typedSSAPrimitiveIntegerStringKernel{
		kind:         typedSSAPrimitiveIntegerStringKernelCompareToS,
		compare:      compare,
		compareValue: compareValue.Data.(int64),
		falseString:  falseString,
	}, true
}

type typedRescueIntegerStringLazyPayload struct {
	inputs      []int64
	length      int
	kernel      typedSSAPrimitiveIntegerStringKernel
	values      []*object.EmeraldValue
	cachedIndex int
	cachedValue *object.EmeraldValue
}

func (payload *typedRescueIntegerStringLazyPayload) lazyLength() int {
	if payload == nil {
		return 0
	}
	return payload.length
}

func (payload *typedRescueIntegerStringLazyPayload) stringLengthAt(index int) (int64, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return 0, false
	}
	argument := payload.inputs[index]
	condition := false
	switch payload.kernel.compare {
	case compiler.OpLessThan:
		condition = argument < payload.kernel.compareValue
	case compiler.OpLessThanOrEqual:
		condition = argument <= payload.kernel.compareValue
	case compiler.OpGreaterThan:
		condition = argument > payload.kernel.compareValue
	case compiler.OpGreaterThanOrEqual:
		condition = argument >= payload.kernel.compareValue
	default:
		return 0, false
	}
	if condition {
		return int64(core.IntegerToSLengthRawBuiltin(argument)), true
	}
	if !typedSSAASCIIString(payload.kernel.falseString) {
		return 0, false
	}
	return int64(len(payload.kernel.falseString)), true
}

func (payload *typedRescueIntegerStringLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	if payload.cachedIndex == index && payload.cachedValue != nil {
		return payload.cachedValue, true
	}
	if payload.values != nil {
		if value := payload.values[index]; value != nil {
			return value, true
		}
	}
	argument := payload.inputs[index]
	condition := false
	switch payload.kernel.compare {
	case compiler.OpLessThan:
		condition = argument < payload.kernel.compareValue
	case compiler.OpLessThanOrEqual:
		condition = argument <= payload.kernel.compareValue
	case compiler.OpGreaterThan:
		condition = argument > payload.kernel.compareValue
	case compiler.OpGreaterThanOrEqual:
		condition = argument >= payload.kernel.compareValue
	default:
		return nil, false
	}
	text := payload.kernel.falseString
	if condition {
		text = core.IntegerToSRawBuiltin(argument)
	}
	value := core.NewStringValue(text)
	if payload.values == nil {
		payload.cachedIndex = index
		payload.cachedValue = value
	} else {
		payload.values[index] = value
	}
	return value, value != nil
}

func (payload *typedRescueIntegerStringLazyPayload) valueAtBatch(index int, batch *core.StringValueBatch) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	argument := payload.inputs[index]
	condition := false
	switch payload.kernel.compare {
	case compiler.OpLessThan:
		condition = argument < payload.kernel.compareValue
	case compiler.OpLessThanOrEqual:
		condition = argument <= payload.kernel.compareValue
	case compiler.OpGreaterThan:
		condition = argument > payload.kernel.compareValue
	case compiler.OpGreaterThanOrEqual:
		condition = argument >= payload.kernel.compareValue
	default:
		return nil, false
	}
	if batch == nil {
		if condition {
			return core.NewStringValue(core.IntegerToSRawBuiltin(argument)), true
		}
		return core.NewStringValue(payload.kernel.falseString), true
	}
	if condition {
		return batch.NewInteger(argument), true
	}
	return batch.New(payload.kernel.falseString), true
}

func (payload *typedRescueIntegerStringLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, payload.length)
	}
	if payload.cachedValue != nil && payload.cachedIndex >= 0 && payload.cachedIndex < payload.length {
		payload.values[payload.cachedIndex] = payload.cachedValue
	}
	batch := core.NewStringValueBatch(payload.length)
	for index := range payload.values {
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

func (payload *typedRescueIntegerStringLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

func typedSSABytecodeWord(instructions []byte, position int) (int, bool) {
	if position < 0 || position+1 >= len(instructions) {
		return 0, false
	}
	return int(instructions[position])<<8 | int(instructions[position+1]), true
}

func typedSSABytecodeLocalLoad(instructions []byte, position, local int) (int, bool) {
	if position < 0 || position+1 >= len(instructions) ||
		compiler.Opcode(instructions[position]) != compiler.OpGetLocal && compiler.Opcode(instructions[position]) != compiler.OpGetLocalFast ||
		int(instructions[position+1]) != local {
		return position, false
	}
	return position + 2, true
}

func typedSSABytecodeConstant(fn *object.Function, instructions []byte, position int) (*object.EmeraldValue, int, bool) {
	if fn == nil || position < 0 || position+2 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpConstant {
		return nil, position, false
	}
	index := int(instructions[position+1])<<8 | int(instructions[position+2])
	if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
		return nil, position, false
	}
	return fn.Constants[index], position + 3, true
}

func typedSSABytecodeJump(instructions []byte, position int, opcode compiler.Opcode) (int, int, bool) {
	if position < 0 || position+2 >= len(instructions) || compiler.Opcode(instructions[position]) != opcode {
		return 0, position, false
	}
	target := int(instructions[position+1])<<8 | int(instructions[position+2])
	return target, position + 3, true
}

// tryExecuteArrayTypedSSARescueIntegerStringBatch handles the pure success
// path of a framed rescue helper after a complete Integer-input preflight.
// Since the helper contains no visible mutation, a miss can replay the whole
// callback through Array's ordinary path without exposing a partial result.
func (vm *VM) tryExecuteArrayTypedSSARescueIntegerStringBatch(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || block == nil || block.Type != object.ValueClosure ||
		!typedSSABatchCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSABatchCallMinElements {
		return nil, false
	}
	shape, ok := vm.prepareTypedSSABatchCallShape(block)
	if !ok || shape.send == nil || shape.send.argc != 1 || !shape.directSelfParam && !shape.directFreeParam {
		return nil, false
	}
	calleeReceiver := shape.self
	if shape.directFreeParam {
		if int(shape.directFreeIndex) >= len(shape.free) || shape.free[shape.directFreeIndex] == nil {
			return nil, false
		}
		calleeReceiver = derefClosureValue(shape.free[shape.directFreeIndex])
	}
	if calleeReceiver == nil || calleeReceiver.Class == nil || !vm.registerIRCacheableReceiverForMethod(calleeReceiver, shape.send.name) {
		return nil, false
	}
	if shape.directFreeParam && derefClosureValue(shape.free[shape.directFreeIndex]) != calleeReceiver {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(calleeReceiver, shape.send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords || methodUsesRefinements(method) {
		return nil, false
	}
	calleeFn, ok := method.Fn.(*object.Function)
	if !ok || calleeFn == nil {
		return nil, false
	}
	kernel, ok := typedSSARescueIntegerStringShape(calleeFn)
	if !ok || !vm.fusedIntegerOperationAvailable(kernel.compare) || !vm.fusedIntegerToSAvailable() {
		return nil, false
	}
	integerClass := core.R.Classes["Integer"]
	if integerClass == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	for _, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		if _, exact := typedSSAExactIntegerValueForClass(elem, integerClass); !exact {
			return nil, false
		}
	}
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	if collect && typedSSARescueStringMapLazyResultEnabled {
		inputs := make([]int64, len(elems))
		for index, elem := range elems {
			value, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
			if !exact {
				return nil, false
			}
			inputs[index] = value
		}
		payload := &typedRescueIntegerStringLazyPayload{
			inputs: inputs, length: len(inputs), kernel: kernel,
		}
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(inputs), Payload: payload, Materialize: payload.materialize,
			ElementAt: payload.elementAt, MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		core.LastBlockResult = nil
		return result, true
	}

	previousBatch := vm.typedStringValueBatch
	defer func() { vm.typedStringValueBatch = previousBatch }()
	if typedStringBatchEnabled && previousBatch == nil && len(elems) <= typedStringBatchMaxValues {
		vm.typedStringValueBatch = core.NewStringValueBatch(len(elems))
	}
	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, len(elems))
	}
	for index, elem := range elems {
		raw, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact {
			return nil, false
		}
		value, executed := vm.executeTypedSSAPrimitiveIntegerStringKernel(kernel, raw)
		if !executed {
			return nil, false
		}
		if collect {
			results[index] = value
		}
	}
	core.LastBlockResult = nil
	if !collect {
		return receiver, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

func (vm *VM) executeTypedSSABatchPrimitiveElement(calleeFn *object.Function, calleeIntegerPlan *integerFunctionPlan, calleePlan *typedSSAPlan, integerClass *object.Class, elem *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if elem == nil {
		elem = core.R.NilVal
	}
	if !typedIntegerArgumentClass(elem, integerClass) || core.AttachedSingletonClass(elem) != nil {
		return nil, false
	}
	return vm.executeTypedSSABatchPrimitiveInteger(calleeFn, calleeIntegerPlan, calleePlan, elem.Data.(int64))
}

func (vm *VM) executeTypedSSABatchPrimitiveInteger(calleeFn *object.Function, calleeIntegerPlan *integerFunctionPlan, calleePlan *typedSSAPlan, argumentValue int64) (*object.EmeraldValue, bool) {
	argument := [1]int64{argumentValue}
	if calleePlan != nil && calleePlan.integerKernel.kind == typedSSAIntegerKernelCompareBinary {
		integerResult, executed := executeTypedSSAIntegerKernel(calleePlan.integerKernel, argument[0])
		if !executed {
			return nil, false
		}
		return core.NewSmallIntegerValue(integerResult), true
	}
	var value typedSSAValue
	var executed bool
	if calleeIntegerPlan != nil {
		var integerResult int64
		integerResult, executed = vm.executeCachedIntegerFunctionPlan(calleeFn, calleeIntegerPlan, argument[:])
		value = typedSSAValue{kind: typedSSAInteger, int: integerResult}
	} else {
		value, executed = vm.executeTypedSSAUnboxedArgsPlanTrusted(calleePlan, calleeFn, argument[:])
	}
	if !executed {
		return nil, false
	}
	if typedSSABatchProfileEnabled {
		vm.typedSSABatchPrimitiveElements++
	}
	return typedSSAValueToObject(value), true
}

func typedSSABatchPrimitiveInputsSafe(elems []*object.EmeraldValue, integerClass *object.Class) bool {
	for _, elem := range elems {
		if !typedIntegerArgumentClass(elem, integerClass) || core.AttachedSingletonClass(elem) != nil {
			return false
		}
	}
	return true
}

func typedSSABatchImplicitSend(fn *object.Function, send *registerIRInstruction) bool {
	return fn != nil && send != nil && send.byteIP >= 0 && send.byteIP+3 < len(fn.Instructions) && fn.Instructions[send.byteIP+3] == 3
}

// typedSSABatchCallRegisters interprets only the pure block prologue around
// the one send.  It never invokes Ruby code; a false result is a clean
// preflight miss and the caller can replay the ordinary block protocol.
func typedSSABatchCallRegisters(shape typedSSABatchCallShape, elem *object.EmeraldValue) ([16]*object.EmeraldValue, bool) {
	var registers [16]*object.EmeraldValue
	var locals [64]*object.EmeraldValue
	if elem == nil {
		elem = core.R.NilVal
	}
	if shape.paramLocal < 0 || shape.paramLocal >= len(locals) {
		return registers, false
	}
	locals[shape.paramLocal] = elem
	for _, instruction := range shape.plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return registers, false
			}
			registers[instruction.dst] = elem
		case registerIRLoadLocal:
			if instruction.param >= uint8(len(locals)) {
				return registers, false
			}
			registers[instruction.dst] = locals[instruction.param]
		case registerIRStoreLocal:
			if instruction.param >= uint8(len(locals)) {
				return registers, false
			}
			locals[instruction.param] = registers[instruction.left]
		case registerIRLoadLiteral:
			registers[instruction.dst] = instruction.value
		case registerIRLoadSelf:
			registers[instruction.dst] = shape.self
		case registerIRLoadFree:
			if int(instruction.param) >= len(shape.free) {
				return registers, false
			}
			registers[instruction.dst] = derefClosureValue(shape.free[instruction.param])
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRSend, registerIRReturn:
			// The caller consumes these positions after the pure prologue.
		default:
			return registers, false
		}
	}
	return registers, true
}

// executeTypedSSASimpleGetterBatch is the narrowest object-call kernel. Once
// the callback is proven to be `|item| item.ivar_reader`, the generic batch
// loop's per-element method checks and typed-SSA register file are redundant.
// The proof is deliberately all-or-nothing: a heterogeneous receiver,
// singleton method, or unsupported object layout returns false before the
// caller exposes any mapped result, allowing the compatibility tier to replay
// the complete Array operation.
func (vm *VM) executeTypedSSASimpleGetterBatch(shape typedSSABatchCallShape, elems []*object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || !shape.directParam || shape.send == nil || shape.send.argc != 0 || len(elems) == 0 {
		return nil, false
	}
	first := elems[0]
	if first == nil || first.Type != object.ValueObject || first.Class == nil ||
		!vm.registerIRCacheableReceiverForMethod(first, shape.send.name) {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(first, shape.send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords ||
		methodUsesRefinements(method) {
		return nil, false
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil || len(fn.Params) != 0 || len(fn.ParamLocalIndices) != 0 || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.RejectKeywords || fn.RejectBlock || !simpleBlockParameterPatterns(fn) ||
		registerIRFunctionNeedsDefaultEvaluation(fn, 0) {
		return nil, false
	}
	plan, ok := vm.cachedTypedSSAPlan(fn)
	if !ok || plan == nil || plan.hasInstanceStore || plan.hasYield {
		return nil, false
	}
	getterIvar, ok := typedSSASimpleGetterIvar(plan)
	if !ok || getterIvar == "" {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	class := first.Class
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	getterSlot, _, getterSlotReady := typedSSACompactGetterSlot(first, getterIvar)
	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, 0, len(elems))
	}
	for _, elem := range elems {
		if elem == nil || elem.Type != object.ValueObject || elem.Class != class {
			return nil, false
		}
		obj, ok := elem.Data.(*object.Object)
		if !ok || obj == nil || len(obj.SingletonMethods) != 0 || obj.SingletonClass != nil {
			return nil, false
		}
		var value *object.EmeraldValue
		if getterSlotReady && obj.InstanceVars == nil {
			if obj.InlineInstanceVarMask&(uint8(1)<<getterSlot) != 0 {
				value = obj.InlineInstanceVars[getterSlot]
			}
		} else {
			if obj.InstanceVars != nil {
				value = obj.InstanceVars[getterIvar]
			} else {
				value = core.DynamicInstanceVar(elem, getterIvar)
			}
		}
		if value == nil {
			value = core.R.NilVal
		}
		if collect {
			results = append(results, value)
		}
	}
	core.LastBlockResult = nil
	if !collect {
		return &object.EmeraldValue{Type: object.ValueArray, Data: elems, Class: core.R.Classes["Array"]}, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

// typedSSAObjectIntegerToStringShape recognizes the first two-edge object
// graph outside the one-send batch: `|item| item.value.to_s`. The outer
// callback is still pure, but the first send is a Ruby getter whose object
// layout must be proved separately before the second primitive edge can use
// a raw Integer.
func typedSSAObjectIntegerToStringShape(plan *typedSSAPlan, fn *object.Function) (string, bool) {
	return typedSSAObjectIntegerToStringShapeMode(plan, fn, false)
}

func typedSSAObjectIntegerToStringLengthShape(plan *typedSSAPlan, fn *object.Function) (string, bool) {
	return typedSSAObjectIntegerToStringShapeMode(plan, fn, true)
}

func typedSSAObjectIntegerToStringShapeMode(plan *typedSSAPlan, fn *object.Function, lengthResult bool) (string, bool) {
	if plan == nil || fn == nil || !plan.blockReturn || plan.hasYield || plan.hasInstanceStore ||
		len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || len(plan.ops) != 4+boolInt(lengthResult) {
		return "", false
	}
	load, getter, toS, ret := plan.ops[0], plan.ops[1], plan.ops[2], plan.ops[3]
	inputRegister := load.dst
	if load.kind == typedSSAOpLoadParam {
		if load.param != 0 {
			return "", false
		}
	} else if load.kind == typedSSAOpLoadLocal {
		if int(load.param) != fn.ParamLocalIndices[0] {
			return "", false
		}
	} else {
		return "", false
	}
	if getter.kind != typedSSAOpCall || getter.name == "" || getter.argc != 0 || getter.implicit || getter.left != inputRegister ||
		toS.kind != typedSSAOpCall || toS.name != "to_s" || toS.argc != 0 || toS.implicit || toS.left != getter.dst {
		return "", false
	}
	if lengthResult {
		length, returnOp := plan.ops[3], plan.ops[4]
		if length.kind != typedSSAOpCall || length.name != "length" || length.argc != 0 || length.implicit ||
			length.left != toS.dst || returnOp.kind != typedSSAOpReturn || returnOp.left != length.dst {
			return "", false
		}
	} else if ret.kind != typedSSAOpReturn || ret.left != toS.dst {
		return "", false
	}
	return getter.name, true
}

func typedSSAObjectStringConcatShape(plan *typedSSAPlan, fn *object.Function) (string, string, bool) {
	return typedSSAObjectStringConcatShapeMode(plan, fn, false)
}

func typedSSAObjectStringConcatLengthShape(plan *typedSSAPlan, fn *object.Function) (string, string, bool) {
	return typedSSAObjectStringConcatShapeMode(plan, fn, true)
}

func typedSSAObjectStringConcatShapeMode(plan *typedSSAPlan, fn *object.Function, lengthResult bool) (string, string, bool) {
	if plan == nil || fn == nil || !plan.blockReturn || plan.hasYield || plan.hasInstanceStore ||
		len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || len(plan.ops) != 5+boolInt(lengthResult) {
		return "", "", false
	}
	load, getter, literal, binary, ret := plan.ops[0], plan.ops[1], plan.ops[2], plan.ops[3], plan.ops[4]
	inputRegister := load.dst
	if load.kind == typedSSAOpLoadParam {
		if load.param != 0 {
			return "", "", false
		}
	} else if load.kind == typedSSAOpLoadLocal {
		if int(load.param) != fn.ParamLocalIndices[0] {
			return "", "", false
		}
	} else {
		return "", "", false
	}
	if getter.kind != typedSSAOpCall || getter.name == "" || getter.argc != 0 || getter.implicit || getter.left != inputRegister ||
		literal.kind != typedSSAOpLoadLiteral || literal.literal.kind != typedSSAString ||
		binary.kind != typedSSAOpBinary || binary.opcode != compiler.OpAdd || binary.left != getter.dst || binary.right != literal.dst {
		return "", "", false
	}
	if lengthResult {
		length, returnOp := plan.ops[4], plan.ops[5]
		if length.kind != typedSSAOpCall || length.name != "length" || length.argc != 0 || length.implicit ||
			length.left != binary.dst || returnOp.kind != typedSSAOpReturn || returnOp.left != length.dst {
			return "", "", false
		}
	} else if ret.kind != typedSSAOpReturn || ret.left != binary.dst {
		return "", "", false
	}
	return getter.name, literal.literal.str, true
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type typedSSAStringLengthShape struct {
	closure          *object.Closure
	fn               *object.Function
	plan             *registerIRPlan
	receiver         *object.EmeraldValue
	receiverFree     uint8
	receiverFromSelf bool
	calleeName       string
}

// prepareTypedSSAStringLengthShape recognizes the reusable outer graph
// `|value| helper.render(value).length`.  The callback itself has no visible
// effect; the only Ruby callee is proven separately as a raw Integer->String
// plan and the final String#length is an exact builtin query.  Keeping the
// shape at the call-graph level lets the region remove both the callback Frame
// and the temporary String object without depending on a Gem/class name.
func (vm *VM) prepareTypedSSAStringLengthShape(block *object.EmeraldValue) (typedSSAStringLengthShape, bool) {
	invalid := typedSSAStringLengthShape{receiverFree: 255}
	if vm == nil || block == nil || block.Type != object.ValueClosure || !typedSSABatchCallEnabled ||
		!registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() {
		return invalid, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || closure.AutoSplat && blockWantsDestructuring(closure.Fn) {
		return invalid, false
	}
	fn := closure.Fn
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
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
		outer.sendCount != 2 || len(outer.instructions) < 5 || len(outer.instructions) > 8 {
		return invalid, false
	}

	var parameterDst uint8
	parameterLoaded := false
	var receiverDst uint8
	receiverLoaded := false
	receiverFree := uint8(255)
	receiverFromSelf := false
	var firstSend, lengthSend, blockReturn *registerIRInstruction
	for index := range outer.instructions {
		instruction := &outer.instructions[index]
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 || parameterLoaded {
				return invalid, false
			}
			parameterLoaded = true
			parameterDst = instruction.dst
		case registerIRLoadLocal:
			if parameterLoaded || int(instruction.param) != fn.ParamLocalIndices[0] {
				return invalid, false
			}
			parameterLoaded = true
			parameterDst = instruction.dst
		case registerIRLoadSelf:
			if receiverLoaded {
				return invalid, false
			}
			receiverLoaded = true
			receiverDst = instruction.dst
			receiverFromSelf = true
		case registerIRLoadFree:
			if receiverLoaded || int(instruction.param) >= len(closure.Free) {
				return invalid, false
			}
			receiverLoaded = true
			receiverDst = instruction.dst
			receiverFree = instruction.param
		case registerIRSend:
			if instruction.opcode != compiler.OpSend || instruction.blockPresent || instruction.splatIndex != 255 {
				return invalid, false
			}
			if firstSend == nil {
				if instruction.argc != 1 {
					return invalid, false
				}
				firstSend = instruction
			} else if lengthSend == nil {
				if instruction.argc != 0 || instruction.name != "length" {
					return invalid, false
				}
				lengthSend = instruction
			} else {
				return invalid, false
			}
		case registerIRReturn:
			if blockReturn != nil {
				return invalid, false
			}
			blockReturn = instruction
		default:
			return invalid, false
		}
	}
	if !parameterLoaded || !receiverLoaded || firstSend == nil || lengthSend == nil || blockReturn == nil ||
		firstSend.left != receiverDst || firstSend.args[0] != parameterDst || lengthSend.left != firstSend.dst ||
		blockReturn.left != lengthSend.dst || (!receiverFromSelf && receiverFree == 255) {
		return invalid, false
	}
	receiver := blockBindingSelf(block)
	if receiverFree != 255 {
		receiver = derefClosureValue(closure.Free[receiverFree])
	}
	if receiver == nil || receiver.Class == nil || firstSend.name == "" {
		return invalid, false
	}
	return typedSSAStringLengthShape{
		closure: closure, fn: fn, plan: outer, receiver: receiver,
		receiverFree: receiverFree, receiverFromSelf: receiverFromSelf, calleeName: firstSend.name,
	}, true
}

// tryExecuteArrayTypedSSAStringLengthBatch is an escape-analysis boundary for
// a common Gem callback.  The callback result is immediately consumed by the
// builtin String#length, so a raw String payload is sufficient; no
// EmeraldValue/String header needs to be created for the intermediate value.
// A complete input preflight keeps a typed miss from exposing a partial map
// result or from replaying a callback with different method semantics.
func (vm *VM) tryExecuteArrayTypedSSAStringLengthBatch(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || block == nil ||
		block.Type != object.ValueClosure {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSABatchCallMinElements {
		return nil, false
	}
	shape, ok := vm.prepareTypedSSAStringLengthShape(block)
	if !ok || !core.StringLengthUsesBuiltinImplementation() || !vm.registerIRCacheableReceiverForMethod(shape.receiver, shape.calleeName) {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(shape.receiver, shape.calleeName, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords ||
		methodUsesRefinements(method) {
		return nil, false
	}
	callee, ok := method.Fn.(*object.Function)
	if !ok || callee == nil || len(callee.Params) != 1 || len(callee.ParamLocalIndices) != 1 ||
		callee.HasRestParam || callee.HasBlockParam || len(callee.KeywordParams) != 0 ||
		callee.KeywordRestParam != "" || callee.KeywordRestOnly || callee.RejectKeywords || callee.RejectBlock ||
		!simpleBlockParameterPatterns(callee) || registerIRFunctionNeedsDefaultEvaluation(callee, 1) {
		return nil, false
	}
	for _, defaultValue := range callee.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(callee)
	if !ok || calleePlan == nil || calleePlan.hasReference || calleePlan.hasFloat || calleePlan.hasYield ||
		calleePlan.hasInstanceStore || !typedSSAPlanCanUseUnboxedStringResult(calleePlan) ||
		!typedSSAPlanASCIIStringLiterals(calleePlan) || !vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) {
		return nil, false
	}
	if typedSSAPlanHasIntegerToS(calleePlan) && !core.IntegerToSUsesBuiltinImplementation() ||
		typedSSAPlanHasStringPlus(calleePlan) && !core.StringPlusUsesBuiltinImplementation() {
		return nil, false
	}
	// When the String result is consumed immediately by builtin length, the
	// intermediate concatenated String is unobservable.  The concat detector
	// already proves the graph is exactly `value.to_s + "literal"` (or the
	// prefix form); with ASCII literals, its character length is the decimal
	// Integer#to_s length plus the literal byte length.  Keep this as a local
	// batch kernel so the general typed-SSA interpreter remains the fallback for
	// Unicode, extra operations, and any future String semantics.
	concatLengthKernel := typedSSAIntegerStringConcatKernel{}
	concatLengthKernelOK := typedSSAIntegerStringConcatEnabled &&
		calleePlan.integerStringConcatKernel.kind != typedSSAIntegerStringConcatKernelNone &&
		typedSSAPlanASCIIStringLiterals(calleePlan) &&
		core.IntegerToSUsesBuiltinImplementation() && core.StringPlusUsesBuiltinImplementation()
	if concatLengthKernelOK {
		concatLengthKernel = calleePlan.integerStringConcatKernel
	}
	generation := object.CurrentMethodGeneration()
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	integerClass := core.R.IntegerClass
	lazyLengthResult := typedSSAStringMapLazyResultEnabled && collect && concatLengthKernelOK && !core.ObjectSpaceAllocationTracing()
	var inputs []int64
	if lazyLengthResult {
		inputs = make([]int64, len(elems))
	}
	var lengths []int64
	if !lazyLengthResult {
		lengths = make([]int64, len(elems))
	}
	for index, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		argument, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact {
			return nil, false
		}
		if concatLengthKernelOK {
			if lazyLengthResult {
				inputs[index] = argument
				continue
			}
			lengths[index] = int64(core.IntegerToSLengthRawBuiltin(argument) + len(concatLengthKernel.literal))
			continue
		}
		value, executed := vm.executeTypedSSAUnboxedArgsPlanTrusted(calleePlan, callee, []int64{argument})
		if !executed || value.kind != typedSSAString {
			return nil, false
		}
		lengths[index] = int64(len(value.str))
	}
	core.LastBlockResult = nil
	if !collect {
		return receiver, true
	}
	if lazyLengthResult {
		payload := &typedIntegerStringMapLazyPayload{
			inputs: inputs,
			kernel: concatLengthKernel, lengthResult: true,
		}
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(inputs), Payload: payload, Materialize: payload.materialize,
			ElementAt: payload.elementAt, MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		return result, true
	}
	results := make([]*object.EmeraldValue, len(lengths))
	for index, length := range lengths {
		results[index] = core.NewIntegerValue(length)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

// typedSSAStringTypeBranchShape recognizes the compiler's compact form for
// `item.is_a?(String) ? item.length : 0`. The batch kernel below deliberately
// admits only the all-exact-String case: that is enough to remove the send and
// branch overhead from the common homogeneous collection without guessing
// about String subclasses or redefined predicate methods in mixed arrays.
func typedSSAStringTypeBranchShape(plan *registerIRPlan) bool {
	if plan == nil || !plan.blockReturn || !plan.hasBranches || plan.sendCount != 2 || len(plan.instructions) != 9 {
		return false
	}
	instructions := plan.instructions
	parameter := instructions[0]
	stringClass := instructions[1]
	predicate := instructions[2]
	branch := instructions[3]
	trueParameter := instructions[4]
	length := instructions[5]
	join := instructions[6]
	falseValue := instructions[7]
	result := instructions[8]
	return parameter.op == registerIRLoadParam && parameter.param == 0 &&
		stringClass.op == registerIRLoadConstant && stringClass.name == "String" &&
		predicate.op == registerIRSend && predicate.opcode == compiler.OpSend && predicate.name == "is_a?" &&
		predicate.argc == 1 && !predicate.blockPresent && predicate.splatIndex == 255 &&
		predicate.left == parameter.dst && predicate.args[0] == stringClass.dst &&
		branch.op == registerIRJumpNotTruthy && branch.left == predicate.dst && branch.target == 7 &&
		trueParameter.op == registerIRLoadParam && trueParameter.param == 0 &&
		length.op == registerIRSend && length.opcode == compiler.OpSend && length.name == "length" &&
		length.argc == 0 && !length.blockPresent && length.splatIndex == 255 && length.left == trueParameter.dst &&
		join.op == registerIRJump && join.target == 8 &&
		falseValue.op == registerIRLoadLiteral && smallIntegerValue(falseValue.value) && falseValue.value.Data.(int64) == 0 &&
		falseValue.dst == length.dst && result.op == registerIRReturn && result.left == length.dst
}

func (vm *VM) tryExecuteArrayTypedSSAStringTypeBranchBatch(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || block == nil || block.Type != object.ValueClosure ||
		!typedSSABatchCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSABatchCallMinElements {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || len(closure.Fn.Params) != 1 || len(closure.Fn.ParamLocalIndices) != 1 ||
		closure.Fn.HasRestParam || closure.Fn.HasBlockParam || len(closure.Fn.KeywordParams) != 0 ||
		closure.Fn.KeywordRestParam != "" || closure.Fn.KeywordRestOnly || !simpleBlockParameterPatterns(closure.Fn) {
		return nil, false
	}
	for _, defaultValue := range closure.Fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	leaf, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil || !typedSSAStringTypeBranchShape(leaf.registerIR) {
		return nil, false
	}
	constant, constantOK := vm.topLevelConstantValue("String")
	if !constantOK || constant == nil || constant.Type != object.ValueClass || constant.Data != core.R.Classes["String"] ||
		!core.StringIsAUsesBuiltinImplementation() || !core.StringLengthUsesBuiltinImplementation() {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, 0, len(elems))
	}
	for _, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		raw, exact := typedSSAExactStringValue(elem)
		if !exact || !typedSSAASCIIString(raw) {
			return nil, false
		}
		if collect {
			results = append(results, core.NewIntegerValue(int64(len(raw))))
		}
	}
	core.LastBlockResult = nil
	if !collect {
		return receiver, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

func typedSSAExactStringValue(value *object.EmeraldValue) (string, bool) {
	if value == nil || value.Type != object.ValueString || value.Class != nil && value.Class != core.R.Classes["String"] ||
		value.Frozen || value.Chilled || len(value.InstanceVars) != 0 || core.AttachedSingletonClass(value) != nil {
		return "", false
	}
	if value.Encoding != "" && value.Encoding != "UTF-8" {
		return "", false
	}
	raw, ok := value.Data.(string)
	return raw, ok
}

func typedSSAASCIIString(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= 0x80 {
			return false
		}
	}
	return true
}

// tryExecuteArrayTypedSSAObjectIntegerToStringBatch removes both the callback
// frame and the getter/to_s send pair for a pure object map. It preflights the
// complete array before returning a mapped result, so a heterogeneous object,
// reflected/materialized shape, redefined getter, Integer subclass, or BigInt
// can safely side-exit without replaying a visible prefix.
func (vm *VM) tryExecuteArrayTypedSSAObjectIntegerToStringBatch(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || block == nil || block.Type != object.ValueClosure ||
		!typedSSABatchCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSABatchCallMinElements {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || closure.AutoSplat && blockWantsDestructuring(closure.Fn) ||
		len(closure.Fn.Params) != 1 || len(closure.Fn.ParamLocalIndices) != 1 || closure.Fn.HasRestParam ||
		closure.Fn.HasBlockParam || len(closure.Fn.KeywordParams) != 0 || closure.Fn.KeywordRestParam != "" ||
		closure.Fn.KeywordRestOnly || !simpleBlockParameterPatterns(closure.Fn) {
		return nil, false
	}
	for _, defaultValue := range closure.Fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	leaf, leafFound := vm.cachedBlockLeafPlan(closure.Fn)
	if !leafFound || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil ||
		closure.ReturnOwnerID > 0 && leaf.registerIR.hasExplicitReturn {
		return nil, false
	}
	plan, ok := vm.cachedTypedSSABlockPlan(closure.Fn)
	integerGetterName, integerShapeOK := typedSSAObjectIntegerToStringShape(plan, closure.Fn)
	integerLengthGetterName, integerLengthShapeOK := typedSSAObjectIntegerToStringLengthShape(plan, closure.Fn)
	concatGetterName, concatSuffix, concatShapeOK := typedSSAObjectStringConcatShape(plan, closure.Fn)
	concatLengthGetterName, concatLengthSuffix, concatLengthShapeOK := typedSSAObjectStringConcatLengthShape(plan, closure.Fn)
	if !ok || !integerShapeOK && !integerLengthShapeOK && !concatShapeOK && !concatLengthShapeOK {
		return nil, false
	}
	integerResult := integerShapeOK || integerLengthShapeOK
	lengthResult := integerLengthShapeOK
	getterName := integerGetterName
	if integerLengthShapeOK {
		getterName = integerLengthGetterName
	} else if !integerResult && concatLengthShapeOK {
		getterName = concatLengthGetterName
		concatSuffix = concatLengthSuffix
		lengthResult = true
	} else if !integerResult {
		getterName = concatGetterName
	}
	if lengthResult && !core.StringLengthUsesBuiltinImplementation() ||
		integerResult && !core.IntegerToSUsesBuiltinImplementation() ||
		!integerResult && !core.StringPlusUsesBuiltinImplementation() {
		return nil, false
	}
	first := elems[0]
	if first == nil || first.Type != object.ValueObject || first.Class == nil ||
		!vm.registerIRCacheableReceiverForMethod(first, getterName) {
		return nil, false
	}
	firstObject, ok := first.Data.(*object.Object)
	if !ok || firstObject == nil || len(firstObject.SingletonMethods) != 0 || firstObject.SingletonClass != nil {
		return nil, false
	}
	getterMethod, _, fallback := vm.lookupMethodForSend(first, getterName, nil, false, true)
	if fallback != nil || getterMethod == nil || getterMethod.DispatchOwner != nil ||
		(getterMethod.Visibility != "" && getterMethod.Visibility != "public") || getterMethod.Ruby2Keywords ||
		methodUsesRefinements(getterMethod) {
		return nil, false
	}
	getterFn, ok := getterMethod.Fn.(*object.Function)
	if !ok || getterFn == nil || len(getterFn.Params) != 0 || len(getterFn.ParamLocalIndices) != 0 || getterFn.HasRestParam ||
		getterFn.HasBlockParam || len(getterFn.KeywordParams) != 0 || getterFn.KeywordRestParam != "" || getterFn.KeywordRestOnly ||
		registerIRFunctionNeedsDefaultEvaluation(getterFn, 0) {
		return nil, false
	}
	getterPlan, ok := vm.cachedTypedSSAPlan(getterFn)
	getterIvar, getterOK := typedSSASimpleGetterIvar(getterPlan)
	if !ok || !getterOK || getterIvar == "" {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	integerClass := core.R.Classes["Integer"]
	getterSlot, getterSlotClass, getterSlotReady := typedSSACompactGetterSlot(first, getterIvar)
	previousStringBatch := vm.typedStringValueBatch
	if collect && previousStringBatch == nil {
		vm.typedStringValueBatch = core.NewStringValueBatch(len(elems))
	}
	defer func() {
		vm.typedStringValueBatch = previousStringBatch
	}()
	var results []*object.EmeraldValue
	if collect {
		results = make([]*object.EmeraldValue, 0, len(elems))
	}
	for _, elem := range elems {
		if elem == nil || elem.Type != object.ValueObject || elem.Class != first.Class {
			return nil, false
		}
		obj, ok := elem.Data.(*object.Object)
		if !ok || obj == nil || len(obj.SingletonMethods) != 0 || obj.SingletonClass != nil {
			return nil, false
		}
		var value *object.EmeraldValue
		var handled bool
		if getterSlotReady {
			value, handled = typedSSACompactGetterValue(elem, getterSlotClass, getterSlot)
		}
		if !handled {
			value, handled = typedSSAMapGetterValue(elem, getterIvar)
		}
		if !handled {
			value = core.DynamicInstanceVar(elem, getterIvar)
		}
		var raw string
		var stringValue string
		var integerValue int64
		if integerResult {
			argument, integerOK := typedSSAExactIntegerValueForClass(value, integerClass)
			if !integerOK {
				return nil, false
			}
			integerValue = argument
			if !collect || lengthResult || vm.typedStringValueBatch == nil || !typedSSAIntegerStringBatchEnabled {
				raw = core.IntegerToSRawBuiltin(argument)
			}
		} else {
			var stringOK bool
			stringValue, stringOK = typedSSAExactStringValue(value)
			if !stringOK {
				return nil, false
			}
			if !lengthResult {
				raw = stringValue + concatSuffix
			}
		}
		if collect {
			if lengthResult {
				if !typedSSAASCIIString(stringValue) || !typedSSAASCIIString(concatSuffix) {
					return nil, false
				}
				results = append(results, core.NewIntegerValue(int64(len(stringValue)+len(concatSuffix))))
			} else if integerResult && typedSSAIntegerStringBatchEnabled && vm.typedStringValueBatch != nil {
				results = append(results, vm.typedStringValueBatch.NewInteger(integerValue))
			} else if vm.typedStringValueBatch != nil {
				results = append(results, vm.typedStringValueBatch.New(raw))
			} else {
				results = append(results, core.NewStringValue(raw))
			}
		} else if lengthResult && (!typedSSAASCIIString(stringValue) || !typedSSAASCIIString(concatSuffix)) {
			return nil, false
		}
	}
	core.LastBlockResult = nil
	if !collect {
		return receiver, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

func (vm *VM) executeTypedSSABatchCall(shape typedSSABatchCallShape, elems []*object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || shape.plan == nil || shape.fn == nil || shape.send == nil || len(elems) == 0 {
		return nil, false
	}
	vm.typedSSABatchCallCount++
	previousStringBatch := vm.typedStringValueBatch
	defer func() {
		vm.typedStringValueBatch = previousStringBatch
	}()
	generation := object.CurrentMethodGeneration()
	var calleeObj *object.Method
	var calleeFn *object.Function
	var calleePlan *typedSSAPlan
	var calleeIntegerPlan *integerFunctionPlan
	var calleeNative func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var getterIvar string
	var getterSlot uint8
	var getterSlotClass *object.Class
	getterSlotReady := false
	var calleeConstant *object.EmeraldValue
	// calleePrimitivePlan is the direct primitive ABI edge for a one-argument
	// Ruby callee. The outer Array callback has already proved the method
	// identity and generation, so the callee can consume one raw Integer and
	// return an unboxed Integer/Bool/Nil value without rebuilding an SSA value
	// array or entering executeTypedSSAPlan for every element.
	var calleePrimitivePlan bool
	// calleeStringPlan is the analogous raw ABI edge for a pure Integer helper
	// returning a newly boxed String. The batch owns only the Ruby String
	// headers; each element still receives a distinct mutable Ruby object.
	var calleeStringPlan bool
	var calleeIntegerStringConcatPlan bool
	var calleeIntegerStringConcatKernel typedSSAIntegerStringConcatKernel
	noCodeGenerationStable := false
	var firstReceiver *object.EmeraldValue
	var classStable bool
	var results []*object.EmeraldValue
	integerClass := core.R.Classes["Integer"]
	// The argument slice is only borrowed by method lookup/execution. Keeping
	// its small backing array at batch scope avoids one heap allocation per
	// element when the typed callback has positional arguments.
	var argStorage [4]*object.EmeraldValue
	var args []*object.EmeraldValue
	primitiveInputsReady := false
	if collect {
		results = make([]*object.EmeraldValue, 0, len(elems))
	}
	for _, elem := range elems {
		if noCodeGenerationStable && (calleePrimitivePlan || calleeStringPlan || calleeIntegerStringConcatPlan) && (shape.directSelfParam || shape.directFreeParam) {
			if !primitiveInputsReady {
				if !typedSSABatchPrimitiveInputsSafe(elems, integerClass) {
					return nil, false
				}
				primitiveInputsReady = true
			}
			if calleePrimitivePlan {
				value, executed := vm.executeTypedSSABatchPrimitiveInteger(calleeFn, calleeIntegerPlan, calleePlan, elem.Data.(int64))
				if !executed {
					return nil, false
				}
				if collect {
					results = append(results, value)
				}
				continue
			}
			if calleeIntegerStringConcatPlan {
				value, executed := vm.executeTypedSSAIntegerStringConcatKernelTrusted(calleeIntegerStringConcatKernel, elem.Data.(int64))
				if !executed {
					return nil, false
				}
				if collect {
					results = append(results, value)
				}
				continue
			}
			value, executed := vm.executeTypedSSAUnboxedArgsPlanTrusted(calleePlan, calleeFn, []int64{elem.Data.(int64)})
			if !executed {
				return nil, false
			}
			if collect {
				results = append(results, vm.typedSSAValueToObjectForVM(value))
			}
			continue
		}
		var registers [16]*object.EmeraldValue
		var receiver *object.EmeraldValue
		if shape.directSelfParam {
			receiver = shape.self
			if receiver == nil {
				return nil, false
			}
			if elem == nil {
				elem = core.R.NilVal
			}
			argStorage[0] = elem
			args = argStorage[:1]
		} else if shape.directFreeParam {
			if int(shape.directFreeIndex) >= len(shape.free) {
				return nil, false
			}
			receiver = derefClosureValue(shape.free[shape.directFreeIndex])
			if receiver == nil {
				return nil, false
			}
			if elem == nil {
				elem = core.R.NilVal
			}
			argStorage[0] = elem
			args = argStorage[:1]
		} else if shape.directParam {
			receiver = elem
			if receiver == nil {
				receiver = core.R.NilVal
			}
			args = argStorage[:0]
		} else {
			var ok bool
			registers, ok = typedSSABatchCallRegisters(shape, elem)
			if !ok {
				return nil, false
			}
			receiver = registers[shape.send.left]
			if receiver == nil {
				receiver = core.R.NilVal
			}
			receiver = derefClosureValue(receiver)
			for index := 0; index < int(shape.send.argc); index++ {
				argStorage[index] = registers[shape.send.args[index]]
				if argStorage[index] == nil {
					argStorage[index] = core.R.NilVal
				}
			}
			args = argStorage[:int(shape.send.argc)]
		}
		var method *object.Method
		if calleeObj != nil && receiver == firstReceiver {
			method = calleeObj
		} else if calleeObj != nil && classStable && receiver.Class == firstReceiver.Class {
			// Exact built-in classes and ordinary objects without singleton
			// methods cannot change the send target while the Ruby VM is
			// single-threaded and the typed callee is pure. Avoid a full lookup
			// for every element; generation still guards redefinition.
			if !vm.registerIRCacheableReceiverForMethod(receiver, shape.send.name) {
				return nil, false
			}
			method = calleeObj
		} else {
			var fallback *object.EmeraldValue
			method, _, fallback = vm.lookupMethodForSend(receiver, shape.send.name, args, false, true)
			if fallback != nil {
				return nil, false
			}
		}
		if method == nil || method.DispatchOwner != nil ||
			(method.Visibility != "" && method.Visibility != "public" &&
				!(method.Visibility == "private" && receiver == shape.self)) || method.Ruby2Keywords ||
			methodUsesRefinements(method) {
			return nil, false
		}
		if nativeFn, nativeOK := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); nativeOK {
			if !registerIRTrustedNativeNoEscapeName(shape.send.name) {
				return nil, false
			}
			// A native edge is safe here only for a fixed positional method. The
			// block shape has already rejected splats/keywords/blocks; rejecting
			// Arity < 0 keeps variable-arity native methods on Ruby's complete
			// argument protocol.
			if method.Arity < 0 || method.Arity != len(args) {
				return nil, false
			}
			if calleeObj == nil {
				calleeObj, calleeNative = method, nativeFn
				firstReceiver = receiver
				classStable = vm.registerIRCacheableReceiverForMethod(receiver, shape.send.name)
			} else if method != calleeObj || calleeNative == nil {
				// A changing singleton/class implementation would make a partial
				// batch unsafe. Deopt before exposing any mapped result.
				return nil, false
			}
			if object.CurrentMethodGeneration() != generation {
				return nil, false
			}
			value := callNativeMethod(calleeNative, receiver, args)
			if value == nil {
				value = core.R.NilVal
			}
			if value.Type == object.ValueException {
				return value, true
			}
			if collect {
				results = append(results, value)
			}
			continue
		}
		fn, ok := method.Fn.(*object.Function)
		if !ok || fn == nil || len(fn.Params) != len(args) || len(fn.ParamLocalIndices) != len(args) ||
			fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) > 0 ||
			fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectKeywords || fn.RejectBlock ||
			!simpleBlockParameterPatterns(fn) || registerIRFunctionNeedsDefaultEvaluation(fn, len(args)) {
			return nil, false
		}
		if calleeObj == nil {
			calleeObj, calleeFn = method, fn
			firstReceiver = receiver
			classStable = vm.registerIRCacheableReceiverForMethod(receiver, shape.send.name)
			typedPlan, typedOK := vm.cachedTypedSSAPlan(fn)
			integerPlan, integerOK := vm.cachedIntegerFunctionPlan(fn)
			if !typedOK && !integerOK {
				vm.typedSSABatchCalleePlanMisses++
				return nil, false
			}
			calleePlan = typedPlan
			var getterOK bool
			getterIvar, getterOK = typedSSASimpleGetterIvar(calleePlan)
			if calleePlan != nil && (calleePlan.hasInstanceStore || calleePlan.hasYield ||
				calleePlan.hasReference && !getterOK) {
				// This batch has no replay log. A callee that mutates an object,
				// yields, or returns an unproven reference must remain on the
				// side-effect-aware direct tier.
				return nil, false
			}
			if calleePlan != nil {
				calleeConstant, _ = typedSSAConstantReturnObject(calleePlan)
			}
			if integerOK && len(fn.Params) == 1 && len(fn.ParamLocalIndices) == 1 {
				calleeIntegerPlan = integerPlan
			}
			if len(args) == 1 {
				vm.typedSSABatchPrimitiveCandidates++
			}
			if calleeIntegerPlan != nil {
				vm.typedSSABatchPrimitivePlans++
			} else if calleePlan == nil {
				vm.typedSSABatchPrimitiveOpRejects++
			} else if calleePlan.hasReference {
				vm.typedSSABatchPrimitiveReferenceRejects++
			} else if calleePlan.hasFloat || calleePlan.hasString || !vm.typedSSAIntegerOpsAvailable(calleePlan) {
				vm.typedSSABatchPrimitiveOpRejects++
			}
			calleePrimitivePlan = len(args) == 1 && (calleeIntegerPlan != nil || calleePlan != nil &&
				!calleePlan.hasReference && !calleePlan.hasFloat && !calleePlan.hasString &&
				vm.typedSSAIntegerOpsAvailable(calleePlan))
			if calleeIntegerPlan == nil && calleePlan != nil {
				if getterIvar != "" {
					getterSlot, getterSlotClass, getterSlotReady = typedSSACompactGetterSlot(receiver, getterIvar)
				}
			}
			calleeStringPlan = len(args) == 1 && typedSSAPlanCanUseUnboxedStringResult(calleePlan) &&
				vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan)
			calleeIntegerStringConcatKernel = typedSSAIntegerStringConcatKernel{}
			if typedSSAIntegerStringConcatEnabled && len(args) == 1 && calleePlan != nil &&
				calleePlan.integerStringConcatKernel.kind != typedSSAIntegerStringConcatKernelNone &&
				vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) &&
				core.IntegerToSUsesBuiltinImplementation() && core.StringPlusUsesBuiltinImplementation() &&
				typedSSAPlanASCIIStringLiterals(calleePlan) {
				calleeIntegerStringConcatKernel = calleePlan.integerStringConcatKernel
				calleeIntegerStringConcatPlan = true
			}
			if (calleeStringPlan || calleeIntegerStringConcatPlan) && collect && previousStringBatch == nil {
				vm.typedStringValueBatch = core.NewStringValueBatch(len(elems))
			}
		} else if method != calleeObj || fn != calleeFn {
			// A different method or singleton/class implementation would make a
			// partial batch unsafe. Deopt before exposing any mapped result.
			return nil, false
		}
		if !noCodeGenerationStable && object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		// Constant returns, simple ivar getters and unboxed integer plans do not
		// execute Ruby/native code inside the element loop. Once the first
		// method-generation check has passed, no element can mutate the global
		// method table, so avoid repeating that atomic read for every item.
		if calleeConstant != nil || getterIvar != "" || calleePrimitivePlan || calleeStringPlan || calleeIntegerStringConcatPlan {
			noCodeGenerationStable = true
		}
		if calleeConstant != nil {
			if collect {
				results = append(results, calleeConstant)
			}
			continue
		}
		if calleePrimitivePlan {
			// typedSSAValueFromObject deliberately treats Integer subclasses and
			// BigInts as references. Keep the same boundary rule here: an
			// overridden/singleton Integer must replay through Ruby dispatch.
			if len(args) != 1 || !typedIntegerArgumentClass(args[0], integerClass) || core.AttachedSingletonClass(args[0]) != nil {
				return nil, false
			}
			value, executed := vm.executeTypedSSABatchPrimitiveElement(calleeFn, calleeIntegerPlan, calleePlan, integerClass, args[0])
			if !executed {
				// Overflow or a changed primitive guard has no observable prefix:
				// this call graph is pure, so the caller can replay the complete
				// Array operation through the compatibility path.
				return nil, false
			}
			if collect {
				results = append(results, value)
			}
			continue
		}
		if calleeIntegerStringConcatPlan {
			if len(args) != 1 {
				return nil, false
			}
			argument, ok := typedSSAExactIntegerValueForClass(args[0], integerClass)
			if !ok || core.AttachedSingletonClass(args[0]) != nil {
				return nil, false
			}
			value, executed := vm.executeTypedSSAIntegerStringConcatKernelTrusted(calleeIntegerStringConcatKernel, argument)
			if !executed {
				return nil, false
			}
			if collect {
				results = append(results, value)
			}
			continue
		}
		if calleeStringPlan {
			if len(args) != 1 {
				return nil, false
			}
			argument, ok := typedSSAExactIntegerValueForClass(args[0], integerClass)
			if !ok || core.AttachedSingletonClass(args[0]) != nil {
				return nil, false
			}
			value, executed := vm.executeTypedSSAUnboxedArgsPlanTrusted(calleePlan, calleeFn, []int64{argument})
			if !executed {
				return nil, false
			}
			if collect {
				results = append(results, vm.typedSSAValueToObjectForVM(value))
			}
			continue
		}
		var value *object.EmeraldValue
		if getterIvar != "" {
			compactHandled := false
			if getterSlotReady {
				value, compactHandled = typedSSACompactGetterValue(receiver, getterSlotClass, getterSlot)
			}
			if !compactHandled {
				value, compactHandled = typedSSAMapGetterValue(receiver, getterIvar)
			}
			if !compactHandled {
				value = core.DynamicInstanceVar(receiver, getterIvar)
			}
			if value == nil {
				value = core.R.NilVal
			}
		} else {
			var executed bool
			value, executed = vm.executeTypedSSAPlan(calleePlan, calleeFn, receiver, args)
			if !executed || value == nil || value.Type == object.ValueException {
				return nil, false
			}
		}
		if collect {
			results = append(results, value)
		}
	}
	core.LastBlockResult = nil
	if !collect {
		return &object.EmeraldValue{Type: object.ValueArray, Data: elems, Class: core.R.Classes["Array"]}, true
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}

// typedSSAMapGetterValue is the map-backed sibling of the compact getter
// path. The compatibility layout stores the common user-object fields in the
// Object map; once the callee and exact ValueObject shape are proven, the
// generic ValueType switch and GetInstanceVar dispatch are unnecessary.
func typedSSAMapGetterValue(receiver *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if receiver == nil || receiver.Type != object.ValueObject || name == "" {
		return nil, false
	}
	obj, ok := receiver.Data.(*object.Object)
	if !ok || obj == nil || obj.InstanceVars == nil {
		return nil, false
	}
	if obj.Class != nil && obj.Class.CompactInstanceVars {
		if value, exists := obj.InstanceVars[name]; exists {
			return value, true
		}
		return nil, false
	}
	return obj.InstanceVars[name], true
}

// typedSSACompactGetterSlot resolves the field name once for a proven typed
// getter batch. Compact objects keep their first few fields in the payload,
// so repeating the class's string-to-slot map lookup for every array element
// is unnecessary. A false result deliberately leaves map-backed and
// reflective objects on DynamicInstanceVar.
func typedSSACompactGetterSlot(receiver *object.EmeraldValue, name string) (uint8, *object.Class, bool) {
	if !typedSSACompactGetterEnabled || receiver == nil || receiver.Type != object.ValueObject ||
		receiver.Class == nil || !receiver.Class.CompactInstanceVars || name == "" {
		return 0, nil, false
	}
	index, ok := receiver.Class.InstanceVarSlots[name]
	if !ok || int(index) >= len((&object.Object{}).InlineInstanceVars) {
		return 0, nil, false
	}
	return uint8(index), receiver.Class, true
}

// typedSSACompactGetterValue returns handled=true only when the object is a
// compact, non-materialized instance. A materialized InstanceVars map is
// authoritative because reflective APIs and compatibility helpers may have
// written it directly.
func typedSSACompactGetterValue(receiver *object.EmeraldValue, class *object.Class, slot uint8) (*object.EmeraldValue, bool) {
	if receiver == nil || receiver.Type != object.ValueObject || receiver.Class != class || class == nil {
		return nil, false
	}
	obj, ok := receiver.Data.(*object.Object)
	if !ok || obj == nil || obj.InstanceVars != nil || int(slot) >= len(obj.InlineInstanceVars) {
		return nil, false
	}
	if obj.InlineInstanceVarMask&(uint8(1)<<slot) == 0 {
		return nil, true
	}
	return obj.InlineInstanceVars[slot], true
}

func (vm *VM) reportTypedSSABatchStats() {
	if vm == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "RGO_TYPED_SSA_BATCH calls=%d callee_plan_misses=%d candidates=%d reference_rejects=%d op_rejects=%d primitive_plans=%d primitive_elements=%d field_attempts=%d field_hits=%d\n",
		vm.typedSSABatchCallCount, vm.typedSSABatchCalleePlanMisses,
		vm.typedSSABatchPrimitiveCandidates, vm.typedSSABatchPrimitiveReferenceRejects,
		vm.typedSSABatchPrimitiveOpRejects, vm.typedSSABatchPrimitivePlans,
		vm.typedSSABatchPrimitiveElements, vm.typedSSABatchFieldReduceAttempts,
		vm.typedSSABatchFieldReduceHits)
}

type typedSSAFieldReduceShape struct {
	closure   *object.Closure
	fn        *object.Function
	plan      *registerIRPlan
	send      *registerIRInstruction
	freeIndex uint8
	op        compiler.Opcode
}

// prepareTypedSSAFieldReduceShape recognizes the first object-layout region
// that is useful outside numeric microbenchmarks:
//
//	total += item.value
//
// The block has one captured Integer, one parameter, one zero-argument Ruby
// getter and one captured store.  The getter itself must reduce to a single
// ivar read, so the loop can prove the method identity once and then use the
// object layout directly.  No capture is written until every element has
// passed its shape/type/overflow guards, which makes a miss safe to replay.
func (vm *VM) prepareTypedSSAFieldReduceShape(block *object.EmeraldValue) (typedSSAFieldReduceShape, bool) {
	invalid := typedSSAFieldReduceShape{}
	if vm == nil || !typedSSABatchCallEnabled || block == nil || block.Type != object.ValueClosure ||
		!registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() {
		return invalid, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) {
		return invalid, false
	}
	fn := closure.Fn
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) {
		return invalid, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}
	leaf, found := vm.cachedBlockLeafPlan(fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil || !leaf.registerIR.blockReturn ||
		leaf.registerIR.hasBranches || leaf.registerIR.sendCount != 1 || len(leaf.registerIR.instructions) != 6 ||
		closure.ReturnOwnerID > 0 && leaf.registerIR.hasExplicitReturn {
		return invalid, false
	}
	instructions := leaf.registerIR.instructions
	loadFree, loadParam, send, binary, storeFree, ret := instructions[0], instructions[1], instructions[2], instructions[3], instructions[4], instructions[5]
	if loadFree.op != registerIRLoadFree || loadParam.op != registerIRLoadParam || loadParam.param != 0 ||
		send.op != registerIRSend || send.argc != 0 || send.blockPresent || send.splatIndex != 255 || send.name == "" ||
		binary.op != registerIRBinary || (binary.opcode != compiler.OpAdd && binary.opcode != compiler.OpSub) ||
		binary.left != loadFree.dst || binary.right != send.dst || binary.dst != loadFree.dst ||
		storeFree.op != registerIRStoreFree || storeFree.param != loadFree.param || storeFree.left != binary.dst ||
		ret.op != registerIRReturn || ret.left != binary.dst {
		return invalid, false
	}
	return typedSSAFieldReduceShape{
		closure: closure, fn: fn, plan: leaf.registerIR, send: &instructions[2],
		freeIndex: loadFree.param, op: binary.opcode,
	}, true
}

func (vm *VM) tryExecuteArrayTypedSSAFieldReduce(receiver, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm != nil {
		vm.typedSSABatchFieldReduceAttempts++
	}
	shape, ok := vm.prepareTypedSSAFieldReduceShape(block)
	if !ok || receiver == nil || receiver.Type != object.ValueArray {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSAFieldReduceMinElements || int(shape.freeIndex) >= len(shape.closure.Free) {
		return nil, false
	}
	current := derefClosureValue(shape.closure.Free[shape.freeIndex])
	if !typedIntegerArgument(current) || core.AttachedSingletonClass(current) != nil {
		return nil, false
	}
	if len(elems) == 0 {
		core.LastBlockResult = nil
		return receiver, true
	}
	total := current.Data.(int64)
	generation := object.CurrentMethodGeneration()
	var calleeObj *object.Method
	var calleeFn *object.Function
	var calleePlan *typedSSAPlan
	var getterIvar string
	var firstReceiver *object.EmeraldValue
	var classStable bool
	for _, elem := range elems {
		if elem == nil {
			return nil, false
		}
		var method *object.Method
		if calleeObj != nil && elem == firstReceiver {
			method = calleeObj
		} else if calleeObj != nil && classStable && elem.Class == firstReceiver.Class && vm.registerIRCacheableReceiverForMethod(elem, shape.send.name) {
			method = calleeObj
		} else {
			var fallback *object.EmeraldValue
			method, _, fallback = vm.lookupMethodForSend(elem, shape.send.name, nil, false, true)
			if fallback != nil {
				return nil, false
			}
		}
		if method == nil || method.DispatchOwner != nil || method.Visibility != "" && method.Visibility != "public" ||
			method.Ruby2Keywords || methodUsesRefinements(method) {
			return nil, false
		}
		fn, fnOK := method.Fn.(*object.Function)
		if !fnOK || fn == nil || len(fn.Params) != 0 || len(fn.ParamLocalIndices) != 0 || fn.HasRestParam ||
			fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
			return nil, false
		}
		if calleeObj == nil {
			calleeObj, calleeFn = method, fn
			firstReceiver = elem
			classStable = vm.registerIRCacheableReceiverForMethod(elem, shape.send.name)
			calleePlan, ok = vm.cachedTypedSSAPlan(fn)
			if !ok || calleePlan == nil {
				return nil, false
			}
			getterIvar, ok = typedSSASimpleGetterIvar(calleePlan)
			if !ok {
				return nil, false
			}
		} else if method != calleeObj || fn != calleeFn {
			return nil, false
		}
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		value := core.DynamicInstanceVar(elem, getterIvar)
		if !typedIntegerArgument(value) || core.AttachedSingletonClass(value) != nil {
			return nil, false
		}
		var updated bool
		total, updated = applyRegisterIRIntegerLinearOpRaw(shape.op, total, value.Data.(int64))
		if !updated {
			return nil, false
		}
	}
	setClosureValue(&shape.closure.Free[shape.freeIndex], core.NewIntegerValue(total))
	vm.typedSSABatchFieldReduceHits++
	core.LastBlockResult = nil
	return receiver, true
}

// typedSSASimpleGetterIvar recognizes the smallest object-layout callee in
// the typed call graph: a fixed-arity Ruby method that only returns one
// instance variable. The batch caller can read the proven field directly for
// every element, avoiding a typed-SSA switch and a fresh boundary value per
// callback while retaining the method-generation/identity guards around the
// edge.
func typedSSASimpleGetterIvar(plan *typedSSAPlan) (string, bool) {
	if plan == nil || len(plan.ops) < 2 || len(plan.ops) > 3 {
		return "", false
	}
	loadIndex := 0
	if len(plan.ops) == 3 {
		if plan.ops[0].kind != typedSSAOpLoadSelf {
			return "", false
		}
		loadIndex = 1
	}
	load, ret := plan.ops[loadIndex], plan.ops[loadIndex+1]
	if load.kind != typedSSAOpLoadInstanceVar || load.name == "" ||
		ret.kind != typedSSAOpReturn || ret.left != load.dst {
		return "", false
	}
	return load.name, true
}

// tryExecuteArrayTypedSSACallBlock fuses Array#map/each's callback protocol
// with a pure typed callee.  Unlike a speculative send inside a larger body,
// every element is executed only after its method identity and typed plan are
// known; a miss therefore has no user-visible prefix to replay.
func (vm *VM) tryExecuteArrayTypedSSACallBlock(receiver, block *object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueArray || block == nil ||
		block.Type != object.ValueClosure || !typedSSABatchCallEnabled {
		return nil, false
	}
	elems, ok := receiver.Data.([]*object.EmeraldValue)
	if !ok || len(elems) < typedSSABatchCallMinElements {
		return nil, false
	}
	if result, handled := vm.tryExecuteArrayTypedSSAStringLengthBatch(receiver, block, collect); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSAStringTypeBranchBatch(receiver, block, collect); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSAObjectIntegerToStringBatch(receiver, block, collect); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSARescueIntegerStringBatch(receiver, block, collect); handled {
		return result, true
	}
	shape, ok := vm.prepareTypedSSABatchCallShape(block)
	if !ok {
		return nil, false
	}
	if len(elems) == 0 {
		core.LastBlockResult = nil
		if collect {
			return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: core.R.Classes["Array"]}, true
		}
		return receiver, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSAStringMapLazyResult(receiver, block, elems, collect); handled {
		return result, true
	}
	if result, handled := vm.tryExecuteArrayTypedSSAIntegerMapLazyResult(receiver, block, elems, collect); handled {
		return result, true
	}
	if result, handled := vm.executeTypedSSASimpleGetterBatch(shape, elems, collect); handled {
		return result, true
	}
	return vm.executeTypedSSABatchCall(shape, elems, collect)
}

// typedIntegerMapLazyPayload keeps the already-proven pure integer callback
// inputs as raw values. Integer results are immutable and object_id is
// value-based, so ElementAt can create only the requested result without a
// per-index pointer cache. Materialization still produces the ordinary Ruby
// []*EmeraldValue representation when a later operation needs all elements.
type typedIntegerMapLazyPayload struct {
	inputs   []int64
	input    int64
	length   int
	constant bool
	kernel   typedSSAIntegerKernel
}

func (payload *typedIntegerMapLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	input := payload.input
	if !payload.constant {
		input = payload.inputs[index]
	}
	result, ok := executeTypedSSAIntegerKernel(payload.kernel, input)
	if !ok {
		return nil, false
	}
	return core.NewSmallIntegerValue(result), true
}

func (payload *typedIntegerMapLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	values := make([]*object.EmeraldValue, payload.length)
	for index := range values {
		value, ok := payload.valueAt(index)
		if !ok {
			values[index] = core.R.NilVal
			continue
		}
		values[index] = value
	}
	return values
}

func (payload *typedIntegerMapLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

// tryExecuteArrayTypedSSAIntegerMapLazyResult admits only the direct captured
// helper shape whose callee is the compact pure Integer branch kernel. The
// complete input/type/overflow preflight runs before exposing the lazy Array,
// so later source-array mutation or method redefinition cannot change the
// already-computed map result.
func (vm *VM) tryExecuteArrayTypedSSAIntegerMapLazyResult(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || !typedSSAIntegerMapLazyResultEnabled || !collect || receiver == nil || block == nil ||
		len(elems) < typedSSABatchCallMinElements || core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	shape, ok := vm.prepareTypedSSABatchCallShape(block)
	if !ok || shape.send == nil || shape.send.argc != 1 || !shape.directSelfParam && !shape.directFreeParam {
		return nil, false
	}
	var calleeReceiver *object.EmeraldValue
	if shape.directSelfParam {
		calleeReceiver = shape.self
	} else {
		if int(shape.directFreeIndex) >= len(shape.free) {
			return nil, false
		}
		calleeReceiver = derefClosureValue(shape.free[shape.directFreeIndex])
	}
	if calleeReceiver == nil {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(calleeReceiver, shape.send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil || method.Ruby2Keywords || methodUsesRefinements(method) ||
		(method.Visibility != "" && method.Visibility != "public" && !(method.Visibility == "private" && calleeReceiver == shape.self)) {
		return nil, false
	}
	calleeFn, ok := method.Fn.(*object.Function)
	if !ok || calleeFn == nil || len(calleeFn.Params) != 1 || len(calleeFn.ParamLocalIndices) != 1 ||
		calleeFn.HasRestParam || calleeFn.HasBlockParam || len(calleeFn.KeywordParams) != 0 || calleeFn.KeywordRestParam != "" ||
		calleeFn.KeywordRestOnly || calleeFn.RejectKeywords || calleeFn.RejectBlock || registerIRFunctionNeedsDefaultEvaluation(calleeFn, 1) {
		return nil, false
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(calleeFn)
	if !ok || calleePlan == nil || calleePlan.blockReturn || calleePlan.hasReference || calleePlan.hasFloat || calleePlan.hasString ||
		calleePlan.hasYield || calleePlan.hasInstanceStore || calleePlan.integerKernel.kind != typedSSAIntegerKernelCompareBinary ||
		!vm.typedSSAIntegerOpsAvailable(calleePlan) || !vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) {
		return nil, false
	}
	integerClass := core.R.IntegerClass
	if integerClass == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	first, exact := typedSSAExactIntegerValueForClass(elems[0], integerClass)
	if !exact || core.AttachedSingletonClass(elems[0]) != nil {
		return nil, false
	}
	constantInput := true
	for _, elem := range elems[1:] {
		if elem != elems[0] {
			constantInput = false
			break
		}
	}
	if constantInput {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		if _, executed := executeTypedSSAIntegerKernel(calleePlan.integerKernel, first); !executed {
			return nil, false
		}
		payload := &typedIntegerMapLazyPayload{
			input: first, length: len(elems), constant: true, kernel: calleePlan.integerKernel,
		}
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(elems), Payload: payload, Materialize: payload.materialize,
			ElementAt: payload.elementAt, MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		core.LastBlockResult = nil
		return result, true
	}
	inputs := make([]int64, len(elems))
	inputs[0] = first
	if _, executed := executeTypedSSAIntegerKernel(calleePlan.integerKernel, first); !executed {
		return nil, false
	}
	for index, elem := range elems[1:] {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		value, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact || core.AttachedSingletonClass(elem) != nil {
			return nil, false
		}
		if _, executed := executeTypedSSAIntegerKernel(calleePlan.integerKernel, value); !executed {
			return nil, false
		}
		inputs[index+1] = value
	}
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	payload := &typedIntegerMapLazyPayload{inputs: inputs, length: len(inputs), kernel: calleePlan.integerKernel}
	result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
	result.SetLazyArrayRegion(&object.LazyArrayRegion{
		Length: len(inputs), Payload: payload, Materialize: payload.materialize,
		ElementAt: payload.elementAt, MethodGeneration: generation,
	})
	core.RegisterLazyArrayRegion(result)
	core.LastBlockResult = nil
	return result, true
}

// typedIntegerStringMapLazyPayload keeps the already-proven pure callback
// inputs as raw Integers. The map operation is complete before this payload
// is exposed; ElementAt only delays allocating the result object and caches
// each one so repeated indexing observes the same object.
type typedIntegerStringMapLazyPayload struct {
	inputs       []int64
	values       []*object.EmeraldValue
	kernel       typedSSAIntegerStringConcatKernel
	encoding     string
	lengthResult bool
}

func (payload *typedIntegerStringMapLazyPayload) lazyLength() int {
	if payload == nil {
		return 0
	}
	return len(payload.inputs)
}

func (payload *typedIntegerStringMapLazyPayload) stringLengthAt(index int) (int64, bool) {
	if payload == nil || payload.lengthResult || index < 0 || index >= len(payload.inputs) ||
		!typedSSAASCIIString(payload.kernel.literal) {
		return 0, false
	}
	return int64(core.IntegerToSLengthRawBuiltin(payload.inputs[index]) + len(payload.kernel.literal)), true
}

func (payload *typedIntegerStringMapLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= len(payload.inputs) {
		return nil, false
	}
	if payload.lengthResult {
		// Integer results are immutable and the runtime's integer identity is
		// value-based, so a lazy length result does not need one cached pointer
		// per index. This keeps the deferred result to the raw input snapshot.
		return core.NewSmallIntegerValue(int64(core.IntegerToSLengthRawBuiltin(payload.inputs[index]) + len(payload.kernel.literal))), true
	}
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, len(payload.inputs))
	}
	if value := payload.values[index]; value != nil {
		return value, true
	}
	raw := core.IntegerToSRawBuiltin(payload.inputs[index])
	switch payload.kernel.kind {
	case typedSSAIntegerStringConcatKernelPrefix:
		raw = payload.kernel.literal + raw
	case typedSSAIntegerStringConcatKernelSuffix:
		raw += payload.kernel.literal
	default:
		return nil, false
	}
	value := core.NewStringValue(raw)
	if payload.encoding != "" {
		value.Encoding = payload.encoding
	}
	payload.values[index] = value
	return value, true
}

func (payload *typedIntegerStringMapLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, len(payload.inputs))
	}
	for index := range payload.inputs {
		if _, ok := payload.valueAt(index); !ok {
			payload.values[index] = core.R.NilVal
		}
	}
	return payload.values
}

func (payload *typedIntegerStringMapLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

type typedObjectStringMapLazyPayload struct {
	integerInputs []int64
	stringInputs  []string
	suffix        string
	values        []*object.EmeraldValue
	batch         *core.StringValueBatch
}

func (payload *typedObjectStringMapLazyPayload) lazyLength() int {
	return payload.length()
}

func (payload *typedObjectStringMapLazyPayload) stringLengthAt(index int) (int64, bool) {
	if payload == nil || index < 0 || index >= payload.length() || !typedSSAASCIIString(payload.suffix) {
		return 0, false
	}
	if payload.integerInputs != nil {
		return int64(core.IntegerToSLengthRawBuiltin(payload.integerInputs[index]) + len(payload.suffix)), true
	}
	raw := payload.stringInputs[index]
	if !typedSSAASCIIString(raw) {
		return 0, false
	}
	return int64(len(raw) + len(payload.suffix)), true
}

func (payload *typedObjectStringMapLazyPayload) length() int {
	if payload == nil {
		return 0
	}
	if payload.integerInputs != nil {
		return len(payload.integerInputs)
	}
	return len(payload.stringInputs)
}

func (payload *typedObjectStringMapLazyPayload) valueAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length() {
		return nil, false
	}
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, payload.length())
	}
	if value := payload.values[index]; value != nil {
		return value, true
	}
	var raw string
	if payload.integerInputs != nil {
		raw = core.IntegerToSRawBuiltin(payload.integerInputs[index])
	} else {
		raw = payload.stringInputs[index]
	}
	if payload.suffix != "" {
		raw += payload.suffix
	}
	var value *object.EmeraldValue
	if payload.batch != nil {
		if payload.integerInputs != nil && payload.suffix == "" {
			value = payload.batch.NewInteger(payload.integerInputs[index])
		} else {
			left := raw
			if payload.suffix != "" {
				left = raw[:len(raw)-len(payload.suffix)]
			}
			value = payload.batch.NewConcat(left, payload.suffix)
		}
	} else {
		value = core.NewStringValue(raw)
	}
	payload.values[index] = value
	return value, value != nil
}

func (payload *typedObjectStringMapLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	length := payload.length()
	if payload.values == nil {
		payload.values = make([]*object.EmeraldValue, length)
	}
	if payload.batch == nil {
		payload.batch = core.NewStringValueBatch(length)
	}
	for index := 0; index < length; index++ {
		if payload.values[index] != nil {
			continue
		}
		if _, ok := payload.valueAt(index); !ok {
			payload.values[index] = core.R.NilVal
		}
	}
	return payload.values
}

func (payload *typedObjectStringMapLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	return payload.valueAt(index)
}

// tryExecuteArrayTypedSSAStringMapLazyResult admits only the direct captured
// helper shape whose callee is a pure Integer-to-ASCII-string concat kernel.
// It runs the complete input/type preflight first, then snapshots raw inputs
// and the kernel so later source-array mutation or method redefinition cannot
// change the already-computed map result.
func (vm *VM) tryExecuteArrayTypedSSAStringMapLazyResult(receiver, block *object.EmeraldValue, elems []*object.EmeraldValue, collect bool) (*object.EmeraldValue, bool) {
	if vm == nil || !typedSSAStringMapLazyResultEnabled || !collect || receiver == nil || block == nil ||
		len(elems) < typedSSABatchCallMinElements || core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	shape, ok := vm.prepareTypedSSABatchCallShape(block)
	if !ok || shape.send == nil || shape.send.argc != 1 || !shape.directSelfParam && !shape.directFreeParam {
		return nil, false
	}
	var calleeReceiver *object.EmeraldValue
	if shape.directSelfParam {
		calleeReceiver = shape.self
	} else {
		if int(shape.directFreeIndex) >= len(shape.free) {
			return nil, false
		}
		calleeReceiver = derefClosureValue(shape.free[shape.directFreeIndex])
	}
	if calleeReceiver == nil {
		return nil, false
	}
	method, _, fallback := vm.lookupMethodForSend(calleeReceiver, shape.send.name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil || method.Ruby2Keywords || methodUsesRefinements(method) ||
		(method.Visibility != "" && method.Visibility != "public" && !(method.Visibility == "private" && calleeReceiver == shape.self)) {
		return nil, false
	}
	calleeFn, ok := method.Fn.(*object.Function)
	if !ok || calleeFn == nil || len(calleeFn.Params) != 1 || len(calleeFn.ParamLocalIndices) != 1 ||
		calleeFn.HasRestParam || calleeFn.HasBlockParam || len(calleeFn.KeywordParams) != 0 || calleeFn.KeywordRestParam != "" ||
		calleeFn.KeywordRestOnly || calleeFn.RejectKeywords || calleeFn.RejectBlock || registerIRFunctionNeedsDefaultEvaluation(calleeFn, 1) {
		return nil, false
	}
	calleePlan, ok := vm.cachedTypedSSAPlan(calleeFn)
	if !ok || calleePlan == nil || calleePlan.blockReturn || calleePlan.hasReference || calleePlan.hasFloat || calleePlan.hasYield ||
		calleePlan.hasInstanceStore || calleePlan.integerStringConcatKernel.kind == typedSSAIntegerStringConcatKernelNone ||
		!typedSSAPlanASCIIStringLiterals(calleePlan) || !vm.typedSSAUnboxedPlanGuardsAvailable(calleePlan) ||
		!core.IntegerToSUsesBuiltinImplementation() || !core.StringPlusUsesBuiltinImplementation() {
		return nil, false
	}
	integerClass := core.R.IntegerClass
	if integerClass == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	inputs := make([]int64, len(elems))
	for index, elem := range elems {
		if object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		value, exact := typedSSAExactIntegerValueForClass(elem, integerClass)
		if !exact || core.AttachedSingletonClass(elem) != nil {
			return nil, false
		}
		inputs[index] = value
	}
	payload := &typedIntegerStringMapLazyPayload{
		inputs: inputs, values: make([]*object.EmeraldValue, len(inputs)),
		kernel: calleePlan.integerStringConcatKernel, encoding: core.DefaultExternalEncoding(),
	}
	result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
	result.SetLazyArrayRegion(&object.LazyArrayRegion{
		Length: len(inputs), Payload: payload, Materialize: payload.materialize,
		ElementAt: payload.elementAt, MethodGeneration: generation,
	})
	core.RegisterLazyArrayRegion(result)
	core.LastBlockResult = nil
	return result, true
}

// lazyObjectArrayField returns the value that the proven constructor would
// have stored in one ivar without materializing the receiver object. The
// constructor plan admits only literals and the Array.new block index, so this
// lookup has no user code or dynamic dispatch.
func lazyObjectArrayField(payload *lazyObjectArrayPayload, index int64, name string) (*object.EmeraldValue, bool) {
	if payload == nil || payload.plan == nil || name == "" || index < 0 || index >= int64(payload.length) {
		return nil, false
	}
	for _, store := range payload.plan.constructor.stores {
		if store.name != name {
			continue
		}
		source := store.source
		if source.kind == constructorSourceParam {
			if int(source.index) >= payload.plan.argc {
				return nil, false
			}
			source = payload.plan.sources[payload.plan.args[source.index]]
		}
		switch source.kind {
		case constructorSourceParam:
			if source.index != 0 {
				return nil, false
			}
			return core.NewIntegerValue(index), true
		case constructorSourceLiteral:
			if source.value == nil {
				return core.R.NilVal, true
			}
			return source.value, true
		default:
			return nil, false
		}
	}
	return nil, false
}

type typedRepeatedValueLazyPayload struct {
	length int
	value  *object.EmeraldValue
}

func (payload *typedRepeatedValueLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil || payload.length <= 0 {
		return nil
	}
	values := make([]*object.EmeraldValue, payload.length)
	for index := range values {
		values[index] = payload.value
	}
	return values
}

func (payload *typedRepeatedValueLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length || payload.value == nil {
		return nil, false
	}
	return payload.value, true
}

func (vm *VM) newTypedRepeatedValueLazyArray(length int, value *object.EmeraldValue, generation uint64) (*object.EmeraldValue, bool) {
	if vm == nil || length <= 0 || value == nil {
		return nil, false
	}
	result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
	payload := &typedRepeatedValueLazyPayload{length: length, value: value}
	result.SetLazyArrayRegion(&object.LazyArrayRegion{
		Length: length, Payload: payload, Materialize: payload.materialize,
		ElementAt: payload.elementAt, MethodGeneration: generation,
	})
	core.RegisterLazyArrayRegion(result)
	return result, true
}

func lazyObjectArrayRepeatedField(payload *lazyObjectArrayPayload, name string) (*object.EmeraldValue, bool) {
	if payload == nil || payload.length <= 0 {
		return nil, false
	}
	first, firstOK := lazyObjectArrayField(payload, 0, name)
	last, lastOK := lazyObjectArrayField(payload, int64(payload.length-1), name)
	if !firstOK || !lastOK || first == nil || first != last {
		return nil, false
	}
	return first, true
}

func (vm *VM) lazyObjectArrayGetter(payload *lazyObjectArrayPayload, name string) (*object.Function, string, bool) {
	if vm == nil || payload == nil || payload.plan == nil || payload.plan.class == nil || name == "" {
		return nil, "", false
	}
	// lookupMethodForSend needs a normal ValueObject receiver to apply the
	// ordinary visibility/ancestor rules. This probe never enters user code and
	// is allocated once per lazy map, not once per element.
	probeObject := &object.Object{Class: payload.plan.class}
	probe := &object.EmeraldValue{Type: object.ValueObject, Data: probeObject, Class: payload.plan.class}
	method, _, fallback := vm.lookupMethodForSend(probe, name, nil, false, true)
	if fallback != nil || method == nil || method.DispatchOwner != nil ||
		(method.Visibility != "" && method.Visibility != "public") || method.Ruby2Keywords || methodUsesRefinements(method) {
		return nil, "", false
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil || len(fn.Params) != 0 || len(fn.ParamLocalIndices) != 0 || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.RejectKeywords || fn.RejectBlock || registerIRFunctionNeedsDefaultEvaluation(fn, 0) {
		return nil, "", false
	}
	plan, ok := vm.cachedTypedSSAPlan(fn)
	if !ok || plan == nil || plan.hasInstanceStore || plan.hasYield {
		return nil, "", false
	}
	ivar, ok := typedSSASimpleGetterIvar(plan)
	if !ok || ivar == "" {
		return nil, "", false
	}
	return fn, ivar, true
}

// tryExecuteLazyObjectArrayMap consumes the common immediate map after
// `Array.new(n) { Box.new(...) }` directly from constructor fields. It is
// intentionally limited to the same pure getter/to_s/concat shapes already
// admitted by typed SSA; every other map first materializes the exact Ruby
// objects and re-enters the established batch paths.
func (vm *VM) tryExecuteLazyObjectArrayMap(receiver, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || block == nil || block.Type != object.ValueClosure {
		return nil, false
	}
	region := receiver.LazyArrayRegionValue()
	if region == nil {
		return nil, false
	}
	if region.MethodGeneration != object.CurrentMethodGeneration() {
		return nil, false
	}
	payload, ok := region.Payload.(*lazyObjectArrayPayload)
	if !ok || payload == nil || payload.plan == nil || payload.length < typedSSABatchCallMinElements {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 || closureUsesRefinements(closure) {
		return nil, false
	}
	fn := closure.Fn
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || !simpleBlockParameterPatterns(fn) {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	if closure.ReturnOwnerID > 0 {
		leaf, leafOK := vm.cachedBlockLeafPlan(fn)
		if !leafOK || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil || leaf.registerIR.hasExplicitReturn {
			return nil, false
		}
	}
	kind := 0 // simple getter
	getterName := ""
	suffix := ""
	lengthResult := false
	integerResult := false
	shape, shapeOK := vm.prepareTypedSSABatchCallShape(block)
	if shapeOK && shape.directParam && shape.send != nil && shape.send.argc == 0 {
		getterName = shape.send.name
	} else {
		callbackPlan, planOK := vm.cachedTypedSSABlockPlan(fn)
		if !planOK || callbackPlan == nil || !callbackPlan.blockReturn || callbackPlan.hasYield || callbackPlan.hasInstanceStore {
			return nil, false
		}
		if integerGetter, matched := typedSSAObjectIntegerToStringShape(callbackPlan, fn); matched {
			kind, getterName, integerResult = 1, integerGetter, true
		} else if integerGetter, matched := typedSSAObjectIntegerToStringLengthShape(callbackPlan, fn); matched {
			kind, getterName, integerResult, lengthResult = 2, integerGetter, true, true
		} else if concatGetter, concatSuffix, matched := typedSSAObjectStringConcatShape(callbackPlan, fn); matched {
			kind, getterName, suffix = 3, concatGetter, concatSuffix
		} else if concatGetter, concatSuffix, matched := typedSSAObjectStringConcatLengthShape(callbackPlan, fn); matched {
			kind, getterName, suffix, lengthResult = 4, concatGetter, concatSuffix, true
		} else {
			return nil, false
		}
	}

	_, getterIvar, getterOK := vm.lazyObjectArrayGetter(payload, getterName)
	if !getterOK {
		return nil, false
	}
	if integerResult && !core.IntegerToSUsesBuiltinImplementation() {
		return nil, false
	}
	if kind >= 3 && !core.StringPlusUsesBuiltinImplementation() {
		return nil, false
	}
	if lengthResult && !core.StringLengthUsesBuiltinImplementation() {
		return nil, false
	}
	if typedSSARepeatedValueLazyResultEnabled {
		// A literal field has the same immutable value for every proven
		// constructor instance. Keep the mapped Array lazy instead of
		// allocating a pointer slice for each repeated map; constructor-param
		// fields are rejected by the endpoint identity check because they vary
		// with the Array.new block index.
		if repeated, repeatedOK := lazyObjectArrayRepeatedField(payload, getterIvar); repeatedOK {
			var repeatedResult *object.EmeraldValue
			switch kind {
			case 0:
				if repeated.Type == object.ValueInteger {
					repeatedResult, _ = vm.newTypedRepeatedValueLazyArray(payload.length, repeated, region.MethodGeneration)
				}
			case 2:
				if repeated.Type == object.ValueInteger {
					integer, integerOK := typedSSAExactIntegerValueForClass(repeated, core.R.Classes["Integer"])
					if integerOK {
						repeatedResult, _ = vm.newTypedRepeatedValueLazyArray(payload.length,
							core.NewIntegerValue(int64(core.IntegerToSLengthRawBuiltin(integer))), region.MethodGeneration)
					}
				}
			case 4:
				if repeated.Type == object.ValueString {
					raw, stringOK := typedSSAExactStringValue(repeated)
					if stringOK && typedSSAASCIIString(raw) && typedSSAASCIIString(suffix) {
						repeatedResult, _ = vm.newTypedRepeatedValueLazyArray(payload.length,
							core.NewIntegerValue(int64(len(raw)+len(suffix))), region.MethodGeneration)
					}
				}
			}
			if repeatedResult != nil {
				core.LastBlockResult = nil
				return repeatedResult, true
			}
		}
	}
	if (kind == 1 || kind == 3) && typedSSAStringMapLazyResultEnabled {
		if kind == 1 {
			inputs := make([]int64, payload.length)
			integerClass := core.R.Classes["Integer"]
			if integerClass == nil {
				return nil, false
			}
			for index := range inputs {
				field, fieldOK := lazyObjectArrayField(payload, int64(index), getterIvar)
				integer, integerOK := typedSSAExactIntegerValueForClass(field, integerClass)
				if !fieldOK || !integerOK {
					return nil, false
				}
				inputs[index] = integer
			}
			stringPayload := &typedObjectStringMapLazyPayload{integerInputs: inputs}
			result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
			result.SetLazyArrayRegion(&object.LazyArrayRegion{
				Length: len(inputs), Payload: stringPayload, Materialize: stringPayload.materialize,
				ElementAt: stringPayload.elementAt, MethodGeneration: region.MethodGeneration,
			})
			core.RegisterLazyArrayRegion(result)
			core.LastBlockResult = nil
			return result, true
		}

		inputs := make([]string, payload.length)
		for index := range inputs {
			field, fieldOK := lazyObjectArrayField(payload, int64(index), getterIvar)
			raw, stringOK := typedSSAExactStringValue(field)
			if !fieldOK || !stringOK {
				return nil, false
			}
			inputs[index] = raw
		}
		stringPayload := &typedObjectStringMapLazyPayload{stringInputs: inputs, suffix: suffix}
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length: len(inputs), Payload: stringPayload, Materialize: stringPayload.materialize,
			ElementAt: stringPayload.elementAt, MethodGeneration: region.MethodGeneration,
		})
		core.RegisterLazyArrayRegion(result)
		core.LastBlockResult = nil
		return result, true
	}

	results := make([]*object.EmeraldValue, 0, payload.length)
	var stringBatch *core.StringValueBatch
	if integerResult && !lengthResult {
		stringBatch = core.NewStringValueBatch(payload.length)
	} else if kind == 3 {
		stringBatch = core.NewStringValueBatchWithByteCapacity(payload.length, payload.length*(len(suffix)+8))
	}
	for index := int64(0); index < int64(payload.length); index++ {
		value, valueOK := lazyObjectArrayField(payload, index, getterIvar)
		if !valueOK {
			return nil, false
		}
		switch kind {
		case 0:
			results = append(results, value)
		case 1, 2:
			integer, integerOK := typedSSAExactIntegerValueForClass(value, core.R.Classes["Integer"])
			if !integerOK {
				return nil, false
			}
			if lengthResult {
				results = append(results, core.NewIntegerValue(int64(core.IntegerToSLengthRawBuiltin(integer))))
			} else {
				results = append(results, stringBatch.NewInteger(integer))
			}
		case 3, 4:
			raw, stringOK := typedSSAExactStringValue(value)
			if !stringOK {
				return nil, false
			}
			if lengthResult {
				// String#+ is immediately consumed by builtin length. For the
				// ASCII/default case the result's character count is additive;
				// avoid allocating the intermediate concatenated Go string.
				if !typedSSAASCIIString(raw) || !typedSSAASCIIString(suffix) {
					return nil, false
				}
				results = append(results, core.NewIntegerValue(int64(len(raw)+len(suffix))))
			} else {
				raw += suffix
				results = append(results, stringBatch.New(raw))
			}
		}
	}
	core.LastBlockResult = nil
	return &object.EmeraldValue{Type: object.ValueArray, Data: results, Class: core.R.Classes["Array"]}, true
}
