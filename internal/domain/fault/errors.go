package fault

import "strings"

/********Repository Errors********/
func SaveAccountError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDatabaseError,
		FriendlyMessage: "Unexpected server error; please try again later.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

func SaveModelError(modelName string, err error) *DomainError {
	var sb strings.Builder
	sb.Grow(len(modelName) + len(err.Error()) + 32)

	sb.WriteString("[")
	sb.WriteString(modelName)
	sb.WriteString("] database operation failed: ")
	sb.WriteString(err.Error())

	return &DomainError{
		Code:             CodeDatabaseError,
		FriendlyMessage:  "Unexpected server error; please try again later.",
		Cause:            err,
		Origin:           CallerName(2),
		FormattedMessage: sb.String(),
	}
}

func FindAccountError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDatabaseError,
		FriendlyMessage: "Unexpected server error; please try again later.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

func FindAccountNotFoundError(err error) *DomainError {
	return &DomainError{
		Code:            CodeNotFound,
		FriendlyMessage: "Account not found.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

func SaveAccountAlreadyExistsError(err error) *DomainError {
	return &DomainError{
		Code:            CodeAlreadyExists,
		FriendlyMessage: "Account already exists.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

func SaveBalanceAlreadyExistsError(err error) *DomainError {
	return &DomainError{
		Code:            CodeAlreadyExists,
		FriendlyMessage: "Balance already exists.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

/***************Domain Errors****************/
func InvalidEntityError(err error, fields map[string]any) *DomainError {
	return &DomainError{
		Code:            CodeInvalidEntity,
		FriendlyMessage: "Some of the information entered is incorrect; please review it and try again.",
		Cause:           err,
		Origin:          CallerName(2),
		Fields:          fields,
	}
}

func ErrCurrencyMismatch(err error) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		Cause:           err,
		FriendlyMessage: err.Error(),
		Origin:          CallerName(2),
	}
}

/***************UOW Errors****************/
func BeginTransactionError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDatabaseError,
		FriendlyMessage: "Unexpected server error; please try again later.",
		Cause:           err,
	}
}

func CommitTransactionError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDatabaseError,
		FriendlyMessage: "Unexpected server error; please try again later.",
		Cause:           err,
	}
}

func RollbackTransactionError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDatabaseError,
		FriendlyMessage: "Unexpected server error; please try again later.",
		Cause:           err,
	}
}

/***********JSON Decoder Error *****************/
func ErrorJSONSyntaxError(err error) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		FriendlyMessage: "json unmarshal type error",
		Cause:           err,
	}
}

func ErrorJSONUnmarshalTypeError(err error) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		FriendlyMessage: "json unmarshal type error",
		Cause:           err,
	}
}

func ErrorJSON(err error) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		FriendlyMessage: "json unmarshal type error",
		Cause:           err,
	}
}

/******************Money errors *************/
func InvalidAmountError(err error) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		FriendlyMessage: "Invalid amount",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

func InvalidCurrencyError(err error, fields map[string]any) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		FriendlyMessage: "Invalid currency",
		Cause:           err,
		Origin:          CallerName(2),
		Fields:          fields,
	}
}

func ConflictError(err error) *DomainError {
	return &DomainError{
		Code:            CodeConflict,
		FriendlyMessage: "Conflict",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

// IdempotencyConflictError reports reuse of a key with a different intent.
func IdempotencyConflictError(err error) *DomainError {
	return &DomainError{
		Code:            CodeDuplicateTransaction,
		FriendlyMessage: "Idempotency key was already used with different parameters.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}

// InvalidTransferError reports a violation of the transfer double-entry invariant.
func InvalidTransferError(err error) *DomainError {
	return &DomainError{
		Code:            CodeInvalidEntity,
		FriendlyMessage: "The transfer must contain one debit and one credit entry.",
		Cause:           err,
		Origin:          CallerName(2),
	}
}
