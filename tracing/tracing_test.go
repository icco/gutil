package tracing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestEnabled(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", nil, false},
		{"endpoint", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://alloy:4318"}, true},
		{"traces endpoint", map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://alloy:4318/v1/traces"}, true},
		{"exporter none", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://alloy:4318", "OTEL_TRACES_EXPORTER": "none"}, false},
		{"sdk disabled", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://alloy:4318", "OTEL_SDK_DISABLED": "TRUE"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_TRACES_EXPORTER", "OTEL_SDK_DISABLED"} {
				t.Setenv(k, tc.env[k])
			}
			if got := Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInitExportsSpans(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/traces" {
			_, _ = io.Copy(io.Discard, r.Body)
			hits.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")

	ctx := context.Background()
	shutdown, err := Init(ctx, "alice")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, span := otel.Tracer("test").Start(ctx, "op")
	if !span.SpanContext().IsValid() {
		t.Fatal("span context is not valid")
	}
	span.End()

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if hits.Load() == 0 {
		t.Error("no spans were exported")
	}
}

func TestInitWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	ctx := context.Background()
	shutdown, err := Init(ctx, "bob")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	_, span := otel.Tracer("test").Start(ctx, "op")
	defer span.End()
	if !span.SpanContext().IsValid() {
		t.Error("spans should still carry IDs without an exporter")
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(trace.ContextWithSpan(ctx, span), carrier)
	if carrier.Get("traceparent") == "" {
		t.Error("traceparent was not injected")
	}
}
