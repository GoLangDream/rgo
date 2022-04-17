package option_test

import (
	"github.com/GoLangDream/rgo/option"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("option.Merge", func() {
	It("能正确的合并option 和 数组", func() {
		opt1 := option.NewOption()
		opt1.Set("name", "gin_cookie")
		opt1.Set("value", "test")
		opt1.Set("maxAge", 3600)
		opt1.Set("path", "/")
		opt1.Set("domain", "localhost")
		opt1.Set("secure", false)
		opt1.Set("httpOnly", false)

		opt2 := []any{"gin_cookie_2", "test_2", 3602, "/abc", "localhost_2", true, true}

		opt3 := option.Merge(opt1, opt2)
		Expect(opt3.Get("name")).To(Equal("gin_cookie_2"))
	})
})
