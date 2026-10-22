//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"

	"github.com/andreis3/isura-ledger-ms/internal/application/command"
	"github.com/andreis3/isura-ledger-ms/internal/application/dto"
	"github.com/andreis3/isura-ledger-ms/internal/domain/money"
	"github.com/andreis3/isura-ledger-ms/internal/domain/outbox"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository"
	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/uow"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: RELEASE HOLD (POSTGRES)", func() {
	It("should leave transactions and entries unchanged when a hold is registered and released", func() {
		accountID := insertAccount(ctx, pool)
		instruction := allocatedHold(accountID, "accounting-invariant", "reference-"+newIDV7())
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, repository.NewOutBoxRepository(pool))
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
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, repository.NewOutBoxRepository(pool))
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

	It("should roll back the release and history when outbox persistence fails", func() {
		accountID := insertAccount(ctx, pool)
		instruction := allocatedHold(accountID, "release-rollback", "reference-"+newIDV7())
		holdRepo := repository.NewHoldRepository(pool)
		Expect(holdRepo.Save(ctx, instruction)).To(Succeed())
		sut := command.NewReleaseHold(uow.NewUnitOfWork(pool, nil), holdRepo, failingHoldOutbox{})
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
