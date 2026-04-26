package logging

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Middleware is a middleware for writing structured request logs. It uses a chi
// Chain to make sure all of the deps are properly included (RequestID and
// Recoverer).
func Middleware(log *zap.Logger) func(next http.Handler) http.Handler {
	return chi.Chain(
		middleware.RealIP,
		middleware.RequestID,
		Handler(log),
		middleware.Recoverer,
	).Handler
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

			logger.Info("request completed", fields...)
		})
	}
}
