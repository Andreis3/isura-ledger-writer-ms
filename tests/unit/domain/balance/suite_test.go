//go:build unit
// +build unit

package balance_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBalance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Balance Domain Suite")
}
