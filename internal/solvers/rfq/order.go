package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
)

// executorABI is the parsed Executor ABI. fill() is overloaded 3×; abigen/go-ethereum name the first
// (the mixed order+swaps+discountSwaps overload) "fill", which is the one we build. The golden
// selector test pins that we encode the intended overload.
var executorABI = mustABI(executor.ExecutorMetaData)

// orderTupleArgs decodes the backend's `encodedOrder` — an ABI-encoded single Order tuple, identical
// to fill()'s first argument type.
var orderTupleArgs = abi.Arguments{{Type: fillOrderType()}}

// emptyExecutorData is abi.encode(ExecutorCall[]) for an empty call list — the executorData a direct
// fill passes (the Executor decodes it to a (target,value,data)[] and runs no extra calls).
var emptyExecutorData = encodeEmptyExecutorData()

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

func encodeEmptyExecutorData() []byte {
	t, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
		{Name: "target", Type: "address"},
		{Name: "value", Type: "uint256"},
		{Name: "data", Type: "bytes"},
	})
	if err != nil {
		panic("rfq: build executorData type: " + err.Error())
	}
	data, err := abi.Arguments{{Type: t}}.Pack([]executorCall{})
	if err != nil {
		panic("rfq: pack empty executorData: " + err.Error())
	}
	return data
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

// encodeFill builds Executor.fill(order, protocolSig, swapInputs, discountSwapInputs, executorData)
// calldata for the mixed overload.
func encodeFill(
	order executor.IReactorOrder,
	protocolSig []byte,
	swaps []executor.IReactorSwapInput,
	discountSwaps []executor.IReactorDiscountSwapInput,
	executorData []byte,
) ([]byte, error) {
	data, err := executorABI.Pack("fill", order, protocolSig, swaps, discountSwaps, executorData)
	if err != nil {
		return nil, errors.Errorf("encode fill: %w", err)
	}
	return data, nil
}

// directSwaps maps a strategy's direct (non-discount) legs to the Executor's SwapInputs: each carries
// its adapter and the per-adapter Swap tuple. The executor itself is the swap recipient (it forwards
// outputs to the Reactor). Mirrors the swapInputs build in execution.ts (#submitOrder).
func directSwaps(selected *strategyRecord, tokenIn, executorAddr common.Address) []executor.IReactorSwapInput {
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
