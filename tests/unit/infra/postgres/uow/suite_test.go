//go:build unit

package uow_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUnitOfWork(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Unit of Work Suite")
}
