package vm

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePDFRendererFinalizeEnabled = os.Getenv("RGO_DISABLE_NATIVE_PDF_RENDERER_FINALIZE") == ""

// executeNativePDFRendererAddContent removes the renderer -> page ->
// reference -> stream Ruby call chain for the ordinary content-stream case.
// It deliberately deopts when the graphics stack, page content, or String
// storage is not the exact Prawn/PDF::Core layout; those cases can invoke Ruby
// hooks and must retain the normal protocol.
func (vm *VM) executeNativePDFRendererAddContent(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "PDF::Core::Renderer" ||
		len(args) != 1 || args[0] == nil || args[0].Type != object.ValueString || args[0].Class != core.R.Classes["String"] {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "add_content" || !strings.HasSuffix(fn.SourcePath, "/renderer.rb") {
		return nil, false
	}
	state := core.DynamicInstanceVar(receiver, "@state")
	if !nativePDFExactObject(state, "PDF::Core::DocumentState") {
		return nil, false
	}
	page := core.DynamicInstanceVar(state, "@page")
	if !nativePDFExactObject(page, "PDF::Core::Page") || !nativePDFPageHasGraphicState(page) {
		return nil, false
	}
	stream := nativePDFPageContentStream(page)
	if !nativePDFExactObject(stream, "PDF::Core::Stream") {
		return nil, false
	}
	streamData := core.DynamicInstanceVar(stream, "@stream")
	if streamData == nil || streamData.Type == object.ValueNil {
		streamData = &object.EmeraldValue{Type: object.ValueString, Data: "", Class: core.R.Classes["String"], Encoding: "UTF-8"}
	}
	if streamData.Type != object.ValueString || streamData.Class != core.R.Classes["String"] || streamData.Frozen ||
		core.AttachedSingletonClass(streamData) != nil {
		return nil, false
	}
	if _, handled := core.AppendStringOneFast(streamData, args[0]); !handled {
		return nil, false
	}
	if errVal := core.AppendASCIIBytes(streamData, "\n"); errVal != nil {
		return errVal, true
	}
	if result := core.SetDynamicInstanceVar(stream, "@stream", streamData); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(stream, "@filtered_stream", core.R.NilVal); result != nil {
		return result, true
	}
	return stream, true
}

// executeNativePDFRendererFinalize removes the Ruby block/Range overhead from
// the ordinary uncompressed page finalization. Page.finalize has no mutation
// when compression is false; the only observable work here is appending the
// closing Q operators and emptying each page's graphics stack.
func (vm *VM) executeNativePDFRendererFinalize(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFRendererFinalizeEnabled || !nativePDFObjectEnabled || methodObj == nil ||
		methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" ||
		receiver == nil || receiver.Type != object.ValueObject || receiver.Class != vm.nativePDFConstructorClass("PDF::Core::Renderer") ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 0 || vm.currentBlock != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "finalize_all_page_contents" || !strings.HasSuffix(fn.SourcePath, "/renderer.rb") ||
		len(fn.Params) != 0 || fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock {
		return nil, false
	}
	state := core.DynamicInstanceVar(receiver, "@state")
	if !nativePDFExactObject(state, "PDF::Core::DocumentState") {
		return nil, false
	}
	compress := core.DynamicInstanceVar(state, "@compress")
	if compress == nil || compress.Type != object.ValueBool || compress.Data != false {
		return nil, false
	}
	pages := core.DynamicInstanceVar(state, "@pages")
	pageItems, pagesOK := nativePDFArrayItems(pages)
	if !pagesOK || pages == nil || pages.Type != object.ValueArray || pages.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(pages) != nil || len(pageItems) == 0 {
		return nil, false
	}
	pageClass := vm.nativePDFConstructorClass("PDF::Core::Page")
	stackClass := vm.nativePDFConstructorClass("PDF::Core::GraphicStateStack")
	if pageClass == nil || stackClass == nil || !nativePrawnExactMethodSource(pageClass, "finalize", "/page.rb") {
		return nil, false
	}
	for _, page := range pageItems {
		if page == nil || page.Type != object.ValueObject || page.Class != pageClass ||
			core.AttachedSingletonClass(page) != nil ||
			core.DynamicInstanceVar(page, "@document") == nil ||
			!nativePDFExactObject(core.DynamicInstanceVar(page, "@stack"), "PDF::Core::GraphicStateStack") ||
			core.DynamicInstanceVar(page, "@stack").Class != stackClass {
			return nil, false
		}
		values := core.DynamicInstanceVar(core.DynamicInstanceVar(page, "@stack"), "@stack")
		if values == nil {
			return nil, false
		}
		items, itemsOK := values.Data.([]*object.EmeraldValue)
		if values == nil || values.Type != object.ValueArray || values.Class != core.R.Classes["Array"] ||
			values.Frozen || core.AttachedSingletonClass(values) != nil || !itemsOK {
			return nil, false
		}
		if len(items) > 0 {
			stream := nativePDFPageContentStream(page)
			streamData := core.DynamicInstanceVar(stream, "@stream")
			if stream == nil || !nativePDFExactObject(stream, "PDF::Core::Stream") || streamData == nil ||
				streamData.Type != object.ValueString || streamData.Class != core.R.Classes["String"] ||
				streamData.Frozen || core.AttachedSingletonClass(streamData) != nil {
				return nil, false
			}
		}
	}

	for _, page := range pageItems {
		core.SetDynamicInstanceVar(state, "@page", page)
		stack := core.DynamicInstanceVar(page, "@stack")
		values := core.DynamicInstanceVar(stack, "@stack")
		items := values.Data.([]*object.EmeraldValue)
		for len(items) > 0 {
			if result, handled := nativePrawnAppendDirectContent(page, "Q"); !handled {
				return nil, false
			} else if result != nil && result.Type == object.ValueException {
				return result, true
			}
			items = items[:len(items)-1]
			values.Data = items
		}
	}
	core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(int64(len(pageItems))))
	return core.R.NilVal, true
}

func nativePDFExactObject(value *object.EmeraldValue, className string) bool {
	return value != nil && value.Type == object.ValueObject && value.Class != nil && value.Class.Name == className
}

func nativePDFPageHasGraphicState(page *object.EmeraldValue) bool {
	stack := core.DynamicInstanceVar(page, "@stack")
	if !nativePDFExactObject(stack, "PDF::Core::GraphicStateStack") {
		return false
	}
	values := core.DynamicInstanceVar(stack, "@stack")
	if values == nil || values.Type != object.ValueArray || values.Class != core.R.Classes["Array"] {
		return false
	}
	items, ok := values.Data.([]*object.EmeraldValue)
	return ok && len(items) > 0 && items[len(items)-1] != nil && items[len(items)-1].Type != object.ValueNil
}

func nativePDFPageContentStream(page *object.EmeraldValue) *object.EmeraldValue {
	if stamp := core.DynamicInstanceVar(page, "@stamp_stream"); nativePDFExactObject(stamp, "PDF::Core::Stream") {
		return stamp
	}
	document := core.DynamicInstanceVar(page, "@document")
	if document == nil || document.Type != object.ValueObject {
		return nil
	}
	state := core.DynamicInstanceVar(document, "@state")
	store := core.DynamicInstanceVar(state, "@store")
	objects := core.DynamicInstanceVar(store, "@objects")
	contentID := core.DynamicInstanceVar(page, "@content")
	if objects == nil || contentID == nil || objects.Type != object.ValueHash || contentID.Type != object.ValueInteger {
		return nil
	}
	content, ok := core.DirectHashIndex(objects, contentID)
	if !ok || !nativePDFExactObject(content, "PDF::Core::Reference") {
		return nil
	}
	stream := core.DynamicInstanceVar(content, "@stream")
	if !nativePDFExactObject(stream, "PDF::Core::Stream") {
		return nil
	}
	return stream
}
