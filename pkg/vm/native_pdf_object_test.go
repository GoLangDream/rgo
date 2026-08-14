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
	key := &object.EmeraldValue{Type: object.ValueSymbol, Data: "A B", Class: core.R.Classes["Symbol"]}
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
	var trustedOutput strings.Builder
	if !nativePDFRenderWriteObjectText(&trustedOutput, hash, false, nil, nil) {
		t.Fatal("trusted renderer serializer rejected test hash")
	}
	if got := trustedOutput.String(); got != want {
		t.Fatalf("trusted renderer serializer = %q, generic serializer = %q", got, want)
	}
}

func TestNativePDFRenderValuePlanKeepsStrictStringGuard(t *testing.T) {
	core.InitWithMspec()
	referenceClass := object.NewClass("PDF::Core::Reference")
	invalid := &object.EmeraldValue{Type: object.ValueString, Data: string([]byte{0xff}), Class: core.R.Classes["String"]}
	array := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{invalid}, Class: core.R.Classes["Array"]}
	plans := make(map[*object.EmeraldValue]nativePDFRenderValuePlan)
	if !nativePDFRenderValueShape(array, true, make(map[*object.EmeraldValue]bool), referenceClass, plans) {
		t.Fatal("content-stream shape should allow the byte string")
	}
	if nativePDFRenderValueShape(array, false, make(map[*object.EmeraldValue]bool), referenceClass, plans) {
		t.Fatal("cached content-stream proof must not bypass strict UTF-8 validation")
	}
}

func TestNativePDFRenderCompilesLargeCompositeWithContextVariants(t *testing.T) {
	core.InitWithMspec()
	referenceClass := object.NewClass("PDF::Core::Reference")
	items := make([]*object.EmeraldValue, nativePDFRenderCompileMinEntries)
	for index := range items {
		items[index] = &object.EmeraldValue{Type: object.ValueString, Data: "A", Class: core.R.Classes["String"]}
	}
	array := &object.EmeraldValue{Type: object.ValueArray, Data: items, Class: core.R.Classes["Array"]}
	plans := make(map[*object.EmeraldValue]nativePDFRenderValuePlan)
	if !nativePDFRenderValueShape(array, false, make(map[*object.EmeraldValue]bool), referenceClass, plans) {
		t.Fatal("large array should compile in ordinary object context")
	}
	ordinary := plans[array].serialized
	wantOrdinary := "[" + strings.Repeat("<feff0041> ", len(items)-1) + "<feff0041>]"
	if ordinary != wantOrdinary {
		t.Fatalf("ordinary compiled array = %q", ordinary)
	}
	if !nativePDFRenderValueShape(array, true, make(map[*object.EmeraldValue]bool), referenceClass, plans) {
		t.Fatal("large array should compile in content-stream context")
	}
	wantContent := "[" + strings.Repeat("<41> ", len(items)-1) + "<41>]"
	if got := plans[array].contentSerialized; got != wantContent {
		t.Fatalf("content compiled array = %q", got)
	}
	var output strings.Builder
	if !nativePDFRenderWriteObjectText(&output, array, true, make(map[*object.EmeraldValue]bool), plans) {
		t.Fatal("compiled content array should write")
	}
	if output.String() != plans[array].contentSerialized {
		t.Fatalf("compiled writer = %q, want %q", output.String(), plans[array].contentSerialized)
	}

	cycleItems := make([]*object.EmeraldValue, nativePDFRenderCompileMinEntries)
	cycle := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.Classes["Array"]}
	cycleItems[0] = cycle
	for index := 1; index < len(cycleItems); index++ {
		cycleItems[index] = core.NewIntegerValue(int64(index))
	}
	cycle.Data = cycleItems
	if nativePDFRenderValueShape(cycle, false, make(map[*object.EmeraldValue]bool), referenceClass, make(map[*object.EmeraldValue]nativePDFRenderValuePlan)) {
		t.Fatal("large cyclic array must side-exit")
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
	text, ok := nativePDFRenderHashWithLength(nativePDFEmptyHash(), 7, make(map[*object.EmeraldValue]bool), nil)
	if !ok {
		t.Fatal("expected empty dictionary to accept stream length")
	}
	if want := "<< /Length 7\n>>"; text != want {
		t.Fatalf("stream dictionary = %q, want %q", text, want)
	}
	var output strings.Builder
	if !nativePDFRenderWriteHashWithLength(&output, nativePDFEmptyHash(), 7, make(map[*object.EmeraldValue]bool), nil) {
		t.Fatal("direct stream dictionary writer rejected empty dictionary")
	}
	if got := output.String(); got != text {
		t.Fatalf("direct stream dictionary = %q, want %q", got, text)
	}

	lengthKey := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Length", Class: core.R.Classes["Symbol"]}
	data := nativePDFHashValue([2]*object.EmeraldValue{lengthKey, core.NewIntegerValue(1)})
	if _, ok := nativePDFRenderHashWithLength(data, 7, make(map[*object.EmeraldValue]bool), nil); ok {
		t.Fatal("expected an existing Length key to deopt instead of duplicating Ruby Hash#merge")
	}
}

func TestNativePDFRenderLayoutTemplateBindsTypedGraphAndRejectsShapeDrift(t *testing.T) {
	core.InitWithMspec()
	referenceClass := object.NewClass("PDF::Core::Reference")
	key := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Value", Class: core.R.Classes["Symbol"]}
	data := nativePDFHashValue([2]*object.EmeraldValue{key, core.NewIntegerValue(42)})
	info := nativePDFEmptyHash()
	ref := &object.EmeraldValue{Type: object.ValueObject, Class: referenceClass, Data: &object.Object{Class: referenceClass}}
	plan := &nativePDFRenderRegionPlan{
		root:  data,
		info:  info,
		refs:  []nativePDFRenderReferencePlan{{ref: ref, data: data}},
		pages: []nativePDFRenderPagePlan{{}},
	}
	template := nativePDFRenderLayoutTemplateFor(plan, referenceClass)
	if template == nil || len(template.nodes) < 3 || len(template.refs) != 1 {
		t.Fatalf("layout template = %#v", template)
	}
	newData := nativePDFHashValue([2]*object.EmeraldValue{key, core.NewIntegerValue(99)})
	bound, ok := nativePDFRenderBindLayoutTemplate(template, newData, nativePDFEmptyHash(), []*object.EmeraldValue{newData}, referenceClass)
	if !ok {
		t.Fatal("fresh graph should bind to the cached layout")
	}
	var output strings.Builder
	if !nativePDFRenderWriteLayoutNode(&output, bound, template.rootNode, false) {
		t.Fatal("typed layout writer rejected bound graph")
	}
	want, ok := nativePDFObjectText(newData, false, make(map[*object.EmeraldValue]bool))
	if !ok || output.String() != want {
		t.Fatalf("typed layout = %q, generic = %q, generic ok=%t", output.String(), want, ok)
	}
	var programOutput strings.Builder
	program := template.writePrograms[template.rootNode]
	if !nativePDFRenderWriteLayoutProgram(&programOutput, bound, program, 0) {
		t.Fatal("compiled typed layout writer rejected bound graph")
	}
	if programOutput.String() != want {
		t.Fatalf("compiled typed layout = %q, generic = %q", programOutput.String(), want)
	}

	wrongKey := &object.EmeraldValue{Type: object.ValueSymbol, Data: "Other", Class: core.R.Classes["Symbol"]}
	wrongData := nativePDFHashValue([2]*object.EmeraldValue{wrongKey, core.NewIntegerValue(99)})
	if _, ok := nativePDFRenderBindLayoutTemplate(template, wrongData, nativePDFEmptyHash(), []*object.EmeraldValue{wrongData}, referenceClass); ok {
		t.Fatal("changed dictionary key must side-exit the cached layout")
	}

	cycle := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.Classes["Array"]}
	cycle.Data = []*object.EmeraldValue{cycle}
	cyclePlan := &nativePDFRenderRegionPlan{
		root:  cycle,
		info:  nativePDFEmptyHash(),
		refs:  []nativePDFRenderReferencePlan{{ref: ref, data: cycle}},
		pages: []nativePDFRenderPagePlan{{}},
	}
	if nativePDFRenderLayoutTemplateFor(cyclePlan, referenceClass) != nil {
		t.Fatal("cyclic graph must not produce a typed layout template")
	}
}

func TestNativePDFRenderRealTextMatchesRubyNumberShape(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 0, want: "0"},
		{value: -0, want: "0"},
		{value: 36, want: "36"},
		{value: 1.3, want: "1.3"},
		{value: -1.23456, want: "-1.23456"},
		{value: 12.345678, want: "12.34568"},
	}
	for _, test := range tests {
		if got := nativePDFRenderRealText(test.value); got != test.want {
			t.Errorf("nativePDFRenderRealText(%v) = %q, want %q", test.value, got, test.want)
		}
	}
}
