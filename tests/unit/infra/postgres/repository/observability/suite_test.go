//go:build unit

package observability_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRepositoryObservability(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Repository Observability Suite")
}
