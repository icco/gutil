package logging

import (
	"context"

	"go.uber.org/zap"
)

type loggerKeyType int

const loggerKey loggerKeyType = iota

// NewContext wraps a context with a logger with default fields.
func NewContext(ctx context.Context, logger *zap.SugaredLogger, fields ...interface{}) context.Context {
	return context.WithValue(ctx, loggerKey, logger.With(fields...))
}

// FromContext returns a logger stored in a context.
func FromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		ctx = context.TODO()
	}

	if ctxLogger, ok := ctx.Value(loggerKey).(*zap.SugaredLogger); ok {
		return ctxLogger
	}

	return Must(NewLogger("unknown"))
}
