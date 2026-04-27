package logging

import (
	"errors"
	"fmt"
	"syscall"

	"go.uber.org/zap"
)

// NewLogger creates a production zap logger tagged with the given service
// name.
//
// Callers should arrange to flush the logger at process exit themselves,
// typically with:
//
//	log := logging.Must(logging.NewLogger("svc"))
//	defer logging.Sync(log)
//
// Note that zap's Sync() returns syscall.EINVAL or syscall.ENOTTY when the
// underlying writer is a non-syncable stream such as stderr/stdout (common
// under Linux/Docker). Those errors are benign; use [Sync] to ignore them.
func NewLogger(serviceName string) (*zap.SugaredLogger, error) {
	config := zap.NewProductionConfig()
	config.Level.SetLevel(zap.DebugLevel)

	logger, err := config.Build(zap.Fields(zap.String("service", serviceName)))
	if err != nil {
		return nil, fmt.Errorf("logger create: %w", err)
	}

	return logger.Sugar(), nil
}

// Must panics if the logger can not be created.
func Must(log *zap.SugaredLogger, err error) *zap.SugaredLogger {
	if err != nil {
		panic(err)
	}

	return log
}

// Sync flushes any buffered log entries, ignoring the benign EINVAL/ENOTTY
// errors that zap returns when the underlying writer is stderr/stdout (or
// any other non-syncable stream). It is intended to be used as
// `defer logging.Sync(log)` at process exit.
func Sync(logger *zap.SugaredLogger) {
	if logger == nil {
		return
	}
	if err := logger.Sync(); err != nil && !isBenignSyncErr(err) {
		logger.Debugw("could not sync logger", "error", err)
	}
}

// isBenignSyncErr reports whether err is one of the well-known errors that
// Sync returns when called on stdout/stderr-backed cores. Those streams
// cannot be fsync'd, so the error carries no useful signal.
func isBenignSyncErr(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTTY)
}
