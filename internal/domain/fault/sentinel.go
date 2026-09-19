package fault

// Sentinel errors by domain.
//
// Sentinel errors are global variables that represent known error conditions.
// They allow comparison with errors.Is without depending on strings.
//
// Convention:
//   - "Err" prefix for business errors (expected)
//   - No prefix for infrastructure errors (wrapped with Wrap())
//
// Usage:
//
//   if errors.Is(err, ErrCustomerNotFound) {
//       // handle 404
//   }

/* --- Account --- */
var (
	ErrAccountNotFound      = &DomainError{Code: CodeNotFound, FriendlyMessage: "account not found"}
	ErrAccountAlreadyExists = &DomainError{Code: CodeConflict, FriendlyMessage: "account already exists"}
	ErrInsufficientFunds    = &DomainError{Code: CodeConflict, FriendlyMessage: "insufficient funds"}
	ErrTransactionNotFound  = &DomainError{Code: CodeNotFound, FriendlyMessage: "transaction not found"}
	ErrInvalidAmount        = &DomainError{Code: CodeBadRequest, FriendlyMessage: "invalid amount"}
	ErrDuplicateTransaction = &DomainError{Code: CodeConflict, FriendlyMessage: "transaction already exists"}
	ErrIdempotencyConflict  = &DomainError{Code: CodeDuplicateTransaction, FriendlyMessage: "idempotency key was already used with different parameters"}
)
