package chain

import (
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestHeaderTimeRejectsUnusableDeadlineClock(t *testing.T) {
	for _, header := range []*types.Header{nil, {Time: math.MaxUint64}} {
		if _, err := HeaderTime(header); err == nil {
			t.Fatal("accepted unusable block timestamp")
		}
	}
	got, err := HeaderTime(&types.Header{Time: 1700000000})
	if err != nil || got.Unix() != 1700000000 {
		t.Fatalf("timestamp=%s, error=%v", got, err)
	}
}
