package metrics

import (
	"context"
	"fmt"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric/controller/basic"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
)

type errHandler struct {
	log *zap.SugaredLogger
}

// Handle logs an error to Zap.
func (eh *errHandler) Handle(err error) {
	eh.log.Errorw("otel error", zap.Error(err))
}

// TraceInit sets up a default tracer for open telemetry.
func TraceInit(ctx context.Context, log *zap.SugaredLogger, projectID string) error {
	opts := []texporter.Option{
		texporter.WithProjectID(projectID),
		texporter.WithContext(ctx),
		texporter.WithOnError(func(err error) {
			log.Errorw("stackdriver trace error", zap.Error(err))
		}),
	}
	topts := []trace.Option{}
	tp, err := texporter.NewExportPipeline(opts, topts...)
	if err != nil {
		return fmt.Errorf("trace exporter init: %w", err)
	}
	otel.SetTracerProvider(tp)

	eh := &errHandler{log: log}
	otel.SetErrorHandler(eh)

	return nil
}

// MetricInit sets up a default metric provider for open telemetry.
func MetricInit(ctx context.Context, log *zap.SugaredLogger, projectID string) error {
	opts := []mexporter.Option{
		mexporter.WithProjectID(projectID),
		mexporter.WithContext(ctx),
		mexporter.WithOnError(func(err error) {
			log.Errorw("stackdriver metric error", zap.Error(err))
		}),
	}
	popts := []basic.Option{}

	pusher, err := mexporter.InstallNewPipeline(opts, popts...)
	if err != nil {
		return fmt.Errorf("metric exporter init: %w", err)
	}

	return nil
}
