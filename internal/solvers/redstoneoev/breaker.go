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
	failures    map[failureKey]time.Time
	serial      uint64
	maxFailures int
	window      time.Duration
}

// A numbered key identifies a failure without an upstream auction ID.
type failureKey struct {
	auction string
	serial  uint64
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
	b.serial++
	b.record(failureKey{serial: b.serial}, now)
}

func (b *breaker) recordFailureOnce(id string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.record(failureKey{auction: id}, now)
}

// record requires the lock and stores each counted event exactly once.
func (b *breaker) record(key failureKey, now time.Time) bool {
	b.prune(now)
	if _, exists := b.failures[key]; exists {
		return false
	}
	if b.failures == nil {
		b.failures = make(map[failureKey]time.Time)
	}
	b.failures[key] = now
	return true
}

// tripped reports whether bidding must halt, with a reason.
func (b *breaker) tripped(now time.Time) (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.blacklisted {
		return true, "api key blacklisted"
	}
	b.prune(now)
	if b.maxFailures > 0 && len(b.failures) >= b.maxFailures {
		return true, "failed-liquidation rate-limit"
	}
	return false, ""
}

// prune drops failures older than the window. Caller holds the lock.
func (b *breaker) prune(now time.Time) {
	cutoff := now.Add(-b.window)
	for key, observedAt := range b.failures {
		if !observedAt.After(cutoff) {
			delete(b.failures, key)
		}
	}
}
