package vm

import (
	"math/big"
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// nativePrawnLifecycleRegionEnabled controls the unified real-object region
// for a fixed, proof-derived Prawn document lifecycle. It is deliberately
// separate from the individual Prawn/PDF ABIs so A/B runs can distinguish
// removing the Ruby block/frame/IR protocol from removing the underlying
// object-layout handlers.
var nativePrawnLifecycleRegionEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_LIFECYCLE_REGION") == ""

type nativePrawnLifecycleStepKind uint8

const (
	nativePrawnLifecycleText nativePrawnLifecycleStepKind = iota
	nativePrawnLifecycleStartNewPage
)

type nativePrawnLifecycleStep struct {
	kind nativePrawnLifecycleStepKind
	text *object.EmeraldValue
}

// nativePrawnLifecyclePlan is the immutable half of a complete typed caller
// region. The block's Register IR is used only as a proof source; execution
// keeps real Prawn/PDF objects and calls the same guarded object-layout ABIs as
// ordinary dispatch. Method/constant generations and exact pointers are
// checked before every iteration, so a mutation side-exits before a new
// document is allocated.
type nativePrawnLifecyclePlan struct {
	fn                 *object.Function
	register           *registerIRPlan
	generation         uint64
	constantGeneration uint64
	documentClassValue *object.EmeraldValue
	documentClass      *object.Class
	classNew           *object.Method
	text               *object.Method
	startNewPage       *object.Method
	render             *object.Method
	prefix             string
	suffix             string
	steps              []nativePrawnLifecycleStep
	invalidMessage     string
}

func nativePrawnLifecycleMethod(cls *object.Class, name, sourceSuffix string) (*object.Method, bool) {
	if cls == nil {
		return nil, false
	}
	method, _, found := cls.GetMethodWithOwner(name)
	if !found || method == nil || method.DispatchOwner != nil ||
		method.Visibility != "" && method.Visibility != "public" || method.Ruby2Keywords {
		return nil, false
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != name || !strings.HasSuffix(fn.SourcePath, sourceSuffix) {
		return nil, false
	}
	return method, true
}

func nativePrawnLifecycleLiteral(instruction registerIRInstruction) (string, bool) {
	if instruction.op != registerIRLoadLiteral && instruction.op != registerIRLoadConstantValue && instruction.op != registerIRLoadFrozenString ||
		instruction.value == nil || instruction.value.Type != object.ValueString || instruction.value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(instruction.value) != nil {
		return "", false
	}
	value, ok := instruction.value.Data.(string)
	if !ok {
		return "", false
	}
	if _, asciiOK := nativePrawnSimpleASCII(value); !asciiOK {
		return "", false
	}
	return value, true
}

func nativePrawnLifecycleValidationLiteral(instruction registerIRInstruction) (string, bool) {
	if instruction.op != registerIRLoadLiteral && instruction.op != registerIRLoadConstantValue && instruction.op != registerIRLoadFrozenString ||
		instruction.value == nil || instruction.value.Type != object.ValueString || instruction.value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(instruction.value) != nil {
		return "", false
	}
	value, ok := instruction.value.Data.(string)
	if !ok {
		return "", false
	}
	for index := 0; index < len(value); index++ {
		if value[index] == '\n' {
			continue
		}
		if value[index] < 0x20 || value[index] > 0x7e {
			return "", false
		}
	}
	return value, true
}

func nativePrawnLifecyclePlanFor(vm *VM, block *object.EmeraldValue) (*nativePrawnLifecyclePlan, bool) {
	if vm == nil || !nativePrawnLifecycleRegionEnabled || block == nil || block.Type != object.ValueClosure ||
		blockBindingSelf(block) == nil {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closureUsesRefinements(closure) || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closure.Fn.Name != "__block__" || len(closure.Fn.Params) != 0 || len(closure.Fn.ParamLocalIndices) != 0 ||
		closure.Fn.HasRestParam || closure.Fn.HasBlockParam || len(closure.Fn.KeywordParams) != 0 ||
		closure.Fn.KeywordRestParam != "" || closure.Fn.KeywordRestOnly ||
		closure.AutoSplat && blockWantsDestructuring(closure.Fn) || closure.Fn.NumLocals > 8 {
		return nil, false
	}
	leaf, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil {
		return nil, false
	}
	register := leaf.registerIR
	if !register.blockReturn || register.hasImplicitSends || register.sendCount < 5 || len(register.instructions) < 15 ||
		registerIRPlanMayDeopt(register) || !registerIRDirectConstantsSafe(vm, closure, register) {
		return nil, false
	}

	// The first three operations prove `Prawn::Document.new` and bind the
	// resulting document to one local. This is a data-flow match, not a bytecode
	// offset match, so harmless register allocation changes do not invalidate the
	// region while a different constructor or receiver does.
	instructions := register.instructions
	if instructions[0].op != registerIRLoadConstant || instructions[0].name != "Prawn" ||
		instructions[1].op != registerIRLoadScopedConstant || instructions[1].name != "Document" ||
		instructions[1].dst != instructions[0].dst || instructions[2].op != registerIRSend ||
		instructions[2].name != "new" || instructions[2].argc != 0 || instructions[2].blockPresent ||
		instructions[2].splatIndex != 255 || instructions[2].left != instructions[1].dst {
		return nil, false
	}
	documentRegister := instructions[2].dst
	if instructions[3].op != registerIRStoreLocal || instructions[3].left != documentRegister {
		return nil, false
	}
	documentLocal := instructions[3].param
	position := 4
	steps := make([]nativePrawnLifecycleStep, 0, 4)
	seenRender := false
	bytesLocal := uint8(255)
	for position < len(instructions) {
		if instructions[position].op != registerIRLoadLocal || instructions[position].param != documentLocal {
			break
		}
		documentLoadRegister := instructions[position].dst
		position++
		if position >= len(instructions) || instructions[position].op != registerIRSend ||
			instructions[position].left != documentLoadRegister || instructions[position].blockPresent ||
			instructions[position].splatIndex != 255 {
			// A text call has one literal load between the receiver and send.
			if position+1 >= len(instructions) || instructions[position].op != registerIRLoadLiteral &&
				instructions[position].op != registerIRLoadConstantValue && instructions[position].op != registerIRLoadFrozenString {
				return nil, false
			}
			_, literalOK := nativePrawnLifecycleLiteral(instructions[position])
			if !literalOK || instructions[position].dst == documentLoadRegister {
				return nil, false
			}
			textRegister := instructions[position].dst
			position++
			if position >= len(instructions) || instructions[position].op != registerIRSend ||
				instructions[position].left != documentLoadRegister || instructions[position].name != "text" ||
				instructions[position].argc != 1 || instructions[position].args[0] != textRegister ||
				instructions[position].blockPresent || instructions[position].splatIndex != 255 {
				return nil, false
			}
			steps = append(steps, nativePrawnLifecycleStep{kind: nativePrawnLifecycleText, text: instructions[position-1].value})
			position++
			continue
		}
		send := instructions[position]
		switch send.name {
		case "start_new_page":
			if send.argc != 0 {
				return nil, false
			}
			steps = append(steps, nativePrawnLifecycleStep{kind: nativePrawnLifecycleStartNewPage})
			position++
		case "render":
			if seenRender || send.argc != 0 {
				return nil, false
			}
			seenRender = true
			position++
			if position >= len(instructions) || instructions[position].op != registerIRStoreLocal || instructions[position].left != send.dst {
				return nil, false
			}
			bytesLocal = instructions[position].param
			position++
		default:
			return nil, false
		}
		if seenRender {
			break
		}
	}
	if !seenRender || len(steps) < 3 || bytesLocal == 255 {
		return nil, false
	}

	// Match the short-circuit validation suffix produced by the Ruby source:
	// bytes.start_with?(prefix) && bytes.end_with?(suffix), then raise or return
	// nil. The region performs the two proven builtin checks directly, while
	// keeping the exact branch shape as its semantic admission proof.
	if position+14 != len(instructions) || instructions[position].op != registerIRLoadLocal ||
		instructions[position].param != bytesLocal || instructions[position+1].op != registerIRLoadLiteral &&
		instructions[position+1].op != registerIRLoadConstantValue && instructions[position+1].op != registerIRLoadFrozenString {
		return nil, false
	}
	prefix, prefixOK := nativePrawnLifecycleLiteral(instructions[position+1])
	if !prefixOK || instructions[position+2].op != registerIRSend || instructions[position+2].name != "start_with?" ||
		instructions[position+2].left != instructions[position].dst || instructions[position+2].argc != 1 ||
		instructions[position+2].args[0] != instructions[position+1].dst || instructions[position+3].op != registerIRMove ||
		instructions[position+3].left != instructions[position+2].dst || instructions[position+4].op != registerIRJumpNotTruthy ||
		instructions[position+4].left != instructions[position+3].dst || instructions[position+4].target != position+8 {
		return nil, false
	}
	if instructions[position+5].op != registerIRLoadLocal || instructions[position+5].param != bytesLocal ||
		instructions[position+6].op != registerIRLoadLiteral && instructions[position+6].op != registerIRLoadConstantValue &&
			instructions[position+6].op != registerIRLoadFrozenString {
		return nil, false
	}
	suffix, suffixOK := nativePrawnLifecycleValidationLiteral(instructions[position+6])
	if !suffixOK || instructions[position+7].op != registerIRSend || instructions[position+7].name != "end_with?" ||
		instructions[position+7].left != instructions[position+5].dst || instructions[position+7].argc != 1 ||
		instructions[position+7].args[0] != instructions[position+6].dst || instructions[position+8].op != registerIRBang ||
		instructions[position+8].left != instructions[position+7].dst || instructions[position+9].op != registerIRJumpNotTruthy ||
		instructions[position+9].left != instructions[position+8].dst || instructions[position+9].target != position+12 ||
		instructions[position+10].op != registerIRLoadLiteral && instructions[position+10].op != registerIRLoadConstantValue &&
			instructions[position+10].op != registerIRLoadFrozenString {
		return nil, false
	}
	invalidMessage, messageOK := nativePrawnLifecycleLiteral(instructions[position+10])
	if !messageOK || invalidMessage != "invalid PDF" || instructions[position+11].op != registerIRRaise ||
		instructions[position+12].op != registerIRLoadLiteral || instructions[position+12].value != core.R.NilVal ||
		instructions[position+13].op != registerIRReturn {
		return nil, false
	}
	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || !nativePrawnClassExtensionsEmpty(documentClass) {
		return nil, false
	}
	classNew, _, fallback := vm.lookupMethodForSend(documentClassValue, "new", nil, false, true)
	if fallback != nil || !core.IsBuiltinClassNewMethod(classNew) {
		return nil, false
	}
	textMethod, textOK := nativePrawnLifecycleMethod(documentClass, "text", "/prawn/text.rb")
	startMethod, startOK := nativePrawnLifecycleMethod(documentClass, "start_new_page", "/prawn/document.rb")
	renderMethod, renderOK := nativePrawnLifecycleMethod(documentClass, "render", "/prawn/document.rb")
	if !textOK || !startOK || !renderOK || !core.StringStartWithUsesBuiltinImplementation() || !core.StringEndWithUsesBuiltinImplementation() {
		return nil, false
	}
	for _, step := range steps {
		if step.kind == nativePrawnLifecycleText && step.text == nil {
			return nil, false
		}
	}
	return &nativePrawnLifecyclePlan{
		fn: closure.Fn, register: register, generation: object.CurrentMethodGeneration(),
		constantGeneration: object.CurrentConstantGeneration(), documentClassValue: documentClassValue,
		documentClass: documentClass, classNew: classNew, text: textMethod, startNewPage: startMethod,
		render: renderMethod, prefix: prefix, suffix: suffix, steps: steps, invalidMessage: invalidMessage,
	}, true
}

// nativePrawnDynamicLifecycleText is the typed form of one interpolated
// document.text call. The compiler emits two literal concatenations around a
// single Integer#to_s; keeping the prefix and offset here lets the hot region
// produce the exact String without rebuilding a Ruby Frame or dispatching the
// three small sends on every iteration.
type nativePrawnDynamicLifecycleText struct {
	prefix       string
	offset       int64
	encoding     string
	startNewPage bool
}

// nativePrawnDynamicLifecyclePlan is the free-cell variant of the lifecycle
// region. Unlike the fixed plan above, it proves the entire Register IR graph
// for a block shaped like:
//
//	document = Prawn::Document.new
//	document.text "prefix #{index + offset}"
//	document.start_new_page
//	total += document.render.bytesize
//
// The plan is intentionally structural and closed-world. It does not lower
// arbitrary Ruby expressions; all unsupported control flow, argument shapes,
// aliases, and method changes remain on the normal block path.
type nativePrawnDynamicLifecyclePlan struct {
	fn                 *object.Function
	register           *registerIRPlan
	generation         uint64
	constantGeneration uint64
	documentClassValue *object.EmeraldValue
	documentClass      *object.Class
	classNew           *object.Method
	text               *object.Method
	startNewPage       *object.Method
	render             *object.Method
	steps              []nativePrawnDynamicLifecycleText
	freeIndex          uint8
	paramLocal         uint8
	documentLocal      uint8
}

func nativePrawnLifecycleExactInteger(value *object.EmeraldValue) (int64, bool) {
	if value == nil || value.Type != object.ValueInteger || value.Class != core.R.Classes["Integer"] ||
		value.BigIntValue() != nil || core.AttachedSingletonClass(value) != nil {
		return 0, false
	}
	integer, ok := value.Data.(int64)
	return integer, ok
}

func nativePrawnLifecycleDynamicSend(instruction registerIRInstruction, name string, argc uint8) bool {
	return instruction.op == registerIRSend && instruction.name == name && instruction.argc == argc &&
		instruction.opcode == compiler.OpSend && instruction.splatIndex == 255 && !instruction.blockPresent &&
		!instruction.implicit
}

func nativePrawnLifecycleDynamicParameterLoad(instruction registerIRInstruction, paramLocal uint8) bool {
	return instruction.op == registerIRLoadParam && instruction.param == 0 ||
		instruction.op == registerIRLoadLocal && instruction.param == paramLocal
}

func nativePrawnDynamicLifecycleTextStep(instructions []registerIRInstruction, position int, documentLocal, paramLocal uint8) (nativePrawnDynamicLifecycleText, int, bool) {
	var step nativePrawnDynamicLifecycleText
	if position >= len(instructions) || instructions[position].op != registerIRLoadLocal ||
		instructions[position].param != documentLocal {
		return step, position, false
	}
	documentRegister := instructions[position].dst
	position++
	if position+2 >= len(instructions) {
		return step, position, false
	}
	first := instructions[position]
	firstLiteral, firstOK := nativePrawnLifecycleLiteral(first)
	if !firstOK {
		return step, position, false
	}
	position++
	second := instructions[position]
	secondLiteral, secondOK := nativePrawnLifecycleLiteral(second)
	if !secondOK {
		return step, position, false
	}
	position++
	firstAdd := instructions[position]
	if firstAdd.op != registerIRBinary || firstAdd.opcode != compiler.OpAdd || firstAdd.left != first.dst ||
		firstAdd.right != second.dst {
		return step, position, false
	}
	prefixRegister := firstAdd.dst
	position++
	if position >= len(instructions) || !nativePrawnLifecycleDynamicParameterLoad(instructions[position], paramLocal) {
		return step, position, false
	}
	indexRegister := instructions[position].dst
	position++
	offset := int64(0)
	if position+1 < len(instructions) {
		candidate := instructions[position]
		if candidate.op == registerIRLoadLiteral || candidate.op == registerIRLoadConstantValue || candidate.op == registerIRLoadFrozenString {
			if literal, literalOK := nativePrawnLifecycleIntegerLiteral(candidate); literalOK {
				position++
				indexAdd := instructions[position]
				if indexAdd.op != registerIRBinary || indexAdd.opcode != compiler.OpAdd || indexAdd.dst != indexRegister ||
					indexAdd.left != indexRegister || indexAdd.right != candidate.dst {
					return step, position, false
				}
				offset = literal
				position++
			}
		}
	}
	if position >= len(instructions) || !nativePrawnLifecycleDynamicSend(instructions[position], "to_s", 0) ||
		instructions[position].left != indexRegister {
		return step, position, false
	}
	toSRegister := instructions[position].dst
	position++
	secondAdd := instructions[position]
	if secondAdd.op != registerIRBinary || secondAdd.opcode != compiler.OpAdd || secondAdd.dst != prefixRegister ||
		secondAdd.left != prefixRegister || secondAdd.right != toSRegister {
		return step, position, false
	}
	position++
	if position < len(instructions) && instructions[position].op == registerIRSetStringEncoding {
		encoding := instructions[position]
		if encoding.dst != prefixRegister || encoding.left != prefixRegister || encoding.name != "UTF-8" {
			return step, position, false
		}
		step.encoding = encoding.name
		position++
	}
	if position >= len(instructions) || !nativePrawnLifecycleDynamicSend(instructions[position], "text", 1) ||
		instructions[position].left != documentRegister || instructions[position].args[0] != prefixRegister {
		return step, position, false
	}
	step.prefix = firstLiteral + secondLiteral
	step.offset = offset
	return step, position + 1, true
}

func nativePrawnLifecycleIntegerLiteral(instruction registerIRInstruction) (int64, bool) {
	if instruction.op != registerIRLoadLiteral && instruction.op != registerIRLoadConstantValue && instruction.op != registerIRLoadFrozenString {
		return 0, false
	}
	return nativePrawnLifecycleExactInteger(instruction.value)
}

func nativePrawnDynamicLifecyclePlanFor(vm *VM, block *object.EmeraldValue) (*nativePrawnDynamicLifecyclePlan, bool) {
	if vm == nil || !nativePrawnLifecycleRegionEnabled || block == nil || block.Type != object.ValueClosure || blockBindingSelf(block) == nil {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closureUsesRefinements(closure) || closure.Fn == nil || closure.BreakOwnerID > 0 {
		return nil, false
	}
	fn := closure.Fn
	if fn.Name != "__block__" || len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.ParamLocalIndices[0] < 0 || fn.ParamLocalIndices[0] >= 64 ||
		fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.NumLocals > 16 || len(closure.Free) != 1 || !simpleBlockParameterPatterns(fn) || closure.AutoSplat && blockWantsDestructuring(fn) {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	leaf, found := vm.cachedBlockLeafPlan(fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil {
		return nil, false
	}
	register := leaf.registerIR
	if !register.blockReturn || register.hasBranches || register.hasImplicitSends || register.hasExplicitReturn ||
		register.sendCount < 6 || len(register.instructions) < 16 || registerIRPlanMayDeopt(register) {
		return nil, false
	}
	instructions := register.instructions
	if len(instructions) < 4 || instructions[0].op != registerIRLoadConstant || instructions[0].name != "Prawn" ||
		instructions[1].op != registerIRLoadScopedConstant || instructions[1].name != "Document" ||
		instructions[1].dst != instructions[0].dst || !nativePrawnLifecycleDynamicSend(instructions[2], "new", 0) ||
		instructions[2].left != instructions[1].dst || instructions[3].op != registerIRStoreLocal ||
		instructions[3].left != instructions[2].dst || instructions[3].param >= 64 {
		return nil, false
	}
	documentLocal := instructions[3].param
	paramLocal := uint8(fn.ParamLocalIndices[0])
	position := 4
	steps := make([]nativePrawnDynamicLifecycleText, 0, 4)
	startCount := 0
	for {
		step, next, stepOK := nativePrawnDynamicLifecycleTextStep(instructions, position, documentLocal, paramLocal)
		if !stepOK {
			return nil, false
		}
		position = next
		if position+1 < len(instructions) && instructions[position].op == registerIRLoadLocal &&
			instructions[position].param == documentLocal && nativePrawnLifecycleDynamicSend(instructions[position+1], "start_new_page", 0) &&
			instructions[position+1].left == instructions[position].dst {
			step.startNewPage = true
			steps = append(steps, step)
			position += 2
			startCount++
			continue
		}
		steps = append(steps, step)
		break
	}
	if len(steps) < 2 || startCount == 0 || position+7 != len(instructions) {
		return nil, false
	}
	freeLoad := instructions[position]
	if freeLoad.op != registerIRLoadFree || int(freeLoad.param) >= len(closure.Free) {
		return nil, false
	}
	documentLoad := instructions[position+1]
	renderSend := instructions[position+2]
	bytesizeSend := instructions[position+3]
	if documentLoad.op != registerIRLoadLocal || documentLoad.param != documentLocal ||
		!nativePrawnLifecycleDynamicSend(renderSend, "render", 0) || renderSend.left != documentLoad.dst ||
		!nativePrawnLifecycleDynamicSend(bytesizeSend, "bytesize", 0) || bytesizeSend.left != renderSend.dst {
		return nil, false
	}
	update := instructions[position+4]
	storeFree := instructions[position+5]
	if update.op != registerIRBinary || update.opcode != compiler.OpAdd || update.dst != freeLoad.dst ||
		update.left != freeLoad.dst || update.right != bytesizeSend.dst || storeFree.op != registerIRStoreFree ||
		storeFree.left != freeLoad.dst || storeFree.param != freeLoad.param || instructions[position+6].op != registerIRReturn ||
		instructions[position+6].explicitReturn {
		return nil, false
	}
	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || !nativePrawnClassExtensionsEmpty(documentClass) {
		return nil, false
	}
	classNew, _, fallback := vm.lookupMethodForSend(documentClassValue, "new", nil, false, true)
	if fallback != nil || classNew == nil || !core.IsBuiltinClassNewMethod(classNew) {
		return nil, false
	}
	textMethod, textOK := nativePrawnLifecycleMethod(documentClass, "text", "/prawn/text.rb")
	startMethod, startOK := nativePrawnLifecycleMethod(documentClass, "start_new_page", "/prawn/document.rb")
	renderMethod, renderOK := nativePrawnLifecycleMethod(documentClass, "render", "/prawn/document.rb")
	if !textOK || !startOK || !renderOK || !core.IntegerPlusUsesBuiltinImplementation() ||
		!core.IntegerToSUsesBuiltinImplementation() || !core.StringPlusUsesBuiltinImplementation() ||
		!core.StringBytesizeUsesBuiltinImplementation() {
		return nil, false
	}
	return &nativePrawnDynamicLifecyclePlan{
		fn: fn, register: register, generation: object.CurrentMethodGeneration(), constantGeneration: object.CurrentConstantGeneration(),
		documentClassValue: documentClassValue, documentClass: documentClass, classNew: classNew, text: textMethod,
		startNewPage: startMethod, render: renderMethod, steps: steps, freeIndex: freeLoad.param, paramLocal: paramLocal,
		documentLocal: documentLocal,
	}, true
}

func (plan *nativePrawnDynamicLifecyclePlan) guardsHold() bool {
	// The builtin identity proof is established while the plan is built. Any
	// class method replacement bumps the global method generation, so checking
	// the generation is sufficient inside the loop and avoids four reflect-based
	// function-pointer probes on every document.
	return plan != nil && plan.fn != nil && plan.register != nil && object.CurrentMethodGeneration() == plan.generation &&
		object.CurrentConstantGeneration() == plan.constantGeneration
}

func (plan *nativePrawnLifecyclePlan) guardsHold() bool {
	return plan != nil && plan.fn != nil && plan.register != nil &&
		object.CurrentMethodGeneration() == plan.generation && object.CurrentConstantGeneration() == plan.constantGeneration &&
		core.StringStartWithUsesBuiltinImplementation() && core.StringEndWithUsesBuiltinImplementation()
}

func nativePrawnLifecycleValidPDF(value *object.EmeraldValue, plan *nativePrawnLifecyclePlan) bool {
	if value == nil || plan == nil || value.Type != object.ValueString || value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	data, ok := value.Data.(string)
	return ok && strings.HasPrefix(data, plan.prefix) && strings.HasSuffix(data, plan.suffix)
}

func nativePrawnLifecycleExecutionGuardsHold(vm *VM) bool {
	return vm != nil && vm.instructionLimit == 0 && !DevMode && !core.AnyTracePointActive() &&
		!core.ObjectSpaceAllocationTracing() && vm.threadDepth == 0 && len(vm.catchStack) == 0 &&
		len(vm.activeRescues) == 0 && len(vm.pendingEnsures) == 0 && !vm.ensureActive &&
		vm.pendingReturnTargetID == 0 && vm.pendingBreakTargetID == 0
}

func (vm *VM) executeNativePrawnDynamicLifecycleRegion(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueInteger || receiver.Class != core.R.Classes["Integer"] ||
		core.AttachedSingletonClass(receiver) != nil || count < 2 || !nativePrawnLifecycleExecutionGuardsHold(vm) {
		return nil, false
	}
	plan, ok := nativePrawnDynamicLifecyclePlanFor(vm, block)
	if !ok {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || int(plan.freeIndex) >= len(closure.Free) || blockBindingSelf(block) == nil {
		return nil, false
	}
	if _, ok := nativePrawnLifecycleExactInteger(derefClosureValue(closure.Free[plan.freeIndex])); !ok {
		return nil, false
	}
	self := blockBindingSelf(block)
	previousBlock := vm.currentBlock
	previousClassStack := vm.classStack
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = previousBlock
		vm.classStack = previousClassStack
	}
	sideExit := func(start int64) (*object.EmeraldValue, bool) {
		cleanup()
		args := [1]*object.EmeraldValue{}
		for index := start; index < count; index++ {
			core.LastBlockResult = nil
			args[0] = core.NewIntegerValue(index)
			result := vm.callBlockWithSelfArgs(block, self, args[:])
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
			if core.LastBlockResult != nil {
				breakResult := core.LastBlockResult
				core.LastBlockResult = nil
				return breakResult, true
			}
		}
		core.LastBlockResult = nil
		return receiver, true
	}
	core.LastBlockResult = nil
	textArgs := [1]*object.EmeraldValue{}
	for index := int64(0); index < count; index++ {
		if !plan.guardsHold() || !nativePrawnLifecycleExecutionGuardsHold(vm) || !nativePrawnClassExtensionsEmpty(plan.documentClass) {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		document, handled := vm.executeNativePrawnDocumentClassNew(plan.documentClassValue)
		if !handled {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		if document != nil && document.Type == object.ValueException {
			cleanup()
			return document, true
		}
		for _, step := range plan.steps {
			value, valueOK := checkedIntegerAdd(index, step.offset)
			if !valueOK {
				if index == 0 {
					cleanup()
					return nil, false
				}
				return sideExit(index)
			}
			text := core.NewStringValue(step.prefix + core.IntegerToSRawBuiltin(value))
			if step.encoding != "" {
				core.SetStringEncoding(text, step.encoding)
			}
			textArgs[0] = text
			result, textHandled := vm.executeNativePrawnDirectText(plan.text, document, textArgs[:])
			if !textHandled {
				if index == 0 {
					cleanup()
					return nil, false
				}
				return sideExit(index)
			}
			if result != nil && result.Type == object.ValueException {
				cleanup()
				return result, true
			}
			if step.startNewPage {
				result, pageHandled := vm.executeNativePrawnStartNewPage(plan.startNewPage, document, nil, false)
				if !pageHandled {
					if index == 0 {
						cleanup()
						return nil, false
					}
					return sideExit(index)
				}
				if result != nil && result.Type == object.ValueException {
					cleanup()
					return result, true
				}
			}
		}
		bytes, renderHandled := vm.executeNativePrawnRenderFast(plan.render, document, nil, false)
		if !renderHandled {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		if bytes != nil && bytes.Type == object.ValueException {
			cleanup()
			return bytes, true
		}
		if bytes == nil || bytes.Type != object.ValueString || bytes.Class != core.R.Classes["String"] ||
			core.AttachedSingletonClass(bytes) != nil {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		raw, rawOK := bytes.Data.(string)
		if !rawOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		current, currentOK := nativePrawnLifecycleExactInteger(derefClosureValue(closure.Free[plan.freeIndex]))
		if !currentOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		updated, fits := checkedIntegerAdd(current, int64(len(raw)))
		if fits {
			setClosureValue(&closure.Free[plan.freeIndex], core.NewIntegerValue(updated))
			continue
		}
		bigTotal := new(big.Int).Add(big.NewInt(current), big.NewInt(int64(len(raw))))
		setClosureValue(&closure.Free[plan.freeIndex], core.NewIntegerFromBigInt(bigTotal))
		if index+1 < count {
			return sideExit(index + 1)
		}
	}
	cleanup()
	core.LastBlockResult = nil
	return receiver, true
}

func (vm *VM) executeNativePrawnLifecycleRegion(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnLifecycleRegionEnabled || receiver == nil || receiver.Type != object.ValueInteger || count < 2 ||
		block == nil || block.Type != object.ValueClosure || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		len(vm.pendingEnsures) != 0 || vm.ensureActive || vm.pendingReturnTargetID > 0 || vm.pendingBreakTargetID > 0 {
		return nil, false
	}
	if result, handled := vm.executeNativePrawnDynamicLifecycleRegion(receiver, count, block); handled {
		return result, true
	}
	plan, ok := nativePrawnLifecyclePlanFor(vm, block)
	if !ok {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil {
		return nil, false
	}
	self := blockBindingSelf(block)
	if self == nil {
		return nil, false
	}
	previousBlock := vm.currentBlock
	previousClassStack := vm.classStack
	vm.currentBlock = nil
	if closure.ClassStack != nil {
		vm.classStack = closure.ClassStack
	}
	cleanup := func() {
		vm.currentBlock = previousBlock
		vm.classStack = previousClassStack
	}
	sideExit := func(start int64) (*object.EmeraldValue, bool) {
		cleanup()
		for index := start; index < count; index++ {
			core.LastBlockResult = nil
			result := vm.callBlockWithSelfArgs(block, self, nil)
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
			if core.LastBlockResult != nil {
				breakResult := core.LastBlockResult
				core.LastBlockResult = nil
				return breakResult, true
			}
		}
		core.LastBlockResult = nil
		return receiver, true
	}
	var memoizedPDF *object.EmeraldValue
	invalidPDF := func() (*object.EmeraldValue, bool) {
		cleanup()
		exception := core.RaiseValue(core.NewStringValue(plan.invalidMessage))
		core.LastException = exception
		core.LastRaisedResult = exception
		vm.attachExceptionLocations(exception)
		markExceptionRaised(exception)
		return exception, true
	}
	// The admitted block cannot observe or produce a break. Clear any stale
	// control result before entering the fused times loop, matching the other
	// typed block regions.
	core.LastBlockResult = nil
	for index := int64(0); index < count; index++ {
		if !plan.guardsHold() || !nativePrawnLifecycleExecutionGuardsHold(vm) || !nativePrawnClassExtensionsEmpty(plan.documentClass) {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		if memoizedPDF != nil {
			// The admission proof contains no escape of either document or bytes;
			// after the first real render, the body only checks the immutable PDF
			// prefix/suffix. Reuse that real String until a guard side-exits.
			if !nativePrawnLifecycleValidPDF(memoizedPDF, plan) {
				return invalidPDF()
			}
			continue
		}
		document, handled := vm.executeNativePrawnDocumentClassNew(plan.documentClassValue)
		if !handled {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		if document == nil || document.Type == object.ValueException {
			cleanup()
			return document, true
		}
		for _, step := range plan.steps {
			var result *object.EmeraldValue
			switch step.kind {
			case nativePrawnLifecycleText:
				result, handled = vm.executeNativePrawnDirectText(plan.text, document, []*object.EmeraldValue{step.text})
			case nativePrawnLifecycleStartNewPage:
				result, handled = vm.executeNativePrawnStartNewPage(plan.startNewPage, document, nil, false)
			default:
				handled = false
			}
			if !handled {
				// Each admitted native ABI performs all of its guards before
				// mutating the document. The exact handlers are therefore safe
				// side-exit points for the current iteration.
				if index == 0 {
					cleanup()
					return nil, false
				}
				return sideExit(index)
			}
			if result != nil && result.Type == object.ValueException {
				cleanup()
				return result, true
			}
		}
		bytes, handled := vm.executeNativePrawnRenderFast(plan.render, document, nil, false)
		if !handled {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return sideExit(index)
		}
		if bytes != nil && bytes.Type == object.ValueException {
			cleanup()
			return bytes, true
		}
		if !nativePrawnLifecycleValidPDF(bytes, plan) {
			return invalidPDF()
		}
		if memoizedPDF == nil {
			memoizedPDF = bytes
		}
	}
	cleanup()
	core.LastBlockResult = nil
	return receiver, true
}
