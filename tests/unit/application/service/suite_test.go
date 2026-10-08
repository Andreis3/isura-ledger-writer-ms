//go:build unit
// +build unit

package service_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLedgerEntryAssigner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application Service Suite")
}
