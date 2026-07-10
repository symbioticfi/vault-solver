package rfq

import (
	"bytes"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// fillSignature is the canonical signature of the mixed fill overload; pinning its selector guards
// against accidentally encoding a different fill overload. It mirrors the selector string in the TS
// filler's encodeExecutorFill (executor.ts): the order carries protocol-authorized outputs,
// swapInputs carry (adapter, (recipient,tokenIn,amountIn, amountOut)); discountSwapInputs carry
// (adapter, ((tokenToRedeem,discount,signer,protocol,nonce,deadline),sig,protocolDeadline),
// protocolSig,recipient,amountIn).
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

func TestEncodeFill_SelectorMatchesMixedOverload(t *testing.T) {
	want := crypto.Keccak256([]byte(fillSignature))[:4]

	swaps := directSwaps(&fillPlan{Legs: []fillLeg{{
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

func TestValidateSignedOrder(t *testing.T) {
	t.Parallel()
	executorAddr := common.HexToAddress("0x0000000000000000000000000000000000000010")
	now := time.Unix(1_000, 0)

	tests := []struct {
		name    string
		mutate  func(*executor.IReactorOrder)
		wantErr string
	}{
		{name: "valid"},
		{
			name: "different decoded filler",
			mutate: func(o *executor.IReactorOrder) {
				o.Filler = common.HexToAddress("0x00000000000000000000000000000000000000ff")
			},
			wantErr: "decoded order filler",
		},
		{
			name: "zero input token",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.TokenIn = common.Address{}
			},
			wantErr: "zero input token",
		},
		{
			name: "nil input amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.AmountIn = nil
			},
			wantErr: "invalid input amount",
		},
		{
			name: "zero input amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.AmountIn = new(big.Int)
			},
			wantErr: "invalid input amount",
		},
		{
			name: "nil deadline",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.Deadline = nil
			},
			wantErr: "deadline has passed",
		},
		{
			name: "expired deadline",
			mutate: func(o *executor.IReactorOrder) {
				o.Request.Deadline = big.NewInt(now.Unix())
			},
			wantErr: "deadline has passed",
		},
		{
			name: "no outputs",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs = nil
			},
			wantErr: "no outputs",
		},
		{
			name: "zero output token",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs[0].Token = common.Address{}
			},
			wantErr: "zero output token",
		},
		{
			name: "mixed output tokens",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs = append(o.Outputs, executor.IReactorOutput{
					Token:     common.HexToAddress("0x00000000000000000000000000000000000000ee"),
					Amount:    big.NewInt(1),
					Recipient: o.Outputs[0].Recipient,
				})
			},
			wantErr: "multiple output tokens",
		},
		{
			name: "nil output amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs[0].Amount = nil
			},
			wantErr: "output 0 has invalid amount",
		},
		{
			name: "zero output amount",
			mutate: func(o *executor.IReactorOrder) {
				o.Outputs[0].Amount = new(big.Int)
			},
			wantErr: "output 0 has invalid amount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			order := sampleOrder()
			if tc.mutate != nil {
				tc.mutate(&order)
			}
			token, required, err := validateSignedOrder(order, executorAddr, now)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateSignedOrder: %v", err)
			}
			if token != tOut || required.Cmp(big.NewInt(900000)) != 0 {
				t.Fatalf("token/required = %s/%s, want %s/900000", token, required, tOut)
			}
		})
	}
}

func TestValidateSignedOrder_LargeUint256DeadlineDoesNotTruncate(t *testing.T) {
	t.Parallel()
	order := sampleOrder()
	order.Request.Deadline = new(big.Int).Lsh(big.NewInt(1), 70)

	_, _, err := validateSignedOrder(
		order,
		common.HexToAddress("0x0000000000000000000000000000000000000010"),
		time.Unix(1_000, 0),
	)
	if err != nil {
		t.Fatalf("large uint256 deadline rejected after narrowing: %v", err)
	}
}

func TestValidateSignedOrder_LargeUint256AmountsDoNotTruncate(t *testing.T) {
	t.Parallel()
	order := sampleOrder()
	order.Request.AmountIn = new(big.Int).Lsh(big.NewInt(1), 200)
	order.Outputs[0].Amount = new(big.Int).Lsh(big.NewInt(1), 190)

	_, required, err := validateSignedOrder(
		order,
		common.HexToAddress("0x0000000000000000000000000000000000000010"),
		time.Unix(1_000, 0),
	)
	if err != nil {
		t.Fatalf("large uint256 amount rejected after narrowing: %v", err)
	}
	if required.Cmp(order.Outputs[0].Amount) != 0 {
		t.Fatalf("required = %s, want %s", required, order.Outputs[0].Amount)
	}
}
