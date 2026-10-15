//go:build unit

package criteria_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCriteria(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postgres Repository Criteria Suite")
}
