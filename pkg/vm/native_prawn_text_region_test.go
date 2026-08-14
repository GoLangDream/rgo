package vm

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func TestNativePrawnTextLayoutIvarPreservesMapAndHotSidecarReads(t *testing.T) {
	core.InitWithMspec()
	class := object.NewClass("RgoTextLayoutObject")
	value := object.NewObjectValue(class)
	want := core.NewStringValue("map")
	core.SetDynamicInstanceVar(value, "@value", want)
	if got := nativePrawnTextLayoutIvar(value, "@value"); got != want {
		t.Fatalf("map ivar = %#v, want %#v", got, want)
	}

	data, ok := value.Data.(*object.Object)
	if !ok || data == nil {
		t.Fatalf("object payload = %#v", value.Data)
	}
	slot, ok := data.PrepareHotIntegerInstanceVar("@value")
	if !ok || !data.SetHotIntegerInstanceVar(slot, 42, core.R.Classes["Integer"]) {
		t.Fatal("failed to prepare hot integer sidecar")
	}
	got := nativePrawnTextLayoutIvar(value, "@value")
	if got == nil || got.Type != object.ValueInteger || got.Data != int64(42) {
		t.Fatalf("hot sidecar ivar = %#v, want Integer(42)", got)
	}
}
