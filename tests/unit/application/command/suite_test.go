//go:build unit
// +build unit

package command_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCreateTransaction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CreateTransaction Application Suite")
}
