package logging

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

type loggerKeyType int

const loggerKey loggerKeyType = iota

var (
	fallbackLoggerOnce sync.Once
	fallbackLogger     *zap.SugaredLogger
)

// fallback returns a process-wide "unknown" logger, built lazily on first
// use. Caching avoids re-allocating a fresh zap core on every FromContext
// miss (which previously also triggered a spurious Sync() per call).
func fallback() *zap.SugaredLogger {
	fallbackLoggerOnce.Do(func() {
		fallbackLogger = Must(NewLogger("unknown"))
	})
	return fallbackLogger
}

// NewContext wraps a context with a logger with default fields.
func NewContext(ctx context.Context, logger *zap.SugaredLogger, fields ...interface{}) context.Context {
	return context.WithValue(ctx, loggerKey, logger.With(fields...))
}

// FromContext returns a logger stored in a context.
//
// If the context has no logger, FromContext returns a process-wide
// "unknown" fallback logger. The fallback is constructed once and reused,
// so repeated FromContext calls on bare contexts do not re-allocate a
// fresh zap core each time.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		ctx = context.TODO()
	}

	if ctxLogger, ok := ctx.Value(loggerKey).(*zap.SugaredLogger); ok {
		return ctxLogger
	}

	return fallback()
}
