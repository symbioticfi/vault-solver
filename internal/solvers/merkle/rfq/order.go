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
var orderTupleArgs = abi.Arguments{{Type: executorBinding.GetABI().Methods["fill2"].Inputs[0].Type}}

func decodeOrder(row rfqbackend.OrdersResponseOrdersInner, filler common.Address) (executor.IReactorOrder, []byte, error) {
	var order executor.IReactorOrder
	if row.OrderStatus != "open" || row.OrderId == "" || row.EncodedOrder == nil || row.ProtocolSignature == nil {
		return order, nil, errors.New("incomplete executable order")
	}
	encoded, err := hexutil.Decode(*row.EncodedOrder)
	if err != nil {
		return order, nil, errors.New("invalid encodedOrder")
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
		return order, nil, errors.New("invalid protocolSignature")
	}
	return order, sig, nil
}

// Execution uses the signed tuple; the backend's display amounts never size a trade.
func orderTerms(order executor.IReactorOrder, filler common.Address) (common.Address, *big.Int, error) {
	if order.Filler != filler || order.Request.TokenIn == (common.Address{}) || order.Request.AmountIn == nil || order.Request.AmountIn.Sign() <= 0 || order.Request.AmountIn.BitLen() > 256 {
		return common.Address{}, nil, errors.New("invalid awarded input or filler")
	}
	if order.Request.Deadline == nil || !order.Request.Deadline.IsInt64() || order.Request.Deadline.Sign() <= 0 {
		return common.Address{}, nil, errors.New("invalid order deadline")
	}
	if len(order.Outputs) == 0 || order.Outputs[0].Token == (common.Address{}) {
		return common.Address{}, nil, errors.New("missing order outputs")
	}
	token, total := order.Outputs[0].Token, new(big.Int)
	for _, output := range order.Outputs {
		if output.Token != token || output.Recipient == (common.Address{}) || output.Amount == nil || output.Amount.Sign() <= 0 {
			return common.Address{}, nil, errors.New("one output token with positive payments required")
		}
		total.Add(total, output.Amount)
	}
	if total.BitLen() > 256 {
		return common.Address{}, nil, errors.New("order output exceeds uint256")
	}
	return token, total, nil
}
