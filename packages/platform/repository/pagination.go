package repository

import (
	"context"
	"strconv"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/jackc/pgx/v5"
)

// Page is a single page of results plus the total count, for offset pagination.
type Page[T any] struct {
	Items      []T   `json:"items"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int64 `json:"total_pages"`
}

// PageParams holds normalized offset-pagination input.
type PageParams struct {
	Page     int
	PageSize int
	OrderBy  string // repository-authored ORDER BY (required for stable paging)
}

func (p PageParams) offset() int { return (p.Page - 1) * p.PageSize }

// Paginate runs a filtered, ordered, offset-paginated query. It first counts
// matching rows, then fetches the requested page. Use PaginateCursor for deep
// pagination where OFFSET's linear cost becomes significant.
func (r *Repository[T]) Paginate(ctx context.Context, cond Condition, params PageParams) (Page[T], error) {
	total, err := r.Count(ctx, cond)
	if err != nil {
		return Page[T]{}, err
	}

	live := r.withLive(cond)
	where, args := live.build(1)
	sql := "SELECT " + r.columns + " FROM " + r.table
	if where != "" {
		sql += " WHERE " + where
	}
	if params.OrderBy != "" {
		sql += " ORDER BY " + params.OrderBy
	}
	limitPos := len(args) + 1
	sql += " LIMIT $" + strconv.Itoa(limitPos) + " OFFSET $" + strconv.Itoa(limitPos+1)
	args = append(args, params.PageSize, params.offset())

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return Page[T]{}, postgres.Translate(err, "repository: paginate")
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return Page[T]{}, postgres.Translate(err, "repository: paginate")
	}

	totalPages := int64(0)
	if params.PageSize > 0 {
		totalPages = (total + int64(params.PageSize) - 1) / int64(params.PageSize)
	}
	return Page[T]{
		Items:      items,
		Total:      total,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalPages: totalPages,
	}, nil
}

// CursorPage is a single page for keyset (cursor) pagination, which scales to
// deep result sets without OFFSET's linear scan cost.
type CursorPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// CursorParams configures keyset pagination over a single, unique, sortable
// column (e.g. an id or created_at+id tuple encoded by the caller).
type CursorParams struct {
	// Column is the keyset column to compare against (must be orderable/unique).
	Column string
	// After is the last value from the previous page; empty starts at the top.
	After any
	// Limit is the page size.
	Limit int
	// Descending reverses the sort and comparison direction.
	Descending bool
}

// PaginateCursor returns up to Limit rows after the cursor, ordered by the
// keyset column. It fetches Limit+1 rows to compute HasMore without a count.
func (r *Repository[T]) PaginateCursor(ctx context.Context, cond Condition, params CursorParams, nextCursor func(T) string) (CursorPage[T], error) {
	op, order := ">", "ASC"
	if params.Descending {
		op, order = "<", "DESC"
	}
	if params.After != nil && params.After != "" {
		cond = And(cond, Raw(params.Column+" "+op+" ?", params.After))
	}
	cond = r.withLive(cond)
	where, args := cond.build(1)

	sql := "SELECT " + r.columns + " FROM " + r.table
	if where != "" {
		sql += " WHERE " + where
	}
	sql += " ORDER BY " + params.Column + " " + order
	limitPos := len(args) + 1
	sql += " LIMIT $" + strconv.Itoa(limitPos)
	args = append(args, params.Limit+1) // +1 sentinel to detect more pages

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return CursorPage[T]{}, postgres.Translate(err, "repository: cursor paginate")
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return CursorPage[T]{}, postgres.Translate(err, "repository: cursor paginate")
	}

	page := CursorPage[T]{Items: items}
	if len(items) > params.Limit {
		page.Items = items[:params.Limit]
		page.HasMore = true
		if nextCursor != nil {
			page.NextCursor = nextCursor(page.Items[len(page.Items)-1])
		}
	}
	return page, nil
}
