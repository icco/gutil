package logging

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/blendle/zapdriver"
	"github.com/felixge/httpsnoop"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Middleware is a middleware for writing request logs in a stuctured
// format to stackdriver.
func Middleware(log *zap.Logger, projectID string) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := zapdriver.NewHTTP(r, nil)
			m := httpsnoop.CaptureMetrics(handler, w, r)
			payload.Status = m.Code
			payload.Latency = fmt.Sprintf("%.9fs", m.Duration.Seconds())
			payload.ResponseSize = strconv.FormatInt(m.Written, 10)

			var fields []zapcore.Field
			trace, span, sampled := ParseTraceHeader(r.Header.Get("X-Cloud-Trace-Context"))
			if trace != "" {
				fields = append(fields, zapdriver.TraceContext(trace, span, sampled, projectID)...)
			}
			fields = append(fields, zapdriver.HTTP(payload))

			log.Info("completed request", fields...)
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
