package otel

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Middleware adds a open tracing http middleware.
func Middleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(
		next,
		"http",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.URL.Path
		}),
	)
}
