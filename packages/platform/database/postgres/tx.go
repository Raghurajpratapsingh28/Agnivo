package postgres

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type txCtxKey struct{}

// txFrom extracts the in-flight transaction from ctx, if any.
func txFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txCtxKey{}).(pgx.Tx)
	return tx, ok
}

// TxOptions configures a transaction. It mirrors the subset of pgx options that
// applications need without re-exporting the pgx type to callers.
type TxOptions struct {
	// ReadOnly starts the transaction in READ ONLY mode.
	ReadOnly bool
	// Serializable upgrades isolation to SERIALIZABLE; combine with retries to
	// handle serialization failures transparently.
	Serializable bool
}

func (o TxOptions) pgx() pgx.TxOptions {
	po := pgx.TxOptions{}
	if o.ReadOnly {
		po.AccessMode = pgx.ReadOnly
	}
	if o.Serializable {
		po.IsoLevel = pgx.Serializable
	}
	return po
}

// Transact runs fn inside a transaction with default options. The transaction
// is stored in the context passed to fn, so any repository call using Conn(ctx)
// joins it. If fn returns nil the transaction commits; otherwise it rolls back.
// A panic also triggers rollback before re-panicking.
//
// When ctx already carries a transaction, Transact opens a nested transaction
// implemented with a SAVEPOINT, giving partial rollback semantics without a new
// connection. This makes service methods freely composable.
func (db *DB) Transact(ctx context.Context, fn func(ctx context.Context) error) error {
	return db.TransactWithOptions(ctx, TxOptions{}, fn)
}

// TransactWithOptions is Transact with explicit isolation/access options. Nested
// transactions ignore the options (a savepoint inherits the parent's mode).
func (db *DB) TransactWithOptions(ctx context.Context, opts TxOptions, fn func(ctx context.Context) error) (err error) {
	var tx pgx.Tx
	if parent, ok := txFrom(ctx); ok {
		// pgx implements Tx.Begin as a SAVEPOINT, giving true nesting.
		tx, err = parent.Begin(ctx)
	} else {
		tx, err = db.BeginTx(ctx, opts.pgx())
	}
	if err != nil {
		return errors.Wrap(err, errors.CodeUnavailable, "postgres: begin transaction")
	}

	txCtx := context.WithValue(ctx, txCtxKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			// Roll back before propagating the panic so the connection is clean.
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if ferr := fn(txCtx); ferr != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return errors.Wrap(ferr, errors.CodeOf(ferr),
				"postgres: rollback failed after error: "+rbErr.Error())
		}
		return ferr
	}

	if cErr := tx.Commit(ctx); cErr != nil {
		return errors.Wrap(cErr, classify(cErr), "postgres: commit transaction")
	}
	return nil
}

// TransactWithRetry runs fn in a transaction, retrying the whole transaction on
// transient failures (serialization conflicts, deadlocks, dropped connections)
// up to the configured MaxRetries. Use it with Serializable isolation for
// correctness-critical multi-statement updates.
func (db *DB) TransactWithRetry(ctx context.Context, opts TxOptions, fn func(ctx context.Context) error) error {
	attempts := db.cfg.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		last = db.TransactWithOptions(ctx, opts, fn)
		if last == nil {
			return nil
		}
		if attempt == attempts || !errors.IsRetryable(last) {
			return last
		}
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, errors.CodeCanceled, "postgres: context canceled during retry")
		}
		db.log.Warn("retrying transaction after transient failure",
			zap.Int("attempt", attempt), zap.Error(last))
	}
	return last
}
