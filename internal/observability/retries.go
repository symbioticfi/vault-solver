package observability

import (
	"context"
	"sync"
	"time"

	"github.com/go-errors/errors"
)

// retryEscalation is how long a retried operation must keep failing before it is logged at Error.
const retryEscalation = 2 * time.Minute

// Retries tracks one operation that a loop retries on its next attempt: a poll, a refresh, a
// reconnect. Its failures are logged at Info, which stays out of Sentry, until the operation has
// failed on consecutive attempts for retryEscalation; that failure is logged once at Error, and Ok
// ends the streak. A one-off failure never reaches Sentry, and an outage reaches it once.
//
// Use it only where the next attempt retries the same work. One-shot work (a fill, a broadcast)
// must log at Error every time. The zero value is ready; a nil *Retries logs every failure at Error.
type Retries struct {
	mu       sync.Mutex
	since    time.Time // first failure of the current streak; zero after a success
	failures int
	reported bool             // the streak has been logged at Error
	now      func() time.Time // nil means time.Now
}

// Fail logs a failed attempt. ctx's own cancellation is logged at V(1) and does not count.
func (r *Retries) Fail(ctx context.Context, err error, msg string, keysAndValues ...any) {
	log := Log(ctx)
	if Canceled(ctx, err) {
		log.V(1).Info(msg, append([]any{"error", err.Error()}, keysAndValues...)...)
		return
	}
	if r == nil {
		log.Error(err, msg, keysAndValues...)
		return
	}
	failures, failingFor, escalate := r.fail()
	keysAndValues = append(keysAndValues, "failures", failures, "failingFor", failingFor.String())
	if escalate {
		log.Error(err, msg, keysAndValues...)
		return
	}
	log.Info(msg, append([]any{"error", err.Error()}, keysAndValues...)...)
}

// Ok records a successful attempt and ends the streak. It logs the recovery of a reported one.
func (r *Retries) Ok(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	reported, failures, failingFor := r.reported, r.failures, r.clock().Sub(r.since)
	r.since, r.failures, r.reported = time.Time{}, 0, false
	r.mu.Unlock()
	if reported {
		Log(ctx).Info("retried operation recovered", "failures", failures, "failingFor", failingFor.String())
	}
}

func (r *Retries) fail() (failures int, failingFor time.Duration, escalate bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock()
	if r.failures == 0 {
		r.since = now
	}
	r.failures++
	failingFor = now.Sub(r.since)
	escalate = !r.reported && r.failures > 1 && failingFor >= retryEscalation
	r.reported = r.reported || escalate
	return r.failures, failingFor, escalate
}

func (r *Retries) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// Retryable marks err as coming from a step that its loop retries, for an iteration that mixes such
// steps with one-shot work. IsRetryable reports the mark; the error text and its chain are unchanged.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return retryableError{err}
}

// IsRetryable reports whether err was marked by Retryable.
func IsRetryable(err error) bool {
	var retryable retryableError
	return errors.As(err, &retryable)
}

type retryableError struct{ error }

func (e retryableError) Unwrap() error { return e.error }
