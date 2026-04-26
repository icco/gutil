package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestMiddlewareInjectsLogger verifies that handlers calling FromContext
// after Middleware ran get the configured logger (with the chi
// request_id attached) and not the "unknown" fallback.
func TestMiddlewareInjectsLogger(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	base := zap.New(core).With(zap.String("service", "test"))

	r := chi.NewRouter()
	r.Use(Middleware(base))
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		FromContext(req.Context()).Infow("from-handler")
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/") //nolint:noctx // test
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Logf("close body: %v", err)
	}

	entries := recorded.FilterMessage("from-handler").All()
	if len(entries) != 1 {
		t.Fatalf("want 1 from-handler entry, got %d (recorded=%d)", len(entries), recorded.Len())
	}
	got := entries[0].ContextMap()
	if got["service"] != "test" {
		t.Errorf("service = %v, want test", got["service"])
	}
	if rid, ok := got["request_id"].(string); !ok || rid == "" {
		t.Errorf("request_id missing: %#v", got)
	}
}

// TestInjectLoggerWithoutRequestID verifies the middleware works
// standalone (no chi RequestID upstream): the logger is still injected,
// just without a request_id field.
func TestInjectLoggerWithoutRequestID(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	base := zap.New(core).With(zap.String("service", "test")).Sugar()

	mw := InjectLogger(base)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		FromContext(req.Context()).Infow("from-handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	entries := recorded.FilterMessage("from-handler").All()
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	got := entries[0].ContextMap()
	if got["service"] != "test" {
		t.Errorf("service = %v, want test", got["service"])
	}
	if _, ok := got["request_id"]; ok {
		t.Errorf("unexpected request_id field: %v", got["request_id"])
	}
}

// TestInjectLoggerNilBase makes sure a nil base does not panic and the
// handler still runs.
func TestInjectLoggerNilBase(t *testing.T) {
	called := false
	mw := InjectLogger(nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		// FromContext must not return nil even though we passed nil in.
		if log := FromContext(req.Context()); log == nil {
			t.Error("FromContext returned nil")
		}
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("handler not called")
	}
}
