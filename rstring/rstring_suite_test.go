package rstring_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRstring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rstring Suite")
}
