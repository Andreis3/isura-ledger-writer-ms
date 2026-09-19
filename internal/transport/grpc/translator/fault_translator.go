package translator

import (
	"errors"
	"fmt"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProtocolError maps a domain Code to external protocol status.
// Add new protocols here (gRPC, GraphQL, etc.) without touching the domain.
type ProtocolError struct {
	GRPCCode codes.Code
}

// translator is the internal conversion map from Code to protocols.
var translator = map[fault.Code]ProtocolError{
	fault.CodeBadRequest:           {GRPCCode: codes.InvalidArgument},
	fault.CodeUnauthorized:         {GRPCCode: codes.Unauthenticated},
	fault.CodeForbidden:            {GRPCCode: codes.PermissionDenied},
	fault.CodeNotFound:             {GRPCCode: codes.NotFound},
	fault.CodeConflict:             {GRPCCode: codes.AlreadyExists},
	fault.CodeUnprocessableEntity:  {GRPCCode: codes.FailedPrecondition},
	fault.CodeInternal:             {GRPCCode: codes.Internal},
	fault.CodeDatabaseError:        {GRPCCode: codes.Unavailable},
	fault.CodeInvalidEntity:        {GRPCCode: codes.InvalidArgument},
	fault.CodeUnknown:              {GRPCCode: codes.Unknown},
	fault.CodeCacheError:           {GRPCCode: codes.Internal},
	fault.CodeExternalService:      {GRPCCode: codes.Unavailable},
	fault.CodeTimeoutError:         {GRPCCode: codes.Unavailable},
	fault.CodeInvalidTransfer:      {GRPCCode: codes.InvalidArgument},
	fault.CodeInsufficientBalance:  {GRPCCode: codes.FailedPrecondition},
	fault.CodeDuplicateTransaction: {GRPCCode: codes.AlreadyExists},
}

// GRPCStatus returns the corresponding GRPC status for the error.
// If the error is not a DomainError, it returns 500.
// If the Code is not mapped, it returns 500.
func GRPCStatus(err error) codes.Code {
	var de *fault.DomainError
	if !errors.As(err, &de) {
		return codes.Internal
	}

	if p, ok := translator[de.Code]; ok {
		return p.GRPCCode
	}

	return codes.Internal
}

// Response is the structure that goes in the body of the HTTP error response.
// Never expose DomainError.Error() here — it contains technical information.
type Response struct {
	Code    fault.Code     `json:"code"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// ToGRPCError converts a DomainError to a gRPC error with custom details.
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}

	// Converts to DomainError
	if _, ok := errors.AsType[*fault.DomainError](err); !ok {
		// If it's not a DomainError, returns a generic internal error
		return status.Error(codes.Internal, "internal server error")
	}

	// Maps the domain code to a gRPC code
	grpcCode := GRPCStatus(err) // your function already exists

	// Creates the friendly response
	resp := ToResponse(err) // returns Response with Code, Message, Fields

	var br *errdetails.BadRequest
	// Creates the base status with the friendly message
	st := status.New(grpcCode, resp.Message)
	// Fills the FieldError if there are fields
	if len(resp.Fields) > 0 {
		fields := make([]*errdetails.BadRequest_FieldViolation, 0, len(resp.Fields))
		for field, val := range resp.Fields {
			// Converts the value to a string (if it's a validation error)
			msg := fmt.Sprintf("%v", val)
			fields = append(fields, &errdetails.BadRequest_FieldViolation{
				Field:       field,
				Description: msg,
			})
		}
		br = &errdetails.BadRequest{FieldViolations: fields}
		// Adds the ErrorResponse as a detail
		stWithDetails, err := st.WithDetails(br)
		if err != nil {
			// If it fails, returns the status without details (but still with the message)
			return st.Err()
		}
		return stWithDetails.Err()
	}

	return st.Err()
}

// ToResponse converts a DomainError to a safe client response.
func ToResponse(err error) Response {
	var de *fault.DomainError
	if !errors.As(err, &de) {
		return Response{
			Code:    fault.CodeInternal,
			Message: "Internal server error",
		}
	}

	return Response{
		Code:    de.Code,
		Message: de.FriendlyMessage,
		Fields:  de.Fields,
	}
}
