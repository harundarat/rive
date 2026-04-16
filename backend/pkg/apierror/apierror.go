package apierror

import "net/http"

type APIError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Message
}

func New(httpStatus int, code, message string) *APIError {
	return &APIError{HTTPStatus: httpStatus, Code: code, Message: message}
}

// Sentinel errors
var (
	ErrBadRequest   = New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload")
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "insufficient permission")
	ErrNotFound     = New(http.StatusNotFound, "NOT_FOUND", "resource not found")
	ErrConflict     = New(http.StatusConflict, "CONFLICT", "resource already exists")
	ErrInternal     = New(http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occured")
)
