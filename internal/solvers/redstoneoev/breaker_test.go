package redstoneoev

import (
	"testing"
	"time"
)

func TestBreakerBlacklistHalts(t *testing.T) {
	b := newBreaker(3, time.Hour)
	now := time.Unix(1_000_000, 0)
	if tripped, _ := b.tripped(now); tripped {
		t.Fatal("fresh breaker must not be tripped")
	}
	b.blacklist()
	tripped, why := b.tripped(now)
	if !tripped || why != "api key blacklisted" {
		t.Fatalf("blacklist must trip: %v %q", tripped, why)
	}
}

func TestBreakerFailureRateLimit(t *testing.T) {
	b := newBreaker(3, time.Hour)
	base := time.Unix(2_000_000, 0)
	b.recordFailure(base)
	b.recordFailure(base.Add(time.Minute))
	if tripped, _ := b.tripped(base.Add(2 * time.Minute)); tripped {
		t.Fatal("2 failures < 3 must not trip")
	}
	b.recordFailure(base.Add(3 * time.Minute))
	tripped, why := b.tripped(base.Add(4 * time.Minute))
	if !tripped || why != "failed-liquidation rate-limit" {
		t.Fatalf("3 failures must trip: %v %q", tripped, why)
	}
}

func TestBreakerWindowPrunes(t *testing.T) {
	b := newBreaker(3, time.Hour)
	base := time.Unix(3_000_000, 0)
	b.recordFailure(base)
	b.recordFailure(base.Add(time.Minute))
	b.recordFailure(base.Add(2 * time.Minute))
	// All three are within the window -> tripped.
	if tripped, _ := b.tripped(base.Add(3 * time.Minute)); !tripped {
		t.Fatal("3 in-window failures must trip")
	}
	// Two hours later they're all pruned -> not tripped.
	if tripped, _ := b.tripped(base.Add(2 * time.Hour)); tripped {
		t.Fatal("failures older than the window must be pruned")
	}
}
