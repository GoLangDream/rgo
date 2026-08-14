package vm

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// BoundingBox#initialize is a small, allocation-only constructor on the
// default Prawn path.  The Ruby method still owns the compatibility behavior;
// this ABI is entered only for the exact class/source/layout shape below.
// RGO_DISABLE_NATIVE_PRAWN_CONSTRUCTOR provides an A/B switch without making
// the normal Prawn path opt-in.
var nativePrawnConstructorEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_CONSTRUCTOR") == ""
var nativePrawnDocumentConstructorEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_DOCUMENT_CONSTRUCTOR") == ""

// executeNativePrawnDocumentClassNew covers the exact no-options/no-block
// Prawn::Document constructor used by the ordinary Prawn steady state. The
// existing PDF constructor intrinsics already know how to build DocumentState,
// Renderer, Page, and BoundingBox; assembling them here removes the large
// Ruby initializer frame while keeping the resulting object fully observable.
// The exact closed-world shape is enabled by default; the disable switch keeps
// a direct A/B and emergency fallback available without changing semantics.
func (vm *VM) executeNativePrawnDocumentClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnDocumentConstructorEnabled || receiver == nil || receiver.Type != object.ValueClass ||
		len(args) != 0 || vm.currentBlock != nil || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() {
		return nil, false
	}
	classValue, ok := receiver.Data.(*object.Class)
	if !ok || classValue == nil || classValue.Name != "Prawn::Document" {
		return nil, false
	}
	documentClass := vm.resolveNativePrawnConstructorClass("Prawn::Document")
	if documentClass == nil || classValue != documentClass {
		return nil, false
	}
	initialize, owner, found := documentClass.GetMethodWithOwner("initialize")
	if !found || initialize == nil || owner != documentClass || initialize.DispatchOwner != nil ||
		initialize.Visibility != "" && initialize.Visibility != "public" || initialize.Ruby2Keywords {
		return nil, false
	}
	initializeFn, ok := initialize.Fn.(*object.Function)
	initShapeOK := ok && initializeFn != nil && initializeFn.Name == "initialize" &&
		strings.HasSuffix(initializeFn.SourcePath, "/prawn/document.rb") && len(initializeFn.Params) == 1 &&
		len(initializeFn.ParamDefaults) == 1 && initializeFn.HasBlockParam && !initializeFn.HasRestParam &&
		len(initializeFn.KeywordParams) == 0 && initializeFn.KeywordRestParam == "" && !initializeFn.KeywordRestOnly &&
		!initializeFn.RejectBlock
	if !initShapeOK {
		return nil, false
	}
	extensions := documentClass.GetInstanceVar("@extensions")
	if extensions != nil {
		if extensions.Type != object.ValueArray || extensions.Class != core.R.Classes["Array"] ||
			core.AttachedSingletonClass(extensions) != nil {
			return nil, false
		}
		extensionsLength := -1
		switch items := extensions.Data.(type) {
		case []*object.EmeraldValue:
			extensionsLength = len(items)
		case *object.RArray:
			if items != nil {
				extensionsLength = len(items.Elements)
			}
		}
		if extensionsLength != 0 {
			return nil, false
		}
	}
	prawnValue, found := vm.topLevelConstantValue("Prawn")
	if !found || prawnValue == nil || prawnValue.Type != object.ValueModule {
		return nil, false
	}
	prawnModule, ok := prawnValue.Data.(*object.Module)
	if !ok || prawnModule == nil || prawnModule.SingletonClass == nil {
		return nil, false
	}
	verifyOptions, verifyOwner, verifyFound := prawnModule.SingletonClass.GetMethodWithOwner("verify_options")
	if !verifyFound || verifyOptions == nil || verifyOwner != prawnModule.SingletonClass || verifyOptions.DispatchOwner != nil ||
		verifyOptions.Visibility != "" && verifyOptions.Visibility != "public" {
		return nil, false
	}
	verifyFn, ok := verifyOptions.Fn.(*object.Function)
	if !ok || verifyFn == nil || !strings.HasSuffix(verifyFn.SourcePath, "/prawn.rb") {
		return nil, false
	}

	stateClass := vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	rendererClass := vm.nativePDFConstructorClass("PDF::Core::Renderer")
	pageClass := vm.nativePDFConstructorClass("PDF::Core::Page")
	if stateClass == nil || rendererClass == nil || pageClass == nil {
		return nil, false
	}
	stateInitialize, stateOwner, stateFound := stateClass.GetMethodWithOwner("initialize")
	if _, valid := pdfConstructorFunction(stateInitialize, stateOwner, stateClass, stateFound, "/document_state.rb"); !valid {
		return nil, false
	}
	rendererInitialize, rendererOwner, rendererFound := rendererClass.GetMethodWithOwner("initialize")
	if _, valid := pdfConstructorFunction(rendererInitialize, rendererOwner, rendererClass, rendererFound, "/renderer.rb"); !valid {
		return nil, false
	}
	pageInitialize, pageOwner, pageFound := pageClass.GetMethodWithOwner("initialize")
	if _, valid := pdfConstructorFunction(pageInitialize, pageOwner, pageClass, pageFound, "/page.rb"); !valid {
		return nil, false
	}
	bboxValue, bboxFound := vm.qualifiedConstantValue("Prawn::Document::BoundingBox")
	if !bboxFound || bboxValue == nil || bboxValue.Type != object.ValueClass {
		return nil, false
	}
	bboxClass, ok := bboxValue.Data.(*object.Class)
	if !ok || bboxClass == nil {
		return nil, false
	}
	bboxInitialize, bboxOwner, bboxFound := bboxClass.GetMethodWithOwner("initialize")
	if _, valid := pdfConstructorFunction(bboxInitialize, bboxOwner, bboxClass, bboxFound, "/bounding_box.rb"); !valid {
		return nil, false
	}
	textFormatter, textFormatterFound := vm.qualifiedConstantValue("Prawn::Text::Formatted::Parser")
	if !textFormatterFound || textFormatter == nil || textFormatter.Type != object.ValueClass {
		return nil, false
	}

	document := object.NewObjectValue(documentClass)
	core.TrackObjectSpaceValue(document)
	options := nativePDFEmptyHash()
	state, handled := vm.nativePDFDocumentStateValue(stateClass, options)
	if !handled {
		return nil, false
	}
	core.TrackObjectSpaceValue(state)
	renderer, handled := vm.nativePDFRendererValue(rendererClass, state)
	if !handled {
		return nil, false
	}
	core.TrackObjectSpaceValue(renderer)
	core.SetDynamicInstanceVar(document, "@state", state)
	core.SetDynamicInstanceVar(document, "@renderer", renderer)
	core.SetDynamicInstanceVar(document, "@background", core.R.NilVal)
	core.SetDynamicInstanceVar(document, "@background_scale", core.NewIntegerValue(1))
	core.SetDynamicInstanceVar(document, "@font_size", core.NewIntegerValue(12))
	core.SetDynamicInstanceVar(document, "@bounding_box", core.R.NilVal)
	core.SetDynamicInstanceVar(document, "@margin_box", core.R.NilVal)
	// The first page is created inline below rather than through
	// Document#start_new_page, so mirror the post-initialization observable
	// state explicitly.  Prawn exposes page_number == 1 immediately after
	// Document.new; subsequent start_new_page calls increment it to 2, ...
	core.SetDynamicInstanceVar(document, "@page_number", core.NewIntegerValue(1))
	core.SetDynamicInstanceVar(document, "@text_formatter", textFormatter)

	pageOptions := nativePDFEmptyHash()
	page, handled := vm.nativePDFPageValue(pageClass, document, pageOptions)
	if !handled {
		return nil, false
	}
	core.TrackObjectSpaceValue(page)
	store := core.DynamicInstanceVar(state, "@store")
	if store == nil || store.Type != object.ValueObject {
		return nil, false
	}
	pageList := core.DynamicInstanceVar(state, "@pages")
	if pageList == nil || pageList.Type != object.ValueArray || !core.AppendArrayValue(pageList, page) {
		return nil, false
	}
	pageDictionaryID := core.DynamicInstanceVar(page, "@dictionary")
	if pageDictionaryID == nil || pageDictionaryID.Type != object.ValueInteger {
		return nil, false
	}
	objects := core.DynamicInstanceVar(store, "@objects")
	dictionary, dictionaryOK := core.DirectHashIndex(objects, pageDictionaryID)
	pagesReference, pagesOK := nativePDFObjectStorePagesReference(store, vm.nativePDFConstructorClass("PDF::Core::Reference"))
	if !dictionaryOK || dictionary == nil || !pagesOK || pagesReference == nil {
		return nil, false
	}
	pagesData := core.DynamicInstanceVar(pagesReference, "@data")
	kids, kidsFound := nativePDFLookupHashEntry(pagesData, "Kids")
	count, countFound := nativePDFLookupHashEntry(pagesData, "Count")
	countValue, countOK := int64(0), false
	if countFound && count != nil {
		countValue, countOK = count.Data.(int64)
	}
	if !kidsFound || !countOK || !core.AppendArrayValue(kids, dictionary) ||
		!core.StoreHashValue(pagesData, nativePDFSymbol("Count"), core.NewIntegerValue(countValue+1)) {
		return nil, false
	}
	core.SetDynamicInstanceVar(state, "@page", page)

	point := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
		core.NewIntegerValue(36),
		core.NewFloatValue(756),
	}, Class: core.R.Classes["Array"]}
	boxOptions := nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("width"), core.NewFloatValue(540)},
		[2]*object.EmeraldValue{nativePDFSymbol("height"), core.NewFloatValue(720)},
	)
	marginBox, handled := vm.executeNativePrawnClassNew(bboxValue, document, core.R.NilVal, point, boxOptions)
	if !handled {
		return nil, false
	}
	core.SetDynamicInstanceVar(document, "@margin_box", marginBox)
	core.SetDynamicInstanceVar(document, "@bounding_box", marginBox)
	core.SetDynamicInstanceVar(document, "@y", core.NewFloatValue(756))
	return document, true
}

func (vm *VM) executeNativePrawnClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnConstructorEnabled || receiver == nil || receiver.Type != object.ValueClass {
		return nil, false
	}
	cls, ok := receiver.Data.(*object.Class)
	if !ok || cls == nil {
		return nil, false
	}
	expected := vm.nativePrawnConstructorClass("Prawn::Document::BoundingBox")
	if cls != expected {
		return nil, false
	}
	if len(args) != 4 {
		return nil, false
	}
	initialize, owner, found := cls.GetMethodWithOwner("initialize")
	if !found || initialize == nil || owner != cls || initialize.DispatchOwner != nil ||
		initialize.Visibility != "" && initialize.Visibility != "public" || initialize.Ruby2Keywords {
		return nil, false
	}
	fn, ok := initialize.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "initialize" || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock ||
		len(fn.Params) != 4 ||
		!strings.HasSuffix(fn.SourcePath, "/prawn/document/bounding_box.rb") {
		return nil, false
	}

	point := args[2]
	if point == nil || point.Type != object.ValueArray || point.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(point) != nil {
		return nil, false
	}
	pointValues, ok := point.Data.([]*object.EmeraldValue)
	if !ok || len(pointValues) != 2 {
		return nil, false
	}

	options := args[3]
	if options == nil || options.Type != object.ValueHash || options.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(options) != nil {
		return nil, false
	}
	hash, ok := options.Data.(*object.RHash)
	if !ok || hash == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity ||
		len(hash.Keys) != len(hash.Pairs) {
		return nil, false
	}
	if fn.RejectKeywords && core.Ruby2KeywordHash(options) {
		return nil, false
	}
	width, found := nativePrawnSymbolHashValue(hash, "width")
	if !found || !width.IsTruthy() {
		return nil, false
	}
	height := core.R.NilVal
	if found, ok := nativePrawnSymbolHashValue(hash, "height"); ok && found != nil {
		height = found
	}

	obj := object.NewObjectValue(cls)
	data, ok := obj.Data.(*object.Object)
	if !ok || data == nil {
		return nil, false
	}
	data.SetInstanceVar("@document", args[0])
	data.SetInstanceVar("@parent", args[1])
	data.SetInstanceVar("@x", pointValues[0])
	data.SetInstanceVar("@y", pointValues[1])
	data.SetInstanceVar("@width", width)
	data.SetInstanceVar("@height", height)
	data.SetInstanceVar("@total_left_padding", core.NewIntegerValue(0))
	data.SetInstanceVar("@total_right_padding", core.NewIntegerValue(0))
	data.SetInstanceVar("@stretched_height", core.R.NilVal)
	core.TrackObjectSpaceValue(obj)
	return obj, true
}

func (vm *VM) nativePrawnConstructorClass(name string) *object.Class {
	if vm == nil {
		return nil
	}
	generation := object.CurrentConstantGeneration()
	if vm.nativePrawnConstructorConstantChecked && vm.nativePrawnConstructorConstantGeneration == generation {
		return vm.nativePrawnBoundingBoxClass
	}
	vm.nativePrawnConstructorConstantGeneration = generation
	vm.nativePrawnConstructorConstantChecked = true
	vm.nativePrawnBoundingBoxClass = vm.resolveNativePrawnConstructorClass(name)
	return vm.nativePrawnBoundingBoxClass
}

func (vm *VM) resolveNativePrawnConstructorClass(name string) *object.Class {
	value, found := vm.qualifiedConstantValue(name)
	if !found || value == nil || value.Type != object.ValueClass {
		return nil
	}
	cls, _ := value.Data.(*object.Class)
	return cls
}

func nativePrawnSymbolHashValue(hash *object.RHash, name string) (*object.EmeraldValue, bool) {
	if hash == nil || name == "" {
		return nil, false
	}
	for _, key := range hash.Keys {
		if key == nil || key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] {
			continue
		}
		keyName, ok := key.Data.(string)
		if !ok || keyName != name {
			continue
		}
		value, exists := hash.Pairs[key]
		if !exists || value == nil {
			return core.R.NilVal, true
		}
		return value, true
	}
	return nil, false
}
