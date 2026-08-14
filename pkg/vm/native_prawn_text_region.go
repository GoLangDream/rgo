package vm

// This file owns the immutable part of the real-object Prawn text ABI.  The
// text entry still checks the document/font/page state on every call because
// Ruby can mutate those objects.  Class, constant, method-source and builtin
// proofs, however, are stable until their corresponding runtime generation
// changes.  Keeping that proof in one per-VM plan turns the entry into a typed
// object-layout region with a clean Ruby side exit instead of repeating the
// same lookup work for every text call.

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePrawnTextLayoutRegionEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_TEXT_LAYOUT_REGION") == ""

type nativePrawnTextLayoutRegionPlan struct {
	methodGeneration   uint64
	constantGeneration uint64
	method             *object.Method
	documentClass      *object.Class
	boxClass           *object.Class
	formattedBoxClass  *object.Class
	fontClass          *object.Class
	stateClass         *object.Class
	pageClass          *object.Class
	storeClass         *object.Class
	referenceClass     *object.Class
	streamClass        *object.Class
	filterListClass    *object.Class
	hashBuiltinsOK     bool
	directTextReady    bool
	defaultFontReady   bool
	valid              bool
}

type nativePrawnTextHotState struct {
	methodGeneration   uint64
	constantGeneration uint64
	method             *object.Method
	documentClass      *object.Class
	state              *object.EmeraldValue
	page               *object.EmeraldValue
	boundingBox        *object.EmeraldValue
	font               *object.EmeraldValue
	stream             *object.EmeraldValue
	stack              *object.EmeraldValue
	stackValues        *object.EmeraldValue
	graphicState       *object.EmeraldValue
	fillColor          *object.EmeraldValue
	strokeColor        *object.EmeraldValue
	lineWidth          *object.EmeraldValue
	capStyle           *object.EmeraldValue
	joinStyle          *object.EmeraldValue
	colorSpace         *object.EmeraldValue
	stackLength        int
	boxX               float64
	boxY               float64
	boxWidth           float64
	boxHeight          float64
}

// nativePrawnTextLayoutIvar is the read side of the proven object layout.
// Ordinary compatibility objects keep their instance variables in the map, so
// the fast case can avoid the generic value-kind switch and method-shaped
// helper on every Prawn field read.  Compact objects and hot scalar sidecars
// still use Object.GetInstanceVar so the Ruby-visible representation is
// flushed before an observable read.
func nativePrawnTextLayoutIvar(value *object.EmeraldValue, name string) *object.EmeraldValue {
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

func (vm *VM) rememberNativePrawnTextHotState(methodObj *object.Method, receiver, state, page, boundingBox, font *object.EmeraldValue, boxX, boxY, boxWidth, boxHeight float64) {
	if vm == nil || methodObj == nil || receiver == nil || state == nil || page == nil || boundingBox == nil || font == nil {
		return
	}
	stream := nativePDFPageContentStream(page)
	stack := nativePrawnTextLayoutIvar(page, "@stack")
	stackValues := nativePrawnTextLayoutIvar(stack, "@stack")
	items, ok := nativePDFArrayItems(stackValues)
	if stream == nil || stack == nil || stackValues == nil || !ok || len(items) == 0 {
		return
	}
	graphicState := items[len(items)-1]
	if graphicState == nil {
		return
	}
	if vm.nativePrawnTextHotStates == nil {
		vm.nativePrawnTextHotStates = make(map[*object.EmeraldValue]*nativePrawnTextHotState)
	}
	vm.nativePrawnTextHotStates[receiver] = &nativePrawnTextHotState{
		methodGeneration:   object.CurrentMethodGeneration(),
		constantGeneration: object.CurrentConstantGeneration(),
		method:             methodObj,
		documentClass:      receiver.Class,
		state:              state,
		page:               page,
		boundingBox:        boundingBox,
		font:               font,
		stream:             stream,
		stack:              stack,
		stackValues:        stackValues,
		graphicState:       graphicState,
		fillColor:          nativePrawnTextLayoutIvar(graphicState, "@fill_color"),
		strokeColor:        nativePrawnTextLayoutIvar(graphicState, "@stroke_color"),
		lineWidth:          nativePrawnTextLayoutIvar(graphicState, "@line_width"),
		capStyle:           nativePrawnTextLayoutIvar(graphicState, "@cap_style"),
		joinStyle:          nativePrawnTextLayoutIvar(graphicState, "@join_style"),
		colorSpace:         nativePrawnTextLayoutIvar(graphicState, "@color_space"),
		stackLength:        len(items),
		boxX:               boxX,
		boxY:               boxY,
		boxWidth:           boxWidth,
		boxHeight:          boxHeight,
	}
}

func (vm *VM) nativePrawnTextHotStateFor(receiver *object.EmeraldValue, methodObj *object.Method, plan *nativePrawnTextLayoutRegionPlan) (*nativePrawnTextHotState, bool) {
	if vm == nil || !nativePrawnTextLayoutRegionEnabled || receiver == nil || methodObj == nil || plan == nil || vm.nativePrawnTextHotStates == nil {
		return nil, false
	}
	hot := vm.nativePrawnTextHotStates[receiver]
	if hot == nil || hot.methodGeneration != object.CurrentMethodGeneration() || hot.constantGeneration != object.CurrentConstantGeneration() ||
		hot.method != methodObj || hot.documentClass != receiver.Class || receiver.Class != plan.documentClass ||
		receiver.Frozen || core.AttachedSingletonClass(receiver) != nil {
		return nil, false
	}
	return hot, true
}

func nativePrawnTextHotValueSame(current, expected *object.EmeraldValue) bool {
	if current == nil || expected == nil {
		return current == expected
	}
	if current != expected || core.AttachedSingletonClass(current) != nil {
		return false
	}
	if current.Type == object.ValueString {
		currentText, currentOK := current.Data.(string)
		expectedText, expectedOK := expected.Data.(string)
		return currentOK && expectedOK && currentText == expectedText
	}
	return true
}

func nativePrawnTextHotGraphicStateGuard(page *object.EmeraldValue, hot *nativePrawnTextHotState) bool {
	if page == nil || hot == nil || page.Frozen || core.AttachedSingletonClass(page) != nil {
		return false
	}
	stack := nativePrawnTextLayoutIvar(page, "@stack")
	stackValues := nativePrawnTextLayoutIvar(stack, "@stack")
	items, ok := nativePDFArrayItems(stackValues)
	if stack != hot.stack || stackValues != hot.stackValues || !ok || len(items) != hot.stackLength || len(items) == 0 || items[len(items)-1] != hot.graphicState {
		return false
	}
	graphicState := hot.graphicState
	if graphicState == nil || graphicState.Frozen || core.AttachedSingletonClass(graphicState) != nil ||
		!nativePrawnTextHotValueSame(nativePrawnTextLayoutIvar(graphicState, "@fill_color"), hot.fillColor) ||
		!nativePrawnTextHotValueSame(nativePrawnTextLayoutIvar(graphicState, "@stroke_color"), hot.strokeColor) ||
		!nativePrawnTextHotValueSame(nativePrawnTextLayoutIvar(graphicState, "@line_width"), hot.lineWidth) ||
		!nativePrawnTextHotValueSame(nativePrawnTextLayoutIvar(graphicState, "@cap_style"), hot.capStyle) ||
		!nativePrawnTextHotValueSame(nativePrawnTextLayoutIvar(graphicState, "@join_style"), hot.joinStyle) {
		return false
	}
	colorSpace := nativePrawnTextLayoutIvar(graphicState, "@color_space")
	entries, entriesOK := nativePDFHashEntries(colorSpace)
	return colorSpace == hot.colorSpace && nativePDFStandardHash(colorSpace) && entriesOK && len(entries) == 0
}

// executeNativePrawnDirectTextHot is the steady-state half of the text
// region.  The first text call performs the complete document/font/resource
// proof; later calls reuse the proven object graph and only check fields Ruby
// can mutate between calls.  A miss is deliberately non-destructive so the
// caller can replay the full direct ABI or the original Ruby method.
func (vm *VM) executeNativePrawnDirectTextHot(methodObj *object.Method, receiver *object.EmeraldValue, text string, plan *nativePrawnTextLayoutRegionPlan) (*object.EmeraldValue, bool) {
	hot, ok := vm.nativePrawnTextHotStateFor(receiver, methodObj, plan)
	if !ok {
		return nil, false
	}
	state := nativePrawnTextLayoutIvar(receiver, "@state")
	page := nativePrawnTextLayoutIvar(state, "@page")
	boundingBox := nativePrawnTextLayoutIvar(receiver, "@bounding_box")
	if state != hot.state || page != hot.page || boundingBox != hot.boundingBox ||
		nativePrawnTextLayoutIvar(receiver, "@margin_box") != hot.boundingBox ||
		nativePrawnTextLayoutIvar(receiver, "@font") != hot.font ||
		!nativePrawnClassExtensionsEmpty(plan.boxClass) || !nativePrawnClassExtensionsEmpty(plan.formattedBoxClass) {
		return nil, false
	}
	boxX, boxXOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@x"))
	boxY, boxYOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@y"))
	boxWidth, boxWidthOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@width"))
	boxHeight, boxHeightOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@height"))
	currentY, currentYOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(receiver, "@y"))
	if !boxXOK || !boxYOK || !boxWidthOK || !boxHeightOK || !currentYOK || boxX != hot.boxX || boxY != hot.boxY ||
		boxWidth != hot.boxWidth || boxHeight != hot.boxHeight || !nativePrawnTextHotGraphicStateGuard(page, hot) {
		return nil, false
	}
	fontSizeValue := nativePrawnTextLayoutIvar(receiver, "@font_size")
	fontSize, fontSizeOK := nativePrawnNumericValue(fontSizeValue)
	if !fontSizeOK || fontSize != 12 {
		return nil, false
	}
	if stateValue := nativePrawnTextLayoutIvar(receiver, "@text_rendering_mode"); stateValue != nil && stateValue.Type != object.ValueNil {
		if stateValue.Type != object.ValueSymbol || stateValue.Data != "fill" || core.AttachedSingletonClass(stateValue) != nil {
			return nil, false
		}
	}
	if spacing := nativePrawnTextLayoutIvar(receiver, "@character_spacing"); spacing != nil && spacing.Type != object.ValueNil {
		value, valid := nativePrawnNumericValue(spacing)
		if !valid || value != 0 || core.AttachedSingletonClass(spacing) != nil {
			return nil, false
		}
	}
	if kerningDefault := nativePrawnTextLayoutIvar(receiver, "@default_kerning"); kerningDefault != nil && kerningDefault.Type != object.ValueNil {
		if kerningDefault.Type != object.ValueBool || kerningDefault.Data != true || core.AttachedSingletonClass(kerningDefault) != nil {
			return nil, false
		}
	}
	if direction := nativePrawnTextLayoutIvar(receiver, "@text_direction"); direction != nil && direction.Type != object.ValueNil {
		if direction.Type != object.ValueSymbol || direction.Data != "ltr" || core.AttachedSingletonClass(direction) != nil {
			return nil, false
		}
	}
	if fallback := nativePrawnTextLayoutIvar(receiver, "@fallback_fonts"); fallback != nil && fallback.Type != object.ValueNil {
		items, valid := nativePDFArrayItems(fallback)
		if fallback.Type != object.ValueArray || fallback.Class != core.R.Classes["Array"] || !valid || len(items) != 0 || core.AttachedSingletonClass(fallback) != nil {
			return nil, false
		}
	}
	if indent := nativePrawnTextLayoutIvar(receiver, "@indent_paragraphs"); indent != nil && indent.Type != object.ValueNil {
		return nil, false
	}
	stream := nativePDFPageContentStream(page)
	if stream != hot.stream || stream == nil || stream.Frozen || core.AttachedSingletonClass(stream) != nil {
		return nil, false
	}
	return vm.nativePrawnEmitDirectText(receiver, page, hot.font, text, boxX, currentY, fontSizeValue, plan)
}

func (vm *VM) nativePrawnTextLayoutPlanFor(methodObj *object.Method, receiverClass *object.Class) (*nativePrawnTextLayoutRegionPlan, bool) {
	if vm == nil || methodObj == nil || receiverClass == nil {
		return nil, false
	}
	methodGeneration := object.CurrentMethodGeneration()
	constantGeneration := object.CurrentConstantGeneration()
	cacheEnabled := nativePrawnTextLayoutRegionEnabled
	plan := &vm.nativePrawnTextLayoutPlan
	if !cacheEnabled {
		plan = &nativePrawnTextLayoutRegionPlan{}
	}
	if cacheEnabled && plan.methodGeneration == methodGeneration && plan.constantGeneration == constantGeneration &&
		plan.method == methodObj && plan.documentClass == receiverClass {
		if !plan.valid || !nativePrawnClassExtensionsEmpty(plan.boxClass) || !nativePrawnClassExtensionsEmpty(plan.formattedBoxClass) {
			return nil, false
		}
		return plan, true
	}

	*plan = nativePrawnTextLayoutRegionPlan{
		methodGeneration:   methodGeneration,
		constantGeneration: constantGeneration,
		method:             methodObj,
		documentClass:      receiverClass,
	}
	if receiverClass.Name != "Prawn::Document" || methodObj.DispatchOwner != nil {
		return nil, false
	}

	textFn, textOK := methodObj.Fn.(*object.Function)
	if !textOK || textFn == nil || methodObj.Name != "text" || textFn.Name != "text" ||
		!strings.HasSuffix(textFn.SourcePath, "/prawn/text.rb") || len(textFn.Params) != 2 ||
		len(textFn.ParamDefaults) != 2 || textFn.HasRestParam || textFn.HasBlockParam ||
		len(textFn.KeywordParams) != 0 || textFn.KeywordRestParam != "" || textFn.KeywordRestOnly {
		return nil, false
	}

	documentValue, found := vm.qualifiedConstantValue("Prawn::Document")
	if !found || documentValue == nil || documentValue.Type != object.ValueClass || documentValue.Data != receiverClass {
		return nil, false
	}
	boxValue, found := vm.qualifiedConstantValue("Prawn::Document::BoundingBox")
	if !found || boxValue == nil || boxValue.Type != object.ValueClass {
		return nil, false
	}
	boxClass, boxOK := boxValue.Data.(*object.Class)
	if !boxOK || boxClass == nil || !nativePrawnClassExtensionsEmpty(boxClass) {
		return nil, false
	}
	formattedBoxValue, found := vm.qualifiedConstantValue("Prawn::Text::Formatted::Box")
	if !found || formattedBoxValue == nil || formattedBoxValue.Type != object.ValueClass {
		return nil, false
	}
	formattedBoxClass, formattedBoxOK := formattedBoxValue.Data.(*object.Class)
	if !formattedBoxOK || formattedBoxClass == nil || !nativePrawnClassExtensionsEmpty(formattedBoxClass) {
		return nil, false
	}
	fontValue, found := vm.qualifiedConstantValue("Prawn::Fonts::AFM")
	if !found || fontValue == nil || fontValue.Type != object.ValueClass {
		return nil, false
	}
	fontClass, fontOK := fontValue.Data.(*object.Class)
	if !fontOK || fontClass == nil {
		return nil, false
	}

	plan.boxClass = boxClass
	plan.formattedBoxClass = formattedBoxClass
	plan.fontClass = fontClass
	plan.stateClass = vm.nativePDFConstructorClass("PDF::Core::DocumentState")
	plan.pageClass = vm.nativePDFConstructorClass("PDF::Core::Page")
	plan.storeClass = vm.nativePDFConstructorClass("PDF::Core::ObjectStore")
	plan.referenceClass = vm.nativePDFConstructorClass("PDF::Core::Reference")
	plan.streamClass = vm.nativePDFConstructorClass("PDF::Core::Stream")
	plan.filterListClass = vm.nativePDFConstructorClass("PDF::Core::FilterList")
	if plan.stateClass == nil || plan.pageClass == nil || plan.storeClass == nil || plan.referenceClass == nil ||
		plan.streamClass == nil || plan.filterListClass == nil {
		return nil, false
	}

	plan.hashBuiltinsOK = core.HashIndexUsesBuiltinImplementation() && core.HashStoreUsesBuiltinImplementation()
	plan.directTextReady = plan.hashBuiltinsOK &&
		nativeAFMMethodSource(fontClass, "normalize_encoding", "/prawn/fonts/afm.rb") &&
		nativeAFMMethodSource(fontClass, "has_kerning_data?", "/prawn/fonts/afm.rb") &&
		nativeAFMMethodSource(fontClass, "encode_text", "/prawn/fonts/afm.rb") &&
		nativeAFMMethodSource(fontClass, "register", "/prawn/fonts/afm.rb") &&
		nativeAFMMethodSource(fontClass, "symbolic?", "/prawn/fonts/afm.rb") &&
		nativeAFMMethodSource(fontClass, "add_to_current_page", "/prawn/font.rb") &&
		nativeAFMMethodSource(fontClass, "identifier_for", "/prawn/font.rb") &&
		nativeAFMMethodSource(plan.pageClass, "resources", "/page.rb") &&
		nativeAFMMethodSource(plan.pageClass, "fonts", "/page.rb")
	plan.defaultFontReady =
		nativeAFMMethodSource(receiverClass, "font", "/prawn/font.rb") &&
			nativeAFMMethodSource(receiverClass, "find_font", "/prawn/font.rb") &&
			nativeAFMMethodSource(receiverClass, "font_registry", "/prawn/font.rb") &&
			nativeAFMMethodSource(receiverClass, "font_families", "/prawn/font.rb") &&
			nativeAFMMethodSource(receiverClass, "set_font", "/prawn/font.rb")
	plan.valid = plan.directTextReady
	if !plan.valid {
		return nil, false
	}
	return plan, true
}
