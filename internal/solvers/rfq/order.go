package rfq

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// executorBinding (abigen --v2) packs fill() via TryPackFill; executorABI is parsed only for tuple
// reflection (decoding the backend's order tuple), which the Pack helpers don't expose. fill() is
// overloaded 3×; PackFill is the mixed order+protocolSig+swaps+discountSwaps overload we build — the
// golden selector test pins it.
var (
	executorBinding = executor.NewExecutor()
	executorABI     = mustExecutorABI()
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

// emptyExecutorData is abi.encode(ExecutorCall[]) for an empty call list — the executorData a direct
// fill passes (the Executor decodes it to a (target,value,data)[] and runs no extra calls).
var emptyExecutorData = encodeEmptyExecutorData()

// executablePayload is the backend wire payload after basic hex/address decoding.
type executablePayload struct {
	quoteID      string
	encodedOrder []byte
	signature    []byte
	deadline     int64
	filler       common.Address
	outputs      []backendOut
}

// executable is a fully cross-checked order ready for planning and submission.
type executable struct {
	quoteID     string
	order       executor.IReactorOrder
	signature   []byte
	outputToken common.Address
	required    *big.Int
	deadline    time.Time
}

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

func executableFromBackend(bo *backendOrder) (*executablePayload, error) {
	if bo.EncodedOrder == nil || bo.ProtocolSignature == nil || bo.Deadline == nil || bo.Filler == nil {
		return nil, errors.New("executable order payload incomplete")
	}
	if !common.IsHexAddress(*bo.Filler) {
		return nil, errors.Errorf("invalid filler %q", *bo.Filler)
	}
	encoded, err := hexutil.Decode(*bo.EncodedOrder)
	if err != nil {
		return nil, errors.Errorf("decode encodedOrder: %w", err)
	}
	signature, err := hexutil.Decode(*bo.ProtocolSignature)
	if err != nil {
		return nil, errors.Errorf("decode protocolSignature: %w", err)
	}
	return &executablePayload{
		quoteID: bo.QuoteID, encodedOrder: encoded, signature: signature,
		deadline: *bo.Deadline, filler: common.HexToAddress(*bo.Filler), outputs: bo.Outputs,
	}, nil
}

func prepareExecutable(
	payload *executablePayload,
	expectedFiller common.Address,
) (*executable, error) {
	order, err := decodeOrder(payload.encodedOrder)
	if err != nil {
		return nil, err
	}
	if order.Filler != expectedFiller {
		return nil, errors.New("signed order assigns a different filler")
	}
	if payload.filler != order.Filler {
		return nil, errors.New("backend filler does not match signed order")
	}
	if order.Request.TokenIn == (common.Address{}) || order.Request.AmountIn == nil || order.Request.AmountIn.Sign() <= 0 {
		return nil, errors.New("signed order has invalid input")
	}
	if order.Request.Deadline == nil || !order.Request.Deadline.IsInt64() || order.Request.Deadline.Sign() <= 0 {
		return nil, errors.New("signed order has invalid deadline")
	}
	if payload.deadline != order.Request.Deadline.Int64() {
		return nil, errors.New("backend deadline does not match signed order")
	}
	if len(order.Outputs) == 0 || len(payload.outputs) != len(order.Outputs) {
		return nil, errors.New("backend outputs do not match signed order")
	}

	outputToken := order.Outputs[0].Token
	if outputToken == (common.Address{}) {
		return nil, errors.New("only single output-token orders are supported")
	}
	required := new(big.Int)
	for i, signed := range order.Outputs {
		if signed.Token != outputToken {
			return nil, errors.New("only single output-token orders are supported")
		}
		if signed.Amount == nil || signed.Amount.Sign() <= 0 {
			return nil, errors.Errorf("signed order output %d has invalid amount", i)
		}
		if signed.Recipient == (common.Address{}) {
			return nil, errors.Errorf("signed order output %d has invalid recipient", i)
		}
		required.Add(required, signed.Amount)

		backend := payload.outputs[i]
		amount, ok := new(big.Int).SetString(backend.Amount, 10)
		if !common.IsHexAddress(backend.Token) || !common.IsHexAddress(backend.Recipient) {
			return nil, errors.Errorf("backend output %d has invalid address", i)
		}
		if !ok || amount.Sign() <= 0 {
			return nil, errors.Errorf("backend output %d has invalid amount", i)
		}
		if common.HexToAddress(backend.Token) != signed.Token ||
			common.HexToAddress(backend.Recipient) != signed.Recipient || amount.Cmp(signed.Amount) != 0 {
			return nil, errors.Errorf("backend output %d does not match signed order", i)
		}
	}
	return &executable{
		quoteID: payload.quoteID, order: order, signature: payload.signature,
		outputToken: outputToken, required: required, deadline: time.Unix(payload.deadline, 0),
	}, nil
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
	data, err := executorBinding.TryPackFill(order, protocolSig, swaps, discountSwaps, executorData)
	if err != nil {
		return nil, errors.Errorf("encode fill: %w", err)
	}
	return data, nil
}

// directSwaps maps a strategy's direct (non-discount) legs to the Executor's SwapInputs: each carries
// its adapter and the per-adapter Swap tuple. The executor itself is the swap recipient (it forwards
// outputs to the Reactor). Mirrors the swapInputs build in execution.ts (#submitOrder).
func directSwaps(selected *liquidlane.Plan, tokenIn, executorAddr common.Address) []executor.IReactorSwapInput {
	swaps := make([]executor.IReactorSwapInput, 0, len(selected.Routes))
	for _, leg := range selected.Routes {
		if leg.DiscountID != nil {
			continue // discount legs are built separately
		}
		swaps = append(swaps, executor.IReactorSwapInput{
			Adapter: leg.Adapter,
			Swap: executor.ILiquidLaneAdapterSwap{
				Recipient: executorAddr,
				TokenIn:   tokenIn,
				AmountIn:  new(big.Int).Set(leg.AmountIn),
				AmountOut: new(big.Int).Set(leg.ExpectedAmountOut),
			},
		})
	}
	return swaps
}
