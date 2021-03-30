package logging

import (
	"fmt"

	"github.com/blendle/zapdriver"
	"go.uber.org/zap"
)

// NewLogger creates a logger with stackdriver settings.
func NewLogger(serviceName string) (*zap.Logger, error) {
	logger, err := zapdriver.NewProductionWithCore(zapdriver.WrapCore(
		zapdriver.ReportAllErrors(true),
		zapdriver.ServiceName(serviceName),
	))

	if err != nil {
		return nil, fmt.Errorf("logger create: %w", err)
	}
	defer logger.Sync()
	logger.Debug("created logger")

	return logger, nil
}

// Must panics if the logger can not be created.
func Must(log *zap.Logger, err error) *zap.Logger {
	if err != nil {
		panic(err)
	}

	return log
}
