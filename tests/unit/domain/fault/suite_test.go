//go:build unit

package fault_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestFault(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Domain Fault Suite")
}
