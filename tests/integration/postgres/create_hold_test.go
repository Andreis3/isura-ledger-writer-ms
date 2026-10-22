//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/account"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE HOLD (POSTGRES)", func() {
	It("should persist one instruction and one event for concurrent identical requests", func() {
		accountID := insertEligibleCommandAccount(ctx, pool)
		holds := repository.NewHoldRepository(pool)
		sut := command.NewCreateHold(uow.NewUnitOfWork(pool), repository.NewAccountRepository(pool), repository.NewTransactionRepository(pool), holds, nil, repository.NewOutBoxRepository(pool))
		input := dto.CreateHoldInput{AccountID: accountID, Currency: string(money.BRL), Origin: "concurrent-origin-" + uuid.NewString(), Reference: "reference", Type: hold.Judicial, Requested: amount(500, money.BRL)}
		start := make(chan struct{})
		outputs := make([]*command.CreateHoldOutput, 2)
		errors := make([]error, 2)
		var wait sync.WaitGroup
		for index := range outputs {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				<-start
				outputs[index], errors[index] = sut.Execute(context.Background(), input)
			}(index)
		}
		close(start)
		wait.Wait()
		Expect(errors[0]).NotTo(HaveOccurred())
		Expect(errors[1]).NotTo(HaveOccurred())
		Expect(outputs[0].Instruction.ID).To(Equal(outputs[1].Instruction.ID))
		Expect(outputs[0].IdempotentReplay || outputs[1].IdempotentReplay).To(BeTrue())
		var holdsCount, historyCount, eventCount int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM holds WHERE origin=$1 AND reference=$2`, input.Origin, input.Reference).Scan(&holdsCount)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1`, outputs[0].Instruction.ID).Scan(&historyCount)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1`, outputs[0].Instruction.ID).Scan(&eventCount)).To(Succeed())
		Expect(holdsCount).To(Equal(1))
		Expect(historyCount).To(Equal(1))
		Expect(eventCount).To(Equal(1))
		var transactionCount, entryCount int
		Expect(pool.QueryRow(ctx, `SELECT count(DISTINCT t.id) FROM transactions t JOIN entries e ON e.transaction_id=t.id WHERE e.account_id=$1`, accountID).Scan(&transactionCount)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE account_id=$1`, accountID).Scan(&entryCount)).To(Succeed())
		Expect(transactionCount).To(BeZero())
		Expect(entryCount).To(BeZero())
	})

	It("should prevent a concurrent outgoing transaction and hold from exceeding the same funds", func() {
		debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 200000, 1)
		var debitAccountID string
		Expect(pool.QueryRow(ctx, `SELECT id FROM accounts WHERE account_external_id=$1`, debitExternalID).Scan(&debitAccountID)).To(Succeed())
		Expect(pool.Exec(ctx, `UPDATE accounts SET holds_eligible=true WHERE id=$1`, debitAccountID)).Error().NotTo(HaveOccurred())
		holds := repository.NewHoldRepository(pool)
		createHold := command.NewCreateHold(uow.NewUnitOfWork(pool), repository.NewAccountRepository(pool), repository.NewTransactionRepository(pool), holds, fixedHoldPolicy{}, repository.NewOutBoxRepository(pool))
		input := dto.CreateHoldInput{AccountID: debitAccountID, Currency: string(money.BRL), Origin: "concurrent-funds-" + uuid.NewString(), Reference: "reference", Type: hold.Judicial, Requested: amount(150000, money.BRL)}
		createTransaction := newIntegrationCreateTransaction(pool)
		transactionInput := outgoingTransactionInput(debitExternalID, creditExternalID, 150000)
		start := make(chan struct{})
		var holdOutput *command.CreateHoldOutput
		var holdErr error
		var transactionOutput commandResult
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			holdOutput, holdErr = createHold.Execute(context.Background(), input)
		}()
		go func() {
			defer wait.Done()
			<-start
			transactionOutput.output, transactionOutput.err = createTransaction.Execute(context.Background(), transactionInput)
		}()
		close(start)
		wait.Wait()
		Expect(holdErr).NotTo(HaveOccurred())
		Expect(holdOutput).NotTo(BeNil())
		Expect(transactionOutput.err == nil || errors.Is(transactionOutput.err, fault.ErrInsufficientBalance)).To(BeTrue(), "unexpected outgoing transaction error: %v", transactionOutput.err)
		_, ledgerBalance, err := repository.NewTransactionRepository(pool).FindLatestLedgerState(ctx, debitAccountID)
		Expect(err).NotTo(HaveOccurred())
		var retained int64
		Expect(pool.QueryRow(ctx, `SELECT COALESCE(sum(allocated_amount-released_amount),0) FROM holds WHERE account_id=$1`, debitAccountID).Scan(&retained)).To(Succeed())
		Expect(ledgerBalance).To(BeNumerically(">=", retained))
		Expect(ledgerBalance).To(BeNumerically(">=", 0))
	})
})

type fixedHoldPolicy struct{}

func (fixedHoldPolicy) PolicyFor(context.Context, account.Account, hold.HoldType) (hold.AllocationPolicy, bool, error) {
	return hold.AllocationPolicy{Version: "integration-v1", TypeOrder: []hold.HoldType{hold.Judicial}, AllowPartial: true, UnresolvedConflict: hold.PendingReviewOnConflict}, true, nil
}

func outgoingTransactionInput(debitExternalID, creditExternalID string, amount int64) dto.CreateTransactionInput {
	key := "out-" + uuid.NewString()
	operation := "TRANSFER"
	return dto.CreateTransactionInput{
		IdempotencyKey: &key, Amount: &amount, Operation: &operation,
		Entries: []dto.EntryInput{
			{AccountID: creditExternalID, Direction: "CREDIT", Amount: amount, Currency: string(money.BRL)},
			{AccountID: debitExternalID, Direction: "DEBIT", Amount: amount, Currency: string(money.BRL)},
		},
		Movements: []dto.MovementInput{{Type: "TRANSFER", Description: "concurrent hold protection", EntryPositions: []int{0, 1}}},
	}
}

func insertEligibleCommandAccount(ctx context.Context, pool *pgxpool.Pool) string {
	id := newIDV7()
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, `INSERT INTO accounts (id, account_external_id, account_number, tax_id, status, type, balance_policy, currency, created_at, updated_at) VALUES ($1, $2, $3, $4, 'ACTIVE', 'ASSET', 'BALANCE_UNRESTRICTED', 'BRL', $5, $5)`, id, uuid.NewString(), "1234567890123456", "52998224725", now)
	Expect(err).NotTo(HaveOccurred())
	return id
}
