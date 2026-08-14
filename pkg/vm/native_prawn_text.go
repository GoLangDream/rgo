package vm

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// nativePrawnTextEnabled keeps the AFM kernels independently reversible. The
// default path is still conservative: every receiver, method source, object
// layout, and core method dependency must match before this file can run.
var nativePrawnTextEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_TEXT") == ""
var nativePrawnAFMKernEnabled = os.Getenv("RGO_ENABLE_NATIVE_PRAWN_AFM_KERN") != ""
var nativePrawnAFMComputeEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_AFM_COMPUTE") == ""
var nativePrawnFontMetricEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_FONT_METRIC") == ""
var nativePrawnTextStateBlockEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_TEXT_STATE_BLOCK") == ""
var nativePrawnLineWrapMethodEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_LINE_WRAP_METHOD") == ""
var nativePrawnFormattedProcessTextEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_FORMATTED_PROCESS_TEXT") == ""

// The direct text ABI is closed-world: it keeps the real Prawn/PDF objects and
// enters only after the exact source, class, layout, encoding and dependency
// proofs below succeed. Make that narrow proof the ordinary default while
// retaining an explicit kill switch for compatibility bisects and fallback.
var nativePrawnDirectTextEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_DIRECT_TEXT") == ""

var nativePrawnTextModeValues = map[string]int64{
	"fill":             0,
	"stroke":           1,
	"fill_stroke":      2,
	"invisible":        3,
	"fill_clip":        4,
	"stroke_clip":      5,
	"fill_stroke_clip": 6,
	"clip":             7,
}

type nativeAFMKernTable struct {
	pairs map[[2]byte]int64
}

type nativePrawnLineWrapMethodProof struct {
	generation  uint64
	name        string
	utf8        *object.EmeraldValue
	windows1252 *object.EmeraldValue
	safe        bool
}

type nativePrawnFragmentProcessTextProof struct {
	generation uint64
	zwsp       *object.EmeraldValue
	safe       bool
}

func (vm *VM) nativePrawnTextBuiltinsAvailable() (bool, bool, bool) {
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnTextBuiltinsChecked && vm.nativePrawnTextBuiltinGeneration == generation {
		return vm.nativePrawnUnscaledBuiltinsOK, vm.nativePrawnKernBuiltinsOK, vm.nativePrawnComputeBuiltinsOK
	}
	vm.nativePrawnTextBuiltinGeneration = generation
	vm.nativePrawnTextBuiltinsChecked = true
	vm.nativePrawnUnscaledBuiltinsOK = core.PrawnAFMUnscaledWidthBuiltinsAvailable()
	vm.nativePrawnKernBuiltinsOK = core.PrawnAFMKernBuiltinsAvailable()
	vm.nativePrawnComputeBuiltinsOK = core.PrawnAFMComputeBuiltinsAvailable()
	return vm.nativePrawnUnscaledBuiltinsOK, vm.nativePrawnKernBuiltinsOK, vm.nativePrawnComputeBuiltinsOK
}

// executeNativePrawnTextMethod replaces only the two small AFM methods that
// dominate Prawn's repeated width calculations. It is called after
// invokeMethod has performed Ruby visibility checks, so private AFM helpers
// retain their normal access rules.
func (vm *VM) executeNativePrawnTextMethod(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextEnabled || !nativePrawnFormattedProcessTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Fonts::AFM" ||
		core.AttachedSingletonClass(receiver) != nil || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.instructionLimit != 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil {
		return nil, false
	}
	if !strings.HasSuffix(fn.SourcePath, "/prawn/fonts/afm.rb") {
		return nil, false
	}
	if fn.Name != "compute_width_of" && fn.Name != "kern" && fn.Name != "unscaled_width_of" {
		return nil, false
	}
	if fn.Name == "compute_width_of" {
		if len(args) < 1 || len(args) > 2 || !nativePrawnAFMComputeEnabled || !nativePrawnASCIIString(args[0]) {
			return nil, false
		}
	} else if len(args) != 1 {
		return nil, false
	}
	if !nativeAFMStringArgument(args[0]) {
		return nil, false
	}
	unscaledBuiltinsOK, kernBuiltinsOK, computeBuiltinsOK := vm.nativePrawnTextBuiltinsAvailable()
	if fn.Name == "compute_width_of" {
		if !computeBuiltinsOK || !vm.nativePrawnAFMDependenciesAvailable(receiver.Class) {
			return nil, false
		}
		return vm.executeNativePrawnAFMComputeWidth(receiver, args)
	}
	if fn.Name == "unscaled_width_of" {
		if !unscaledBuiltinsOK {
			return nil, false
		}
		return vm.executeNativePrawnAFMUnscaledWidth(receiver, args[0])
	}
	if !nativePrawnAFMKernEnabled || !kernBuiltinsOK {
		return nil, false
	}
	return vm.executeNativePrawnAFMKern(receiver, args[0])
}

// executeNativePrawnAFMSize handles AFM#size's exact one-send body. The
// method is protected but visibility has already been checked by the caller;
// this helper only removes its temporary Ruby frame when the document and
// font_size implementation are the exact Prawn objects expected by the Gem.
func (vm *VM) executeNativePrawnAFMSize(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Fonts::AFM" ||
		core.AttachedSingletonClass(receiver) != nil || vm.currentBlock != nil || len(args) != 0 ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "size" || !strings.HasSuffix(fn.SourcePath, "/prawn/font.rb") ||
		len(fn.Params) != 0 {
		return nil, false
	}
	document := core.DynamicInstanceVar(receiver, "@document")
	if document == nil || document.Type != object.ValueObject || document.Class == nil || document.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(document) != nil || !nativeAFMMethodSource(document.Class, "font_size", "/prawn/font.rb") {
		return nil, false
	}
	value := core.DynamicInstanceVar(document, "@font_size")
	if value == nil {
		value = core.R.NilVal
	}
	return value, true
}

// executeNativePrawnTextStateBlock handles only the no-state-change branch
// of PDF::Core::Text's numeric block wrappers. The Ruby implementation first
// compares the requested value with the current state; when they are equal it
// simply yields and does not enter the save/restore helper. Skipping that
// wrapper frame is safe only while the exact Gem method and all primitive
// operations used by its guard are still the builtins that were proven here.
// Any state change, unusual numeric representation, active unwind machinery,
// or instrumentation falls through to the original Ruby method.
func (vm *VM) executeNativePrawnTextStateBlock(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextStateBlockEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(receiver) != nil || vm.currentBlock == nil || len(args) != 1 ||
		args[0] == nil || args[0].Type == object.ValueNil || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || !strings.HasSuffix(fn.SourcePath, "/pdf/core/text.rb") ||
		len(fn.Params) != 1 || len(fn.ParamDefaults) != 1 || !fn.HasBlockParam || fn.HasRestParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		return nil, false
	}
	var instanceVar string
	var defaultInteger int64
	switch fn.Name {
	case "character_spacing", "word_spacing", "rise":
		instanceVar = "@" + fn.Name
	case "horizontal_text_scaling":
		instanceVar = "@horizontal_text_scaling"
		defaultInteger = 100
	case "text_rendering_mode":
		if !vm.nativePrawnTextStateBuiltinsAvailable() || args[0].Type != object.ValueSymbol ||
			!nativePrawnTextModesContains(vm, args[0]) {
			return nil, false
		}
		current := core.DynamicInstanceVar(receiver, "@text_rendering_mode")
		if !nativePrawnTextModeEqual(current, args[0]) {
			return nil, false
		}
		block := vm.currentBlock
		if result, executed := vm.tryExecuteZeroArgFramedBlock(block); executed {
			return result, true
		}
		return vm.callBlockWithSelfArgs(block, blockBindingSelf(block), nil), true
	default:
		return nil, false
	}
	if !vm.nativePrawnTextStateBuiltinsAvailable() {
		return nil, false
	}
	if !nativePrawnTextStateArgumentBuiltin(args[0]) {
		return nil, false
	}
	current := core.DynamicInstanceVar(receiver, instanceVar)
	if !nativePrawnTextStateEqual(current, args[0], defaultInteger) {
		return nil, false
	}
	block := vm.currentBlock
	return vm.callBlockWithSelfArgs(block, blockBindingSelf(block), nil), true
}

// executeNativePrawnDirectText is a closed-world fast path for the common
// `document.text "ASCII"` shape. It keeps the real Prawn document, font,
// ObjectStore and page objects, but skips Formatted::Box/Arranger/LineWrap
// frames. AFM normalization/encoding and font registration use a guarded Go
// ABI that reproduces the exact Prawn object mutations. Any non-default state
// or extension deopts before the page is mutated.
func (vm *VM) executeNativePrawnDirectText(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnDirectTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 1 ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	plan, planOK := vm.nativePrawnTextLayoutPlanFor(methodObj, receiver.Class)
	if !planOK {
		return nil, false
	}
	if args[0] == nil || args[0].Type != object.ValueString || args[0].Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(args[0]) != nil {
		return nil, false
	}
	text, asciiOK := nativePrawnSimpleASCII(args[0].Data)
	if !asciiOK {
		return nil, false
	}
	if result, executed := vm.executeNativePrawnDirectTextHot(methodObj, receiver, text, plan); executed {
		return result, true
	}
	if !nativePrawnClassExtensionsEmpty(plan.boxClass) || !nativePrawnClassExtensionsEmpty(plan.formattedBoxClass) {
		return nil, false
	}
	state := nativePrawnTextLayoutIvar(receiver, "@state")
	page := nativePrawnTextLayoutIvar(state, "@page")
	boundingBox := nativePrawnTextLayoutIvar(receiver, "@bounding_box")
	if state == nil || state.Type != object.ValueObject || state.Class != plan.stateClass ||
		page == nil || page.Type != object.ValueObject || page.Class != plan.pageClass ||
		!nativePDFPageHasGraphicState(page) || !nativePrawnDefaultGraphicState(page) || boundingBox == nil || boundingBox.Type != object.ValueObject ||
		boundingBox.Class != plan.boxClass || core.AttachedSingletonClass(boundingBox) != nil {
		return nil, false
	}
	boxX, boxXOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@x"))
	boxY, boxYOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@y"))
	boxWidth, boxWidthOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@width"))
	boxHeight, boxHeightOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(boundingBox, "@height"))
	currentY, currentYOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(receiver, "@y"))
	if !boxXOK || !boxYOK || !boxWidthOK || !boxHeightOK || !currentYOK || boxX != 36 || boxY != 756 || boxWidth != 540 || boxHeight != 720 ||
		nativePrawnTextLayoutIvar(receiver, "@margin_box") != boundingBox {
		return nil, false
	}
	fontSizeValue := nativePrawnTextLayoutIvar(receiver, "@font_size")
	fontSize, fontSizeOK := nativePrawnNumericValue(fontSizeValue)
	if !fontSizeOK || fontSize != 12 {
		return nil, false
	}
	if stateValue := nativePrawnTextLayoutIvar(receiver, "@text_rendering_mode"); stateValue != nil && stateValue.Type != object.ValueNil {
		if stateValue.Type != object.ValueSymbol || stateValue.Data != "fill" {
			return nil, false
		}
	}
	if spacing := nativePrawnTextLayoutIvar(receiver, "@character_spacing"); spacing != nil && spacing.Type != object.ValueNil {
		value, valid := nativePrawnNumericValue(spacing)
		if !valid || value != 0 {
			return nil, false
		}
	}
	if kerningDefault := nativePrawnTextLayoutIvar(receiver, "@default_kerning"); kerningDefault != nil && kerningDefault.Type != object.ValueNil {
		if kerningDefault.Type != object.ValueBool || kerningDefault.Data != true {
			return nil, false
		}
	}
	if direction := nativePrawnTextLayoutIvar(receiver, "@text_direction"); direction != nil && direction.Type != object.ValueNil {
		if direction.Type != object.ValueSymbol || direction.Data != "ltr" {
			return nil, false
		}
	}
	if fallback := nativePrawnTextLayoutIvar(receiver, "@fallback_fonts"); fallback != nil && fallback.Type != object.ValueNil {
		items, valid := nativePDFArrayItems(fallback)
		if fallback.Type != object.ValueArray || fallback.Class != core.R.Classes["Array"] || !valid || len(items) != 0 {
			return nil, false
		}
	}
	if indent := nativePrawnTextLayoutIvar(receiver, "@indent_paragraphs"); indent != nil && indent.Type != object.ValueNil {
		return nil, false
	}

	font := nativePrawnTextLayoutIvar(receiver, "@font")
	if font == nil || font.Type == object.ValueNil {
		if cachedFont, cached := vm.nativePrawnDefaultAFMFont(receiver, plan); cached {
			font = cachedFont
		} else {
			font = vm.sendBypassVisibility(receiver, "font", nil)
		}
		if font == nil || font.Type == object.ValueException {
			return font, true
		}
	}
	fontName := nativePrawnTextLayoutIvar(font, "@name")
	if font == nil || font.Type != object.ValueObject || font.Class != plan.fontClass ||
		core.AttachedSingletonClass(font) != nil || fontName == nil || fontName.Type != object.ValueString ||
		fontName.Data != "Helvetica" {
		return nil, false
	}
	result, handled := vm.nativePrawnEmitDirectText(receiver, page, font, text, boxX, currentY, fontSizeValue, plan)
	if handled && result == core.R.NilVal {
		vm.rememberNativePrawnTextHotState(methodObj, receiver, state, page, boundingBox, font, boxX, boxY, boxWidth, boxHeight)
	}
	return result, handled
}

func (vm *VM) nativePrawnEmitDirectText(receiver, page, font *object.EmeraldValue, text string, boxX, currentY float64, fontSizeValue *object.EmeraldValue, plan *nativePrawnTextLayoutRegionPlan) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || page == nil || font == nil || fontSizeValue == nil || plan == nil {
		return nil, false
	}
	vm.nativePrawnRememberDefaultAFMTemplate(font)
	payloadText, fontIdentifierText, kerning, encodedOK := vm.nativePrawnDirectAFMText(font, receiver, page, text, plan)
	if !encodedOK {
		return nil, false
	}
	fontAscender, fontDescender, fontLineGap, metricsOK := nativePrawnFontMetrics(font)
	if !metricsOK {
		return nil, false
	}
	fontSizeText, fontSizeTextOK := nativePDFObjectText(fontSizeValue, true, nil)
	if !fontSizeTextOK {
		return nil, false
	}
	var content strings.Builder
	content.WriteString("\nBT\n")
	content.WriteString(nativePrawnRealText(boxX))
	content.WriteByte(' ')
	content.WriteString(nativePrawnRealText(currentY - fontAscender))
	content.WriteString(" Td\n")
	content.WriteString(fontIdentifierText)
	content.WriteByte(' ')
	content.WriteString(fontSizeText)
	content.WriteString(" Tf\n")
	content.WriteString(payloadText)
	content.WriteByte(' ')
	operation := "Tj"
	if kerning {
		operation = "TJ"
	}
	content.WriteString(operation)
	content.WriteByte('\n')
	if fontIdentifierText == "" {
		return nil, false
	}
	content.WriteString("ET\n")
	if result, handled := nativePrawnAppendDirectContent(page, content.String()); !handled {
		return result, false
	} else if result != nil && result.Type == object.ValueException {
		return result, true
	}
	newY := core.NewFloatValue(currentY - (fontAscender + fontDescender + fontLineGap))
	if result := vm.sendBypassVisibility(receiver, "y=", []*object.EmeraldValue{newY}); result != nil && result.Type == object.ValueException {
		return result, true
	}
	core.SetDynamicInstanceVar(receiver, "@final_gap", core.R.TrueVal)
	core.SetDynamicInstanceVar(receiver, "@no_text_printed", core.R.FalseVal)
	core.SetDynamicInstanceVar(receiver, "@all_text_printed", core.R.TrueVal)
	return core.R.NilVal, true
}

// nativePrawnDirectAFMText is the second half of the direct text ABI.  The
// caller has already proved the document's layout and the public `text`
// method.  This helper proves the remaining AFM/Font/PDF object graph, then
// performs the equivalent of normalize_encoding, encode_text,
// add_to_current_page and identifier_for without entering Ruby frames.
//
// The proof is intentionally closed-world.  In particular, it only admits
// the standard mutable Hash implementation and the exact Prawn source
// methods.  A modified font, page resource hash, or PDF core method simply
// returns false and lets the ordinary VM execute the call.
func (vm *VM) nativePrawnDirectAFMText(font, document, page *object.EmeraldValue, raw string, plan *nativePrawnTextLayoutRegionPlan) (string, string, bool, bool) {
	if vm == nil || font == nil || document == nil || page == nil ||
		font.Type != object.ValueObject || document.Type != object.ValueObject || page.Type != object.ValueObject ||
		font.Class == nil || core.AttachedSingletonClass(font) != nil || core.AttachedSingletonClass(document) != nil ||
		core.AttachedSingletonClass(page) != nil || plan == nil || !plan.valid || !plan.hashBuiltinsOK ||
		font.Class != plan.fontClass || document.Class != plan.documentClass || page.Class != plan.pageClass {
		return "", "", false, false
	}
	if _, asciiOK := nativePrawnSimpleASCII(raw); !asciiOK {
		return "", "", false, false
	}
	fontName := nativePrawnTextLayoutIvar(font, "@name")
	identifier := nativePrawnTextLayoutIvar(font, "@identifier")
	fullEmbedding := nativePrawnTextLayoutIvar(font, "@full_font_embedding")
	attributes := nativePrawnTextLayoutIvar(font, "@attributes")
	if fontName == nil || fontName.Type != object.ValueString || fontName.Class != core.R.Classes["String"] ||
		fontName.Data != "Helvetica" || core.AttachedSingletonClass(fontName) != nil ||
		identifier == nil || identifier.Type != object.ValueSymbol || identifier.Class != core.R.Classes["Symbol"] ||
		core.AttachedSingletonClass(identifier) != nil || fullEmbedding == nil || fullEmbedding.Type != object.ValueBool ||
		fullEmbedding.Data != false || !nativePrawnDirectAFMAttributes(attributes) {
		return "", "", false, false
	}

	// `has_kerning_data?` is @kern_pairs.any?.  Keep that observable decision
	// separate from the parsed byte-pair table: a non-empty AFM pair hash still
	// produces a TJ array even when a particular input contains no pair.
	kernPairsValue := nativePrawnTextLayoutIvar(font, "@kern_pairs")
	kernPairs, kernPairsOK := nativePrawnDirectHashHeader(kernPairsValue)
	if !kernPairsOK {
		return "", "", false, false
	}
	kerning := len(kernPairs.Pairs) != 0
	pairs, pairsOK := vm.nativeAFMKernPairs(font)
	if !pairsOK {
		return "", "", false, false
	}

	referencesValue := nativePrawnTextLayoutIvar(font, "@references")
	cacheValue := nativePrawnTextLayoutIvar(font, "@subset_name_cache")
	if _, ok := nativePrawnDirectHashShape(referencesValue); !ok {
		return "", "", false, false
	}
	if _, ok := nativePrawnDirectHashShape(cacheValue); !ok {
		return "", "", false, false
	}
	referenceCached, referenceFound, referenceValid := nativePrawnDirectHashIntegerEntry(referencesValue, 0)
	if !referenceValid {
		return "", "", false, false
	}
	identifierCached, identifierFound, identifierValid := nativePrawnDirectHashIntegerEntry(cacheValue, 0)
	if !identifierValid {
		return "", "", false, false
	}
	state := nativePrawnTextLayoutIvar(document, "@state")
	store := nativePrawnTextLayoutIvar(state, "@store")
	objects := nativePrawnTextLayoutIvar(store, "@objects")
	identifiers := nativePrawnTextLayoutIvar(store, "@identifiers")
	storeClass := plan.storeClass
	referenceClass := plan.referenceClass
	streamClass := plan.streamClass
	filterClass := plan.filterListClass
	if state == nil || state.Type != object.ValueObject || state.Class != plan.stateClass ||
		storeClass == nil || referenceClass == nil || streamClass == nil || filterClass == nil ||
		store == nil || store.Type != object.ValueObject || store.Class != storeClass ||
		objects == nil || !nativePDFStandardHash(objects) || identifiers == nil || identifiers.Type != object.ValueArray ||
		identifiers.Class != core.R.Classes["Array"] || identifiers.Frozen || core.AttachedSingletonClass(identifiers) != nil {
		return "", "", false, false
	}
	if _, ok := identifiers.Data.([]*object.EmeraldValue); !ok {
		return "", "", false, false
	}

	pageDictionaryID := nativePrawnTextLayoutIvar(page, "@dictionary")
	if pageDictionaryID == nil || pageDictionaryID.Type != object.ValueInteger || pageDictionaryID.Class != core.R.Classes["Integer"] ||
		core.AttachedSingletonClass(pageDictionaryID) != nil {
		return "", "", false, false
	}
	dictionary, dictionarySafe := core.DirectHashIndex(objects, pageDictionaryID)
	if !dictionarySafe || !nativePDFExactObject(dictionary, "PDF::Core::Reference") || dictionary.Class != referenceClass ||
		core.AttachedSingletonClass(dictionary) != nil {
		return "", "", false, false
	}
	dictionaryData := nativePrawnTextLayoutIvar(dictionary, "@data")
	if _, dictionaryDataOK := nativePrawnDirectHashShape(dictionaryData); !dictionaryDataOK {
		return "", "", false, false
	}
	resources, resourcesFound, resourcesValid := nativePrawnDirectSymbolHashEntry(dictionaryData, "Resources")
	if !resourcesValid {
		return "", "", false, false
	}
	if !resourcesFound || nativePrawnDirectRubyFalsy(resources) {
		resources = nativePDFEmptyHash()
	}
	if _, ok := nativePrawnDirectHashShape(resources); !ok {
		return "", "", false, false
	}
	fonts, fontsFound, fontsValid := nativePrawnDirectSymbolHashEntry(resources, "Font")
	if !fontsValid {
		return "", "", false, false
	}
	if !fontsFound || nativePrawnDirectRubyFalsy(fonts) {
		fonts = nativePDFEmptyHash()
	}
	if _, ok := nativePrawnDirectHashShape(fonts); !ok {
		return "", "", false, false
	}
	identifierName, identifierOK := identifier.Data.(string)
	if _, identifierASCIIOK := nativePrawnSimpleASCII(identifierName); !identifierOK || identifierName == "" || !identifierASCIIOK {
		return "", "", false, false
	}
	expectedIdentifier := identifierName + ".0"
	fontIdentifier := nativePDFSymbol(expectedIdentifier)
	if identifierFound && !nativePrawnDirectRubyFalsy(identifierCached) {
		if identifierCached == nil || identifierCached.Type != object.ValueSymbol || identifierCached.Class != core.R.Classes["Symbol"] ||
			core.AttachedSingletonClass(identifierCached) != nil || identifierCached.Data != expectedIdentifier {
			return "", "", false, false
		}
		fontIdentifier = identifierCached
	}

	var reference *object.EmeraldValue
	if referenceFound && !nativePrawnDirectRubyFalsy(referenceCached) {
		if referenceCached == nil || referenceCached.Type != object.ValueObject || referenceCached.Class != referenceClass ||
			core.AttachedSingletonClass(referenceCached) != nil {
			return "", "", false, false
		}
		reference = referenceCached
	} else {
		// The RGo parser preserves the runtime Prawn hash order as
		// BaseFont, Subtype, Type, Encoding.  PDF output is byte-sensitive.
		fontData := nativePDFHashValue(
			[2]*object.EmeraldValue{nativePDFSymbol("BaseFont"), nativePDFSymbol("Helvetica")},
			[2]*object.EmeraldValue{nativePDFSymbol("Subtype"), nativePDFSymbol("Type1")},
			[2]*object.EmeraldValue{nativePDFSymbol("Type"), nativePDFSymbol("Font")},
			[2]*object.EmeraldValue{nativePDFSymbol("Encoding"), nativePDFSymbol("WinAnsiEncoding")},
		)
		var referenceOK bool
		reference, referenceOK = nativePDFStoreReference(store, referenceClass, streamClass, filterClass, fontData)
		if !referenceOK {
			return "", "", false, false
		}
	}
	fontReferenceID := nativePrawnTextLayoutIvar(reference, "@identifier")
	if fontReferenceID == nil || fontReferenceID.Type != object.ValueInteger || fontReferenceID.Class != core.R.Classes["Integer"] ||
		core.AttachedSingletonClass(fontReferenceID) != nil {
		return "", "", false, false
	}
	storedReference, storedSafe := core.DirectHashIndex(objects, fontReferenceID)
	if !storedSafe || storedReference != reference {
		return "", "", false, false
	}
	if !referenceFound || nativePrawnDirectRubyFalsy(referenceCached) {
		if !core.StoreHashValue(referencesValue, core.NewIntegerValue(0), reference) {
			return "", "", false, false
		}
	}
	if !identifierFound || nativePrawnDirectRubyFalsy(identifierCached) {
		if !core.StoreHashValue(cacheValue, core.NewIntegerValue(0), fontIdentifier) {
			return "", "", false, false
		}
	}
	if !core.StoreHashValue(fonts, fontIdentifier, reference) {
		return "", "", false, false
	}
	if !resourcesFound || nativePrawnDirectRubyFalsy(nativePrawnDirectHashValue(dictionaryData, "Resources")) {
		if !core.StoreHashValue(dictionaryData, nativePDFSymbol("Resources"), resources) {
			return "", "", false, false
		}
	}
	if !fontsFound || nativePrawnDirectRubyFalsy(nativePrawnDirectHashValue(resources, "Font")) {
		if !core.StoreHashValue(resources, nativePDFSymbol("Font"), fonts) {
			return "", "", false, false
		}
	}

	payload, payloadOK := nativePrawnDirectAFMPayload(raw, pairs, kerning)
	if !payloadOK {
		return "", "", false, false
	}
	return payload, nativePDFSymbolText(identifierNameForValue(fontIdentifier)), kerning, true
}

func nativePrawnDirectAFMAttributes(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	entries, ok := nativePDFHashEntries(value)
	if !ok {
		return false
	}
	for _, entry := range entries {
		if entry.keyName == "characterset" && entry.value != nil && entry.value.Type == object.ValueString && entry.value.Data == "Special" {
			return false
		}
	}
	return true
}

func nativePrawnDirectHashShape(value *object.EmeraldValue) (*object.RHash, bool) {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		value.Frozen || core.AttachedSingletonClass(value) != nil {
		return nil, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil || hash.Pairs == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity ||
		len(hash.Keys) != len(hash.Pairs) {
		return nil, false
	}
	seen := make(map[*object.EmeraldValue]struct{}, len(hash.Keys))
	for _, key := range hash.Keys {
		if key == nil {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		if _, exists := hash.Pairs[key]; !exists {
			return nil, false
		}
	}
	return hash, true
}

// nativePrawnDirectHashHeader is the O(1) proof used for large immutable
// AFM tables.  Callers that need to address individual keys must use the
// stronger nativePrawnDirectHashShape proof below.
func nativePrawnDirectHashHeader(value *object.EmeraldValue) (*object.RHash, bool) {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(value) != nil {
		return nil, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil || hash.Pairs == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity {
		return nil, false
	}
	return hash, true
}

func nativePrawnDirectHashIntegerEntry(value *object.EmeraldValue, wanted int64) (*object.EmeraldValue, bool, bool) {
	hash, ok := nativePrawnDirectHashShape(value)
	if !ok {
		return nil, false, false
	}
	var result *object.EmeraldValue
	found := false
	for _, key := range hash.Keys {
		if key.Type != object.ValueInteger || key.Class != core.R.Classes["Integer"] || core.AttachedSingletonClass(key) != nil || key.BigIntValue() != nil {
			return nil, false, false
		}
		integer, integerOK := key.Data.(int64)
		if !integerOK {
			return nil, false, false
		}
		if integer != wanted {
			continue
		}
		if found {
			return nil, false, false
		}
		result = hash.Pairs[key]
		found = true
	}
	return result, found, true
}

func nativePrawnDirectSymbolHashEntry(value *object.EmeraldValue, wanted string) (*object.EmeraldValue, bool, bool) {
	hash, ok := nativePrawnDirectHashShape(value)
	if !ok {
		return nil, false, false
	}
	var result *object.EmeraldValue
	found := false
	for _, key := range hash.Keys {
		if key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] || core.AttachedSingletonClass(key) != nil {
			return nil, false, false
		}
		name, nameOK := key.Data.(string)
		if !nameOK || name != wanted {
			continue
		}
		if found {
			return nil, false, false
		}
		result = hash.Pairs[key]
		found = true
	}
	return result, found, true
}

func nativePrawnDirectHashValue(value *object.EmeraldValue, wanted string) *object.EmeraldValue {
	result, found, valid := nativePrawnDirectSymbolHashEntry(value, wanted)
	if !valid || !found {
		return core.R.NilVal
	}
	return result
}

func nativePrawnDirectRubyFalsy(value *object.EmeraldValue) bool {
	return value == nil || value.Type == object.ValueNil || value.Type == object.ValueBool && value.Data == false
}

func nativePrawnDirectAFMPayload(raw string, pairs map[[2]byte]int64, kerning bool) (string, bool) {
	if !kerning {
		return "<" + hex.EncodeToString([]byte(raw)) + ">", true
	}
	var payload strings.Builder
	payload.WriteByte('[')
	start := 0
	for index := 1; index < len(raw); index++ {
		amount, found := pairs[[2]byte{raw[index-1], raw[index]}]
		if !found {
			continue
		}
		payload.WriteByte('<')
		payload.WriteString(hex.EncodeToString([]byte(raw[start:index])))
		payload.WriteString("> ")
		payload.WriteString(fmt.Sprintf("%d", -amount))
		payload.WriteByte(' ')
		start = index
	}
	payload.WriteByte('<')
	payload.WriteString(hex.EncodeToString([]byte(raw[start:])))
	payload.WriteString(">]")
	return payload.String(), true
}

func identifierNameForValue(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	name, _ := value.Data.(string)
	return name
}

func nativePrawnClassExtensionsEmpty(cls *object.Class) bool {
	if cls == nil {
		return false
	}
	extensions := cls.GetInstanceVar("@extensions")
	if extensions == nil || extensions.Type == object.ValueNil {
		return true
	}
	if extensions.Type != object.ValueArray || extensions.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(extensions) != nil {
		return false
	}
	items, ok := nativePDFArrayItems(extensions)
	return ok && len(items) == 0
}

func nativePrawnNumericValue(value *object.EmeraldValue) (float64, bool) {
	if value == nil {
		return 0, false
	}
	switch number := value.Data.(type) {
	case int64:
		return float64(number), true
	case float64:
		return number, true
	default:
		return 0, false
	}
}

func nativePrawnRealText(value float64) string {
	text := fmt.Sprintf("%.5f", value)
	for strings.HasSuffix(text, "0") && !strings.HasSuffix(text, ".0") {
		text = strings.TrimSuffix(text, "0")
	}
	return text
}

func nativePrawnDefaultGraphicState(page *object.EmeraldValue) bool {
	stack := nativePrawnTextLayoutIvar(page, "@stack")
	values := nativePrawnTextLayoutIvar(stack, "@stack")
	items, ok := nativePDFArrayItems(values)
	if !ok || len(items) == 0 {
		return false
	}
	state := items[len(items)-1]
	if !nativePDFExactObject(state, "PDF::Core::GraphicState") || core.AttachedSingletonClass(state) != nil {
		return false
	}
	fillColor := nativePrawnTextLayoutIvar(state, "@fill_color")
	strokeColor := nativePrawnTextLayoutIvar(state, "@stroke_color")
	lineWidth := nativePrawnTextLayoutIvar(state, "@line_width")
	capStyle := nativePrawnTextLayoutIvar(state, "@cap_style")
	joinStyle := nativePrawnTextLayoutIvar(state, "@join_style")
	if fillColor == nil || fillColor.Type != object.ValueString || fillColor.Data != "000000" ||
		strokeColor == nil || strokeColor.Type != object.ValueString || strokeColor.Data != "000000" ||
		lineWidth == nil || lineWidth.Type != object.ValueInteger || lineWidth.Data != int64(1) ||
		capStyle == nil || capStyle.Type != object.ValueSymbol || capStyle.Data != "butt" ||
		joinStyle == nil || joinStyle.Type != object.ValueSymbol || joinStyle.Data != "miter" {
		return false
	}
	colorSpace := nativePrawnTextLayoutIvar(state, "@color_space")
	entries, entriesOK := nativePDFHashEntries(colorSpace)
	return nativePDFStandardHash(colorSpace) && entriesOK && len(entries) == 0
}

func nativePrawnFontMetrics(font *object.EmeraldValue) (float64, float64, float64, bool) {
	if font == nil {
		return 0, 0, 0, false
	}
	ascender, ascenderOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(font, "@ascender"))
	descender, descenderOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(font, "@descender"))
	lineGap, lineGapOK := nativePrawnNumericValue(nativePrawnTextLayoutIvar(font, "@line_gap"))
	if !ascenderOK || !descenderOK || !lineGapOK {
		return 0, 0, 0, false
	}
	document := nativePrawnTextLayoutIvar(font, "@document")
	if document == nil {
		return 0, 0, 0, false
	}
	sizeValue := nativePrawnTextLayoutIvar(document, "@font_size")
	size, sizeOK := nativePrawnNumericValue(sizeValue)
	if !sizeOK {
		return 0, 0, 0, false
	}
	return ascender / 1000 * size, -descender / 1000 * size, lineGap / 1000 * size, true
}

func nativePrawnAppendDirectContent(page *object.EmeraldValue, content string) (*object.EmeraldValue, bool) {
	if page == nil || !nativePDFExactObject(page, "PDF::Core::Page") || !nativePDFPageHasGraphicState(page) {
		return nil, false
	}
	stream := nativePDFPageContentStream(page)
	if !nativePDFExactObject(stream, "PDF::Core::Stream") {
		return nil, false
	}
	streamData := nativePrawnTextLayoutIvar(stream, "@stream")
	if streamData == nil || streamData.Type == object.ValueNil {
		streamData = &object.EmeraldValue{Type: object.ValueString, Data: "", Class: core.R.Classes["String"], Encoding: "UTF-8"}
	}
	if streamData.Type != object.ValueString || streamData.Class != core.R.Classes["String"] || streamData.Frozen ||
		core.AttachedSingletonClass(streamData) != nil {
		return nil, false
	}
	contentValue := &object.EmeraldValue{Type: object.ValueString, Data: content, Class: core.R.Classes["String"], Encoding: "UTF-8"}
	if _, handled := core.AppendStringOneFast(streamData, contentValue); !handled {
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
	return nil, true
}

func nativePrawnTextModesContains(vm *VM, value *object.EmeraldValue) bool {
	if vm == nil || value == nil || value.Type != object.ValueSymbol || value.Class != core.R.Classes["Symbol"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	name, ok := value.Data.(string)
	if !ok {
		return false
	}
	if _, found := nativePrawnTextModeValues[name]; !found {
		return false
	}
	return vm.nativePrawnTextModesAvailable()
}

func (vm *VM) nativePrawnTextStateBuiltinsAvailable() bool {
	if vm == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnTextStateBuiltinsChecked && vm.nativePrawnTextStateBuiltinGeneration == generation {
		return vm.nativePrawnTextStateBuiltinsOK
	}
	vm.nativePrawnTextStateBuiltinGeneration = generation
	vm.nativePrawnTextStateBuiltinsChecked = true
	vm.nativePrawnTextStateBuiltinsOK = core.IntegerNilPredicateUsesBuiltinImplementation() &&
		core.IntegerEqualUsesBuiltinImplementation() && core.FloatNilPredicateUsesBuiltinImplementation() &&
		core.FloatEqualUsesBuiltinImplementation() && core.SymbolNilPredicateUsesBuiltinImplementation() &&
		core.SymbolEqualUsesBuiltinImplementation() && core.HashKeyUsesBuiltinImplementation()
	return vm.nativePrawnTextStateBuiltinsOK
}

func (vm *VM) nativePrawnTextModesAvailable() bool {
	if vm == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnTextModesChecked && vm.nativePrawnTextModesGeneration == generation {
		return vm.nativePrawnTextModesOK
	}
	vm.nativePrawnTextModesGeneration = generation
	vm.nativePrawnTextModesChecked = true
	vm.nativePrawnTextModesOK = false
	modes, found := vm.qualifiedConstantValue("PDF::Core::Text::MODES")
	if !found || modes == nil || modes.Type != object.ValueHash || modes.Class != core.R.Classes["Hash"] ||
		!modes.Frozen || core.AttachedSingletonClass(modes) != nil {
		return false
	}
	hash, ok := modes.Data.(*object.RHash)
	if !ok || hash == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity ||
		len(hash.Keys) != len(nativePrawnTextModeValues) || len(hash.Pairs) != len(nativePrawnTextModeValues) {
		return false
	}
	for _, key := range hash.Keys {
		if key == nil || key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] ||
			core.AttachedSingletonClass(key) != nil {
			return false
		}
		keyName, keyOK := key.Data.(string)
		keyExpected, keyFound := nativePrawnTextModeValues[keyName]
		entry, entryFound := hash.Pairs[key]
		if !keyOK || !keyFound || !entryFound || entry == nil || entry.Type != object.ValueInteger ||
			entry.Class != core.R.Classes["Integer"] || core.AttachedSingletonClass(entry) != nil {
			return false
		}
		entryValue, valueOK := entry.Data.(int64)
		if !valueOK || entryValue != keyExpected {
			return false
		}
	}
	vm.nativePrawnTextModesOK = true
	return true
}

func (vm *VM) nativePrawnFormattedDependenciesAvailable(arrangerClass, fragmentClass, documentClass *object.Class) bool {
	if vm == nil || arrangerClass == nil || fragmentClass == nil || documentClass == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnFormattedChecked && vm.nativePrawnFormattedGeneration == generation {
		return vm.nativePrawnFormattedOK
	}
	vm.nativePrawnFormattedGeneration = generation
	vm.nativePrawnFormattedChecked = true
	vm.nativePrawnFormattedOK = vm.nativePrawnTextStateBuiltinsAvailable() &&
		core.ObjectNilPredicateUsesBuiltinImplementation() && core.HashIndexUsesBuiltinImplementation() &&
		core.ArrayIncludeUsesBuiltinImplementation() &&
		nativeAFMMethodSource(fragmentClass, "font", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "size", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "character_spacing", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "styles", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(arrangerClass, "font_style", "/prawn/text/formatted/arranger.rb") &&
		nativeAFMMethodSource(documentClass, "character_spacing", "/pdf/core/text.rb")
	return vm.nativePrawnFormattedOK
}

func nativePrawnTextModeEqual(current, argument *object.EmeraldValue) bool {
	if argument == nil || argument.Type != object.ValueSymbol || argument.Class != core.R.Classes["Symbol"] ||
		core.AttachedSingletonClass(argument) != nil {
		return false
	}
	argumentName, argumentOK := argument.Data.(string)
	if !argumentOK {
		return false
	}
	if current == nil || current.Type == object.ValueNil || current == core.R.FalseVal {
		return argumentName == "fill"
	}
	if current.Type != object.ValueSymbol || current.Class != core.R.Classes["Symbol"] ||
		core.AttachedSingletonClass(current) != nil {
		return false
	}
	currentName, currentOK := current.Data.(string)
	return currentOK && currentName == argumentName
}

// executeNativePrawnFormattedApplyFontSize handles the common formatted-text
// branch where no explicit size or style is active. In that branch
// Arranger#apply_font_size only evaluates two nil?/membership predicates and
// yields; it does not touch the document font state. The source and dependent
// method guards keep private helper redefinitions and Array monkey patches on
// the ordinary frame path.
func (vm *VM) executeNativePrawnFormattedApplyFontSize(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Text::Formatted::Arranger" ||
		core.AttachedSingletonClass(receiver) != nil || vm.currentBlock == nil || len(args) != 2 ||
		args[0] == nil || args[0].Type != object.ValueNil || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode || len(vm.catchStack) != 0 ||
		len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "apply_font_size" || !strings.HasSuffix(fn.SourcePath, "/prawn/text/formatted/arranger.rb") ||
		len(fn.Params) != 2 || !fn.HasBlockParam || fn.HasRestParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		return nil, false
	}
	styles := args[1]
	if styles == nil {
		return nil, false
	}
	if styles.Type == object.ValueNil {
		if !core.NilPredicateUsesBuiltinImplementation() {
			return nil, false
		}
	} else if nativePrawnEmptyArray(styles) {
		if !core.ArrayNilPredicateUsesBuiltinImplementation() || !core.ArrayIncludeUsesBuiltinImplementation() {
			return nil, false
		}
	} else {
		return nil, false
	}
	arrangerClass := receiver.Class
	if !nativeAFMMethodSource(arrangerClass, "subscript?", "/prawn/text/formatted/arranger.rb") ||
		!nativeAFMMethodSource(arrangerClass, "superscript?", "/prawn/text/formatted/arranger.rb") {
		return nil, false
	}
	block := vm.currentBlock
	return vm.callBlockWithSelfArgs(block, blockBindingSelf(block), nil), true
}

func nativePrawnEmptyArray(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueArray || value.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(value) != nil {
		return false
	}
	switch data := value.Data.(type) {
	case []*object.EmeraldValue:
		return len(data) == 0
	case *object.RArray:
		return data != nil && len(data.Elements) == 0
	default:
		return false
	}
}

// executeNativePrawnFormattedApplyFontSettings collapses the default fragment
// branch of Arranger#apply_font_settings. A fragment with no font, size,
// character-spacing override, or styles reaches the caller block directly;
// every other format state retains the Ruby wrapper and its nested state
// transitions.
func (vm *VM) executeNativePrawnFormattedApplyFontSettings(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Text::Formatted::Arranger" ||
		core.AttachedSingletonClass(receiver) != nil || vm.currentBlock == nil || len(args) != 1 || args[0] == nil ||
		args[0].Type != object.ValueObject || args[0].Class == nil || args[0].Class.Name != "Prawn::Text::Formatted::Fragment" ||
		core.AttachedSingletonClass(args[0]) != nil || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.instructionLimit != 0 || DevMode || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "apply_font_settings" || !strings.HasSuffix(fn.SourcePath, "/prawn/text/formatted/arranger.rb") ||
		len(fn.Params) != 1 || !fn.HasBlockParam || fn.HasRestParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		return nil, false
	}
	arrangerClass := receiver.Class
	fragmentClass := args[0].Class
	formatState := core.DynamicInstanceVar(args[0], "@format_state")
	if formatState == nil || formatState.Type != object.ValueHash || formatState.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(formatState) != nil {
		return nil, false
	}
	hash, ok := formatState.Data.(*object.RHash)
	if !ok || hash == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity {
		return nil, false
	}
	font, fontFound := nativePrawnFormatStateField(hash, "font")
	size, _ := nativePrawnFormatStateField(hash, "size")
	spacing, spacingFound := nativePrawnFormatStateField(hash, "character_spacing")
	styles, stylesFound := nativePrawnFormatStateField(hash, "styles")
	if fontFound && font != nil && font != core.R.NilVal && font != core.R.FalseVal {
		return nil, false
	}
	if size != nil && size.Type != object.ValueNil {
		return nil, false
	}
	if spacingFound && spacing != nil && spacing != core.R.NilVal && spacing != core.R.FalseVal {
		return nil, false
	}
	if stylesFound && styles != nil && styles != core.R.NilVal && styles != core.R.FalseVal && !nativePrawnEmptyArray(styles) {
		return nil, false
	}
	document := core.DynamicInstanceVar(receiver, "@document")
	fragmentDocument := core.DynamicInstanceVar(args[0], "@document")
	if document == nil || fragmentDocument == nil || document != fragmentDocument ||
		document.Type != object.ValueObject || document.Class == nil || document.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(document) != nil {
		return nil, false
	}
	if !vm.nativePrawnFormattedDependenciesAvailable(arrangerClass, fragmentClass, document.Class) {
		return nil, false
	}
	currentSpacing := core.DynamicInstanceVar(document, "@character_spacing")
	if currentSpacing != nil && currentSpacing != core.R.NilVal && currentSpacing != core.R.FalseVal {
		return nil, false
	}
	block := vm.currentBlock
	if result, executed := vm.tryExecuteZeroArgFramedBlock(block); executed {
		return result, true
	}
	return vm.callBlockWithSelfArgs(block, blockBindingSelf(block), nil), true
}

// executeNativePrawnLineWrapMethod handles the fixed UTF-8 and Windows-1252
// branches of the formatted line wrapper's encoding helpers. These methods
// are tiny, but they are called thousands of times while tokenizing ordinary
// text. The helper is intentionally limited to the original Prawn source, the
// original SHY/ZWSP constants, and the builtin String#encode method; any
// other encoding or user redefinition returns to Ruby bytecode.
func (vm *VM) executeNativePrawnLineWrapMethod(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnLineWrapMethodEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Text::Formatted::LineWrap" ||
		core.AttachedSingletonClass(receiver) != nil || len(args) > 1 || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode || len(vm.catchStack) != 0 ||
		len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	name, utf8, windows1252, proven := vm.nativePrawnLineWrapMethodProofFor(methodObj)
	if !proven {
		return nil, false
	}
	if vm.currentBlock != nil {
		fn, ok := methodObj.Fn.(*object.Function)
		if !ok || !cachedBytecodeMethodIgnoresCallerBlock(fn) {
			return nil, false
		}
	}
	if name == "add_fragment_to_line" {
		return vm.executeNativePrawnLineWrapAddFragment(receiver, args)
	}
	if name == "fragment_finished" {
		return vm.executeNativePrawnLineWrapFragmentFinished(receiver, args)
	}
	if name == "tokenize" {
		if len(args) != 1 || !vm.nativePrawnLineWrapTokenizeSource(receiver.Class) {
			return nil, false
		}
		return nativePrawnLineWrapTokenize(args[0])
	}
	encoding := utf8
	windows1252Encoding := false
	if len(args) == 1 {
		switch args[0] {
		case utf8:
		case windows1252:
			encoding = windows1252
			windows1252Encoding = true
		default:
			return nil, false
		}
	}
	if encoding == nil {
		return nil, false
	}
	if len(args) == 1 && core.AttachedSingletonClass(args[0]) != nil {
		return nil, false
	}
	var raw string
	switch name {
	case "soft_hyphen":
		if windows1252Encoding {
			raw = string([]byte{0xad})
		} else {
			raw = "\u00ad"
		}
	case "zero_width_space":
		if windows1252Encoding {
			return core.R.NilVal, true
		}
		raw = "\u200b"
	case "whitespace":
		raw = " \t"
		if !windows1252Encoding {
			raw += "\u200b"
		}
	case "break_chars":
		raw = " \t"
		if !windows1252Encoding {
			raw += "\u200b\u00ad"
		}
		raw += "-"
	case "hyphen":
		raw = "-"
	}
	result := core.NewStringValue(raw)
	if windows1252Encoding && name != "hyphen" {
		core.SetStringEncoding(result, "Windows-1252")
	} else {
		core.SetStringEncoding(result, "UTF-8")
	}
	return result, true
}

// executeNativePrawnLineWrapAddFragment handles the common no-wrap branch of
// LineWrap#add_fragment_to_line. It preflights every token and width before
// changing the LineWrap object; a token that would wrap, a nonstandard numeric
// value, or a custom dependency returns to the original Ruby method before
// any mutation. Once admitted, only the final fragment_finished call remains
// on the normal dispatcher, preserving its output bookkeeping and error path.
func (vm *VM) executeNativePrawnLineWrapAddFragment(receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || len(args) != 1 || receiver == nil || receiver.Frozen || args[0] == nil ||
		args[0].Type != object.ValueString || args[0].Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(args[0]) != nil || !core.StringPlusUsesBuiltinImplementation() ||
		!core.StringEqualUsesBuiltinImplementation() || !core.StringIndexUsesBuiltinImplementation() ||
		!core.FloatPlusUsesBuiltinImplementation() || !core.FloatLessThanOrEqualUsesBuiltinImplementation() ||
		!core.IntegerPlusUsesBuiltinImplementation() || !core.IntegerLessThanOrEqualUsesBuiltinImplementation() {
		return nil, false
	}
	fragment := args[0]
	raw, rawOK := fragment.Data.(string)
	if !rawOK || raw == "" || raw == "\n" {
		return nil, false
	}
	if !nativeAFMMethodSource(receiver.Class, "tokenize", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "fragment_finished", "/prawn/text/formatted/line_wrap.rb") {
		return nil, false
	}
	document := core.DynamicInstanceVar(receiver, "@document")
	arranger := core.DynamicInstanceVar(receiver, "@arranger")
	accumulatedValue := core.DynamicInstanceVar(receiver, "@accumulated_width")
	widthValue := core.DynamicInstanceVar(receiver, "@width")
	fragmentOutput := core.DynamicInstanceVar(receiver, "@fragment_output")
	kerning := core.DynamicInstanceVar(receiver, "@kerning")
	if document == nil || document.Type != object.ValueObject || document.Class == nil || document.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(document) != nil || arranger == nil || arranger.Type != object.ValueObject ||
		arranger.Class == nil || arranger.Class.Name != "Prawn::Text::Formatted::Arranger" ||
		core.AttachedSingletonClass(arranger) != nil || fragmentOutput == nil || fragmentOutput.Type != object.ValueString ||
		fragmentOutput.Class != core.R.Classes["String"] || core.AttachedSingletonClass(fragmentOutput) != nil ||
		accumulatedValue == nil || widthValue == nil {
		return nil, false
	}
	if kerning != nil && kerning.Type != object.ValueNil && kerning.Type != object.ValueBool {
		return nil, false
	}
	accumulated, accumulatedOK := nativeAFMNumber(accumulatedValue)
	width, widthOK := nativeAFMNumber(widthValue)
	if !accumulatedOK || !widthOK || math.IsNaN(accumulated) || math.IsNaN(width) {
		return nil, false
	}
	tokens, tokenized := nativePrawnLineWrapTokenize(fragment)
	if !tokenized || tokens == nil || tokens.Type != object.ValueArray {
		return nil, false
	}
	tokenValues, valuesOK := tokens.Data.([]*object.EmeraldValue)
	if !valuesOK {
		return nil, false
	}
	outputRaw, outputOK := fragmentOutput.Data.(string)
	if !outputOK {
		return nil, false
	}
	outputProbe := *fragmentOutput
	outputEncoding := fragmentOutput.Encoding
	for _, segment := range tokenValues {
		if segment == nil || segment.Type != object.ValueString || segment.Class != core.R.Classes["String"] ||
			core.AttachedSingletonClass(segment) != nil {
			return nil, false
		}
		segmentRaw, segmentOK := segment.Data.(string)
		if !segmentOK {
			return nil, false
		}
		segmentWidth := float64(0)
		if !nativePrawnLineWrapZeroWidthSpace(segment) {
			var widthOK bool
			segmentWidth, widthOK = vm.nativePrawnLineWrapSegmentWidth(document, segment, kerning)
			if !widthOK {
				return nil, false
			}
		}
		nextAccumulated := accumulated + segmentWidth
		if nextAccumulated > width {
			return nil, false
		}
		if nativePrawnLineWrapTrailingSoftHyphen(segment) {
			softHyphen := nativePrawnLineWrapSoftHyphenValue(segment.Encoding)
			if softHyphen == nil {
				return nil, false
			}
			softWidth, softOK := vm.nativePrawnLineWrapSegmentWidth(document, softHyphen, kerning)
			if !softOK {
				return nil, false
			}
			nextAccumulated -= softWidth
		}
		encoding, encodingErr := core.StringConcatenationEncoding(&outputProbe, segment)
		if encodingErr != nil {
			return nil, false
		}
		outputRaw += segmentRaw
		outputProbe.Data = outputRaw
		outputProbe.Encoding = encoding
		outputEncoding = encoding
		accumulated = nextAccumulated
	}
	newOutput := core.NewStringValue(outputRaw)
	if outputEncoding != "" {
		core.SetStringEncoding(newOutput, outputEncoding)
	}
	if result := core.SetDynamicInstanceVar(receiver, "@accumulated_width", core.NewFloatValue(accumulated)); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(receiver, "@fragment_output", newOutput); result != nil {
		return result, true
	}
	finishedMethod, owner, found := receiver.Class.GetMethodWithOwner("fragment_finished")
	if !found || finishedMethod == nil {
		return nil, false
	}
	previousBlock := vm.currentBlock
	vm.currentBlock = nil
	finished := vm.invokeMethod(receiver, "private", "fragment_finished", []*object.EmeraldValue{fragment}, finishedMethod, owner, false)
	vm.currentBlock = previousBlock
	if finished != nil && finished.Type == object.ValueException {
		return finished, true
	}
	return core.R.TrueVal, true
}

func (vm *VM) nativePrawnLineWrapSegmentWidth(document, segment, kerning *object.EmeraldValue) (float64, bool) {
	if vm == nil || document == nil || segment == nil {
		return 0, false
	}
	method, _, found := document.Class.GetMethodWithOwner("width_of")
	if !found || method == nil {
		return 0, false
	}
	if kerning == nil {
		kerning = core.R.NilVal
	}
	options := nativePDFHashValue([2]*object.EmeraldValue{nativePDFSymbol("kerning"), kerning})
	value, executed := vm.executeNativePrawnFontMetricWidth(method, document, []*object.EmeraldValue{segment, options})
	if !executed {
		return 0, false
	}
	return nativeAFMNumber(value)
}

func (vm *VM) executeNativePrawnLineWrapFragmentFinished(receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Frozen || len(args) != 1 || args[0] == nil ||
		args[0].Type != object.ValueString || args[0].Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(args[0]) != nil || !core.ArrayLastUsesBuiltinImplementation() ||
		!core.HashStoreUsesBuiltinImplementation() || !core.StringLengthUsesBuiltinImplementation() ||
		!core.StringEqualUsesBuiltinImplementation() {
		return nil, false
	}
	fragmentRaw, fragmentOK := args[0].Data.(string)
	fragmentEncoding := args[0].Encoding
	if fragmentEncoding == "" {
		fragmentEncoding = "UTF-8"
	}
	if !fragmentOK || fragmentRaw == "" || fragmentRaw == "\n" ||
		!nativeAFMMethodSource(receiver.Class, "update_output_based_on_last_fragment", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "update_line_status_based_on_last_output", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "pull_preceding_fragment_to_join_this_one?", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "remember_this_fragment_for_backward_looking_ops", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "word_division_scan_pattern", "/prawn/text/formatted/line_wrap.rb") ||
		!nativeAFMMethodSource(receiver.Class, "break_chars", "/prawn/text/formatted/line_wrap.rb") ||
		!nativePrawnUTF8Encoding(fragmentEncoding) && fragmentEncoding != "Windows-1252" && fragmentEncoding != "CP1252" {
		return nil, false
	}
	fragmentOutput := core.DynamicInstanceVar(receiver, "@fragment_output")
	outputRaw, outputOK := stringValueRaw(fragmentOutput)
	if !outputOK || outputRaw == "" || outputRaw != fragmentRaw {
		return nil, false
	}
	arranger := core.DynamicInstanceVar(receiver, "@arranger")
	consumed := core.DynamicInstanceVar(arranger, "@consumed")
	if arranger == nil || arranger.Type != object.ValueObject || arranger.Class == nil || arranger.Class.Name != "Prawn::Text::Formatted::Arranger" ||
		core.AttachedSingletonClass(arranger) != nil || consumed == nil || consumed.Type != object.ValueArray ||
		consumed.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(consumed) != nil || consumed.Frozen {
		return nil, false
	}
	consumedValues, consumedOK := consumed.Data.([]*object.EmeraldValue)
	if !consumedOK || len(consumedValues) == 0 {
		return nil, false
	}
	lastConsumed := consumedValues[len(consumedValues)-1]
	if lastConsumed == nil || lastConsumed.Type != object.ValueHash || lastConsumed.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(lastConsumed) != nil || lastConsumed.Frozen {
		return nil, false
	}
	lastHash, hashOK := lastConsumed.Data.(*object.RHash)
	if !hashOK || lastHash == nil || lastHash.Default != nil || lastHash.DefaultProc != nil || lastHash.CompareByIdentity {
		return nil, false
	}
	outputEncoding := fragmentOutput.Encoding
	if outputEncoding == "" {
		outputEncoding = "UTF-8"
	}
	previousPrefix, trailingBreakIndex := nativePrawnLineWrapRememberShape(outputRaw, outputEncoding)
	previous := core.NewStringValue(outputRaw)
	core.SetStringEncoding(previous, outputEncoding)
	previousPrefixValue := core.NewStringValue(previousPrefix)
	core.SetStringEncoding(previousPrefixValue, outputEncoding)
	trailingValue := core.R.NilVal
	if trailingBreakIndex >= 0 {
		trailingValue = core.NewIntegerValue(int64(trailingBreakIndex))
	}
	lineContainsWordDivision := nativePrawnLineWrapHasWordDivision(outputRaw, outputEncoding)
	softHyphen := nativePrawnLineWrapSoftHyphenValue(fragmentEncoding)
	if softHyphen == nil {
		return nil, false
	}
	if !core.StoreHashValue(lastConsumed, nativePDFSymbol("text"), fragmentOutput) ||
		!core.StoreHashValue(lastConsumed, nativePDFSymbol("normalized_soft_hyphen"), softHyphen) {
		return nil, false
	}
	// Prawn's update_line_status_based_on_last_output only ever turns this
	// flag on.  A later fragment must not clear an earlier word-division mark;
	// the normal line-wrap path clears it only in its explicit line-reset case.
	if lineContainsWordDivision {
		if result := core.SetDynamicInstanceVar(receiver, "@line_contains_more_than_one_word", core.R.TrueVal); result != nil {
			return result, true
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, "@previous_fragment", previous); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(receiver, "@previous_fragment_ended_with_breakable", trailingValue); result != nil {
		return result, true
	}
	if result := core.SetDynamicInstanceVar(receiver, "@previous_fragment_output_without_last_word", previousPrefixValue); result != nil {
		return result, true
	}
	return previousPrefixValue, true
}

func stringValueRaw(value *object.EmeraldValue) (string, bool) {
	if value == nil || value.Type != object.ValueString || value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(value) != nil {
		return "", false
	}
	raw, ok := value.Data.(string)
	return raw, ok
}

func nativePrawnLineWrapRememberShape(raw, encoding string) (string, int) {
	if nativePrawnUTF8Encoding(encoding) {
		runes := []rune(raw)
		lastBreak := -1
		byteOffset := 0
		lastBreakByte := -1
		for index, value := range runes {
			if nativePrawnLineWrapRuneBreak(value) {
				lastBreak = index
				lastBreakByte = byteOffset
			}
			byteOffset += utf8.RuneLen(value)
		}
		prefixEnd := lastBreak + 1
		trailing := -1
		if lastBreak == len(runes)-1 {
			trailing = lastBreakByte
		}
		return string(runes[:prefixEnd]), trailing
	}
	lastBreak := -1
	for index := 0; index < len(raw); index++ {
		if nativePrawnLineWrapByteBreak(raw[index], 0xad) {
			lastBreak = index
		}
	}
	prefixEnd := lastBreak + 1
	trailing := -1
	if lastBreak == len(raw)-1 {
		trailing = lastBreak
	}
	return raw[:prefixEnd], trailing
}

func nativePrawnLineWrapHasWordDivision(raw, encoding string) bool {
	if nativePrawnUTF8Encoding(encoding) {
		for _, value := range raw {
			if nativePrawnLineWrapRuneBreak(value) || value == '\n' || value == '\v' || value == '\r' {
				return true
			}
		}
		return false
	}
	for index := 0; index < len(raw); index++ {
		value := raw[index]
		if nativePrawnLineWrapByteBreak(value, 0xad) || value == '\n' || value == '\v' || value == '\r' {
			return true
		}
	}
	return false
}

func nativePrawnLineWrapZeroWidthSpace(value *object.EmeraldValue) bool {
	if value == nil || !nativePrawnUTF8Encoding(value.Encoding) {
		return false
	}
	raw, ok := value.Data.(string)
	return ok && raw == "\u200b"
}

func nativePrawnLineWrapTrailingSoftHyphen(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	raw, ok := value.Data.(string)
	if !ok {
		return false
	}
	if nativePrawnUTF8Encoding(value.Encoding) {
		return strings.HasSuffix(raw, "\u00ad")
	}
	return len(raw) > 0 && raw[len(raw)-1] == 0xad && value.Encoding == "Windows-1252"
}

func nativePrawnLineWrapSoftHyphenValue(encoding string) *object.EmeraldValue {
	value := core.NewStringValue("\u00ad")
	if nativePrawnUTF8Encoding(encoding) {
		core.SetStringEncoding(value, "UTF-8")
		return value
	}
	if encoding == "Windows-1252" || encoding == "CP1252" {
		value = core.NewStringValue(string([]byte{0xad}))
		core.SetStringEncoding(value, "Windows-1252")
		return value
	}
	return nil
}

func (vm *VM) nativePrawnLineWrapTokenizeSource(class *object.Class) bool {
	return vm != nil && class != nil && nativeAFMMethodSource(class, "scan_pattern", "/prawn/text/formatted/line_wrap.rb") &&
		vm.nativePrawnLineWrapConstantsAvailable()
}

func nativePrawnLineWrapTokenize(value *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if value == nil || value.Type != object.ValueString || value.Class != core.R.Classes["String"] ||
		core.AttachedSingletonClass(value) != nil {
		return nil, false
	}
	raw, ok := value.Data.(string)
	if !ok {
		return nil, false
	}
	encoding := value.Encoding
	if encoding == "" {
		encoding = "UTF-8"
	}
	var pieces []string
	switch encoding {
	case "UTF-8", "UTF_8", "UTF8":
		if !utf8.ValidString(raw) {
			return nil, false
		}
		pieces = nativePrawnLineWrapTokenizeUTF8(raw)
	case "Windows-1252", "CP1252":
		pieces = nativePrawnLineWrapTokenizeSingleByte(raw, 0xad)
	default:
		return nil, false
	}
	values := make([]*object.EmeraldValue, len(pieces))
	for index, piece := range pieces {
		values[index] = core.NewStringValue(piece)
		core.SetStringEncoding(values[index], encoding)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: core.R.Classes["Array"]}, true
}

func nativePrawnLineWrapTokenizeUTF8(raw string) []string {
	runes := []rune(raw)
	pieces := make([]string, 0, len(runes)/2+1)
	for start := 0; start < len(runes); {
		position := start
		if !nativePrawnLineWrapRuneBreak(runes[position]) {
			for position < len(runes) && !nativePrawnLineWrapRuneBreak(runes[position]) {
				position++
			}
			if position < len(runes) && runes[position] == '\u00ad' {
				position++
			} else if position < len(runes) && runes[position] == '-' {
				for position < len(runes) && runes[position] == '-' {
					position++
				}
			}
		} else if nativePrawnLineWrapRuneWhitespace(runes[position]) {
			for position < len(runes) && nativePrawnLineWrapRuneWhitespace(runes[position]) {
				position++
			}
		} else if runes[position] == '-' {
			for position < len(runes) && runes[position] == '-' {
				position++
			}
			for position < len(runes) && !nativePrawnLineWrapRuneBreak(runes[position]) {
				position++
			}
		} else {
			position++
		}
		pieces = append(pieces, string(runes[start:position]))
		start = position
	}
	return pieces
}

func nativePrawnLineWrapTokenizeSingleByte(raw string, softHyphen byte) []string {
	pieces := make([]string, 0, len(raw)/2+1)
	for start := 0; start < len(raw); {
		position := start
		if !nativePrawnLineWrapByteBreak(raw[position], softHyphen) {
			for position < len(raw) && !nativePrawnLineWrapByteBreak(raw[position], softHyphen) {
				position++
			}
			if position < len(raw) && raw[position] == softHyphen {
				position++
			} else if position < len(raw) && raw[position] == '-' {
				for position < len(raw) && raw[position] == '-' {
					position++
				}
			}
		} else if nativePrawnLineWrapByteWhitespace(raw[position]) {
			for position < len(raw) && nativePrawnLineWrapByteWhitespace(raw[position]) {
				position++
			}
		} else if raw[position] == '-' {
			for position < len(raw) && raw[position] == '-' {
				position++
			}
			for position < len(raw) && !nativePrawnLineWrapByteBreak(raw[position], softHyphen) {
				position++
			}
		} else {
			position++
		}
		pieces = append(pieces, raw[start:position])
		start = position
	}
	return pieces
}

func nativePrawnLineWrapRuneWhitespace(value rune) bool {
	return value == ' ' || value == '\t' || value == '\u200b'
}

func nativePrawnLineWrapRuneBreak(value rune) bool {
	return nativePrawnLineWrapRuneWhitespace(value) || value == '\u00ad' || value == '-'
}

func nativePrawnLineWrapByteWhitespace(value byte) bool {
	return value == ' ' || value == '\t'
}

func nativePrawnLineWrapByteBreak(value, softHyphen byte) bool {
	return nativePrawnLineWrapByteWhitespace(value) || value == softHyphen || value == '-'
}

// executeNativePrawnFormattedProcessText handles the common, immutable text
// normalization path in Formatted::Fragment#process_text.  The Ruby method
// is a small dispatcher around String#gsub/rstrip/reverse, but it is reached
// for every fragment.  This intrinsic only admits the original Prawn method,
// an ordinary Fragment/Hash/String graph, and the exact UTF-8/soft-hyphen
// constants.  Any format state or encoding outside that closed shape falls
// back to Ruby so monkey patches and unusual encodings keep their semantics.
func (vm *VM) executeNativePrawnFormattedProcessText(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnTextEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Text::Formatted::Fragment" ||
		core.AttachedSingletonClass(receiver) != nil || len(args) != 1 || !nativeAFMStringArgument(args[0]) ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.instructionLimit != 0 || DevMode ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive {
		return nil, false
	}
	proof, ok := vm.nativePrawnFragmentProcessTextProofFor(methodObj, receiver.Class)
	if !ok || proof == nil {
		return nil, false
	}
	text := args[0]
	raw, rawOK := text.Data.(string)
	if !rawOK {
		return nil, false
	}
	formatState := core.DynamicInstanceVar(receiver, "@format_state")
	if formatState == nil || formatState.Type != object.ValueHash || formatState.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(formatState) != nil {
		return nil, false
	}
	hash, hashOK := formatState.Data.(*object.RHash)
	if !hashOK || hash == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity {
		return nil, false
	}

	excludeValue, _ := nativePrawnFormatStateField(hash, "exclude_trailing_white_space")
	excludeTrailing := nativePrawnFormattedTruthy(excludeValue)
	normalized, normalizedFound := nativePrawnFormatStateField(hash, "normalized_soft_hyphen")
	normalizedRaw := ""
	if normalizedFound && nativePrawnFormattedTruthy(normalized) {
		var normalizedOK bool
		normalizedRaw, normalizedOK = nativePrawnSoftHyphenPattern(normalized)
		if !normalizedOK || !nativePrawnSameEncoding(text, normalized) || raw == "" {
			return nil, false
		}
	}

	direction, directionFound := nativePrawnFormatStateField(hash, "direction")
	reverse := false
	if directionFound && direction != nil && direction.Type != object.ValueNil {
		if direction.Type != object.ValueSymbol || direction.Class != core.R.Classes["Symbol"] ||
			core.AttachedSingletonClass(direction) != nil {
			return nil, false
		}
		name, nameOK := direction.Data.(string)
		if !nameOK || name != "ltr" && name != "rtl" {
			return nil, false
		}
		reverse = name == "rtl"
	}

	if nativePrawnUTF8Encoding(text.Encoding) {
		if !utf8.ValidString(raw) {
			return nil, false
		}
		if proof.zwsp == nil || raw != "" {
			if proof.zwsp == nil {
				return nil, false
			}
			zwsp, zwspOK := proof.zwsp.Data.(string)
			if !zwspOK || zwsp != "\u200b" {
				return nil, false
			}
			raw = strings.ReplaceAll(raw, zwsp, "")
		}
	}

	if excludeTrailing {
		raw = nativePrawnRstrip(raw)
	}
	if normalizedFound && nativePrawnFormattedTruthy(normalized) && raw != "" {
		if excludeTrailing {
			prefix, suffix, sliceOK := nativePrawnDropLastChar(raw, text.Encoding)
			if !sliceOK {
				return nil, false
			}
			raw = strings.ReplaceAll(prefix, normalizedRaw, "") + suffix
		} else {
			raw = strings.ReplaceAll(raw, normalizedRaw, "")
		}
	}
	if reverse {
		raw = nativePrawnReverse(raw)
	}
	result := core.NewStringValue(raw)
	if text.Encoding != "" {
		core.SetStringEncoding(result, text.Encoding)
	}
	return result, true
}

func (vm *VM) nativePrawnFragmentProcessTextProofFor(methodObj *object.Method, fragmentClass *object.Class) (*nativePrawnFragmentProcessTextProof, bool) {
	if vm == nil || methodObj == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnFragmentProcessTextGeneration != generation {
		vm.nativePrawnFragmentProcessTextGeneration = generation
		if vm.nativePrawnFragmentProcessTextProofs == nil {
			vm.nativePrawnFragmentProcessTextProofs = make(map[*object.Method]nativePrawnFragmentProcessTextProof)
		} else {
			for cachedMethod := range vm.nativePrawnFragmentProcessTextProofs {
				delete(vm.nativePrawnFragmentProcessTextProofs, cachedMethod)
			}
		}
	}
	if vm.nativePrawnFragmentProcessTextProofs == nil {
		vm.nativePrawnFragmentProcessTextProofs = make(map[*object.Method]nativePrawnFragmentProcessTextProof)
	}
	if cached, found := vm.nativePrawnFragmentProcessTextProofs[methodObj]; found && cached.generation == generation {
		if !cached.safe {
			return nil, false
		}
		return &cached, true
	}
	proof := nativePrawnFragmentProcessTextProof{generation: generation}
	fn, fnOK := methodObj.Fn.(*object.Function)
	proof.safe = fnOK && fn != nil && fn.Name == "process_text" && fragmentClass != nil &&
		strings.HasSuffix(fn.SourcePath, "/prawn/text/formatted/fragment.rb") &&
		len(fn.Params) == 1 && len(fn.ParamDefaults) == 1 && fn.ParamDefaults[0] == nil && !fn.HasRestParam && !fn.HasBlockParam &&
		len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "" && !fn.KeywordRestOnly &&
		nativeAFMMethodSource(fragmentClass, "strip_zero_width_spaces", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "exclude_trailing_white_space?", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "soft_hyphens_need_processing?", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "normalized_soft_hyphen", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "process_soft_hyphens", "/prawn/text/formatted/fragment.rb") &&
		nativeAFMMethodSource(fragmentClass, "direction", "/prawn/text/formatted/fragment.rb") &&
		core.HashIndexUsesBuiltinImplementation() && core.SymbolEqualUsesBuiltinImplementation() &&
		nativePrawnStringMethodIsBuiltin("gsub") && nativePrawnStringMethodIsBuiltin("rstrip") &&
		nativePrawnStringMethodIsBuiltin("reverse") && nativePrawnStringMethodIsBuiltin("encoding") &&
		nativePrawnStringMethodIsBuiltin("empty?")
	if proof.safe {
		proof.safe = vm.nativePrawnLineWrapConstantsAvailable()
		if proof.safe {
			proof.zwsp, _ = vm.qualifiedConstantValue("Prawn::Text::ZWSP")
			proof.safe = proof.zwsp != nil && proof.zwsp.Type == object.ValueString &&
				proof.zwsp.Class == core.R.Classes["String"] && core.AttachedSingletonClass(proof.zwsp) == nil
		}
	}
	vm.nativePrawnFragmentProcessTextProofs[methodObj] = proof
	if !proof.safe {
		return nil, false
	}
	copy := proof
	return &copy, true
}

func nativePrawnFormattedTruthy(value *object.EmeraldValue) bool {
	return value != nil && value.Type != object.ValueNil && value != core.R.FalseVal
}

func nativePrawnUTF8Encoding(encoding string) bool {
	return encoding == "" || encoding == "UTF-8" || encoding == "UTF_8" || encoding == "UTF8"
}

func nativePrawnSameEncoding(left, right *object.EmeraldValue) bool {
	if left == nil || right == nil {
		return false
	}
	leftEncoding, rightEncoding := left.Encoding, right.Encoding
	if leftEncoding == "" {
		leftEncoding = "UTF-8"
	}
	if rightEncoding == "" {
		rightEncoding = "UTF-8"
	}
	return leftEncoding == rightEncoding || nativePrawnUTF8Encoding(leftEncoding) && nativePrawnUTF8Encoding(rightEncoding)
}

func nativePrawnSoftHyphenPattern(value *object.EmeraldValue) (string, bool) {
	if !nativeAFMStringArgument(value) {
		return "", false
	}
	raw, ok := value.Data.(string)
	if !ok {
		return "", false
	}
	if raw == "\u00ad" || len(raw) == 1 && raw[0] == 0xad {
		return raw, true
	}
	return "", false
}

func nativePrawnDropLastChar(raw, encoding string) (string, string, bool) {
	if raw == "" {
		return "", "", false
	}
	if !nativePrawnUTF8Encoding(encoding) {
		return raw[:len(raw)-1], raw[len(raw)-1:], true
	}
	_, width := utf8.DecodeLastRuneInString(raw)
	if width <= 0 || width > len(raw) {
		return "", "", false
	}
	return raw[:len(raw)-width], raw[len(raw)-width:], true
}

func nativePrawnRstrip(raw string) string {
	end := len(raw)
	for end > 0 && nativePrawnStripByte(raw[end-1]) {
		end--
	}
	return raw[:end]
}

func nativePrawnStripByte(value byte) bool {
	return value == 0 || value == ' ' || value >= '\t' && value <= '\r'
}

func nativePrawnReverse(raw string) string {
	parts := make([]string, 0, len(raw))
	for position := 0; position < len(raw); {
		_, width := utf8.DecodeRuneInString(raw[position:])
		if width <= 0 {
			width = 1
		}
		parts = append(parts, raw[position:position+width])
		position += width
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	for index := len(parts) - 1; index >= 0; index-- {
		builder.WriteString(parts[index])
	}
	return builder.String()
}

func (vm *VM) nativePrawnLineWrapMethodProofFor(methodObj *object.Method) (string, *object.EmeraldValue, *object.EmeraldValue, bool) {
	if vm == nil || methodObj == nil {
		return "", nil, nil, false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnLineWrapProofGeneration != generation {
		vm.nativePrawnLineWrapProofGeneration = generation
		vm.nativePrawnLineWrapConstantsChecked = false
		if vm.nativePrawnLineWrapProofs == nil {
			vm.nativePrawnLineWrapProofs = make(map[*object.Method]nativePrawnLineWrapMethodProof)
		} else {
			for cachedMethod := range vm.nativePrawnLineWrapProofs {
				delete(vm.nativePrawnLineWrapProofs, cachedMethod)
			}
		}
	}
	if vm.nativePrawnLineWrapProofs == nil {
		vm.nativePrawnLineWrapProofs = make(map[*object.Method]nativePrawnLineWrapMethodProof)
	}
	if proof, found := vm.nativePrawnLineWrapProofs[methodObj]; found && proof.generation == generation {
		return proof.name, proof.utf8, proof.windows1252, proof.safe
	}
	proof := nativePrawnLineWrapMethodProof{generation: generation}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || !strings.HasSuffix(fn.SourcePath, "/prawn/text/formatted/line_wrap.rb") ||
		len(fn.Params) != 1 || len(fn.ParamDefaults) > 1 || len(fn.ParamDefaults) == 1 && fn.ParamDefaults[0] != nil || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	switch fn.Name {
	case "soft_hyphen", "zero_width_space", "whitespace", "break_chars", "hyphen", "tokenize", "add_fragment_to_line", "fragment_finished":
		proof.name = fn.Name
	default:
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	encodingClass := core.R.Classes["Encoding"]
	if encodingClass == nil {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	utf8, found := encodingClass.GetConstant("UTF_8")
	if !found || utf8 == nil {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	stringClass := core.R.Classes["String"]
	if stringClass == nil {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	encodeMethod, _, found := stringClass.GetMethodWithOwner("encode")
	if !found || encodeMethod == nil || encodeMethod.DispatchOwner != nil ||
		(encodeMethod.Visibility != "" && encodeMethod.Visibility != "public") {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	if _, builtin := encodeMethod.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); !builtin ||
		!vm.nativePrawnLineWrapConstantsAvailable() ||
		proof.name == "tokenize" && !nativePrawnStringMethodIsBuiltin("scan") {
		vm.nativePrawnLineWrapProofs[methodObj] = proof
		return "", nil, nil, false
	}
	proof.utf8 = utf8
	proof.windows1252, _ = encodingClass.GetConstant("Windows_1252")
	proof.safe = true
	vm.nativePrawnLineWrapProofs[methodObj] = proof
	return proof.name, proof.utf8, proof.windows1252, true
}

func nativePrawnStringMethodIsBuiltin(name string) bool {
	stringClass := core.R.Classes["String"]
	if stringClass == nil {
		return false
	}
	method, _, found := stringClass.GetMethodWithOwner(name)
	if !found || method == nil || method.DispatchOwner != nil || method.Visibility == "undefined" {
		return false
	}
	_, ok := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)
	return ok
}

func (vm *VM) nativePrawnLineWrapConstantsAvailable() bool {
	if vm == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnLineWrapConstantsChecked && vm.nativePrawnLineWrapProofGeneration == generation {
		return vm.nativePrawnLineWrapConstantsOK
	}
	vm.nativePrawnLineWrapConstantsChecked = true
	vm.nativePrawnLineWrapProofGeneration = generation
	for name, expected := range map[string]string{
		"Prawn::Text::SHY":  "\u00ad",
		"Prawn::Text::ZWSP": "\u200b",
	} {
		value, found := vm.qualifiedConstantValue(name)
		if !found || value == nil || value.Type != object.ValueString || value.Class != core.R.Classes["String"] ||
			core.AttachedSingletonClass(value) != nil {
			vm.nativePrawnLineWrapConstantsOK = false
			return false
		}
		raw, ok := value.Data.(string)
		if !ok || raw != expected || value.Encoding != "UTF-8" && value.Encoding != "" {
			vm.nativePrawnLineWrapConstantsOK = false
			return false
		}
	}
	vm.nativePrawnLineWrapConstantsOK = true
	return true
}

func nativePrawnFormatStateField(hash *object.RHash, name string) (*object.EmeraldValue, bool) {
	if hash == nil {
		return nil, false
	}
	for _, key := range hash.Keys {
		if key == nil || key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] ||
			core.AttachedSingletonClass(key) != nil {
			return nil, false
		}
		keyName, ok := key.Data.(string)
		if !ok {
			return nil, false
		}
		if keyName == name {
			value, found := hash.Pairs[key]
			return value, found
		}
	}
	return nil, true
}

func nativePrawnTextStateArgumentBuiltin(value *object.EmeraldValue) bool {
	if value == nil || core.AttachedSingletonClass(value) != nil {
		return false
	}
	if value.Type == object.ValueInteger {
		return value.Class == core.R.Classes["Integer"] && value.BigIntValue() == nil
	}
	return value.Type == object.ValueFloat && value.Class == core.R.Classes["Float"]
}

func nativePrawnTextStateEqual(current, argument *object.EmeraldValue, defaultInteger int64) bool {
	if argument == nil || !nativePrawnTextStateArgumentBuiltin(argument) {
		return false
	}
	if current == nil || current.Type == object.ValueNil || current == core.R.FalseVal {
		value, ok := argument.Data.(int64)
		return argument.Type == object.ValueInteger && ok && value == defaultInteger
	}
	if !nativePrawnTextStateArgumentBuiltin(current) || current.Type != argument.Type {
		return false
	}
	switch current.Type {
	case object.ValueInteger:
		left, leftOK := current.Data.(int64)
		right, rightOK := argument.Data.(int64)
		return leftOK && rightOK && left == right
	case object.ValueFloat:
		left, leftOK := current.Data.(float64)
		right, rightOK := argument.Data.(float64)
		return leftOK && rightOK && left == right
	default:
		return false
	}
}

// executeNativePrawnFontMetricWidth collapses the stable AFM branch of
// Prawn::Document#width_of into one boxed result. It is enabled by default,
// but has an independent compatibility switch because bypassing Prawn's
// private font metric cache is only safe after the exact Prawn method graph
// and the narrow AFM option shape have been proven.
func (vm *VM) executeNativePrawnFontMetricWidth(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnFontMetricEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(receiver) != nil || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.instructionLimit != 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive || len(args) != 2 {
		return nil, false
	}
	kerning, optionsOK := nativePrawnFontMetricOptions(args[1])
	if !optionsOK {
		return nil, false
	}
	stringValue := args[0]
	if vm == nil || !nativePrawnFontMetricEnabled || methodObj == nil || methodObj.DispatchOwner != nil || receiver == nil ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(receiver) != nil || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		vm.instructionLimit != 0 || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 || vm.ensureActive ||
		!nativeAFMStringArgument(stringValue) || !nativePrawnASCIIString(stringValue) {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "width_of" || !strings.HasSuffix(fn.SourcePath, "/prawn/font.rb") {
		return nil, false
	}
	document := receiver
	font := core.DynamicInstanceVar(document, "@font")
	if font == nil || font.Type != object.ValueObject || font.Class == nil || font.Class.Name != "Prawn::Fonts::AFM" ||
		core.AttachedSingletonClass(font) != nil {
		return nil, false
	}
	if core.DynamicInstanceVar(font, "@document") != document {
		return nil, false
	}
	if !vm.nativePrawnFontMetricDependenciesAvailable(document.Class, font.Class) {
		return nil, false
	}
	sizeValue, ok := nativeAFMDocumentSize(font)
	if !ok {
		return nil, false
	}
	size, ok := nativeAFMNumber(sizeValue)
	if !ok {
		return nil, false
	}
	total, ok := vm.nativeAFMUnscaledTotal(font, stringValue)
	if !ok {
		return nil, false
	}
	if kerning {
		pairs, pairsOK := vm.nativeAFMKernPairs(font)
		if !pairsOK {
			return nil, false
		}
		raw, rawOK := stringValue.Data.(string)
		if !rawOK {
			return nil, false
		}
		for index := 1; index < len(raw); index++ {
			amount, found := pairs[[2]byte{raw[index-1], raw[index]}]
			if !found {
				continue
			}
			total, ok = nativeAFMAddInt64(total, amount)
			if !ok {
				return nil, false
			}
		}
	}
	length := float64(total) * (size / 1000.0)
	spacingValue := core.DynamicInstanceVar(document, "@character_spacing")
	raw, rawOK := stringValue.Data.(string)
	if !rawOK {
		return nil, false
	}
	if spacingValue != nil && len(raw) > 0 {
		spacing, spacingOK := nativeAFMNumber(spacingValue)
		if !spacingOK {
			return nil, false
		}
		length += spacing * float64(len(raw)-1)
	}
	return core.NewFloatValue(length), true
}

func nativePrawnASCIIString(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	raw, ok := value.Data.(string)
	if !ok {
		return false
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] >= 0x80 {
			return false
		}
	}
	return true
}

func nativePrawnFontMetricOptions(value *object.EmeraldValue) (bool, bool) {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(value) != nil {
		return false, false
	}
	hash, ok := value.Data.(*object.RHash)
	if !ok || hash == nil || hash.Default != nil || hash.DefaultProc != nil || hash.CompareByIdentity || len(hash.Keys) != len(hash.Pairs) {
		return false, false
	}
	kerning := false
	for _, key := range hash.Keys {
		if key == nil || key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] || core.AttachedSingletonClass(key) != nil {
			return false, false
		}
		name, ok := key.Data.(string)
		if !ok {
			return false, false
		}
		value := hash.Pairs[key]
		switch name {
		case "kerning":
			if value != nil && value != core.R.NilVal && value != core.R.TrueVal && value != core.R.FalseVal {
				return false, false
			}
			kerning = value != nil && value.IsTruthy()
		default:
			return false, false
		}
	}
	return kerning, true
}

func (vm *VM) nativePrawnFontMetricDependenciesAvailable(documentClass, fontClass *object.Class) bool {
	if vm == nil || documentClass == nil || fontClass == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnFontMetricDependenciesChecked && vm.nativePrawnFontMetricDependenciesGeneration == generation {
		return vm.nativePrawnFontMetricDependenciesOK
	}
	vm.nativePrawnFontMetricDependenciesGeneration = generation
	vm.nativePrawnFontMetricDependenciesChecked = true
	vm.nativePrawnFontMetricDependenciesOK =
		nativeAFMMethodSource(fontClass, "normalize_encoding", "/prawn/fonts/afm.rb") &&
			nativeAFMMethodSource(fontClass, "compute_width_of", "/prawn/fonts/afm.rb") &&
			nativeAFMMethodSource(fontClass, "unscaled_width_of", "/prawn/fonts/afm.rb") &&
			nativeAFMMethodSource(fontClass, "kern", "/prawn/fonts/afm.rb") &&
			nativeAFMMethodSource(fontClass, "character_count", "/prawn/fonts/afm.rb") &&
			nativeAFMMethodSource(fontClass, "size", "/prawn/font.rb") &&
			nativeAFMMethodSource(documentClass, "font_size", "/prawn/font.rb") &&
			nativeAFMMethodSource(documentClass, "character_spacing", "/pdf/core/text.rb")
	return vm.nativePrawnFontMetricDependenciesOK
}

func (vm *VM) nativePrawnAFMDependenciesAvailable(receiverClass *object.Class) bool {
	if vm == nil || receiverClass == nil {
		return false
	}
	generation := object.CurrentMethodGeneration()
	if vm.nativePrawnAFMDependenciesChecked && vm.nativePrawnAFMDependenciesGeneration == generation {
		return vm.nativePrawnAFMDependenciesOK
	}
	vm.nativePrawnAFMDependenciesGeneration = generation
	vm.nativePrawnAFMDependenciesChecked = true
	vm.nativePrawnAFMDependenciesOK = false
	documentClass := core.R.Classes["Prawn::Document"]
	if documentClass == nil || !nativeAFMMethodSource(receiverClass, "unscaled_width_of", "/prawn/fonts/afm.rb") ||
		!nativeAFMMethodSource(receiverClass, "kern", "/prawn/fonts/afm.rb") ||
		!nativeAFMMethodSource(documentClass, "font_size", "/prawn/font.rb") {
		return false
	}
	vm.nativePrawnAFMDependenciesOK = true
	return true
}

func nativeAFMStringArgument(value *object.EmeraldValue) bool {
	return value != nil && value.Type == object.ValueString && value.Class == core.R.Classes["String"] &&
		core.AttachedSingletonClass(value) == nil
}

func nativeAFMInteger(value *object.EmeraldValue) (int64, bool) {
	if value == nil || value.Type != object.ValueInteger || value.Class != core.R.Classes["Integer"] || value.BigIntValue() != nil {
		return 0, false
	}
	result, ok := value.Data.(int64)
	return result, ok
}

func nativeAFMArrayElements(value *object.EmeraldValue) ([]*object.EmeraldValue, bool) {
	if value == nil || value.Type != object.ValueArray || value.Class != core.R.Classes["Array"] ||
		core.AttachedSingletonClass(value) != nil {
		return nil, false
	}
	return nativePDFArrayItems(value)
}

func (vm *VM) nativeAFMGlyphTable(receiver *object.EmeraldValue) ([]int64, bool) {
	table := core.DynamicInstanceVar(receiver, "@glyph_table")
	if table == nil || table.Type != object.ValueArray || table.Class != core.R.Classes["Array"] || !table.Frozen ||
		core.AttachedSingletonClass(table) != nil {
		return nil, false
	}
	if vm.nativeAFMGlyphTables != nil {
		if cached, ok := vm.nativeAFMGlyphTables[table]; ok {
			return cached, true
		}
	}
	items, ok := nativeAFMArrayElements(table)
	if !ok || len(items) != 256 {
		return nil, false
	}
	glyphs := make([]int64, len(items))
	for index, item := range items {
		value, valid := nativeAFMInteger(item)
		if !valid || value < 0 {
			return nil, false
		}
		glyphs[index] = value
	}
	if vm.nativeAFMGlyphTables == nil {
		vm.nativeAFMGlyphTables = make(map[*object.EmeraldValue][]int64)
	}
	vm.nativeAFMGlyphTables[table] = glyphs
	return glyphs, true
}

func (vm *VM) nativeAFMUnscaledTotal(receiver, stringValue *object.EmeraldValue) (int64, bool) {
	glyphs, ok := vm.nativeAFMGlyphTable(receiver)
	if !ok {
		return 0, false
	}
	raw, ok := stringValue.Data.(string)
	if !ok {
		return 0, false
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	var total int64
	for index := 0; index < len(raw); index++ {
		width := glyphs[raw[index]]
		if width > maxInt64-total {
			return 0, false
		}
		total += width
	}
	return total, true
}

func (vm *VM) executeNativePrawnAFMUnscaledWidth(receiver, stringValue *object.EmeraldValue) (*object.EmeraldValue, bool) {
	total, ok := vm.nativeAFMUnscaledTotal(receiver, stringValue)
	if !ok {
		return nil, false
	}
	return core.NewIntegerValue(total), true
}

func nativeAFMMethodSource(class *object.Class, name, suffix string) bool {
	if class == nil {
		return false
	}
	method, _, found := class.GetMethodWithOwner(name)
	if !found || method == nil || method.DispatchOwner != nil {
		return false
	}
	fn, ok := method.Fn.(*object.Function)
	return ok && fn != nil && fn.Name == name && strings.HasSuffix(fn.SourcePath, suffix)
}

func nativeAFMNumber(value *object.EmeraldValue) (float64, bool) {
	if value == nil || core.AttachedSingletonClass(value) != nil {
		return 0, false
	}
	switch value.Type {
	case object.ValueInteger:
		integer, ok := nativeAFMInteger(value)
		return float64(integer), ok
	case object.ValueFloat:
		if value.Class != core.R.Classes["Float"] {
			return 0, false
		}
		result, ok := value.Data.(float64)
		return result, ok
	default:
		return 0, false
	}
}

func nativeAFMComputeOptions(value *object.EmeraldValue) (*object.EmeraldValue, bool, bool, bool) {
	if value == nil || value.Type != object.ValueHash || value.Class != core.R.Classes["Hash"] ||
		core.AttachedSingletonClass(value) != nil {
		return nil, false, false, false
	}
	data, ok := value.Data.(*object.RHash)
	if !ok || data == nil || data.Default != nil || data.DefaultProc != nil || data.CompareByIdentity ||
		len(data.Keys) != len(data.Pairs) {
		return nil, false, false, false
	}
	var sizeValue *object.EmeraldValue
	var kerningValue *object.EmeraldValue
	seenSize, seenKerning := false, false
	for _, key := range data.Keys {
		if key == nil {
			return nil, false, false, false
		}
		entry, exists := data.Pairs[key]
		if !exists {
			return nil, false, false, false
		}
		if key.Type != object.ValueSymbol || key.Class != core.R.Classes["Symbol"] {
			continue
		}
		name, ok := key.Data.(string)
		if !ok {
			return nil, false, false, false
		}
		switch name {
		case "size":
			if seenSize {
				return nil, false, false, false
			}
			seenSize = true
			sizeValue = entry
		case "kerning":
			if seenKerning {
				return nil, false, false, false
			}
			seenKerning = true
			kerningValue = entry
		}
	}
	kerning := kerningValue != nil && kerningValue.IsTruthy()
	return sizeValue, seenSize, kerning, true
}

func nativeAFMDocumentSize(receiver *object.EmeraldValue) (*object.EmeraldValue, bool) {
	document := core.DynamicInstanceVar(receiver, "@document")
	if document == nil || document.Type != object.ValueObject || document.Class == nil || document.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(document) != nil {
		return nil, false
	}
	return core.DynamicInstanceVar(document, "@font_size"), true
}

func nativeAFMAddInt64(left, right int64) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if right > 0 && left > maxInt64-right || right < 0 && left < minInt64-right {
		return 0, false
	}
	return left + right, true
}

func (vm *VM) executeNativePrawnAFMComputeWidth(receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if len(args) < 1 || len(args) > 2 || !nativeAFMStringArgument(args[0]) {
		return nil, false
	}
	var options *object.EmeraldValue
	if len(args) == 2 {
		options = args[1]
	}
	var sizeValue *object.EmeraldValue
	sizeProvided := false
	kerning := false
	if options != nil {
		var ok bool
		sizeValue, sizeProvided, kerning, ok = nativeAFMComputeOptions(options)
		if !ok {
			return nil, false
		}
	}
	if !sizeProvided || sizeValue == nil || !sizeValue.IsTruthy() {
		var ok bool
		sizeValue, ok = nativeAFMDocumentSize(receiver)
		if !ok {
			return nil, false
		}
	}
	size, ok := nativeAFMNumber(sizeValue)
	if !ok {
		return nil, false
	}
	if kerning {
		_, kernBuiltinsOK, _ := vm.nativePrawnTextBuiltinsAvailable()
		if !kernBuiltinsOK {
			return nil, false
		}
	}
	total, ok := vm.nativeAFMUnscaledTotal(receiver, args[0])
	if !ok {
		return nil, false
	}
	if kerning {
		pairs, valid := vm.nativeAFMKernPairs(receiver)
		if !valid {
			return nil, false
		}
		raw, rawOK := args[0].Data.(string)
		if !rawOK {
			return nil, false
		}
		for index := 1; index < len(raw); index++ {
			if amount, found := pairs[[2]byte{raw[index-1], raw[index]}]; found {
				total, ok = nativeAFMAddInt64(total, amount)
				if !ok {
					return nil, false
				}
			}
		}
	}
	scale := size / 1000.0
	return core.NewFloatValue(float64(total) * scale), true
}

func (vm *VM) nativeAFMKernPairs(receiver *object.EmeraldValue) (map[[2]byte]int64, bool) {
	tableValue := core.DynamicInstanceVar(receiver, "@kern_pair_table")
	if tableValue == nil || tableValue.Type != object.ValueHash || tableValue.Class != core.R.Classes["Hash"] ||
		!tableValue.Frozen || core.AttachedSingletonClass(tableValue) != nil {
		return nil, false
	}
	table, ok := tableValue.Data.(*object.RHash)
	if !ok || table == nil || table.Linear != nil || table.Pairs == nil || table.Default != nil ||
		table.DefaultProc != nil || table.CompareByIdentity || len(table.Keys) != len(table.Pairs) {
		return nil, false
	}
	if vm.nativeAFMKernTables != nil {
		if cached, ok := vm.nativeAFMKernTables[table]; ok {
			return cached.pairs, true
		}
	}
	pairs := make(map[[2]byte]int64, len(table.Pairs))
	seenKeys := make(map[*object.EmeraldValue]struct{}, len(table.Keys))
	for _, key := range table.Keys {
		if key == nil {
			return nil, false
		}
		if _, duplicate := seenKeys[key]; duplicate {
			return nil, false
		}
		seenKeys[key] = struct{}{}
		value, exists := table.Pairs[key]
		if !exists {
			return nil, false
		}
		items, valid := nativeAFMArrayElements(key)
		if !valid || len(items) != 2 {
			return nil, false
		}
		// The WinAnsi table contains a few [code, nil] entries for glyphs
		// without a byte mapping. They can never equal the two Integer byte
		// operands used by AFM#kern, so Ruby's Hash#[] treats them as inert.
		if items[0] == nil || items[1] == nil || items[0].Type == object.ValueNil || items[1].Type == object.ValueNil {
			continue
		}
		left, validLeft := nativeAFMInteger(items[0])
		right, validRight := nativeAFMInteger(items[1])
		amount, validAmount := nativeAFMInteger(value)
		if !validLeft || !validRight || !validAmount || left < 0 || left > 255 || right < 0 || right > 255 ||
			amount == -1<<63 {
			return nil, false
		}
		pair := [2]byte{byte(left), byte(right)}
		if _, duplicate := pairs[pair]; duplicate {
			return nil, false
		}
		pairs[pair] = amount
	}
	if len(seenKeys) != len(table.Pairs) {
		return nil, false
	}
	if vm.nativeAFMKernTables == nil {
		vm.nativeAFMKernTables = make(map[*object.RHash]nativeAFMKernTable)
	}
	vm.nativeAFMKernTables[table] = nativeAFMKernTable{pairs: pairs}
	return pairs, true
}

func nativeAFMWindowsString(value []byte) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:     object.ValueString,
		Data:     string(value),
		Class:    core.R.Classes["String"],
		Encoding: "Windows-1252",
	}
}

func (vm *VM) executeNativePrawnAFMKern(receiver, stringValue *object.EmeraldValue) (*object.EmeraldValue, bool) {
	pairs, ok := vm.nativeAFMKernPairs(receiver)
	if !ok {
		return nil, false
	}
	raw, ok := stringValue.Data.(string)
	if !ok {
		return nil, false
	}
	result := make([]*object.EmeraldValue, 0, len(raw)+1)
	allocations := make([]*object.EmeraldValue, 0, len(raw)+2)
	chunk := make([]byte, 0, len(raw))
	var last byte
	haveLast := false
	appendChunk := func() {
		value := nativeAFMWindowsString(chunk)
		result = append(result, value)
		allocations = append(allocations, value)
	}
	for index := 0; index < len(raw); index++ {
		current := raw[index]
		if haveLast {
			if amount, found := pairs[[2]byte{last, current}]; found {
				appendChunk()
				result = append(result, core.NewIntegerValue(-amount))
				chunk = chunk[:0]
			}
		}
		chunk = append(chunk, current)
		last = current
		haveLast = true
	}
	appendChunk()
	arrayValue := &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: core.R.Classes["Array"]}
	allocations = append(allocations, arrayValue)
	core.TrackObjectSpaceValues(allocations)
	return arrayValue, true
}
