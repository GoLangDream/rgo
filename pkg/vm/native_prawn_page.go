package vm

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// The page-break ABI is deliberately narrower than Prawn's public API.  It
// only replaces the no-options transition between two default LETTER pages;
// all option handling, callbacks, nested boxes, and customized page state
// remain in Ruby.
var nativePrawnStartNewPageEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_START_NEW_PAGE") == ""
var nativePrawnRenderFastEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_RENDER_FAST") == ""

// executeNativePrawnRenderFast removes only Document#render's empty repeater
// walk. The actual serialization remains PDF::Core::Renderer#render, so PDF
// callbacks, finalization, object ordering, and output errors keep their Ruby
// implementation and observable behavior.
func (vm *VM) executeNativePrawnRenderFast(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue, keywordSyntax bool) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnRenderFastEnabled || !nativePDFObjectEnabled || methodObj == nil ||
		methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" ||
		receiver == nil || receiver.Type != object.ValueObject || receiver.Class == nil ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 0 || keywordSyntax ||
		vm.currentBlock != nil || vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}
	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || receiver.Class != documentClass || !nativePrawnClassExtensionsEmpty(documentClass) {
		return nil, false
	}
	renderMethod, owner, found := documentClass.GetMethodWithOwner("render")
	if !found || renderMethod == nil || owner != documentClass || renderMethod != methodObj {
		return nil, false
	}
	fn, ok := renderMethod.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "render" || !strings.HasSuffix(fn.SourcePath, "/prawn/document.rb") ||
		len(fn.Params) != 0 || !fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock {
		return nil, false
	}

	state := core.DynamicInstanceVar(receiver, "@state")
	renderer := core.DynamicInstanceVar(receiver, "@renderer")
	stateClass := vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	rendererClass := vm.nativePDFConstructorClass("PDF::Core::Renderer")
	if state == nil || state.Type != object.ValueObject || state.Class != stateClass ||
		renderer == nil || renderer.Type != object.ValueObject || renderer.Class != rendererClass ||
		core.AttachedSingletonClass(state) != nil || core.AttachedSingletonClass(renderer) != nil ||
		core.DynamicInstanceVar(renderer, "@state") != state ||
		!nativePrawnExactMethodSource(rendererClass, "render", "/renderer.rb") ||
		!nativePrawnExactMethodSource(documentClass, "repeaters", "/prawn/repeater.rb") {
		return nil, false
	}
	pages := core.DynamicInstanceVar(state, "@pages")
	pageItems, pagesOK := nativePDFArrayItems(pages)
	if !pagesOK || pages == nil || pages.Type != object.ValueArray || pages.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(pages) != nil || len(pageItems) == 0 {
		return nil, false
	}
	repeaters := core.DynamicInstanceVar(receiver, "@repeaters")
	if repeaters != nil && repeaters.Type != object.ValueNil {
		repeaterItems, repeatersOK := nativePDFArrayItems(repeaters)
		if !repeatersOK || repeaters.Type != object.ValueArray || repeaters.Class != core.R.Classes["Array"] ||
			core.AttachedSingletonClass(repeaters) != nil || len(repeaterItems) != 0 {
			return nil, false
		}
	} else {
		// Document#render calls repeaters before Renderer#render, which creates
		// the empty memoized array on a fresh document. Preserve that visible
		// side effect before taking the shorter path.
		repeaters = nativePDFArrayValue()
		if core.SetDynamicInstanceVar(receiver, "@repeaters", repeaters) != nil {
			return nil, false
		}
	}

	// The wrapper's go_to_page loop leaves the document on its last page even
	// when the caller had previously selected another page. Renderer#render
	// performs the same finalization over state.pages, so only mirror the
	// document-level page number here.
	core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(int64(len(pageItems))))
	return vm.sendWithCallInfo(renderer, "render", nil, false), true
}

func (vm *VM) executeNativePrawnStartNewPage(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue, keywordSyntax bool) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnStartNewPageEnabled || !nativePrawnConstructorEnabled || !nativePDFObjectEnabled ||
		methodObj == nil || methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" ||
		receiver == nil || receiver.Type != object.ValueObject || receiver.Class == nil ||
		core.AttachedSingletonClass(receiver) != nil || keywordSyntax || vm.currentBlock != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 {
		return nil, false
	}

	documentClassValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentClassValue == nil || documentClassValue.Type != object.ValueClass {
		return nil, false
	}
	documentClass, ok := documentClassValue.Data.(*object.Class)
	if !ok || documentClass == nil || receiver.Class != documentClass || !nativePrawnClassExtensionsEmpty(documentClass) {
		return nil, false
	}

	startNewPage, owner, found := documentClass.GetMethodWithOwner("start_new_page")
	if !found || startNewPage == nil || owner != documentClass || methodObj != startNewPage {
		return nil, false
	}
	fn, ok := startNewPage.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "start_new_page" ||
		!strings.HasSuffix(fn.SourcePath, "/prawn/document.rb") ||
		len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock ||
		!nativePrawnStartNewPageNoOptions(args) {
		return nil, false
	}

	state := core.DynamicInstanceVar(receiver, "@state")
	stateClass := vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	pageClass := vm.nativePDFConstructorClass("PDF::Core::Page")
	if state == nil || state.Type != object.ValueObject || state.Class != stateClass ||
		core.AttachedSingletonClass(state) != nil || pageClass == nil ||
		!nativePrawnExactMethodSource(stateClass, "insert_page", "/document_state.rb") ||
		!nativePrawnExactMethodSource(stateClass, "on_page_create_action", "/document_state.rb") {
		return nil, false
	}

	page := core.DynamicInstanceVar(state, "@page")
	pages := core.DynamicInstanceVar(state, "@pages")
	pageItems, pagesOK := nativePDFArrayItems(pages)
	pageNumberValue := core.DynamicInstanceVar(receiver, "@page_number")
	pageNumber, pageNumberOK := nativePrawnIntegerValue(pageNumberValue)
	if page == nil || page.Type != object.ValueObject || page.Class != pageClass ||
		core.AttachedSingletonClass(page) != nil || !pagesOK || pages.Type != object.ValueArray ||
		pages.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(pages) != nil ||
		!pageNumberOK || pageNumber < 1 || pageNumber != int64(len(pageItems)) ||
		len(pageItems) == 0 || pageItems[len(pageItems)-1] != page ||
		!nativePrawnDefaultPageForStartNewPage(page, receiver) {
		return nil, false
	}

	callback := core.DynamicInstanceVar(state, "@on_page_create_callback")
	if callback != nil && callback.Type != object.ValueNil {
		return nil, false
	}
	background := core.DynamicInstanceVar(receiver, "@background")
	if background != nil && background.Type != object.ValueNil {
		return nil, false
	}
	boundingBox := core.DynamicInstanceVar(receiver, "@bounding_box")
	marginBox := core.DynamicInstanceVar(receiver, "@margin_box")
	boxValue, boxFound := vm.qualifiedConstantValue("Prawn::Document::BoundingBox")
	boxClass, boxOK := (*object.Class)(nil), false
	if boxFound && boxValue != nil && boxValue.Type == object.ValueClass {
		boxClass, boxOK = boxValue.Data.(*object.Class)
	}
	if !boxOK || boundingBox == nil || boundingBox != marginBox || boundingBox.Type != object.ValueObject ||
		boundingBox.Class != boxClass || core.AttachedSingletonClass(boundingBox) != nil ||
		!nativePrawnDefaultBoundingBoxForStartNewPage(boundingBox, receiver) {
		return nil, false
	}

	// These are the Ruby calls in start_new_page after the Page constructor.
	// Proving that the methods are still the Prawn implementations keeps an
	// override in an extension from being silently skipped.
	for _, methodName := range []string{"apply_margin_options", "generate_margin_box", "use_graphic_settings", "float"} {
		if !nativePrawnExactMethodSource(documentClass, methodName, "/prawn/document.rb") {
			return nil, false
		}
	}
	for _, methodName := range []string{"margins", "size", "layout", "graphic_state"} {
		if !nativePrawnPageMethodSource(pageClass, methodName) {
			return nil, false
		}
	}

	newPage, handled := vm.nativePDFPageValue(pageClass, receiver, nativePDFEmptyHash())
	if !handled {
		return nil, false
	}
	core.TrackObjectSpaceValue(newPage)

	store := core.DynamicInstanceVar(state, "@store")
	pageDictionaryID := core.DynamicInstanceVar(newPage, "@dictionary")
	objects := core.DynamicInstanceVar(store, "@objects")
	dictionary, dictionaryOK := core.DirectHashIndex(objects, pageDictionaryID)
	pagesReference, pagesOK := nativePDFObjectStorePagesReference(store, vm.nativePDFConstructorClass("PDF::Core::Reference"))
	if store == nil || store.Type != object.ValueObject || core.AttachedSingletonClass(store) != nil ||
		pageDictionaryID == nil || pageDictionaryID.Type != object.ValueInteger || !dictionaryOK || dictionary == nil ||
		!pagesOK || pagesReference == nil {
		return nil, false
	}
	pagesData := core.DynamicInstanceVar(pagesReference, "@data")
	kids, kidsFound := nativePDFLookupHashEntry(pagesData, "Kids")
	count, countFound := nativePDFLookupHashEntry(pagesData, "Count")
	countValue, countOK := int64(0), false
	if countFound && count != nil {
		countValue, countOK = count.Data.(int64)
	}
	kidItems, kidItemsOK := nativePDFArrayItems(kids)
	if !kidsFound || !countOK || countValue != int64(len(pageItems)) || !kidItemsOK ||
		len(kidItems) != len(pageItems) || !core.AppendArrayValue(pages, newPage) ||
		!core.AppendArrayValue(kids, dictionary) ||
		!core.StoreHashValue(pagesData, nativePDFSymbol("Count"), core.NewIntegerValue(countValue+1)) {
		return nil, false
	}

	point := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
		core.NewIntegerValue(36),
		core.NewFloatValue(756),
	}, Class: core.R.Classes["Array"]}
	boxOptions := nativePDFHashValue(
		[2]*object.EmeraldValue{nativePDFSymbol("width"), core.NewFloatValue(540)},
		[2]*object.EmeraldValue{nativePDFSymbol("height"), core.NewFloatValue(720)},
	)
	newMarginBox, handled := vm.executeNativePrawnClassNew(boxValue, receiver, core.R.NilVal, point, boxOptions)
	if !handled {
		return nil, false
	}
	core.SetDynamicInstanceVar(state, "@page", newPage)
	core.SetDynamicInstanceVar(receiver, "@margin_box", newMarginBox)
	core.SetDynamicInstanceVar(receiver, "@bounding_box", newMarginBox)
	core.SetDynamicInstanceVar(receiver, "@page_number", core.NewIntegerValue(pageNumber+1))
	core.SetDynamicInstanceVar(receiver, "@y", core.NewFloatValue(756))
	return core.R.NilVal, true
}

func nativePrawnStartNewPageNoOptions(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) != 1 || !nativePDFStandardHash(args[0]) {
		return false
	}
	hash, ok := args[0].Data.(*object.RHash)
	return ok && hash != nil && len(hash.Keys) == 0 && len(hash.Pairs) == 0
}

func nativePrawnExactMethodSource(cls *object.Class, name, sourceSuffix string) bool {
	if cls == nil {
		return false
	}
	method, owner, found := cls.GetMethodWithOwner(name)
	if !found || method == nil || owner != cls || method.DispatchOwner != nil {
		return false
	}
	fn, ok := method.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == name && strings.HasSuffix(fn.SourcePath, sourceSuffix)
}

func nativePrawnPageMethodSource(cls *object.Class, name string) bool {
	method, owner, found := cls.GetMethodWithOwner(name)
	if found && method != nil && owner == cls && method.DispatchOwner == nil &&
		method.AttrReaderName == "@"+name && method.AttrWriterName == "" && method.Fn != nil {
		return true
	}
	return nativePrawnExactMethodSource(cls, name, "/page.rb")
}

func nativePrawnDefaultPageForStartNewPage(page, document *object.EmeraldValue) bool {
	if page == nil || document == nil || page.Type != object.ValueObject ||
		core.AttachedSingletonClass(page) != nil || core.DynamicInstanceVar(page, "@document") != document ||
		!nativePrawnDefaultPageMargins(core.DynamicInstanceVar(page, "@margins")) ||
		!nativePrawnDefaultDashState(page) || !nativePrawnDefaultGraphicState(page) {
		return false
	}
	size := core.DynamicInstanceVar(page, "@size")
	layout := core.DynamicInstanceVar(page, "@layout")
	if size == nil || size.Type != object.ValueString || size.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(size) != nil || size.Data != "LETTER" || layout == nil ||
		layout.Type != object.ValueSymbol || layout.Class != core.R.Classes["Symbol"] || layout.Data != "portrait" {
		return false
	}
	for _, name := range []string{"@stamp_stream", "@stamp_dictionary"} {
		value := core.DynamicInstanceVar(page, name)
		if value != nil && value.Type != object.ValueNil {
			return false
		}
	}
	return true
}

func nativePrawnDefaultPageMargins(margins *object.EmeraldValue) bool {
	if !nativePDFStandardHash(margins) {
		return false
	}
	entries, ok := nativePDFHashEntries(margins)
	if !ok || len(entries) != 4 {
		return false
	}
	for _, name := range []string{"left", "right", "top", "bottom"} {
		value, found := nativePDFLookupHashEntry(margins, name)
		if !found || value == nil || value.Type != object.ValueInteger || value.Data != int64(36) {
			return false
		}
	}
	return true
}

func nativePrawnDefaultDashState(page *object.EmeraldValue) bool {
	stack := core.DynamicInstanceVar(page, "@stack")
	values := core.DynamicInstanceVar(stack, "@stack")
	items, ok := nativePDFArrayItems(values)
	if !ok || len(items) == 0 {
		return false
	}
	state := items[len(items)-1]
	dash := core.DynamicInstanceVar(state, "@dash")
	if !nativePDFStandardHash(dash) {
		return false
	}
	entries, ok := nativePDFHashEntries(dash)
	if !ok || len(entries) != 3 {
		return false
	}
	dashValue, dashFound := nativePDFLookupHashEntry(dash, "dash")
	spaceValue, spaceFound := nativePDFLookupHashEntry(dash, "space")
	phaseValue, phaseFound := nativePDFLookupHashEntry(dash, "phase")
	return dashFound && spaceFound && phaseFound && dashValue != nil && spaceValue != nil &&
		dashValue.Type == object.ValueNil && spaceValue.Type == object.ValueNil &&
		phaseValue != nil && phaseValue.Type == object.ValueInteger && phaseValue.Data == int64(0)
}

func nativePrawnDefaultBoundingBoxForStartNewPage(box, document *object.EmeraldValue) bool {
	if box == nil || document == nil || box.Type != object.ValueObject || core.AttachedSingletonClass(box) != nil ||
		core.DynamicInstanceVar(box, "@document") != document {
		return false
	}
	parent := core.DynamicInstanceVar(box, "@parent")
	leftPadding := core.DynamicInstanceVar(box, "@total_left_padding")
	rightPadding := core.DynamicInstanceVar(box, "@total_right_padding")
	stretchedHeight := core.DynamicInstanceVar(box, "@stretched_height")
	x, xOK := nativePrawnNumericValue(core.DynamicInstanceVar(box, "@x"))
	y, yOK := nativePrawnNumericValue(core.DynamicInstanceVar(box, "@y"))
	width, widthOK := nativePrawnNumericValue(core.DynamicInstanceVar(box, "@width"))
	height, heightOK := nativePrawnNumericValue(core.DynamicInstanceVar(box, "@height"))
	return parent != nil && parent.Type == object.ValueNil && leftPadding != nil && rightPadding != nil &&
		leftPadding.Type == object.ValueInteger && leftPadding.Data == int64(0) &&
		rightPadding.Type == object.ValueInteger && rightPadding.Data == int64(0) &&
		(stretchedHeight == nil || stretchedHeight.Type == object.ValueNil) && xOK && yOK && widthOK && heightOK &&
		x == 36 && y == 756 && width == 540 && height == 720
}

func nativePrawnIntegerValue(value *object.EmeraldValue) (int64, bool) {
	if value == nil || value.Type != object.ValueInteger {
		return 0, false
	}
	number, ok := value.Data.(int64)
	return number, ok
}
