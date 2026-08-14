package vm

// This file contains the in-process native ABI for selected Gem hot
// functions. Every entry is source/class/layout guarded; the fallback VM
// remains authoritative for monkey-patched PDF::Core code or objects that the
// small typed serializer cannot prove safe.

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// The PDF/Core ABI is enabled by default because every entry is independently
// guarded by exact method source, class identity and built-in object layout.
// Unsupported subclasses, cycles, monkey-patches and encodings deopt to Ruby.
// RGO_DISABLE_NATIVE_PDF_OBJECT remains the compatibility switch for an exact
// old-VM comparison; RGO_ENABLE_NATIVE_PDF_OBJECT is still consumed by the
// separate closed-world Prawn constructor intrinsic.
var nativePDFObjectEnabled = os.Getenv("RGO_DISABLE_NATIVE_PDF_OBJECT") == ""

func (vm *VM) executeNativePDFObject(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !nativePDFObjectEnabled || methodObj == nil || methodObj.DispatchOwner != nil ||
		methodObj.Visibility != "" && methodObj.Visibility != "public" || receiver == nil ||
		receiver.Type != object.ValueModule || len(args) < 1 || len(args) > 2 {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "pdf_object" ||
		!strings.HasSuffix(fn.SourcePath, "/pdf_object.rb") {
		return nil, false
	}
	mod, ok := receiver.Data.(*object.Module)
	if !ok || mod == nil || mod.Name != "PDF::Core" {
		return nil, false
	}
	inContentStream := false
	if len(args) == 2 {
		if args[1] == nil || args[1].Type != object.ValueBool {
			return nil, false
		}
		inContentStream = args[1].Data.(bool)
	}
	var seen map[*object.EmeraldValue]bool
	if args[0] != nil && (args[0].Type == object.ValueArray || args[0].Type == object.ValueHash) {
		seen = make(map[*object.EmeraldValue]bool)
	}
	text, ok := nativePDFObjectText(args[0], inContentStream, seen)
	if !ok {
		return nil, false
	}
	// PDF::Core returns UTF-8 for composite/string values and US-ASCII for the
	// direct Integer#to_s branch.  The distinction is observable through
	// String#encoding, so retain it at the ABI boundary.
	encoding := "UTF-8"
	if args[0] != nil && args[0].Type == object.ValueInteger {
		encoding = "US-ASCII"
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: text, Class: core.R.Classes["String"], Encoding: encoding}, true
}

func nativePDFObjectText(value *object.EmeraldValue, inContentStream bool, seen map[*object.EmeraldValue]bool) (string, bool) {
	if value == nil {
		return "null", true
	}
	switch value.Type {
	case object.ValueNil:
		return "null", true
	case object.ValueBool:
		if value.Data.(bool) {
			return "true", true
		}
		return "false", true
	case object.ValueInteger:
		return nativePDFIntegerText(value)
	case object.ValueFloat:
		v, ok := value.Data.(float64)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", false
		}
		return nativePDFRealText(v), true
	case object.ValueSymbol:
		name, ok := value.Data.(string)
		if !ok {
			return "", false
		}
		return nativePDFSymbolText(name), true
	case object.ValueString:
		return nativePDFStringText(value, inContentStream)
	case object.ValueArray:
		if value.Class != core.R.Classes["Array"] || seen[value] {
			return "", false
		}
		items, ok := nativePDFArrayItems(value)
		if !ok {
			return "", false
		}
		seen[value] = true
		defer delete(seen, value)
		var out strings.Builder
		out.Grow(2 + len(items)*4)
		out.WriteByte('[')
		for index, item := range items {
			if index != 0 {
				out.WriteByte(' ')
			}
			text, itemOK := nativePDFObjectText(item, inContentStream, seen)
			if !itemOK {
				return "", false
			}
			out.WriteString(text)
		}
		out.WriteByte(']')
		return out.String(), true
	case object.ValueHash:
		if value.Class != core.R.Classes["Hash"] || seen[value] {
			return "", false
		}
		entries, ok := nativePDFHashEntries(value)
		if !ok {
			return "", false
		}
		seen[value] = true
		defer delete(seen, value)
		var out strings.Builder
		out.Grow(3 + len(entries)*12)
		out.WriteString("<< ")
		for _, entry := range entries {
			// PDF::Core converts String keys to Symbols before serializing them.
			keyText := nativePDFSymbolText(entry.keyName)
			valueText, valueOK := nativePDFObjectText(entry.value, inContentStream, seen)
			if !valueOK {
				return "", false
			}
			out.WriteString(keyText)
			out.WriteByte(' ')
			out.WriteString(valueText)
			out.WriteByte('\n')
		}
		out.WriteString(">>")
		return out.String(), true
	case object.ValueObject:
		return nativePDFReferenceText(value)
	default:
		return "", false
	}
}

func nativePDFIntegerText(value *object.EmeraldValue) (string, bool) {
	if value == nil {
		return "0", true
	}
	if bigInteger := value.BigIntValue(); bigInteger != nil {
		return bigInteger.Text(10), true
	}
	if integer, ok := value.Data.(int64); ok {
		return fmt.Sprintf("%d", integer), true
	}
	return "", false
}

func nativePDFRealText(value float64) string {
	text := fmt.Sprintf("%.5f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "-0" {
		return "0"
	}
	return text
}

func nativePDFStringText(value *object.EmeraldValue, inContentStream bool) (string, bool) {
	raw, ok := value.Data.(string)
	if !ok || value.Class == nil {
		return "", false
	}
	className := value.Class.Name
	switch className {
	case "PDF::Core::LiteralString":
		var out strings.Builder
		out.Grow(len(raw) + 2)
		out.WriteByte('(')
		for index := 0; index < len(raw); index++ {
			switch raw[index] {
			case '\\':
				out.WriteString(`\\`)
			case '\r':
				out.WriteString(`\r`)
			case '(':
				out.WriteString(`\(`)
			case ')':
				out.WriteString(`\)`)
			default:
				out.WriteByte(raw[index])
			}
		}
		out.WriteByte(')')
		return out.String(), true
	case "PDF::Core::ByteString":
		return "<" + hex.EncodeToString([]byte(raw)) + ">", true
	case "String":
		if inContentStream {
			return "<" + hex.EncodeToString([]byte(raw)) + ">", true
		}
		encoded, ok := nativePDFUTF16BE(raw)
		if !ok {
			return "", false
		}
		return "<" + hex.EncodeToString(encoded) + ">", true
	default:
		// A String subclass can override encoding/to_s behavior.  Deopt rather
		// than silently applying the base String implementation.
		return "", false
	}
}

func nativePDFUTF16BE(raw string) ([]byte, bool) {
	if !utf8Valid(raw) {
		return nil, false
	}
	encoded := utf16.Encode([]rune(raw))
	result := make([]byte, 2+len(encoded)*2)
	binary.BigEndian.PutUint16(result[:2], 0xFEFF)
	for index, unit := range encoded {
		binary.BigEndian.PutUint16(result[2+index*2:], unit)
	}
	return result, true
}

func utf8Valid(raw string) bool {
	// Avoid importing unicode/utf8 into the hot serializer's large dependency
	// surface through a tiny local check; the VM's String values are normally
	// already valid UTF-8, and malformed values should simply deopt.
	for index := 0; index < len(raw); {
		if raw[index] < 0x80 {
			index++
			continue
		}
		width := 0
		switch {
		case raw[index]&0xe0 == 0xc0:
			width = 2
		case raw[index]&0xf0 == 0xe0:
			width = 3
		case raw[index]&0xf8 == 0xf0:
			width = 4
		default:
			return false
		}
		if index+width > len(raw) {
			return false
		}
		for continuation := index + 1; continuation < index+width; continuation++ {
			if raw[continuation]&0xc0 != 0x80 {
				return false
			}
		}
		index += width
	}
	return true
}

func nativePDFSymbolText(name string) string {
	const upper = "0123456789ABCDEF"
	var out strings.Builder
	out.Grow(len(name) + 1)
	out.WriteByte('/')
	for index := 0; index < len(name); index++ {
		value := name[index]
		if value <= 32 || value == 35 || value == 40 || value == 41 || value == 47 ||
			value == 60 || value == 62 || value >= 127 {
			out.WriteByte('#')
			out.WriteByte(upper[value>>4])
			out.WriteByte(upper[value&0x0f])
			continue
		}
		out.WriteByte(value)
	}
	return out.String()
}

func nativePDFArrayItems(value *object.EmeraldValue) ([]*object.EmeraldValue, bool) {
	switch items := value.Data.(type) {
	case []*object.EmeraldValue:
		return items, true
	case *object.RArray:
		if items == nil {
			return nil, false
		}
		return items.Elements, true
	default:
		return nil, false
	}
}

type nativePDFHashEntry struct {
	keyName string
	value   *object.EmeraldValue
	order   int
}

func nativePDFHashEntries(value *object.EmeraldValue) ([]nativePDFHashEntry, bool) {
	var pairs map[*object.EmeraldValue]*object.EmeraldValue
	var orderedKeys []*object.EmeraldValue
	switch hash := value.Data.(type) {
	case map[*object.EmeraldValue]*object.EmeraldValue:
		pairs = hash
	case *object.RHash:
		if hash == nil {
			return nil, false
		}
		pairs = hash.Pairs
		orderedKeys = hash.Keys
	default:
		return nil, false
	}
	if pairs == nil {
		return nil, true
	}
	entries := make([]nativePDFHashEntry, 0, len(pairs))
	seenNames := make(map[string]bool, len(pairs))
	appendEntry := func(key, entryValue *object.EmeraldValue, order int) bool {
		if key == nil || entryValue == nil {
			return false
		}
		var name string
		switch key.Type {
		case object.ValueString:
			if key.Class == nil || key.Class.Name != "String" {
				return false
			}
			name, _ = key.Data.(string)
		case object.ValueSymbol:
			name, _ = key.Data.(string)
		default:
			return false
		}
		if seenNames[name] {
			// sort_by is stable; duplicate String/Symbol names require preserving
			// Ruby insertion order, so reject the map-only ambiguity.
			return false
		}
		seenNames[name] = true
		entries = append(entries, nativePDFHashEntry{keyName: name, value: entryValue, order: order})
		return true
	}
	if len(orderedKeys) > 0 {
		for order, key := range orderedKeys {
			entryValue, exists := pairs[key]
			if !exists || !appendEntry(key, entryValue, order) {
				return nil, false
			}
		}
		if len(entries) != len(pairs) {
			return nil, false
		}
	} else {
		order := 0
		for key, entryValue := range pairs {
			if !appendEntry(key, entryValue, order) {
				return nil, false
			}
			order++
		}
	}
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].keyName == entries[right].keyName {
			return entries[left].order < entries[right].order
		}
		return entries[left].keyName < entries[right].keyName
	})
	return entries, true
}

func nativePDFReferenceText(value *object.EmeraldValue) (string, bool) {
	if value == nil || value.Class == nil || value.Class.Name != "PDF::Core::Reference" {
		return "", false
	}
	method, ok := value.Class.GetMethod("to_s")
	if !ok || method == nil {
		return "", false
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil || fn.Name != "to_s" || !strings.HasSuffix(fn.SourcePath, "/reference.rb") {
		return "", false
	}
	identifier, ok := nativePDFObjectIntegerIvar(value, "@identifier")
	if !ok {
		return "", false
	}
	generation, ok := nativePDFObjectIntegerIvar(value, "@gen")
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d %d R", identifier, generation), true
}

func nativePDFObjectIntegerIvar(value *object.EmeraldValue, name string) (int64, bool) {
	item := core.DynamicInstanceVar(value, name)
	if item == nil || item.Type != object.ValueInteger || item.BigIntValue() != nil {
		return 0, false
	}
	integer, ok := item.Data.(int64)
	return integer, ok
}
