package errors

import stderrors "errors"

// The standard library error primitives are re-exported so callers depend on a
// single errors package. These behave identically to the stdlib equivalents.
var (
	// Is reports whether any error in err's chain matches target.
	Is = stderrors.Is
	// As finds the first error in err's chain that matches target.
	As = stderrors.As
	// Unwrap returns the result of calling Unwrap on err.
	Unwrap = stderrors.Unwrap
	// Join combines multiple errors into a single error.
	Join = stderrors.Join
)

// CodeOf returns the classification of err, resolving wrapped *Error values.
// Plain (non-platform) errors are reported as CodeInternal.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var e *Error
	if stderrors.As(err, &e) {
		return e.code
	}
	return CodeInternal
}

// HTTPStatusOf returns the HTTP status mapping for err.
func HTTPStatusOf(err error) int {
	return From(err).HTTPStatus()
}

// IsCode reports whether err (or anything it wraps) carries the given code.
func IsCode(err error, code Code) bool {
	var e *Error
	if stderrors.As(err, &e) {
		return e.code == code
	}
	return false
}

// IsRetryable reports whether err is safe to retry. Non-platform errors are
// treated as non-retryable.
func IsRetryable(err error) bool {
	var e *Error
	if stderrors.As(err, &e) {
		return e.Retryable()
	}
	return false
}

// IsFatal reports whether err was explicitly marked fatal.
func IsFatal(err error) bool {
	var e *Error
	if stderrors.As(err, &e) {
		return e.Fatal()
	}
	return false
}

// Convenience constructors for the common codes. Each returns an *Error so the
// fluent With* builders remain available at the call site.

// Internal builds a CodeInternal error.
func Internal(message string) *Error { return New(CodeInternal, message) }

// InvalidArgument builds a CodeInvalidArgument error.
func InvalidArgument(message string) *Error { return New(CodeInvalidArgument, message) }

// Validation builds a CodeValidation error.
func Validation(message string) *Error { return New(CodeValidation, message) }

// NotFound builds a CodeNotFound error.
func NotFound(message string) *Error { return New(CodeNotFound, message) }

// AlreadyExists builds a CodeAlreadyExists error.
func AlreadyExists(message string) *Error { return New(CodeAlreadyExists, message) }

// Conflict builds a CodeConflict error.
func Conflict(message string) *Error { return New(CodeConflict, message) }

// Unauthenticated builds a CodeUnauthenticated error.
func Unauthenticated(message string) *Error { return New(CodeUnauthenticated, message) }

// PermissionDenied builds a CodePermission error.
func PermissionDenied(message string) *Error { return New(CodePermission, message) }

// RateLimited builds a CodeRateLimited error.
func RateLimited(message string) *Error { return New(CodeRateLimited, message) }

// Timeout builds a CodeTimeout error.
func Timeout(message string) *Error { return New(CodeTimeout, message) }

// Unavailable builds a CodeUnavailable error.
func Unavailable(message string) *Error { return New(CodeUnavailable, message) }

// NotImplemented builds a CodeNotImplemented error.
func NotImplemented(message string) *Error { return New(CodeNotImplemented, message) }

// FailedPrecondition builds a CodeFailedPrecond error.
func FailedPrecondition(message string) *Error { return New(CodeFailedPrecond, message) }
