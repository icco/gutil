package logging

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/blendle/zapdriver"
	"github.com/felixge/httpsnoop"
)

// LoggingMiddleware is a middleware for writing request logs in a stuctured
// format to stackdriver.
func LoggingMiddleware(log *zap.Logger, projectID string) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := zap.NewHTTP(r, w)
			m := httpsnoop.CaptureMetrics(handler, w, r)

			payload.Status = strconv.Itoa(m.Code)
			payload.Latency = fmt.Sprintf("%.9fs", m.Duration.Seconds())
			payload.ResponseSize = strconv.FormatInt(m.Written, 10)

			trace, span, sampled := ParseTraceHeader(r.Header.Get("X-Cloud-Trace-Context"))

			log.Info("completed request", zapdriver.HTTP(payload), zapdriver.TraceContext(trace, span, sampled, projectID)...)
		})
	}
}

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
