package vm

import (
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// executeNativePDFObjectStoreEach removes the Ruby loop/frame around the
// exact PDF::Core::ObjectStore#each implementation.  The iterator itself is
// intentionally tiny (`@identifiers.each { |id| yield(@objects[id]) }`), but
// it is called for every render and used to pay a full closure protocol for
// every object.  A narrow one-argument Register IR callback can reuse a frame
// and receive values directly; all other callbacks still enter
// VM.callBlockOneExplicit, so user blocks, break/next and exceptions retain
// their normal semantics.
func (vm *VM) executeNativePDFObjectStoreEach(methodObj *object.Method, receiver *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "PDF::Core::ObjectStore" ||
		vm.currentBlock == nil || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "each" || !strings.HasSuffix(fn.SourcePath, "/object_store.rb") {
		return nil, false
	}
	identifiers := core.DynamicInstanceVar(receiver, "@identifiers")
	objects := core.DynamicInstanceVar(receiver, "@objects")
	if identifiers == nil || identifiers.Type != object.ValueArray || identifiers.Class != core.R.Classes["Array"] ||
		objects == nil || objects.Type != object.ValueHash || objects.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(identifiers) != nil || core.AttachedSingletonClass(objects) != nil {
		return nil, false
	}
	ids, ok := identifiers.Data.([]*object.EmeraldValue)
	if !ok {
		return nil, false
	}
	block := vm.currentBlock
	runFrom := func(start int) (*object.EmeraldValue, bool) {
		for index := start; index < len(ids); index++ {
			id := ids[index]
			value, handled := core.DirectHashIndex(objects, id)
			if !handled {
				previousBlock := vm.currentBlock
				vm.currentBlock = nil
				value = vm.send(objects, "[]", []*object.EmeraldValue{id})
				vm.currentBlock = previousBlock
				if value != nil && value.Type == object.ValueException {
					return value, true
				}
			}
			core.LastBlockResult = nil
			core.ForEachClearNext()
			result := vm.callBlockOneExplicit(block, value)
			if result != nil && result.Type == object.ValueException && core.LastException == result {
				return result, true
			}
			if core.LastBlockResult != nil {
				breakValue := core.LastBlockResult
				core.LastBlockResult = nil
				return breakValue, true
			}
			if core.ForEachConsumeNext() {
				continue
			}
		}
		core.LastBlockResult = nil
		return identifiers, true
	}
	if result, handled := vm.tryExecuteFramedIRBlockValueStream(block, len(ids), func(index int) (*object.EmeraldValue, bool) {
		return core.DirectHashIndex(objects, ids[index])
	}, runFrom); handled {
		if result != nil {
			if result == identifiers {
				return identifiers, true
			}
			return result, true
		}
		return identifiers, true
	}
	return runFrom(0)
}

// executeNativePDFObjectStoreIndex is the typed equivalent of
// PDF::Core::ObjectStore#[].  ObjectStore lookups are pure exact-Hash reads;
// a missing key returns Ruby nil through DirectHashIndex just as Hash#[] does.
func (vm *VM) executeNativePDFObjectStoreIndex(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "PDF::Core::ObjectStore" ||
		len(args) != 1 || args[0] == nil || args[0].Type != object.ValueInteger {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "[]" || !strings.HasSuffix(fn.SourcePath, "/object_store.rb") {
		return nil, false
	}
	objects := core.DynamicInstanceVar(receiver, "@objects")
	if objects == nil || objects.Type != object.ValueHash || objects.Class != core.R.Classes["Hash"] {
		return nil, false
	}
	value, handled := core.DirectHashIndex(objects, args[0])
	if !handled {
		return nil, false
	}
	return value, true
}

// executeNativePDFRendererGraphicState handles the steady-state reader used
// by add_content and text drawing.  When the stack is empty the Ruby method
// must call save_graphics_state, so that case deopts to preserve the side
// effect and exception protocol.
func (vm *VM) executeNativePDFRendererGraphicState(methodObj *object.Method, receiver *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "PDF::Core::Renderer" {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "graphic_state" || !strings.HasSuffix(fn.SourcePath, "/renderer.rb") {
		return nil, false
	}
	state := core.DynamicInstanceVar(receiver, "@state")
	page := core.DynamicInstanceVar(state, "@page")
	if !nativePDFExactObject(page, "PDF::Core::Page") {
		return nil, false
	}
	stack := core.DynamicInstanceVar(page, "@stack")
	if !nativePDFExactObject(stack, "PDF::Core::GraphicStateStack") {
		return nil, false
	}
	values := core.DynamicInstanceVar(stack, "@stack")
	if values == nil || values.Type != object.ValueArray || values.Class != core.R.Classes["Array"] {
		return nil, false
	}
	items, ok := values.Data.([]*object.EmeraldValue)
	if !ok || len(items) == 0 || items[len(items)-1] == nil || items[len(items)-1].Type == object.ValueNil {
		return nil, false
	}
	return items[len(items)-1], true
}
