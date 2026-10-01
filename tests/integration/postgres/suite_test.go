//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var (
	ctx       = context.Background()
	container *postgres.PostgresContainer
	pool      *pgxpool.Pool
	tx        pgx.Tx
)

func TestPostgresIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PostgreSQL integration suite")
}

var _ = BeforeSuite(func() {
	var err error
	container, err = postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("isura_ledger_test"),
		postgres.WithUsername("admin"),
		postgres.WithPassword("admin"),
		postgres.BasicWaitStrategies(),
	)
	Expect(err).NotTo(HaveOccurred())

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())
	_, sourceFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	dbDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../db"))
	migration := exec.CommandContext(ctx, "atlas", "schema", "apply", "--auto-approve", "--url", url, "--to", "file://"+dbDir)
	output, err := migration.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(output))

	pool, err = pgxpool.New(ctx, url)
	Expect(err).NotTo(HaveOccurred())
	Expect(pool.Ping(ctx)).To(Succeed())
})

var _ = AfterSuite(func() {
	if pool != nil {
		pool.Close()
	}
	if container != nil {
		Expect(testcontainers.TerminateContainer(container)).To(Succeed())
	}
})

var _ = BeforeEach(func() {
	var err error
	tx, err = pool.Begin(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterEach(func() {
	if tx == nil {
		return
	}
	err := tx.Rollback(ctx)
	Expect(err == nil || errors.Is(err, pgx.ErrTxClosed)).To(BeTrue())
	tx = nil
})
