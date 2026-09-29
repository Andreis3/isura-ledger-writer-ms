//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
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

	ginkgo.It("keeps reconciliation on one snapshot while a balance is updated concurrently", func() {
		accountID := insertAccount(ctx, pool)
		transactionID := newIDV7()
		entryID := newIDV7()
		now := time.Now()
		_, err := pool.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 50, 'BRL', $4, $4)`,
			transactionID, uuid.NewString(), "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = pool.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, 1, 'DEBIT', 50, 50, 'BRL', $4)`, entryID, accountID, transactionID, now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
			gomega.Expect(cleanupErr).NotTo(gomega.HaveOccurred())
			_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
			gomega.Expect(cleanupErr).NotTo(gomega.HaveOccurred())
			_, cleanupErr = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
			gomega.Expect(cleanupErr).NotTo(gomega.HaveOccurred())
		})

		auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			rollbackErr := auditTx.Rollback(ctx)
			gomega.Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(gomega.BeTrue())
		})
		var snapshotBalance int64
		gomega.Expect(auditTx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE id = $1", entryID).Scan(&snapshotBalance)).To(gomega.Succeed())
		gomega.Expect(snapshotBalance).To(gomega.Equal(int64(50)))
		_, err = pool.Exec(ctx, "UPDATE entries SET running_balance = 51 WHERE id = $1", entryID)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(report.EntriesChecked).To(gomega.Equal(1))
		gomega.Expect(report.Reconciled).To(gomega.BeTrue())
		gomega.Expect(report.MismatchCount).To(gomega.BeZero())
		gomega.Expect(auditTx.Commit(ctx)).To(gomega.Succeed())
	})

	ginkgo.It("audits persisted entries without changing ledger records", func() {
		accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 125)
		ginkgo.DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

		before := reconciliationSnapshot(ctx, pool, accountID)
		auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(auditTx.Rollback(ctx)).To(gomega.Succeed())

		gomega.Expect(report.AccountsChecked).To(gomega.BeNumerically(">=", 1))
		gomega.Expect(report.EntriesChecked).To(gomega.BeNumerically(">=", 1))
		gomega.Expect(report.Mismatches).NotTo(gomega.ContainElement(gomega.HaveField("AccountID", accountID)))
		gomega.Expect(reconciliationSnapshot(ctx, pool, accountID)).To(gomega.Equal(before))
	})

	ginkgo.It("reports the replayed and persisted balances for a PostgreSQL mismatch", func() {
		accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 120)
		ginkgo.DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

		auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer func() {
			rollbackErr := auditTx.Rollback(ctx)
			gomega.Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(gomega.BeTrue())
		}()
		report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		mismatch, found := reconciliationMismatchFor(report, accountID)
		gomega.Expect(found).To(gomega.BeTrue())
		gomega.Expect(mismatch.EntriesChecked).To(gomega.Equal(1))
		gomega.Expect(mismatch.ExpectedBalance).To(gomega.Equal(int64(125)))
		gomega.Expect(mismatch.PersistedBalance).To(gomega.Equal(int64(120)))
		gomega.Expect(mismatch.Currency).To(gomega.Equal("BRL"))
	})

	ginkgo.It("returns an explicit error for an entry referencing an unknown account", func() {
		transactionID := newIDV7()
		entryID := newIDV7()
		missingAccountID := newIDV7()
		now := time.Now()
		_, err := pool.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 75, 'BRL', $4, $4)`,
			transactionID, uuid.NewString(), "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.DeferCleanup(func() {
			_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
			gomega.Expect(cleanupErr).NotTo(gomega.HaveOccurred())
			_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
			gomega.Expect(cleanupErr).NotTo(gomega.HaveOccurred())
		})
		fixtureTx, err := pool.Begin(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = fixtureTx.Exec(ctx, "SET LOCAL session_replication_role = replica")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = fixtureTx.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, 1, 'DEBIT', 75, 75, 'BRL', $4)`, entryID, missingAccountID, transactionID, now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(fixtureTx.Commit(ctx)).To(gomega.Succeed())

		before := reconciliationEntrySnapshot(ctx, pool, entryID)
		auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("unknown account"))
		gomega.Expect(auditTx.Rollback(ctx)).To(gomega.Succeed())
		gomega.Expect(reconciliationEntrySnapshot(ctx, pool, entryID)).To(gomega.Equal(before))
	})

	ginkgo.It("persists and reads the account balance policy", func() {
		externalID := uuid.NewString()
		entityAccount, err := account.NewAccountBuilder().
			WithID(newIDV7()).
			WithAccountExternalID(externalID).
			WithAccountNumber(fmt.Sprintf("%d", time.Now().UnixNano())).
			WithTaxID("52998224725").
			WithStatus().
			WithType(string(account.Asset)).
			WithBalancePolicy(string(account.BalanceNonNegative)).
			WithCurrency(string(money.BRL)).
			Build()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		repo := repository.NewAccountRepository(pool)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repo.Save(txContext, entityAccount)).To(gomega.Succeed())

		found, err := repo.FindAccount(txContext, criteria.AccountCriteria{AccountExternalID: &externalID})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(found).NotTo(gomega.BeNil())
		gomega.Expect(found.BalancePolicy).To(gomega.Equal(account.BalanceNonNegative))
	})

	ginkgo.It("recalculates historical balances using the account nature and preserves sequences", func() {
		accountID := newIDV7()
		transactionID := newIDV7()
		now := time.Now()
		_, err := tx.Exec(ctx, `
			INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_NON_NEGATIVE', 'BRL', $5, $5)`,
			accountID, uuid.NewString(), fmt.Sprintf("%d", now.UnixNano()), "52998224725", now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = tx.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'PENDING', 'TRANSFER', 100, 'BRL', $4, $4)`,
			transactionID, uuid.NewString(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, entry := range []struct {
			id, direction                    string
			sequence, amount, runningBalance int64
		}{
			{id: newIDV7(), direction: "DEBIT", sequence: 1, amount: 100, runningBalance: -100},
			{id: newIDV7(), direction: "CREDIT", sequence: 2, amount: 40, runningBalance: -60},
		} {
			_, err = tx.Exec(ctx, `
				INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 'BRL', $8)`,
				entry.id, accountID, transactionID, entry.sequence, entry.direction, entry.amount, entry.runningBalance, now)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		report, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(report.Accounts).To(gomega.Equal(1))
		gomega.Expect(report.Entries).To(gomega.Equal(2))
		gomega.Expect(report.UpdatedEntries).To(gomega.Equal(2))
		gomega.Expect(report.PersistedBalanceMismatches).To(gomega.Equal(2))
		gomega.Expect(report.SequenceMismatches).To(gomega.Equal(0))
		gomega.Expect(report.PolicyViolations).To(gomega.BeEmpty())
		gomega.Expect(report.ReadyForActivation).To(gomega.BeTrue())

		repeatedReport, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(repeatedReport.PersistedBalanceMismatches).To(gomega.Equal(0))
		gomega.Expect(repeatedReport.UpdatedEntries).To(gomega.Equal(0))
		gomega.Expect(repeatedReport.ReadyForActivation).To(gomega.BeTrue())

		rows, err := tx.Query(ctx, `SELECT sequence_number, running_balance FROM entries WHERE account_id = $1 ORDER BY sequence_number`, accountID)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer rows.Close()
		var balances []int64
		for rows.Next() {
			var sequence, balance int64
			gomega.Expect(rows.Scan(&sequence, &balance)).To(gomega.Succeed())
			balances = append(balances, balance)
		}
		gomega.Expect(rows.Err()).NotTo(gomega.HaveOccurred())
		gomega.Expect(balances).To(gomega.Equal([]int64{100, 60}))
	})

	ginkgo.It("commits or rolls back transaction, entries and outbox as one unit", func() {
		accountA := insertAccount(ctx, pool)
		accountB := insertAccount(ctx, pool)
		transactionRepo := repository.NewTransactionRepository(pool)
		outboxRepo := repository.NewOutBoxRepository(pool)
		unitOfWork := uow.NewUnitOfWork(pool)

		committed := newTransaction(accountA, accountB)
		err := unitOfWork.WithTransaction(ctx, func(txCtx context.Context) error {
			if err := transactionRepo.Save(txCtx, committed); err != nil {
				return err
			}
			return outboxRepo.Save(txCtx, newOutbox(committed.ID.String()))
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		assertLedgerRecords(ctx, pool, committed.ID.String(), 1, 2, 1)

		rolledBack := newTransaction(accountA, accountB)
		expectedFailure := errors.New("force atomic rollback")
		err = unitOfWork.WithTransaction(ctx, func(txCtx context.Context) error {
			if err := transactionRepo.Save(txCtx, rolledBack); err != nil {
				return err
			}
			if err := outboxRepo.Save(txCtx, newOutbox(rolledBack.ID.String())); err != nil {
				return err
			}
			return expectedFailure
		})
		gomega.Expect(err).To(gomega.MatchError(expectedFailure))

		assertLedgerRecords(ctx, pool, rolledBack.ID.String(), 0, 0, 0)
	})

	ginkgo.It("rejects insufficient balance before creating ledger records", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		_, err := tx.Exec(ctx, `UPDATE accounts SET balance_policy = 'BALANCE_NON_NEGATIVE' WHERE id IN ($1, $2)`, accountA, accountB)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		entityTransaction := newTransaction(accountA, accountB)
		err = repository.NewTransactionRepository(pool).Save(database.WithTx(ctx, tx), entityTransaction)

		gomega.Expect(errors.Is(err, fault.ErrInsufficientBalance)).To(gomega.BeTrue())
		assertLedgerRecords(ctx, pool, entityTransaction.ID.String(), 0, 0, 0)
		gomega.Expect(entityTransaction.Entries[0].SequenceNumber).To(gomega.Equal(int64(0)))
		gomega.Expect(entityTransaction.Entries[1].SequenceNumber).To(gomega.Equal(int64(0)))
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

		id := original.ID.String()
		byID, err := repo.Find(txContext, transaction.TransactionCriteria{ID: &id, WithEntries: true})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(byID.ID).To(gomega.Equal(original.ID))
		gomega.Expect(byID.Entries).To(gomega.HaveLen(2))
	})

	ginkgo.It("serializes concurrent requests with the same idempotency key", func() {
		debitExternalID, creditExternalID := insertAccountsForCommand(ctx, pool)
		key := uuid.NewString()
		amount := int64(1500)
		currency := string(money.BRL)
		operation := string(transaction.OperationTransfer)
		newInput := func() dto.CreateTransactionInput {
			return dto.CreateTransactionInput{
				IdempotencyKey:  &key,
				DebitAccountID:  &debitExternalID,
				CreditAccountID: &creditExternalID,
				Amount:          &amount,
				Currency:        &currency,
				Operation:       &operation,
			}
		}

		createTransaction := command.NewCreateTransaction(
			uow.NewUnitOfWork(pool),
			repository.NewAccountRepository(pool),
			repository.NewTransactionRepository(pool),
			repository.NewOutBoxRepository(pool),
			integrationTracer{}, integrationLogger{}, integrationMetrics{},
		)
		results := make(chan *dto.CreateTransactionOutput, 2)
		errorsCh := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := createTransaction.Execute(ctx, newInput())
				results <- result
				errorsCh <- err
			}()
		}
		wg.Wait()
		close(results)
		close(errorsCh)

		var transactionID string
		replays := 0
		for err := range errorsCh {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
		for result := range results {
			gomega.Expect(result).NotTo(gomega.BeNil())
			if result.IdempotentReplay {
				replays++
			}
			if transactionID == "" {
				transactionID = *result.TransactionID
			}
			gomega.Expect(*result.TransactionID).To(gomega.Equal(transactionID))
		}
		gomega.Expect(replays).To(gomega.Equal(1))

		var count int
		gomega.Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE idempotency_key = $1", key).Scan(&count)).To(gomega.Succeed())
		gomega.Expect(count).To(gomega.Equal(1))
	})

	ginkgo.It("approves only one of two concurrent debits that consume the available balance", func() {
		debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 100, 10)
		createTransaction := newIntegrationCreateTransaction(pool)

		start := make(chan struct{})
		results := make(chan commandResult, 2)
		var wg sync.WaitGroup
		for index := range 2 {
			key := fmt.Sprintf("cd-%d-%s", index, uuid.NewString())
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				<-start
				results <- executeIntegrationTransaction(ctx, createTransaction, debitExternalID, creditExternalID, key, 100)
			}(key)
		}
		close(start)
		wg.Wait()
		close(results)

		successes := 0
		insufficientBalance := 0
		for result := range results {
			if result.err == nil {
				successes++
				continue
			}
			gomega.Expect(errors.Is(result.err, fault.ErrInsufficientBalance)).To(gomega.BeTrue(), "unexpected concurrent result: %v", result.err)
			insufficientBalance++
		}
		gomega.Expect(successes).To(gomega.Equal(1))
		gomega.Expect(insufficientBalance).To(gomega.Equal(1))

		var sourceEntries, sourceSequence, sourceBalance int64
		gomega.Expect(pool.QueryRow(ctx, `
			SELECT count(*), max(sequence_number)
			FROM entries e JOIN accounts a ON a.id = e.account_id
			WHERE a.account_external_id = $1`, debitExternalID).
			Scan(&sourceEntries, &sourceSequence)).To(gomega.Succeed())
		gomega.Expect(pool.QueryRow(ctx, `
			SELECT e.running_balance
			FROM entries e JOIN accounts a ON a.id = e.account_id
			WHERE a.account_external_id = $1
			ORDER BY e.sequence_number DESC
			LIMIT 1`, debitExternalID).Scan(&sourceBalance)).To(gomega.Succeed())
		gomega.Expect(sourceEntries).To(gomega.Equal(int64(2)))
		gomega.Expect(sourceSequence).To(gomega.Equal(int64(11)))
		gomega.Expect(sourceBalance).To(gomega.Equal(int64(0)))

		var createdTransactions int
		gomega.Expect(pool.QueryRow(ctx, `
			SELECT count(*) FROM transactions
			WHERE idempotency_key LIKE 'cd-%'`).Scan(&createdTransactions)).To(gomega.Succeed())
		gomega.Expect(createdTransactions).To(gomega.Equal(1))
	})

	ginkgo.It("replays a successful transaction without creating another entry", func() {
		debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 100, 1)
		createTransaction := newIntegrationCreateTransaction(pool)
		key := "replay-" + uuid.NewString()

		first := executeIntegrationTransaction(ctx, createTransaction, debitExternalID, creditExternalID, key, 100)
		gomega.Expect(first.err).NotTo(gomega.HaveOccurred())
		gomega.Expect(first.output).NotTo(gomega.BeNil())
		gomega.Expect(first.output.IdempotentReplay).To(gomega.BeFalse())

		second := executeIntegrationTransaction(ctx, createTransaction, debitExternalID, creditExternalID, key, 100)
		gomega.Expect(second.err).NotTo(gomega.HaveOccurred())
		gomega.Expect(second.output).NotTo(gomega.BeNil())
		gomega.Expect(second.output.IdempotentReplay).To(gomega.BeTrue())
		gomega.Expect(second.output.TransactionID).To(gomega.Equal(first.output.TransactionID))

		var transactions, entries, outboxes int
		gomega.Expect(pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE idempotency_key = $1`, key).Scan(&transactions)).To(gomega.Succeed())
		gomega.Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE transaction_id = $1`, *first.output.TransactionID).Scan(&entries)).To(gomega.Succeed())
		gomega.Expect(pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, *first.output.TransactionID).Scan(&outboxes)).To(gomega.Succeed())
		gomega.Expect(transactions).To(gomega.Equal(1))
		gomega.Expect(entries).To(gomega.Equal(2))
		gomega.Expect(outboxes).To(gomega.Equal(1))
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
		_, err := tx.Exec(ctx, `INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at) VALUES ($1, $2, $3, 1, 'INVALID', 0, 0, 'BRL', $4)`, uuid.NewString(), accountA, uuid.NewString(), time.Now())
		gomega.Expect(err).To(gomega.HaveOccurred())
		_ = accountB
	})

	ginkgo.It("applies the unique sequence index per account", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(gomega.Succeed())

		var indexDefinition string
		gomega.Expect(tx.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'entries' AND indexname = 'unique_entry_sequence_number'`).Scan(&indexDefinition)).To(gomega.Succeed())
		gomega.Expect(indexDefinition).To(gomega.ContainSubstring("UNIQUE"))
		gomega.Expect(indexDefinition).To(gomega.ContainSubstring("account_id, sequence_number"))

		_, err := tx.Exec(ctx, `
			INSERT INTO entries (
				id, account_id, transaction_id, sequence_number, direction, amount,
				running_balance, currency, created_at
			) VALUES ($1, $2, $3, 1, 'DEBIT', 1500, -1500, 'BRL', $4)
		`, uuid.NewString(), accountA, entityTransaction.ID.String(), time.Now())
		var pgErr *pgconn.PgError
		gomega.Expect(errors.As(err, &pgErr)).To(gomega.BeTrue())
		gomega.Expect(pgErr.ConstraintName).To(gomega.Equal("unique_entry_sequence_number"))
	})

	ginkgo.It("reconstructs a transfer from its append-only entries", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		txContext := database.WithTx(ctx, tx)
		gomega.Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(gomega.Succeed())

		var debit, credit int64
		gomega.Expect(tx.QueryRow(ctx, "SELECT COALESCE(sum(amount) FILTER (WHERE direction = 'DEBIT'), 0), COALESCE(sum(amount) FILTER (WHERE direction = 'CREDIT'), 0) FROM entries WHERE transaction_id = $1", entityTransaction.ID.String()).Scan(&debit, &credit)).To(gomega.Succeed())
		gomega.Expect(debit).To(gomega.Equal(credit))

		var debitBalance, creditBalance int64
		gomega.Expect(tx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE account_id = $1", accountA).Scan(&debitBalance)).To(gomega.Succeed())
		gomega.Expect(tx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE account_id = $1", accountB).Scan(&creditBalance)).To(gomega.Succeed())
		gomega.Expect(debitBalance).To(gomega.Equal(int64(1500)))
		gomega.Expect(creditBalance).To(gomega.Equal(int64(-1500)))
	})

	ginkgo.It("does not expose mutation methods for confirmed ledger facts", func() {
		typeOfRepository := reflect.TypeOf(repository.NewTransactionRepository(pool))
		_, hasUpdate := typeOfRepository.MethodByName("Update")
		_, hasDelete := typeOfRepository.MethodByName("Delete")
		gomega.Expect(hasUpdate).To(gomega.BeFalse())
		gomega.Expect(hasDelete).To(gomega.BeFalse())
	})

	ginkgo.It("reads accounts without acquiring a pessimistic lock", func() {
		externalID, _ := insertAccountsForCommand(ctx, pool)
		blocker, err := pool.Begin(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer blocker.Rollback(ctx)
		_, err = blocker.Exec(ctx, "SELECT id FROM accounts WHERE account_external_id = $1 FOR UPDATE", externalID)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		found, err := repository.NewAccountRepository(pool).FindAccount(readCtx, criteria.AccountCriteria{
			AccountExternalID: &externalID,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(found).NotTo(gomega.BeNil())
	})

	ginkgo.It("assigns unique monotonic sequences for concurrent transfers sharing an account", func() {
		sharedAccount := insertAccount(ctx, pool)
		otherAccountA := insertAccount(ctx, pool)
		otherAccountB := insertAccount(ctx, pool)

		transfers := []*transaction.Transaction{
			newTransaction(sharedAccount, otherAccountA),
			newTransaction(otherAccountB, sharedAccount),
		}
		errs := make(chan error, len(transfers))
		var wg sync.WaitGroup
		for _, transfer := range transfers {
			wg.Add(1)
			go func(transfer *transaction.Transaction) {
				defer wg.Done()
				transactionCtx, err := pool.Begin(ctx)
				if err != nil {
					errs <- err
					return
				}
				if err := repository.NewTransactionRepository(pool).Save(database.WithTx(ctx, transactionCtx), transfer); err != nil {
					rollbackErr := transactionCtx.Rollback(ctx)
					if rollbackErr != nil {
						errs <- errors.Join(err, rollbackErr)
						return
					}
					errs <- err
					return
				}
				errs <- transactionCtx.Commit(ctx)
			}(transfer)
		}
		wg.Wait()
		close(errs)
		successes := 0
		for err := range errs {
			if err == nil {
				successes++
				continue
			}
			var pgErr *pgconn.PgError
			gomega.Expect(errors.As(err, &pgErr)).To(gomega.BeTrue())
			gomega.Expect(pgErr.ConstraintName).To(gomega.Equal("unique_entry_sequence_number"))
		}
		gomega.Expect(successes).To(gomega.BeNumerically(">=", 1))

		rows, err := pool.Query(ctx, `SELECT sequence_number FROM entries WHERE account_id = $1 ORDER BY sequence_number`, sharedAccount)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer rows.Close()
		sequences := make([]int64, 0, 2)
		for rows.Next() {
			var sequence int64
			gomega.Expect(rows.Scan(&sequence)).To(gomega.Succeed())
			sequences = append(sequences, sequence)
		}
		gomega.Expect(rows.Err()).NotTo(gomega.HaveOccurred())
		gomega.Expect(sequences).NotTo(gomega.BeEmpty())
		gomega.Expect(sequences[0]).To(gomega.Equal(int64(1)))
		if len(sequences) == 2 {
			gomega.Expect(sequences[1]).To(gomega.Equal(int64(2)))
		}
	})

	ginkgo.It("returns ILMS-2004 after exhausting serialization retries", func() {
		unitOfWork := uow.NewUnitOfWork(pool)
		attempts := 0

		err := unitOfWork.WithRetryableTransaction(ctx, func(context.Context) error {
			attempts++
			return &pgconn.PgError{Code: "40001", Message: "serialization failure"}
		})

		var domainErr *fault.DomainError
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(errors.As(err, &domainErr)).To(gomega.BeTrue())
		gomega.Expect(domainErr.Code).To(gomega.Equal(fault.CodeTimeoutError))
		gomega.Expect(attempts).To(gomega.Equal(5))
	})
})

func insertAccounts(ctx context.Context, tx pgx.Tx) (string, string) {
	ids := []string{uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		_, err := tx.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`, id, uuid.NewString(), uuid.NewString(), "12345678901234", time.Now())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	return ids[0], ids[1]
}

func insertReconciliationFixture(ctx context.Context, pool *pgxpool.Pool, sequence int64, direction string, amount, persistedBalance int64) (string, string, string) {
	accountID, transactionID, entryID := newIDV7(), newIDV7(), newIDV7()
	now := time.Now()
	_, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`,
		accountID, uuid.NewString(), uuid.NewString(), "12345678901234", now)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = pool.Exec(ctx, `
		INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', $4, 'BRL', $5, $5)`,
		transactionID, uuid.NewString(), "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", amount, now)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = pool.Exec(ctx, `
		INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'BRL', $8)`,
		entryID, accountID, transactionID, sequence, direction, amount, persistedBalance, now)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return accountID, transactionID, entryID
}

func deleteReconciliationFixture(ctx context.Context, pool *pgxpool.Pool, accountID, transactionID string) {
	_, err := pool.Exec(ctx, "DELETE FROM entries WHERE account_id = $1", accountID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func reconciliationSnapshot(ctx context.Context, pool *pgxpool.Pool, accountID string) string {
	var snapshot string
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(jsonb_agg(to_jsonb(a)), '[]'::jsonb)::text FROM accounts a WHERE a.id = $1) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(t)), '[]'::jsonb)::text FROM transactions t WHERE t.id IN (SELECT transaction_id FROM entries WHERE account_id = $1)) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY e.sequence_number), '[]'::jsonb)::text FROM entries e WHERE e.account_id = $1) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(o)), '[]'::jsonb)::text FROM outbox_events o WHERE o.aggregate_id IN (SELECT transaction_id::text FROM entries WHERE account_id = $1))`, accountID).Scan(&snapshot)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return snapshot
}

func reconciliationEntrySnapshot(ctx context.Context, pool *pgxpool.Pool, entryID string) string {
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM entries e WHERE e.id = $1`, entryID).Scan(&snapshot)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return snapshot
}

func reconciliationMismatchFor(report repository.ReconciliationReport, accountID string) (repository.ReconciliationMismatch, bool) {
	for _, mismatch := range report.Mismatches {
		if mismatch.AccountID == accountID {
			return mismatch, true
		}
	}
	return repository.ReconciliationMismatch{}, false
}

func insertAccount(ctx context.Context, pool *pgxpool.Pool) string {
	id := uuid.NewString()
	_, err := pool.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`, id, uuid.NewString(), uuid.NewString(), "12345678901234", time.Now())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return id
}

func insertAccountsForCommand(ctx context.Context, pool *pgxpool.Pool) (string, string) {
	ids := []string{uuid.NewString(), uuid.NewString()}
	for index, externalID := range ids {
		accountType := "ASSET"
		if index == 1 {
			accountType = "LIABILITY"
		}
		_, err := pool.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, 'BALANCE_NON_NEGATIVE', 'BRL', $6, $6)`, newIDV7(), externalID, fmt.Sprintf("%d", time.Now().UnixNano()+int64(index)), "52998224725", accountType, time.Now())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	return ids[0], ids[1]
}

func insertFundedAccountsForCommand(ctx context.Context, pool *pgxpool.Pool, balance, sequence int64) (string, string) {
	debitExternalID := uuid.NewString()
	creditExternalID := uuid.NewString()
	debitID := newIDV7()
	creditID := newIDV7()
	now := time.Now()
	for index, data := range []struct {
		id, externalID, accountType, policy string
	}{
		{id: debitID, externalID: debitExternalID, accountType: "LIABILITY", policy: "BALANCE_NON_NEGATIVE"},
		{id: creditID, externalID: creditExternalID, accountType: "ASSET", policy: "BALANCE_UNRESTRICTED"},
	} {
		_, err := pool.Exec(ctx, `
			INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, 'BRL', $7, $7)`,
			data.id, data.externalID, fmt.Sprintf("%d", now.UnixNano()+int64(index)), "52998224725", data.accountType, data.policy, now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	seedTransactionID := newIDV7()
	_, err := pool.Exec(ctx, `
		INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', $4, 'BRL', $5, $5)`,
		seedTransactionID, "seed-"+uuid.NewString(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", balance, now)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	for _, entry := range []struct {
		accountID, direction string
		balance              int64
	}{
		{accountID: debitID, direction: "CREDIT", balance: balance},
		{accountID: creditID, direction: "DEBIT", balance: -balance},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'BRL', $8)`,
			newIDV7(), entry.accountID, seedTransactionID, sequence, entry.direction, balance, entry.balance, now)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
	return debitExternalID, creditExternalID
}

type commandResult struct {
	output *dto.CreateTransactionOutput
	err    error
}

func newIntegrationCreateTransaction(pool *pgxpool.Pool) *command.CreateTransaction {
	return command.NewCreateTransaction(
		uow.NewUnitOfWork(pool),
		repository.NewAccountRepository(pool),
		repository.NewTransactionRepository(pool),
		repository.NewOutBoxRepository(pool),
		integrationTracer{}, integrationLogger{}, integrationMetrics{},
	)
}

func executeIntegrationTransaction(ctx context.Context, createTransaction *command.CreateTransaction, debitExternalID, creditExternalID, key string, amountValue int64) commandResult {
	currency := string(money.BRL)
	operation := string(transaction.OperationTransfer)
	amount := amountValue
	output, err := createTransaction.Execute(ctx, dto.CreateTransactionInput{
		IdempotencyKey:  &key,
		DebitAccountID:  &debitExternalID,
		CreditAccountID: &creditExternalID,
		Amount:          &amount,
		Currency:        &currency,
		Operation:       &operation,
	})
	return commandResult{output: output, err: err}
}

type integrationTracer struct{}

func (integrationTracer) Start(ctx context.Context, _ string) (context.Context, application.Span) {
	return ctx, integrationSpan{}
}

type integrationSpan struct{}

func (integrationSpan) End()                                 {}
func (integrationSpan) SpanContext() application.SpanContext { return integrationSpanContext{} }
func (integrationSpan) RecordError(error)                    {}

type integrationSpanContext struct{}

func (integrationSpanContext) TraceID() string { return "integration-trace" }

type integrationLogger struct{}

func (integrationLogger) DebugJSON(string, ...any)               {}
func (integrationLogger) InfoJSON(string, ...any)                {}
func (integrationLogger) WarnJSON(string, ...any)                {}
func (integrationLogger) ErrorJSON(string, ...any)               {}
func (integrationLogger) CriticalJSON(string, ...any)            {}
func (integrationLogger) DebugText(string, ...any)               {}
func (integrationLogger) InfoText(string, ...any)                {}
func (integrationLogger) WarnText(string, ...any)                {}
func (integrationLogger) ErrorText(string, ...any)               {}
func (integrationLogger) CriticalText(string, ...any)            {}
func (integrationLogger) WithTrace(context.Context) *slog.Logger { return slog.Default() }
func (integrationLogger) SlogJSON() *slog.Logger                 { return slog.Default() }
func (integrationLogger) SlogText() *slog.Logger                 { return slog.Default() }

type integrationMetrics struct{}

func (integrationMetrics) RecordRequestTotal(string, string, int)                {}
func (integrationMetrics) RecordDBQueryDuration(string, string, string, float64) {}
func (integrationMetrics) RecordRequestDuration(string, string, int, float64)    {}
func (integrationMetrics) RecordTransactionTotal(string)                         {}
func (integrationMetrics) RecordCommandTotal(string, string)                     {}
func (integrationMetrics) RecordCommandDuration(string, float64)                 {}
func (integrationMetrics) RecordIdempotencyTotal(string)                         {}
func (integrationMetrics) RecordConcurrencyRetry()                               {}
func (integrationMetrics) RecordOutboxTotal(string, string)                      {}

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

func criteriaForKey(key string) transaction.TransactionCriteria {
	return transaction.TransactionCriteria{IdempotencyKey: &key, WithEntries: true}
}

func assertLedgerRecords(ctx context.Context, pool *pgxpool.Pool, transactionID string, transactions, entries, outboxes int) {
	var transactionCount, entryCount, outboxCount int
	gomega.Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", transactionID).Scan(&transactionCount)).To(gomega.Succeed())
	gomega.Expect(pool.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1", transactionID).Scan(&entryCount)).To(gomega.Succeed())
	gomega.Expect(pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE aggregate_id = $1", transactionID).Scan(&outboxCount)).To(gomega.Succeed())
	gomega.Expect(transactionCount).To(gomega.Equal(transactions))
	gomega.Expect(entryCount).To(gomega.Equal(entries))
	gomega.Expect(outboxCount).To(gomega.Equal(outboxes))
}

type nopTx struct{ pgx.Tx }

func (nopTx) Rollback(context.Context) error { return nil }
