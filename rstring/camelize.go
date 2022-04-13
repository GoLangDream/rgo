package rstring

import "github.com/huandu/xstrings"

func Camelize(str string) string {
	return xstrings.ToCamelCase(str)
}
