package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("String", func() {
	It("能把字符串的字母都变成小写", func() {
		var str = "ABc Test WORD"
		Expect(Downcase(str)).To(
			Equal("abc test word"))
	})
})
