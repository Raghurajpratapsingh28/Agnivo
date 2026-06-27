package httpx

import (
	"net/http"
	"strconv"
)

// Defaults and bounds for pagination.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// PageParams holds normalized pagination input.
type PageParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// Offset computes the SQL offset for the page.
func (p PageParams) Offset() int { return (p.Page - 1) * p.PageSize }

// Limit returns the page size as the SQL limit.
func (p PageParams) Limit() int { return p.PageSize }

// PageMeta describes pagination state in responses.
type PageMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}

// ParsePage extracts and clamps pagination query params (?page, ?page_size).
func ParsePage(r *http.Request) PageParams {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), DefaultPage)
	size := atoiDefault(q.Get("page_size"), DefaultPageSize)
	if page < 1 {
		page = DefaultPage
	}
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return PageParams{Page: page, PageSize: size}
}

// NewPageMeta builds response metadata from params and a total count.
func NewPageMeta(p PageParams, total int64) PageMeta {
	totalPages := int64(0)
	if p.PageSize > 0 {
		totalPages = (total + int64(p.PageSize) - 1) / int64(p.PageSize)
	}
	return PageMeta{Page: p.Page, PageSize: p.PageSize, TotalItems: total, TotalPages: totalPages}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
