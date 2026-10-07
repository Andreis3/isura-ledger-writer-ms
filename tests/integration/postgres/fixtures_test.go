//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/entity"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/domain/transaction"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/gomega"
)

func insertAccounts(ctx context.Context, tx pgx.Tx) (string, string) {
	ids := []string{uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		_, err := tx.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`, id, uuid.NewString(), uuid.NewString(), "12345678901234", time.Now())
		Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())
	_, err = pool.Exec(ctx, `
		INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', $4, 'BRL', $5, $5)`,
		transactionID, uuid.NewString(), "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", amount, now)
	Expect(err).NotTo(HaveOccurred())
	_, err = pool.Exec(ctx, `
		INSERT INTO entries (id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, created_at)
		VALUES ($1, $2, $3, $4, 0, $5, $6, $7, 'BRL', $8)`,
		entryID, accountID, transactionID, sequence, direction, amount, persistedBalance, now)
	Expect(err).NotTo(HaveOccurred())
	return accountID, transactionID, entryID
}

func deleteReconciliationFixture(ctx context.Context, pool *pgxpool.Pool, accountID, transactionID string) {
	_, err := pool.Exec(ctx, "DELETE FROM entries WHERE account_id = $1", accountID)
	Expect(err).NotTo(HaveOccurred())
	_, err = pool.Exec(ctx, "DELETE FROM transactions WHERE id = $1", transactionID)
	Expect(err).NotTo(HaveOccurred())
	_, err = pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", accountID)
	Expect(err).NotTo(HaveOccurred())
}

func reconciliationSnapshot(ctx context.Context, pool *pgxpool.Pool, accountID string) string {
	var snapshot string
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(jsonb_agg(to_jsonb(a)), '[]'::jsonb)::text FROM accounts a WHERE a.id = $1) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(t)), '[]'::jsonb)::text FROM transactions t WHERE t.id IN (SELECT transaction_id FROM entries WHERE account_id = $1)) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY e.sequence_number), '[]'::jsonb)::text FROM entries e WHERE e.account_id = $1) || '|' ||
			(SELECT COALESCE(jsonb_agg(to_jsonb(o)), '[]'::jsonb)::text FROM outbox_events o WHERE o.aggregate_id IN (SELECT transaction_id::text FROM entries WHERE account_id = $1))`, accountID).Scan(&snapshot)
	Expect(err).NotTo(HaveOccurred())
	return snapshot
}

func reconciliationEntrySnapshot(ctx context.Context, pool *pgxpool.Pool, entryID string) string {
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM entries e WHERE e.id = $1`, entryID).Scan(&snapshot)
	Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())
	return id
}

func insertAccountV7(ctx context.Context, tx pgx.Tx) string {
	id := newIDV7()
	now := time.Now()
	_, err := tx.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`, id, uuid.NewString(), uuid.NewString(), "12345678901234", now)
	Expect(err).NotTo(HaveOccurred())
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
		Expect(err).NotTo(HaveOccurred())
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
		Expect(err).NotTo(HaveOccurred())
	}

	seedTransactionID := newIDV7()
	_, err := pool.Exec(ctx, `
		INSERT INTO transactions (id, idempotency_key, request_fingerprint, status, operation, amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, 'COMPLETED', 'TRANSFER', $4, 'BRL', $5, $5)`,
		seedTransactionID, "seed-"+uuid.NewString(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", balance, now)
	Expect(err).NotTo(HaveOccurred())
	for position, entry := range []struct {
		accountID, direction string
		balance              int64
	}{
		{accountID: debitID, direction: "CREDIT", balance: balance},
		{accountID: creditID, direction: "DEBIT", balance: -balance},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO entries (id, account_id, transaction_id, sequence_number, transaction_position, direction, amount, running_balance, currency, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'BRL', $9)`,
			newIDV7(), entry.accountID, seedTransactionID, sequence, position, entry.direction, balance, entry.balance, now)
		Expect(err).NotTo(HaveOccurred())
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
		adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{},
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

func newTransaction(accountA, accountB string) *transaction.Transaction {
	amount, err := money.NewMoney(1500, money.BRL)
	Expect(err).NotTo(HaveOccurred())
	id := newIDV7()
	debit, err := transaction.NewEntryBuilder().WithID(newIDV7()).WithAccountExternalID(uuid.NewString()).WithTransactionID(id).WithDirection(transaction.Debit).WithAmount(amount).Build()
	Expect(err).NotTo(HaveOccurred())
	credit, err := transaction.NewEntryBuilder().WithID(newIDV7()).WithAccountExternalID(uuid.NewString()).WithTransactionID(id).WithDirection(transaction.Credit).WithAmount(amount).Build()
	Expect(err).NotTo(HaveOccurred())
	debit.AssignAccountID(accountA)
	credit.AssignAccountID(accountB)
	entityTransaction, err := transaction.NewTransactionBuilder().WithID(id).WithIdempotencyKey(uuid.NewString()).WithFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").WithStatus(transaction.Pending).WithAmount(amount).WithOperation(transaction.OperationTransfer).WithEntries([]*transaction.Entry{debit, credit}).Build()
	Expect(err).NotTo(HaveOccurred())
	return entityTransaction
}

func newIDV7() string {
	id, err := entity.NewIDV7()
	Expect(err).NotTo(HaveOccurred())
	return id.String()
}

func newOutbox(transactionID string) *outbox.Outbox {
	entry, err := outbox.NewOutbox(transactionID, []byte(`{"transaction_id":"`+transactionID+`"}`))
	Expect(err).NotTo(HaveOccurred())
	return entry
}

func criteriaForKey(key string) transaction.TransactionCriteria {
	return transaction.TransactionCriteria{IdempotencyKey: &key, WithEntries: true}
}

func assertLedgerRecords(ctx context.Context, pool *pgxpool.Pool, transactionID string, transactions, entries, outboxes int) {
	var transactionCount, entryCount, outboxCount int
	Expect(pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE id = $1", transactionID).Scan(&transactionCount)).To(Succeed())
	Expect(pool.QueryRow(ctx, "SELECT count(*) FROM entries WHERE transaction_id = $1", transactionID).Scan(&entryCount)).To(Succeed())
	Expect(pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE aggregate_id = $1", transactionID).Scan(&outboxCount)).To(Succeed())
	Expect(transactionCount).To(Equal(transactions))
	Expect(entryCount).To(Equal(entries))
	Expect(outboxCount).To(Equal(outboxes))
}

type nopTx struct{ pgx.Tx }

func (nopTx) Rollback(context.Context) error { return nil }
