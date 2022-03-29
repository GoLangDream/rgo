package goby

import (
	"fmt"
	"github.com/huandu/xstrings"
	"strings"
	"unicode"
)

type String struct {
	str string
}

func NewString(str string) String {
	return String{str}
}

func TryCovertString(object Object) String {
	return object.ToS()
}

func (s String) ToS() String {
	return s
}

func (s String) Append(object Object) String {
	return NewString(fmt.Sprint(s.str, object.ToS()))
}

func (s String) AsciiOnly() bool {
	for i := 0; i < len(s.str); i++ {
		if s.str[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func (s String) Capitalize() String {
	return NewString(
		xstrings.FirstRuneToUpper(
			s.Downcase().str))
}

func (s String) Downcase() String {
	return NewString(strings.ToLower(s.str))
}

func (s String) Center(width int, pad string) String {
	return NewString(xstrings.Center(s.str, width, pad))
}

func (s String) CenterWithSpacePad(width int) String {
	return NewString(xstrings.Center(s.str, width, " "))
}
