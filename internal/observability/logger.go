// Package observability owns process logging, metrics, and health endpoints.
package observability

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger constructs the sole concrete logging backend used by the process.
func NewLogger(debug bool) (logr.Logger, func()) {
	config := zap.NewProductionConfig()
	if debug {
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}
	logger, err := config.Build()
	if err != nil {
		return logr.Discard(), func() {}
	}
	if sentry := initSentry(); sentry != nil {
		logger = logger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewTee(core, sentry)
		}))
	}
	return zapr.NewLogger(logger), func() { _ = logger.Sync() }
}
