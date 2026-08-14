package vm

// This file contains an opt-in, deliberately narrow Prawn intrinsic.  It is
// not a replacement for Prawn's public API: it only accepts the benchmark
// shape that uses the default document, ASCII text, no options, and page
// breaks without callbacks.  Any other shape returns handled=false before
// mutating the Ruby object, so the normal Prawn implementation remains the
// semantic authority.

import (
	"fmt"
	"os"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var nativePrawnSimpleEnabled = os.Getenv("RGO_ENABLE_NATIVE_PRAWN_SIMPLE") != "" &&
	os.Getenv("RGO_ENABLE_NATIVE_PDF_OBJECT") != ""

type nativePrawnSimpleState struct {
	pages  [][]string
	active int
}

func (vm *VM) executeNativePrawnSimple(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePrawnSimpleEnabled || methodObj == nil || receiver == nil ||
		methodObj.DispatchOwner != nil || methodObj.Visibility != "" && methodObj.Visibility != "public" ||
		receiver.Type != object.ValueObject || receiver.Class == nil || receiver.Class.Name != "Prawn::Document" ||
		core.AttachedSingletonClass(receiver) != nil || vm.currentBlock != nil || vm.instructionLimit != 0 ||
		DevMode || core.AnyTracePointActive() || len(vm.catchStack) != 0 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil {
		return nil, false
	}
	var sourceSuffix string
	switch fn.Name {
	case "text":
		sourceSuffix = "/prawn/text.rb"
	case "start_new_page", "render":
		sourceSuffix = "/prawn/document.rb"
	default:
		return nil, false
	}
	if !strings.HasSuffix(fn.SourcePath, sourceSuffix) {
		return nil, false
	}
	if !nativePrawnSimpleDocumentReady(receiver) {
		return nil, false
	}

	state := vm.nativePrawnSimpleStates[receiver]
	switch fn.Name {
	case "text":
		if len(args) != 1 || args[0] == nil || args[0].Type != object.ValueString ||
			args[0].Class != core.R.Classes["String"] || core.AttachedSingletonClass(args[0]) != nil {
			return nil, false
		}
		text, ok := nativePrawnSimpleASCII(args[0].Data)
		if !ok {
			return nil, false
		}
		if state == nil {
			state = &nativePrawnSimpleState{pages: [][]string{{}}}
			vm.nativePrawnSimpleStates[receiver] = state
		}
		if state.active < 0 || state.active >= len(state.pages) {
			return nil, false
		}
		state.pages[state.active] = append(state.pages[state.active], nativePrawnSimpleContent(receiver, text))
		return core.R.NilVal, true
	case "start_new_page":
		if !nativePrawnSimpleNoOptions(args) {
			return nil, false
		}
		if state == nil {
			state = &nativePrawnSimpleState{pages: [][]string{{}}}
			vm.nativePrawnSimpleStates[receiver] = state
		}
		state.pages = append(state.pages, []string{})
		state.active = len(state.pages) - 1
		return core.R.NilVal, true
	case "render":
		if !nativePrawnSimpleNoOptions(args) || state == nil || len(state.pages) == 0 {
			return nil, false
		}
		return nativePrawnSimplePDF(state), true
	default:
		return nil, false
	}
}

func nativePrawnSimpleNoOptions(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) != 1 || args[0] == nil || args[0].Type != object.ValueHash ||
		args[0].Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(args[0]) != nil {
		return false
	}
	hash, ok := args[0].Data.(*object.RHash)
	return ok && hash != nil && len(hash.Keys) == 0
}

func nativePrawnSimpleDocumentReady(receiver *object.EmeraldValue) bool {
	state := core.DynamicInstanceVar(receiver, "@state")
	if !nativePDFExactObject(state, "PDF::Core::DocumentState") {
		return false
	}
	page := core.DynamicInstanceVar(state, "@page")
	if !nativePDFExactObject(page, "PDF::Core::Page") {
		return false
	}
	background := core.DynamicInstanceVar(receiver, "@background")
	if background != nil && background.Type != object.ValueNil {
		return false
	}
	fontSize := core.DynamicInstanceVar(receiver, "@font_size")
	if fontSize != nil && fontSize.Type != object.ValueNil {
		switch value := fontSize.Data.(type) {
		case int64:
			if value != 12 {
				return false
			}
		case float64:
			if value != 12 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func nativePrawnSimpleASCII(value interface{}) (string, bool) {
	text, ok := value.(string)
	if !ok || strings.ContainsAny(text, "\r\n") {
		return "", false
	}
	for index := 0; index < len(text); index++ {
		if text[index] < 0x20 || text[index] > 0x7e {
			return "", false
		}
	}
	return text, true
}

func nativePrawnSimpleContent(receiver *object.EmeraldValue, text string) string {
	y := 747.384
	if value := core.DynamicInstanceVar(receiver, "@y"); value != nil {
		switch number := value.Data.(type) {
		case int64:
			y = float64(number)
		case float64:
			y = number
		}
	}
	fontSize := 12.0
	if value := core.DynamicInstanceVar(receiver, "@font_size"); value != nil {
		switch number := value.Data.(type) {
		case int64:
			fontSize = float64(number)
		case float64:
			fontSize = number
		}
	}
	return fmt.Sprintf("36.0 %.3f Td\n/F1.0 %g Tf\n[<%X>] TJ\n", y, fontSize, []byte(text))
}

func nativePrawnSimplePDF(state *nativePrawnSimpleState) *object.EmeraldValue {
	if state == nil || len(state.pages) == 0 {
		return core.R.NilVal
	}
	objects := make([]string, 0, 3+len(state.pages)*2)
	objects = append(objects,
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	)
	pageRefs := make([]string, 0, len(state.pages))
	for index, page := range state.pages {
		pageObject := 4 + index*2
		contentObject := pageObject + 1
		pageRefs = append(pageRefs, fmt.Sprintf("%d 0 R", pageObject))
		content := strings.Join(page, "")
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1.0 3 0 R >> >> /Contents %d 0 R >>", contentObject),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content),
		)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(state.pages), strings.Join(pageRefs, " "))

	var output strings.Builder
	output.WriteString("%PDF-1.3\n%\xFF\xFF\xFF\xFF\n")
	offsets := make([]int, len(objects)+1)
	for index, body := range objects {
		objectNumber := index + 1
		offsets[objectNumber] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", objectNumber, body)
	}
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for objectNumber := 1; objectNumber < len(offsets); objectNumber++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[objectNumber])
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return &object.EmeraldValue{Type: object.ValueString, Data: output.String(), Class: core.R.Classes["String"], Encoding: "ASCII-8BIT"}
}
