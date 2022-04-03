package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("rstring.AsciiOnly", func() {
	When("内容都是ascii的时候", func() {
		It("返回true", func() {
			var str = "abcdeadf"
			Expect(AsciiOnly(str)).To(BeTrue())
		})
	})

	When("内容包含特殊符号的时候", func() {
		It("返回false", func() {
			var str = "abc你好啊!efs"
			Expect(AsciiOnly(str)).To(BeFalse())
		})
	})
})
