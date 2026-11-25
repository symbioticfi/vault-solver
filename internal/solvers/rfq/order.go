package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

var (
	executorBinding   = executor.NewExecutor()
	executorABI       = mustExecutorABI()
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
		return executor.IReactorOrder{}, errors.Errorf("decode order: cannot convert %T to IReactorOrder", vals[0])
	}
	return *out, nil
}

// Native Executors execute swapInputs and discountSwapInputs themselves. Extra calls
// must stay empty: repeating those swaps in executorData spends the input twice.
// Merkle RFQ builds explicit recipe calls in its separate execution path.
func encodeFill(
	order executor.IReactorOrder,
	protocolSig []byte,
	swaps []executor.IReactorSwapInput,
	discountSwaps []executor.IReactorDiscountSwapInput,
) ([]byte, error) {
	executorData, err := executorCallsArgs.Pack([]executorCall{})
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
