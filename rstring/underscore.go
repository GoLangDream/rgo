package rstring

import "github.com/huandu/xstrings"

func Underscore(str string) string {
	return xstrings.ToSnakeCase(str)
}
