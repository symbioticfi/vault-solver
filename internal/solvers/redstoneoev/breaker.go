package redstoneoev

import (
	"sync"
	"time"
)

// breaker halts bidding when RedStone blacklists our key, or when too many liquidations fail in a
// rolling window (a revert storm bleeds gas + nonce and risks blacklisting — §6.2). Safe for
// concurrent use; `now` is injected so it's testable.
type breaker struct {
	mu          sync.Mutex
	blacklisted bool
	failures    []time.Time
	maxFailures int
	window      time.Duration
}

func newBreaker(maxFailures int, window time.Duration) *breaker {
	return &breaker{maxFailures: maxFailures, window: window}
}

// blacklist permanently trips the breaker (until restart). Called on the `blacklisted` WS frame.
func (b *breaker) blacklist() {
	b.mu.Lock()
	b.blacklisted = true
	b.mu.Unlock()
}

// recordFailure logs a failed settlement and prunes the window.
func (b *breaker) recordFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = append(b.failures, now)
	b.pruneLocked(now)
}

// tripped reports whether bidding must halt.
func (b *breaker) tripped(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.blacklisted {
		return true
	}
	b.pruneLocked(now)
	if b.maxFailures > 0 && len(b.failures) >= b.maxFailures {
		return true
	}
	return false
}

// prune drops failures older than the window. Caller holds the lock.
func (b *breaker) pruneLocked(now time.Time) {
	cutoff := now.Add(-b.window)
	kept := b.failures[:0]
	for _, failure := range b.failures {
		if failure.After(cutoff) {
			kept = append(kept, failure)
		}
	}
	b.failures = kept
}
