package core

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/object"
)

func cgiResult(raw string, source *object.EmeraldValue) *object.EmeraldValue {
	result := rubyString(raw)
	if source != nil {
		if encoding := stringEncodingName(source); encoding != "" {
			stringEncodings[result] = encoding
		}
	}
	return result
}

func cgiEscape(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	for index := 0; index < len(raw); index++ {
		value := raw[index]
		switch {
		case value == ' ':
			out.WriteByte('+')
		case value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || strings.ContainsRune("-._~", rune(value)):
			out.WriteByte(value)
		default:
			out.WriteByte('%')
			out.WriteByte(hex[value>>4])
			out.WriteByte(hex[value&15])
		}
	}
	return cgiResult(out.String(), source)
}

func cgiUnescape(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	out := make([]byte, 0, len(raw))
	for index := 0; index < len(raw); index++ {
		if raw[index] == '+' {
			out = append(out, ' ')
			continue
		}
		if raw[index] == '%' && index+2 < len(raw) {
			hi, lo := fromHex(raw[index+1]), fromHex(raw[index+2])
			if hi >= 0 && lo >= 0 {
				out = append(out, byte(hi<<4|lo))
				index += 2
				continue
			}
		}
		out = append(out, raw[index])
	}
	return cgiResult(string(out), source)
}

func cgiEscapeHTML(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	replacer := strings.NewReplacer("&", "&amp;", "'", "&#39;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return cgiResult(replacer.Replace(raw), source)
}

var cgiHTMLEntity = regexp.MustCompile(`&(apos|amp|quot|gt|lt|#[0-9]+|#[xX][0-9A-Fa-f]+);`)

func cgiUnescapeHTML(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	decoded := cgiHTMLEntity.ReplaceAllStringFunc(raw, func(entity string) string {
		name := entity[1 : len(entity)-1]
		switch name {
		case "apos":
			return "'"
		case "amp":
			return "&"
		case "quot":
			return `"`
		case "gt":
			return ">"
		case "lt":
			return "<"
		}
		base, digits := 10, name[1:]
		if len(digits) > 1 && (digits[0] == 'x' || digits[0] == 'X') {
			base, digits = 16, digits[1:]
		}
		value, err := strconv.ParseInt(digits, base, 32)
		if err != nil || value < 0 || value > utf8.MaxRune {
			return entity
		}
		return string(rune(value))
	})
	return cgiResult(decoded, source)
}

func cgiElementNames(args []*object.EmeraldValue) ([]string, *object.EmeraldValue) {
	values := args
	if len(values) == 1 && values[0] != nil && values[0].Type == object.ValueArray {
		values = values[0].Data.([]*object.EmeraldValue)
	}
	names := make([]string, 0, len(values))
	for _, value := range values {
		_, name, errVal := cgiStringArg(value)
		if errVal != nil {
			return nil, errVal
		}
		names = append(names, regexp.QuoteMeta(name))
	}
	return names, nil
}

func cgiEscapeElement(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	names, errVal := cgiElementNames(args[1:])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return cgiResult(raw, source)
	}
	pattern := regexp.MustCompile(`(?i)</?(?:` + strings.Join(names, "|") + `)\b[^<>]*>?`)
	result := pattern.ReplaceAllStringFunc(raw, func(tag string) string {
		return strings.NewReplacer("&", "&amp;", "'", "&#39;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(tag)
	})
	return cgiResult(result, source)
}

func cgiUnescapeElement(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	source, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	names, errVal := cgiElementNames(args[1:])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return cgiResult(raw, source)
	}
	pattern := regexp.MustCompile(`(?i)&lt;/?(?:` + strings.Join(names, "|") + `)\b.*?&gt;`)
	result := pattern.ReplaceAllStringFunc(raw, func(tag string) string {
		value := cgiUnescapeHTML(receiver, rubyString(tag))
		if value != nil && value.Type == object.ValueString {
			return value.Data.(string)
		}
		return tag
	})
	return cgiResult(result, source)
}
