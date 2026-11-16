package rfq

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm/runtime"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// Execute the actual deployed legacy callback offline, not a reimplementation of
// its loops. The fixture records its mainnet address and finalized block/hash.
func TestNativeFillAgainstDeployedLegacyExecutor(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy-executor.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Address     string `json:"address"`
		RuntimeCode string `json:"runtimeCode"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	target := common.HexToAddress(fixture.Address)
	reactor := common.HexToAddress("0xC323B898d7E4105E3980082B74CC5D4602996B10")
	for _, tc := range []struct {
		name               string
		direct, discounted bool
	}{
		{"direct", true, false}, {"discounted", false, true}, {"mixed", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := sampleOrder()
			order.Request.Outputs, order.Outputs = nil, nil
			var swaps []executor.IReactorSwapInput
			var discounted []executor.IReactorDiscountSwapInput
			directAdapter, discountAdapter := common.HexToAddress("0xc001"), common.HexToAddress("0xc002")
			if tc.direct {
				swaps = []executor.IReactorSwapInput{{Adapter: directAdapter, Swap: executor.ILiquidLaneAdapterSwap{Recipient: target, TokenIn: tIn, AmountIn: new(big.Int), AmountOut: new(big.Int)}}}
			}
			if tc.discounted {
				discounted = []executor.IReactorDiscountSwapInput{{Adapter: discountAdapter, Recipient: target, AmountIn: new(big.Int), DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{Discount: executor.ILiquidLaneAdapterDiscount{Discount: new(big.Int), Nonce: new(big.Int), Deadline: new(big.Int)}, ProtocolDeadline: new(big.Int)}}}
			}
			packed, err := encodeFill(order, nil, swaps, discounted)
			if err != nil {
				t.Fatal(err)
			}
			args, err := executorABI.Methods["fill"].Inputs.Unpack(packed[4:])
			if err != nil {
				t.Fatal(err)
			}
			// Replay the callback exactly as Reactor invokes it with the solver's arrays
			// and executorData; the outer fill's caller authorization is unrelated here.
			callback, err := executorABI.Pack("execute", order, args[2], args[3], args[4])
			if err != nil {
				t.Fatal(err)
			}
			db, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
			if err != nil {
				t.Fatal(err)
			}
			db.SetCode(target, common.FromHex(fixture.RuntimeCode), tracing.CodeChangeUnspecified)
			// Accept one call, record it, and revert on a second call. Returning uint256
			// zero satisfies both adapter swap overloads without moving any real funds.
			oneShot := common.FromHex("0x5f54156009575f5ffd5b60015f555f5f5260205ff3")
			db.SetCode(directAdapter, oneShot, tracing.CodeChangeUnspecified)
			db.SetCode(discountAdapter, oneShot, tracing.CodeChangeUnspecified)
			if _, _, err := runtime.Call(target, callback, &runtime.Config{State: db, Origin: reactor, GasLimit: 1000000}); err != nil {
				t.Fatalf("legacy callback reverted: %v", err)
			}
			for i, enabled := range []bool{tc.direct, tc.discounted} {
				address := []common.Address{directAdapter, discountAdapter}[i]
				want := common.Hash{}
				if enabled {
					want = common.BigToHash(big.NewInt(1))
				}
				if got := db.GetState(address, common.Hash{}); got != want {
					t.Fatalf("swap execution count %s=%s want=%s", address, got, want)
				}
			}
			decoded, err := executorCallsArgs.Unpack(args[4].([]byte))
			if err != nil {
				t.Fatal(err)
			}
			if calls := *abi.ConvertType(decoded[0], new([]executorCall)).(*[]executorCall); len(calls) != 0 {
				t.Fatal("legacy callback received duplicate explicit swaps")
			}
		})
	}
}
