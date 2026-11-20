package observability

import (
	"context"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func retriesContext(t *testing.T) (context.Context, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.DebugLevel)
	return WithLogger(t.Context(), zapr.NewLogger(zap.New(core))), logs
}

func takeLevels(logs *observer.ObservedLogs) []zapcore.Level {
	var levels []zapcore.Level
	for _, entry := range logs.TakeAll() {
		levels = append(levels, entry.Level)
	}
	return levels
}

func assertLevels(t *testing.T, logs *observer.ObservedLogs, want ...zapcore.Level) {
	t.Helper()
	got := takeLevels(logs)
	if len(got) != len(want) {
		t.Fatalf("levels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels = %v, want %v", got, want)
		}
	}
}

func TestRetriesEscalateOnceAfterConsecutiveFailures(t *testing.T) {
	ctx, logs := retriesContext(t)
	now := time.Unix(0, 0)
	retries := &Retries{now: func() time.Time { return now }}
	outage := errors.New("backend: list open orders: 500 Internal Server Error")

	retries.Fail(ctx, outage, "poll open orders", "k", "v")
	now = now.Add(retryEscalation - time.Second)
	retries.Fail(ctx, outage, "poll open orders", "k", "v")
	assertLevels(t, logs, zap.InfoLevel, zap.InfoLevel)

	now = now.Add(time.Second)
	retries.Fail(ctx, outage, "poll open orders", "k", "v")
	escalated := logs.All()[0].ContextMap()
	if escalated["k"] != "v" || escalated["error"] != outage.Error() || escalated["failures"] != int64(3) {
		t.Fatalf("escalation lost fields: %v", escalated)
	}
	retries.Fail(ctx, outage, "poll open orders")
	assertLevels(t, logs, zap.ErrorLevel, zap.InfoLevel)

	retries.Ok(ctx)
	if got := logs.TakeAll(); len(got) != 1 || got[0].Message != "retried operation recovered" {
		t.Fatalf("recovery = %+v", got)
	}
	now = now.Add(time.Hour)
	retries.Fail(ctx, outage, "poll open orders")
	assertLevels(t, logs, zap.InfoLevel)
}

func TestRetriesSuccessResetsTheStreak(t *testing.T) {
	ctx, logs := retriesContext(t)
	now := time.Unix(0, 0)
	retries := &Retries{now: func() time.Time { return now }}
	blip := errors.New("rpc: timeout")
	for range 10 {
		retries.Fail(ctx, blip, "quote refresh: read routes")
		now = now.Add(time.Minute)
		retries.Ok(ctx)
	}
	for _, level := range takeLevels(logs) {
		if level != zap.InfoLevel {
			t.Fatalf("an isolated failure was logged at %s", level)
		}
	}
}

func TestRetriesSkipCancellation(t *testing.T) {
	ctx, logs := retriesContext(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	now := time.Unix(0, 0)
	retries := &Retries{now: func() time.Time { return now }}
	retries.Fail(canceled, errors.Errorf("poll: %w", context.Canceled), "order poll failed")
	now = now.Add(time.Hour)
	retries.Fail(ctx, errors.New("boom"), "order poll failed")
	assertLevels(t, logs, zap.DebugLevel, zap.InfoLevel)
}

func TestNilRetriesLogEveryFailureAtError(t *testing.T) {
	ctx, logs := retriesContext(t)
	var retries *Retries
	retries.Fail(ctx, errors.New("boom"), "3f mint iteration failed")
	retries.Ok(ctx)
	assertLevels(t, logs, zap.ErrorLevel)
}

func TestRetryableKeepsTheError(t *testing.T) {
	cause := errors.Errorf("read: %w", context.DeadlineExceeded)
	err := Retryable(cause)
	if !IsRetryable(errors.Errorf("tick: %w", err)) || IsRetryable(cause) || Retryable(nil) != nil {
		t.Fatal("the retryable mark is wrong")
	}
	if err.Error() != cause.Error() || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Retryable changed the error: %v", err)
	}
}
