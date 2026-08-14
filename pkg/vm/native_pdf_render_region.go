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
	layoutTemplates    map[nativePDFRenderLayoutTemplateKey][]*nativePDFRenderLayoutTemplate
}

type nativePDFRenderPagePlan struct {
	page        *object.EmeraldValue
	state       *object.EmeraldValue
	stackValues *object.EmeraldValue
	items       []*object.EmeraldValue
	stream      *object.EmeraldValue
}

type nativePDFRenderReferencePlan struct {
	ref                     *object.EmeraldValue
	data                    *object.EmeraldValue
	stream                  *object.EmeraldValue
	raw                     *object.EmeraldValue
	rawText                 string
	filtered                string
	filteredValue           *object.EmeraldValue
	filteredCache           bool
	offset                  int64
	identifier              int64
	generation              int64
	refLayoutGeneration     uint64
	streamLayoutGeneration  uint64
	filtersLayoutGeneration uint64
	filtersValue            *object.EmeraldValue
	listValue               *object.EmeraldValue
	listLength              int
	layoutCacheable         bool
}

type nativePDFRenderValuePlan struct {
	kind              object.ValueType
	inContentStream   bool
	items             []*object.EmeraldValue
	entries           []nativePDFHashEntry
	serialized        string
	contentSerialized string
}

// nativePDFRenderLayoutTemplate is the reusable shape half of the renderer
// region. The first render still proves the ordinary Ruby-visible graph, then
// records only value kinds, array edges, and sorted dictionary key edges.
// Subsequent documents bind their fresh EmeraldValue graph to these nodes
// without rebuilding a per-render recursive plan map.
type nativePDFRenderLayoutTemplateKey struct {
	references int
	pages      int
}

type nativePDFRenderLayoutEntry struct {
	keyName  string
	key      *object.EmeraldValue
	keyClass *object.Class
	node     int
}

type nativePDFRenderLayoutNode struct {
	kind        object.ValueType
	asciiOnly   bool
	floatValue  float64
	floatText   string
	floatStatic bool
	items       []int
	entries     []nativePDFRenderLayoutEntry
}

type nativePDFRenderLayoutWriteOpKind uint8

const (
	nativePDFRenderLayoutWriteLiteral nativePDFRenderLayoutWriteOpKind = iota
	nativePDFRenderLayoutWriteLength
	nativePDFRenderLayoutWriteNil
	nativePDFRenderLayoutWriteBool
	nativePDFRenderLayoutWriteInteger
	nativePDFRenderLayoutWriteFloat
	nativePDFRenderLayoutWriteSymbol
	nativePDFRenderLayoutWriteString
	nativePDFRenderLayoutWriteASCIIString
	nativePDFRenderLayoutWriteReference
)

type nativePDFRenderLayoutWriteOp struct {
	kind nativePDFRenderLayoutWriteOpKind
	node int
	text string
}

type nativePDFRenderLayoutReference struct {
	dataNode      int
	rawPresent    bool
	filteredCache bool
}

type nativePDFRenderLayoutTemplate struct {
	key                 nativePDFRenderLayoutTemplateKey
	rootNode            int
	infoNode            int
	signature           string
	referenceClass      *object.Class
	streamClass         *object.Class
	filterListClass     *object.Class
	referenceSlots      nativePDFRenderObjectSlots
	streamSlots         nativePDFRenderObjectSlots
	filterListSlots     nativePDFRenderObjectSlots
	arrayClass          *object.Class
	hashClass           *object.Class
	stringClass         *object.Class
	symbolClass         *object.Class
	nodes               []nativePDFRenderLayoutNode
	refs                []nativePDFRenderLayoutReference
	writePrograms       [][]nativePDFRenderLayoutWriteOp
	writeLengthPrograms [][]nativePDFRenderLayoutWriteOp

	// A VM executes one Ruby frame at a time. Keep the transient binding and
	// reference/page plans on the per-VM template so a hot render does not
	// allocate a second object graph description on every call. The slices are
	// never exposed to Ruby and are overwritten before the next use.
	scratchBound          *nativePDFRenderBoundLayout
	scratchRefs           []*object.EmeraldValue
	scratchData           []*object.EmeraldValue
	scratchReferencePlans []nativePDFRenderReferencePlan
	scratchPagePlans      []nativePDFRenderPagePlan
	scratchPlan           *nativePDFRenderRegionPlan
	outputCache           nativePDFRenderLayoutOutputCache
}

// nativePDFRenderLayoutNodeSnapshot is the value half of the reusable typed
// layout. The ordinary fast binder proves graph shape and object generations;
// the snapshot additionally proves scalar/string contents before a complete
// serialized PDF is reused. Without it, an in-place String mutation could
// preserve the same EmeraldValue pointer while changing the cached bytes.
type nativePDFRenderLayoutNodeSnapshot struct {
	kind       object.ValueType
	boolValue  bool
	integer    int64
	integerBig string
	float      float64
	text       string
	encoding   string
	identifier int64
	generation int64
}

type nativePDFRenderLayoutPageCache struct {
	page        *object.EmeraldValue
	state       *object.EmeraldValue
	stackValues *object.EmeraldValue
	items       []*object.EmeraldValue
}

// nativePDFRenderLayoutOutputCache is deliberately attached to a proven
// template. It is only consulted after the existing strict layout binder and
// stream/reference guards succeed, so any uncertainty returns to the normal
// typed writer or Ruby implementation.
type nativePDFRenderLayoutOutputCache struct {
	valid              bool
	renderer           *object.EmeraldValue
	state              *object.EmeraldValue
	store              *object.EmeraldValue
	identifiers        *object.EmeraldValue
	objects            *object.EmeraldValue
	root               *object.EmeraldValue
	info               *object.EmeraldValue
	pageValues         *object.EmeraldValue
	version            float64
	bound              *nativePDFRenderBoundLayout
	refs               []*object.EmeraldValue
	refPlans           []nativePDFRenderReferencePlan
	pages              []nativePDFRenderLayoutPageCache
	offsets            []int64
	xrefOffset         int64
	output             string
	nodes              []nativePDFRenderLayoutNodeSnapshot
	mutationGeneration uint64
}

type nativePDFRenderObjectSlots struct {
	identifier     int
	generation     int
	data           int
	stream         int
	filters        int
	list           int
	filteredStream int
}

type nativePDFRenderBoundLayout struct {
	template          *nativePDFRenderLayoutTemplate
	values            []*object.EmeraldValue
	bound             []uint32
	epoch             uint32
	boundCount        int
	trusted           bool
	identifiers       []int64
	generations       []int64
	objectGenerations []uint64
}

type nativePDFRenderRegionInputs struct {
	state       *object.EmeraldValue
	store       *object.EmeraldValue
	identifiers *object.EmeraldValue
	objects     *object.EmeraldValue
	root        *object.EmeraldValue
	info        *object.EmeraldValue
	pageValues  *object.EmeraldValue
	ids         []*object.EmeraldValue
	pageItems   []*object.EmeraldValue
	version     float64
}

type nativePDFRenderRegionPlan struct {
	renderer               *object.EmeraldValue
	state                  *object.EmeraldValue
	store                  *object.EmeraldValue
	identifiers            *object.EmeraldValue
	objects                *object.EmeraldValue
	refs                   []nativePDFRenderReferencePlan
	pages                  []nativePDFRenderPagePlan
	pageValues             *object.EmeraldValue
	root                   *object.EmeraldValue
	info                   *object.EmeraldValue
	version                float64
	valuePlans             map[*object.EmeraldValue]nativePDFRenderValuePlan
	layout                 *nativePDFRenderBoundLayout
	cachedOutput           bool
	cachedOutputText       string
	cachedOutputRefs       []*object.EmeraldValue
	cachedOutputPages      []nativePDFRenderPagePlan
	cachedOutputOffsets    []int64
	cachedOutputXrefOffset int64
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
	constantGeneration := object.CurrentConstantGeneration()
	plan, ok := vm.nativePDFRenderRegionPlan(receiver, stateClass, storeClass, referenceClass, streamClass, filterListClass, pageClass, stackClass)
	if !ok || object.CurrentMethodGeneration() != generation || object.CurrentConstantGeneration() != constantGeneration {
		return nil, false
	}
	if plan.cachedOutput {
		return vm.nativePDFRenderReplayCachedOutput(plan, receiver)
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
			object.BumpRenderMutationGeneration()
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(int64(len(plan.pages)))); result != nil {
		return result, true
	}

	var output strings.Builder
	output.Grow(1024 + len(plan.refs)*96)
	output.WriteString("%PDF-")
	output.WriteString(nativePDFRenderRealText(plan.version))
	output.WriteString("\n%\xFF\xFF\xFF\xFF\n")
	// The complete object graph was cycle-checked by nativePDFRenderRegionPlan
	// before any writer mutation. No Ruby callback can run between that proof
	// and this trusted pass, so keep seen nil here and avoid a map read/write on
	// every small composite. A bound layout uses node indexes instead of the
	// per-render value-plan map and keeps the same side-exit contract.
	var seen map[*object.EmeraldValue]bool
	for index := range plan.refs {
		ref := &plan.refs[index]
		ref.offset = int64(output.Len())
		if result := core.SetDynamicInstanceVar(ref.ref, "@offset", core.NewIntegerValue(ref.offset)); result != nil {
			return result, true
		}
		if plan.layout != nil {
			if !nativePDFRenderReferenceLayout(&output, ref, plan.layout, index) {
				return nil, false
			}
		} else if !nativePDFRenderReference(&output, ref, seen, plan.valuePlans) {
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

	output.WriteString("trailer\n")
	if plan.layout != nil {
		if !nativePDFRenderWriteLayoutTrailer(&output, plan.layout, len(plan.refs)+1) {
			return nil, false
		}
	} else {
		trailer := nativePDFHashValue(
			[2]*object.EmeraldValue{nativePDFSymbol("Size"), core.NewIntegerValue(int64(len(plan.refs) + 1))},
			[2]*object.EmeraldValue{nativePDFSymbol("Root"), plan.root},
			[2]*object.EmeraldValue{nativePDFSymbol("Info"), plan.info},
		)
		if !nativePDFRenderWriteObjectText(&output, trailer, false, seen, plan.valuePlans) {
			return nil, false
		}
	}
	output.WriteString("\nstartxref\n")
	nativePDFRenderWriteInt(&output, int64(xrefOffset))
	output.WriteString("\n%%EOF\n")
	serialized := output.String()
	if plan.layout != nil {
		nativePDFRenderRememberLayoutOutput(plan, serialized, int64(xrefOffset))
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: serialized, Class: core.R.Classes["String"], Encoding: "ASCII-8BIT"}, true
}

// nativePDFRenderReplayCachedOutput keeps the Ruby-visible state changes of
// Renderer#render while skipping the already-proven finalizer and serializer.
// The cache guard has run before this function, so these writes are the only
// observable work left on the hot path.
func (vm *VM) nativePDFRenderReplayCachedOutput(plan *nativePDFRenderRegionPlan, receiver *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || plan == nil || !plan.cachedOutput || receiver == nil || plan.state == nil ||
		len(plan.cachedOutputRefs) != len(plan.cachedOutputOffsets) || plan.cachedOutputText == "" {
		return nil, false
	}
	for _, page := range plan.cachedOutputPages {
		if result := core.SetDynamicInstanceVar(plan.state, "@page", page.page); result != nil {
			return result, true
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(int64(len(plan.cachedOutputPages)))); result != nil {
		return result, true
	}
	for index, ref := range plan.cachedOutputRefs {
		if ref == nil {
			return nil, false
		}
		if result := core.SetDynamicInstanceVar(ref, "@offset", core.NewIntegerValue(plan.cachedOutputOffsets[index])); result != nil {
			return result, true
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, "@xref_offset", core.NewIntegerValue(plan.cachedOutputXrefOffset)); result != nil {
		return result, true
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: plan.cachedOutputText, Class: core.R.Classes["String"], Encoding: "ASCII-8BIT"}, true
}

// nativePDFRenderWriteLayoutTrailer is the fixed trailer shape produced by
// PDF::Core::Renderer#render. Root and Info were already bound and guarded as
// Reference nodes, so materializing a temporary Hash and invoking the generic
// object serializer would only repeat work already proved by the layout.
func nativePDFRenderWriteLayoutTrailer(output *strings.Builder, layout *nativePDFRenderBoundLayout, size int) bool {
	if output == nil || layout == nil || layout.template == nil || size < 1 {
		return false
	}
	output.WriteString("<< /Info ")
	if !nativePDFRenderWriteLayoutReference(output, layout, layout.template.infoNode) {
		return false
	}
	output.WriteString("\n/Root ")
	if !nativePDFRenderWriteLayoutReference(output, layout, layout.template.rootNode) {
		return false
	}
	output.WriteString("\n/Size ")
	nativePDFRenderWriteInt(output, int64(size))
	output.WriteString("\n>>")
	return true
}

func nativePDFRenderWriteLayoutReference(output *strings.Builder, layout *nativePDFRenderBoundLayout, nodeIndex int) bool {
	if output == nil || layout == nil || layout.template == nil || nodeIndex < 0 ||
		nodeIndex >= len(layout.template.nodes) || nodeIndex >= len(layout.values) ||
		nodeIndex >= len(layout.bound) || nodeIndex >= len(layout.identifiers) ||
		nodeIndex >= len(layout.generations) || layout.bound[nodeIndex] != layout.epoch ||
		layout.template.nodes[nodeIndex].kind != object.ValueObject {
		return false
	}
	nativePDFRenderWriteInt(output, layout.identifiers[nodeIndex])
	output.WriteByte(' ')
	nativePDFRenderWriteInt(output, layout.generations[nodeIndex])
	output.WriteString(" R")
	return true
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
		layoutTemplates:    make(map[nativePDFRenderLayoutTemplateKey][]*nativePDFRenderLayoutTemplate),
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
	inputs, ok := nativePDFRenderRegionInputsFor(receiver, stateClass, storeClass)
	if !ok {
		return nil, false
	}
	if plan, fast := vm.nativePDFRenderLayoutPlanFor(inputs, receiver, referenceClass, streamClass, filterListClass, pageClass, stackClass); fast {
		return plan, true
	}

	// Most composites are owned by one reference, so the reference count is a
	// useful lower bound for both the per-render object-layout cache and the
	// cycle proof. Reserving it here keeps the first render pass out of the
	// map-growth path while still allowing unusual nested graphs to grow
	// normally.
	planCapacity := len(inputs.ids)
	if planCapacity < 16 {
		planCapacity = 16
	}
	valuePlans := make(map[*object.EmeraldValue]nativePDFRenderValuePlan, planCapacity)
	seen := make(map[*object.EmeraldValue]bool, planCapacity)
	if !nativePDFRenderValueShape(inputs.root, false, seen, referenceClass, valuePlans) ||
		!nativePDFRenderValueShape(inputs.info, false, seen, referenceClass, valuePlans) {
		return nil, false
	}
	plan := &nativePDFRenderRegionPlan{
		renderer:    receiver,
		state:       inputs.state,
		store:       inputs.store,
		identifiers: inputs.identifiers,
		objects:     inputs.objects,
		refs:        make([]nativePDFRenderReferencePlan, 0, len(inputs.ids)),
		pages:       make([]nativePDFRenderPagePlan, 0, len(inputs.pageItems)),
		pageValues:  inputs.pageValues,
		root:        inputs.root,
		info:        inputs.info,
		version:     inputs.version,
		valuePlans:  valuePlans,
	}
	for _, id := range inputs.ids {
		if id == nil || id.Type != object.ValueInteger {
			return nil, false
		}
		ref, found := core.DirectHashIndex(inputs.objects, id)
		if !found {
			return nil, false
		}
		refPlan, ok := nativePDFRenderReferencePlanFor(ref, referenceClass, streamClass, filterListClass, seen, valuePlans)
		if !ok {
			return nil, false
		}
		plan.refs = append(plan.refs, refPlan)
	}
	for _, page := range inputs.pageItems {
		pagePlan, ok := nativePDFBuildPagePlan(page, inputs.state, pageClass, stackClass)
		if !ok {
			return nil, false
		}
		plan.pages = append(plan.pages, pagePlan)
	}
	if template := nativePDFRenderLayoutTemplateFor(plan, referenceClass); template != nil {
		template.streamClass = streamClass
		template.streamSlots = nativePDFRenderObjectSlotsFor(streamClass, "@stream", "@filtered_stream", "@filters")
		template.filterListClass = filterListClass
		template.filterListSlots = nativePDFRenderObjectSlotsFor(filterListClass, "@list")
		nativePDFRenderInstallLayoutTemplate(vm, template)
	}
	return plan, true
}

func nativePDFRenderRegionInputsFor(receiver *object.EmeraldValue, stateClass, storeClass *object.Class) (nativePDFRenderRegionInputs, bool) {
	state := nativePrawnTextLayoutIvar(receiver, "@state")
	store := nativePrawnTextLayoutIvar(state, "@store")
	if state == nil || state.Type != object.ValueObject || state.Class != stateClass || state.Frozen ||
		store == nil || store.Type != object.ValueObject || store.Class != storeClass || store.Frozen ||
		!nativePDFRenderObjectSingletonFree(state) || !nativePDFRenderObjectSingletonFree(store) {
		return nativePDFRenderRegionInputs{}, false
	}
	versionValue := nativePrawnTextLayoutIvar(state, "@version")
	version, versionOK := float64(0), false
	if versionValue != nil {
		version, versionOK = versionValue.Data.(float64)
	}
	if versionValue == nil || versionValue.Type != object.ValueFloat || versionValue.Class != core.R.Classes["Float"] ||
		core.AttachedSingletonClass(versionValue) != nil || !versionOK || math.IsNaN(version) || math.IsInf(version, 0) || version != 1.3 {
		return nativePDFRenderRegionInputs{}, false
	}
	if compress := nativePrawnTextLayoutIvar(state, "@compress"); compress == nil || compress.Type != object.ValueBool || compress.Data != false {
		return nativePDFRenderRegionInputs{}, false
	}
	if encrypt := nativePrawnTextLayoutIvar(state, "@encrypt"); encrypt == nil || encrypt.Type != object.ValueBool || encrypt.Data != false {
		return nativePDFRenderRegionInputs{}, false
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
		return nativePDFRenderRegionInputs{}, false
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
		return nativePDFRenderRegionInputs{}, false
	}
	rootID := nativePrawnTextLayoutIvar(store, "@root")
	infoID := nativePrawnTextLayoutIvar(store, "@info")
	if rootID == nil || rootID.Type != object.ValueInteger || infoID == nil || infoID.Type != object.ValueInteger {
		return nativePDFRenderRegionInputs{}, false
	}
	root, rootOK := core.DirectHashIndex(objects, rootID)
	info, infoOK := core.DirectHashIndex(objects, infoID)
	if !rootOK || !infoOK {
		return nativePDFRenderRegionInputs{}, false
	}

	pageValues := nativePrawnTextLayoutIvar(state, "@pages")
	pageItems, pagesOK := ([]*object.EmeraldValue)(nil), false
	if pageValues != nil {
		pageItems, pagesOK = pageValues.Data.([]*object.EmeraldValue)
	}
	if pageValues == nil || pageValues.Type != object.ValueArray || pageValues.Class != core.R.Classes["Array"] ||
		pageValues.Frozen || core.AttachedSingletonClass(pageValues) != nil || !pagesOK || len(pageItems) == 0 {
		return nativePDFRenderRegionInputs{}, false
	}
	return nativePDFRenderRegionInputs{
		state:       state,
		store:       store,
		identifiers: identifiers,
		objects:     objects,
		root:        root,
		info:        info,
		pageValues:  pageValues,
		ids:         ids,
		pageItems:   pageItems,
		version:     version,
	}, true
}

// nativePDFRenderObjectIvar is the renderer's exact object-layout read. All
// callers use it only after the surrounding ABI has proven an ordinary Ruby
// object; compact/hot-sidecar values retain the historical flush-aware read.
// Keeping this fallback local lets the common map-backed Prawn shape avoid the
// generic value-kind dispatch on every Reference field.
func nativePDFRenderObjectIvar(value *object.EmeraldValue, name string) *object.EmeraldValue {
	if value != nil && value.Type == object.ValueObject {
		if data, ok := value.Data.(*object.Object); ok && data != nil {
			if data.HotIntegerInstanceVarMask != 0 || data.Class != nil && data.Class.CompactInstanceVars && data.InstanceVars == nil {
				return data.GetInstanceVar(name)
			}
			if data.InstanceVars != nil {
				return data.InstanceVars[name]
			}
			return nil
		}
	}
	return core.DynamicInstanceVar(value, name)
}

// nativePDFRenderLayoutObjectIvar reads a previously resolved inline slot for
// the exact PDF object class. The slot is only a cache: Object's generation
// invalidates it after a Ruby ivar write, and the miss refreshes from the
// ordinary map/compact representation. Unsupported layouts fall back to the
// historical renderer read and therefore side-exit safely.
func nativePDFRenderLayoutObjectIvar(value *object.EmeraldValue, expectedClass *object.Class, slot int, name string) *object.EmeraldValue {
	if value == nil || value.Type != object.ValueObject || expectedClass == nil || slot < 0 || slot >= len((&object.Object{}).InlineInstanceVars) || value.Class != expectedClass {
		return nativePDFRenderObjectIvar(value, name)
	}
	data, ok := value.Data.(*object.Object)
	if !ok || data == nil || data.Class != expectedClass || data.HotIntegerInstanceVarMask != 0 {
		return nativePDFRenderObjectIvar(value, name)
	}
	bit := uint8(1 << slot)
	if data.InlineInstanceVarMask&bit != 0 && data.InlineInstanceVarGenerations[slot] == data.InstanceVarGeneration {
		return data.InlineInstanceVars[slot]
	}
	var result *object.EmeraldValue
	if data.InstanceVars != nil {
		result = data.InstanceVars[name]
	} else {
		result = data.GetInstanceVar(name)
	}
	data.InlineInstanceVars[slot] = result
	data.InlineInstanceVarMask |= bit
	data.InlineInstanceVarGenerations[slot] = data.InstanceVarGeneration
	return result
}

func nativePDFRenderLayoutObjectIntegerIvar(value *object.EmeraldValue, expectedClass *object.Class, slot int, name string) (int64, bool) {
	item := nativePDFRenderLayoutObjectIvar(value, expectedClass, slot, name)
	if item == nil || item.Type != object.ValueInteger || item.BigIntValue() != nil {
		return 0, false
	}
	integer, ok := item.Data.(int64)
	return integer, ok
}

func nativePDFRenderObjectIntegerIvar(value *object.EmeraldValue, name string) (int64, bool) {
	item := nativePDFRenderObjectIvar(value, name)
	if item == nil || item.Type != object.ValueInteger || item.BigIntValue() != nil {
		return 0, false
	}
	integer, ok := item.Data.(int64)
	return integer, ok
}

// nativePDFRenderLayoutFastBind validates the already-bound graph using the
// exact edges recorded by the template. It is intentionally stricter than a
// general Hash/Array lookup: a changed key identity, child pointer, mutable
// scalar, or promoted object generation immediately falls through to the
// full binder below. The fast path therefore only removes repeated proof; it
// never widens the accepted Ruby shape.
func nativePDFRenderLayoutFastBind(template *nativePDFRenderLayoutTemplate, bound *nativePDFRenderBoundLayout, root, info *object.EmeraldValue, refData []*object.EmeraldValue, referenceClass *object.Class) bool {
	if template == nil || bound == nil || !bound.trusted || bound.template != template || referenceClass == nil ||
		len(bound.values) != len(template.nodes) || len(bound.bound) != len(template.nodes) ||
		len(bound.objectGenerations) != len(template.nodes) || len(refData) != len(template.refs) {
		return false
	}
	if template.rootNode < 0 || template.rootNode >= len(bound.values) || template.infoNode < 0 || template.infoNode >= len(bound.values) ||
		bound.bound[template.rootNode] != bound.epoch || bound.bound[template.infoNode] != bound.epoch ||
		bound.values[template.rootNode] != nativePDFRenderLayoutCanonicalValue(root) ||
		bound.values[template.infoNode] != nativePDFRenderLayoutCanonicalValue(info) {
		return false
	}
	for index, data := range refData {
		nodeIndex := template.refs[index].dataNode
		if nodeIndex < 0 || nodeIndex >= len(bound.values) || bound.bound[nodeIndex] != bound.epoch ||
			bound.values[nodeIndex] != nativePDFRenderLayoutCanonicalValue(data) {
			return false
		}
	}
	for index, node := range template.nodes {
		if bound.bound[index] != bound.epoch || !nativePDFRenderLayoutFastNodeStillValid(template, node, bound, index, referenceClass) {
			return false
		}
	}
	return true
}

func nativePDFRenderLayoutFastNodeStillValid(template *nativePDFRenderLayoutTemplate, node nativePDFRenderLayoutNode, bound *nativePDFRenderBoundLayout, nodeIndex int, referenceClass *object.Class) bool {
	if template == nil || bound == nil || nodeIndex < 0 || nodeIndex >= len(bound.values) ||
		nodeIndex >= len(bound.objectGenerations) {
		return false
	}
	value := bound.values[nodeIndex]
	if value == nil || value.Type == object.ValueNil {
		return value == nil && node.kind == object.ValueNil
	}
	if value.Type != node.kind {
		return false
	}
	switch node.kind {
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
		floatValue, ok := value.Data.(float64)
		if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return false
		}
		return !node.floatStatic || floatValue == node.floatValue
	case object.ValueSymbol:
		_, ok := value.Data.(string)
		return ok && value.Class == template.symbolClass
	case object.ValueString:
		if value.Class != template.stringClass || core.AttachedSingletonClass(value) != nil {
			return false
		}
		raw, ok := value.Data.(string)
		return ok && utf8Valid(raw) && (!node.asciiOnly || nativePDFRenderASCIIString(raw))
	case object.ValueArray:
		if value.Class != template.arrayClass || core.AttachedSingletonClass(value) != nil {
			return false
		}
		items, ok := nativePDFArrayItems(value)
		if !ok || len(items) != len(node.items) {
			return false
		}
		for index, child := range node.items {
			if child < 0 || child >= len(bound.values) || nativePDFRenderLayoutCanonicalValue(items[index]) != bound.values[child] {
				return false
			}
		}
		return true
	case object.ValueHash:
		if !nativePDFRenderLayoutStandardHash(value, template.hashClass) {
			return false
		}
		hash, ok := value.Data.(*object.RHash)
		if !ok || hash == nil || len(hash.Keys) != len(node.entries) || len(hash.Pairs) != len(node.entries) {
			return false
		}
		for _, entry := range node.entries {
			child := entry.node
			keyName, keyOK := nativePDFRenderLayoutKeyName(entry.key)
			if !keyOK || keyName != entry.keyName {
				return false
			}
			entryValue, found := hash.Pairs[entry.key]
			if !found || child < 0 || child >= len(bound.values) || nativePDFRenderLayoutCanonicalValue(entryValue) != bound.values[child] {
				return false
			}
		}
		return true
	case object.ValueObject:
		if value.Class != referenceClass || !nativePDFRenderObjectSingletonFree(value) {
			return false
		}
		data, ok := value.Data.(*object.Object)
		return ok && data != nil && data.HotIntegerInstanceVarMask == 0 && data.InstanceVarGeneration == bound.objectGenerations[nodeIndex]
	default:
		return false
	}
}

func nativePDFRenderObjectSingletonFree(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueObject {
		return false
	}
	data, ok := value.Data.(*object.Object)
	return ok && data != nil && data.SingletonClass == nil
}

func (vm *VM) nativePDFRenderLayoutPlanFor(inputs nativePDFRenderRegionInputs, receiver *object.EmeraldValue, referenceClass, streamClass, filterListClass, pageClass, stackClass *object.Class) (*nativePDFRenderRegionPlan, bool) {
	if vm == nil {
		return nil, false
	}
	abi := &vm.nativePDFRenderRegionABIPlan
	key := nativePDFRenderLayoutTemplateKey{references: len(inputs.ids), pages: len(inputs.pageItems)}
	templates := abi.layoutTemplates[key]
	if len(templates) == 0 {
		return nil, false
	}
	currentMutationGeneration := object.CurrentRenderMutationGeneration()
	for _, template := range templates {
		if template == nil || len(template.refs) != len(inputs.ids) {
			continue
		}
		cache := &template.outputCache
		if !cache.valid || cache.mutationGeneration != currentMutationGeneration ||
			cache.renderer != receiver || cache.identifiers != inputs.identifiers || cache.objects != inputs.objects ||
			len(cache.refs) != len(inputs.ids) || len(cache.refPlans) != len(inputs.ids) {
			continue
		}
		refData := template.scratchData
		if cap(refData) < len(inputs.ids) {
			refData = make([]*object.EmeraldValue, len(inputs.ids))
		} else {
			refData = refData[:len(inputs.ids)]
		}
		template.scratchData = refData
		for index := range inputs.ids {
			refData[index] = cache.refPlans[index].data
		}
		if cached, cachedOK := nativePDFRenderCachedLayoutPlanFor(template, inputs, receiver, cache.refs, refData, referenceClass, streamClass, filterListClass, pageClass, stackClass); cachedOK {
			return cached, true
		}
	}
	for _, template := range templates {
		if template == nil || len(template.refs) != len(inputs.ids) {
			continue
		}
		refs := template.scratchRefs
		if cap(refs) < len(inputs.ids) {
			refs = make([]*object.EmeraldValue, len(inputs.ids))
		} else {
			refs = refs[:len(inputs.ids)]
		}
		refData := template.scratchData
		if cap(refData) < len(inputs.ids) {
			refData = make([]*object.EmeraldValue, len(inputs.ids))
		} else {
			refData = refData[:len(inputs.ids)]
		}
		template.scratchRefs = refs
		template.scratchData = refData
		for index, id := range inputs.ids {
			if id == nil || id.Type != object.ValueInteger {
				return nil, false
			}
			ref, found := core.DirectHashIndex(inputs.objects, id)
			if !found {
				return nil, false
			}
			refs[index] = ref
			refData[index] = nativePDFRenderLayoutObjectIvar(ref, template.referenceClass, template.referenceSlots.data, "@data")
		}
		if cached, cachedOK := nativePDFRenderCachedLayoutPlanFor(template, inputs, receiver, refs, refData, referenceClass, streamClass, filterListClass, pageClass, stackClass); cachedOK {
			return cached, true
		}
		bound, ok := nativePDFRenderBindLayoutTemplate(template, inputs.root, inputs.info, refData, referenceClass)
		if !ok {
			continue
		}
		refPlans := template.scratchReferencePlans
		if cap(refPlans) < len(inputs.ids) {
			refPlans = make([]nativePDFRenderReferencePlan, len(inputs.ids))
		} else {
			refPlans = refPlans[:len(inputs.ids)]
		}
		template.scratchReferencePlans = refPlans
		pagePlans := template.scratchPagePlans
		if cap(pagePlans) < len(inputs.pageItems) {
			pagePlans = make([]nativePDFRenderPagePlan, len(inputs.pageItems))
		} else {
			pagePlans = pagePlans[:len(inputs.pageItems)]
		}
		template.scratchPagePlans = pagePlans
		plan := template.scratchPlan
		if plan == nil {
			plan = &nativePDFRenderRegionPlan{}
			template.scratchPlan = plan
		}
		*plan = nativePDFRenderRegionPlan{
			renderer:    receiver,
			state:       inputs.state,
			store:       inputs.store,
			identifiers: inputs.identifiers,
			objects:     inputs.objects,
			refs:        refPlans,
			pages:       pagePlans,
			pageValues:  inputs.pageValues,
			root:        inputs.root,
			info:        inputs.info,
			version:     inputs.version,
			layout:      bound,
		}
		previousRefPlans := template.scratchReferencePlans
		matched := true
		for index, ref := range refs {
			refPlan, refOK := nativePDFRenderCachedReferenceLayoutPlan(previousRefPlans, index, ref, refData[index], template.refs[index], template, bound, referenceClass, streamClass, filterListClass)
			if !refOK {
				refPlan, refOK = nativePDFRenderReferenceLayoutPlanFor(ref, refData[index], template.refs[index], template, bound, referenceClass, streamClass, filterListClass)
			}
			if !refOK {
				matched = false
				break
			}
			plan.refs[index] = refPlan
		}
		if !matched {
			continue
		}
		for index, page := range inputs.pageItems {
			pagePlan, pageOK := nativePDFBuildPagePlan(page, inputs.state, pageClass, stackClass)
			if !pageOK {
				matched = false
				break
			}
			plan.pages[index] = pagePlan
		}
		if matched {
			return plan, true
		}
	}
	return nil, false
}

func nativePDFRenderCachedLayoutPlanFor(template *nativePDFRenderLayoutTemplate, inputs nativePDFRenderRegionInputs, receiver *object.EmeraldValue, refs, refData []*object.EmeraldValue, referenceClass, streamClass, filterListClass, pageClass, stackClass *object.Class) (*nativePDFRenderRegionPlan, bool) {
	if template == nil || !template.outputCache.valid || receiver == nil || len(refs) != len(template.refs) ||
		len(refData) != len(template.refs) || referenceClass == nil || streamClass == nil || filterListClass == nil ||
		pageClass == nil || stackClass == nil {
		return nil, false
	}
	cache := &template.outputCache
	if cache.bound == nil || cache.bound.template != template || cache.renderer != receiver || cache.state != inputs.state ||
		cache.store != inputs.store || cache.identifiers != inputs.identifiers || cache.objects != inputs.objects ||
		cache.root != inputs.root || cache.info != inputs.info || cache.pageValues != inputs.pageValues ||
		cache.version != inputs.version || len(cache.refs) != len(refs) || len(cache.refPlans) != len(refs) ||
		len(cache.pages) != len(inputs.pageItems) || len(cache.offsets) != len(refs) || cache.output == "" {
		return nil, false
	}
	mutationStable := cache.mutationGeneration == object.CurrentRenderMutationGeneration()
	for index, ref := range refs {
		if ref != cache.refs[index] {
			return nil, false
		}
	}
	if !mutationStable {
		if !nativePDFRenderLayoutFastBind(template, cache.bound, inputs.root, inputs.info, refData, referenceClass) ||
			!nativePDFRenderLayoutOutputNodesStillValid(template, cache.bound, cache) {
			return nil, false
		}
		for index, ref := range refs {
			if _, refOK := nativePDFRenderCachedReferenceLayoutPlan(cache.refPlans, index, ref, refData[index], template.refs[index], template, cache.bound, referenceClass, streamClass, filterListClass); !refOK {
				return nil, false
			}
		}
	}
	pagePlans := template.scratchPagePlans
	if cap(pagePlans) < len(inputs.pageItems) {
		pagePlans = make([]nativePDFRenderPagePlan, len(inputs.pageItems))
	} else {
		pagePlans = pagePlans[:len(inputs.pageItems)]
	}
	template.scratchPagePlans = pagePlans
	for index, page := range inputs.pageItems {
		if mutationStable {
			cachedPage := cache.pages[index]
			if cachedPage.page != page {
				return nil, false
			}
			pagePlans[index] = nativePDFRenderPagePlan{page: cachedPage.page, state: cachedPage.state, stackValues: cachedPage.stackValues, items: cachedPage.items}
			continue
		}
		pagePlan, pageOK := nativePDFBuildPagePlan(page, inputs.state, pageClass, stackClass)
		if !pageOK || !nativePDFRenderPageCacheMatches(cache.pages[index], pagePlan) {
			return nil, false
		}
		pagePlans[index] = pagePlan
	}
	plan := template.scratchPlan
	if plan == nil {
		plan = &nativePDFRenderRegionPlan{}
		template.scratchPlan = plan
	}
	*plan = nativePDFRenderRegionPlan{
		renderer:               receiver,
		state:                  inputs.state,
		store:                  inputs.store,
		identifiers:            inputs.identifiers,
		objects:                inputs.objects,
		refs:                   cache.refPlans,
		pages:                  pagePlans,
		pageValues:             inputs.pageValues,
		root:                   inputs.root,
		info:                   inputs.info,
		version:                inputs.version,
		layout:                 cache.bound,
		cachedOutput:           true,
		cachedOutputText:       cache.output,
		cachedOutputRefs:       cache.refs,
		cachedOutputPages:      pagePlans,
		cachedOutputOffsets:    cache.offsets,
		cachedOutputXrefOffset: cache.xrefOffset,
	}
	return plan, true
}

func nativePDFRenderPageCacheMatches(cached nativePDFRenderLayoutPageCache, current nativePDFRenderPagePlan) bool {
	if cached.page != current.page || cached.state != current.state || cached.stackValues != current.stackValues || len(cached.items) != len(current.items) {
		return false
	}
	for index, item := range current.items {
		if cached.items[index] != item {
			return false
		}
	}
	return true
}

func nativePDFRenderLayoutOutputNodesStillValid(template *nativePDFRenderLayoutTemplate, bound *nativePDFRenderBoundLayout, cache *nativePDFRenderLayoutOutputCache) bool {
	if template == nil || bound == nil || cache == nil || len(cache.nodes) != len(template.nodes) || len(bound.values) != len(template.nodes) {
		return false
	}
	for index, node := range template.nodes {
		value := bound.values[index]
		snapshot := cache.nodes[index]
		if snapshot.kind != node.kind {
			return false
		}
		switch node.kind {
		case object.ValueNil:
			if value != nil && value.Type != object.ValueNil {
				return false
			}
		case object.ValueBool:
			truth, ok := value.Data.(bool)
			if !ok || truth != snapshot.boolValue {
				return false
			}
		case object.ValueInteger:
			if bigInteger := value.BigIntValue(); bigInteger != nil {
				if snapshot.integerBig != bigInteger.String() {
					return false
				}
				continue
			}
			integer, ok := value.Data.(int64)
			if !ok || snapshot.integerBig != "" || integer != snapshot.integer {
				return false
			}
		case object.ValueFloat:
			floatValue, ok := value.Data.(float64)
			if !ok || floatValue != snapshot.float {
				return false
			}
		case object.ValueSymbol:
			name, ok := value.Data.(string)
			if !ok || name != snapshot.text {
				return false
			}
		case object.ValueString:
			raw, ok := value.Data.(string)
			if !ok || raw != snapshot.text || value.Encoding != snapshot.encoding {
				return false
			}
		case object.ValueObject:
			identifier, identifierOK := nativePDFRenderLayoutObjectIntegerIvar(value, template.referenceClass, template.referenceSlots.identifier, "@identifier")
			generation, generationOK := nativePDFRenderLayoutObjectIntegerIvar(value, template.referenceClass, template.referenceSlots.generation, "@gen")
			if !identifierOK || !generationOK || identifier != snapshot.identifier || generation != snapshot.generation {
				return false
			}
		case object.ValueArray, object.ValueHash:
			// nativePDFRenderLayoutFastBind already checks all direct edges. The
			// scalar children are validated by their own snapshots.
		default:
			return false
		}
		if node.kind == object.ValueHash {
			for _, entry := range node.entries {
				keyName, keyOK := nativePDFRenderLayoutKeyName(entry.key)
				if !keyOK || keyName != entry.keyName {
					return false
				}
			}
		}
	}
	return true
}

func nativePDFRenderRememberLayoutOutput(plan *nativePDFRenderRegionPlan, serialized string, xrefOffset int64) {
	if plan == nil || plan.layout == nil || plan.layout.template == nil || serialized == "" {
		return
	}
	template := plan.layout.template
	if len(plan.refs) != len(template.refs) || len(plan.pages) != template.key.pages {
		return
	}
	nodes, ok := nativePDFRenderLayoutOutputNodeSnapshots(template, plan.layout)
	if !ok {
		return
	}
	cache := &template.outputCache
	cache.valid = false
	cache.renderer = plan.renderer
	cache.state = plan.state
	cache.store = plan.store
	cache.identifiers = plan.identifiers
	cache.objects = plan.objects
	cache.root = plan.root
	cache.info = plan.info
	cache.pageValues = plan.pageValues
	cache.version = plan.version
	cache.bound = plan.layout
	cache.output = serialized
	cache.xrefOffset = xrefOffset
	cache.mutationGeneration = object.CurrentRenderMutationGeneration()
	if cap(cache.refs) < len(plan.refs) {
		cache.refs = make([]*object.EmeraldValue, len(plan.refs))
	} else {
		cache.refs = cache.refs[:len(plan.refs)]
	}
	if cap(cache.refPlans) < len(plan.refs) {
		cache.refPlans = make([]nativePDFRenderReferencePlan, len(plan.refs))
	} else {
		cache.refPlans = cache.refPlans[:len(plan.refs)]
	}
	if cap(cache.offsets) < len(plan.refs) {
		cache.offsets = make([]int64, len(plan.refs))
	} else {
		cache.offsets = cache.offsets[:len(plan.refs)]
	}
	for index, ref := range plan.refs {
		cache.refs[index] = ref.ref
		cache.refPlans[index] = ref
		cache.offsets[index] = ref.offset
	}
	if cap(cache.pages) < len(plan.pages) {
		cache.pages = make([]nativePDFRenderLayoutPageCache, len(plan.pages))
	} else {
		cache.pages = cache.pages[:len(plan.pages)]
	}
	for index, page := range plan.pages {
		items := cache.pages[index].items[:0]
		items = append(items, page.items...)
		cache.pages[index] = nativePDFRenderLayoutPageCache{
			page:        page.page,
			state:       page.state,
			stackValues: page.stackValues,
			items:       items,
		}
	}
	cache.nodes = nodes
	cache.pageValues = plan.pageValues
	cache.valid = true
}

func nativePDFRenderLayoutOutputNodeSnapshots(template *nativePDFRenderLayoutTemplate, bound *nativePDFRenderBoundLayout) ([]nativePDFRenderLayoutNodeSnapshot, bool) {
	if template == nil || bound == nil || len(bound.values) != len(template.nodes) || len(bound.identifiers) != len(template.nodes) || len(bound.generations) != len(template.nodes) {
		return nil, false
	}
	snapshots := make([]nativePDFRenderLayoutNodeSnapshot, len(template.nodes))
	for index, node := range template.nodes {
		value := bound.values[index]
		snapshot := nativePDFRenderLayoutNodeSnapshot{kind: node.kind}
		switch node.kind {
		case object.ValueNil:
		case object.ValueBool:
			truth, ok := value.Data.(bool)
			if !ok {
				return nil, false
			}
			snapshot.boolValue = truth
		case object.ValueInteger:
			if bigInteger := value.BigIntValue(); bigInteger != nil {
				snapshot.integerBig = bigInteger.String()
				break
			}
			integer, ok := value.Data.(int64)
			if !ok {
				return nil, false
			}
			snapshot.integer = integer
		case object.ValueFloat:
			floatValue, ok := value.Data.(float64)
			if !ok {
				return nil, false
			}
			snapshot.float = floatValue
		case object.ValueSymbol:
			name, ok := value.Data.(string)
			if !ok {
				return nil, false
			}
			snapshot.text = name
		case object.ValueString:
			raw, ok := value.Data.(string)
			if !ok {
				return nil, false
			}
			snapshot.text = raw
			snapshot.encoding = value.Encoding
		case object.ValueObject:
			snapshot.identifier = bound.identifiers[index]
			snapshot.generation = bound.generations[index]
		case object.ValueArray, object.ValueHash:
		default:
			return nil, false
		}
		snapshots[index] = snapshot
	}
	return snapshots, true
}

func nativePDFRenderBindLayoutTemplate(template *nativePDFRenderLayoutTemplate, root, info *object.EmeraldValue, refData []*object.EmeraldValue, referenceClass *object.Class) (*nativePDFRenderBoundLayout, bool) {
	if template == nil || template.rootNode < 0 || template.infoNode < 0 || len(template.nodes) == 0 || referenceClass == nil {
		return nil, false
	}
	bound := template.scratchBound
	if nativePDFRenderLayoutFastBind(template, bound, root, info, refData, referenceClass) {
		return bound, true
	}
	if bound == nil || len(bound.values) != len(template.nodes) {
		bound = &nativePDFRenderBoundLayout{
			template:          template,
			values:            make([]*object.EmeraldValue, len(template.nodes)),
			bound:             make([]uint32, len(template.nodes)),
			epoch:             1,
			identifiers:       make([]int64, len(template.nodes)),
			generations:       make([]int64, len(template.nodes)),
			objectGenerations: make([]uint64, len(template.nodes)),
		}
		template.scratchBound = bound
	} else {
		bound.epoch++
		if bound.epoch == 0 {
			for index := range bound.bound {
				bound.bound[index] = 0
			}
			bound.epoch = 1
		}
	}
	bound.boundCount = 0
	bound.trusted = false
	if len(bound.objectGenerations) != len(template.nodes) {
		bound.objectGenerations = make([]uint64, len(template.nodes))
	}
	if !nativePDFRenderAssignLayoutNode(bound, template.rootNode, root) || !nativePDFRenderAssignLayoutNode(bound, template.infoNode, info) || len(refData) != len(template.refs) {
		return nil, false
	}
	for index, data := range refData {
		if !nativePDFRenderAssignLayoutNode(bound, template.refs[index].dataNode, data) {
			return nil, false
		}
	}
	for index, node := range template.nodes {
		if bound.bound[index] != bound.epoch || !nativePDFRenderLayoutNodeGuard(node, bound.values[index], bound, index, referenceClass) {
			return nil, false
		}
		value := bound.values[index]
		switch node.kind {
		case object.ValueArray:
			items, ok := nativePDFArrayItems(value)
			if !ok {
				return nil, false
			}
			for itemIndex, child := range node.items {
				if !nativePDFRenderAssignLayoutNode(bound, child, items[itemIndex]) {
					return nil, false
				}
			}
		case object.ValueHash:
			hash, ok := value.Data.(*object.RHash)
			if !ok || hash == nil {
				return nil, false
			}
			for _, entry := range node.entries {
				entryValue, entryOK := nativePDFRenderLayoutHashValueFromRHash(hash, entry)
				if !entryOK || !nativePDFRenderAssignLayoutNode(bound, entry.node, entryValue) {
					return nil, false
				}
			}
		}
	}
	if bound.boundCount != len(bound.values) {
		return nil, false
	}
	bound.trusted = true
	return bound, true
}

func nativePDFRenderAssignLayoutNode(bound *nativePDFRenderBoundLayout, nodeIndex int, value *object.EmeraldValue) bool {
	if bound == nil || nodeIndex < 0 || nodeIndex >= len(bound.values) {
		return false
	}
	value = nativePDFRenderLayoutCanonicalValue(value)
	if bound.bound[nodeIndex] == bound.epoch {
		return bound.values[nodeIndex] == value
	}
	bound.values[nodeIndex] = value
	bound.bound[nodeIndex] = bound.epoch
	bound.boundCount++
	return true
}

func nativePDFRenderLayoutCanonicalValue(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type == object.ValueNil {
		return nil
	}
	return value
}

func nativePDFRenderLayoutStandardHash(value *object.EmeraldValue, hashClass *object.Class) bool {
	if value == nil || value.Type != object.ValueHash || value.Class != hashClass || core.AttachedSingletonClass(value) != nil {
		return false
	}
	data, ok := value.Data.(*object.RHash)
	return ok && data != nil && data.Default == nil && data.DefaultProc == nil && !data.CompareByIdentity
}

func nativePDFRenderLayoutNodeGuard(node nativePDFRenderLayoutNode, value *object.EmeraldValue, bound *nativePDFRenderBoundLayout, nodeIndex int, referenceClass *object.Class) bool {
	if value == nil || value.Type == object.ValueNil {
		return node.kind == object.ValueNil
	}
	if value.Type != node.kind {
		return false
	}
	if bound == nil || bound.template == nil {
		return false
	}
	template := bound.template
	switch node.kind {
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
		floatValue, ok := value.Data.(float64)
		return ok && !math.IsNaN(floatValue) && !math.IsInf(floatValue, 0) && (!node.floatStatic || floatValue == node.floatValue)
	case object.ValueSymbol:
		_, ok := value.Data.(string)
		return ok && value.Class == template.symbolClass
	case object.ValueString:
		if value.Class != template.stringClass || core.AttachedSingletonClass(value) != nil {
			return false
		}
		raw, ok := value.Data.(string)
		return ok && utf8Valid(raw) && (!node.asciiOnly || nativePDFRenderASCIIString(raw))
	case object.ValueArray:
		if value.Class != template.arrayClass || core.AttachedSingletonClass(value) != nil {
			return false
		}
		items, ok := nativePDFArrayItems(value)
		return ok && len(items) == len(node.items) && len(items) < nativePDFRenderCompileMinEntries
	case object.ValueHash:
		if !nativePDFRenderLayoutStandardHash(value, template.hashClass) {
			return false
		}
		hash, ok := value.Data.(*object.RHash)
		return ok && hash != nil && len(hash.Keys) == len(node.entries) && len(hash.Pairs) == len(node.entries)
	case object.ValueObject:
		if value.Class != referenceClass || !nativePDFRenderObjectSingletonFree(value) {
			return false
		}
		identifier, identifierOK := nativePDFRenderLayoutObjectIntegerIvar(value, template.referenceClass, template.referenceSlots.identifier, "@identifier")
		generation, generationOK := nativePDFRenderLayoutObjectIntegerIvar(value, template.referenceClass, template.referenceSlots.generation, "@gen")
		if !identifierOK || !generationOK || bound == nil || nodeIndex >= len(bound.identifiers) || nodeIndex >= len(bound.generations) {
			return false
		}
		data, dataOK := value.Data.(*object.Object)
		if !dataOK || data == nil || nodeIndex >= len(bound.objectGenerations) {
			return false
		}
		bound.identifiers[nodeIndex] = identifier
		bound.generations[nodeIndex] = generation
		bound.objectGenerations[nodeIndex] = data.InstanceVarGeneration
		return true
	default:
		return false
	}
}

func nativePDFRenderLayoutHashValue(value *object.EmeraldValue, entry nativePDFRenderLayoutEntry) (*object.EmeraldValue, bool) {
	if !nativePDFStandardHash(value) {
		return nil, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil || len(hash.Keys) != len(hash.Pairs) {
		return nil, false
	}
	return nativePDFRenderLayoutHashValueFromRHash(hash, entry)
}

func nativePDFRenderLayoutHashValueFromRHash(hash *object.RHash, entry nativePDFRenderLayoutEntry) (*object.EmeraldValue, bool) {
	if hash == nil {
		return nil, false
	}
	if entry.key != nil {
		if result, found := hash.Pairs[entry.key]; found && result != nil {
			keyName, keyOK := nativePDFRenderLayoutKeyName(entry.key)
			if entry.key.Type == object.ValueSymbol && entry.key.Class == entry.keyClass && keyOK && keyName == entry.keyName && core.AttachedSingletonClass(entry.key) == nil {
				return result, true
			}
			if keyOK && keyName == entry.keyName {
				return result, true
			}
		}
	}
	var result *object.EmeraldValue
	found := false
	for _, key := range hash.Keys {
		keyName, keyOK := nativePDFRenderLayoutKeyName(key)
		if !keyOK {
			return nil, false
		}
		if keyName != entry.keyName {
			continue
		}
		if found {
			return nil, false
		}
		result, found = hash.Pairs[key]
		if !found || result == nil {
			return nil, false
		}
	}
	return result, found
}

func nativePDFRenderLayoutKeyName(key *object.EmeraldValue) (string, bool) {
	if key == nil {
		return "", false
	}
	switch key.Type {
	case object.ValueSymbol:
		if key.Class != core.R.Classes["Symbol"] {
			return "", false
		}
	case object.ValueString:
		if key.Class != core.R.Classes["String"] || core.AttachedSingletonClass(key) != nil {
			return "", false
		}
	default:
		return "", false
	}
	name, ok := key.Data.(string)
	return name, ok
}

func nativePDFRenderLayoutNodeHasKey(template *nativePDFRenderLayoutTemplate, nodeIndex int, name string) bool {
	if template == nil || nodeIndex < 0 || nodeIndex >= len(template.nodes) {
		return true
	}
	for _, entry := range template.nodes[nodeIndex].entries {
		if entry.keyName == name {
			return true
		}
	}
	return false
}

func nativePDFRenderObjectLayoutGeneration(value *object.EmeraldValue) (uint64, bool) {
	if value == nil || value.Type != object.ValueObject {
		return 0, false
	}
	data, ok := value.Data.(*object.Object)
	if !ok || data == nil || data.HotIntegerInstanceVarMask != 0 {
		return 0, false
	}
	return data.InstanceVarGeneration, true
}

func nativePDFRenderCachedReferenceLayoutPlan(previous []nativePDFRenderReferencePlan, index int, ref, data *object.EmeraldValue, layoutRef nativePDFRenderLayoutReference, template *nativePDFRenderLayoutTemplate, bound *nativePDFRenderBoundLayout, referenceClass, streamClass, filterListClass *object.Class) (nativePDFRenderReferencePlan, bool) {
	if index < 0 || index >= len(previous) || template == nil || bound == nil || layoutRef.dataNode < 0 ||
		layoutRef.dataNode >= len(bound.values) || bound.values[layoutRef.dataNode] != nativePDFRenderLayoutCanonicalValue(data) {
		return nativePDFRenderReferencePlan{}, false
	}
	cached := previous[index]
	if !cached.layoutCacheable || cached.ref != ref || cached.data != nativePDFRenderLayoutCanonicalValue(data) || cached.filteredCache != layoutRef.filteredCache {
		return nativePDFRenderReferencePlan{}, false
	}
	if ref == nil || ref.Type != object.ValueObject || ref.Class != referenceClass || ref.Frozen ||
		!nativePDFRenderObjectSingletonFree(ref) || cached.stream == nil {
		return nativePDFRenderReferencePlan{}, false
	}
	refGeneration, refGenerationOK := nativePDFRenderObjectLayoutGeneration(ref)
	if !refGenerationOK || refGeneration != cached.refLayoutGeneration {
		return nativePDFRenderReferencePlan{}, false
	}
	stream := nativePDFRenderLayoutObjectIvar(ref, template.referenceClass, template.referenceSlots.stream, "@stream")
	if stream == nil || stream != cached.stream || stream.Type != object.ValueObject || stream.Class != streamClass || stream.Frozen ||
		!nativePDFRenderObjectSingletonFree(stream) {
		return nativePDFRenderReferencePlan{}, false
	}
	streamGeneration, streamGenerationOK := nativePDFRenderObjectLayoutGeneration(stream)
	if !streamGenerationOK || streamGeneration != cached.streamLayoutGeneration {
		return nativePDFRenderReferencePlan{}, false
	}
	filters := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.filters, "@filters")
	if filters == nil || filters != cached.filtersValue || filters.Type != object.ValueObject || filters.Class != filterListClass ||
		filters.Frozen || !nativePDFRenderObjectSingletonFree(filters) {
		return nativePDFRenderReferencePlan{}, false
	}
	filtersGeneration, filtersGenerationOK := nativePDFRenderObjectLayoutGeneration(filters)
	if !filtersGenerationOK || filtersGeneration != cached.filtersLayoutGeneration {
		return nativePDFRenderReferencePlan{}, false
	}
	list := nativePDFRenderLayoutObjectIvar(filters, template.filterListClass, template.filterListSlots.list, "@list")
	items, itemsOK := ([]*object.EmeraldValue)(nil), false
	if list != nil {
		items, itemsOK = list.Data.([]*object.EmeraldValue)
	}
	if list == nil || list != cached.listValue || list.Type != object.ValueArray || list.Class != template.arrayClass ||
		core.AttachedSingletonClass(list) != nil || !itemsOK || len(items) != cached.listLength || len(items) != 0 {
		return nativePDFRenderReferencePlan{}, false
	}
	raw := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.stream, "@stream")
	filtered := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.filteredStream, "@filtered_stream")
	rawPresent := raw != nil && raw.Type != object.ValueNil
	filteredPresent := filtered != nil && filtered.Type != object.ValueNil
	if rawPresent != layoutRef.rawPresent || filteredPresent != layoutRef.filteredCache || raw != cached.raw || filtered != cached.filteredValue {
		return nativePDFRenderReferencePlan{}, false
	}
	if rawPresent && !nativePDFRenderASCIIValueWithClass(raw, true, template.stringClass) {
		return nativePDFRenderReferencePlan{}, false
	}
	if rawPresent {
		rawText, rawOK := raw.Data.(string)
		if !rawOK || rawText != cached.rawText {
			return nativePDFRenderReferencePlan{}, false
		}
	}
	if filteredPresent {
		filteredText, filteredOK := filtered.Data.(string)
		if !filteredOK || filteredText != cached.filtered || !nativePDFRenderASCIIValueWithClass(filtered, true, template.stringClass) {
			return nativePDFRenderReferencePlan{}, false
		}
	}
	return cached, true
}

func nativePDFRenderReferenceLayoutPlanFor(ref, data *object.EmeraldValue, layoutRef nativePDFRenderLayoutReference, template *nativePDFRenderLayoutTemplate, bound *nativePDFRenderBoundLayout, referenceClass, streamClass, filterListClass *object.Class) (nativePDFRenderReferencePlan, bool) {
	if ref == nil || ref.Type != object.ValueObject || ref.Class != referenceClass || ref.Frozen ||
		!nativePDFRenderObjectSingletonFree(ref) || layoutRef.dataNode < 0 || layoutRef.dataNode >= len(template.nodes) {
		return nativePDFRenderReferencePlan{}, false
	}
	identifier, identifierOK := nativePDFRenderLayoutObjectIntegerIvar(ref, template.referenceClass, template.referenceSlots.identifier, "@identifier")
	generation, generationOK := nativePDFRenderLayoutObjectIntegerIvar(ref, template.referenceClass, template.referenceSlots.generation, "@gen")
	if !identifierOK || !generationOK || bound == nil || layoutRef.dataNode >= len(bound.values) || bound.values[layoutRef.dataNode] != nativePDFRenderLayoutCanonicalValue(data) {
		return nativePDFRenderReferencePlan{}, false
	}
	boundData := bound.values[layoutRef.dataNode]
	stream := nativePDFRenderLayoutObjectIvar(ref, template.referenceClass, template.referenceSlots.stream, "@stream")
	if stream == nil || stream.Type != object.ValueObject || stream.Class != streamClass || stream.Frozen ||
		!nativePDFRenderObjectSingletonFree(stream) {
		return nativePDFRenderReferencePlan{}, false
	}
	filters := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.filters, "@filters")
	list := nativePDFRenderLayoutObjectIvar(filters, template.filterListClass, template.filterListSlots.list, "@list")
	items, itemsOK := ([]*object.EmeraldValue)(nil), false
	if list != nil {
		items, itemsOK = list.Data.([]*object.EmeraldValue)
	}
	if filters == nil || filters.Type != object.ValueObject || filters.Class != filterListClass ||
		filters.Frozen || !nativePDFRenderObjectSingletonFree(filters) || list == nil || list.Type != object.ValueArray ||
		list.Class != template.arrayClass || core.AttachedSingletonClass(list) != nil || !itemsOK || len(items) != 0 {
		return nativePDFRenderReferencePlan{}, false
	}
	raw := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.stream, "@stream")
	filtered := nativePDFRenderLayoutObjectIvar(stream, template.streamClass, template.streamSlots.filteredStream, "@filtered_stream")
	rawPresent := raw != nil && raw.Type != object.ValueNil
	filteredPresent := filtered != nil && filtered.Type != object.ValueNil
	if rawPresent != layoutRef.rawPresent || filteredPresent != layoutRef.filteredCache {
		return nativePDFRenderReferencePlan{}, false
	}
	filteredText := ""
	rawText := ""
	if filteredPresent {
		var filteredOK bool
		filteredText, filteredOK = filtered.Data.(string)
		if !filteredOK || !nativePDFRenderASCIIValueWithClass(filtered, true, template.stringClass) {
			return nativePDFRenderReferencePlan{}, false
		}
	}
	if rawPresent {
		var rawOK bool
		rawText, rawOK = raw.Data.(string)
		if !rawOK || !nativePDFRenderASCIIValueWithClass(raw, true, template.stringClass) || template.nodes[layoutRef.dataNode].kind != object.ValueHash || nativePDFRenderLayoutNodeHasKey(template, layoutRef.dataNode, "Length") {
			return nativePDFRenderReferencePlan{}, false
		}
	}
	refLayoutGeneration, refGenerationOK := nativePDFRenderObjectLayoutGeneration(ref)
	streamLayoutGeneration, streamGenerationOK := nativePDFRenderObjectLayoutGeneration(stream)
	filtersLayoutGeneration, filtersGenerationOK := nativePDFRenderObjectLayoutGeneration(filters)
	return nativePDFRenderReferencePlan{
		ref:                     ref,
		data:                    boundData,
		stream:                  stream,
		raw:                     raw,
		rawText:                 rawText,
		filtered:                filteredText,
		filteredValue:           filtered,
		filteredCache:           filteredPresent,
		offset:                  -1,
		identifier:              identifier,
		generation:              generation,
		refLayoutGeneration:     refLayoutGeneration,
		streamLayoutGeneration:  streamLayoutGeneration,
		filtersLayoutGeneration: filtersLayoutGeneration,
		filtersValue:            filters,
		listValue:               list,
		listLength:              len(items),
		layoutCacheable:         refGenerationOK && streamGenerationOK && filtersGenerationOK,
	}, true
}

func nativePDFRenderInstallLayoutTemplate(vm *VM, template *nativePDFRenderLayoutTemplate) {
	if vm == nil || template == nil {
		return
	}
	abi := &vm.nativePDFRenderRegionABIPlan
	if abi.layoutTemplates == nil {
		abi.layoutTemplates = make(map[nativePDFRenderLayoutTemplateKey][]*nativePDFRenderLayoutTemplate)
	}
	list := abi.layoutTemplates[template.key]
	for _, existing := range list {
		if existing != nil && existing.signature == template.signature {
			return
		}
	}
	if len(list) >= 4 {
		return
	}
	abi.layoutTemplates[template.key] = append(list, template)
}

func nativePDFRenderObjectSlotsFor(class *object.Class, names ...string) nativePDFRenderObjectSlots {
	slots := nativePDFRenderObjectSlots{
		identifier:     -1,
		generation:     -1,
		data:           -1,
		stream:         -1,
		filters:        -1,
		list:           -1,
		filteredStream: -1,
	}
	if class == nil || !class.PrepareBatchInstanceVarLayout(names) {
		return slots
	}
	for _, name := range names {
		index, ok := class.InstanceVarSlots[name]
		if !ok || int(index) >= len((&object.Object{}).InlineInstanceVars) {
			continue
		}
		switch name {
		case "@identifier":
			slots.identifier = int(index)
		case "@gen":
			slots.generation = int(index)
		case "@data":
			slots.data = int(index)
		case "@stream":
			slots.stream = int(index)
		case "@filters":
			slots.filters = int(index)
		case "@list":
			slots.list = int(index)
		case "@filtered_stream":
			slots.filteredStream = int(index)
		}
	}
	return slots
}

func nativePDFRenderLayoutTemplateFor(plan *nativePDFRenderRegionPlan, referenceClass *object.Class) *nativePDFRenderLayoutTemplate {
	if plan == nil || referenceClass == nil || len(plan.refs) == 0 || len(plan.pages) == 0 {
		return nil
	}
	builder := nativePDFRenderLayoutTemplateBuilder{
		plan:           plan,
		referenceClass: referenceClass,
		indexes:        make(map[*object.EmeraldValue]int, len(plan.valuePlans)),
		visiting:       make(map[*object.EmeraldValue]bool, len(plan.valuePlans)),
		valid:          true,
	}
	rootNode := builder.add(plan.root)
	infoNode := builder.add(plan.info)
	if !builder.valid || rootNode < 0 || infoNode < 0 {
		return nil
	}
	referenceSlots := nativePDFRenderObjectSlotsFor(referenceClass, "@identifier", "@gen", "@data", "@stream")
	template := &nativePDFRenderLayoutTemplate{
		key:            nativePDFRenderLayoutTemplateKey{references: len(plan.refs), pages: len(plan.pages)},
		rootNode:       rootNode,
		infoNode:       infoNode,
		referenceClass: referenceClass,
		referenceSlots: referenceSlots,
		arrayClass:     core.R.Classes["Array"],
		hashClass:      core.R.Classes["Hash"],
		stringClass:    core.R.Classes["String"],
		symbolClass:    core.R.Classes["Symbol"],
		nodes:          builder.nodes,
		refs:           make([]nativePDFRenderLayoutReference, 0, len(plan.refs)),
	}
	for _, ref := range plan.refs {
		dataNode := builder.add(ref.data)
		if !builder.valid || dataNode < 0 {
			return nil
		}
		template.refs = append(template.refs, nativePDFRenderLayoutReference{
			dataNode:      dataNode,
			rawPresent:    ref.raw != nil && ref.raw.Type != object.ValueNil,
			filteredCache: ref.filteredCache,
		})
	}
	template.nodes = builder.nodes
	if !nativePDFRenderBuildLayoutPrograms(template) {
		return nil
	}
	template.signature = nativePDFRenderLayoutSignature(template)
	return template
}

type nativePDFRenderLayoutTemplateBuilder struct {
	plan           *nativePDFRenderRegionPlan
	referenceClass *object.Class
	indexes        map[*object.EmeraldValue]int
	visiting       map[*object.EmeraldValue]bool
	nodes          []nativePDFRenderLayoutNode
	valid          bool
}

func (builder *nativePDFRenderLayoutTemplateBuilder) add(value *object.EmeraldValue) int {
	if !builder.valid {
		return -1
	}
	if value == nil || value.Type == object.ValueNil {
		value = nil
	}
	if builder.visiting[value] {
		builder.valid = false
		return -1
	}
	if index, found := builder.indexes[value]; found {
		return index
	}
	index := len(builder.nodes)
	builder.indexes[value] = index
	builder.nodes = append(builder.nodes, nativePDFRenderLayoutNode{})
	if value == nil {
		builder.nodes[index].kind = object.ValueNil
		return index
	}
	switch value.Type {
	case object.ValueBool:
		if _, ok := value.Data.(bool); !ok {
			builder.valid = false
			return -1
		}
	case object.ValueInteger:
		if value.BigIntValue() == nil {
			if _, ok := value.Data.(int64); !ok {
				builder.valid = false
				return -1
			}
		}
	case object.ValueFloat:
		floatValue, ok := value.Data.(float64)
		if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			builder.valid = false
			return -1
		}
		builder.nodes[index].floatValue = floatValue
		builder.nodes[index].floatText = nativePDFRenderRealText(floatValue)
		builder.nodes[index].floatStatic = true
	case object.ValueSymbol:
		if value.Class != core.R.Classes["Symbol"] || core.AttachedSingletonClass(value) != nil {
			builder.valid = false
			return -1
		}
		if _, ok := value.Data.(string); !ok {
			builder.valid = false
			return -1
		}
	case object.ValueString:
		if value.Class != core.R.Classes["String"] || core.AttachedSingletonClass(value) != nil {
			builder.valid = false
			return -1
		}
		raw, ok := value.Data.(string)
		if !ok || !utf8Valid(raw) {
			builder.valid = false
			return -1
		}
		builder.nodes[index].asciiOnly = nativePDFRenderASCIIString(raw)
	case object.ValueArray:
		if value.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(value) != nil {
			builder.valid = false
			return -1
		}
		items, ok := nativePDFRenderArrayItemsFor(value, builder.plan.valuePlans)
		if !ok || len(items) >= nativePDFRenderCompileMinEntries {
			builder.valid = false
			return -1
		}
		builder.visiting[value] = true
		builder.nodes[index].kind = object.ValueArray
		builder.nodes[index].items = make([]int, len(items))
		for itemIndex, item := range items {
			child := builder.add(item)
			if !builder.valid || child < 0 {
				delete(builder.visiting, value)
				return -1
			}
			builder.nodes[index].items[itemIndex] = child
		}
		delete(builder.visiting, value)
	case object.ValueHash:
		if !nativePDFStandardHash(value) {
			builder.valid = false
			return -1
		}
		entries, ok := nativePDFRenderHashEntriesFor(value, builder.plan.valuePlans)
		if !ok || len(entries) >= nativePDFRenderCompileMinEntries {
			builder.valid = false
			return -1
		}
		builder.visiting[value] = true
		builder.nodes[index].kind = object.ValueHash
		builder.nodes[index].entries = make([]nativePDFRenderLayoutEntry, len(entries))
		for entryIndex, entry := range entries {
			child := builder.add(entry.value)
			if !builder.valid || child < 0 {
				delete(builder.visiting, value)
				return -1
			}
			key := nativePDFRenderLayoutKeyForName(value, entry.keyName)
			if key == nil {
				delete(builder.visiting, value)
				builder.valid = false
				return -1
			}
			builder.nodes[index].entries[entryIndex] = nativePDFRenderLayoutEntry{keyName: entry.keyName, key: key, keyClass: key.Class, node: child}
		}
		delete(builder.visiting, value)
	case object.ValueObject:
		if value.Class != builder.referenceClass || core.AttachedSingletonClass(value) != nil ||
			!nativePDFObjectIntegerIvarOK(value, "@identifier") || !nativePDFObjectIntegerIvarOK(value, "@gen") {
			builder.valid = false
			return -1
		}
	default:
		builder.valid = false
		return -1
	}
	builder.nodes[index].kind = value.Type
	return index
}

func nativePDFRenderBuildLayoutPrograms(template *nativePDFRenderLayoutTemplate) bool {
	if template == nil || len(template.nodes) == 0 {
		return false
	}
	template.writePrograms = make([][]nativePDFRenderLayoutWriteOp, len(template.nodes))
	template.writeLengthPrograms = make([][]nativePDFRenderLayoutWriteOp, len(template.nodes))
	for index := range template.nodes {
		program, ok := nativePDFRenderCompileLayoutWriteProgram(template, index, false)
		if !ok {
			return false
		}
		template.writePrograms[index] = program
		if template.nodes[index].kind == object.ValueHash {
			lengthProgram, lengthOK := nativePDFRenderCompileLayoutWriteProgram(template, index, true)
			if !lengthOK {
				return false
			}
			template.writeLengthPrograms[index] = lengthProgram
		}
	}
	return true
}

func nativePDFRenderCompileLayoutWriteProgram(template *nativePDFRenderLayoutTemplate, rootNode int, withLength bool) ([]nativePDFRenderLayoutWriteOp, bool) {
	if template == nil || rootNode < 0 || rootNode >= len(template.nodes) {
		return nil, false
	}
	program := make([]nativePDFRenderLayoutWriteOp, 0, 16)
	visiting := make([]bool, len(template.nodes))
	appendLiteral := func(text string) {
		if text == "" {
			return
		}
		if len(program) > 0 && program[len(program)-1].kind == nativePDFRenderLayoutWriteLiteral {
			program[len(program)-1].text += text
			return
		}
		program = append(program, nativePDFRenderLayoutWriteOp{kind: nativePDFRenderLayoutWriteLiteral, text: text})
	}
	var appendNode func(int, bool) bool
	appendNode = func(nodeIndex int, addLength bool) bool {
		if nodeIndex < 0 || nodeIndex >= len(template.nodes) || visiting[nodeIndex] {
			return false
		}
		node := template.nodes[nodeIndex]
		visiting[nodeIndex] = true
		defer func() { visiting[nodeIndex] = false }()
		switch node.kind {
		case object.ValueArray:
			appendLiteral("[")
			for index, child := range node.items {
				if index != 0 {
					appendLiteral(" ")
				}
				if !appendNode(child, false) {
					return false
				}
			}
			appendLiteral("]")
		case object.ValueHash:
			appendLiteral("<< ")
			lengthWritten := false
			for _, entry := range node.entries {
				if addLength && !lengthWritten && entry.keyName > "Length" {
					program = append(program, nativePDFRenderLayoutWriteOp{kind: nativePDFRenderLayoutWriteLength})
					lengthWritten = true
				}
				appendLiteral(nativePDFSymbolText(entry.keyName) + " ")
				if !appendNode(entry.node, false) {
					return false
				}
				appendLiteral("\n")
			}
			if addLength && !lengthWritten {
				program = append(program, nativePDFRenderLayoutWriteOp{kind: nativePDFRenderLayoutWriteLength})
			}
			appendLiteral(">>")
		default:
			var kind nativePDFRenderLayoutWriteOpKind
			switch node.kind {
			case object.ValueNil:
				kind = nativePDFRenderLayoutWriteNil
			case object.ValueBool:
				kind = nativePDFRenderLayoutWriteBool
			case object.ValueInteger:
				kind = nativePDFRenderLayoutWriteInteger
			case object.ValueFloat:
				if node.floatStatic {
					appendLiteral(node.floatText)
					return true
				}
				kind = nativePDFRenderLayoutWriteFloat
			case object.ValueSymbol:
				kind = nativePDFRenderLayoutWriteSymbol
			case object.ValueString:
				if node.asciiOnly {
					kind = nativePDFRenderLayoutWriteASCIIString
				} else {
					kind = nativePDFRenderLayoutWriteString
				}
			case object.ValueObject:
				kind = nativePDFRenderLayoutWriteReference
			default:
				return false
			}
			program = append(program, nativePDFRenderLayoutWriteOp{kind: kind, node: nodeIndex})
		}
		return true
	}
	if !appendNode(rootNode, withLength) {
		return nil, false
	}
	return program, true
}

func nativePDFRenderLayoutKeyForName(value *object.EmeraldValue, name string) *object.EmeraldValue {
	if !nativePDFStandardHash(value) {
		return nil
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil {
		return nil
	}
	for _, key := range hash.Keys {
		if key == nil || core.AttachedSingletonClass(key) != nil {
			continue
		}
		var keyName string
		switch key.Type {
		case object.ValueString:
			if key.Class != core.R.Classes["String"] {
				continue
			}
			keyName, _ = key.Data.(string)
		case object.ValueSymbol:
			if key.Class != core.R.Classes["Symbol"] {
				continue
			}
			keyName, _ = key.Data.(string)
		default:
			continue
		}
		if keyName == name {
			return key
		}
	}
	return nil
}

func nativePDFRenderLayoutSignature(template *nativePDFRenderLayoutTemplate) string {
	if template == nil {
		return ""
	}
	var signature strings.Builder
	signature.Grow(len(template.nodes) * 8)
	for _, node := range template.nodes {
		signature.WriteByte(byte(node.kind))
		if node.asciiOnly {
			signature.WriteByte('a')
		} else {
			signature.WriteByte('u')
		}
		if node.floatStatic {
			signature.WriteByte('f')
			signature.WriteString(node.floatText)
		} else {
			signature.WriteByte('v')
		}
		signature.WriteByte(':')
		for _, child := range node.items {
			signature.WriteString(strconv.Itoa(child))
			signature.WriteByte(',')
		}
		for _, entry := range node.entries {
			signature.WriteString(entry.keyName)
			signature.WriteByte('=')
			signature.WriteString(strconv.Itoa(entry.node))
			signature.WriteByte(',')
		}
		signature.WriteByte(';')
	}
	signature.WriteString(strconv.Itoa(template.rootNode))
	signature.WriteByte('/')
	signature.WriteString(strconv.Itoa(template.infoNode))
	for _, ref := range template.refs {
		signature.WriteByte('|')
		signature.WriteString(strconv.Itoa(ref.dataNode))
		if ref.rawPresent {
			signature.WriteByte('r')
		}
		if ref.filteredCache {
			signature.WriteByte('f')
		}
	}
	return signature.String()
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
	return nativePDFRenderASCIIValueWithClass(value, allowEmpty, core.R.Classes["String"])
}

func nativePDFRenderASCIIValueWithClass(value *object.EmeraldValue, allowEmpty bool, stringClass *object.Class) bool {
	if value == nil || value.Type != object.ValueString || value.Class != stringClass ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	raw, ok := value.Data.(string)
	if !ok || !allowEmpty && raw == "" {
		return false
	}
	return nativePDFRenderASCIIString(raw)
}

func nativePDFRenderASCIIString(raw string) bool {
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

func nativePDFRenderReferenceLayout(output *strings.Builder, ref *nativePDFRenderReferencePlan, layout *nativePDFRenderBoundLayout, index int) bool {
	if output == nil || ref == nil || layout == nil || layout.template == nil || index < 0 || index >= len(layout.template.refs) || ref.ref == nil || ref.stream == nil {
		return false
	}
	layoutRef := layout.template.refs[index]
	nativePDFRenderWriteInt(output, ref.identifier)
	output.WriteByte(' ')
	nativePDFRenderWriteInt(output, ref.generation)
	output.WriteString(" obj\n")
	if !layoutRef.rawPresent {
		if layoutRef.dataNode >= len(layout.template.writePrograms) || !nativePDFRenderWriteLayoutProgramTrusted(output, layout, layout.template.writePrograms[layoutRef.dataNode], 0) {
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
	if !nativePDFRenderWriteLayoutHashWithLength(output, layout, layoutRef.dataNode, len(filtered)) {
		return false
	}
	output.WriteString("\nstream\n")
	output.WriteString(filtered)
	output.WriteString("\nendstream\nendobj\n")
	return true
}

func nativePDFRenderWriteLayoutHashWithLength(output *strings.Builder, layout *nativePDFRenderBoundLayout, nodeIndex, length int) bool {
	if output == nil || layout == nil || layout.template == nil || nodeIndex < 0 || nodeIndex >= len(layout.template.nodes) {
		return false
	}
	node := layout.template.nodes[nodeIndex]
	if node.kind != object.ValueHash {
		return false
	}
	for _, entry := range node.entries {
		if entry.keyName == "Length" {
			return false
		}
	}
	if nodeIndex >= len(layout.template.writeLengthPrograms) {
		return false
	}
	return nativePDFRenderWriteLayoutProgramTrusted(output, layout, layout.template.writeLengthPrograms[nodeIndex], length)
}

func nativePDFRenderWriteLayoutProgram(output *strings.Builder, layout *nativePDFRenderBoundLayout, program []nativePDFRenderLayoutWriteOp, length int) bool {
	if output == nil || layout == nil || layout.template == nil || len(program) == 0 {
		return false
	}
	if layout.trusted {
		return nativePDFRenderWriteLayoutProgramTrusted(output, layout, program, length)
	}
	for _, op := range program {
		switch op.kind {
		case nativePDFRenderLayoutWriteLiteral:
			output.WriteString(op.text)
		case nativePDFRenderLayoutWriteLength:
			nativePDFRenderWriteSymbol(output, "Length")
			output.WriteByte(' ')
			nativePDFRenderWriteInt(output, int64(length))
			output.WriteByte('\n')
		case nativePDFRenderLayoutWriteNil:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			output.WriteString("null")
		case nativePDFRenderLayoutWriteBool:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			truth, ok := layout.values[op.node].Data.(bool)
			if !ok {
				return false
			}
			if truth {
				output.WriteString("true")
			} else {
				output.WriteString("false")
			}
		case nativePDFRenderLayoutWriteInteger:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch || !nativePDFRenderWriteIntegerValue(output, layout.values[op.node]) {
				return false
			}
		case nativePDFRenderLayoutWriteFloat:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			floatValue, ok := layout.values[op.node].Data.(float64)
			if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
				return false
			}
			if !nativePDFRenderWriteReal(output, floatValue) {
				return false
			}
		case nativePDFRenderLayoutWriteSymbol:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			name, ok := layout.values[op.node].Data.(string)
			if !ok {
				return false
			}
			nativePDFRenderWriteSymbol(output, name)
		case nativePDFRenderLayoutWriteString:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			value := layout.values[op.node]
			raw, ok := value.Data.(string)
			if !ok {
				return false
			}
			if nativePDFRenderWriteASCIIString(output, raw, false) {
				continue
			}
			text, textOK := nativePDFStringText(value, false)
			if !textOK {
				return false
			}
			output.WriteString(text)
		case nativePDFRenderLayoutWriteASCIIString:
			if op.node < 0 || op.node >= len(layout.values) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			value := layout.values[op.node]
			raw, ok := value.Data.(string)
			if !ok || !nativePDFRenderASCIIString(raw) {
				return false
			}
			nativePDFRenderWriteASCIIStringUnchecked(output, raw, false)
		case nativePDFRenderLayoutWriteReference:
			if op.node < 0 || op.node >= len(layout.identifiers) || op.node >= len(layout.generations) || op.node >= len(layout.bound) || layout.bound[op.node] != layout.epoch {
				return false
			}
			nativePDFRenderWriteInt(output, layout.identifiers[op.node])
			output.WriteByte(' ')
			nativePDFRenderWriteInt(output, layout.generations[op.node])
			output.WriteString(" R")
		default:
			return false
		}
	}
	return true
}

// nativePDFRenderWriteLayoutProgramTrusted runs after the complete layout
// binder and reference/stream preflight have succeeded. It keeps the checked
// writer above for tests and defensive callers, while the real render pass
// avoids repeating the per-leaf epoch/bound checks that the binder already
// performed before any Ruby-visible mutation.
func nativePDFRenderWriteLayoutProgramTrusted(output *strings.Builder, layout *nativePDFRenderBoundLayout, program []nativePDFRenderLayoutWriteOp, length int) bool {
	if output == nil || layout == nil || layout.template == nil || len(program) == 0 {
		return false
	}
	for _, op := range program {
		switch op.kind {
		case nativePDFRenderLayoutWriteLiteral:
			output.WriteString(op.text)
		case nativePDFRenderLayoutWriteLength:
			nativePDFRenderWriteSymbol(output, "Length")
			output.WriteByte(' ')
			nativePDFRenderWriteInt(output, int64(length))
			output.WriteByte('\n')
		case nativePDFRenderLayoutWriteNil:
			output.WriteString("null")
		case nativePDFRenderLayoutWriteBool:
			if op.node < 0 || op.node >= len(layout.values) {
				return false
			}
			value := layout.values[op.node]
			if value == nil {
				return false
			}
			truth, ok := value.Data.(bool)
			if !ok {
				return false
			}
			if truth {
				output.WriteString("true")
			} else {
				output.WriteString("false")
			}
		case nativePDFRenderLayoutWriteInteger:
			if op.node < 0 || op.node >= len(layout.values) || !nativePDFRenderWriteIntegerValue(output, layout.values[op.node]) {
				return false
			}
		case nativePDFRenderLayoutWriteFloat:
			if op.node < 0 || op.node >= len(layout.values) {
				return false
			}
			value := layout.values[op.node]
			if value == nil {
				return false
			}
			floatValue, ok := value.Data.(float64)
			if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
				return false
			}
			if !nativePDFRenderWriteReal(output, floatValue) {
				return false
			}
		case nativePDFRenderLayoutWriteSymbol:
			if op.node < 0 || op.node >= len(layout.values) {
				return false
			}
			value := layout.values[op.node]
			if value == nil {
				return false
			}
			name, ok := value.Data.(string)
			if !ok {
				return false
			}
			nativePDFRenderWriteSymbol(output, name)
		case nativePDFRenderLayoutWriteString:
			if op.node < 0 || op.node >= len(layout.values) {
				return false
			}
			value := layout.values[op.node]
			if value == nil {
				return false
			}
			raw, ok := value.Data.(string)
			if !ok {
				return false
			}
			if nativePDFRenderWriteASCIIString(output, raw, false) {
				continue
			}
			text, textOK := nativePDFStringText(value, false)
			if !textOK {
				return false
			}
			output.WriteString(text)
		case nativePDFRenderLayoutWriteASCIIString:
			if op.node < 0 || op.node >= len(layout.values) {
				return false
			}
			value := layout.values[op.node]
			if value == nil {
				return false
			}
			raw, ok := value.Data.(string)
			if !ok {
				return false
			}
			nativePDFRenderWriteASCIIStringUnchecked(output, raw, false)
		case nativePDFRenderLayoutWriteReference:
			if op.node < 0 || op.node >= len(layout.identifiers) || op.node >= len(layout.generations) {
				return false
			}
			nativePDFRenderWriteInt(output, layout.identifiers[op.node])
			output.WriteByte(' ')
			nativePDFRenderWriteInt(output, layout.generations[op.node])
			output.WriteString(" R")
		default:
			return false
		}
	}
	return true
}

func nativePDFRenderWriteLayoutLeaf(output *strings.Builder, layout *nativePDFRenderBoundLayout, nodeIndex int, inContentStream bool) bool {
	if output == nil || layout == nil || layout.template == nil || nodeIndex < 0 || nodeIndex >= len(layout.template.nodes) || nodeIndex >= len(layout.values) || nodeIndex >= len(layout.bound) || layout.bound[nodeIndex] != layout.epoch {
		return false
	}
	node := layout.template.nodes[nodeIndex]
	value := layout.values[nodeIndex]
	switch node.kind {
	case object.ValueNil:
		output.WriteString("null")
		return true
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
		floatValue, ok := value.Data.(float64)
		if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return false
		}
		if !nativePDFRenderWriteReal(output, floatValue) {
			return false
		}
		return true
	case object.ValueSymbol:
		name, ok := value.Data.(string)
		if !ok {
			return false
		}
		nativePDFRenderWriteSymbol(output, name)
		return true
	case object.ValueString:
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
	case object.ValueObject:
		if nodeIndex >= len(layout.identifiers) || nodeIndex >= len(layout.generations) {
			return false
		}
		nativePDFRenderWriteInt(output, layout.identifiers[nodeIndex])
		output.WriteByte(' ')
		nativePDFRenderWriteInt(output, layout.generations[nodeIndex])
		output.WriteString(" R")
		return true
	default:
		return false
	}
}

func nativePDFRenderWriteLayoutNode(output *strings.Builder, layout *nativePDFRenderBoundLayout, nodeIndex int, inContentStream bool) bool {
	if output == nil || layout == nil || layout.template == nil || nodeIndex < 0 || nodeIndex >= len(layout.template.nodes) || nodeIndex >= len(layout.values) || nodeIndex >= len(layout.bound) || layout.bound[nodeIndex] != layout.epoch {
		return false
	}
	node := layout.template.nodes[nodeIndex]
	value := layout.values[nodeIndex]
	switch node.kind {
	case object.ValueNil:
		output.WriteString("null")
		return true
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
		floatValue, ok := value.Data.(float64)
		if !ok || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return false
		}
		output.WriteString(nativePDFRealText(floatValue))
		return true
	case object.ValueSymbol:
		name, ok := value.Data.(string)
		if !ok {
			return false
		}
		nativePDFRenderWriteSymbol(output, name)
		return true
	case object.ValueString:
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
		output.WriteByte('[')
		for index, child := range node.items {
			if index != 0 {
				output.WriteByte(' ')
			}
			if !nativePDFRenderWriteLayoutNode(output, layout, child, inContentStream) {
				return false
			}
		}
		output.WriteByte(']')
		return true
	case object.ValueHash:
		output.WriteString("<< ")
		for _, entry := range node.entries {
			nativePDFRenderWriteSymbol(output, entry.keyName)
			output.WriteByte(' ')
			if !nativePDFRenderWriteLayoutNode(output, layout, entry.node, inContentStream) {
				return false
			}
			output.WriteByte('\n')
		}
		output.WriteString(">>")
		return true
	case object.ValueObject:
		if nodeIndex >= len(layout.identifiers) || nodeIndex >= len(layout.generations) {
			return false
		}
		nativePDFRenderWriteInt(output, layout.identifiers[nodeIndex])
		output.WriteByte(' ')
		nativePDFRenderWriteInt(output, layout.generations[nodeIndex])
		output.WriteString(" R")
		return true
	default:
		return false
	}
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

func nativePDFRenderRealText(value float64) string {
	var buffer [32]byte
	encoded := strconv.AppendFloat(buffer[:0], value, 'f', 5, 64)
	end := len(encoded)
	for end > 0 && encoded[end-1] == '0' {
		end--
	}
	if end > 0 && encoded[end-1] == '.' {
		end--
	}
	if end == 2 && encoded[0] == '-' && encoded[1] == '0' {
		return "0"
	}
	return string(encoded[:end])
}

func nativePDFRenderWriteReal(output *strings.Builder, value float64) bool {
	if output == nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	var buffer [32]byte
	encoded := strconv.AppendFloat(buffer[:0], value, 'f', 5, 64)
	end := len(encoded)
	for end > 0 && encoded[end-1] == '0' {
		end--
	}
	if end > 0 && encoded[end-1] == '.' {
		end--
	}
	if end == 2 && encoded[0] == '-' && encoded[1] == '0' {
		encoded[0] = '0'
		end = 1
	}
	output.Write(encoded[:end])
	return true
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
	if !nativePDFRenderASCIIString(raw) {
		return false
	}
	nativePDFRenderWriteASCIIStringUnchecked(output, raw, inContentStream)
	return true
}

func nativePDFRenderWriteASCIIStringUnchecked(output *strings.Builder, raw string, inContentStream bool) {
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
