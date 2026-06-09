package rfq

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// fillSignature is the canonical signature of the mixed fill overload; pinning its selector guards
// against accidentally encoding a different fill overload. It mirrors the selector string in the TS
// filler's encodeExecutorFill (executor.ts): swapInputs carry (adapter, (recipient,tokenIn,amountIn,
// amountOut)); discountSwapInputs carry (adapter, ((tokenToRedeem,discount,signer,protocol,nonce,
// deadline),sig,protocolDeadline),protocolSig,recipient,amountIn).
const fillSignature = "fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address)," +
	"bytes,(address,(address,address,uint256,uint256))[]," +
	"(address,((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)[],bytes)"

func sampleOrder() executor.IReactorOrder {
	return executor.IReactorOrder{
		Request: executor.IReactorRequest{
			TokenIn:  tIn,
			AmountIn: big.NewInt(1_000000000000000000),
			Outputs: []executor.IReactorOutput{{
				Token: tOut, Amount: big.NewInt(900000), Recipient: common.HexToAddress("0x0000000000000000000000000000000000000099"),
			}},
			Deadline: big.NewInt(4_102_444_800),
			Nonce:    big.NewInt(1),
			Protocol: common.HexToAddress("0x00000000000000000000000000000000000000aa"),
		},
		SwapperSignature: []byte{0x01, 0x02},
		Swapper:          common.HexToAddress("0x0000000000000000000000000000000000000099"),
		Filler:           common.HexToAddress("0x0000000000000000000000000000000000000010"),
	}
}

func TestEncodeFill_SelectorMatchesMixedOverload(t *testing.T) {
	want := crypto.Keccak256([]byte(fillSignature))[:4]

	swaps := directSwaps(&strategyRecord{Legs: []strategyLeg{{
		Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000),
	}}}, tIn, common.HexToAddress("0x0000000000000000000000000000000000000010"))

	data, err := encodeFill(sampleOrder(), []byte{0xaa, 0xbb}, swaps, nil, emptyExecutorData)
	if err != nil {
		t.Fatalf("encodeFill: %v", err)
	}
	if len(data) < 4 || !bytes.Equal(data[:4], want) {
		t.Fatalf("selector = %x, want %x (wrong fill overload?)", data[:4], want)
	}
}

func TestDecodeOrder_RoundTrip(t *testing.T) {
	order := sampleOrder()
	// Encode just the Order tuple (as the backend's encodedOrder is) using the same arg set.
	encoded, err := orderTupleArgs.Pack(order)
	if err != nil {
		t.Fatalf("pack order: %v", err)
	}
	got, err := decodeOrder(encoded)
	if err != nil {
		t.Fatalf("decodeOrder: %v", err)
	}
	if got.Request.TokenIn != order.Request.TokenIn ||
		got.Request.AmountIn.Cmp(order.Request.AmountIn) != 0 ||
		got.Filler != order.Filler ||
		len(got.Request.Outputs) != 1 ||
		got.Request.Outputs[0].Amount.Cmp(big.NewInt(900000)) != 0 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestDirectSwaps_SkipsDiscountLegs(t *testing.T) {
	h := common.HexToHash("0x01")
	selected := &strategyRecord{Legs: []strategyLeg{
		{Adapter: vlt, AmountIn: big.NewInt(1), AmountOut: big.NewInt(2)},
		{Adapter: vlt, AmountIn: big.NewInt(3), AmountOut: big.NewInt(4), DiscountID: &h}, // discount leg → skipped (P3)
	}}
	swaps := directSwaps(selected, tIn, common.HexToAddress("0x0000000000000000000000000000000000000010"))
	if len(swaps) != 1 || swaps[0].Swap.AmountIn.Cmp(big.NewInt(1)) != 0 || swaps[0].Adapter != vlt {
		t.Fatalf("expected 1 direct swap, got %d", len(swaps))
	}
}
