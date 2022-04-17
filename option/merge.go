package option

import (
	. "github.com/elliotchance/orderedmap/v2"
)

type Option struct {
	*OrderedMap[string, any]
}

func Merge(opt1 *Option, opt2 []any) *Option {
	for index, key := range opt1.Keys() {
		if index < len(opt2) {
			opt1.Set(key, opt2[index])
		}
	}
	return opt1
}

func NewOption() *Option {
	return &Option{NewOrderedMap[string, any]()}
}

func (o *Option) Get(name string) any {
	return o.GetOrDefault(name, "")
}
