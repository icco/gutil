// Command otel-example is a tiny HTTP server that wires gutil's logging
// middleware together with OpenTelemetry HTTP instrumentation. It prints
// spans to stdout and emits structured request logs whose trace_id and
// span_id match the spans, so you can see log <-> trace correlation in
// action without standing up a backend.
//
// Run it with:
//
//	go run ./examples/otel
//	curl -i http://localhost:8080/hello
package main

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/icco/gutil/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const serviceName = "gutil-otel-example"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tp, err := newTracerProvider(serviceName)
	if err != nil {
		stdlog.Fatalf("tracer provider: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			stdlog.Printf("shutdown tracer provider: %v", err)
		}
	}()

	otel.SetTracerProvider(tp)
	// W3C TraceContext propagation. Without this, incoming `traceparent`
	// headers are ignored and outgoing requests don't carry trace context.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger := logging.Must(logging.NewLogger(serviceName))

	r := chi.NewRouter()
	// gutil's middleware reads the active span out of r.Context() and
	// stamps trace_id / span_id onto every request log. otelhttp.NewHandler
	// (below) is what puts the span there in the first place.
	r.Use(logging.Middleware(logger.Desugar()))

	r.Get("/hello", func(w http.ResponseWriter, req *http.Request) {
		// Anything you log via the logger from r.Context() can also be
		// decorated with trace_id/span_id if you propagate it that way;
		// here we just write a body so curl has something to show.
		fmt.Fprintln(w, "hello world")
	})

	srv := &http.Server{
		Addr: ":8080",
		// otelhttp creates a server span per request and injects it into
		// the request context that downstream handlers (and gutil's
		// middleware) see.
		Handler:           otelhttp.NewHandler(r, "http.server"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Infof("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("server shutdown: %v", err)
	}
}

func newTracerProvider(name string) (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("stdout exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(name),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	), nil
}
