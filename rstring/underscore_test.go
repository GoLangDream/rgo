package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Underscore", func() {
	It("类似ruby on rails的underscore方法，可以把大小写转换成下划线", func() {
		str := Underscore("AbcDef")
		Expect(str).To(Equal("abc_def"))
	})
})
