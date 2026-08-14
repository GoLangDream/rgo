package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePrawnDefaultAFMTemplateEnabled = os.Getenv("RGO_DISABLE_NATIVE_PRAWN_FONT_TEMPLATE") == ""

// nativePrawnRememberDefaultAFMTemplate keeps one immutable copy of the
// standard Helvetica AFM payload. A new default document still receives a
// fresh Font object, identifier, references, and registry entry; only the
// parsed metric tables are shared. The first document remains the Ruby
// authority that populates this template.
func (vm *VM) nativePrawnRememberDefaultAFMTemplate(font *object.EmeraldValue) {
	if vm == nil || !nativePrawnDefaultAFMTemplateEnabled || font == nil || font.Type != object.ValueObject ||
		font.Class == nil || font.Class.Name != "Prawn::Fonts::AFM" || core.AttachedSingletonClass(font) != nil {
		return
	}
	name := core.DynamicInstanceVar(font, "@name")
	if name == nil || name.Type != object.ValueString || name.Class != core.R.Classes["String"] || name.Data != "Helvetica" ||
		core.AttachedSingletonClass(name) != nil {
		return
	}
	template := nativePrawnCopyObjectInstanceVars(font, font.Class)
	if template == nil {
		return
	}
	core.SetDynamicInstanceVar(template, "@document", core.R.NilVal)
	core.SetDynamicInstanceVar(template, "@identifier", nativePDFSymbol("F1"))
	core.SetDynamicInstanceVar(template, "@references", nativePDFEmptyHash())
	core.SetDynamicInstanceVar(template, "@subset_name_cache", nativePDFEmptyHash())
	vm.nativePrawnDefaultAFMTemplate = template
	vm.nativePrawnDefaultAFMTemplateClass = font.Class
	vm.nativePrawnDefaultAFMTemplateGeneration = object.CurrentMethodGeneration()
}

func (vm *VM) nativePrawnDefaultAFMFont(document *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnDefaultAFMTemplateEnabled || document == nil ||
		document.Type != object.ValueObject || document.Class == nil || document.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(document) != nil || vm.nativePrawnDefaultAFMTemplate == nil ||
		vm.nativePrawnDefaultAFMTemplateGeneration != object.CurrentMethodGeneration() {
		return nil, false
	}
	fontClassValue, found := vm.qualifiedConstantValue("Prawn::Fonts::AFM")
	if !found || fontClassValue == nil || fontClassValue.Type != object.ValueClass {
		return nil, false
	}
	fontClass, ok := fontClassValue.Data.(*object.Class)
	if !ok || fontClass == nil || fontClass != vm.nativePrawnDefaultAFMTemplateClass ||
		!nativeAFMMethodSource(document.Class, "font", "/prawn/font.rb") ||
		!nativeAFMMethodSource(document.Class, "find_font", "/prawn/font.rb") ||
		!nativeAFMMethodSource(document.Class, "font_registry", "/prawn/font.rb") ||
		!nativeAFMMethodSource(document.Class, "font_families", "/prawn/font.rb") ||
		!nativeAFMMethodSource(document.Class, "set_font", "/prawn/font.rb") {
		return nil, false
	}
	currentFont := core.DynamicInstanceVar(document, "@font")
	if currentFont != nil && currentFont.Type != object.ValueNil {
		return nil, false
	}
	fontRegistry := core.DynamicInstanceVar(document, "@font_registry")
	if fontRegistry != nil && fontRegistry.Type != object.ValueNil && !nativePrawnEmptyStandardHash(fontRegistry) {
		return nil, false
	}
	fontFamilies := core.DynamicInstanceVar(document, "@font_families")
	if fontFamilies != nil && fontFamilies.Type != object.ValueNil {
		return nil, false
	}
	state := core.DynamicInstanceVar(document, "@state")
	page := core.DynamicInstanceVar(state, "@page")
	if !nativePrawnDefaultFontPageRegistryEmpty(page, document, vm) {
		return nil, false
	}

	font := nativePrawnCopyObjectInstanceVars(vm.nativePrawnDefaultAFMTemplate, fontClass)
	if font == nil {
		return nil, false
	}
	core.SetDynamicInstanceVar(font, "@document", document)
	core.SetDynamicInstanceVar(font, "@identifier", nativePDFSymbol("F1"))
	core.SetDynamicInstanceVar(font, "@references", nativePDFEmptyHash())
	core.SetDynamicInstanceVar(font, "@subset_name_cache", nativePDFEmptyHash())
	core.TrackObjectSpaceValue(font)
	registryKey := core.NewStringValue("Helvetica:Helvetica:0")
	registry := nativePDFHashValue([2]*object.EmeraldValue{registryKey, font})
	if core.SetDynamicInstanceVar(document, "@font_registry", registry) != nil ||
		core.SetDynamicInstanceVar(document, "@font", font) != nil {
		return nil, false
	}
	return font, true
}

func nativePrawnCopyObjectInstanceVars(source *object.EmeraldValue, class *object.Class) *object.EmeraldValue {
	if source == nil || source.Type != object.ValueObject || class == nil || source.Class != class ||
		core.AttachedSingletonClass(source) != nil {
		return nil
	}
	sourceObject, ok := source.Data.(*object.Object)
	if !ok || sourceObject == nil {
		return nil
	}
	clone := object.NewObjectValue(class)
	cloneObject, ok := clone.Data.(*object.Object)
	if !ok || cloneObject == nil {
		return nil
	}
	for name, value := range sourceObject.InstanceVars {
		cloneObject.SetInstanceVar(name, value)
	}
	return clone
}

func nativePrawnEmptyStandardHash(value *object.EmeraldValue) bool {
	if !nativePDFStandardHash(value) {
		return false
	}
	entries, ok := nativePDFHashEntries(value)
	return ok && len(entries) == 0
}

func nativePrawnDefaultFontPageRegistryEmpty(page, document *object.EmeraldValue, vm *VM) bool {
	if vm == nil || page == nil || document == nil || !nativePDFExactObject(page, "PDF::Core::Page") ||
		core.DynamicInstanceVar(page, "@document") != document {
		return false
	}
	state := core.DynamicInstanceVar(document, "@state")
	store := core.DynamicInstanceVar(state, "@store")
	objects := core.DynamicInstanceVar(store, "@objects")
	dictionaryID := core.DynamicInstanceVar(page, "@dictionary")
	if !nativePDFExactObject(state, "PDF::Core::DocumentState") || store == nil ||
		store.Class != vm.nativePDFConstructorClass("PDF::Core::ObjectStore") || objects == nil ||
		!nativePDFStandardHash(objects) || dictionaryID == nil || dictionaryID.Type != object.ValueInteger {
		return false
	}
	dictionary, ok := core.DirectHashIndex(objects, dictionaryID)
	if !ok || !nativePDFExactObject(dictionary, "PDF::Core::Reference") {
		return false
	}
	dictionaryData := core.DynamicInstanceVar(dictionary, "@data")
	resources, resourcesFound := nativePDFLookupHashEntry(dictionaryData, "Resources")
	if !resourcesFound || resources == nil || resources.Type == object.ValueNil {
		return true
	}
	if !nativePDFStandardHash(resources) {
		return false
	}
	fonts, fontsFound := nativePDFLookupHashEntry(resources, "Font")
	return !fontsFound || fonts == nil || fonts.Type == object.ValueNil || nativePrawnEmptyStandardHash(fonts)
}
