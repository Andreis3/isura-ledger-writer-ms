//go:build integration

package nats_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNATSIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NATS JetStream integration suite")
}
