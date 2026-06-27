package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is the minimal database surface that repositories depend on. Both the
// connection pool (*DB) and an in-flight transaction satisfy it, so a repository
// written against Querier transparently participates in a surrounding
// transaction or runs standalone. This is the single seam that keeps pgx out of
// higher layers: only the repository package imports pgx row scanning.
type Querier interface {
	// Exec runs a statement that returns no rows and reports the affected count.
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	// Query runs a statement returning rows the caller must iterate and close.
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	// QueryRow runs a statement expected to return at most one row.
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	// CopyFrom performs a high-throughput bulk insert via the COPY protocol.
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	// SendBatch executes a pipelined batch of statements in one round trip.
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Compile-time assertions that the pool and transactions implement Querier.
var (
	_ Querier = (*DB)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// Conn returns the active Querier for ctx: the in-flight transaction when one
// is present (see Transact), otherwise the pool. Repositories should always
// route through Conn so they automatically join the caller's transaction.
func (db *DB) Conn(ctx context.Context) Querier {
	if tx, ok := txFrom(ctx); ok {
		return tx
	}
	return db
}
