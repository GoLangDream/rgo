package vm

// This file contains the unified PDF renderer hot region.  The older native
// PDF entries remove individual Ruby calls, but Renderer#render still paid for
// the Ruby orchestration around those entries.  This region proves the whole
// standard uncompressed renderer object layout first, then serializes the
// existing Ruby-visible object graph in one typed pass.  Unsupported state
// returns handled=false before any observable mutation.

import (
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePDFRendererRenderRegionEnabled = os.Getenv("RGO_DISABLE_NATIVE_PDF_RENDER_REGION") == ""

const nativePDFRenderCompileMinEntries = 16

// nativePDFRenderRegionABIPlan is the immutable half of the renderer region.
// The object graph is rebuilt for every document, but the class/source/builtin
// proof is shared by all renders in one VM and invalidated by the runtime
// method/constant generations.  Keeping this separate from the per-render
// object plan removes the largest repeated guard block without weakening the
// Ruby side exit for a redefinition.
type nativePDFRenderRegionABIPlan struct {
	methodGeneration   uint64
	constantGeneration uint64
	rendererClass      *object.Class
	stateClass         *object.Class
	storeClass         *object.Class
	referenceClass     *object.Class
	streamClass        *object.Class
	filterListClass    *object.Class
	pageClass          *object.Class
	stackClass         *object.Class
	renderMethod       *object.Method
	valid              bool
}

type nativePDFRenderPagePlan struct {
	page        *object.EmeraldValue
	state       *object.EmeraldValue
	stackValues *object.EmeraldValue
	items       []*object.EmeraldValue
	stream      *object.EmeraldValue
}

type nativePDFRenderReferencePlan struct {
	ref           *object.EmeraldValue
	data          *object.EmeraldValue
	stream        *object.EmeraldValue
	raw           *object.EmeraldValue
	filtered      string
	filteredCache bool
	offset        int64
	identifier    int64
	generation    int64
}

type nativePDFRenderValuePlan struct {
	kind              object.ValueType
	inContentStream   bool
	items             []*object.EmeraldValue
	entries           []nativePDFHashEntry
	serialized        string
	contentSerialized string
}

type nativePDFRenderRegionPlan struct {
	renderer    *object.EmeraldValue
	state       *object.EmeraldValue
	store       *object.EmeraldValue
	identifiers *object.EmeraldValue
	objects     *object.EmeraldValue
	refs        []nativePDFRenderReferencePlan
	pages       []nativePDFRenderPagePlan
	root        *object.EmeraldValue
	info        *object.EmeraldValue
	version     float64
	valuePlans  map[*object.EmeraldValue]nativePDFRenderValuePlan
}

// executeNativePDFRendererRenderRegion replaces the Ruby StringIO/render
// orchestration only for the exact PDF::Core renderer shape.  It deliberately
// accepts no output argument: Renderer#render(output) has observable writes to
// a user object and must remain on Ruby.
func (vm *VM) executeNativePDFRendererRenderRegion(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFRendererRenderRegionEnabled || !nativePDFObjectEnabled || methodObj == nil ||
		methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" ||
		receiver == nil || receiver.Type != object.ValueObject || receiver.Class == nil ||
		receiver.Frozen ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 0 || vm.currentBlock != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 ||
		!core.StringAppendUsesBuiltinImplementation() || !core.HashIndexUsesBuiltinImplementation() {
		return nil, false
	}

	abi, ok := vm.nativePDFRenderRegionABIPlanFor(methodObj, receiver.Class)
	if !ok {
		return nil, false
	}
	stateClass := abi.stateClass
	storeClass := abi.storeClass
	referenceClass := abi.referenceClass
	streamClass := abi.streamClass
	filterListClass := abi.filterListClass
	pageClass := abi.pageClass
	stackClass := abi.stackClass

	generation := object.CurrentMethodGeneration()
	plan, ok := vm.nativePDFRenderRegionPlan(receiver, stateClass, storeClass, referenceClass, streamClass, filterListClass, pageClass, stackClass)
	if !ok || object.CurrentMethodGeneration() != generation {
		return nil, false
	}

	// The page plan has already checked every condition used by the existing
	// native finalizer.  Keep the mutation in this region so no Ruby call or
	// partial side exit can occur between finalization and serialization.
	for index := range plan.pages {
		page := &plan.pages[index]
		if result := core.SetDynamicInstanceVar(plan.state, "@page", page.page); result != nil {
			return result, true
		}
		for len(page.items) > 0 {
			if result, handled := nativePDFRenderAppendQ(page.stream); !handled {
				return nil, false
			} else if result != nil {
				return result, true
			}
			page.items = page.items[:len(page.items)-1]
			page.stackValues.Data = page.items
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(int64(len(plan.pages)))); result != nil {
		return result, true
	}

	var output strings.Builder
	output.Grow(1024 + len(plan.refs)*96)
	output.WriteString("%PDF-")
	output.WriteString(nativePDFRealText(plan.version))
	output.WriteString("\n%\xFF\xFF\xFF\xFF\n")
	// The complete object graph was cycle-checked by nativePDFRenderRegionPlan
	// before any writer mutation. No Ruby callback can run between that proof
	// and this trusted pass, so keep seen nil here and avoid a map read/write on
	// every small composite. Standalone serializer tests pass a real seen map
	// and retain the defensive recursive behavior.
	var seen map[*object.EmeraldValue]bool
	for index := range plan.refs {
		ref := &plan.refs[index]
		ref.offset = int64(output.Len())
		if result := core.SetDynamicInstanceVar(ref.ref, "@offset", core.NewIntegerValue(ref.offset)); result != nil {
			return result, true
		}
		if !nativePDFRenderReference(&output, ref, seen, plan.valuePlans) {
			return nil, false
		}
	}

	xrefOffset := output.Len()
	if result := core.SetDynamicInstanceVar(receiver, "@xref_offset", core.NewIntegerValue(int64(xrefOffset))); result != nil {
		return result, true
	}
	output.WriteString("xref\n0 ")
	nativePDFRenderWriteInt(&output, int64(len(plan.refs)+1))
	output.WriteString("\n0000000000 65535 f \n")
	for index := range plan.refs {
		ref := &plan.refs[index]
		if ref.offset < 0 {
			return nil, false
		}
		if !nativePDFRenderWritePaddedOffset(&output, ref.offset) {
			return nil, false
		}
		output.WriteString(" 00000 n \n")
	}

	trailer := nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("Size"), core.NewIntegerValue(int64(len(plan.refs) + 1))},
		[2]*object.EmeraldValue{nativePDFSymbol("Root"), plan.root},
		[2]*object.EmeraldValue{nativePDFSymbol("Info"), plan.info},
	)
	output.WriteString("trailer\n")
	if !nativePDFRenderWriteObjectText(&output, trailer, false, seen, plan.valuePlans) {
		return nil, false
	}
	output.WriteString("\nstartxref\n")
	nativePDFRenderWriteInt(&output, int64(xrefOffset))
	output.WriteString("\n%%EOF\n")
	return &object.EmeraldValue{Type: object.ValueString, Data: output.String(), Class: core.R.Classes["String"], Encoding: "ASCII-8BIT"}, true
}

func (vm *VM) nativePDFRenderRegionABIPlanFor(methodObj *object.Method, rendererClass *object.Class) (*nativePDFRenderRegionABIPlan, bool) {
	if vm == nil || methodObj == nil || rendererClass == nil {
		return nil, false
	}
	methodGeneration := object.CurrentMethodGeneration()
	constantGeneration := object.CurrentConstantGeneration()
	plan := &vm.nativePDFRenderRegionABIPlan
	if plan.methodGeneration == methodGeneration && plan.constantGeneration == constantGeneration &&
		plan.renderMethod == methodObj && plan.rendererClass == rendererClass {
		if !plan.valid || methodObj.DispatchOwner != nil || !nativePDFRenderRegionMethodShapeCached(methodObj) {
			return nil, false
		}
		return plan, true
	}

	*plan = nativePDFRenderRegionABIPlan{
		methodGeneration:   methodGeneration,
		constantGeneration: constantGeneration,
		rendererClass:      rendererClass,
		renderMethod:       methodObj,
	}
	if rendererClass.Name != "PDF::Core::Renderer" || methodObj.Name != "render" ||
		methodObj.DispatchOwner != nil || !nativePDFRenderRegionMethodShape(rendererClass, methodObj) {
		return nil, false
	}
	plan.stateClass = vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	plan.storeClass = vm.nativePDFConstructorClass("PDF::Core::ObjectStore")
	plan.referenceClass = vm.nativePDFConstructorClass("PDF::Core::Reference")
	plan.streamClass = vm.nativePDFConstructorClass("PDF::Core::Stream")
	plan.filterListClass = vm.nativePDFConstructorClass("PDF::Core::FilterList")
	plan.pageClass = vm.nativePDFConstructorClass("PDF::Core::Page")
	plan.stackClass = vm.nativePDFConstructorClass("PDF::Core::GraphicStateStack")
	if plan.stateClass == nil || plan.storeClass == nil || plan.referenceClass == nil || plan.streamClass == nil ||
		plan.filterListClass == nil || plan.pageClass == nil || plan.stackClass == nil ||
		!nativePDFRenderRegionDependencies(vm, rendererClass, plan.stateClass, plan.storeClass, plan.referenceClass, plan.streamClass, plan.pageClass) {
		return nil, false
	}
	plan.valid = true
	return plan, true
}

func nativePDFRenderRegionMethodShapeCached(methodObj *object.Method) bool {
	if methodObj == nil || methodObj.Name != "render" || methodObj.DispatchOwner != nil {
		return false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == "render" && strings.HasSuffix(fn.SourcePath, "/renderer.rb") &&
		len(fn.Params) == 1 && len(fn.ParamDefaults) == 1 && !fn.HasRestParam && !fn.HasBlockParam &&
		len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "" && !fn.KeywordRestOnly && !fn.RejectBlock
}

func nativePDFRenderRegionMethodShape(cls *object.Class, methodObj *object.Method) bool {
	if cls == nil || methodObj == nil {
		return false
	}
	method, owner, found := cls.GetMethodWithOwner("render")
	if !found || method != methodObj || owner != cls || method.DispatchOwner != nil {
		return false
	}
	fn, ok := method.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == "render" && strings.HasSuffix(fn.SourcePath, "/renderer.rb") &&
		len(fn.Params) == 1 && len(fn.ParamDefaults) == 1 && !fn.HasRestParam && !fn.HasBlockParam &&
		len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "" && !fn.KeywordRestOnly && !fn.RejectBlock
}

func nativePDFRenderRegionDependencies(vm *VM, rendererClass, stateClass, storeClass, referenceClass, streamClass, pageClass *object.Class) bool {
	if vm == nil || !nativePDFRenderExactMethod(rendererClass, "render_header", "/renderer.rb") ||
		!nativePDFRenderExactMethod(rendererClass, "render_body", "/renderer.rb") ||
		!nativePDFRenderExactMethod(rendererClass, "render_xref", "/renderer.rb") ||
		!nativePDFRenderExactMethod(rendererClass, "render_trailer", "/renderer.rb") ||
		!nativePDFRenderExactMethod(rendererClass, "finalize_all_page_contents", "/renderer.rb") ||
		!nativePDFRenderExactMethod(stateClass, "before_render_actions", "/document_state.rb") ||
		!nativePDFRenderExactMethod(stateClass, "render_body", "/document_state.rb") ||
		!nativePDFRenderExactMethod(storeClass, "each", "/object_store.rb") ||
		!nativePDFRenderExactMethod(storeClass, "size", "/object_store.rb") ||
		!nativePDFRenderExactMethod(storeClass, "root", "/object_store.rb") ||
		!nativePDFRenderExactMethod(storeClass, "info", "/object_store.rb") ||
		!nativePDFRenderExactMethod(referenceClass, "object", "/reference.rb") ||
		!nativePDFRenderExactMethod(referenceClass, "to_s", "/reference.rb") ||
		!nativePDFRenderExactMethod(streamClass, "empty?", "/stream.rb") ||
		!nativePDFRenderExactMethod(streamClass, "filtered_stream", "/stream.rb") ||
		!nativePDFRenderExactMethod(streamClass, "data", "/stream.rb") ||
		!nativePDFRenderExactMethod(streamClass, "object", "/stream.rb") ||
		!nativePDFRenderExactMethod(pageClass, "finalize", "/page.rb") ||
		!nativePDFRenderExactAttrReader(rendererClass, "state") ||
		!nativePDFRenderExactAttrReader(stateClass, "store") ||
		!nativePDFRenderExactAttrReader(stateClass, "version") ||
		!nativePDFRenderExactAttrReader(stateClass, "pages") ||
		!nativePDFRenderExactAttrReader(stateClass, "trailer") ||
		!nativePDFRenderExactAttrReader(stateClass, "compress") ||
		!nativePDFRenderExactAttrReader(stateClass, "encrypt") ||
		!nativePDFRenderExactAttrReader(referenceClass, "gen") ||
		!nativePDFRenderExactAttrReader(referenceClass, "data") ||
		!nativePDFRenderExactAttrReader(referenceClass, "stream") ||
		!nativePDFRenderExactAttrWriter(referenceClass, "offset") ||
		!nativePDFRenderCorePDFObject(vm) {
		return false
	}
	return core.HashIndexUsesBuiltinImplementation() && core.StringAppendUsesBuiltinImplementation()
}

func nativePDFRenderExactMethod(cls *object.Class, name, suffix string) bool {
	return nativePrawnExactMethodSource(cls, name, suffix)
}

func nativePDFRenderExactAttrReader(cls *object.Class, name string) bool {
	if cls == nil {
		return false
	}
	method, owner, found := cls.GetMethodWithOwner(name)
	return found && method != nil && owner == cls && method.DispatchOwner == nil &&
		method.AttrReaderName == "@"+name && method.AttrWriterName == "" && method.Fn != nil
}

func nativePDFRenderExactAttrWriter(cls *object.Class, name string) bool {
	if cls == nil {
		return false
	}
	method, owner, found := cls.GetMethodWithOwner(name + "=")
	return found && method != nil && owner == cls && method.DispatchOwner == nil &&
		method.AttrWriterName == "@"+name && method.AttrReaderName == "" && method.Fn != nil
}

func nativePDFRenderCorePDFObject(vm *VM) bool {
	value, found := vm.qualifiedConstantValue("PDF::Core")
	if !found || value == nil || value.Type != object.ValueModule {
		return false
	}
	module, ok := value.Data.(*object.Module)
	if !ok || module == nil || module.Name != "PDF::Core" {
		return false
	}
	method, found := module.GetMethod("pdf_object")
	if !found || method == nil || module.Methods["pdf_object"] != method || method.DispatchOwner != nil {
		return false
	}
	fn, ok := method.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == "pdf_object" && strings.HasSuffix(fn.SourcePath, "/pdf_object.rb")
}

func (vm *VM) nativePDFRenderRegionPlan(receiver *object.EmeraldValue, stateClass, storeClass, referenceClass, streamClass, filterListClass, pageClass, stackClass *object.Class) (*nativePDFRenderRegionPlan, bool) {
	state := nativePrawnTextLayoutIvar(receiver, "@state")
	store := nativePrawnTextLayoutIvar(state, "@store")
	if state == nil || state.Type != object.ValueObject || state.Class != stateClass || state.Frozen ||
		store == nil || store.Type != object.ValueObject || store.Class != storeClass || store.Frozen ||
		core.AttachedSingletonClass(state) != nil || core.AttachedSingletonClass(store) != nil {
		return nil, false
	}
	versionValue := nativePrawnTextLayoutIvar(state, "@version")
	version, versionOK := float64(0), false
	if versionValue != nil {
		version, versionOK = versionValue.Data.(float64)
	}
	if versionValue == nil || versionValue.Type != object.ValueFloat || versionValue.Class != core.R.Classes["Float"] ||
		core.AttachedSingletonClass(versionValue) != nil || !versionOK || math.IsNaN(version) || math.IsInf(version, 0) || version != 1.3 {
		return nil, false
	}
	if compress := nativePrawnTextLayoutIvar(state, "@compress"); compress == nil || compress.Type != object.ValueBool || compress.Data != false {
		return nil, false
	}
	if encrypt := nativePrawnTextLayoutIvar(state, "@encrypt"); encrypt == nil || encrypt.Type != object.ValueBool || encrypt.Data != false {
		return nil, false
	}
	callbacks := nativePrawnTextLayoutIvar(state, "@before_render_callbacks")
	callbackItems, callbacksOK := nativePDFArrayItems(callbacks)
	trailer := nativePrawnTextLayoutIvar(state, "@trailer")
	trailerData, trailerOK := (*object.RHash)(nil), false
	if trailer != nil {
		trailerData, trailerOK = trailer.Data.(*object.RHash)
	}
	if callbacks == nil || callbacks.Type != object.ValueArray || callbacks.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(callbacks) != nil || !callbacksOK || len(callbackItems) != 0 ||
		!nativePDFStandardHash(trailer) || !trailerOK || trailerData == nil || len(trailerData.Keys) != 0 || len(trailerData.Pairs) != 0 {
		return nil, false
	}

	identifiers := nativePrawnTextLayoutIvar(store, "@identifiers")
	objects := nativePrawnTextLayoutIvar(store, "@objects")
	ids, idsOK := ([]*object.EmeraldValue)(nil), false
	if identifiers != nil {
		ids, idsOK = identifiers.Data.([]*object.EmeraldValue)
	}
	if identifiers == nil || identifiers.Type != object.ValueArray || identifiers.Class != core.R.Classes["Array"] ||
		identifiers.Frozen || core.AttachedSingletonClass(identifiers) != nil || !idsOK ||
		!nativePDFStandardHash(objects) || objects.Frozen {
		return nil, false
	}
	rootID := nativePrawnTextLayoutIvar(store, "@root")
	infoID := nativePrawnTextLayoutIvar(store, "@info")
	if rootID == nil || rootID.Type != object.ValueInteger || infoID == nil || infoID.Type != object.ValueInteger {
		return nil, false
	}
	root, rootOK := core.DirectHashIndex(objects, rootID)
	info, infoOK := core.DirectHashIndex(objects, infoID)
	// Most composites are owned by one reference, so the reference count is a
	// useful lower bound for both the per-render object-layout cache and the
	// cycle proof. Reserving it here keeps the first render pass out of the
	// map-growth path while still allowing unusual nested graphs to grow
	// normally.
	planCapacity := len(ids)
	if planCapacity < 16 {
		planCapacity = 16
	}
	valuePlans := make(map[*object.EmeraldValue]nativePDFRenderValuePlan, planCapacity)
	seen := make(map[*object.EmeraldValue]bool, planCapacity)
	if !rootOK || !infoOK || !nativePDFRenderValueShape(root, false, seen, referenceClass, valuePlans) ||
		!nativePDFRenderValueShape(info, false, seen, referenceClass, valuePlans) {
		return nil, false
	}

	pageValues := nativePrawnTextLayoutIvar(state, "@pages")
	pageItems, pagesOK := ([]*object.EmeraldValue)(nil), false
	if pageValues != nil {
		pageItems, pagesOK = pageValues.Data.([]*object.EmeraldValue)
	}
	if pageValues == nil || pageValues.Type != object.ValueArray || pageValues.Class != core.R.Classes["Array"] ||
		pageValues.Frozen || core.AttachedSingletonClass(pageValues) != nil || !pagesOK || len(pageItems) == 0 {
		return nil, false
	}
	plan := &nativePDFRenderRegionPlan{
		renderer:    receiver,
		state:       state,
		store:       store,
		identifiers: identifiers,
		objects:     objects,
		refs:        make([]nativePDFRenderReferencePlan, 0, len(ids)),
		pages:       make([]nativePDFRenderPagePlan, 0, len(pageItems)),
		root:        root,
		info:        info,
		version:     version,
		valuePlans:  valuePlans,
	}
	for _, id := range ids {
		if id == nil || id.Type != object.ValueInteger {
			return nil, false
		}
		ref, found := core.DirectHashIndex(objects, id)
		if !found {
			return nil, false
		}
		refPlan, ok := nativePDFRenderReferencePlanFor(ref, referenceClass, streamClass, filterListClass, seen, valuePlans)
		if !ok {
			return nil, false
		}
		plan.refs = append(plan.refs, refPlan)
	}
	for _, page := range pageItems {
		pagePlan, ok := nativePDFBuildPagePlan(page, state, pageClass, stackClass)
		if !ok {
			return nil, false
		}
		plan.pages = append(plan.pages, pagePlan)
	}
	return plan, true
}

func nativePDFBuildPagePlan(page, state *object.EmeraldValue, pageClass, stackClass *object.Class) (nativePDFRenderPagePlan, bool) {
	document := nativePrawnTextLayoutIvar(page, "@document")
	if page == nil || page.Type != object.ValueObject || page.Class != pageClass || page.Frozen ||
		core.AttachedSingletonClass(page) != nil || document == nil || document.Type != object.ValueObject ||
		nativePrawnTextLayoutIvar(document, "@state") != state {
		return nativePDFRenderPagePlan{}, false
	}
	stack := nativePrawnTextLayoutIvar(page, "@stack")
	values := nativePrawnTextLayoutIvar(stack, "@stack")
	items, itemsOK := ([]*object.EmeraldValue)(nil), false
	if values != nil {
		items, itemsOK = values.Data.([]*object.EmeraldValue)
	}
	if stack == nil || stack.Type != object.ValueObject || stack.Class != stackClass || stack.Frozen ||
		core.AttachedSingletonClass(stack) != nil || values == nil || values.Type != object.ValueArray ||
		values.Class != core.R.Classes["Array"] || values.Frozen || core.AttachedSingletonClass(values) != nil || !itemsOK {
		return nativePDFRenderPagePlan{}, false
	}
	pagePlan := nativePDFRenderPagePlan{page: page, state: state, stackValues: values, items: items}
	if len(items) == 0 {
		return pagePlan, true
	}
	last := items[len(items)-1]
	if last == nil || last.Type == object.ValueNil {
		return nativePDFRenderPagePlan{}, false
	}
	stream := nativePDFPageContentStream(page)
	if !nativePDFRenderStreamMutationShape(stream) {
		return nativePDFRenderPagePlan{}, false
	}
	pagePlan.stream = stream
	return pagePlan, true
}

func nativePDFRenderReferencePlanFor(ref *object.EmeraldValue, referenceClass, streamClass, filterListClass *object.Class, seen map[*object.EmeraldValue]bool, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) (nativePDFRenderReferencePlan, bool) {
	if ref == nil || ref.Type != object.ValueObject || ref.Class != referenceClass || ref.Frozen ||
		core.AttachedSingletonClass(ref) != nil {
		return nativePDFRenderReferencePlan{}, false
	}
	identifier, identifierOK := nativePDFObjectIntegerIvar(ref, "@identifier")
	generation, generationOK := nativePDFObjectIntegerIvar(ref, "@gen")
	if !identifierOK || !generationOK {
		return nativePDFRenderReferencePlan{}, false
	}
	data := nativePrawnTextLayoutIvar(ref, "@data")
	if !nativePDFRenderValueShape(data, false, seen, referenceClass, valuePlans) {
		return nativePDFRenderReferencePlan{}, false
	}
	stream := nativePrawnTextLayoutIvar(ref, "@stream")
	if stream == nil || stream.Type != object.ValueObject || stream.Class != streamClass || stream.Frozen ||
		core.AttachedSingletonClass(stream) != nil {
		return nativePDFRenderReferencePlan{}, false
	}
	filters := nativePrawnTextLayoutIvar(stream, "@filters")
	list := nativePrawnTextLayoutIvar(filters, "@list")
	items, itemsOK := ([]*object.EmeraldValue)(nil), false
	if list != nil {
		items, itemsOK = list.Data.([]*object.EmeraldValue)
	}
	if filters == nil || filters.Type != object.ValueObject || filters.Class != filterListClass ||
		filters.Frozen || core.AttachedSingletonClass(filters) != nil || list == nil || list.Type != object.ValueArray ||
		list.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(list) != nil || !itemsOK || len(items) != 0 {
		return nativePDFRenderReferencePlan{}, false
	}
	raw := nativePrawnTextLayoutIvar(stream, "@stream")
	filtered := nativePrawnTextLayoutIvar(stream, "@filtered_stream")
	filteredText, filteredCache := "", false
	if filtered != nil && filtered.Type != object.ValueNil {
		var filteredOK bool
		filteredText, filteredOK = filtered.Data.(string)
		if !filteredOK || !nativePDFRenderASCIIValue(filtered, true) {
			return nativePDFRenderReferencePlan{}, false
		}
		filteredCache = true
	}
	if raw != nil && raw.Type != object.ValueNil {
		if !nativePDFRenderASCIIValue(raw, true) {
			return nativePDFRenderReferencePlan{}, false
		}
		if nativePDFRenderHashHasKey(data, "Length", valuePlans) {
			return nativePDFRenderReferencePlan{}, false
		}
	}
	return nativePDFRenderReferencePlan{
		ref:           ref,
		data:          data,
		stream:        stream,
		raw:           raw,
		filtered:      filteredText,
		filteredCache: filteredCache,
		offset:        -1,
		identifier:    identifier,
		generation:    generation,
	}, true
}

func nativePDFRenderStreamMutationShape(stream *object.EmeraldValue) bool {
	if stream == nil || stream.Type != object.ValueObject || stream.Frozen || core.AttachedSingletonClass(stream) != nil {
		return false
	}
	raw := nativePrawnTextLayoutIvar(stream, "@stream")
	if raw != nil && raw.Type != object.ValueNil && !nativePDFRenderASCIIValue(raw, true) {
		return false
	}
	return true
}

func nativePDFRenderASCIIValue(value *object.EmeraldValue, allowEmpty bool) bool {
	if value == nil || value.Type != object.ValueString || value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	raw, ok := value.Data.(string)
	if !ok || !allowEmpty && raw == "" {
		return false
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] >= 0x80 {
			return false
		}
	}
	return true
}

func nativePDFObjectIntegerIvarOK(value *object.EmeraldValue, name string) bool {
	_, ok := nativePDFObjectIntegerIvar(value, name)
	return ok
}

func nativePDFRenderCachedValueText(plan nativePDFRenderValuePlan, inContentStream bool) (string, bool) {
	if inContentStream {
		return plan.contentSerialized, plan.contentSerialized != ""
	}
	return plan.serialized, plan.serialized != ""
}

// nativePDFRenderCompileValue combines object-layout validation with the first
// serialization of composite values. The old region validated arrays/hashes
// and then walked them again during the writer; this compiler turns that into
// one guarded traversal and leaves an immutable text fragment for the writer.
// Separate content-stream text is retained because String encoding is
// intentionally different in a PDF content stream.
func nativePDFRenderCompileValue(value *object.EmeraldValue, inContentStream bool, seen map[*object.EmeraldValue]bool, referenceClass *object.Class, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) (string, bool) {
	if value == nil || value.Type == object.ValueNil {
		return "null", true
	}
	if (value.Type == object.ValueArray || value.Type == object.ValueHash) && valuePlans != nil && !seen[value] {
		if cached, found := valuePlans[value]; found && cached.kind == value.Type {
			if text, cachedOK := nativePDFRenderCachedValueText(cached, inContentStream); cachedOK {
				return text, true
			}
		}
	}
	switch value.Type {
	case object.ValueBool:
		truth, ok := value.Data.(bool)
		if !ok {
			return "", false
		}
		if truth {
			return "true", true
		}
		return "false", true
	case object.ValueInteger:
		if bigInteger := value.BigIntValue(); bigInteger != nil {
			return bigInteger.Text(10), true
		}
		integer, ok := value.Data.(int64)
		if !ok {
			return "", false
		}
		return strconv.FormatInt(integer, 10), true
	case object.ValueFloat:
		floatValue, ok := value.Data.(float64)
		if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return "", false
		}
		return nativePDFRealText(floatValue), true
	case object.ValueSymbol:
		name, ok := value.Data.(string)
		if !ok || value.Class != core.R.Classes["Symbol"] || core.AttachedSingletonClass(value) != nil {
			return "", false
		}
		return nativePDFSymbolText(name), true
	case object.ValueString:
		if value.Class != core.R.Classes["String"] || core.AttachedSingletonClass(value) != nil {
			return "", false
		}
		raw, ok := value.Data.(string)
		if !ok || !inContentStream && !utf8Valid(raw) {
			return "", false
		}
		return nativePDFStringText(value, inContentStream)
	case object.ValueArray:
		if value.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(value) != nil || seen[value] {
			return "", false
		}
		items, ok := nativePDFRenderArrayItemsFor(value, valuePlans)
		if !ok {
			return "", false
		}
		plan := valuePlans[value]
		plan.kind = object.ValueArray
		plan.items = items
		seen[value] = true
		var output strings.Builder
		output.Grow(2 + len(items)*4)
		output.WriteByte('[')
		for index, item := range items {
			if index != 0 {
				output.WriteByte(' ')
			}
			text, itemOK := nativePDFRenderCompileValue(item, inContentStream, seen, referenceClass, valuePlans)
			if !itemOK {
				delete(seen, value)
				return "", false
			}
			output.WriteString(text)
		}
		output.WriteByte(']')
		delete(seen, value)
		if inContentStream {
			plan.contentSerialized = output.String()
		} else {
			plan.serialized = output.String()
		}
		valuePlans[value] = plan
		if text, compiled := nativePDFRenderCachedValueText(plan, inContentStream); compiled {
			return text, true
		}
		return "", false
	case object.ValueHash:
		if !nativePDFStandardHash(value) || seen[value] {
			return "", false
		}
		hash, hashOK := value.Data.(*object.RHash)
		if !hashOK || hash == nil || len(hash.Keys) != len(hash.Pairs) {
			return "", false
		}
		for _, key := range hash.Keys {
			if key == nil || core.AttachedSingletonClass(key) != nil ||
				(key.Type != object.ValueString && key.Type != object.ValueSymbol) ||
				(key.Type == object.ValueString && key.Class != core.R.Classes["String"]) ||
				(key.Type == object.ValueSymbol && key.Class != core.R.Classes["Symbol"]) {
				return "", false
			}
		}
		entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
		if !ok {
			return "", false
		}
		plan := valuePlans[value]
		plan.kind = object.ValueHash
		plan.entries = entries
		seen[value] = true
		var output strings.Builder
		output.Grow(3 + len(entries)*12)
		output.WriteString("<< ")
		for _, entry := range entries {
			output.WriteString(nativePDFSymbolText(entry.keyName))
			output.WriteByte(' ')
			text, itemOK := nativePDFRenderCompileValue(entry.value, inContentStream, seen, referenceClass, valuePlans)
			if !itemOK {
				delete(seen, value)
				return "", false
			}
			output.WriteString(text)
			output.WriteByte('\n')
		}
		output.WriteString(">>")
		delete(seen, value)
		if inContentStream {
			plan.contentSerialized = output.String()
		} else {
			plan.serialized = output.String()
		}
		valuePlans[value] = plan
		if text, compiled := nativePDFRenderCachedValueText(plan, inContentStream); compiled {
			return text, true
		}
		return "", false
	case object.ValueObject:
		if value.Class != referenceClass || core.AttachedSingletonClass(value) != nil {
			return "", false
		}
		identifier, identifierOK := nativePDFObjectIntegerIvar(value, "@identifier")
		generation, generationOK := nativePDFObjectIntegerIvar(value, "@gen")
		if !identifierOK || !generationOK {
			return "", false
		}
		var output strings.Builder
		nativePDFRenderWriteInt(&output, identifier)
		output.WriteByte(' ')
		nativePDFRenderWriteInt(&output, generation)
		output.WriteString(" R")
		return output.String(), true
	default:
		return "", false
	}
}

func nativePDFRenderCompileCandidate(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	switch value.Type {
	case object.ValueArray:
		items, ok := nativePDFArrayItems(value)
		return ok && len(items) >= nativePDFRenderCompileMinEntries
	case object.ValueHash:
		if !nativePDFStandardHash(value) {
			return false
		}
		hash, ok := value.Data.(*object.RHash)
		return ok && hash != nil && len(hash.Keys) >= nativePDFRenderCompileMinEntries
	default:
		return false
	}
}

func nativePDFRenderValueShape(value *object.EmeraldValue, inContentStream bool, seen map[*object.EmeraldValue]bool, referenceClass *object.Class, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) bool {
	if valuePlans != nil && nativePDFRenderCompileCandidate(value) {
		_, ok := nativePDFRenderCompileValue(value, inContentStream, seen, referenceClass, valuePlans)
		return ok
	}
	if value == nil || value.Type == object.ValueNil {
		return true
	}
	if valuePlans != nil && (value.Type == object.ValueArray || value.Type == object.ValueHash) {
		if cached, found := valuePlans[value]; found && cached.kind == value.Type &&
			cached.inContentStream == inContentStream && !seen[value] {
			return true
		}
	}
	switch value.Type {
	case object.ValueBool:
		_, ok := value.Data.(bool)
		return ok
	case object.ValueInteger:
		if value.BigIntValue() != nil {
			return true
		}
		_, ok := value.Data.(int64)
		return ok
	case object.ValueFloat:
		v, ok := value.Data.(float64)
		return ok && !math.IsNaN(v) && !math.IsInf(v, 0)
	case object.ValueSymbol:
		_, ok := value.Data.(string)
		return ok && value.Class == core.R.Classes["Symbol"] && core.AttachedSingletonClass(value) == nil
	case object.ValueString:
		if value.Class != core.R.Classes["String"] || core.AttachedSingletonClass(value) != nil {
			return false
		}
		raw, ok := value.Data.(string)
		return ok && (inContentStream || utf8Valid(raw))
	case object.ValueArray:
		if value.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(value) != nil || seen[value] {
			return false
		}
		items, ok := nativePDFArrayItems(value)
		if !ok {
			return false
		}
		if valuePlans != nil {
			valuePlans[value] = nativePDFRenderValuePlan{kind: object.ValueArray, inContentStream: inContentStream, items: items}
		}
		seen[value] = true
		defer delete(seen, value)
		for _, item := range items {
			if !nativePDFRenderValueShape(item, inContentStream, seen, referenceClass, valuePlans) {
				return false
			}
		}
		return true
	case object.ValueHash:
		if !nativePDFStandardHash(value) || seen[value] {
			return false
		}
		hash, hashOK := value.Data.(*object.RHash)
		if !hashOK || hash == nil || len(hash.Keys) != len(hash.Pairs) {
			return false
		}
		for _, key := range hash.Keys {
			if key == nil || core.AttachedSingletonClass(key) != nil ||
				(key.Type != object.ValueString && key.Type != object.ValueSymbol) ||
				(key.Type == object.ValueString && key.Class != core.R.Classes["String"]) ||
				(key.Type == object.ValueSymbol && key.Class != core.R.Classes["Symbol"]) {
				return false
			}
		}
		entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
		if !ok {
			return false
		}
		if valuePlans != nil {
			valuePlans[value] = nativePDFRenderValuePlan{kind: object.ValueHash, inContentStream: inContentStream, entries: entries}
		}
		seen[value] = true
		defer delete(seen, value)
		for _, entry := range entries {
			if !nativePDFRenderValueShape(entry.value, inContentStream, seen, referenceClass, valuePlans) {
				return false
			}
		}
		return true
	case object.ValueObject:
		return value.Class == referenceClass && core.AttachedSingletonClass(value) == nil &&
			nativePDFObjectIntegerIvarOK(value, "@identifier") && nativePDFObjectIntegerIvarOK(value, "@gen")
	default:
		return false
	}
}

func nativePDFRenderHashHasKey(value *object.EmeraldValue, name string, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) bool {
	entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
	if !ok {
		return true
	}
	for _, entry := range entries {
		if entry.keyName == name {
			return true
		}
	}
	return false
}

func nativePDFRenderHashEntriesFor(value *object.EmeraldValue, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) ([]nativePDFHashEntry, bool) {
	if valuePlans != nil {
		if plan, found := valuePlans[value]; found && plan.kind == object.ValueHash {
			return plan.entries, true
		}
	}
	if entries, ok, specialized := nativePDFRenderSmallHashEntries(value); specialized {
		return entries, ok
	}
	return nativePDFHashEntries(value)
}

// nativePDFRenderSmallHashEntries is the renderer-only layout path for the
// ordinary small RHash shape. Prawn creates many dictionaries with only a few
// Symbol/String keys; allocating a duplicate-name map and invoking
// sort.SliceStable's reflection-heavy comparator for each one costs more than
// the actual PDF key ordering. The same duplicate and insertion-order rules
// are retained with a bounded quadratic check and insertion sort. Larger or
// non-standard hashes use the generic implementation unchanged.
func nativePDFRenderSmallHashEntries(value *object.EmeraldValue) ([]nativePDFHashEntry, bool, bool) {
	if !nativePDFStandardHash(value) {
		return nil, false, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil || len(hash.Keys) > 16 || len(hash.Keys) != len(hash.Pairs) {
		return nil, false, false
	}
	entries := make([]nativePDFHashEntry, 0, len(hash.Keys))
	for order, key := range hash.Keys {
		entryValue, exists := hash.Pairs[key]
		if !exists || key == nil || entryValue == nil || core.AttachedSingletonClass(key) != nil {
			return nil, false, true
		}
		var name string
		switch key.Type {
		case object.ValueString:
			if key.Class == nil || key.Class.Name != "String" {
				return nil, false, true
			}
			name, _ = key.Data.(string)
		case object.ValueSymbol:
			name, _ = key.Data.(string)
		default:
			return nil, false, true
		}
		for _, existing := range entries {
			if existing.keyName == name {
				return nil, false, true
			}
		}
		entries = append(entries, nativePDFHashEntry{keyName: name, value: entryValue, order: order})
	}
	for index := 1; index < len(entries); index++ {
		current := entries[index]
		position := index
		for position > 0 {
			previous := entries[position-1]
			if previous.keyName < current.keyName ||
				previous.keyName == current.keyName && previous.order <= current.order {
				break
			}
			entries[position] = previous
			position--
		}
		entries[position] = current
	}
	return entries, true, true
}

func nativePDFRenderArrayItemsFor(value *object.EmeraldValue, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) ([]*object.EmeraldValue, bool) {
	if valuePlans != nil {
		if plan, found := valuePlans[value]; found && plan.kind == object.ValueArray {
			return plan.items, true
		}
	}
	return nativePDFArrayItems(value)
}

func nativePDFRenderAppendQ(stream *object.EmeraldValue) (*object.EmeraldValue, bool) {
	data := nativePrawnTextLayoutIvar(stream, "@stream")
	if data == nil || data.Type == object.ValueNil {
		data = &object.EmeraldValue{Type: object.ValueString, Data: "", Class: core.R.Classes["String"], Encoding: "UTF-8"}
	}
	if !nativePDFRenderASCIIValue(data, true) {
		return nil, false
	}
	content := &object.EmeraldValue{Type: object.ValueString, Data: "Q", Class: core.R.Classes["String"], Encoding: "UTF-8"}
	if _, handled := core.AppendStringOneFast(data, content); !handled {
		return nil, false
	}
	if errVal := core.AppendASCIIBytes(data, "\n"); errVal != nil {
		return errVal, true
	}
	if result := core.SetDynamicInstanceVar(stream, "@stream", data); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(stream, "@filtered_stream", core.R.NilVal); result != nil {
		return result, true
	}
	return nil, true
}

func nativePDFRenderReference(output *strings.Builder, ref *nativePDFRenderReferencePlan, seen map[*object.EmeraldValue]bool, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) bool {
	if output == nil || ref == nil || ref.ref == nil || ref.stream == nil {
		return false
	}
	nativePDFRenderWriteInt(output, ref.identifier)
	output.WriteByte(' ')
	nativePDFRenderWriteInt(output, ref.generation)
	output.WriteString(" obj\n")
	if ref.raw == nil || ref.raw.Type == object.ValueNil {
		if !nativePDFRenderWriteObjectText(output, ref.data, false, seen, valuePlans) {
			return false
		}
		output.WriteString("\nendobj\n")
		return true
	}
	filtered := ref.filtered
	if !ref.filteredCache {
		var ok bool
		filtered, ok = nativePDFRenderStreamPayload(ref.stream, ref.raw)
		if !ok {
			return false
		}
	}
	if !nativePDFRenderWriteHashWithLength(output, ref.data, len(filtered), seen, valuePlans) {
		return false
	}
	output.WriteString("\nstream\n")
	output.WriteString(filtered)
	output.WriteString("\nendstream\nendobj\n")
	return true
}

// nativePDFRenderWriteInt keeps the serializer's numeric ABI on strconv's
// typed append path. The stack buffer is copied into the builder and avoids
// fmt's interface formatting and per-value reflection in reference/xref hot
// loops.
func nativePDFRenderWriteInt(output *strings.Builder, value int64) {
	var buffer [32]byte
	output.Write(strconv.AppendInt(buffer[:0], value, 10))
}

func nativePDFRenderWriteIntegerValue(output *strings.Builder, value *object.EmeraldValue) bool {
	if output == nil || value == nil {
		return false
	}
	if bigInteger := value.BigIntValue(); bigInteger != nil {
		output.WriteString(bigInteger.Text(10))
		return true
	}
	integer, ok := value.Data.(int64)
	if !ok {
		return false
	}
	nativePDFRenderWriteInt(output, integer)
	return true
}

func nativePDFRenderWriteHexByte(output *strings.Builder, value byte) {
	const digits = "0123456789abcdef"
	output.WriteByte(digits[value>>4])
	output.WriteByte(digits[value&0x0f])
}

// nativePDFRenderWriteSymbol is the allocation-free form of
// nativePDFSymbolText used after the renderer preflight has proved a Symbol
// key/value. Keep the exact PDF name escaping rules in this local writer so
// the ordinary nativePDFObject ABI remains unchanged.
func nativePDFRenderWriteSymbol(output *strings.Builder, name string) {
	const upper = "0123456789ABCDEF"
	output.WriteByte('/')
	for index := 0; index < len(name); index++ {
		value := name[index]
		if value <= 32 || value == 35 || value == 40 || value == 41 || value == 47 ||
			value == 60 || value == 62 || value >= 127 {
			output.WriteByte('#')
			output.WriteByte(upper[value>>4])
			output.WriteByte(upper[value&0x0f])
			continue
		}
		output.WriteByte(value)
	}
}

// nativePDFRenderWriteASCIIString handles the overwhelmingly common String
// shape without allocating the intermediate UTF-16/hex byte slice. The
// validated non-ASCII path still delegates to nativePDFStringText below.
func nativePDFRenderWriteASCIIString(output *strings.Builder, raw string, inContentStream bool) bool {
	if output == nil {
		return false
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] >= 0x80 {
			return false
		}
	}
	output.WriteByte('<')
	if !inContentStream {
		output.WriteString("feff")
	}
	for index := 0; index < len(raw); index++ {
		if !inContentStream {
			output.WriteString("00")
		}
		nativePDFRenderWriteHexByte(output, raw[index])
	}
	output.WriteByte('>')
	return true
}

func nativePDFRenderWritePaddedOffset(output *strings.Builder, value int64) bool {
	if output == nil || value < 0 {
		return false
	}
	var buffer [32]byte
	encoded := strconv.AppendInt(buffer[:0], value, 10)
	if len(encoded) > 10 {
		return false
	}
	for index := len(encoded); index < 10; index++ {
		output.WriteByte('0')
	}
	output.Write(encoded)
	return true
}

func nativePDFRenderStreamPayload(stream, raw *object.EmeraldValue) (string, bool) {
	cached := nativePrawnTextLayoutIvar(stream, "@filtered_stream")
	if cached != nil && cached.Type != object.ValueNil {
		return cached.Data.(string), true
	}
	clone := nativePDFCloneString(raw)
	if clone == nil {
		return "", false
	}
	core.TrackObjectSpaceValue(clone)
	if result := core.SetDynamicInstanceVar(stream, "@filtered_stream", clone); result != nil {
		return "", false
	}
	return clone.Data.(string), true
}

func nativePDFRenderHashWithLength(value *object.EmeraldValue, length int, seen map[*object.EmeraldValue]bool, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) (string, bool) {
	entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
	if !ok {
		return "", false
	}
	for _, entry := range entries {
		if entry.keyName == "Length" {
			return "", false
		}
	}
	entries = append(append([]nativePDFHashEntry(nil), entries...), nativePDFHashEntry{keyName: "Length", value: core.NewIntegerValue(int64(length)), order: len(entries)})
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].keyName == entries[right].keyName {
			return entries[left].order < entries[right].order
		}
		return entries[left].keyName < entries[right].keyName
	})
	var output strings.Builder
	output.WriteString("<< ")
	for _, entry := range entries {
		nativePDFRenderWriteSymbol(&output, entry.keyName)
		output.WriteByte(' ')
		if !nativePDFRenderWriteObjectText(&output, entry.value, false, seen, valuePlans) {
			return "", false
		}
		output.WriteByte('\n')
	}
	output.WriteString(">>")
	return output.String(), true
}

// nativePDFRenderWriteHashWithLength is the steady-state form used by the
// renderer region. Hash entries are already sorted by nativePDFHashEntries
// during preflight, so the hot pass only merges the synthetic Length entry
// into that order and writes directly to the final output builder. This keeps
// one temporary hash slice and one temporary serialized string out of every
// stream reference while preserving PDF::Core's key order.
func nativePDFRenderWriteHashWithLength(output *strings.Builder, value *object.EmeraldValue, length int, seen map[*object.EmeraldValue]bool, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) bool {
	if output == nil {
		return false
	}
	entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
	if !ok {
		return false
	}
	for _, entry := range entries {
		if entry.keyName == "Length" {
			return false
		}
	}
	output.WriteString("<< ")
	lengthWritten := false
	writeLength := func() {
		nativePDFRenderWriteSymbol(output, "Length")
		output.WriteByte(' ')
		nativePDFRenderWriteInt(output, int64(length))
		output.WriteByte('\n')
		lengthWritten = true
	}
	for _, entry := range entries {
		if !lengthWritten && entry.keyName > "Length" {
			writeLength()
		}
		nativePDFRenderWriteSymbol(output, entry.keyName)
		output.WriteByte(' ')
		if !nativePDFRenderWriteObjectText(output, entry.value, false, seen, valuePlans) {
			return false
		}
		output.WriteByte('\n')
	}
	if !lengthWritten {
		writeLength()
	}
	output.WriteString(">>")
	return true
}

// nativePDFRenderWriteObjectText is the renderer-only serializer. The
// preflight has already proved the object graph's class/layout shape, so the
// hot pass writes composite values directly into the final builder instead of
// allocating one temporary string per nested array/hash/reference. The
// generic nativePDFObjectText path remains unchanged for ordinary PDF calls.
func nativePDFRenderWriteObjectText(output *strings.Builder, value *object.EmeraldValue, inContentStream bool, seen map[*object.EmeraldValue]bool, valuePlans map[*object.EmeraldValue]nativePDFRenderValuePlan) bool {
	if output == nil {
		return false
	}
	if value == nil || value.Type == object.ValueNil {
		output.WriteString("null")
		return true
	}
	if (value.Type == object.ValueArray || value.Type == object.ValueHash) && valuePlans != nil && !seen[value] {
		if plan, found := valuePlans[value]; found && plan.kind == value.Type {
			if text, compiled := nativePDFRenderCachedValueText(plan, inContentStream); compiled {
				output.WriteString(text)
				return true
			}
		}
	}
	switch value.Type {
	case object.ValueBool:
		truth, ok := value.Data.(bool)
		if !ok {
			return false
		}
		if truth {
			output.WriteString("true")
		} else {
			output.WriteString("false")
		}
		return true
	case object.ValueInteger:
		return nativePDFRenderWriteIntegerValue(output, value)
	case object.ValueFloat:
		v, ok := value.Data.(float64)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
		output.WriteString(nativePDFRealText(v))
		return true
	case object.ValueSymbol:
		name, ok := value.Data.(string)
		if !ok {
			return false
		}
		nativePDFRenderWriteSymbol(output, name)
		return true
	case object.ValueString:
		if value.Class != core.R.Classes["String"] || core.AttachedSingletonClass(value) != nil {
			return false
		}
		raw, ok := value.Data.(string)
		if !ok {
			return false
		}
		if nativePDFRenderWriteASCIIString(output, raw, inContentStream) {
			return true
		}
		text, textOK := nativePDFStringText(value, inContentStream)
		if !textOK {
			return false
		}
		output.WriteString(text)
		return true
	case object.ValueArray:
		if value.Class != core.R.Classes["Array"] || seen != nil && seen[value] {
			return false
		}
		items, ok := nativePDFRenderArrayItemsFor(value, valuePlans)
		if !ok {
			return false
		}
		if seen != nil {
			seen[value] = true
			defer delete(seen, value)
		}
		output.WriteByte('[')
		for index, item := range items {
			if index != 0 {
				output.WriteByte(' ')
			}
			if !nativePDFRenderWriteObjectText(output, item, inContentStream, seen, valuePlans) {
				return false
			}
		}
		output.WriteByte(']')
		return true
	case object.ValueHash:
		if value.Class != core.R.Classes["Hash"] || seen != nil && seen[value] {
			return false
		}
		entries, ok := nativePDFRenderHashEntriesFor(value, valuePlans)
		if !ok {
			return false
		}
		if seen != nil {
			seen[value] = true
			defer delete(seen, value)
		}
		output.WriteString("<< ")
		for _, entry := range entries {
			nativePDFRenderWriteSymbol(output, entry.keyName)
			output.WriteByte(' ')
			if !nativePDFRenderWriteObjectText(output, entry.value, inContentStream, seen, valuePlans) {
				return false
			}
			output.WriteByte('\n')
		}
		output.WriteString(">>")
		return true
	case object.ValueObject:
		if value.Class == nil || value.Class.Name != "PDF::Core::Reference" {
			return false
		}
		identifier, identifierOK := nativePDFObjectIntegerIvar(value, "@identifier")
		generation, generationOK := nativePDFObjectIntegerIvar(value, "@gen")
		if !identifierOK || !generationOK {
			return false
		}
		nativePDFRenderWriteInt(output, identifier)
		output.WriteByte(' ')
		nativePDFRenderWriteInt(output, generation)
		output.WriteString(" R")
		return true
	default:
		return false
	}
}
