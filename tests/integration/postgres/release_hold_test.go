//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/andreis3/isura-ledger-ms/internal/application"
	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RELEASE HOLD (POSTGRES)", func() {
	It("should leave transactions and entries unchanged when a hold is registered and released", func() {
		accountID := insertAccount(ctx, pool)
		instruction := allocatedHold(accountID, "accounting-invariant", "reference-"+newIDV7())
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, repository.NewOutBoxRepository(pool), adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		input := dto.ReleaseHoldInput{HoldID: instruction.ID, IdempotencyKey: "release-" + newIDV7(), AuthorizationReference: "auth-ref", Amount: amount(300, money.BRL)}
		_, err := sut.Execute(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		var transactions, entries int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE id IN (SELECT transaction_id FROM entries WHERE account_id=$1)`, accountID).Scan(&transactions)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE account_id=$1`, accountID).Scan(&entries)).To(Succeed())
		Expect(transactions).To(BeZero())
		Expect(entries).To(BeZero())
		loaded, err := holdRepo.FindByID(ctx, instruction.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Released.Amount()).To(Equal(int64(300)))
	})

	It("should apply one release for concurrent retries with the same idempotency key", func() {
		accountID := insertAccount(ctx, pool)
		instruction := allocatedHold(accountID, "release-command", "reference-"+newIDV7())
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, repository.NewOutBoxRepository(pool), adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		input := dto.ReleaseHoldInput{HoldID: instruction.ID, IdempotencyKey: "release-" + newIDV7(), AuthorizationReference: "auth-ref", Amount: amount(300, money.BRL)}

		start := make(chan struct{})
		results := make([]*command.ReleaseHoldOutput, 2)
		errors := make([]error, 2)
		var wait sync.WaitGroup
		for index := range results {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				<-start
				results[index], errors[index] = sut.Execute(context.Background(), input)
			}(index)
		}
		close(start)
		wait.Wait()
		Expect(errors[0]).NotTo(HaveOccurred())
		Expect(errors[1]).NotTo(HaveOccurred())
		Expect(results[0].IdempotentReplay || results[1].IdempotentReplay).To(BeTrue())

		loaded, err := holdRepo.FindByID(ctx, instruction.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Released.Amount()).To(Equal(int64(300)))
		var releases, histories, events int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_releases WHERE idempotency_key=$1`, input.IdempotencyKey).Scan(&releases)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1 AND event_type='RELEASE_RECORDED'`, instruction.ID).Scan(&histories)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type=$2`, instruction.ID, outbox.HoldReleased).Scan(&events)).To(Succeed())
		Expect(releases).To(Equal(1))
		Expect(histories).To(Equal(1))
		Expect(events).To(Equal(1))
	})

	It("should serialize an outgoing transaction with an authorized hold release", func() {
		debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 200000, 1)
		var debitAccountID string
		Expect(pool.QueryRow(ctx, `SELECT id FROM accounts WHERE account_external_id=$1`, debitExternalID).Scan(&debitAccountID)).To(Succeed())
		Expect(pool.Exec(ctx, `UPDATE accounts SET holds_eligible=true WHERE id=$1`, debitAccountID)).Error().NotTo(HaveOccurred())

		instruction := allocatedHold(debitAccountID, "release-race", "reference-"+newIDV7())
		instruction.Requested = amount(100000, money.BRL)
		instruction.Allocated = amount(100000, money.BRL)
		instruction.Pending = amount(0, money.BRL)
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		barrierRepo := &holdReadBarrierRepository{
			Repository: holdRepo, effectiveRead: make(chan struct{}, 1), releaseRead: make(chan struct{}, 1),
			continueReads: make(chan struct{}), holdID: instruction.ID,
		}

		releaseCommand := command.NewReleaseHold(
			uow.NewUnitOfWork(pool, nil), barrierRepo, repository.NewOutBoxRepository(pool),
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{},
		)
		releaseInput := dto.ReleaseHoldInput{
			HoldID: instruction.ID, IdempotencyKey: "release-race-" + newIDV7(),
			AuthorizationReference: "authorized-release", Amount: amount(100000, money.BRL),
		}
		transactionInput := outgoingTransactionInput(debitExternalID, creditExternalID, 150000)
		createTransaction := command.NewCreateTransaction(
			uow.NewUnitOfWork(pool), repository.NewAccountRepository(pool), repository.NewTransactionRepository(pool),
			repository.NewOutBoxRepository(pool), adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{},
			adaptermocks.SilentMetricsMock{}, application.DefaultMaxTransactionEntries, barrierRepo,
		)

		traceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var releaseOutput *command.ReleaseHoldOutput
		var releaseErr error
		var transactionOutput commandResult
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			releaseOutput, releaseErr = releaseCommand.Execute(traceCtx, releaseInput)
		}()
		go func() {
			defer wait.Done()
			transactionOutput.output, transactionOutput.err = createTransaction.Execute(traceCtx, transactionInput)
		}()
		releaseBarrier := sync.OnceFunc(func() { close(barrierRepo.continueReads) })
		defer releaseBarrier()
		readsReady := true
		for _, read := range []<-chan struct{}{barrierRepo.effectiveRead, barrierRepo.releaseRead} {
			select {
			case <-read:
			case <-traceCtx.Done():
				readsReady = false
			}
		}
		releaseBarrier()
		wait.Wait()
		Expect(readsReady).To(BeTrue(), "timed out waiting for both transactions to read the effective hold")

		Expect(releaseErr).NotTo(HaveOccurred())
		Expect(releaseOutput).NotTo(BeNil())
		Expect(transactionOutput.output).To(BeNil())
		Expect(errors.Is(transactionOutput.err, fault.ErrInsufficientBalance)).To(BeTrue(), "unexpected outgoing transaction error: %v", transactionOutput.err)
		loaded, err := holdRepo.FindByID(ctx, instruction.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(loaded.Released.Amount()).To(Equal(int64(100000)))
		_, ledgerBalance, err := repository.NewTransactionRepository(pool).FindLatestLedgerState(ctx, debitAccountID)
		Expect(err).NotTo(HaveOccurred())
		Expect(ledgerBalance).To(Equal(int64(200000)))
	})

	It("should roll back the release and history when outbox persistence fails", func() {
		accountID := insertAccount(ctx, pool)
		instruction := allocatedHold(accountID, "release-rollback", "reference-"+newIDV7())
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, failingHoldOutbox{}, adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		input := dto.ReleaseHoldInput{HoldID: instruction.ID, IdempotencyKey: "release-" + newIDV7(), AuthorizationReference: "auth-ref", Amount: amount(300, money.BRL)}

		_, err := sut.Execute(ctx, input)

		Expect(err).To(HaveOccurred())
		loaded, findErr := holdRepo.FindByID(ctx, instruction.ID)
		Expect(findErr).NotTo(HaveOccurred())
		Expect(loaded.Released.Amount()).To(BeZero())
		var releases, histories int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_releases WHERE idempotency_key=$1`, input.IdempotencyKey).Scan(&releases)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1 AND event_type='RELEASE_RECORDED'`, instruction.ID).Scan(&histories)).To(Succeed())
		Expect(releases).To(BeZero())
		Expect(histories).To(BeZero())
	})
})

type failingHoldOutbox struct{ outbox.Repository }

func (failingHoldOutbox) Save(context.Context, *outbox.Outbox) error {
	return errors.New("outbox unavailable")
}

type holdReadBarrierRepository struct {
	hold.Repository
	effectiveRead chan struct{}
	releaseRead   chan struct{}
	continueReads chan struct{}
	holdID        string
}

func (r *holdReadBarrierRepository) FindEffectiveByAccount(ctx context.Context, accountID string, currency money.Currency) ([]hold.Instruction, error) {
	instructions, err := r.Repository.FindEffectiveByAccount(ctx, accountID, currency)
	if err != nil {
		return nil, err
	}
	r.effectiveRead <- struct{}{}
	<-r.continueReads
	return instructions, nil
}

func (r *holdReadBarrierRepository) FindByID(ctx context.Context, id string) (*hold.Instruction, error) {
	instruction, err := r.Repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if id == r.holdID {
		r.releaseRead <- struct{}{}
		<-r.continueReads
	}
	return instruction, nil
}
