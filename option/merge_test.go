package option_test

import (
	. "github.com/GoLangDream/rgo/option"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("option.Merge", func() {
	It("能正确的合并option 和 数组", func() {
		opt1 := Option{
			"name":     "gin_cookie",
			"value":    "test",
			"maxAge":   3600,
			"path":     "/",
			"domain":   "localhost",
			"secure":   false,
			"httpOnly": false,
		}

		opt2 := Option{
			"name":     "gin_cookie_2",
			"value":    "test_2",
			"maxAge":   3602,
			"path":     "/2",
			"domain":   "localhost_2",
			"secure":   true,
			"httpOnly": true,
		}

		Merge(opt1, opt2)
		Expect(opt2["name"]).To(Equal("gin_cookie_2"))
	})
})
