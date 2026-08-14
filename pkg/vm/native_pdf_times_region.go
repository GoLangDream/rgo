package vm

import (
	"math/big"
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// nativePDFRenderTimesRegionEnabled enables the typed caller region for the
// common hot loop `total += document.render.bytesize`. The loop body is
// admitted from its complete Register IR graph, while the render leaf keeps
// the existing Prawn/PDF object-layout and mutation guards.
var nativePDFRenderTimesRegionEnabled = os.Getenv("RGO_DISABLE_NATIVE_PDF_RENDER_TIMES_REGION") == ""

const nativePDFRenderTimesRegionMinIterations int64 = 1024

type nativePDFRenderTimesBlockShape struct {
	pdfFree       uint8
	totalFree     uint8
	hasIndexParam bool
}

// nativePDFRenderTimesBlockShapeFor recognizes only the straight-line
// captured callback emitted for `total += document.render.bytesize`:
//
//	GetFree(total), GetFree(document), Send(render), Send(bytesize),
//	Add, SetFree(total), BlockReturn
//
// The data-flow checks are intentionally based on Register IR registers rather
// than bytecode positions. No user operation is reordered: the native loop
// only replaces the two already-proven builtin sends and the integer add.
func nativePDFRenderTimesBlockShapeFor(fn *object.Function, plan *registerIRPlan) (nativePDFRenderTimesBlockShape, bool) {
	invalid := nativePDFRenderTimesBlockShape{}
	if fn == nil || plan == nil || len(plan.instructions) != 7 || plan.sendCount != 2 ||
		!plan.blockReturn || plan.hasBranches || plan.hasImplicitSends || plan.hasExplicitReturn ||
		len(fn.Params) > 1 || len(fn.ParamLocalIndices) != len(fn.Params) || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) {
		return invalid, false
	}
	instructions := plan.instructions
	totalLoad, documentLoad := instructions[0], instructions[1]
	renderSend, bytesizeSend := instructions[2], instructions[3]
	add, store, result := instructions[4], instructions[5], instructions[6]
	if totalLoad.op != registerIRLoadFree || documentLoad.op != registerIRLoadFree ||
		totalLoad.param == documentLoad.param || renderSend.op != registerIRSend ||
		bytesizeSend.op != registerIRSend || add.op != registerIRBinary || add.opcode != compiler.OpAdd ||
		store.op != registerIRStoreFree || result.op != registerIRReturn || result.explicitReturn {
		return invalid, false
	}
	if renderSend.name != "render" || renderSend.argc != 0 || renderSend.left != documentLoad.dst ||
		renderSend.opcode != compiler.OpSend || renderSend.blockPresent || renderSend.splatIndex != 255 || renderSend.implicit ||
		bytesizeSend.name != "bytesize" || bytesizeSend.argc != 0 || bytesizeSend.left != renderSend.dst ||
		bytesizeSend.opcode != compiler.OpSend || bytesizeSend.blockPresent || bytesizeSend.splatIndex != 255 || bytesizeSend.implicit {
		return invalid, false
	}
	if add.left != totalLoad.dst || add.right != bytesizeSend.dst || add.dst != totalLoad.dst ||
		store.param != totalLoad.param || store.left != add.dst || result.left != add.dst {
		return invalid, false
	}
	return nativePDFRenderTimesBlockShape{pdfFree: documentLoad.param, totalFree: totalLoad.param, hasIndexParam: len(fn.Params) == 1}, true
}

func nativePDFRenderTimesExactInteger(value *object.EmeraldValue) (int64, bool) {
	if value == nil || value.Type != object.ValueInteger || value.Class != core.R.Classes["Integer"] ||
		value.BigIntValue() != nil || core.AttachedSingletonClass(value) != nil {
		return 0, false
	}
	integer, ok := value.Data.(int64)
	return integer, ok
}

// tryExecuteIntegerTimesNativePDFRender replaces only the repeated callback
// protocol around an already guarded Prawn/PDF render region. It keeps the
// captured document and total as Ruby-visible values, but carries the running
// total as raw int64 until the next observable boundary. Method/constant
// generation, class-extension, object identity and builtin-plus/bytesize
// guards are checked before each iteration; a miss replays the unexecuted
// suffix through the original block.
func (vm *VM) tryExecuteIntegerTimesNativePDFRender(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFRenderTimesRegionEnabled || !nativePDFObjectEnabled || !nativePrawnRenderFastEnabled || !nativePDFRendererRenderRegionEnabled ||
		!registerIRBatchBlockEnabled || receiver == nil || receiver.Type != object.ValueInteger ||
		receiver.Class != core.R.Classes["Integer"] || core.AttachedSingletonClass(receiver) != nil ||
		count < nativePDFRenderTimesRegionMinIterations || block == nil || block.Type != object.ValueClosure ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.threadDepth > 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 ||
		vm.ensureActive || vm.pendingReturnTargetID > 0 || vm.pendingBreakTargetID > 0 {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 ||
		closureUsesRefinements(closure) || closure.AutoSplat && blockWantsDestructuring(closure.Fn) {
		return nil, false
	}
	fn := closure.Fn
	leaf, found := vm.cachedBlockLeafPlan(fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil {
		return nil, false
	}
	shape, ok := nativePDFRenderTimesBlockShapeFor(fn, leaf.registerIR)
	if !ok || int(shape.pdfFree) >= len(closure.Free) || int(shape.totalFree) >= len(closure.Free) {
		return nil, false
	}
	document := derefClosureValue(closure.Free[shape.pdfFree])
	if document == nil || document.Type != object.ValueObject || document.Frozen || core.AttachedSingletonClass(document) != nil {
		return nil, false
	}
	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || document.Class != documentClass || !nativePrawnClassExtensionsEmpty(documentClass) {
		return nil, false
	}
	renderMethod, owner, found := documentClass.GetMethodWithOwner("render")
	if !found || renderMethod == nil || owner != documentClass || renderMethod.DispatchOwner != nil ||
		renderMethod.Visibility != "" && renderMethod.Visibility != "public" {
		return nil, false
	}
	renderFn, ok := renderMethod.Fn.(*object.Function)
	if !ok || renderFn == nil || renderFn.Name != "render" || renderFn.SourcePath == "" ||
		!nativePrawnExactMethodSource(documentClass, "render", "/prawn/document.rb") ||
		!core.IntegerPlusUsesBuiltinImplementation() || !core.StringBytesizeUsesBuiltinImplementation() {
		return nil, false
	}
	total, ok := nativePDFRenderTimesExactInteger(derefClosureValue(closure.Free[shape.totalFree]))
	if !ok {
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
	renderer, rendererMethod, targetOK := vm.nativePrawnRenderFastTarget(renderMethod, document, nil, false)
	if !targetOK {
		cleanup()
		return nil, false
	}
	state := core.DynamicInstanceVar(document, "@state")
	pages := core.DynamicInstanceVar(state, "@pages")
	pageItems, pagesOK := nativePDFArrayItems(pages)
	if !pagesOK || pages == nil || pages.Type != object.ValueArray || pages.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(pages) != nil || len(pageItems) == 0 {
		cleanup()
		return nil, false
	}
	pageCount := int64(len(pageItems))
	totalSlot := &closure.Free[shape.totalFree]
	setTotal := func(value *object.EmeraldValue) {
		setClosureValue(totalSlot, value)
	}
	fallback := func(start int64) (*object.EmeraldValue, bool) {
		setTotal(core.NewIntegerValue(total))
		cleanup()
		var args [1]*object.EmeraldValue
		for index := start; index < count; index++ {
			var fallbackArgs []*object.EmeraldValue
			if shape.hasIndexParam {
				args[0] = core.NewIntegerValue(index)
				fallbackArgs = args[:]
			}
			core.LastBlockResult = nil
			result := vm.callBlockWithSelfArgs(block, self, fallbackArgs)
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
	sideException := func(result *object.EmeraldValue) (*object.EmeraldValue, bool) {
		setTotal(core.NewIntegerValue(total))
		cleanup()
		return result, true
	}
	generation := object.CurrentMethodGeneration()
	constantGeneration := object.CurrentConstantGeneration()
	core.LastBlockResult = nil
	for index := int64(0); index < count; index++ {
		if object.CurrentMethodGeneration() != generation || object.CurrentConstantGeneration() != constantGeneration ||
			!core.IntegerPlusUsesBuiltinImplementation() || !core.StringBytesizeUsesBuiltinImplementation() ||
			!nativePrawnClassExtensionsEmpty(documentClass) || derefClosureValue(closure.Free[shape.pdfFree]) != document {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		if index > 0 {
			if result := nativePDFRenderSetCachedBookkeeping(document, "@page_number", core.NewIntegerValue(pageCount)); result != nil {
				return sideException(result)
			}
		}
		bytes, executed := vm.executeNativePDFRendererRenderRegion(rendererMethod, renderer, nil)
		if !executed {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		if bytes != nil && bytes.Type == object.ValueException {
			return sideException(bytes)
		}
		if bytes == nil || bytes.Type != object.ValueString || bytes.Class != core.R.Classes["String"] ||
			core.AttachedSingletonClass(bytes) != nil {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		raw, rawOK := bytes.Data.(string)
		if !rawOK {
			if index == 0 {
				cleanup()
				return nil, false
			}
			return fallback(index)
		}
		updated, fits := checkedIntegerAdd(total, int64(len(raw)))
		if !fits {
			bigTotal := new(big.Int).Add(big.NewInt(total), big.NewInt(int64(len(raw))))
			setTotal(core.NewIntegerFromBigInt(bigTotal))
			cleanup()
			for rest := index + 1; rest < count; rest++ {
				var fallbackArgs []*object.EmeraldValue
				if shape.hasIndexParam {
					args := [1]*object.EmeraldValue{core.NewIntegerValue(rest)}
					fallbackArgs = args[:]
				}
				core.LastBlockResult = nil
				result := vm.callBlockWithSelfArgs(block, self, fallbackArgs)
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
		total = updated
	}
	setTotal(core.NewIntegerValue(total))
	cleanup()
	core.LastBlockResult = nil
	return receiver, true
}
