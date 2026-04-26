package logging

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Middleware is a middleware for writing structured request logs. It uses a chi
// Chain to make sure all of the deps are properly included (RequestID,
// per-request logger injection, and Recoverer).
//
// The injected logger is what FromContext returns inside handlers, so
// downstream code gets the application's configured logger (decorated
// with the chi request_id) instead of the "unknown" fallback FromContext
// constructs when the context has no logger.
func Middleware(log *zap.Logger) func(next http.Handler) http.Handler {
	return chi.Chain(
		middleware.RealIP,
		middleware.RequestID,
		InjectLogger(log.Sugar()),
		Handler(log),
		middleware.Recoverer,
	).Handler
}

// InjectLogger attaches base to the request context so handlers calling
// FromContext receive base (decorated with the chi request_id when
// present) instead of the "unknown" logger FromContext builds for empty
// contexts.
//
// It is safe to use without chi/middleware.RequestID; in that case the
// request_id field is simply omitted. A nil base is replaced with a
// no-op logger so handlers never receive nil.
func InjectLogger(base *zap.SugaredLogger) func(next http.Handler) http.Handler {
	if base == nil {
		base = zap.NewNop().Sugar()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fields := make([]any, 0, 2)
			if id, ok := r.Context().Value(middleware.RequestIDKey).(string); ok && id != "" {
				fields = append(fields, "request_id", id)
			}
			ctx := NewContext(r.Context(), base, fields...)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Handler does the actual work of the http middleware.
func Handler(logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			var requestID string
			if reqID := r.Context().Value(middleware.RequestIDKey); reqID != nil {
				requestID = reqID.(string)
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			latency := time.Since(start)

			fields := []zapcore.Field{
				zap.String("http.method", r.Method),
				zap.String("http.url", r.URL.String()),
				zap.String("http.remote", r.RemoteAddr),
				zap.String("http.user_agent", r.UserAgent()),
				zap.String("http.referer", r.Referer()),
				zap.String("http.proto", r.Proto),
				zap.Int("http.status", ww.Status()),
				zap.Int("http.response_size", ww.BytesWritten()),
				zap.Duration("http.latency", latency),
			}

			if requestID != "" {
				fields = append(fields, zap.String("request-id", requestID))
			}

			// Correlate logs with the active OpenTelemetry span (if any).
			// The app is responsible for installing a propagator/tracer; we
			// just read whatever ended up on the request context.
			if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
				fields = append(fields,
					zap.String("trace_id", sc.TraceID().String()),
					zap.String("span_id", sc.SpanID().String()),
					zap.Bool("trace_sampled", sc.IsSampled()),
				)
			}

			logger.Info("request completed", fields...)
		})
	}
}
