package otel

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Middleware adds a open tracing http middleware.
func Middleware(next http.HandlerFunc) http.Handler {
	return otelhttp.NewHandler(next, "http")
}
