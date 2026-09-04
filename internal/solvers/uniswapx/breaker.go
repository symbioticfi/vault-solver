package uniswapx

import (
	"sync"
	"sync/atomic"
	"time"
)

// fillBreaker is the sole owner of remote, local-failure, exclusivity, and startup blocks.
// Solver methods apply logging, metrics, and quote invalidation around these atomic transitions.
type fillBreaker struct {
	remoteUntil atomic.Int64
	localUntil  atomic.Int64
	warmupUntil atomic.Int64

	failureMu    sync.Mutex
	failureTimes []time.Time

	exclusiveUntil    atomic.Int64
	exclusiveUnknown  atomic.Bool
	lastExclusivePoll atomic.Int64
}

func (b *fillBreaker) blocked(now int64) bool {
	return b.remoteUntil.Load() > now || b.localUntil.Load() > now ||
		b.exclusiveUntil.Load() > now || b.warmupUntil.Load() > now
}

func (b *fillBreaker) recordFailure(now time.Time, window time.Duration, maximum int) (bool, int64) {
	b.failureMu.Lock()
	defer b.failureMu.Unlock()
	cutoff := now.Add(-window)
	kept := b.failureTimes[:0]
	for _, failure := range b.failureTimes {
		if failure.After(cutoff) {
			kept = append(kept, failure)
		}
	}
	b.failureTimes = append(kept, now)
	if len(b.failureTimes) < maximum {
		return false, b.localUntil.Load()
	}
	b.failureTimes = nil
	until := now.Add(window).Unix()
	b.localUntil.Store(until)
	return true, until
}

func (b *fillBreaker) recordSuccess() (bool, int64) {
	b.failureMu.Lock()
	hadFailures := len(b.failureTimes) > 0
	b.failureTimes = nil
	b.failureMu.Unlock()
	return hadFailures, b.localUntil.Swap(0)
}

func (b *fillBreaker) openExclusive(until int64) int64 {
	for {
		current := b.exclusiveUntil.Load()
		if current >= until || b.exclusiveUntil.CompareAndSwap(current, until) {
			return max(current, until)
		}
	}
}

func (b *fillBreaker) maxUntil() int64 {
	return max(b.remoteUntil.Load(), b.localUntil.Load(), b.exclusiveUntil.Load())
}
