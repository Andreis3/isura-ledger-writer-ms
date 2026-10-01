//go:build integration

package postgres_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPostgresIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PostgreSQL integration suite")
}
