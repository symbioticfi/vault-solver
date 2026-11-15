package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

var (
	executorBinding   = executor.NewExecutor()
	executorABI       = mustExecutorABI()
	adapterBinding    = adapter.NewLiquidLaneAdapter()
	executorCallsArgs = executorCallArguments()
)

func mustExecutorABI() abi.ABI {
	parsed, err := executor.ExecutorMetaData.ParseABI()
	if err != nil {
		panic("rfq: parse executor ABI: " + err.Error())
	}
	return *parsed
}

// orderTupleArgs decodes the backend's `encodedOrder` — an ABI-encoded single Order tuple, identical
// to fill()'s first argument type.
var orderTupleArgs = abi.Arguments{{Type: fillOrderType()}}

func fillOrderType() abi.Type {
	m, ok := executorABI.Methods["fill"]
	if !ok || len(m.Inputs) == 0 {
		panic("rfq: executor ABI missing fill(order, ...)")
	}
	return m.Inputs[0].Type
}

// executorCall mirrors the Executor's (address target, uint256 value, bytes data) callback tuple.
type executorCall struct {
	Target common.Address
	Value  *big.Int
	Data   []byte
}

func executorCallArguments() abi.Arguments {
	t, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
		{Name: "target", Type: "address"},
		{Name: "value", Type: "uint256"},
		{Name: "data", Type: "bytes"},
	})
	if err != nil {
		panic("rfq: build executorData type: " + err.Error())
	}
	return abi.Arguments{{Type: t}}
}

// decodeOrder decodes a backend-provided ABI-encoded Reactor order into the typed struct fill expects.
func decodeOrder(encoded []byte) (executor.IReactorOrder, error) {
	vals, err := orderTupleArgs.Unpack(encoded)
	if err != nil {
		return executor.IReactorOrder{}, errors.Errorf("decode order: %w", err)
	}
	if len(vals) != 1 {
		return executor.IReactorOrder{}, errors.Errorf("decode order: got %d values, want 1", len(vals))
	}
	out, ok := abi.ConvertType(vals[0], new(executor.IReactorOrder)).(*executor.IReactorOrder)
	if !ok {
		return executor.IReactorOrder{}, errors.New("decode order: type conversion failed")
	}
	return *out, nil
}

// Reactor funds the input arrays; Executor runs the corresponding calls in this order.
func encodeFill(
	order executor.IReactorOrder,
	protocolSig []byte,
	swaps []executor.IReactorSwapInput,
	discountSwaps []executor.IReactorDiscountSwapInput,
) ([]byte, error) {
	calls := make([]executorCall, 0, len(swaps)+len(discountSwaps))
	for _, input := range swaps {
		data, err := adapterBinding.TryPackSwap(adapter.ILiquidLaneAdapterSwap(input.Swap))
		if err != nil {
			return nil, errors.Errorf("encode adapter swap: %w", err)
		}
		calls = append(calls, executorCall{Target: input.Adapter, Value: new(big.Int), Data: data})
	}
	for _, input := range discountSwaps {
		discount := adapter.ILiquidLaneAdapterDiscountSwap{
			Discount:         adapter.ILiquidLaneAdapterDiscount(input.DiscountSwap.Discount),
			SignerSignature:  input.DiscountSwap.SignerSignature,
			ProtocolDeadline: input.DiscountSwap.ProtocolDeadline,
		}
		data, err := adapterBinding.TryPackSwap0(discount, input.ProtocolSignature, input.Recipient, input.AmountIn)
		if err != nil {
			return nil, errors.Errorf("encode discounted adapter swap: %w", err)
		}
		calls = append(calls, executorCall{Target: input.Adapter, Value: new(big.Int), Data: data})
	}
	executorData, err := executorCallsArgs.Pack(calls)
	if err != nil {
		return nil, errors.Errorf("encode executor calls: %w", err)
	}
	data, err := executorBinding.TryPackFill(order, protocolSig, swaps, discountSwaps, executorData)
	if err != nil {
		return nil, errors.Errorf("encode fill: %w", err)
	}
	return data, nil
}

// directSwaps maps a strategy's direct (non-discount) legs to the Executor's SwapInputs: each carries
// its adapter and the per-adapter Swap tuple. The executor itself is the swap recipient (it forwards
// outputs to the Reactor). Mirrors the swapInputs build in execution.ts (#submitOrder).
func directSwaps(selected *fillPlan, tokenIn, executorAddr common.Address) []executor.IReactorSwapInput {
	swaps := make([]executor.IReactorSwapInput, 0, len(selected.Legs))
	for _, leg := range selected.Legs {
		if leg.DiscountID != nil {
			continue // discount legs are built separately
		}
		swaps = append(swaps, executor.IReactorSwapInput{
			Adapter: leg.Adapter,
			Swap: executor.ILiquidLaneAdapterSwap{
				Recipient: executorAddr,
				TokenIn:   tokenIn,
				AmountIn:  new(big.Int).Set(leg.AmountIn),
				AmountOut: new(big.Int).Set(leg.AmountOut),
			},
		})
	}
	return swaps
}
