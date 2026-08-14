package vm

import (
	"strings"
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func TestNativePDFObjectTextPrimitiveAndCollectionShapes(t *testing.T) {
	core.InitWithMspec()
	integer := core.NewIntegerValue(42)
	stringValue := &object.EmeraldValue{Type: object.ValueString, Data: "hi", Class: core.R.Classes["String"]}
	symbol := &object.EmeraldValue{Type: object.ValueSymbol, Data: "A B", Class: core.R.Classes["Symbol"]}
	array := &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  []*object.EmeraldValue{integer, stringValue, symbol},
		Class: core.R.Classes["Array"],
	}
	key := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Value", Class: core.R.Classes["Symbol"]}
	hash := &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  &object.RHash{Pairs: map[*object.EmeraldValue]*object.EmeraldValue{key: array}, Keys: []*object.EmeraldValue{key}},
		Class: core.R.Classes["Hash"],
	}

	if got, ok := nativePDFObjectText(integer, false, make(map[*object.EmeraldValue]bool)); !ok || got != "42" {
		t.Fatalf("integer serialization: got %q, ok=%t", got, ok)
	}
	if got, ok := nativePDFObjectText(stringValue, false, make(map[*object.EmeraldValue]bool)); !ok || got != "<feff00680069>" {
		t.Fatalf("string serialization: got %q, ok=%t", got, ok)
	}
	if got, ok := nativePDFObjectText(stringValue, true, make(map[*object.EmeraldValue]bool)); !ok || got != "<6869>" {
		t.Fatalf("content stream serialization: got %q, ok=%t", got, ok)
	}
	if got, ok := nativePDFObjectText(symbol, false, make(map[*object.EmeraldValue]bool)); !ok || got != "/A#20B" {
		t.Fatalf("symbol serialization: got %q, ok=%t", got, ok)
	}
	if got, ok := nativePDFObjectText(array, false, make(map[*object.EmeraldValue]bool)); !ok || got != `[42 <feff00680069> /A#20B]` {
		t.Fatalf("array serialization: got %q, ok=%t", got, ok)
	}
	if got, ok := nativePDFObjectText(hash, false, make(map[*object.EmeraldValue]bool)); !ok || got != "<< /Value [42 <feff00680069> /A#20B]\n>>" {
		t.Fatalf("hash serialization: got %q, ok=%t", got, ok)
	}
}

func TestNativePDFRenderWriterMatchesObjectSerializer(t *testing.T) {
	core.InitWithMspec()
	key := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Value", Class: core.R.Classes["Symbol"]}
	value := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
		core.NewIntegerValue(42),
		&object.EmeraldValue{Type: object.ValueString, Data: "hi", Class: core.R.Classes["String"]},
	}, Class: core.R.Classes["Array"]}
	hash := nativePDFHashValue([2]*object.EmeraldValue{key, value})
	want, ok := nativePDFObjectText(hash, false, make(map[*object.EmeraldValue]bool))
	if !ok {
		t.Fatal("generic serializer rejected test hash")
	}
	var output strings.Builder
	if !nativePDFRenderWriteObjectText(&output, hash, false, make(map[*object.EmeraldValue]bool), nil) {
		t.Fatal("renderer serializer rejected test hash")
	}
	if got := output.String(); got != want {
		t.Fatalf("renderer serializer = %q, generic serializer = %q", got, want)
	}
}

func TestNativePDFObjectTextRejectsUnsupportedStringSubclassAndCycles(t *testing.T) {
	core.InitWithMspec()
	subclass := object.NewClass("PDF::CustomString")
	value := &object.EmeraldValue{Type: object.ValueString, Data: "x", Class: subclass}
	if _, ok := nativePDFObjectText(value, false, make(map[*object.EmeraldValue]bool)); ok {
		t.Fatal("expected String subclass to deopt")
	}
	cycle := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.Classes["Array"]}
	cycle.Data = []*object.EmeraldValue{cycle}
	if _, ok := nativePDFObjectText(cycle, false, make(map[*object.EmeraldValue]bool)); ok {
		t.Fatal("expected cyclic Array to deopt")
	}
}

func TestNativePDFRenderHashWithLengthUsesRubyDictionaryShape(t *testing.T) {
	core.InitWithMspec()
	text, ok := nativePDFRenderHashWithLength(nativePDFEmptyHash(), 7, nil)
	if !ok {
		t.Fatal("expected empty dictionary to accept stream length")
	}
	if want := "<< /Length 7\n>>"; text != want {
		t.Fatalf("stream dictionary = %q, want %q", text, want)
	}

	lengthKey := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Length", Class: core.R.Classes["Symbol"]}
	data := nativePDFHashValue([2]*object.EmeraldValue{lengthKey, core.NewIntegerValue(1)})
	if _, ok := nativePDFRenderHashWithLength(data, 7, nil); ok {
		t.Fatal("expected an existing Length key to deopt instead of duplicating Ruby Hash#merge")
	}
}
