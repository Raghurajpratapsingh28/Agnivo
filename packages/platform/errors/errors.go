// Package errors provides the canonical, production-grade error framework used
// by every executable. It unifies typed error codes, error wrapping, HTTP
// status mapping, retry/fatal classification, structured logging fields, and
// trace correlation behind a single *Error type.
//
// Business and platform code should construct errors with the New* helpers (or
// wrap existing ones with Wrap) and let the transport layer translate them via
// HTTPStatus. Equality and unwrapping follow the standard library contract, so
// errors.Is and errors.As work as expected.
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// Code is a stable, machine-readable error classification. Codes are part of
// the public API contract and must not change meaning once published.
type Code string

// The canonical error codes. Each maps to a default HTTP status and retry
// disposition (see codeMeta).
const (
	CodeInternal        Code = "internal"
	CodeUnknown         Code = "unknown"
	CodeInvalidArgument Code = "invalid_argument"
	CodeValidation      Code = "validation"
	CodeNotFound        Code = "not_found"
	CodeAlreadyExists   Code = "already_exists"
	CodeConflict        Code = "conflict"
	CodeUnauthenticated Code = "unauthenticated"
	CodePermission      Code = "permission_denied"
	CodeRateLimited     Code = "rate_limited"
	CodeTimeout         Code = "timeout"
	CodeUnavailable     Code = "unavailable"
	CodeCanceled        Code = "canceled"
	CodeNotImplemented  Code = "not_implemented"
	CodeFailedPrecond   Code = "failed_precondition"
	CodeExhausted       Code = "resource_exhausted"
)

// meta describes the default transport and control-flow semantics for a code.
type meta struct {
	status    int
	retryable bool
}

// codeMeta is the single source of truth mapping codes to HTTP status and
// default retryability. Centralizing it keeps transport mapping consistent.
var codeMeta = map[Code]meta{
	CodeInternal:        {http.StatusInternalServerError, false},
	CodeUnknown:         {http.StatusInternalServerError, false},
	CodeInvalidArgument: {http.StatusBadRequest, false},
	CodeValidation:      {http.StatusUnprocessableEntity, false},
	CodeNotFound:        {http.StatusNotFound, false},
	CodeAlreadyExists:   {http.StatusConflict, false},
	CodeConflict:        {http.StatusConflict, false},
	CodeUnauthenticated: {http.StatusUnauthorized, false},
	CodePermission:      {http.StatusForbidden, false},
	CodeRateLimited:     {http.StatusTooManyRequests, true},
	CodeTimeout:         {http.StatusGatewayTimeout, true},
	CodeUnavailable:     {http.StatusServiceUnavailable, true},
	CodeCanceled:        {499, false}, // client closed request
	CodeNotImplemented:  {http.StatusNotImplemented, false},
	CodeFailedPrecond:   {http.StatusPreconditionFailed, false},
	CodeExhausted:       {http.StatusTooManyRequests, true},
}

// Error is the platform's structured error type. It is safe to wrap with %w and
// to compare with errors.Is/As. The zero value is not valid; always construct
// via New, Newf, or Wrap.
type Error struct {
	code    Code
	message string         // user-facing, safe to surface to clients
	cause   error          // wrapped underlying error, may be nil
	fields  map[string]any // structured context for logging; never returned to clients
	op      string         // logical operation (e.g. "repo.User.Get") for tracing
	// retryable / fatal override the code defaults when explicitly set.
	retryableSet bool
	retryable    bool
	fatal        bool
}

// Error implements the error interface, producing a single-line, log-friendly
// representation. It includes the wrapped cause when present.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b []byte
	if e.op != "" {
		b = append(b, e.op...)
		b = append(b, ": "...)
	}
	b = append(b, string(e.code)...)
	if e.message != "" {
		b = append(b, ": "...)
		b = append(b, e.message...)
	}
	if e.cause != nil {
		b = append(b, ": "...)
		b = append(b, e.cause.Error()...)
	}
	return string(b)
}

// Unwrap returns the wrapped cause, enabling errors.Is/As traversal.
func (e *Error) Unwrap() error { return e.cause }

// Is reports whether target is an *Error with the same code. This makes
// sentinel comparisons (errors.Is(err, errors.NotFound(""))) match on code.
func (e *Error) Is(target error) bool {
	var t *Error
	if !stderrors.As(target, &t) {
		return false
	}
	return e.code == t.code
}

// Code returns the error's classification, resolving wrapped *Error causes.
func (e *Error) Code() Code { return e.code }

// Message returns the user-facing message.
func (e *Error) Message() string { return e.message }

// Op returns the logical operation annotation, if any.
func (e *Error) Op() string { return e.op }

// HTTPStatus maps the error to an HTTP status code.
func (e *Error) HTTPStatus() int {
	if m, ok := codeMeta[e.code]; ok {
		return m.status
	}
	return http.StatusInternalServerError
}

// Retryable reports whether the operation may succeed if retried. An explicit
// override (WithRetryable) takes precedence over the code default.
func (e *Error) Retryable() bool {
	if e.retryableSet {
		return e.retryable
	}
	if m, ok := codeMeta[e.code]; ok {
		return m.retryable
	}
	return false
}

// Fatal reports whether the error should halt the surrounding process or
// pipeline rather than be retried or degraded.
func (e *Error) Fatal() bool { return e.fatal }

// Fields returns a copy of the structured context attached to the error,
// suitable for structured logging. The returned map is never nil.
func (e *Error) Fields() map[string]any {
	out := make(map[string]any, len(e.fields)+2)
	for k, v := range e.fields {
		out[k] = v
	}
	out["error_code"] = string(e.code)
	if e.op != "" {
		out["op"] = e.op
	}
	return out
}

// clone returns a shallow copy so With* builders never mutate shared errors.
func (e *Error) clone() *Error {
	c := *e
	if e.fields != nil {
		c.fields = make(map[string]any, len(e.fields))
		for k, v := range e.fields {
			c.fields[k] = v
		}
	}
	return &c
}

// WithField attaches a single structured key/value for logging and returns a
// new error (the receiver is not mutated).
func (e *Error) WithField(key string, value any) *Error {
	c := e.clone()
	if c.fields == nil {
		c.fields = make(map[string]any, 1)
	}
	c.fields[key] = value
	return c
}

// WithFields attaches multiple structured key/values and returns a new error.
func (e *Error) WithFields(fields map[string]any) *Error {
	c := e.clone()
	if c.fields == nil {
		c.fields = make(map[string]any, len(fields))
	}
	for k, v := range fields {
		c.fields[k] = v
	}
	return c
}

// WithOp annotates the error with the logical operation that produced it. Op
// annotations form a lightweight call trace as errors propagate upward.
func (e *Error) WithOp(op string) *Error {
	c := e.clone()
	c.op = op
	return c
}

// WithRetryable explicitly overrides the code's default retry disposition.
func (e *Error) WithRetryable(retryable bool) *Error {
	c := e.clone()
	c.retryableSet = true
	c.retryable = retryable
	return c
}

// AsFatal marks the error as fatal and returns a new error.
func (e *Error) AsFatal() *Error {
	c := e.clone()
	c.fatal = true
	return c
}

// New constructs an *Error with the given code and user-facing message.
func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

// Newf constructs an *Error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{code: code, message: fmt.Sprintf(format, args...)}
}

// Wrap wraps cause with the given code and message, preserving the original via
// Unwrap. If cause is nil, Wrap returns nil so it can be used inline.
func Wrap(cause error, code Code, message string) *Error {
	if cause == nil {
		return nil
	}
	return &Error{code: code, message: message, cause: cause}
}

// Wrapf wraps cause with a formatted message.
func Wrapf(cause error, code Code, format string, args ...any) *Error {
	if cause == nil {
		return nil
	}
	return &Error{code: code, message: fmt.Sprintf(format, args...), cause: cause}
}

// From coerces any error into an *Error. If err already is (or wraps) an
// *Error, that is returned unchanged; otherwise it is wrapped as CodeInternal.
// From(nil) returns nil.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if stderrors.As(err, &e) {
		return e
	}
	return &Error{code: CodeInternal, message: "internal error", cause: err}
}
