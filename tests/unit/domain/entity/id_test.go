//go:build unit

package entity_test

import (
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = It("validates and formats values", func() {
	id, err := entity.NewID("d589965c-1622-4329-98f9-f13354a2e4dc")
	Expect(err).NotTo(HaveOccurred())
	Expect(id.String()).To(Equal("d589965c-1622-4329-98f9-f13354a2e4dc"))

	_, err = entity.NewID("invalid")
	Expect(err).To(HaveOccurred())

	id, err = entity.NewIDV7()
	Expect(err).NotTo(HaveOccurred())
	Expect(id.String()).NotTo(BeEmpty())
})
