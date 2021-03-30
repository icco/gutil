package logging

import (
	"fmt"

	"github.com/blendle/zapdriver"
	"go.uber.org/zap"
)

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
