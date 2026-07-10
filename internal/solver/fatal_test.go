package solver

import (
	"context"
	"testing"
	"time"

	"github.com/go-errors/errors"
)

func TestFatalSignalReportsFirstError(t *testing.T) {
	first := errors.New("first component failure")
	second := errors.New("second component failure")
	signal := NewFatalSignal()

	signal.Report(first)
	signal.Report(second)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := signal.Wait(ctx)
	if !errors.Is(err, first) || errors.Is(err, second) {
		t.Fatalf("Wait error = %v, want first reported failure only", err)
	}
}

func TestFatalSignalParentCancellationIsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := NewFatalSignal().Wait(ctx); err != nil {
		t.Fatalf("Wait error = %v, want clean parent cancellation", err)
	}
}

func TestFatalSignalBufferedErrorWinsOverLaterCancellation(t *testing.T) {
	fatalErr := errors.New("buffered component failure")
	for i := range 1_000 {
		signal := NewFatalSignal()
		signal.Report(fatalErr)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if err := signal.Wait(ctx); !errors.Is(err, fatalErr) {
			t.Fatalf("iteration %d: Wait error = %v, want buffered fatal failure", i, err)
		}
	}
}
