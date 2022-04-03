package rstring

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("rstring.Capitalize", func() {

	When("第一个字母是小写，其他字母有大小写的情况下", func() {
		It("能把第一个字母大写，其他的全部小写", func() {
			var str = "abc Test WORD"
			Expect(Capitalize(str)).To(
				Equal("Abc test word"))
		})
	})

	When("第一个字母是大写，其他字母有大小写的情况下", func() {
		It("能把第一个字母大写，其他的全部小写", func() {
			var str = "ABc Test WORD"
			Expect(Capitalize(str)).To(
				Equal("Abc test word"))
		})
	})
})
