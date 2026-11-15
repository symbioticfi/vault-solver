// Package swapcalls checks generic callback calldata in solver tests.
package swapcalls

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
)

type Direct struct {
	Adapter             common.Address
	AmountIn, AmountOut *big.Int
}
type Discount struct {
	Adapter           common.Address
	AmountIn          *big.Int
	DiscountSwap      adapter.ILiquidLaneAdapterDiscountSwap
	ProtocolSignature []byte
}
type Plan struct {
	Routes         []Direct
	DiscountRoutes []Discount
}

func Decode(tb testing.TB, raw any) Plan {
	tb.Helper()
	calls := *abi.ConvertType(raw, new([]execution.Call)).(*[]execution.Call)
	if len(calls)%2 != 0 {
		tb.Fatalf("expected transfer/swap pairs, got %d calls", len(calls))
	}
	tokenABI, err := erc4626.IERC4626MetaData.ParseABI()
	if err != nil {
		tb.Fatal(err)
	}
	venueABI, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		tb.Fatal(err)
	}
	var plan Plan
	for i := 0; i < len(calls); i += 2 {
		funding, swap := calls[i], calls[i+1]
		if funding.Value.Sign() != 0 || swap.Value.Sign() != 0 {
			tb.Fatal("unexpected native call value")
		}
		method, err := tokenABI.MethodById(funding.Data)
		if err != nil || method.RawName != "transfer" {
			tb.Fatal("missing funding transfer")
		}
		values, err := method.Inputs.Unpack(funding.Data[4:])
		if err != nil {
			tb.Fatal(err)
		}
		if values[0].(common.Address) != swap.Target {
			tb.Fatal("funding recipient differs from swap target")
		}
		amount := values[1].(*big.Int)
		method, err = venueABI.MethodById(swap.Data)
		if err != nil {
			tb.Fatal(err)
		}
		values, err = method.Inputs.Unpack(swap.Data[4:])
		if err != nil {
			tb.Fatal(err)
		}
		switch method.Sig {
		case "swap((address,address,uint256,uint256))":
			args := *abi.ConvertType(values[0], new(adapter.ILiquidLaneAdapterSwap)).(*adapter.ILiquidLaneAdapterSwap)
			if args.TokenIn != funding.Target || args.AmountIn.Cmp(amount) != 0 {
				tb.Fatal("funding does not match direct swap")
			}
			plan.Routes = append(plan.Routes, Direct{swap.Target, amount, args.AmountOut})
		case "swap(((address,uint256,address,address,uint256,uint48),bytes,uint48),bytes,address,uint256)":
			args := *abi.ConvertType(values[0], new(adapter.ILiquidLaneAdapterDiscountSwap)).(*adapter.ILiquidLaneAdapterDiscountSwap)
			if args.Discount.TokenToRedeem != funding.Target || values[3].(*big.Int).Cmp(amount) != 0 {
				tb.Fatal("funding does not match discount swap")
			}
			plan.DiscountRoutes = append(plan.DiscountRoutes, Discount{swap.Target, amount, args, values[1].([]byte)})
		default:
			tb.Fatalf("unexpected swap %s", method.Sig)
		}
	}
	return plan
}
