package bridgefacilitator

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

func TestDecodeAddrMarksOnlyZeroAsUnconfigured(t *testing.T) {
	unpack := func(data []byte) (common.Address, error) { return common.BytesToAddress(data), nil }
	_, zeroErr := decodeAddr(chain.CallResult{Success: true, ReturnData: make([]byte, 32)}, unpack, "address")
	_, revertErr := decodeAddr(chain.CallResult{}, unpack, "address")
	if !errors.Is(zeroErr, errAdapterUnconfigured) {
		t.Fatalf("zero address error = %v", zeroErr)
	}
	if revertErr == nil || errors.Is(revertErr, errAdapterUnconfigured) {
		t.Fatalf("revert error = %v", revertErr)
	}
}
