package txmanager

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr/funcr"
)

// A stuck RPC used to produce one error per poll per pending transaction (1,400 events in an hour
// from three pods). A failure streak is now one error at its start, debug while it lasts, and one
// info line with the count and duration when reads recover.
func TestReceiptReadFailuresLogOncePerStreak(t *testing.T) {
	var logs []string
	logger := funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{Verbosity: 1})
	b := &receiptErrorBackend{mockBackend: newMockBackend(), receiptFailures: 3}
	m := New(
		b, mustSigner(t), big.NewInt(11155111),
		Config{
			MaxFeeGwei:          100,
			PollInterval:        time.Millisecond,
			ReplacementInterval: time.Second,
			PendingTimeout:      time.Second,
		},
		logger,
	)
	startManagerForTest(t, m)

	result, accepted := m.SendAsync(t.Context(), Request{
		To: common.HexToAddress("0xabc"), GasLimit: 21_000, Label: "receipt streak",
	})
	if !accepted {
		t.Fatal("transaction was not accepted")
	}
	if got := <-result; got.Err != nil {
		t.Fatalf("result: %v", got.Err)
	}

	count := func(substr string) int {
		n := 0
		for _, entry := range logs {
			if strings.Contains(entry, substr) {
				n++
			}
		}
		return n
	}
	if got := count(`"pending transaction receipt unavailable"`); got != 1 {
		t.Fatalf("error-level receipt log lines = %d, want 1; logs:\n%s", got, strings.Join(logs, "\n"))
	}
	if got := count(`"pending transaction receipt still unavailable"`); got != 2 {
		t.Fatalf("debug follow-up lines = %d, want 2; logs:\n%s", got, strings.Join(logs, "\n"))
	}
	if got := count(`"pending transaction receipt reads recovered"`); got != 1 {
		t.Fatalf("recovery lines = %d, want 1; logs:\n%s", got, strings.Join(logs, "\n"))
	}
	for _, entry := range logs {
		if strings.Contains(entry, "reads recovered") && !strings.Contains(entry, `"consecutiveFailures":3`) {
			t.Fatalf("recovery line should carry the streak length: %s", entry)
		}
	}
}
