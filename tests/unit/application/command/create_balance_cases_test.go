//go:build unit

package command_test

import (
 "context"
 "errors"

 "github.com/andreis3/isura-ledger-ms/internal/application/command"
 "github.com/andreis3/isura-ledger-ms/internal/application/dto"
 "github.com/andreis3/isura-ledger-ms/internal/domain/account"
 "github.com/andreis3/isura-ledger-ms/internal/domain/balance"
 "github.com/andreis3/isura-ledger-ms/internal/domain/entity"
 "github.com/andreis3/isura-ledger-ms/internal/domain/money"
 "github.com/andreis3/isura-ledger-ms/internal/infra/postgres/repository/criteria"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

const balanceAccountID = "019ff448-c43d-70d3-83c7-dfa0674469b7"

type balanceRepositoryStub struct {
 found *balance.Balance
 findErr error
 saveErr error
 findCalls int
 saveCalls int
}

func (r *balanceRepositoryStub) Find(_ context.Context, _ criteria.BalanceCriteria) (*balance.Balance, error) {
 r.findCalls++
 return r.found, r.findErr
}
func (r *balanceRepositoryStub) Save(_ context.Context, _ *balance.Balance) error {
 r.saveCalls++
 return r.saveErr
}

type balanceAccountRepositoryStub struct {
 result *account.Account
 findErr error
 findCalls int
}
func (r *balanceAccountRepositoryStub) Save(context.Context, *account.Account) error { return nil }
func (r *balanceAccountRepositoryStub) FindAccount(_ context.Context, _ criteria.AccountCriteria) (*account.Account, error) {
 r.findCalls++
 return r.result, r.findErr
}

func newBalanceAccount() *account.Account {
 id, err := entity.NewID(balanceAccountID)
 Expect(err).NotTo(HaveOccurred())
 return &account.Account{ID: id, Currency: money.BRL, AccountType: account.Asset, BalancePolicy: account.BalanceUnrestricted}
}

func newBalanceCommand(b *balanceRepositoryStub, a *balanceAccountRepositoryStub) *command.CreateBalance {
 return command.NewCreateBalance(b, a, testLogger{}, testTracer{}, newTestMetrics())
}

var _ = Describe("INTERNAL :: APPLICATION :: COMMAND :: CREATE BALANCE", func() {
 Describe("#Execute", func() {
  Context("success cases", func() {
   It("should create an initial balance only when the account exists", func() {
    // Arrange.
    balances := &balanceRepositoryStub{findErr: balance.ErrBalanceNotFound}
    accounts := &balanceAccountRepositoryStub{result: newBalanceAccount()}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(err).NotTo(HaveOccurred())
    Expect(balances.findCalls).To(Equal(1))
    Expect(accounts.findCalls).To(Equal(1))
    Expect(balances.saveCalls).To(Equal(1))
   })

   It("should not create another balance when one already exists", func() {
    // Arrange.
    balances := &balanceRepositoryStub{found: &balance.Balance{}}
    accounts := &balanceAccountRepositoryStub{}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(err).NotTo(HaveOccurred())
    Expect(balances.findCalls).To(Equal(1))
    Expect(accounts.findCalls).To(BeZero())
    Expect(balances.saveCalls).To(BeZero())
   })
  })

  Context("error cases", func() {
   It("should reject invalid balance data before accessing repositories", func() {
    // Arrange.
    balances := &balanceRepositoryStub{}
    accounts := &balanceAccountRepositoryStub{}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: "INVALID"})

    // Assert.
    Expect(err).To(HaveOccurred())
    Expect(balances.findCalls).To(BeZero())
    Expect(accounts.findCalls).To(BeZero())
    Expect(balances.saveCalls).To(BeZero())
   })

   It("should propagate balance lookup failures without writing", func() {
    // Arrange.
    failure := errors.New("balance lookup unavailable")
    balances := &balanceRepositoryStub{findErr: failure}
    accounts := &balanceAccountRepositoryStub{}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(errors.Is(err, failure)).To(BeTrue())
    Expect(accounts.findCalls).To(BeZero())
    Expect(balances.saveCalls).To(BeZero())
   })

   It("should reject an account that cannot be found", func() {
    // Arrange.
    balances := &balanceRepositoryStub{findErr: balance.ErrBalanceNotFound}
    accounts := &balanceAccountRepositoryStub{findErr: account.ErrAccountNotFound}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(errors.Is(err, account.ErrAccountNotFound)).To(BeTrue())
    Expect(balances.saveCalls).To(BeZero())
   })

   It("should reject an absent account returned without error", func() {
    // Arrange.
    balances := &balanceRepositoryStub{findErr: balance.ErrBalanceNotFound}
    accounts := &balanceAccountRepositoryStub{}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(err).To(HaveOccurred())
    Expect(balances.saveCalls).To(BeZero())
   })

   It("should propagate persistence failures without reporting success", func() {
    // Arrange.
    failure := errors.New("balance write failed")
    balances := &balanceRepositoryStub{findErr: balance.ErrBalanceNotFound, saveErr: failure}
    accounts := &balanceAccountRepositoryStub{result: newBalanceAccount()}
    sut := newBalanceCommand(balances, accounts)

    // Act.
    err := sut.Execute(context.Background(), dto.CreateBalanceInput{AccountID: balanceAccountID, Currency: string(money.BRL)})

    // Assert.
    Expect(errors.Is(err, failure)).To(BeTrue())
    Expect(balances.saveCalls).To(Equal(1))
   })
  })
 })
})
