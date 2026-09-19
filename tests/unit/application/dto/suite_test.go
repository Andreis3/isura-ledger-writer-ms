//go:build unit
// +build unit

package dto_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDTO(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Application DTO Suite")
}
