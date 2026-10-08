//go:build unit

package configs_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
)

func TestConfigs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Infrastructure Configs Suite")
}

var _ = BeforeEach(func() {
	viper.Reset()
})
