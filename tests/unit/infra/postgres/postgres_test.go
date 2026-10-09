//go:build unit

package postgres_test

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/infra/configs"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: INFRA :: POSTGRES :: POSTGRES", func() {
	Describe("#NewPostgresWithContext", func() {
		Context("error cases", func() {
			It("should stop waiting for PostgreSQL when the connection deadline expires", func() {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				Expect(err).NotTo(HaveOccurred())
				done := make(chan struct{})
				DeferCleanup(func() {
					Expect(listener.Close()).To(Succeed())
					<-done
				})

				go func() {
					defer close(done)
					conn, acceptErr := listener.Accept()
					if acceptErr == nil {
						defer conn.Close()
						_, _ = io.Copy(io.Discard, conn)
					}
				}()

				address := listener.Addr().(*net.TCPAddr)
				cfg := &configs.Configs{DataBase: configs.DataBase{Postgres: configs.Postgres{
					Host: "127.0.0.1", Port: address.Port, User: "test", Database: "test", SSLMode: "disable", MaxConnections: 1,
				}}}
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				started := time.Now()
				pg, err := postgres.NewPostgresWithContext(ctx, cfg)
				elapsed := time.Since(started)
				if pg != nil {
					pg.Close()
				}

				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue(), "error: %v", err)
				Expect(elapsed).To(BeNumerically("<", time.Second))
			})
		})
	})
})
