//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	command "github.com/andreis3/isura-ledger-ms/internal/application/command"
	dto "github.com/andreis3/isura-ledger-ms/internal/application/dto"
	fault "github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	money "github.com/andreis3/isura-ledger-ms/internal/domain/money"
	transaction "github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	database "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	repository "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	uow "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	uuid "github.com/google/uuid"
	pgconn "github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"reflect"
	"sync"
	"time"
)

var _ = Describe("INTEGRATION :: INFRA :: POSTGRES :: TRANSACTION REPOSITORY", func() {
	Context("database behavior", func() {
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
