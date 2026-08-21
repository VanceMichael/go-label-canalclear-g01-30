package httpapi

import (
	"errors"
	"net/http"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type internalError struct{}

func (internalError) Error() string { return "internal server error" }

func classifyError(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusBadRequest, "invalid_request", "The request is invalid."
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden", "You are not allowed to perform this operation."
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found", "The requested resource was not found."
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict", "The operation conflicts with current state."
	case errors.Is(err, domain.ErrState):
		return http.StatusConflict, "invalid_state", "The resource is not in a valid state for this operation."
	case errors.Is(err, domain.ErrExpired):
		return http.StatusUnauthorized, "session_expired", "The session has expired."
	default:
		return http.StatusInternalServerError, "internal_error", "The service could not complete the request."
	}
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := classifyError(err)
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, RequestID: RequestID(r.Context())}})
}
