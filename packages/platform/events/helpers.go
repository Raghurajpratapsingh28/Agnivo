package events

import (
	"context"
	"reflect"
	"runtime"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// handlerName derives a stable, human-readable name for a handler for logging
// and dead-lettering. HandlerFunc values report their function name; other
// implementations report their concrete type name.
func handlerName(h Handler) string {
	if hf, ok := h.(HandlerFunc); ok {
		if fn := runtime.FuncForPC(reflect.ValueOf(hf).Pointer()); fn != nil {
			return fn.Name()
		}
		return "events.HandlerFunc"
	}
	t := reflect.TypeOf(h)
	if t == nil {
		return "unknown"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.PkgPath() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return t.Name()
}

// injectCorrelation seeds ctx with the event's correlation ID so handler logs
// and downstream calls stay correlated even on the async delivery path, where
// the originating request context is no longer available.
func injectCorrelation(ctx context.Context, e Event) context.Context {
	if e.CorrelationID == "" {
		return ctx
	}
	return logger.WithCorrelationID(ctx, e.CorrelationID)
}
