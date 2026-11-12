package rfq

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// Pin the full funding ABI independently of the generated binding.
const fillSignature = "fill(((address,uint256,(address,uint256,address)[],uint256,uint256,address),bytes,address,address,(address,uint256,address)[])," +
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
		Outputs: []executor.IReactorOutput{{
			Token: tOut, Amount: big.NewInt(900000), Recipient: common.HexToAddress("0x0000000000000000000000000000000000000099"),
		}},
	}
}

func TestEncodeFill_Selector(t *testing.T) {
	want := crypto.Keccak256([]byte(fillSignature))[:4]

	swaps := directSwaps(&fillPlan{Legs: []fillLeg{{
		Adapter: vlt, AmountIn: big.NewInt(1_000000000000000000), AmountOut: big.NewInt(900000),
	}}}, tIn, common.HexToAddress("0x0000000000000000000000000000000000000010"))

	data, err := encodeFill(sampleOrder(), []byte{0xaa, 0xbb}, swaps, nil)
	if err != nil {
		t.Fatalf("encodeFill: %v", err)
	}
	if len(data) < 4 || !bytes.Equal(data[:4], want) {
		t.Fatalf("selector = %x, want %x (wrong fill overload?)", data[:4], want)
	}
}

func TestEncodeFill_ExplicitSwapCalls(t *testing.T) {
	adapterABI := adapterBinding.GetABI()
	direct := directSwaps(&fillPlan{Legs: []fillLeg{
		{Adapter: vlt, AmountIn: big.NewInt(11), AmountOut: big.NewInt(9)},
		{Adapter: common.HexToAddress("0x1234"), AmountIn: big.NewInt(7), AmountOut: big.NewInt(5)},
	}}, tIn, sampleOrder().Filler)
	discounted := []executor.IReactorDiscountSwapInput{{
		Adapter: common.HexToAddress("0x5678"),
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount: executor.ILiquidLaneAdapterDiscount{
				TokenToRedeem: tIn, Discount: big.NewInt(1234), Signer: vlt,
				Protocol: sampleOrder().Request.Protocol, Nonce: big.NewInt(42), Deadline: big.NewInt(1000),
			},
			SignerSignature: []byte{0xaa, 0xbb}, ProtocolDeadline: big.NewInt(999),
		},
		ProtocolSignature: []byte{0xcc, 0xdd}, Recipient: sampleOrder().Filler, AmountIn: big.NewInt(13),
	}}
	for _, tc := range []struct {
		name       string
		direct     []executor.IReactorSwapInput
		discounted []executor.IReactorDiscountSwapInput
	}{
		{"empty", nil, nil},
		{"direct", direct, nil},
		{"discounted", nil, discounted},
		{"mixed", direct, discounted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := encodeFill(sampleOrder(), []byte{0x12}, tc.direct, tc.discounted)
			if err != nil {
				t.Fatal(err)
			}
			values, err := executorABI.Methods["fill"].Inputs.Unpack(data[4:])
			if err != nil {
				t.Fatal(err)
			}
			gotDirect := *abi.ConvertType(values[2], new([]executor.IReactorSwapInput)).(*[]executor.IReactorSwapInput)
			gotDiscounted := *abi.ConvertType(values[3], new([]executor.IReactorDiscountSwapInput)).(*[]executor.IReactorDiscountSwapInput)
			if (len(tc.direct) > 0 && !reflect.DeepEqual(gotDirect, tc.direct)) || (len(tc.discounted) > 0 && !reflect.DeepEqual(gotDiscounted, tc.discounted)) {
				t.Fatal("funding inputs changed")
			}
			decoded, err := executorCallsArgs.Unpack(values[4].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			calls := *abi.ConvertType(decoded[0], new([]executorCall)).(*[]executorCall)
			if len(calls) != len(tc.direct)+len(tc.discounted) {
				t.Fatalf("calls: %d", len(calls))
			}
			for i, call := range calls {
				if call.Value.Sign() != 0 {
					t.Fatal("unexpected native value")
				}
				method, err := adapterABI.MethodById(call.Data[:4])
				if err != nil {
					t.Fatal(err)
				}
				args, err := method.Inputs.Unpack(call.Data[4:])
				if err != nil {
					t.Fatal(err)
				}
				if i < len(tc.direct) {
					want := tc.direct[i]
					got := *abi.ConvertType(args[0], new(executor.ILiquidLaneAdapterSwap)).(*executor.ILiquidLaneAdapterSwap)
					if method.Sig != "swap((address,address,uint256,uint256))" || call.Target != want.Adapter || !reflect.DeepEqual(got, want.Swap) {
						t.Fatal("direct swap call differs from funding input")
					}
				} else {
					want := tc.discounted[i-len(tc.direct)]
					got := *abi.ConvertType(args[0], new(executor.ILiquidLaneAdapterDiscountSwap)).(*executor.ILiquidLaneAdapterDiscountSwap)
					if method.Sig != "swap(((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)" || call.Target != want.Adapter || !reflect.DeepEqual(got, want.DiscountSwap) || !bytes.Equal(args[1].([]byte), want.ProtocolSignature) || args[2].(common.Address) != want.Recipient || args[3].(*big.Int).Cmp(want.AmountIn) != 0 {
						t.Fatal("discounted swap call differs from funding input or signed terms")
					}
				}
			}
		})
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
		got.Request.Outputs[0].Amount.Cmp(big.NewInt(900000)) != 0 ||
		len(got.Outputs) != 1 ||
		got.Outputs[0].Amount.Cmp(big.NewInt(900000)) != 0 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestDirectSwaps_SkipsDiscountLegs(t *testing.T) {
	h := common.HexToHash("0x01")
	selected := &fillPlan{Legs: []fillLeg{
		{Adapter: vlt, AmountIn: big.NewInt(1), AmountOut: big.NewInt(2)},
		{Adapter: vlt, AmountIn: big.NewInt(3), AmountOut: big.NewInt(4), DiscountID: &h}, // discount leg → skipped (P3)
	}}
	swaps := directSwaps(selected, tIn, common.HexToAddress("0x0000000000000000000000000000000000000010"))
	if len(swaps) != 1 || swaps[0].Swap.AmountIn.Cmp(big.NewInt(1)) != 0 || swaps[0].Adapter != vlt {
		t.Fatalf("expected 1 direct swap, got %d", len(swaps))
	}
}
