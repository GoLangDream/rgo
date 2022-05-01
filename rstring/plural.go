package rstring

import "github.com/gertd/go-pluralize"

func Plural(str string) string {
	pluralize := pluralize.NewClient()
	return pluralize.Plural(str)
}
