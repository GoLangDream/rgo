package rgo_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rgo Suite")
}
