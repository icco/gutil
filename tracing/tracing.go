// Package tracing installs an OpenTelemetry tracer provider that exports
// spans over OTLP/HTTP when the standard OTEL_EXPORTER_OTLP_* environment
// variables point at a collector.
package tracing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// ShutdownFunc flushes buffered spans and stops the tracer provider.
type ShutdownFunc func(context.Context) error

// Init sets the global tracer provider and W3C trace context propagator for
// service. Spans are exported over OTLP/HTTP only when
// OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is set,
// OTEL_TRACES_EXPORTER is not "none", and OTEL_SDK_DISABLED is not "true".
// Otherwise spans are still created, so logs carry trace IDs, but nothing is
// exported. OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the
// resource, and OTEL_TRACES_SAMPLER selects the sampler.
func Init(ctx context.Context, service string) (ShutdownFunc, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(service)),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithFromEnv(),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("tracing resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if Enabled() {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// Enabled reports whether the environment configures span export.
func Enabled() bool {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return false
	}
	if strings.EqualFold(os.Getenv("OTEL_TRACES_EXPORTER"), "none") {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}
