package dto

import (
	"encoding/json"
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"github.com/agnivo/agnivo/packages/platform/validation"
)

// WriteJSON marshals body and writes it with the given status and a JSON
// content type. Encoding errors are logged but cannot be recovered after the
// header is sent.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

// Success writes data as a Response envelope with the given status and options.
func Success(w http.ResponseWriter, status int, data any, opts ...SuccessOption) {
	resp := Response{}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			Error(w, nil, errors.Wrap(err, errors.CodeInternal, "dto: marshal data"))
			return
		}
		resp.Data = raw
	}
	meta := &Meta{}
	for _, opt := range opts {
		opt(&resp, meta)
	}
	if !meta.empty() {
		resp.Meta = meta
	}
	WriteJSON(w, status, resp)
}

// OK writes a 200 success envelope.
func OK(w http.ResponseWriter, data any, opts ...SuccessOption) {
	Success(w, http.StatusOK, data, opts...)
}

// Created writes a 201 success envelope.
func Created(w http.ResponseWriter, data any, opts ...SuccessOption) {
	Success(w, http.StatusCreated, data, opts...)
}

// NoContent writes a 204 with no body.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Page writes a 200 success envelope with offset pagination metadata.
func Page(w http.ResponseWriter, items any, meta PageMeta, opts ...SuccessOption) {
	Success(w, http.StatusOK, items, append(opts, WithPageMeta(meta))...)
}

// Cursor writes a 200 success envelope with cursor pagination metadata.
func Cursor(w http.ResponseWriter, items any, meta CursorMeta, opts ...SuccessOption) {
	Success(w, http.StatusOK, items, append(opts, WithCursorMeta(meta))...)
}

// Error writes a standardized error envelope derived from err via the platform
// error framework. Validation errors surface their field details. Server-fault
// errors (5xx) are logged with trace correlation; client faults are not, to
// keep logs signal-rich. r may be nil when no request context is available.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	e := errors.From(err)
	body := ErrorBody{Code: string(e.Code()), Message: e.Message()}

	// Attach structured validation details when present.
	var verr *validation.Error
	if errors.As(err, &verr) {
		body.Details = verr.Fields
	}

	status := e.HTTPStatus()
	if r != nil && (status >= http.StatusInternalServerError) {
		errors.Log(r.Context(), logger.From(r.Context()), "request failed", e)
		errors.RecordSpan(r.Context(), e)
	}
	WriteJSON(w, status, Response{Error: &body})
}

func (m *Meta) empty() bool {
	return m.RequestID == "" && m.Page == nil && m.Cursor == nil && len(m.Extra) == 0
}
