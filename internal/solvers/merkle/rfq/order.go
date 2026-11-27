package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
)

var executorBinding = executor.NewExecutor()
var orderTupleArgs = abi.Arguments{{Type: executorBinding.GetABI().Methods["fill"].Inputs[0].Type}}

func decodeOrder(row rfqbackend.OrdersResponseOrdersInner, filler common.Address) (executor.IReactorOrder, []byte, error) {
	var order executor.IReactorOrder
	switch {
	case row.OrderStatus != "open":
		return order, nil, errors.Errorf("order status is %q, not open", row.OrderStatus)
	case row.OrderId == "":
		return order, nil, errors.New("awarded order has no orderId")
	case row.EncodedOrder == nil:
		return order, nil, errors.New("awarded order has no encodedOrder")
	case row.ProtocolSignature == nil:
		return order, nil, errors.New("awarded order has no protocolSignature")
	}
	encoded, err := hexutil.Decode(*row.EncodedOrder)
	if err != nil {
		return order, nil, errors.Errorf("invalid encodedOrder: %w", err)
	}
	values, err := orderTupleArgs.Unpack(encoded)
	if err != nil {
		return order, nil, errors.Errorf("decode order: %w", err)
	}
	order = *abi.ConvertType(values[0], &order).(*executor.IReactorOrder)
	if _, _, err := orderTerms(order, filler); err != nil {
		return order, nil, err
	}
	sig, err := hexutil.Decode(*row.ProtocolSignature)
	if err != nil {
		return order, nil, errors.Errorf("invalid protocolSignature: %w", err)
	}
	return order, sig, nil
}

// Execution uses the signed tuple; the backend's display amounts never size a trade.
func orderTerms(order executor.IReactorOrder, filler common.Address) (common.Address, *big.Int, error) {
	switch request := order.Request; {
	case order.Filler != filler:
		return common.Address{}, nil, errors.Errorf("order filler %s is not this filler %s", order.Filler.Hex(), filler.Hex())
	case request.TokenIn == (common.Address{}):
		return common.Address{}, nil, errors.New("order has no input token")
	case request.AmountIn == nil || request.AmountIn.Sign() <= 0 || request.AmountIn.BitLen() > 256:
		return common.Address{}, nil, errors.Errorf("order input amount %v is not a positive uint256", request.AmountIn)
	case request.Deadline == nil || !request.Deadline.IsInt64() || request.Deadline.Sign() <= 0:
		return common.Address{}, nil, errors.Errorf("order deadline %v is not a positive int64", request.Deadline)
	case len(order.Outputs) == 0:
		return common.Address{}, nil, errors.New("order has no outputs")
	case order.Outputs[0].Token == (common.Address{}):
		return common.Address{}, nil, errors.New("order output has no token")
	}
	token, total := order.Outputs[0].Token, new(big.Int)
	for i, output := range order.Outputs {
		switch {
		case output.Token != token:
			return common.Address{}, nil, errors.Errorf("order output %d pays %s, not the first output's token %s", i, output.Token.Hex(), token.Hex())
		case output.Recipient == (common.Address{}):
			return common.Address{}, nil, errors.Errorf("order output %d has no recipient", i)
		case output.Amount == nil || output.Amount.Sign() <= 0:
			return common.Address{}, nil, errors.Errorf("order output %d amount %v is not positive", i, output.Amount)
		}
		total.Add(total, output.Amount)
	}
	if total.BitLen() > 256 {
		return common.Address{}, nil, errors.Errorf("order output total %s exceeds uint256", total)
	}
	return token, total, nil
}

// executorData is the existing opaque Call[] payload, separate from Reactor funding.
type executorCall struct {
	Target common.Address
	Value  *big.Int
	Data   []byte
}

var executorCallsArgs = executorCallArguments()

func executorCallArguments() abi.Arguments {
	t, err := abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{
		{Name: "target", Type: "address"},
		{Name: "value", Type: "uint256"},
		{Name: "data", Type: "bytes"},
	})
	if err != nil {
		panic("rfq: executorData type: " + err.Error())
	}
	return abi.Arguments{{Type: t}}
}
