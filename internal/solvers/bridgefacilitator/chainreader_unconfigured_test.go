package bridgefacilitator

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
)

// A zero address means the adapter is not wired up yet, which discovery skips quietly; a reverted or
// undecodable read is a real failure and must not carry the same tag.
func TestDecodeAddrTagsOnlyZeroAddressAsUnconfigured(t *testing.T) {
	unpack := func(b []byte) (common.Address, error) { return common.BytesToAddress(b), nil }

	_, err := decodeAddr(chain.CallResult{Success: true, ReturnData: make([]byte, 32)}, unpack, "adapter.offerSigner()")
	if !errors.Is(err, errAdapterUnconfigured) {
		t.Fatalf("zero address: err = %v, want errAdapterUnconfigured", err)
	}

	_, err = decodeAddr(chain.CallResult{Success: false}, unpack, "adapter.offerSigner()")
	if err == nil || errors.Is(err, errAdapterUnconfigured) {
		t.Fatalf("reverted read: err = %v, want a plain error", err)
	}

	addr, err := decodeAddr(chain.CallResult{Success: true, ReturnData: common.HexToAddress("0x1").Bytes()}, unpack, "vault()")
	if err != nil || addr != common.HexToAddress("0x1") {
		t.Fatalf("non-zero address: addr = %s, err = %v", addr.Hex(), err)
	}
}
