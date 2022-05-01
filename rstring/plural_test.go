package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("String", func() {
	It("能正确转换单词的复数形式", func() {
		var str = "Empire"
		Expect(Plural(str)).To(
			Equal("Empires"))
	})
})
