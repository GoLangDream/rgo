package rstring_test

import (
	"github.com/GoLangDream/rgo/rstring"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Camelize", func() {
	It("单词是已下划线分割的时候，能返回正确的值", func() {
		str := rstring.Camelize("abc_def")
		Expect(str).To(Equal("AbcDef"))
	})

	It("包含大小写的单词的时候，能返回正确的值", func() {
		str := rstring.Camelize("abcDef")
		Expect(str).To(Equal("AbcDef"))
	})
})
