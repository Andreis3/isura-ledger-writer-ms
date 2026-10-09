//go:build unit

package fault_test

import (
 "errors"
 "log/slog"

 "github.com/andreis3/isura-ledger-ms/internal/domain/fault"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)

var _ = Describe("INTERNAL :: DOMAIN :: FAULT :: ERRORS",func(){
 Describe("#DomainError",func(){
  Context("success cases",func(){
   It("should classify repository, transaction and validation errors while preserving their causes",func(){
    cause:=errors.New("original failure")
    cases:=[]struct{name string;makeError func()*fault.DomainError;code fault.Code}{
     {"save account",func()*fault.DomainError{return fault.SaveAccountError(cause)},fault.CodeDatabaseError},
     {"save model",func()*fault.DomainError{return fault.SaveModelError("entries",cause)},fault.CodeDatabaseError},
     {"find account",func()*fault.DomainError{return fault.FindAccountError(cause)},fault.CodeDatabaseError},
     {"not found",func()*fault.DomainError{return fault.FindAccountNotFoundError(cause)},fault.CodeNotFound},
     {"duplicate account",func()*fault.DomainError{return fault.SaveAccountAlreadyExistsError(cause)},fault.CodeAlreadyExists},
     {"duplicate balance",func()*fault.DomainError{return fault.SaveBalanceAlreadyExistsError(cause)},fault.CodeAlreadyExists},
     {"invalid entity",func()*fault.DomainError{return fault.InvalidEntityError(cause,map[string]any{"amount":"invalid"})},fault.CodeInvalidEntity},
     {"currency mismatch",func()*fault.DomainError{return fault.ErrCurrencyMismatch(cause)},fault.CodeBadRequest},
     {"begin transaction",func()*fault.DomainError{return fault.BeginTransactionError(cause)},fault.CodeDatabaseError},
     {"commit transaction",func()*fault.DomainError{return fault.CommitTransactionError(cause)},fault.CodeDatabaseError},
     {"rollback transaction",func()*fault.DomainError{return fault.RollbackTransactionError(cause)},fault.CodeDatabaseError},
     {"invalid JSON syntax",func()*fault.DomainError{return fault.ErrorJSONSyntaxError(cause)},fault.CodeBadRequest},
     {"invalid JSON type",func()*fault.DomainError{return fault.ErrorJSONUnmarshalTypeError(cause)},fault.CodeBadRequest},
     {"invalid JSON",func()*fault.DomainError{return fault.ErrorJSON(cause)},fault.CodeBadRequest},
     {"invalid amount",func()*fault.DomainError{return fault.InvalidAmountError(cause)},fault.CodeBadRequest},
     {"invalid currency",func()*fault.DomainError{return fault.InvalidCurrencyError(cause,map[string]any{"currency":"invalid"})},fault.CodeBadRequest},
     {"conflict",func()*fault.DomainError{return fault.ConflictError(cause)},fault.CodeConflict},
     {"retry exhausted",func()*fault.DomainError{return fault.TransactionConflictError(cause)},fault.CodeTimeoutError},
     {"idempotency",func()*fault.DomainError{return fault.IdempotencyConflictError(cause)},fault.CodeDuplicateTransaction},
     {"transfer",func()*fault.DomainError{return fault.InvalidTransferError(cause)},fault.CodeInvalidTransfer},
    }
    for _,tc:=range cases {
     By(tc.name)
     got:=tc.makeError()
     Expect(got.Code).To(Equal(tc.code))
     Expect(got.FriendlyMessage).NotTo(BeEmpty())
     Expect(errors.Is(got,cause)).To(BeTrue())
     Expect(errors.Is(got,&fault.DomainError{Code:tc.code})).To(BeTrue())
     Expect(errors.Is(got,&fault.DomainError{Code:fault.CodeUnknown})).To(BeFalse())
    }
    Expect(fault.SaveModelError("entries",cause).FormattedMessage).To(ContainSubstring("[entries]"))
   })
   It("should preserve cause and fields through generic constructors and formatted logs",func(){
    cause:=errors.New("original failure")
    wrapped:=fault.Wrap(fault.CodeDatabaseError,"try later",cause)
    Expect(errors.Is(wrapped,cause)).To(BeTrue())
    Expect(wrapped.Origin).NotTo(BeEmpty())
    Expect(fault.New(fault.CodeDatabaseError,"try later",cause).Cause).To(Equal(cause))
    fields:=fault.NewWithFields(fault.CodeInvalidEntity,"invalid",map[string]any{"amount":"required"})
    Expect(fields.Fields).To(HaveKeyWithValue("amount","required"))
    Expect(fault.NewValidationError(map[string]any{"amount":"missing"}).Fields).To(HaveKey("amount"))
    Expect((&fault.ValidationError{Errors:map[string]any{"one":1,"two":2}}).Error()).To(ContainSubstring("2 error(s)"))
    attrs:=fault.Attrs(wrapped)
    Expect(attrs).NotTo(BeEmpty())
    first,ok:=attrs[0].(slog.Attr)
    Expect(ok).To(BeTrue())
    Expect(first.Key).To(Equal("error_code"))
    Expect(first.Value.String()).To(Equal(string(fault.CodeDatabaseError)))
    Expect(fault.Attrs(errors.New("plain"))).To(HaveLen(1))
    Expect(wrapped.Error()).To(ContainSubstring(string(fault.CodeDatabaseError)))
   })
  })
 })
})
