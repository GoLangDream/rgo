package vm

import (
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// executeNativePrawnRendererReader handles the warm half of
// Prawn::Document#renderer.  The cold nil case intentionally falls through so
// Ruby still performs Renderer.new and memoization exactly once; all later
// calls are a typed ivar read.
func (vm *VM) executeNativePrawnRendererReader(methodObj *object.Method, receiver *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" {
		return nil, false
	}
	if methodObj.DispatchOwner != nil {
		ownerClass, ownerOK := methodObj.DispatchOwner.Data.(*object.Class)
		if methodObj.DispatchOwner.Type != object.ValueClass || !ownerOK || ownerClass == nil || ownerClass.Name != "Prawn::Document" {
			return nil, false
		}
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "renderer" || !strings.HasSuffix(fn.SourcePath, "/prawn/document/internals.rb") {
		return nil, false
	}
	renderer := core.DynamicInstanceVar(receiver, "@renderer")
	if renderer == nil || renderer.Type == object.ValueNil {
		return nil, false
	}
	return renderer, true
}

// executeNativeForwardableDelegatorFast recognizes the compact Forwardable
// implementation shipped with RGo's stdlib.  Its closure captures only an
// accessor and a target name, so a target lookup can be performed without
// allocating the wrapper's rest-argument Frame.  The caller's block is
// restored only for the second send, matching `target.send(name, *args, &block)`
// in the Ruby implementation.
func (vm *VM) executeNativeForwardableDelegatorFast(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue, keywordSyntax bool) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		core.AnyTracePointActive() || vm.instructionLimit != 0 || len(vm.catchStack) != 0 {
		return nil, false
	}
	if methodObj.DispatchOwner != nil {
		ownerClass, ownerOK := methodObj.DispatchOwner.Data.(*object.Class)
		if methodObj.DispatchOwner.Type != object.ValueClass || !ownerOK || ownerClass == nil || ownerClass.Name != "Prawn::Document" {
			return nil, false
		}
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || !strings.HasSuffix(fn.SourcePath, "/forwardable.rb") || fn.Name != "__block__" ||
		!fn.HasRestParam || !fn.HasBlockParam || len(fn.Params) != 0 || len(fn.FreeVarNames) != 2 ||
		fn.FreeVarNames[0] != "accessor" || fn.FreeVarNames[1] != "name" ||
		registerIROpcodeSequence(fn) != "OpSelf>OpGetFree>OpSend>OpSetLocal>OpPop>OpGetLocal>OpGetFree>OpGetLocal>OpSplatToArray>OpGetLocal>OpSend>OpBlockReturn" {
		return nil, false
	}
	closure := methodObj.Closure
	if closure == nil || closure.BreakOwnerID > 0 || len(closure.Free) != 2 || closureUsesRefinements(closure) {
		return nil, false
	}
	accessorName, accessorOK := core.MethodNameFromValue(derefClosureValue(closure.Free[0]))
	targetName, targetOK := core.MethodNameFromValue(derefClosureValue(closure.Free[1]))
	if !accessorOK || !targetOK || accessorName == "" || targetName == "" {
		return nil, false
	}
	previousBlock := vm.currentBlock
	vm.currentBlock = nil
	accessorResult := vm.sendWithCallInfo(receiver, accessorName, nil, false)
	vm.currentBlock = previousBlock
	if accessorResult == nil || accessorResult.Type == object.ValueException {
		return accessorResult, true
	}
	return vm.sendWithCallInfo(accessorResult, targetName, args, keywordSyntax), true
}

// nativeForwardableDelegatorCandidate is a cheap shape filter used before the
// full closure/free-variable and opcode proof. Forwardable's generated
// delegators are the only methods that can pass the expensive intrinsic; all
// other methods can skip that probe entirely.
func nativeForwardableDelegatorCandidate(methodObj *object.Method) bool {
	if methodObj == nil {
		return false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == "__block__" &&
		strings.HasSuffix(fn.SourcePath, "/forwardable.rb") &&
		fn.HasRestParam && fn.HasBlockParam && len(fn.Params) == 0 && len(fn.FreeVarNames) == 2
}
