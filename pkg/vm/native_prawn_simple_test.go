package vm

import (
	"strings"
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func TestNativePrawnSimplePDFIsSelfContained(t *testing.T) {
	core.InitWithMspec()
	state := &nativePrawnSimpleState{pages: [][]string{{nativePrawnSimpleContent(nil, "Hello")}, {nativePrawnSimpleContent(nil, "Page 2")}}}
	value := nativePrawnSimplePDF(state)
	if value == nil || value.Type != object.ValueString {
		t.Fatalf("expected PDF String value, got %#v", value)
	}
	pdf, ok := value.Data.(string)
	if !ok {
		t.Fatalf("expected string PDF payload, got %T", value.Data)
	}
	for _, fragment := range []string{"%PDF-1.3\n", "/Count 2", "<48656C6C6F>", "<506167652032>", "xref\n", "%%EOF\n"} {
		if !strings.Contains(pdf, fragment) {
			t.Fatalf("PDF missing %q: %q", fragment, pdf)
		}
	}
	if !strings.HasSuffix(pdf, "%%EOF\n") {
		t.Fatalf("PDF must end with EOF marker: %q", pdf[len(pdf)-24:])
	}
}

func TestNativePrawnSimpleASCIIGuard(t *testing.T) {
	if text, ok := nativePrawnSimpleASCII("Hello"); !ok || text != "Hello" {
		t.Fatalf("ASCII text guard rejected valid input: %q, %t", text, ok)
	}
	for _, text := range []string{"line\nfeed", "\u4f60"} {
		if _, ok := nativePrawnSimpleASCII(text); ok {
			t.Fatalf("ASCII text guard accepted %q", text)
		}
	}
}

func TestNativePrawnFontMetricOptionsKeepsOnlyBooleanKerning(t *testing.T) {
	core.InitWithMspec()
	makeHash := func(name string, value *object.EmeraldValue) *object.EmeraldValue {
		key := &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]}
		return &object.EmeraldValue{
			Type:  object.ValueHash,
			Data:  &object.RHash{Keys: []*object.EmeraldValue{key}, Pairs: map[*object.EmeraldValue]*object.EmeraldValue{key: value}},
			Class: core.R.Classes["Hash"],
		}
	}
	empty := &object.EmeraldValue{Type: object.ValueHash, Data: &object.RHash{Keys: []*object.EmeraldValue{}, Pairs: map[*object.EmeraldValue]*object.EmeraldValue{}}, Class: core.R.Classes["Hash"]}
	if kerning, ok := nativePrawnFontMetricOptions(empty); kerning || !ok {
		t.Fatalf("empty options = kerning %t, valid %t", kerning, ok)
	}
	if kerning, ok := nativePrawnFontMetricOptions(makeHash("kerning", core.R.TrueVal)); !kerning || !ok {
		t.Fatalf("true kerning options = kerning %t, valid %t", kerning, ok)
	}
	if kerning, ok := nativePrawnFontMetricOptions(makeHash("kerning", core.R.FalseVal)); kerning || !ok {
		t.Fatalf("false kerning options = kerning %t, valid %t", kerning, ok)
	}
	if _, ok := nativePrawnFontMetricOptions(makeHash("inline_format", core.R.FalseVal)); ok {
		t.Fatal("inline_format must remain on the Ruby path even when false")
	}
}
