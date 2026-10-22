//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/application/security"
	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/domain/hold"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	adaptermocks "github.com/andreis3/isura-ledger-ms/tests/mocks/infra/adapter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RESOLVE PENDING HOLD (POSTGRES)", func() {
	It("should commit the decision, history and versioned outbox event in one transaction", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		beforeEntries := entryCountForAccount(accountID)
		sut := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		input := rejectDecision(instruction, "decision-"+uuid.NewString())

		result, err := sut.Execute(authorizedResolutionContext(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Decision).NotTo(BeNil())
		Expect(result.Instruction.Status).To(Equal(hold.StatusRejected))
		Expect(result.Instruction.Version).To(Equal(uint64(2)))
		assertResolutionRows(instruction.ID, input.DecisionID, 1, 1, 1)
		var eventType, payload string
		Expect(pool.QueryRow(ctx, `SELECT event_type, payload::text FROM outbox_events WHERE aggregate_id=$1`, instruction.ID).Scan(&eventType, &payload)).To(Succeed())
		Expect(eventType).To(Equal(string(outbox.HoldDecisionResolved)))
		var event map[string]any
		Expect(json.Unmarshal([]byte(payload), &event)).To(Succeed())
		Expect(event["decision_id"]).To(Equal(input.DecisionID))
		Expect(event["event_version"]).To(BeNumerically("==", 1))
		Expect(event).NotTo(HaveKey("reason"))
		Expect(entryCountForAccount(accountID)).To(Equal(beforeEntries))
	})

	It("should roll back the state and history when outbox persistence fails", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		input := rejectDecision(instruction, "decision-"+uuid.NewString())
		sut := integrationResolveHold(holds, failingResolutionOutbox{repository.NewOutBoxRepository(pool)})

		_, err := sut.Execute(authorizedResolutionContext(), input)

		Expect(err).To(HaveOccurred())
		loaded, loadErr := holds.FindByID(ctx, instruction.ID)
		Expect(loadErr).NotTo(HaveOccurred())
		Expect(loaded.Status).To(Equal(hold.StatusPendingReview))
		Expect(loaded.Version).To(Equal(uint64(1)))
		assertResolutionRows(instruction.ID, input.DecisionID, 0, 0, 0)
	})

	It("should replay the original result and reject a conflicting fingerprint", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		sut := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		input := rejectDecision(instruction, "decision-"+uuid.NewString())
		first, err := sut.Execute(authorizedResolutionContext(), input)
		Expect(err).NotTo(HaveOccurred())

		replay, err := sut.Execute(authorizedResolutionContext(), input)
		Expect(err).NotTo(HaveOccurred())
		Expect(replay.IdempotentReplay).To(BeTrue())
		Expect(replay.Decision.ID).To(Equal(first.Decision.ID))
		Expect(replay.Decision.ResultStatus).To(Equal(string(hold.StatusRejected)))
		assertResolutionRows(instruction.ID, input.DecisionID, 1, 1, 1)

		input.Reason = "materially different"
		_, err = sut.Execute(authorizedResolutionContext(), input)
		Expect(errors.Is(err, hold.ErrDecisionConflict)).To(BeTrue())
		assertResolutionRows(instruction.ID, input.DecisionID, 1, 1, 1)
	})

	It("should recover a committed decision when the commit acknowledgement is lost", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		injected := false
		begin := func(ctx context.Context) (pgx.Tx, error) {
			realTx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
			if err != nil {
				return nil, err
			}
			if !injected {
				injected = true
				return &acknowledgedCommitLostTx{Tx: realTx}, nil
			}
			return realTx, nil
		}
		sut := command.NewResolvePendingHold(uow.NewUnitOfWorkWithBegin(begin), repository.NewAccountRepository(pool),
			repository.NewTransactionRepository(pool), holds, fixedHoldPolicy{}, repository.NewOutBoxRepository(pool),
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		input := rejectDecision(instruction, "decision-"+uuid.NewString())

		result, err := sut.Execute(authorizedResolutionContext(), input)

		Expect(err).NotTo(HaveOccurred())
		Expect(injected).To(BeTrue())
		Expect(result.IdempotentReplay).To(BeTrue())
		Expect(result.Decision.IdempotencyKey).To(Equal(input.DecisionID))
		Expect(result.Instruction.Status).To(Equal(hold.StatusRejected))
		assertResolutionRows(instruction.ID, input.DecisionID, 1, 1, 1)
	})

	It("should persist one result for concurrent identical decisions", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		sut := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		input := rejectDecision(instruction, "decision-"+uuid.NewString())
		start := make(chan struct{})
		results := make([]*dto.ResolvePendingHoldOutput, 2)
		errorsFound := make([]error, 2)
		var wait sync.WaitGroup
		for index := range results {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				<-start
				results[index], errorsFound[index] = sut.Execute(authorizedResolutionContext(), input)
			}(index)
		}
		close(start)
		wait.Wait()
		Expect(errorsFound[0]).NotTo(HaveOccurred())
		Expect(errorsFound[1]).NotTo(HaveOccurred())
		Expect(results[0].Decision.ID).To(Equal(results[1].Decision.ID))
		Expect(results[0].IdempotentReplay || results[1].IdempotentReplay).To(BeTrue())
		assertResolutionRows(instruction.ID, input.DecisionID, 1, 1, 1)
	})

	It("should reject a concurrent decision that reuses a key with a different fingerprint", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		sut := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		inputs := make([]dto.ResolvePendingHoldInput, 2)
		inputs[0] = rejectDecision(instruction, "decision-"+uuid.NewString())
		inputs[1] = inputs[0]
		inputs[1].Reason = "different reason"
		start := make(chan struct{})
		errorsFound := make([]error, 2)
		var wait sync.WaitGroup
		for index := range inputs {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				<-start
				_, errorsFound[index] = sut.Execute(authorizedResolutionContext(), inputs[index])
			}(index)
		}
		close(start)
		wait.Wait()
		Expect((errorsFound[0] == nil) != (errorsFound[1] == nil)).To(BeTrue())
		conflict := errorsFound[0]
		if conflict == nil {
			conflict = errorsFound[1]
		}
		Expect(errors.Is(conflict, hold.ErrDecisionConflict)).To(BeTrue())
		assertResolutionRows(instruction.ID, inputs[0].DecisionID, 1, 1, 1)
	})

	It("should serialize a hold resolution with a concurrent debit", func() {
		debitExternalID, creditExternalID := insertFundedAccountsForCommand(ctx, pool, 200000, 1)
		var accountID string
		Expect(pool.QueryRow(ctx, `SELECT id FROM accounts WHERE account_external_id=$1`, debitExternalID).Scan(&accountID)).To(Succeed())
		Expect(pool.Exec(ctx, `UPDATE accounts SET holds_eligible=true WHERE id=$1`, accountID)).Error().NotTo(HaveOccurred())
		instruction := pendingHoldFixture(accountID)
		instruction.Requested, instruction.Pending = amount(150000, money.BRL), amount(150000, money.BRL)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		resolve := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		resolution := dto.ResolvePendingHoldInput{HoldID: instruction.ID, DecisionID: "decision-" + uuid.NewString(),
			AuthorizationReference: "authorization", Reason: "approved", Action: hold.ActionAllocate,
			PolicyVersion: "integration-v1", ExpectedVersion: 1, RequestedAmount: amount(150000, money.BRL)}
		debit := newIntegrationCreateTransaction(pool)
		transactionInput := outgoingTransactionInput(debitExternalID, creditExternalID, 150000)
		var resolveErr, debitErr error
		var wait sync.WaitGroup
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, resolveErr = resolve.Execute(authorizedResolutionContext(), resolution)
		}()
		go func() {
			defer wait.Done()
			<-start
			_, debitErr = debit.Execute(ctx, transactionInput)
		}()
		close(start)
		wait.Wait()
		Expect(resolveErr == nil || errors.Is(resolveErr, fault.ErrInsufficientBalance) || errors.Is(resolveErr, hold.ErrInvalidResolution)).To(BeTrue(), "unexpected resolution error: %v", resolveErr)
		Expect(debitErr == nil || errors.Is(debitErr, fault.ErrInsufficientBalance)).To(BeTrue(), "unexpected debit error: %v", debitErr)
		Expect((resolveErr == nil) != (debitErr == nil)).To(BeTrue(), "exactly one operation must consume the competing funds")
		loaded, err := holds.FindByID(ctx, instruction.ID)
		Expect(err).NotTo(HaveOccurred())
		_, balance, err := repository.NewTransactionRepository(pool).FindLatestLedgerState(ctx, accountID)
		Expect(err).NotTo(HaveOccurred())
		Expect(balance).To(BeNumerically(">=", loaded.Allocated.Amount()-loaded.Released.Amount()))
		Expect(balance).To(BeNumerically(">=", 0))
	})

	It("should serialize a hold resolution with another hold creation", func() {
		debitExternalID, _ := insertFundedAccountsForCommand(ctx, pool, 200000, 1)
		var accountID string
		Expect(pool.QueryRow(ctx, `SELECT id FROM accounts WHERE account_external_id=$1`, debitExternalID).Scan(&accountID)).To(Succeed())
		Expect(pool.Exec(ctx, `UPDATE accounts SET holds_eligible=true WHERE id=$1`, accountID)).Error().NotTo(HaveOccurred())
		instruction := pendingHoldFixture(accountID)
		instruction.Requested, instruction.Pending = amount(150000, money.BRL), amount(150000, money.BRL)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		resolution := dto.ResolvePendingHoldInput{HoldID: instruction.ID, DecisionID: "decision-" + uuid.NewString(),
			AuthorizationReference: "authorization", Reason: "approved", Action: hold.ActionAllocate,
			PolicyVersion: "integration-v1", ExpectedVersion: 1, RequestedAmount: amount(150000, money.BRL)}
		resolve := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		create := command.NewCreateHold(uow.NewUnitOfWork(pool), repository.NewAccountRepository(pool),
			repository.NewTransactionRepository(pool), holds, fixedHoldPolicy{}, repository.NewOutBoxRepository(pool),
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		createInput := dto.CreateHoldInput{AccountID: accountID, Currency: string(money.BRL), Origin: "competing-" + uuid.NewString(),
			Reference: "reference", Type: hold.Judicial, Requested: amount(150000, money.BRL)}
		start := make(chan struct{})
		var resolveErr, createErr error
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, resolveErr = resolve.Execute(authorizedResolutionContext(), resolution)
		}()
		go func() { defer wait.Done(); <-start; _, createErr = create.Execute(ctx, createInput) }()
		close(start)
		wait.Wait()
		Expect(resolveErr == nil || errors.Is(resolveErr, hold.ErrInvalidResolution)).To(BeTrue(), "unexpected resolution error: %v", resolveErr)
		Expect(createErr).NotTo(HaveOccurred())
		var allocated int64
		Expect(pool.QueryRow(ctx, `SELECT COALESCE(sum(allocated_amount-released_amount),0) FROM holds WHERE account_id=$1`, accountID).Scan(&allocated)).To(Succeed())
		_, balance, err := repository.NewTransactionRepository(pool).FindLatestLedgerState(ctx, accountID)
		Expect(err).NotTo(HaveOccurred())
		Expect(allocated).To(BeNumerically("<=", balance))
	})

	It("should serialize a hold resolution with a concurrent release", func() {
		debitExternalID, _ := insertFundedAccountsForCommand(ctx, pool, 200000, 1)
		var accountID string
		Expect(pool.QueryRow(ctx, `SELECT id FROM accounts WHERE account_external_id=$1`, debitExternalID).Scan(&accountID)).To(Succeed())
		Expect(pool.Exec(ctx, `UPDATE accounts SET holds_eligible=true WHERE id=$1`, accountID)).Error().NotTo(HaveOccurred())
		releaseTarget := allocatedHold(accountID, "release-race-"+uuid.NewString(), "reference-"+uuid.NewString())
		releaseTarget.Requested, releaseTarget.Allocated = amount(100000, money.BRL), amount(100000, money.BRL)
		releaseTarget.Pending = amount(0, money.BRL)
		pending := pendingHoldFixture(accountID)
		pending.Requested, pending.Pending = amount(150000, money.BRL), amount(150000, money.BRL)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, releaseTarget)).To(Succeed())
		Expect(holds.Save(ctx, pending)).To(Succeed())
		resolveInput := dto.ResolvePendingHoldInput{HoldID: pending.ID, DecisionID: "decision-" + uuid.NewString(),
			AuthorizationReference: "authorization", Reason: "approved", Action: hold.ActionAllocate,
			PolicyVersion: "integration-v1", ExpectedVersion: 1, RequestedAmount: amount(150000, money.BRL)}
		resolve := integrationResolveHold(holds, repository.NewOutBoxRepository(pool))
		release := command.NewReleaseHold(uow.NewUnitOfWork(pool), holds, repository.NewOutBoxRepository(pool),
			adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
		releaseInput := dto.ReleaseHoldInput{HoldID: releaseTarget.ID, IdempotencyKey: "release-" + uuid.NewString(),
			AuthorizationReference: "release-authorization", Amount: amount(100000, money.BRL)}
		start := make(chan struct{})
		var resolveErr, releaseErr error
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, resolveErr = resolve.Execute(authorizedResolutionContext(), resolveInput)
		}()
		go func() { defer wait.Done(); <-start; _, releaseErr = release.Execute(ctx, releaseInput) }()
		close(start)
		wait.Wait()
		Expect(releaseErr).NotTo(HaveOccurred())
		Expect(resolveErr == nil || errors.Is(resolveErr, hold.ErrInvalidResolution)).To(BeTrue(), "unexpected resolution error: %v", resolveErr)
		var retained int64
		Expect(pool.QueryRow(ctx, `SELECT COALESCE(sum(allocated_amount-released_amount),0) FROM holds WHERE account_id=$1`, accountID).Scan(&retained)).To(Succeed())
		_, balance, err := repository.NewTransactionRepository(pool).FindLatestLedgerState(ctx, accountID)
		Expect(err).NotTo(HaveOccurred())
		Expect(retained).To(BeNumerically("<=", balance))
	})

	It("should keep rejection distinct from release and leave ledger entries unchanged", func() {
		accountID := insertAccount(ctx, pool)
		instruction := pendingHoldFixture(accountID)
		holds := repository.NewHoldRepository(pool)
		Expect(holds.Save(ctx, instruction)).To(Succeed())
		entriesBefore := entryCountForAccount(accountID)
		input := rejectDecision(instruction, "decision-"+uuid.NewString())
		_, err := integrationResolveHold(holds, repository.NewOutBoxRepository(pool)).Execute(authorizedResolutionContext(), input)
		Expect(err).NotTo(HaveOccurred())
		var releases, rejectionHistory int
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_releases WHERE hold_id=$1`, instruction.ID).Scan(&releases)).To(Succeed())
		Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1 AND event_type='DECISION_RESOLVED'`, instruction.ID).Scan(&rejectionHistory)).To(Succeed())
		Expect(releases).To(BeZero())
		Expect(rejectionHistory).To(Equal(1))
		Expect(entryCountForAccount(accountID)).To(Equal(entriesBefore))
	})
})

type failingResolutionOutbox struct{ outbox.Repository }

func (f failingResolutionOutbox) Save(context.Context, *outbox.Outbox) error {
	return errors.New("forced outbox failure")
}

func integrationResolveHold(holds hold.Repository, events outbox.Repository) *command.ResolvePendingHold {
	return command.NewResolvePendingHold(uow.NewUnitOfWork(pool), repository.NewAccountRepository(pool),
		repository.NewTransactionRepository(pool), holds, fixedHoldPolicy{}, events,
		adaptermocks.SilentTracerMock{}, adaptermocks.SilentLoggerMock{}, adaptermocks.SilentMetricsMock{})
}

func pendingHoldFixture(accountID string) *hold.Instruction {
	return &hold.Instruction{ID: newIDV7(), AccountID: accountID, Currency: string(money.BRL),
		Origin: "pending-origin-" + uuid.NewString(), Reference: "reference-" + uuid.NewString(), Type: hold.Judicial,
		Requested: amount(900, money.BRL), Allocated: amount(0, money.BRL), Pending: amount(900, money.BRL),
		Released: amount(0, money.BRL), Status: hold.StatusPendingReview, Version: 1, AllowPartial: true,
		RequestFingerprint: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}
}

func rejectDecision(instruction *hold.Instruction, id string) dto.ResolvePendingHoldInput {
	return dto.ResolvePendingHoldInput{HoldID: instruction.ID, DecisionID: id, AuthorizationReference: "auth-" + uuid.NewString(),
		Reason: "authorized rejection", Action: hold.ActionReject, ExpectedVersion: instruction.Version,
		RequestedAmount: amount(0, money.BRL)}
}

func authorizedResolutionContext() context.Context {
	return security.WithPrincipal(ctx, security.Principal{Subject: "integration-operator", Permissions: []string{security.ResolveFundsHold}})
}

func assertResolutionRows(holdID, decisionID string, decisions, history, events int) {
	var decisionCount, historyCount, eventCount int
	Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_decisions WHERE idempotency_key=$1`, decisionID).Scan(&decisionCount)).To(Succeed())
	Expect(pool.QueryRow(ctx, `SELECT count(*) FROM hold_history WHERE hold_id=$1 AND idempotency_key=$2`, holdID, decisionID).Scan(&historyCount)).To(Succeed())
	Expect(pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type=$2`, holdID, outbox.HoldDecisionResolved).Scan(&eventCount)).To(Succeed())
	Expect(decisionCount).To(Equal(decisions))
	Expect(historyCount).To(Equal(history))
	Expect(eventCount).To(Equal(events))
}

func entryCountForAccount(accountID string) int {
	var count int
	Expect(pool.QueryRow(ctx, `SELECT count(*) FROM entries WHERE account_id=$1`, accountID).Scan(&count)).To(Succeed())
	return count
}

var _ command.HoldPolicyProvider = fixedHoldPolicy{}
var _ outbox.Repository = failingResolutionOutbox{}
