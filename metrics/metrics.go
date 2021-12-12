package metrics

import (
	"context"
	"fmt"
	"net/http"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.uber.org/zap"
)

type errHandler struct {
	log *zap.SugaredLogger
}

// Handle logs an error to Zap.
func (eh *errHandler) Handle(err error) {
	eh.log.Errorw("otel error", zap.Error(err))
}

// Init sets up a default trace and metric provider.
func Init(ctx context.Context, log *zap.SugaredLogger, projectID, serviceName string) error {
	if err := TraceInit(ctx, log, projectID, serviceName); err != nil {
		return err
	}
	if err := MetricInit(ctx, log, projectID, serviceName); err != nil {
		return err
	}

	return nil
}

// TraceInit sets up a default tracer for open telemetry.
func TraceInit(ctx context.Context, log *zap.SugaredLogger, projectID, serviceName string) error {
	eh := &errHandler{log: log}
	eopts := []texporter.Option{
		texporter.WithProjectID(projectID),
		texporter.WithContext(ctx),
		texporter.WithErrorHandler(eh),
	}
	exporter, err := texporter.New(eopts...)
	if err != nil {
		return fmt.Errorf("trace exporter init: %w", err)
	}

	topts := []trace.TracerProviderOption{
		trace.WithBatcher(exporter),
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceNameKey.String(serviceName))),
	}
	tp := trace.NewTracerProvider(topts...)
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(eh)

	return nil
}

// MetricInit sets up a default metric provider for open telemetry.
func MetricInit(ctx context.Context, log *zap.SugaredLogger, projectID, serviceName string) error {
	opts := []mexporter.Option{
		mexporter.WithProjectID(projectID),
		mexporter.WithOnError(func(err error) {
			log.Errorw("stackdriver metric error", zap.Error(err))
		}),
	}
	popts := []basic.Option{
		basic.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceNameKey.String(serviceName))),
	}

	_, err := mexporter.InstallNewPipeline(opts, popts...)
	if err != nil {
		return fmt.Errorf("metric exporter init: %w", err)
	}

	return nil
}

// Middleware adds a open tracing http.
func Middleware(next http.HandlerFunc) http.Handler {
	return otelhttp.NewHandler(next, "http")
}
