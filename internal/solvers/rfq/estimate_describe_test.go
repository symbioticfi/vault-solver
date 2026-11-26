package rfq

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Classification is unchanged; the description names the revert even when the kind is unknown.
func TestClassifyEstimateRevertDescribesEveryPayload(t *testing.T) {
	word := strings.Repeat("0", 63) + "1"
	for _, tc := range []struct {
		name, data string
		kind       estimateRevertKind
		want       string
	}{
		{"no data", "", estimateRevertUnknown, "no revert data"},
		{"short", "02", estimateRevertUnknown, "malformed revert data 0x02"},
		{"reactor fatal", "02f5c732", estimateRevertFatal, "InvalidFiller()"},
		{"reactor args", "cf479181" + word + word, estimateRevertFatal, "InsufficientBalance(balance=1, needed=1)"},
		{"non-canonical", "02f5c73201", estimateRevertUnknown, "InvalidFiller() (non-canonical encoding)"},
		{"adapter error", "4e6ae978", estimateRevertUnknown, "AccountHasAssets()"},
		{"error string", "08c379a0" + strings.Repeat("0", 62) + "20" + strings.Repeat("0", 62) + "1d" +
			"45524332303a20696e73756666696369656e7420616c6c6f77616e6365000000",
			estimateRevertUnknown, `"ERC20: insufficient allowance"`},
		{"panic", "4e487b71" + strings.Repeat("0", 62) + "11", estimateRevertUnknown, "panic: arithmetic underflow or overflow"},
		{"unknown selector", "deadbeef", estimateRevertUnknown, "unknown error 0xdeadbeef (4 bytes)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.Errorf("estimate: %w", &txmanager.ExecutionRevertError{
				Data: hexutil.MustDecode("0x" + tc.data), Cause: errors.New("execution reverted")})
			kind, got := classifyEstimateRevert(err)
			if kind != tc.kind || got != tc.want {
				t.Fatalf("classifyEstimateRevert = (%s, %q), want (%s, %q)", kind, got, tc.kind, tc.want)
			}
		})
	}
}
