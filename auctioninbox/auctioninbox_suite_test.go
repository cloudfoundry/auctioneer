package auctioninbox_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestAuctioninbox(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auctioninbox Suite")
}
