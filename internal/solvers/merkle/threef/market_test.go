package threef

import (
	"context"
	"strings"
	"testing"

	"github.com/go-errors/errors"
)

func TestReadFailureKeepsCancellationAndDropsTransport(t *testing.T) {
	if err := readFailure("policy budget read", errors.Errorf("rpc: %w", context.Canceled)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v, want it to stay context.Canceled", err)
	}
	err := readFailure("policy budget read", errors.New(`Post "https://rpc.example/key-123": connection refused`))
	if err.Error() != "3f policy budget read failed" || strings.Contains(err.Error(), "key-123") {
		t.Fatalf("transport read = %q, want the cause dropped", err)
	}
}
