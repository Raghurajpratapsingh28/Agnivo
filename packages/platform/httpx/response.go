// Package httpx provides shared HTTP infrastructure: a configured router,
// middleware, request decoding, response envelopes, and pagination — with no
// business logic.
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/logger"
)

// Envelope is the standard success response shape.
type Envelope struct {
	Data any `json:"data"`
	// Meta carries offset pagination metadata when present.
	Meta *PageMeta `json:"meta,omitempty"`
}

// CursorEnvelope is the success response shape for cursor-paginated endpoints.
type CursorEnvelope struct {
	Data any         `json:"data"`
	Meta *CursorMeta `json:"meta,omitempty"`
}

// ErrorEnvelope is the standard error response shape.
type ErrorEnvelope struct {
	Error *APIError `json:"error"`
}

// JSON writes a JSON body with the given status code.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

// OK writes a 200 success envelope.
func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Envelope{Data: data})
}

// Created writes a 201 success envelope.
func Created(w http.ResponseWriter, data any) {
	JSON(w, http.StatusCreated, Envelope{Data: data})
}

// NoContent writes a 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Page writes a paginated success envelope.
func Page(w http.ResponseWriter, data any, meta PageMeta) {
	JSON(w, http.StatusOK, Envelope{Data: data, Meta: &meta})
}

// Error writes a standardized error response. Unexpected server errors (5xx)
// are logged with the request context so they are correlated and traceable.
// Expected conditions surfaced as 5xx (e.g. not_implemented) are logged at warn
// without a stack trace to keep logs signal-rich.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	apiErr := asAPIError(err)
	switch {
	case apiErr.Status >= http.StatusInternalServerError && apiErr.Code == "internal_error":
		logger.From(r.Context()).Error("request failed",
			zapError(err), zapInt("status", apiErr.Status), zapString("code", apiErr.Code))
	case apiErr.Status >= http.StatusInternalServerError:
		logger.From(r.Context()).Warn("request not served",
			zapInt("status", apiErr.Status), zapString("code", apiErr.Code))
	}
	JSON(w, apiErr.Status, ErrorEnvelope{Error: apiErr})
}

// NotImplemented is a placeholder handler for routes not yet wired to business
// logic. It keeps the API surface explicit during incremental development.
func NotImplemented(w http.ResponseWriter, r *http.Request) {
	Error(w, r, ErrNotImplemented("this endpoint is not implemented yet"))
}
