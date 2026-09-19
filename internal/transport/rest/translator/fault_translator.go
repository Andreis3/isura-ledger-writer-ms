package translator

import (
	"net/http"

	"github.com/andreis3/isura-ledger-ms/internal/domain/fault"
)

type ProtocolError struct {
	HTTPStatus int
}

var TranslatorStatusCode = map[fault.Code]ProtocolError{
	fault.CodeBadRequest:           {HTTPStatus: http.StatusBadRequest},
	fault.CodeUnauthorized:         {HTTPStatus: http.StatusUnauthorized},
	fault.CodeForbidden:            {HTTPStatus: http.StatusForbidden},
	fault.CodeNotFound:             {HTTPStatus: http.StatusNotFound},
	fault.CodeConflict:             {HTTPStatus: http.StatusConflict},
	fault.CodeUnprocessableEntity:  {HTTPStatus: http.StatusUnprocessableEntity},
	fault.CodeInternal:             {HTTPStatus: http.StatusInternalServerError},
	fault.CodeUnknown:              {HTTPStatus: http.StatusInternalServerError},
	fault.CodeDatabaseError:        {HTTPStatus: http.StatusServiceUnavailable},
	fault.CodeCacheError:           {HTTPStatus: http.StatusInternalServerError},
	fault.CodeExternalService:      {HTTPStatus: http.StatusServiceUnavailable},
	fault.CodeTimeoutError:         {HTTPStatus: http.StatusServiceUnavailable},
	fault.CodeInvalidEntity:        {HTTPStatus: http.StatusBadRequest},
	fault.CodeInvalidTransfer:      {HTTPStatus: http.StatusBadRequest},
	fault.CodeInsufficientBalance:  {HTTPStatus: http.StatusUnprocessableEntity},
	fault.CodeDuplicateTransaction: {HTTPStatus: http.StatusConflict},
	fault.CodeAlreadyExists:        {HTTPStatus: http.StatusConflict},
}

func HTTPStatus(code fault.Code) int {
	if protocolError, ok := TranslatorStatusCode[code]; ok {
		return protocolError.HTTPStatus
	}
	return http.StatusInternalServerError
}
