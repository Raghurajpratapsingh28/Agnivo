package logger

import (
	"context"

	"go.uber.org/zap"
)

type ctxKey int

const (
	loggerKey ctxKey = iota
	requestIDKey
	correlationIDKey
)

// Field keys used consistently across the platform.
const (
	FieldRequestID     = "request_id"
	FieldCorrelationID = "correlation_id"
)

// Into stores a logger in the context.
func Into(ctx context.Context, log *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

// From returns the context logger, or a no-op logger if none is present.
// It automatically enriches the logger with any request/correlation IDs found
// in the context so callers get correlated logs for free.
func From(ctx context.Context) *zap.Logger {
	log, _ := ctx.Value(loggerKey).(*zap.Logger)
	if log == nil {
		log = zap.NewNop()
	}
	if rid := RequestID(ctx); rid != "" {
		log = log.With(zap.String(FieldRequestID, rid))
	}
	if cid := CorrelationID(ctx); cid != "" {
		log = log.With(zap.String(FieldCorrelationID, cid))
	}
	return log
}

// WithRequestID stores the request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request ID from the context, if any.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithCorrelationID stores the correlation ID in the context.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationID returns the correlation ID from the context, if any.
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey).(string)
	return id
}
