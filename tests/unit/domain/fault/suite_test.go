//go:build unit

package fault_test

import (
 "testing"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

func TestFault(t *testing.T) {
 RegisterFailHandler(Fail)
 RunSpecs(t, "Domain Fault Suite")
}
