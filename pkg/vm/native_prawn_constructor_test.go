package vm

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func TestNativePrawnBoundingBoxConstructorStoresRubyFields(t *testing.T) {
	core.InitWithMspec()

	prawn := object.NewModule("Prawn")
	prawnValue := &object.EmeraldValue{Type: object.ValueModule, Data: prawn, Class: core.R.Classes["Module"]}
	document := object.NewClass("Prawn::Document")
	document.SuperClass = core.R.Classes["Object"]
	documentValue := &object.EmeraldValue{Type: object.ValueClass, Data: document, Class: core.R.Classes["Class"]}
	boxClass := object.NewClass("Prawn::Document::BoundingBox")
	boxClass.SuperClass = core.R.Classes["Object"]
	boxValue := &object.EmeraldValue{Type: object.ValueClass, Data: boxClass, Class: core.R.Classes["Class"]}
	document.DefineConstant("BoundingBox", boxValue)
	prawn.DefineConstant("Document", documentValue)
	vm := &VM{rubyConsts: map[string]*object.EmeraldValue{"Prawn": prawnValue, "Prawn::Document": documentValue}}
	boxClass.DefineMethod("initialize", &object.Method{
		Name: "initialize",
		Fn: &object.Function{
			Name:       "initialize",
			SourcePath: "/gems/prawn-2.5.0/lib/prawn/document/bounding_box.rb",
			Params:     []string{"document", "parent", "point", "options"},
		},
	})

	point := &object.EmeraldValue{
		Type:  object.ValueArray,
		Class: core.R.Classes["Array"],
		Data:  []*object.EmeraldValue{core.NewIntegerValue(10), core.NewIntegerValue(20)},
	}
	widthKey := &object.EmeraldValue{Type: object.ValueSymbol, Class: core.R.Classes["Symbol"], Data: "width"}
	heightKey := &object.EmeraldValue{Type: object.ValueSymbol, Class: core.R.Classes["Symbol"], Data: "height"}
	options := &object.EmeraldValue{
		Type:  object.ValueHash,
		Class: core.R.Classes["Hash"],
		Data: &object.RHash{
			Keys: []*object.EmeraldValue{widthKey, heightKey},
			Pairs: map[*object.EmeraldValue]*object.EmeraldValue{
				widthKey:  core.NewIntegerValue(300),
				heightKey: core.NewIntegerValue(400),
			},
		},
	}
	receiver := &object.EmeraldValue{Type: object.ValueClass, Data: boxClass, Class: core.R.Classes["Class"]}
	args := []*object.EmeraldValue{object.NewObjectValue(document), core.R.NilVal, point, options}
	result, handled := vm.executeNativePrawnClassNew(receiver, args...)
	if !handled || result == nil || result.Type != object.ValueObject {
		t.Fatalf("constructor handled=%t result=%#v", handled, result)
	}
	data, ok := result.Data.(*object.Object)
	if !ok || data == nil {
		t.Fatalf("constructor result data = %#v", result.Data)
	}
	checks := map[string]*object.EmeraldValue{
		"@document": args[0],
		"@parent":   args[1],
		"@x":        point.Data.([]*object.EmeraldValue)[0],
		"@y":        point.Data.([]*object.EmeraldValue)[1],
		"@width":    options.Data.(*object.RHash).Pairs[widthKey],
		"@height":   options.Data.(*object.RHash).Pairs[heightKey],
	}
	for name, want := range checks {
		if got := data.GetInstanceVar(name); got != want {
			t.Fatalf("%s = %#v, want %#v", name, got, want)
		}
	}
	if got := data.GetInstanceVar("@total_left_padding"); got == nil || got.BigIntValue() != nil || got.Data != int64(0) {
		t.Fatalf("@total_left_padding = %#v, want small integer zero", got)
	}
	if got := data.GetInstanceVar("@total_right_padding"); got == nil || got.BigIntValue() != nil || got.Data != int64(0) {
		t.Fatalf("@total_right_padding = %#v, want small integer zero", got)
	}
	if got := data.GetInstanceVar("@stretched_height"); got != core.R.NilVal {
		t.Fatalf("@stretched_height = %#v, want nil", got)
	}
}

func TestNativePrawnBoundingBoxConstructorRejectsMissingWidth(t *testing.T) {
	core.InitWithMspec()
	boxClass := object.NewClass("Prawn::Document::BoundingBox")
	boxClass.DefineMethod("initialize", &object.Method{
		Name: "initialize",
		Fn: &object.Function{
			Name:       "initialize",
			SourcePath: "/gems/prawn-2.5.0/lib/prawn/document/bounding_box.rb",
			Params:     []string{"document", "parent", "point", "options"},
		},
	})
	document := object.NewClass("Prawn::Document")
	prawn := object.NewModule("Prawn")
	prawn.DefineConstant("Document", &object.EmeraldValue{Type: object.ValueClass, Data: document, Class: core.R.Classes["Class"]})
	document.DefineConstant("BoundingBox", &object.EmeraldValue{Type: object.ValueClass, Data: boxClass, Class: core.R.Classes["Class"]})
	vm := &VM{rubyConsts: map[string]*object.EmeraldValue{
		"Prawn":           {Type: object.ValueModule, Data: prawn, Class: core.R.Classes["Module"]},
		"Prawn::Document": {Type: object.ValueClass, Data: document, Class: core.R.Classes["Class"]},
	}}
	receiver := &object.EmeraldValue{Type: object.ValueClass, Data: boxClass, Class: core.R.Classes["Class"]}
	point := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.Classes["Array"], Data: []*object.EmeraldValue{core.NewIntegerValue(0), core.NewIntegerValue(0)}}
	options := &object.EmeraldValue{Type: object.ValueHash, Class: core.R.Classes["Hash"], Data: &object.RHash{Keys: []*object.EmeraldValue{}, Pairs: map[*object.EmeraldValue]*object.EmeraldValue{}}}
	if result, handled := vm.executeNativePrawnClassNew(receiver, core.R.NilVal, core.R.NilVal, point, options); handled || result != nil {
		t.Fatalf("missing width handled=%t result=%#v, want fallback", handled, result)
	}
}
