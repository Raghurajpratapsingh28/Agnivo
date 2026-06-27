// Package repository provides generic, business-logic-free persistence
// abstractions over the postgres package. A Repository[T] offers CRUD,
// pagination (offset and cursor), filtering, optimistic locking, soft deletes,
// bulk inserts/updates, and consistent error translation, while leaving all
// domain rules to the calling service layer.
//
// Scanning uses pgx's struct mapping (the `db` struct tag), so entities are
// plain structs with no persistence framework leaking into them. Repositories
// route every query through postgres.DB.Conn, so they transparently join an
// ambient transaction.
package repository

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
)

// ErrOptimisticLock indicates a versioned update lost a race: the row's version
// no longer matches the expected one. It is a retryable conflict.
var ErrOptimisticLock = errors.New(errors.CodeConflict, "repository: optimistic lock conflict").WithRetryable(true)

// Repository is a generic data-access object for entity type T mapped to a
// single table. T must use `db` struct tags matching column names.
type Repository[T any] struct {
	db            *postgres.DB
	table         string
	pk            string
	columns       string // explicit projection, defaults to "*"
	softDeleteCol string
	versionCol    string
}

// Option configures a Repository.
type Option func(*config)

type config struct {
	pk            string
	columns       string
	softDeleteCol string
	versionCol    string
}

// WithPrimaryKey sets the primary key column (default "id").
func WithPrimaryKey(col string) Option { return func(c *config) { c.pk = col } }

// WithColumns sets an explicit projection instead of "*".
func WithColumns(cols ...string) Option {
	return func(c *config) { c.columns = strings.Join(cols, ", ") }
}

// WithSoftDelete enables soft deletes via the given timestamp column. Reads
// automatically exclude rows where the column is non-null.
func WithSoftDelete(col string) Option { return func(c *config) { c.softDeleteCol = col } }

// WithOptimisticLock enables optimistic concurrency via the given integer
// version column.
func WithOptimisticLock(col string) Option { return func(c *config) { c.versionCol = col } }

// New constructs a Repository for table.
func New[T any](db *postgres.DB, table string, opts ...Option) *Repository[T] {
	c := &config{pk: "id", columns: "*"}
	for _, fn := range opts {
		fn(c)
	}
	return &Repository[T]{
		db:            db,
		table:         table,
		pk:            c.pk,
		columns:       c.columns,
		softDeleteCol: c.softDeleteCol,
		versionCol:    c.versionCol,
	}
}

// liveClause returns the soft-delete predicate (or empty) for read queries.
func (r *Repository[T]) liveClause() string {
	if r.softDeleteCol == "" {
		return ""
	}
	return r.softDeleteCol + " IS NULL"
}

// GetByID fetches a single row by primary key. It returns a CodeNotFound error
// when no live row matches.
func (r *Repository[T]) GetByID(ctx context.Context, id any) (T, error) {
	var zero T
	cond := Eq(r.pk, id)
	if live := r.liveClause(); live != "" {
		cond = And(cond, Raw(live))
	}
	where, args := cond.build(1)
	sql := "SELECT " + r.columns + " FROM " + r.table + " WHERE " + where + " LIMIT 1"

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return zero, postgres.Translate(err, "repository: get by id")
	}
	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return zero, postgres.Translate(err, "repository: get by id")
	}
	return entity, nil
}

// Find returns all live rows matching cond, ordered by orderBy (a safe,
// repository-authored ORDER BY clause; empty means unordered).
func (r *Repository[T]) Find(ctx context.Context, cond Condition, orderBy string) ([]T, error) {
	cond = r.withLive(cond)
	where, args := cond.build(1)
	sql := "SELECT " + r.columns + " FROM " + r.table
	if where != "" {
		sql += " WHERE " + where
	}
	if orderBy != "" {
		sql += " ORDER BY " + orderBy
	}
	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, postgres.Translate(err, "repository: find")
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, postgres.Translate(err, "repository: find")
	}
	return out, nil
}

// Count returns the number of live rows matching cond.
func (r *Repository[T]) Count(ctx context.Context, cond Condition) (int64, error) {
	cond = r.withLive(cond)
	where, args := cond.build(1)
	sql := "SELECT count(*) FROM " + r.table
	if where != "" {
		sql += " WHERE " + where
	}
	var n int64
	if err := r.db.Conn(ctx).QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, postgres.Translate(err, "repository: count")
	}
	return n, nil
}

// Exists reports whether any live row matches cond.
func (r *Repository[T]) Exists(ctx context.Context, cond Condition) (bool, error) {
	cond = r.withLive(cond)
	where, args := cond.build(1)
	sql := "SELECT EXISTS(SELECT 1 FROM " + r.table
	if where != "" {
		sql += " WHERE " + where
	}
	sql += ")"
	var ok bool
	if err := r.db.Conn(ctx).QueryRow(ctx, sql, args...).Scan(&ok); err != nil {
		return false, postgres.Translate(err, "repository: exists")
	}
	return ok, nil
}

// Insert writes a row from a column→value map and returns the persisted entity
// (via RETURNING), so database defaults and triggers are reflected. Column
// order is deterministic to maximize prepared-statement cache hits.
func (r *Repository[T]) Insert(ctx context.Context, values map[string]any) (T, error) {
	var zero T
	cols, placeholders, args := splitValues(values, 0)
	sql := "INSERT INTO " + r.table + " (" + strings.Join(cols, ", ") + ") VALUES (" +
		strings.Join(placeholders, ", ") + ") RETURNING " + r.columns

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return zero, postgres.Translate(err, "repository: insert")
	}
	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return zero, postgres.Translate(err, "repository: insert")
	}
	return entity, nil
}

// Update applies a column→value map to the row identified by id, returning the
// updated entity. Returns CodeNotFound when no live row matches.
func (r *Repository[T]) Update(ctx context.Context, id any, values map[string]any) (T, error) {
	var zero T
	setClause, args := buildSet(values, 0)
	next := len(args) + 1
	sql := "UPDATE " + r.table + " SET " + setClause + " WHERE " + r.pk + " = $" + strconv.Itoa(next)
	args = append(args, id)
	if live := r.liveClause(); live != "" {
		sql += " AND " + live
	}
	sql += " RETURNING " + r.columns

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return zero, postgres.Translate(err, "repository: update")
	}
	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return zero, postgres.Translate(err, "repository: update")
	}
	return entity, nil
}

// UpdateOptimistic applies values only if the row's version equals expected,
// then bumps the version. It returns ErrOptimisticLock when the version moved
// (concurrent update). Requires WithOptimisticLock.
func (r *Repository[T]) UpdateOptimistic(ctx context.Context, id any, expectedVersion int64, values map[string]any) (T, error) {
	var zero T
	if r.versionCol == "" {
		return zero, errors.New(errors.CodeInternal, "repository: optimistic lock not configured")
	}
	setClause, args := buildSet(values, 0)
	// Bump the version as part of the same statement.
	setClause += ", " + r.versionCol + " = " + r.versionCol + " + 1"

	next := len(args) + 1
	sql := "UPDATE " + r.table + " SET " + setClause +
		" WHERE " + r.pk + " = $" + strconv.Itoa(next) +
		" AND " + r.versionCol + " = $" + strconv.Itoa(next+1)
	args = append(args, id, expectedVersion)
	if live := r.liveClause(); live != "" {
		sql += " AND " + live
	}
	sql += " RETURNING " + r.columns

	rows, err := r.db.Conn(ctx).Query(ctx, sql, args...)
	if err != nil {
		return zero, postgres.Translate(err, "repository: update optimistic")
	}
	entity, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		if errors.IsCode(postgres.Translate(err, ""), errors.CodeNotFound) {
			// No row with this id+version: either gone or version moved.
			return zero, ErrOptimisticLock
		}
		return zero, postgres.Translate(err, "repository: update optimistic")
	}
	return entity, nil
}

// Delete permanently removes the row by id, returning whether a row was deleted.
func (r *Repository[T]) Delete(ctx context.Context, id any) (bool, error) {
	sql := "DELETE FROM " + r.table + " WHERE " + r.pk + " = $1"
	tag, err := r.db.Conn(ctx).Exec(ctx, sql, id)
	if err != nil {
		return false, postgres.Translate(err, "repository: delete")
	}
	return tag.RowsAffected() > 0, nil
}

// SoftDelete sets the soft-delete column to now() for the row by id. Requires
// WithSoftDelete.
func (r *Repository[T]) SoftDelete(ctx context.Context, id any) (bool, error) {
	if r.softDeleteCol == "" {
		return false, errors.New(errors.CodeInternal, "repository: soft delete not configured")
	}
	sql := "UPDATE " + r.table + " SET " + r.softDeleteCol + " = now() WHERE " + r.pk +
		" = $1 AND " + r.softDeleteCol + " IS NULL"
	tag, err := r.db.Conn(ctx).Exec(ctx, sql, id)
	if err != nil {
		return false, postgres.Translate(err, "repository: soft delete")
	}
	return tag.RowsAffected() > 0, nil
}

// withLive ANDs the soft-delete predicate onto cond for read queries.
func (r *Repository[T]) withLive(cond Condition) Condition {
	if live := r.liveClause(); live != "" {
		if cond.empty() {
			return Raw(live)
		}
		return And(cond, Raw(live))
	}
	return cond
}

// splitValues returns deterministically-ordered columns, placeholders, and args
// for an INSERT, starting placeholders at offset+1.
func splitValues(values map[string]any, offset int) (cols, placeholders []string, args []any) {
	cols = make([]string, 0, len(values))
	for k := range values {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	placeholders = make([]string, len(cols))
	args = make([]any, len(cols))
	for i, c := range cols {
		placeholders[i] = "$" + strconv.Itoa(offset+i+1)
		args[i] = values[c]
	}
	return cols, placeholders, args
}

// buildSet returns a deterministic "col = $n, ..." SET clause and its args.
func buildSet(values map[string]any, offset int) (string, []any) {
	cols := make([]string, 0, len(values))
	for k := range values {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	parts := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		parts[i] = c + " = $" + strconv.Itoa(offset+i+1)
		args[i] = values[c]
	}
	return strings.Join(parts, ", "), args
}
