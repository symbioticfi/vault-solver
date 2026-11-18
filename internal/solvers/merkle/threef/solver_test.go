package threef

import (
	"context"
	"testing"

	"github.com/go-errors/errors"
	"github.com/go-logr/zapr"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogMintIteration(t *testing.T) {
	failure := errors.New("receipt RPC unavailable")
	for _, tc := range []struct {
		name  string
		err   error
		level zapcore.Level
	}{
		{name: "not ready", err: provider.ErrNotReady, level: zap.DebugLevel},
		{name: "wrapped wait", err: errors.Errorf("pending mint: %w", provider.ErrNotReady), level: zap.DebugLevel},
		{name: "joined waits", err: errors.Join(provider.ErrNotReady, provider.ErrNotReady), level: zap.DebugLevel},
		{name: "wrapped joined waits", err: errors.Errorf("mint batch: %w", errors.Join(provider.ErrNotReady, context.Canceled)), level: zap.DebugLevel},
		{name: "failure", err: failure, level: zap.ErrorLevel},
		{name: "timeout without pending transaction", err: context.DeadlineExceeded, level: zap.ErrorLevel},
		{name: "wait and failure", err: errors.Join(provider.ErrNotReady, failure), level: zap.ErrorLevel},
		{name: "wrapped wait and failure", err: errors.Errorf("mint batch: %w", errors.Join(provider.ErrNotReady, failure)), level: zap.ErrorLevel},
		{name: "cancel and failure", err: errors.Join(context.Canceled, failure), level: zap.ErrorLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			ctx := observability.WithLogger(t.Context(), zapr.NewLogger(zap.New(core)))
			logMintIteration(ctx, tc.err)
			entries := logs.All()
			if len(entries) != 1 || entries[0].Level != tc.level {
				t.Fatalf("got logs %+v, want one %s entry", entries, tc.level)
			}
			if tc.level == zap.ErrorLevel && entries[0].ContextMap()["error"] != tc.err.Error() {
				t.Fatalf("error context lost: %+v", entries[0])
			}
		})
	}
}

func TestLogMintIterationIgnoresSuccessAndShutdown(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	ctx := observability.WithLogger(t.Context(), zapr.NewLogger(zap.New(core)))
	logMintIteration(ctx, nil)
	logMintIteration(ctx, context.Canceled)
	if logs.Len() != 0 {
		t.Fatalf("unexpected logs: %+v", logs.All())
	}
}
