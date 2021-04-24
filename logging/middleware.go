package logging

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/icco/zapdriver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Middleware is a middleware for writing request logs in a stuctured format to
// stackdriver. It uses a chi Chain to make sure all of the deps are properly
// included (RequestID and Recoverer).
func Middleware(log *zap.Logger, projectID string) func(next http.Handler) http.Handler {
	return chi.Chain(
		middleware.RealIP,
		middleware.RequestID,
		Handler(log, projectID),
		middleware.Recoverer,
	).Handler
}

// Handler does the actual work of the http middleware.
func Handler(logger *zap.Logger, projectID string) func(next http.Handler) http.Handler {
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

			payload := zapdriver.NewHTTP(r, nil)
			payload.Status = ww.Status()
			payload.Latency = fmt.Sprintf("%.9fs", latency.Seconds())
			payload.ResponseSize = fmt.Sprintf("%d", ww.BytesWritten())

			var fields []zapcore.Field
			trace, span, sampled := ParseTraceHeader(r.Header.Get("X-Cloud-Trace-Context"))
			if trace != "" {
				fields = append(fields, zapdriver.TraceContext(trace, span, sampled, projectID)...)
			}
			fields = append(fields, zapdriver.HTTP(payload))

			if requestID != "" {
				fields = append(fields, zap.String("request-id", requestID))
			}

			logger.Info("request completed", fields...)
		})
	}
}

// ParseTraceHeader takes a GCP trace header and translates it to a trace,
// span, and whether or not this was sampled.
func ParseTraceHeader(header string) (string, string, bool) {
	if header == "" {
		return "", "", false
	}

	pieces := strings.Split(header, "/")
	if len(pieces) != 2 {
		return "", "", false
	}

	return pieces[0], pieces[1], true
}
