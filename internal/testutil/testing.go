// Package testutil contains small assertions and bounded waits shared only by tests.
package testutil

import (
	"testing"
	"time"
)

// NoError fails at the calling test line, retaining an optional diagnostic format.
func NoError(tb testing.TB, err error, format ...string) {
	tb.Helper()
	if err != nil {
		if len(format) != 0 {
			tb.Fatalf(format[0], err)
		}
		tb.Fatal(err)
	}
}

// ReceiveWithin preserves the scenario's timeout and diagnostic; result assertions stay local.
func ReceiveWithin[T any](tb testing.TB, channel <-chan T, timeout time.Duration, message string) T {
	tb.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(timeout):
		tb.Fatal(message)
		var zero T
		return zero
	}
}
