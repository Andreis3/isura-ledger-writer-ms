package decoder

import (
	"errors"
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
	"github.com/andreis3/isura-ledger-ms/internal/transport/rest/translator"
	"github.com/andreis3/isura-ledger-ms/internal/util"
	"github.com/bytedance/sonic"
)

const (
	ContentType     = "Content-Type"
	ApplicationJSON = "application/json"
)

type TypeResponseError struct {
	Error ErrorResponse `json:"error"`
}

type ErrorResponse struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func ResponseError(write http.ResponseWriter, err error) {
	write.Header().Set(ContentType, ApplicationJSON)

	if t, ok := errors.AsType[*fault.DomainError](err); ok {
		result := TypeResponseError{
			Error: ErrorResponse{
				Code:      string(t.Code),
				Message:   t.FriendlyMessage,
				Retryable: isRetryable(t.Code),
				Fields:    t.Fields,
			},
		}

		write.WriteHeader(translator.HTTPStatus(t.Code))
		_ = sonic.ConfigDefault.NewEncoder(write).Encode(result)
		return
	}

	write.WriteHeader(http.StatusInternalServerError)

	result := TypeResponseError{Error: ErrorResponse{
		Code:      "ILMS-9001",
		Message:   "Internal server error",
		Retryable: false,
	}}

	_ = util.JsonEngine.NewEncoder(write).Encode(result)
}

func isRetryable(code fault.Code) bool {
	return code == fault.CodeDatabaseError || code == fault.CodeTimeoutError || code == fault.CodeExternalService
}
