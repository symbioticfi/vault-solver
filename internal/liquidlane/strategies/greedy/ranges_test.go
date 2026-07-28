package greedy

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestBestRouteCandidatesKeepsUsefulAlternatives(t *testing.T) {
	bestDiscount := common.HexToHash("0x01")
	worseDiscount := common.HexToHash("0x02")
	direct := candidate("direct", "route-1", 90, 100)
	bestPrivate := candidate("best-private", "route-1", 100, 100)
	bestPrivate.DiscountID = &bestDiscount
	worsePrivate := candidate("worse-private", "route-1", 95, 1_000)
	worsePrivate.DiscountID = &worseDiscount
	dominatedPrivate := candidate("dominated-private", "route-1", 80, 100)
	dominatedPrivate.DiscountID = &worseDiscount
	dominatedPrivate.ValidUntil = time.Unix(100, 0)

	got := BestRouteCandidates([]Candidate{direct, bestPrivate, worsePrivate, dominatedPrivate}, 1)
	if len(got) != 3 {
		t.Fatalf("candidates = %+v, want three useful alternatives", got)
	}
	seen := make(map[string]bool, len(got))
	for _, item := range got {
		seen[string(item.ID)] = true
	}
	for _, want := range []string{"direct", "best-private", "worse-private"} {
		if !seen[want] {
			t.Fatalf("candidates = %+v, missing %s", got, want)
		}
	}
	if seen["dominated-private"] {
		t.Fatalf("candidates = %+v, contains dominated private alternative", got)
	}
}
