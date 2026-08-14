package vm

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePDFStreamFilteredEnabled = os.Getenv("RGO_DISABLE_NATIVE_PDF_STREAM_FILTERED") == ""

// executeNativePDFStreamAppend is the typed counterpart of
// PDF::Core::Stream#<<.  Prawn appends every content fragment through this
// method; the exact class/source guards retain normal Ruby dispatch whenever a
// subclass, singleton method, encoding mismatch, or monkey patch is present.
func (vm *VM) executeNativePDFStreamAppend(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "PDF::Core::Stream" ||
		receiver.Frozen || len(args) != 1 || args[0] == nil || args[0].Type != object.ValueString ||
		args[0].Class != core.R.Classes["String"] {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "<<" || !strings.HasSuffix(fn.SourcePath, "/stream.rb") {
		return nil, false
	}
	stream := core.DynamicInstanceVar(receiver, "@stream")
	if stream == nil || stream.Type == object.ValueNil {
		stream = &object.EmeraldValue{Type: object.ValueString, Data: "", Class: core.R.Classes["String"], Encoding: "UTF-8"}
	}
	if stream.Type != object.ValueString || stream.Class != core.R.Classes["String"] || stream.Frozen ||
		core.AttachedSingletonClass(stream) != nil {
		return nil, false
	}
	if _, handled := core.AppendStringOneFast(stream, args[0]); !handled {
		return nil, false
	}
	if result := core.SetDynamicInstanceVar(receiver, "@stream", stream); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(receiver, "@filtered_stream", core.R.NilVal); result != nil {
		return result, true
	}
	return receiver, true
}

// executeNativePDFStreamFilteredStream handles the unfiltered stream case.
// Prawn's Ruby implementation otherwise allocates a block and walks an empty
// FilterList for every ordinary PDF content stream. Compressed/custom filter
// lists still use the full Ruby path.
func (vm *VM) executeNativePDFStreamFilteredStream(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFStreamFilteredEnabled || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class != vm.nativePDFConstructorClass("PDF::Core::Stream") ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 0 || vm.currentBlock != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "filtered_stream" || !strings.HasSuffix(fn.SourcePath, "/stream.rb") ||
		len(fn.Params) != 0 || fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock {
		return nil, false
	}
	stream := core.DynamicInstanceVar(receiver, "@stream")
	if stream == nil || stream.Type == object.ValueNil {
		return core.R.NilVal, true
	}
	if stream.Type != object.ValueString || stream.Class != core.R.Classes["String"] || core.AttachedSingletonClass(stream) != nil {
		return nil, false
	}
	filtered := core.DynamicInstanceVar(receiver, "@filtered_stream")
	if filtered != nil && filtered.Type != object.ValueNil {
		return filtered, true
	}
	filters := core.DynamicInstanceVar(receiver, "@filters")
	filterClass := vm.nativePDFConstructorClass("PDF::Core::FilterList")
	filterList := core.DynamicInstanceVar(filters, "@list")
	filterItems, filtersOK := nativePDFArrayItems(filterList)
	if filters == nil || filters.Type != object.ValueObject || filters.Class != filterClass ||
		core.AttachedSingletonClass(filters) != nil || !filtersOK || filterList == nil ||
		filterList.Type != object.ValueArray || filterList.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(filterList) != nil || len(filterItems) != 0 {
		return nil, false
	}
	filtered = nativePDFCloneString(stream)
	if filtered == nil {
		return nil, false
	}
	core.TrackObjectSpaceValue(filtered)
	if result := core.SetDynamicInstanceVar(receiver, "@filtered_stream", filtered); result != nil {
		return result, true
	}
	return filtered, true
}
