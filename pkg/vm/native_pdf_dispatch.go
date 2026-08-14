package vm

import "github.com/GoLangDream/rgo/pkg/object"

// nativePDFDispatchCandidate keeps the cache-hit probe out of unrelated Ruby
// sends. The individual ABI handlers still repeat their exact method/source/
// layout guards; this helper only narrows by the receiver family.
func nativePDFDispatchCandidate(receiver *object.EmeraldValue) bool {
	if !nativePDFObjectEnabled || receiver == nil {
		return false
	}
	if receiver.Type == object.ValueModule {
		mod, ok := receiver.Data.(*object.Module)
		return ok && mod != nil && mod.Name == "PDF::Core"
	}
	if receiver.Type != object.ValueObject || receiver.Class == nil {
		return false
	}
	switch receiver.Class.Name {
	case "PDF::Core::Stream", "PDF::Core::Renderer", "PDF::Core::ObjectStore", "Prawn::Document":
		return true
	default:
		return false
	}
}

// nativePDFDispatchCandidateForMethod adds the method-shape filter to the
// receiver-family filter. Most sends on a Prawn/PDF object are ordinary Ruby
// methods; only this small set can enter the native ABI below. Keeping the
// filter at the call site avoids repeatedly running every source/layout probe
// for methods that can never be handled.
func nativePDFDispatchCandidateForMethod(receiver *object.EmeraldValue, methodObj *object.Method) bool {
	if !nativePDFDispatchCandidate(receiver) || methodObj == nil {
		return false
	}
	if receiver.Type == object.ValueModule {
		return methodObj.Name == "pdf_object"
	}
	switch receiver.Class.Name {
	case "PDF::Core::Stream":
		return methodObj.Name == "<<" || methodObj.Name == "filtered_stream"
	case "PDF::Core::Renderer":
		return methodObj.Name == "add_content" || methodObj.Name == "graphic_state" || methodObj.Name == "finalize_all_page_contents" || methodObj.Name == "render"
	case "PDF::Core::ObjectStore":
		return methodObj.Name == "push" || methodObj.Name == "ref" || methodObj.Name == "each" || methodObj.Name == "[]"
	case "Prawn::Document":
		if methodObj.Name == "renderer" || nativeForwardableDelegatorCandidate(methodObj) {
			return true
		}
		return (nativePrawnSimpleEnabled && (methodObj.Name == "text" || methodObj.Name == "start_new_page" || methodObj.Name == "render")) ||
			(nativePrawnStartNewPageEnabled && methodObj.Name == "start_new_page") ||
			(nativePrawnRenderFastEnabled && methodObj.Name == "render") ||
			(nativePrawnDirectTextEnabled && methodObj.Name == "text")
	default:
		return false
	}
}

// executeNativePDFDispatch is the single cheap admission point for the
// opt-in native Gem ABI.  Calling every intrinsic probe for every Ruby send
// made the first implementation pay several repeated nil/env/class checks on
// unrelated methods.  Class/module identity narrows the probe before the
// source and bytecode guards in the individual handlers run.
func (vm *VM) executeNativePDFDispatch(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue, keywordSyntax bool) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || receiver == nil {
		return nil, false
	}
	if receiver.Type == object.ValueModule {
		if mod, ok := receiver.Data.(*object.Module); ok && mod != nil && mod.Name == "PDF::Core" {
			return vm.executeNativePDFObject(methodObj, receiver, args)
		}
		return nil, false
	}
	if receiver.Type != object.ValueObject || receiver.Class == nil {
		return nil, false
	}
	switch receiver.Class.Name {
	case "PDF::Core::Stream":
		if len(args) == 0 && methodObj.Name == "filtered_stream" {
			if result, executed := vm.executeNativePDFStreamFilteredStream(methodObj, receiver, args); executed {
				return result, true
			}
		}
		return vm.executeNativePDFStreamAppend(methodObj, receiver, args)
	case "PDF::Core::Renderer":
		if len(args) == 0 {
			if methodObj.Name == "render" {
				if result, executed := vm.executeNativePDFRendererRenderRegion(methodObj, receiver, args); executed {
					return result, true
				}
			}
			if methodObj.Name == "finalize_all_page_contents" {
				if result, executed := vm.executeNativePDFRendererFinalize(methodObj, receiver, args); executed {
					return result, true
				}
			}
			if result, executed := vm.executeNativePDFRendererGraphicState(methodObj, receiver); executed {
				return result, true
			}
		}
		return vm.executeNativePDFRendererAddContent(methodObj, receiver, args)
	case "PDF::Core::ObjectStore":
		if len(args) == 1 || len(args) == 2 {
			if result, executed := vm.executeNativePDFObjectStorePush(methodObj, receiver, args); executed {
				return result, true
			}
		}
		if len(args) == 0 {
			if result, executed := vm.executeNativePDFObjectStoreEach(methodObj, receiver); executed {
				return result, true
			}
		}
		return vm.executeNativePDFObjectStoreIndex(methodObj, receiver, args)
	case "Prawn::Document":
		if nativePrawnSimpleEnabled {
			if result, executed := vm.executeNativePrawnSimple(methodObj, receiver, args); executed {
				return result, true
			}
		}
		if nativePrawnStartNewPageEnabled && methodObj.Name == "start_new_page" {
			if result, executed := vm.executeNativePrawnStartNewPage(methodObj, receiver, args, keywordSyntax); executed {
				return result, true
			}
		}
		if nativePrawnRenderFastEnabled && methodObj.Name == "render" {
			if result, executed := vm.executeNativePrawnRenderFast(methodObj, receiver, args, keywordSyntax); executed {
				return result, true
			}
		}
		if nativePrawnDirectTextEnabled && methodObj.Name == "text" {
			if result, executed := vm.executeNativePrawnDirectText(methodObj, receiver, args); executed {
				return result, true
			}
		}
		if len(args) == 0 && methodObj.Name == "renderer" {
			if result, executed := vm.executeNativePrawnRendererReader(methodObj, receiver); executed {
				return result, true
			}
		}
		if nativeForwardableDelegatorCandidate(methodObj) {
			if result, executed := vm.executeNativeForwardableDelegatorFast(methodObj, receiver, args, keywordSyntax); executed {
				return result, true
			}
		}
	}
	return nil, false
}
