package rfq

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/bigmath"
)

var executorBinding = executor.NewExecutor()

// Use the actual mixed fill overload's order shape for backend tuple decoding.
// The generated PackFill selector is pinned by the existing golden tests.
var orderTupleArgs = func() abi.Arguments {
	contract, err := executor.ExecutorMetaData.ParseABI()
	if err != nil {
		panic("rfq executor ABI: " + err.Error())
	}
	method, exists := contract.Methods["fill"]
	if !exists || len(method.Inputs) == 0 {
		panic("rfq executor ABI missing fill(order, ...)")
	}
	return abi.Arguments{{Name: "order", Type: method.Inputs[0].Type}}
}()

// abi.encode(ExecutorCall[]): one offset word (32), then an empty array length.
// No callback elements exist, so their tuple layout contributes no encoded words.
var emptyExecutorData = []byte{31: 32, 63: 0}

func decodeOrder(encoded []byte) (executor.IReactorOrder, error) {
	var decoded struct{ Order executor.IReactorOrder }
	values, err := orderTupleArgs.Unpack(encoded)
	if err == nil {
		err = orderTupleArgs.Copy(&decoded, values)
	}
	if err != nil {
		return executor.IReactorOrder{}, errors.Errorf("decode order: %w", err)
	}
	return decoded.Order, nil
}

func encodeFill(order executor.IReactorOrder, protocolSig []byte, swaps []executor.IReactorSwapInput,
	discounts []executor.IReactorDiscountSwapInput, executorData []byte) ([]byte, error) {
	encoded, err := executorBinding.TryPackFill(order, protocolSig, swaps, discounts, executorData)
	if err != nil {
		return nil, errors.Errorf("encode fill: %w", err)
	}
	return encoded, nil
}

// Executor forwards these outputs to the Reactor. Own the selected amounts so a
// strategy result cannot mutate a prepared transaction through shared pointers.
func directSwaps(plan *fillPlan, tokenIn, recipient common.Address) []executor.IReactorSwapInput {
	out := make([]executor.IReactorSwapInput, 0, len(plan.Legs))
	for _, leg := range plan.Legs {
		if leg.DiscountID == nil {
			out = append(out, executor.IReactorSwapInput{Adapter: leg.Adapter, Swap: executor.ILiquidLaneAdapterSwap{
				Recipient: recipient, TokenIn: tokenIn, AmountIn: bigmath.Clone(leg.AmountIn), AmountOut: bigmath.Clone(leg.AmountOut),
			}})
		}
	}
	return out
}
