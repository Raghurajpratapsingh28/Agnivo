package errors

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ZapFields renders err as structured Zap fields for logging integration. It is
// safe to call with any error; non-platform errors yield a minimal field set.
// The returned slice always includes "error" and "error_code".
func ZapFields(err error) []zap.Field {
	if err == nil {
		return nil
	}
	e := From(err)
	fields := make([]zap.Field, 0, len(e.fields)+5)
	fields = append(fields,
		zap.String("error", e.Error()),
		zap.String("error_code", string(e.code)),
		zap.Bool("retryable", e.Retryable()),
	)
	if e.fatal {
		fields = append(fields, zap.Bool("fatal", true))
	}
	if e.op != "" {
		fields = append(fields, zap.String("op", e.op))
	}
	for k, v := range e.fields {
		fields = append(fields, zap.Any(k, v))
	}
	return fields
}

// Log writes err to log at a severity derived from its classification: 5xx and
// fatal errors log at Error, retryable/transient conditions at Warn, and
// client-fault codes at Info. Trace identifiers from ctx are attached so logs
// correlate with spans.
func Log(ctx context.Context, log *zap.Logger, msg string, err error) {
	if err == nil || log == nil {
		return
	}
	e := From(err)
	fields := ZapFields(e)
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		fields = append(fields,
			zap.String("trace_id", sc.TraceID().String()),
			zap.String("span_id", sc.SpanID().String()),
		)
	}

	switch {
	case e.fatal, e.HTTPStatus() >= 500:
		log.Error(msg, fields...)
	case e.Retryable():
		log.Warn(msg, fields...)
	default:
		log.Info(msg, fields...)
	}
}

// RecordSpan marks the active span in ctx as errored with this error and tags
// it with the error code, so failures are visible in trace backends.
func RecordSpan(ctx context.Context, err error) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	e := From(err)
	span.RecordError(e)
	span.SetAttributes(attribute.String("error.code", string(e.code)))
}
