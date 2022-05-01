package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("String", func() {
	It("能正确转换单词的单数形式", func() {

		Expect(Singular("Empire")).To(
			Equal("Empire"))

		Expect(Singular("Empires")).To(
			Equal("Empire"))
	})
})
