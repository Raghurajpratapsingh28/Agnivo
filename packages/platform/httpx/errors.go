package httpx

import (
	"errors"
	"net/http"

	platerr "github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/validation"
)

// APIError is a transport-level error with a stable machine-readable code.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries optional structured context (e.g. validation fields).
	Details any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// NewAPIError constructs an APIError.
func NewAPIError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// WithDetails attaches structured details and returns the error.
func (e *APIError) WithDetails(d any) *APIError {
	e.Details = d
	return e
}

// Common, reusable errors.
var (
	ErrBadRequest   = func(msg string) *APIError { return NewAPIError(http.StatusBadRequest, "bad_request", msg) }
	ErrUnauthorized = func(msg string) *APIError { return NewAPIError(http.StatusUnauthorized, "unauthorized", msg) }
	ErrForbidden    = func(msg string) *APIError { return NewAPIError(http.StatusForbidden, "forbidden", msg) }
	ErrNotFound     = func(msg string) *APIError { return NewAPIError(http.StatusNotFound, "not_found", msg) }
	ErrConflict     = func(msg string) *APIError { return NewAPIError(http.StatusConflict, "conflict", msg) }
	ErrUnprocessable = func(msg string) *APIError {
		return NewAPIError(http.StatusUnprocessableEntity, "unprocessable_entity", msg)
	}
	ErrTooManyRequests = func(msg string) *APIError {
		return NewAPIError(http.StatusTooManyRequests, "rate_limited", msg)
	}
	ErrInternal = func(msg string) *APIError {
		return NewAPIError(http.StatusInternalServerError, "internal_error", msg)
	}
	ErrNotImplemented = func(msg string) *APIError {
		return NewAPIError(http.StatusNotImplemented, "not_implemented", msg)
	}
)

// asAPIError extracts an *APIError from err. When err is (or wraps) a platform
// *errors.Error, it is translated to an APIError using the framework's code and
// HTTP mapping, so handlers can return platform errors directly. Validation
// errors surface their field details. Anything else becomes a generic 500.
func asAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}

	var platErr *platerr.Error
	if errors.As(err, &platErr) {
		out := NewAPIError(platErr.HTTPStatus(), string(platErr.Code()), platErr.Message())
		var verr *validation.Error
		if errors.As(err, &verr) {
			out.Details = verr.Fields
		}
		return out
	}

	return ErrInternal("an unexpected error occurred")
}
