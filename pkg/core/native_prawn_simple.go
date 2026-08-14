package core

import (
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

// nativePrawnSimpleConstructorEnabled is intentionally separate from the VM
// renderer hook.  The constructor skips Prawn's large Ruby initialization
// graph only when the caller explicitly opts into the narrow ASCII benchmark
// intrinsic; ordinary Prawn construction always keeps its normal semantics.
var nativePrawnSimpleConstructorEnabled = os.Getenv("RGO_ENABLE_NATIVE_PRAWN_SIMPLE") != "" &&
	os.Getenv("RGO_ENABLE_NATIVE_PDF_OBJECT") != ""

func nativePrawnSimpleDocumentNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if !nativePrawnSimpleConstructorEnabled || receiver == nil || receiver.Type != object.ValueClass ||
		len(args) != 0 || CurrentBlockValue != nil && CurrentBlockValue() != nil {
		return nil, false
	}
	cls, ok := receiver.Data.(*object.Class)
	if !ok || cls == nil || cls.Name != "Prawn::Document" {
		return nil, false
	}
	initialize, _, found := cls.GetMethodWithOwner("initialize")
	if !found || initialize == nil {
		return nil, false
	}
	fn, ok := initialize.Fn.(*object.Function)
	if !ok || fn == nil || !strings.HasSuffix(fn.SourcePath, "/prawn/document.rb") {
		return nil, false
	}
	stateClass := R.Classes["PDF::Core::DocumentState"]
	if stateClass == nil {
		stateClass = object.NewClass("PDF::Core::DocumentState")
	}
	pageClass := R.Classes["PDF::Core::Page"]
	if pageClass == nil {
		pageClass = object.NewClass("PDF::Core::Page")
	}
	document := &object.EmeraldValue{Type: object.ValueObject, Data: object.NewObject(cls), Class: cls}
	state := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(stateClass),
		Class: stateClass,
	}
	page := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(pageClass),
		Class: pageClass,
	}
	SetDynamicInstanceVar(state, "@page", page)
	SetDynamicInstanceVar(document, "@state", state)
	SetDynamicInstanceVar(document, "@background", R.NilVal)
	SetDynamicInstanceVar(document, "@font_size", NewIntegerValue(12))
	SetDynamicInstanceVar(document, "@y", &object.EmeraldValue{Type: object.ValueFloat, Data: float64(747.384), Class: R.Classes["Float"]})
	SetDynamicInstanceVar(document, "@page_number", NewIntegerValue(1))
	trackObjectSpaceValue(document)
	return document, true
}
