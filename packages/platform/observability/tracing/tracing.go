// Package tracing bootstraps OpenTelemetry tracing with an OTLP/HTTP or stdout
// exporter, returning a shutdown function that flushes pending spans.
package tracing

import (
	"context"
	"fmt"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Provider holds the configured tracer and a shutdown hook.
type Provider struct {
	Tracer   trace.Tracer
	shutdown func(context.Context) error
}

// Shutdown flushes and stops the trace provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// New configures the global tracer provider and propagators. When tracing is
// disabled it installs a no-op provider so instrumentation code is always safe.
func New(ctx context.Context, cfg *config.Config) (*Provider, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if !cfg.Observability.Tracing.Enabled {
		return &Provider{Tracer: otel.Tracer(cfg.App.Name)}, nil
	}

	exp, err := newExporter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", cfg.App.Name),
		attribute.String("deployment.environment", string(cfg.App.Environment)),
	))
	if err != nil {
		return nil, fmt.Errorf("tracing: resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Observability.Tracing.Sampler))),
	)
	otel.SetTracerProvider(tp)

	return &Provider{Tracer: tp.Tracer(cfg.App.Name), shutdown: tp.Shutdown}, nil
}

func newExporter(ctx context.Context, cfg *config.Config) (sdktrace.SpanExporter, error) {
	switch cfg.Observability.Tracing.Exporter {
	case "stdout":
		return stdouttrace.New(stdouttrace.WithPrettyPrint())
	default:
		opts := []otlptracehttp.Option{}
		if cfg.Observability.Tracing.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Observability.Tracing.Endpoint))
		}
		if cfg.Observability.Tracing.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exp, err := otlptracehttp.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("tracing: otlp exporter: %w", err)
		}
		return exp, nil
	}
}
