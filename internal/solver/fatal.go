package solver

import (
	"context"
	"sync"

	"github.com/go-errors/errors"
)

// FatalReporter is the narrow dependency integrations use to surface a fatal child error before
// joining their remaining workers. The root runtime owns the corresponding FatalSignal.
type FatalReporter interface {
	Report(err error)
}

// FatalSignal carries the first asynchronously surfaced fatal error to one root runtime observer.
// Report is non-blocking even when the observer has not started yet.
type FatalSignal struct {
	once sync.Once
	errs chan error
}

// NewFatalSignal constructs a fatal runtime signal.
func NewFatalSignal() *FatalSignal {
	return &FatalSignal{errs: make(chan error, 1)}
}

// Report publishes a fatal runtime error.
func (s *FatalSignal) Report(err error) {
	if err == nil {
		return
	}
	s.once.Do(func() {
		s.errs <- errors.Errorf("runtime component failed: %w", err)
	})
}

// Wait waits for a fatal runtime error or clean parent cancellation.
func (s *FatalSignal) Wait(ctx context.Context) error {
	// Prefer an error that Report already buffered, even when cancellation is also ready.
	select {
	case err := <-s.errs:
		return err
	default:
	}
	select {
	case err := <-s.errs:
		return err
	case <-ctx.Done():
		// Report can race the blocking select; give a concurrently buffered fatal one final priority.
		select {
		case err := <-s.errs:
			return err
		default:
			return nil
		}
	}
}
