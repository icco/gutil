package otel

import (
	"context"
	"fmt"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.uber.org/zap"
)

type errHandler struct {
	log   *zap.SugaredLogger
	owner string
}

// Handle logs an error to Zap.
func (eh *errHandler) Handle(err error) {
	eh.log.Errorw(fmt.Sprintf("otel %s error", eh.owner), zap.Error(err))
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
	traceEh := &errHandler{log: log, owner: "gcp trace"}
	eopts := []texporter.Option{
		texporter.WithProjectID(projectID),
		texporter.WithContext(ctx),
		texporter.WithErrorHandler(traceEh),
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
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Fatalw("shutting down tracer provider error", zap.Error(err))
		}
	}()

	oeh := &errHandler{log: log, owner: "trace"}
	otel.SetErrorHandler(oeh)

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
