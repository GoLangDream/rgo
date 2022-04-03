package rstring

import "github.com/huandu/xstrings"

func Center(str string, width int, pad string) string {
	return xstrings.Center(str, width, pad)
}

func CenterWithSpacePad(str string, width int) string {
	return Center(str, width, " ")
}
