//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: REPOSITORY", Ordered, func() {
	var (
		ctx       context.Context
		container *postgres.PostgresContainer
		pool      *pgxpool.Pool
		tx        pgx.Tx
	)

	BeforeAll(func() {
		ctx = context.Background()
		var err error
		container, err = postgres.Run(ctx,
			"postgres:18-alpine",
			postgres.WithDatabase("isura_ledger_test"),
			postgres.WithUsername("admin"),
			postgres.WithPassword("admin"),
			postgres.BasicWaitStrategies(),
		)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			if pool != nil {
				pool.Close()
			}
			Expect(testcontainers.TerminateContainer(container)).To(Succeed())
		})

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

	BeforeEach(func() {
		var err error
		tx, err = pool.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(tx.Rollback(ctx)).To(Succeed())
	})

	It("should persist a balanced transaction and its outbox atomically", func() {
		accountA, accountB := insertAccounts(ctx, tx)
		entityTransaction := newTransaction(accountA, accountB)
		transactionRepo := repository.NewTransactionRepository(pool)
		outboxRepo := repository.NewOutBoxRepository(pool)
		txContext := database.WithTx(ctx, tx)

		Expect(transactionRepo.Save(txContext, entityTransaction)).To(Succeed())
		Expect(outboxRepo.Save(txContext, newOutbox(entityTransaction.ID.String()))).To(Succeed())

		var transactions, entries, outboxes int
		Expect(tx.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&transactions)).To(Succeed())
		Expect(tx.QueryRow(ctx, "SELECT count(*) FROM entries").Scan(&entries)).To(Succeed())
		Expect(tx.QueryRow(ctx, "SELECT count(*) FROM outbox_events").Scan(&outboxes)).To(Succeed())
		Expect(transactions).To(Equal(1))
		Expect(entries).To(Equal(2))
		Expect(outboxes).To(Equal(1))
	})

	Describe("ledger reconciliation", func() {
		It("should keep reconciliation on one snapshot while a balance is updated concurrently", func() {
			accountID := insertAccount(ctx, pool)
			transactionID := newIDV7()
			entryID := newIDV7()
			now := time.Now()
			_, err := pool.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 50, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", now)
			Expect(err).NotTo(HaveOccurred())
			_, err = pool.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, 1, 'DEBIT', 50, 50, 'BRL', $4)`, entryID, accountID, transactionID, now)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
				Expect(cleanupErr).NotTo(HaveOccurred())
			})

			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			})
			var snapshotBalance int64
			Expect(auditTx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE id = $1", entryID).Scan(&snapshotBalance)).To(Succeed())
			Expect(snapshotBalance).To(Equal(int64(50)))
			_, err = pool.Exec(ctx, "UPDATE entries SET running_balance = 51 WHERE id = $1", entryID)
			Expect(err).NotTo(HaveOccurred())

			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())
			Expect(report.EntriesChecked).To(Equal(1))
			Expect(report.Reconciled).To(BeTrue())
			Expect(report.MismatchCount).To(BeZero())
			Expect(auditTx.Commit(ctx)).To(Succeed())
		})

		It("should audit persisted entries without changing ledger records", func() {
			accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 125)
			DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

			before := reconciliationSnapshot(ctx, pool, accountID)
			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())
			Expect(auditTx.Rollback(ctx)).To(Succeed())

			Expect(report.AccountsChecked).To(BeNumerically(">=", 1))
			Expect(report.EntriesChecked).To(BeNumerically(">=", 1))
			Expect(report.Mismatches).NotTo(ContainElement(HaveField("AccountID", accountID)))
			Expect(reconciliationSnapshot(ctx, pool, accountID)).To(Equal(before))
		})

		It("should report the replayed and persisted balances for a PostgreSQL mismatch", func() {
			accountID, transactionID, _ := insertReconciliationFixture(ctx, pool, 1, "DEBIT", 125, 120)
			DeferCleanup(func() { deleteReconciliationFixture(ctx, pool, accountID, transactionID) })

			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				rollbackErr := auditTx.Rollback(ctx)
				Expect(rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed)).To(BeTrue())
			}()
			report, err := repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).NotTo(HaveOccurred())

			mismatch, found := reconciliationMismatchFor(report, accountID)
			Expect(found).To(BeTrue())
			Expect(mismatch.EntriesChecked).To(Equal(1))
			Expect(mismatch.ExpectedBalance).To(Equal(int64(125)))
			Expect(mismatch.PersistedBalance).To(Equal(int64(120)))
			Expect(mismatch.Currency).To(Equal("BRL"))
		})

		It("should return an explicit error for an entry referencing an unknown account", func() {
			transactionID := newIDV7()
			entryID := newIDV7()
			missingAccountID := newIDV7()
			now := time.Now()
			_, err := pool.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', 75, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", now)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_, cleanupErr := pool.Exec(ctx, "DELETE FROM entries WHERE id = $1", entryID)
				Expect(cleanupErr).NotTo(HaveOccurred())
				_, cleanupErr = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
				Expect(cleanupErr).NotTo(HaveOccurred())
			})
			fixtureTx, err := pool.Begin(ctx)
			Expect(err).NotTo(HaveOccurred())
			_, err = fixtureTx.Exec(ctx, "SET LOCAL session_replication_role = replica")
			Expect(err).NotTo(HaveOccurred())
			_, err = fixtureTx.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, 1, 'DEBIT', 75, 75, 'BRL', $4)`, entryID, missingAccountID, transactionID, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(fixtureTx.Commit(ctx)).To(Succeed())

			before := reconciliationEntrySnapshot(ctx, pool, entryID)
			auditTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			Expect(err).NotTo(HaveOccurred())
			_, err = repository.NewLedgerReconciliation(pool).Run(database.WithTx(ctx, auditTx))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown account"))
			Expect(auditTx.Rollback(ctx)).To(Succeed())
			Expect(reconciliationEntrySnapshot(ctx, pool, entryID)).To(Equal(before))
		})

		Describe("account persistence and historical balance backfill", func() {
		})

		It("should persist and read the account balance policy", func() {
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
			Expect(err).NotTo(HaveOccurred())

			repo := repository.NewAccountRepository(pool)
			txContext := database.WithTx(ctx, tx)
			Expect(repo.Save(txContext, entityAccount)).To(Succeed())

			found, err := repo.FindAccount(txContext, criteria.AccountCriteria{AccountExternalID: &externalID})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).NotTo(BeNil())
			Expect(found.BalancePolicy).To(Equal(account.BalanceNonNegative))
		})

		It("should recalculate historical balances using account nature and preserve sequences", func() {
			accountID := newIDV7()
			transactionID := newIDV7()
			now := time.Now()
			_, err := tx.Exec(ctx, `
			INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_NON_NEGATIVE', 'BRL', $5, $5)`,
				accountID, uuid.NewString(), fmt.Sprintf("%d", now.UnixNano()), "52998224725", now)
			Expect(err).NotTo(HaveOccurred())
			_, err = tx.Exec(ctx, `
			INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
			VALUES ($1, $2, $3, 'PENDING', 'TRANSFER', 100, 'BRL', $4, $4)`,
				transactionID, uuid.NewString(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now)
			Expect(err).NotTo(HaveOccurred())
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
				Expect(err).NotTo(HaveOccurred())
			}

			report, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
			Expect(err).NotTo(HaveOccurred())
			Expect(report.Accounts).To(Equal(1))
			Expect(report.Entries).To(Equal(2))
			Expect(report.UpdatedEntries).To(Equal(2))
			Expect(report.PersistedBalanceMismatches).To(Equal(2))
			Expect(report.SequenceMismatches).To(Equal(0))
			Expect(report.PolicyViolations).To(BeEmpty())
			Expect(report.ReadyForActivation).To(BeTrue())

			repeatedReport, err := repository.NewHistoricalBalanceBackfill(pool).Run(database.WithTx(ctx, tx))
			Expect(err).NotTo(HaveOccurred())
			Expect(repeatedReport.PersistedBalanceMismatches).To(Equal(0))
			Expect(repeatedReport.UpdatedEntries).To(Equal(0))
			Expect(repeatedReport.ReadyForActivation).To(BeTrue())

			rows, err := tx.Query(ctx, `SELECT sequence_number, running_balance FROM entries WHERE account_id = $1 ORDER BY sequence_number`, accountID)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()
			var balances []int64
			for rows.Next() {
				var sequence, balance int64
				Expect(rows.Scan(&sequence, &balance)).To(Succeed())
				balances = append(balances, balance)
			}
			Expect(rows.Err()).NotTo(HaveOccurred())
			Expect(balances).To(Equal([]int64{100, 60}))
		})

		Describe("transaction execution, idempotency, and atomicity", func() {
		})

		It("should commit or roll back transaction, entries, and outbox as one unit", func() {
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
			Expect(err).NotTo(HaveOccurred())

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
			Expect(err).To(MatchError(expectedFailure))

			assertLedgerRecords(ctx, pool, rolledBack.ID.String(), 0, 0, 0)
		})

		It("should reject insufficient balance before creating ledger records", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			_, err := tx.Exec(ctx, `UPDATE accounts SET balance_policy = 'BALANCE_NON_NEGATIVE' WHERE id IN ($1, $2)`, accountA, accountB)
			Expect(err).NotTo(HaveOccurred())

			entityTransaction := newTransaction(accountA, accountB)
			err = repository.NewTransactionRepository(pool).Save(database.WithTx(ctx, tx), entityTransaction)

			Expect(errors.Is(err, fault.ErrInsufficientBalance)).To(BeTrue())
			assertLedgerRecords(ctx, pool, entityTransaction.ID.String(), 0, 0, 0)
			Expect(entityTransaction.Entries[0].SequenceNumber).To(Equal(int64(0)))
			Expect(entityTransaction.Entries[1].SequenceNumber).To(Equal(int64(0)))
		})

		It("should return the same transaction and entries for an idempotency replay", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			original := newTransaction(accountA, accountB)
			repo := repository.NewTransactionRepository(pool)
			txContext := database.WithTx(ctx, tx)
			Expect(repo.Save(txContext, original)).To(Succeed())

			key := original.IdempotencyKey
			replayed, err := repo.Find(txContext, criteriaForKey(key))
			Expect(err).NotTo(HaveOccurred())
			Expect(replayed.ID).To(Equal(original.ID))
			Expect(replayed.Fingerprint).To(Equal(original.Fingerprint))
			Expect(replayed.Entries).To(HaveLen(2))

			id := original.ID.String()
			byID, err := repo.Find(txContext, transaction.TransactionCriteria{ID: &id, WithEntries: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(byID.ID).To(Equal(original.ID))
			Expect(byID.Entries).To(HaveLen(2))
		})

		It("should serialize concurrent requests with the same idempotency key", func() {
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
				adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{},
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
				Expect(err).NotTo(HaveOccurred())
			}
			for result := range results {
				Expect(result).NotTo(BeNil())
				if result.IdempotentReplay {
					replays++
				}
				if transactionID == "" {
					transactionID = *result.TransactionID
				}
				Expect(*result.TransactionID).To(Equal(transactionID))
			}
			Expect(replays).To(Equal(1))

			var count int
			Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE idempotency_key = $1", key).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})

		It("should approve only one of two concurrent debits that consume the available balance", func() {
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
				Expect(errors.Is(result.err, fault.ErrInsufficientBalance)).To(BeTrue(), "unexpected concurrent result: %v", result.err)
				insufficientBalance++
			}
			Expect(successes).To(Equal(1))
			Expect(insufficientBalance).To(Equal(1))

			var sourceEntries, sourceSequence, sourceBalance int64
			Expect(pool.QueryRow(ctx, `
			SELECT count(*), max(sequence_number)
			FROM entries e JOIN accounts a ON a.id = e.account_id
			WHERE a.account_external_id = $1`, debitExternalID).
				Scan(&sourceEntries, &sourceSequence)).To(Succeed())
			Expect(pool.QueryRow(ctx, `
			SELECT e.running_balance
			FROM entries e JOIN accounts a ON a.id = e.account_id
			WHERE a.account_external_id = $1
			ORDER BY e.sequence_number DESC
			LIMIT 1`, debitExternalID).Scan(&sourceBalance)).To(Succeed())
			Expect(sourceEntries).To(Equal(int64(2)))
			Expect(sourceSequence).To(Equal(int64(11)))
			Expect(sourceBalance).To(Equal(int64(0)))

			var createdTransactions int
			Expect(pool.QueryRow(ctx, `
			SELECT count(*) FROM transactions
			WHERE idempotency_key LIKE 'cd-%'`).Scan(&createdTransactions)).To(Succeed())
			Expect(createdTransactions).To(Equal(1))
		})

		It("should replay a successful transaction without creating another entry", func() {
			debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 100, 1)
			createTransaction := newIntegrationCreateTransaction(pool)
			key := "replay-" + uuid.NewString()

			first := executeIntegrationTransaction(ctx, createTransaction, debitExternalID, creditExternalID, key, 100)
			Expect(first.err).NotTo(HaveOccurred())
			Expect(first.output).NotTo(BeNil())
			Expect(first.output.IdempotentReplay).To(BeFalse())

			second := executeIntegrationTransaction(ctx, createTransaction, debitExternalID, creditExternalID, key, 100)
			Expect(second.err).NotTo(HaveOccurred())
			Expect(second.output).NotTo(BeNil())
			Expect(second.output.IdempotentReplay).To(BeTrue())
			Expect(second.output.TransactionID).To(Equal(first.output.TransactionID))

			var transactions, entries, outboxes int
			Expect(pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE idempotency_key = $1`, key).Scan(&transactions)).To(Succeed())
			Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE transaction_id = $1`, *first.output.TransactionID).Scan(&entries)).To(Succeed())
			Expect(pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, *first.output.TransactionID).Scan(&outboxes)).To(Succeed())
			Expect(transactions).To(Equal(1))
			Expect(entries).To(Equal(2))
			Expect(outboxes).To(Equal(1))
		})

		It("should roll back transaction, entries and outbox together", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			entityTransaction := newTransaction(accountA, accountB)
			txContext := database.WithTx(ctx, tx)
			Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(Succeed())
			Expect(repository.NewOutBoxRepository(pool).Save(txContext, newOutbox(entityTransaction.ID.String()))).To(Succeed())
			Expect(tx.Rollback(ctx)).To(Succeed())

			var count int
			Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", entityTransaction.ID.String()).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(0))
			tx = nopTx{}
		})

		Describe("database constraints and repository contracts", func() {
		})

		It("should enforce positive amounts and valid directions in the database", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			_, err := tx.Exec(ctx, `INSERT INTO entries (id, account_id, transaction_id, sequence_number, direction, amount, running_balance, currency, created_at) VALUES ($1, $2, $3, 1, 'INVALID', 0, 0, 'BRL', $4)`, uuid.NewString(), accountA, uuid.NewString(), time.Now())
			Expect(err).To(HaveOccurred())
			_ = accountB
		})

		It("should apply the unique sequence index per account", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			entityTransaction := newTransaction(accountA, accountB)
			txContext := database.WithTx(ctx, tx)
			Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(Succeed())

			var indexDefinition string
			Expect(tx.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'entries' AND indexname = 'unique_entry_sequence_number'`).Scan(&indexDefinition)).To(Succeed())
			Expect(indexDefinition).To(ContainSubstring("UNIQUE"))
			Expect(indexDefinition).To(ContainSubstring("account_id, sequence_number"))

			_, err := tx.Exec(ctx, `
			INSERT INTO entries (
				id, account_id, transaction_id, sequence_number, direction, amount,
				running_balance, currency, created_at
			) VALUES ($1, $2, $3, 1, 'DEBIT', 1500, -1500, 'BRL', $4)
		`, uuid.NewString(), accountA, entityTransaction.ID.String(), time.Now())
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue())
			Expect(pgErr.ConstraintName).To(Equal("unique_entry_sequence_number"))
		})

		It("should reconstruct a transfer from its append-only entries", func() {
			accountA, accountB := insertAccounts(ctx, tx)
			entityTransaction := newTransaction(accountA, accountB)
			txContext := database.WithTx(ctx, tx)
			Expect(repository.NewTransactionRepository(pool).Save(txContext, entityTransaction)).To(Succeed())

			var debit, credit int64
			Expect(tx.QueryRow(ctx, "SELECT COALESCE(sum(amount) FILTER (WHERE direction = 'DEBIT'), 0), COALESCE(sum(amount) FILTER (WHERE direction = 'CREDIT'), 0) FROM entries WHERE transaction_id = $1", entityTransaction.ID.String()).Scan(&debit, &credit)).To(Succeed())
			Expect(debit).To(Equal(credit))

			var debitBalance, creditBalance int64
			Expect(tx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE account_id = $1", accountA).Scan(&debitBalance)).To(Succeed())
			Expect(tx.QueryRow(ctx, "SELECT running_balance FROM entries WHERE account_id = $1", accountB).Scan(&creditBalance)).To(Succeed())
			Expect(debitBalance).To(Equal(int64(1500)))
			Expect(creditBalance).To(Equal(int64(-1500)))
		})

		It("should not expose mutation methods for confirmed ledger facts", func() {
			typeOfRepository := reflect.TypeOf(repository.NewTransactionRepository(pool))
			_, hasUpdate := typeOfRepository.MethodByName("Update")
			_, hasDelete := typeOfRepository.MethodByName("Delete")
			Expect(hasUpdate).To(BeFalse())
			Expect(hasDelete).To(BeFalse())
		})

		It("should read accounts without acquiring a pessimistic lock", func() {
			externalID, _ := insertAccountsForCommand(ctx, pool)
			blocker, err := pool.Begin(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer blocker.Rollback(ctx)
			_, err = blocker.Exec(ctx, "SELECT id FROM accounts WHERE account_external_id = $1 FOR UPDATE", externalID)
			Expect(err).NotTo(HaveOccurred())

			readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			defer cancel()
			found, err := repository.NewAccountRepository(pool).FindAccount(readCtx, criteria.AccountCriteria{
				AccountExternalID: &externalID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(found).NotTo(BeNil())
		})

		Describe("concurrent ledger operations", func() {
		})

		It("should assign unique monotonic sequences for concurrent transfers sharing an account", func() {
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
				Expect(errors.As(err, &pgErr)).To(BeTrue())
				Expect(pgErr.ConstraintName).To(Equal("unique_entry_sequence_number"))
			}
			Expect(successes).To(BeNumerically(">=", 1))

			rows, err := pool.Query(ctx, `SELECT sequence_number FROM entries WHERE account_id = $1 ORDER BY sequence_number`, sharedAccount)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()
			sequences := make([]int64, 0, 2)
			for rows.Next() {
				var sequence int64
				Expect(rows.Scan(&sequence)).To(Succeed())
				sequences = append(sequences, sequence)
			}
			Expect(rows.Err()).NotTo(HaveOccurred())
			Expect(sequences).NotTo(BeEmpty())
			Expect(sequences[0]).To(Equal(int64(1)))
			if len(sequences) == 2 {
				Expect(sequences[1]).To(Equal(int64(2)))
			}
		})

		It("should return ILMS-2004 after exhausting serialization retries", func() {
			unitOfWork := uow.NewUnitOfWork(pool)
			attempts := 0

			err := unitOfWork.WithRetryableTransaction(ctx, func(context.Context) error {
				attempts++
				return &pgconn.PgError{Code: "40001", Message: "serialization failure"}
			})

			var domainErr *fault.DomainError
			Expect(err).To(HaveOccurred())
			Expect(errors.As(err, &domainErr)).To(BeTrue())
			Expect(domainErr.Code).To(Equal(fault.CodeTimeoutError))
			Expect(attempts).To(Equal(5))
		})
	})

})
