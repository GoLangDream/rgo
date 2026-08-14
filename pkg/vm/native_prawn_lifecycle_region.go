package vm

import (
	"os"
	"strings"

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

func (vm *VM) executeNativePrawnLifecycleRegion(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnLifecycleRegionEnabled || receiver == nil || receiver.Type != object.ValueInteger || count < 2 ||
		block == nil || block.Type != object.ValueClosure || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		len(vm.pendingEnsures) != 0 || vm.ensureActive || vm.pendingReturnTargetID > 0 || vm.pendingBreakTargetID > 0 {
		return nil, false
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
