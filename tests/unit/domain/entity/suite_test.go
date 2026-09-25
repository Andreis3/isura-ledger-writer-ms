//go:build unit
// +build unit

package entity_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEntity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Entity Domain Suite")
}
