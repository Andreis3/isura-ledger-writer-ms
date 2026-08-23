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
	CodeError       string         `json:"code_error"`
	Cause           string         `json:"cause,omitempty"`
	ErrorFields     map[string]any `json:"error_fields,omitempty"`
	FriendlyMessage any            `json:"friendly_message"`
}

func ResponseError(write http.ResponseWriter, err error) {
	write.Header().Set(ContentType, ApplicationJSON)

	if t, ok := errors.AsType[*fault.DomainError](err); ok {
		result := TypeResponseError{
			CodeError:       string(t.Code),
			Cause:           t.Cause.Error(),
			ErrorFields:     t.Fields,
			FriendlyMessage: t.FriendlyMessage,
		}

		write.WriteHeader(translator.TranslatorStatusCode[t.Code].HTTPStatus)
		_ = sonic.ConfigDefault.NewEncoder(write).Encode(result)
		return
	}

	write.WriteHeader(http.StatusInternalServerError)

	result := TypeResponseError{
		FriendlyMessage: "Internal server error",
	}

	_ = util.JsonEngine.NewEncoder(write).Encode(result)
}
