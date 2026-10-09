package fault

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
)

// Code represents the semantic classification of the error in the domain.
// Using a string allows direct serialization in logs and JSON responses.
type Code string

const (
	CodeBadRequest          Code = "ERR_400"
	CodeUnauthorized        Code = "ERR_401"
	CodeForbidden           Code = "ERR_403"
	CodeConflict            Code = "ERR_409"
	CodeUnprocessableEntity Code = "ERR_422"

	// --- Client Errors (ILMS-1xxx) ─────────────────────────
	CodeInvalidEntity        Code = "ILMS-1001" // Invalid entity (validation)
	CodeNotFound             Code = "ILMS-1002" // Resource not found
	CodeAlreadyExists        Code = "ILMS-1003" // Resource already exists
	CodeInsufficientBalance  Code = "ILMS-1004" // Insufficient balance
	CodeDuplicateTransaction Code = "ILMS-1005" // Duplicate transaction
	CodeInvalidTransfer      Code = "ILMS-1006" // Invalid transfer

	// --- Server Errors (ILMS-2xxx) ─────────────────────────
	CodeDatabaseError   Code = "ILMS-2001" // Database error
	CodeCacheError      Code = "ILMS-2002" // Cache error
	CodeExternalService Code = "ILMS-2003" // External service error
	CodeTimeoutError    Code = "ILMS-2004" // Timeout

	// --- Generic Errors (ILMS-9xxx) ──────────────────────────
	CodeInternal Code = "ILMS-9001" // Internal error
	CodeUnknown  Code = "ILMS-9999" // Unknown error
)

// DomainError is the rich domain error.
// It implements the error interface and is compatible with errors.Is / errors.As / errors.Unwrap.
// ErrCommitOutcomeUnknown indicates that the database may have committed despite a lost acknowledgement.
var ErrCommitOutcomeUnknown = errors.New("transaction commit outcome unknown")

type DomainError struct {
	Code             Code           // Semantic classification
	FriendlyMessage  string         // Safe message to expose to the client
	Fields           map[string]any // Errors by field (e.g., form validation)
	Origin           string         // Origin function, filled automatically
	Cause            error          // Original error preserved (allows wrapping)
	FormattedMessage string         // Formated message

}

type ValidationError struct {
	Errors map[string]any
}

func NewValidationError(fields map[string]any) *DomainError {
	return &DomainError{
		Code:            CodeBadRequest,
		Cause:           errors.New("Invalid entity"),
		Fields:          fields,
		Origin:          CallerName(2),
		FriendlyMessage: "Some of the information entered is incorrect; please review it and try again.",
	}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed with %d error(s)", len(e.Errors))
}

// Error implements the error interface.
// Returns complete technical information — use only in internal logs.
func (e *DomainError) Error() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s]", e.Code))

	if e.Origin != "" {
		sb.WriteString(fmt.Sprintf(" %s", e.Origin))
	}

	if e.Cause != nil {
		sb.WriteString(fmt.Sprintf(" %s", e.Cause.Error()))
	}

	if len(e.Fields) > 0 {
		//sb.WriteString(fmt.Sprintf(" fields=%v", e.Fields))
		sb.WriteString(" [")
		for k, v := range e.Fields {
			sb.WriteString(fmt.Sprintf("%s=%v ", k, v))
		}
		sb.WriteString("]")
	}

	return sb.String()
}

// Is allows comparing DomainError by Code using errors.Is.
//
// Example:
//
//	var ErrNotFound = &DomainError{Code: CodeNotFound}
//	errors.Is(err, ErrNotFound) // true if both have CodeNotFound
func (e *DomainError) Is(target error) bool {
	if t, ok := errors.AsType[*DomainError](target); ok {
		return e.Code == t.Code
	}
	return false
}

// Unwrap implements the errors.Unwrap interface.
func (e *DomainError) Unwrap() error {
	return e.Cause
}

// New creates a DomainError.
// origin is automatically filled with the name of the calling function.
func New(code Code, friendly string, cause error) *DomainError {
	return &DomainError{
		Code:            code,
		FriendlyMessage: friendly,
		Cause:           cause,
		Origin:          CallerName(2),
	}
}

// NewWithFields creates a DomainError with a map of errors by field.
// Useful for validation errors where each field has its own message.
func NewWithFields(code Code, friendly string, fields map[string]any) *DomainError {
	return &DomainError{
		Code:            code,
		FriendlyMessage: friendly,
		Fields:          fields,
		Origin:          CallerName(2),
		Cause:           nil,
	}
}

// Wrap wraps an existing error in a DomainError, preserving the original cause.
// Semantically equivalent to fmt.Errorf("op: %w", err) but with rich metadata.
func Wrap(code Code, friendly string, cause error) *DomainError {
	return &DomainError{
		Code:            code,
		FriendlyMessage: friendly,
		Cause:           cause,
		Origin:          CallerName(2),
	}
}

func CallerName(skip int) string {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "unknown"
	}

	full := fn.Name()
	if idx := strings.LastIndex(full, "/"); idx >= 0 {
		full = full[idx+1:]
	}

	// gets only the file name without the full path
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}

	return fmt.Sprintf("%s (%s:%d)", full, file, line)
}

// Attrs returns the slog attributes of the DomainError for use in logs.
// If err is not a DomainError, it returns only the error as a string.
func Attrs(err error) []any {
	if de, ok := errors.AsType[*DomainError](err); ok {
		var attrs []any
		if de.Code != "" {
			attrs = append(attrs, slog.String("error_code", string(de.Code)))
		}
		if de.Cause != nil {
			attrs = append(attrs, slog.String("error_cause", de.Cause.Error()))
		}
		if de.Fields != nil {
			attrs = append(attrs, slog.Any("error_fields", de.Fields))
		}
		if de.Origin != "" {
			attrs = append(attrs, slog.String("error_origin", de.Origin))
		}
		if de.FriendlyMessage != "" {
			attrs = append(attrs, slog.String("error_friendly_message", de.FriendlyMessage))
		}

		if de.FormattedMessage != "" {
			attrs = append(attrs, slog.String("error_formatted_message", de.FormattedMessage))
		}
		return attrs
	}
	return []any{
		slog.String("error", err.Error()),
	}
}
