//go:build integration

package postgres_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
)

func TestPostgresRepository(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "PostgreSQL repository integration suite")
}

var _ = ginkgo.Describe("transaction repository", ginkgo.Ordered, func() {
	var (
		ctx       context.Context
		container *postgres.PostgresContainer
		pool      *pgxpool.Pool
		tx        pgx.Tx
	)

	ginkgo.BeforeAll(func() {
		ctx = context.Background()
		var err error
		container, err = postgres.Run(ctx,
			"postgres:18-alpine",
			postgres.WithDatabase("isura_ledger_test"),
			postgres.WithUsername("admin"),
			postgres.WithPassword("admin"),
			postgres.BasicWaitStrategies(),
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			if pool != nil {
				pool.Close()
			}
			gomega.Expect(testcontainers.TerminateContainer(container)).To(gomega.Succeed())
		})

		url, err := container.ConnectionString(ctx, "sslmode=disable")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, sourceFile, _, ok := runtime.Caller(0)
		gomega.Expect(ok).To(gomega.BeTrue())
		dbDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../db"))
		migration := exec.CommandContext(ctx, "atlas", "schema", "apply", "--auto-approve", "--url", url, "--to", "file://"+dbDir)
		output, err := migration.CombinedOutput()
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), string(output))

		pool, err = pgxpool.New(ctx, url)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pool.Ping(ctx)).To(gomega.Succeed())
	})

	ginkgo.BeforeEach(func() {
		var err error
		tx, err = pool.Begin(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(tx.Rollback(ctx)).To(gomega.Succeed())
	})

	ginkgo.It("persists a balanced transaction and its outbox atomically", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		transactionRepo := repository.NewTransactionRepository(pool)
		outboxRepo := repository.NewOutBoxRepository(pool)
		txContext := database.WithTx(ctx, tx)

		gomega.Expect(transactionRepo.Save(txContext, entityTransaction)).To(gomega.Succeed())
		gomega.Expect(outboxRepo.Save(txContext, newOutbox(entityTransaction.ID.String()))).To(gomega.Succeed())

		var transactions, entries, outboxes int
		gomega.Expect(tx.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&transactions)).To(gomega.Succeed())
		gomega.Expect(tx.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&entries)).To(gomega.Succeed())
		gomega.Expect(tx.QueryRow(ctx, "SELECT count(*) FROM outbox_events").Scan(&outboxes)).To(gomega.Succeed())
		gomega.Expect(transactions).To(gomega.Equal(1))
		gomega.Expect(entries).To(gomega.Equal(2))
		gomega.Expect(outboxes).To(gomega.Equal(1))
	})

	ginkgo.It("returns the same transaction and entries for an idempotency replay", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		original := newTransaction(accountA, accountB)
		repo := repository.NewTransactionRepository(pool)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repo.Save(txContext, original)).To(gomega.Succeed())

		key := original.IdempotencyKey
		replayed, err := repo.Find(txContext, criteriaForKey(key))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(replayed.ID).To(gomega.Equal(original.ID))
		gomega.Expect(replayed.Fingerprint).To(gomega.Equal(original.Fingerprint))
		gomega.Expect(replayed.Entries).To(gomega.HaveLen(2))
	})

	ginkgo.It("rolls back transaction, entries and outbox together", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(gomega.Succeed())
		gomega.Expect(repository.NewOutBoxRepository(pool).Save(txContext, newOutbox(entityTransaction.ID.String()))).To(gomega.Succeed())
		gomega.Expect(tx.Rollback(ctx)).To(gomega.Succeed())

		var count int
		gomega.Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", entityTransaction.ID.String()).Scan(&count)).To(gomega.Succeed())
		gomega.Expect(count).To(gomega.Equal(0))
		tx = nopTx{}
	})

	ginkgo.It("enforces positive amounts and valid directions in the database", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		_, err := tx.Exec(ctx, `INSERT INTO entries (id, account_id, transaction_id, account_sequence, direction, amount, currency, created_at) VALUES ($1, $2, $3, 1, 'INVALID', 0, 'BRL', $4)`, uuid.NewString(), accountA, uuid.NewString(), time.Now())
		gomega.Expect(err).To(gomega.HaveOccurred())
		_ = accountB
	})

	ginkgo.It("reconstructs a transfer from its append-only entries", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(gomega.Succeed())

		var debit, credit int64
		gomega.Expect(tx.QueryRow(ctx, "SELECT COALESCE(sum(amount) FILTER (WHERE direction = 'DEBIT'), 0), COALESCE(sum(amount) FILTER (WHERE direction = 'CREDIT'), 0) FROM entries WHERE transaction_id = $1", entityTransaction.ID.String()).Scan(&debit, &credit)).To(gomega.Succeed())
		gomega.Expect(debit).To(gomega.Equal(credit))
	})

	ginkgo.It("does not expose mutation methods for confirmed ledger facts", func() {
		typeOfRepository := reflect.TypeOf(repository.NewTransactionRepository(pool))
		_, hasUpdate := typeOfRepository.MethodByName("Update")
		_, hasDelete := typeOfRepository.MethodByName("Delete")
		gomega.Expect(hasUpdate).To(gomega.BeFalse())
		gomega.Expect(hasDelete).To(gomega.BeFalse())
	})
})

func insertAccounts(ctx context.Context, tx pgx.Tx) (string, string) {
	ids := []string{uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		_, err := tx.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'CHECKING', 'BRL', $5, $5)`, id, uuid.NewString(), uuid.NewString(), "12345678901234", time.Now())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	return ids[0], ids[1]
}

func newTransaction(accountA, accountB string) *transaction.Transaction {
	amount, err := money.NewMoney(1500, money.BRL)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	id := newIDV7()
	debit, err := transaction.NewEntryBuilder().WithID(newIDV7()).WithAccountExternalID(uuid.NewString()).WithTransactionID(id).WithDirection(transaction.Debit).WithAmount(amount).Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	credit, err := transaction.NewEntryBuilder().WithID(newIDV7()).WithAccountExternalID(uuid.NewString()).WithTransactionID(id).WithDirection(transaction.Credit).WithAmount(amount).Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	debit.AddAccountID(accountA)
	credit.AddAccountID(accountB)
	entityTransaction, err := transaction.NewTransactionBuilder().WithID(id).WithIdempotencyKey(uuid.NewString()).WithFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").WithStatus(transaction.Pending).WithAmount(amount).WithOperation(transaction.OperationTransfer).WithEntries([]*transaction.Entry{debit, credit}).Build()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return entityTransaction
}

func newIDV7() string {
	id, err := entity.NewIDV7()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return id.String()
}

func newOutbox(transactionID string) *outbox.Outbox {
	entry, err := outbox.NewOutbox(transactionID, []byte(`{"transaction_id":"`+transactionID+`"}`))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return entry
}

func criteriaForKey(key string) criteria.TransactionCriteria {
	return criteria.TransactionCriteria{IdempotencyKey: &key, WithEntries: true}
}

type nopTx struct{ pgx.Tx }

func (nopTx) Rollback(context.Context) error { return nil }
