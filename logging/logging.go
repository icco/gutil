package logging

import (
	"fmt"

	"github.com/icco/zapdriver"
	"go.uber.org/zap"
)

// NewLogger creates a logger with stackdriver settings.
func NewLogger(serviceName string) (*zap.SugaredLogger, error) {
	logger, err := zapdriver.NewProductionWithCore(zapdriver.WrapCore(
		zapdriver.ReportAllErrors(true),
		zapdriver.ServiceName(serviceName),
	))

	if err != nil {
		return nil, fmt.Errorf("logger create: %w", err)
	}
	defer logger.Sync()
	logger.Debug("created logger")

	return logger.Sugar(), nil
}

// Must panics if the logger can not be created.
func Must(log *zap.SugaredLogger, err error) *zap.SugaredLogger {
	if err != nil {
		panic(err)
	}

	return log
}
