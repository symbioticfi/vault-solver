package chain

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type decimalsMulticall struct {
	result CallResult
	calls  []Call
}

func (m *decimalsMulticall) Multicall(_ context.Context, calls []Call) ([]CallResult, error) {
	m.calls = calls
	return []CallResult{m.result}, nil
}

func TestDecimalsGetNamesTheRevert(t *testing.T) {
	t.Parallel()
	str, _ := abi.NewType("string", "", nil)
	payload := encodeRevert(t, "Error(string)", abi.Arguments{{Type: str}}, "not a token")
	fake := &decimalsMulticall{result: CallResult{Success: false, ReturnData: payload}}
	d := &Decimals{chain: fake, cache: map[common.Address]int{}}
	token := common.HexToAddress("0x1")

	_, err := d.Get(t.Context(), token)
	if err == nil || err.Error() != `erc20.decimals() reverted for `+token.Hex()+`: "not a token"` {
		t.Fatalf("Get err = %v", err)
	}
	if len(fake.calls) != 1 || !fake.calls[0].AllowFailure {
		t.Fatalf("decimals() must allow failure so the revert payload survives: %+v", fake.calls)
	}

	uint8Type, _ := abi.NewType("uint8", "", nil)
	ok, _ := abi.Arguments{{Type: uint8Type}}.Pack(uint8(6))
	fake.result = CallResult{Success: true, ReturnData: ok}
	if got, err := d.Get(t.Context(), token); err != nil || got != 6 {
		t.Fatalf("Get = %d, %v; want 6", got, err)
	}
}
