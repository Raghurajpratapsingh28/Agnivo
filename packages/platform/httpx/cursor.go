package httpx

import (
	"net/http"
)

// DefaultCursorLimit is the default page size for cursor pagination.
const DefaultCursorLimit = 20

// MaxCursorLimit caps cursor page sizes.
const MaxCursorLimit = 100

// CursorParams holds normalized cursor pagination input from query parameters
// (?cursor, ?limit).
type CursorParams struct {
	// After is the opaque cursor from the previous page's next_cursor.
	After string `json:"cursor,omitempty"`
	// Limit is the requested page size, clamped to [1, MaxCursorLimit].
	Limit int `json:"limit"`
}

// ParseCursor extracts and clamps cursor pagination query params.
func ParseCursor(r *http.Request) CursorParams {
	after := QueryString(r, "cursor")
	limit := DefaultCursorLimit
	if n, ok := QueryInt(r, "limit"); ok {
		limit = n
	}
	if limit < 1 {
		limit = DefaultCursorLimit
	}
	if limit > MaxCursorLimit {
		limit = MaxCursorLimit
	}
	return CursorParams{After: after, Limit: limit}
}

// CursorMeta describes cursor pagination state in responses.
type CursorMeta struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// NewCursorMeta builds response metadata from a next cursor and has-more flag.
func NewCursorMeta(nextCursor string, hasMore bool) CursorMeta {
	return CursorMeta{NextCursor: nextCursor, HasMore: hasMore}
}

// CursorPage writes a 200 success envelope with cursor pagination metadata.
func CursorPage(w http.ResponseWriter, data any, meta CursorMeta) {
	JSON(w, http.StatusOK, CursorEnvelope{Data: data, Meta: &meta})
}
