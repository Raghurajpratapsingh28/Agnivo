// Package dto provides reusable request and response Data Transfer Object
// infrastructure shared by every HTTP surface: a single envelope shape, success
// and error encoders, offset and cursor pagination responses, request decoding
// with validation, and consistent error mapping driven by the platform error
// framework. Keeping the wire contract here means every executable speaks the
// same JSON dialect.
package dto

import "encoding/json"

// Response is the canonical envelope for every API response. Exactly one of
// Data or Error is populated; Meta is optional.
type Response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Meta  *Meta           `json:"meta,omitempty"`
	Error *ErrorBody      `json:"error,omitempty"`
}

// Meta carries response-level metadata such as the request ID and pagination
// state. All fields are optional and omitted when empty.
type Meta struct {
	RequestID string         `json:"request_id,omitempty"`
	Page      *PageMeta      `json:"page,omitempty"`
	Cursor    *CursorMeta    `json:"cursor,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// PageMeta describes offset-based pagination state.
type PageMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}

// CursorMeta describes keyset (cursor) pagination state.
type CursorMeta struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// ErrorBody is the client-facing error shape. Code is the stable machine code;
// Message is human-readable; Details carries structured context (e.g. field
// validation errors). Internal details are never placed here.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// SuccessOption mutates a Response before it is written.
type SuccessOption func(*Response, *Meta)

// WithRequestID stamps the response metadata with a request ID.
func WithRequestID(id string) SuccessOption {
	return func(_ *Response, m *Meta) {
		if id != "" {
			m.RequestID = id
		}
	}
}

// WithPageMeta attaches offset pagination metadata.
func WithPageMeta(p PageMeta) SuccessOption {
	return func(_ *Response, m *Meta) { m.Page = &p }
}

// WithCursorMeta attaches cursor pagination metadata.
func WithCursorMeta(c CursorMeta) SuccessOption {
	return func(_ *Response, m *Meta) { m.Cursor = &c }
}

// WithExtra attaches an arbitrary metadata key/value.
func WithExtra(key string, value any) SuccessOption {
	return func(_ *Response, m *Meta) {
		if m.Extra == nil {
			m.Extra = make(map[string]any, 1)
		}
		m.Extra[key] = value
	}
}

// NewPageMeta computes pagination metadata from page, size, and total count.
func NewPageMeta(page, pageSize int, total int64) PageMeta {
	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return PageMeta{Page: page, PageSize: pageSize, TotalItems: total, TotalPages: totalPages}
}
