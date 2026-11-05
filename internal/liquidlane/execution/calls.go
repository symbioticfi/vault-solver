// Package execution builds explicit Liquid Lane funding and swap calls for settlement callbacks.
package execution

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

type Call struct {
	Target common.Address
	Value  *big.Int
	Data   []byte
}

var token = erc4626.NewIERC4626()
var venue = adapter.NewLiquidLaneAdapter()

func Transfer(asset, recipient common.Address, amount *big.Int) (Call, error) {
	data, err := token.TryPackTransfer(recipient, amount)
	if err != nil {
		return Call{}, errors.Errorf("pack input transfer: %w", err)
	}
	return Call{Target: asset, Value: new(big.Int), Data: data}, nil
}

func Swap(target, recipient, asset common.Address, amountIn, amountOut *big.Int) (Call, error) {
	data, err := venue.TryPackSwap(adapter.ILiquidLaneAdapterSwap{
		Recipient: recipient, TokenIn: asset, AmountIn: amountIn, AmountOut: amountOut,
	})
	if err != nil {
		return Call{}, errors.Errorf("pack adapter swap: %w", err)
	}
	return Call{Target: target, Value: new(big.Int), Data: data}, nil
}

func Discount(signed *discounts.Signed, recipient common.Address, amountIn *big.Int) (Call, error) {
	data, err := venue.TryPackSwap0(adapter.ILiquidLaneAdapterDiscountSwap{
		Discount:        adapter.ILiquidLaneAdapterDiscount(signed.Terms),
		SignerSignature: signed.SignerSignature, ProtocolDeadline: signed.ProtocolDeadline,
	}, signed.ProtocolSignature, recipient, amountIn)
	if err != nil {
		return Call{}, errors.Errorf("pack discounted adapter swap: %w", err)
	}
	return Call{Target: signed.Adapter, Value: new(big.Int), Data: data}, nil
}
