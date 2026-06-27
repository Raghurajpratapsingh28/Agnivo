package postgres

import (
	stderrors "errors"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE codes the platform treats specially.
const (
	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
	sqlStateCheckViolation      = "23514"
	sqlStateNotNullViolation    = "23502"
	sqlStateSerializationFail   = "40001"
	sqlStateDeadlockDetected    = "40P01"
	sqlStateLockNotAvailable    = "55P03"
	sqlStateTooManyConnections  = "53300"
)

// classify maps a raw pgx/database error to a platform error Code. It is the
// single place that interprets driver errors, so callers reason in domain terms
// (NotFound, Conflict, Unavailable) rather than SQLSTATEs.
func classify(err error) errors.Code {
	if err == nil {
		return ""
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		return errors.CodeNotFound
	}
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateUniqueViolation:
			return errors.CodeAlreadyExists
		case sqlStateForeignKeyViolation, sqlStateCheckViolation, sqlStateNotNullViolation:
			return errors.CodeFailedPrecond
		case sqlStateSerializationFail, sqlStateDeadlockDetected, sqlStateLockNotAvailable:
			return errors.CodeConflict
		case sqlStateTooManyConnections:
			return errors.CodeExhausted
		}
	}
	return errors.CodeInternal
}

// isTransient reports whether err is a retryable transient database failure.
func isTransient(err error) bool {
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateSerializationFail, sqlStateDeadlockDetected, sqlStateLockNotAvailable, sqlStateTooManyConnections:
			return true
		}
		return false
	}
	// Connection-level disruptions (reset, closed pool) are safe to retry for
	// idempotent transactions.
	return pgconn.SafeToRetry(err)
}

// Translate converts a raw database error into a platform *errors.Error with an
// appropriate code and retry disposition. Repositories wrap their driver errors
// with Translate so business code never inspects SQLSTATEs. Returns nil for nil.
func Translate(err error, message string) error {
	if err == nil {
		return nil
	}
	code := classify(err)
	wrapped := errors.Wrap(err, code, message)
	if isTransient(err) {
		return wrapped.WithRetryable(true)
	}
	return wrapped
}

// IsNotFound reports whether err represents "no rows" / not found.
func IsNotFound(err error) bool { return errors.IsCode(err, errors.CodeNotFound) }

// IsUniqueViolation reports whether err is a unique-constraint conflict.
func IsUniqueViolation(err error) bool { return errors.IsCode(err, errors.CodeAlreadyExists) }
