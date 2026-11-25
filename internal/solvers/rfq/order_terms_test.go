package rfq

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// Every executable-order rejection names the field that failed and the values compared.
func TestExecutableOrderTermsNameFailedCheck(t *testing.T) {
	executor := common.HexToAddress("0x0000000000000000000000000000000000000010")
	other := common.HexToAddress("0x0000000000000000000000000000000000000077")
	for name, tc := range map[string]struct {
		change func(*executable)
		want   string
	}{
		"filler": {func(e *executable) { e.filler = other },
			"backend filler " + other.Hex() + " does not match signed order filler " + executor.Hex()},
		"deadline": {func(e *executable) { e.deadline = 1 },
			"backend deadline 1 does not match signed order deadline 4102444800"},
		"output count": {func(e *executable) { e.outputs = nil },
			"backend outputs do not match signed order: 0 backend outputs, 1 signed"},
		"output amount": {func(e *executable) { e.outputs[0].Amount = "899999" },
			"backend output 0 (token " + tOut.Hex() + ", recipient 0x0000000000000000000000000000000000000099, amount 899999) " +
				"does not match signed order (" + tOut.Hex() + ", 0x0000000000000000000000000000000000000099, 900000)"},
	} {
		t.Run(name, func(t *testing.T) {
			exec := &executable{filler: executor, deadline: 4_102_444_800, outputs: []backendOut{{
				Token: tOut.Hex(), Amount: "900000", Recipient: "0x0000000000000000000000000000000000000099",
			}}}
			tc.change(exec)
			_, _, err := executableOrderTerms(exec, sampleOrder(), executor)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	order := sampleOrder()
	order.Request.AmountIn = big.NewInt(0)
	exec := &executable{filler: executor, deadline: 4_102_444_800}
	if _, _, err := executableOrderTerms(exec, order, executor); err == nil ||
		!strings.Contains(err.Error(), "signed order has invalid input: tokenIn "+tIn.Hex()+" amountIn 0") {
		t.Fatalf("invalid input err = %v", err)
	}
}
