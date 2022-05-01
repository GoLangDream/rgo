package rstring

import "github.com/gertd/go-pluralize"

func Singular(str string) string {
	pluralize := pluralize.NewClient()
	return pluralize.Singular(str)
}
