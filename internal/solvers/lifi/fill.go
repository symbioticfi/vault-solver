package lifi

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

var lifiExecutor = executor.NewLiquidLaneLifiExecutor()

type fillCalldata struct {
	OrderID  common.Hash
	Finalise []byte
}

func executorRoutes(
	order submittedOrder,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) ([]executor.ILiquidLaneLifiExecutorFillRoute, error) {
	if plan == nil || len(plan.Routes) == 0 {
		return nil, errors.New("fill plan has no routes")
	}
	routes := make([]executor.ILiquidLaneLifiExecutorFillRoute, 0, len(plan.Routes))
	totalAmountIn := new(big.Int)
	for i, route := range plan.Routes {
		if route.Adapter == (common.Address{}) || route.AmountIn == nil || route.AmountIn.Sign() <= 0 ||
			route.ExpectedAmountOut == nil || route.ExpectedAmountOut.Sign() <= 0 ||
			route.MinAmountOut == nil || route.MinAmountOut.Sign() <= 0 ||
			route.MinAmountOut.Cmp(route.ExpectedAmountOut) > 0 {
			return nil, errors.Errorf("fill plan route %d is invalid", i)
		}
		discount, err := executorDiscount(route, order.TokenIn, resolvedDiscounts)
		if err != nil {
			return nil, errors.Errorf("fill plan route %d discount: %w", i, err)
		}
		routes = append(routes, executor.ILiquidLaneLifiExecutorFillRoute{
			Adapter: route.Adapter, AmountIn: route.AmountIn, AmountOut: route.ExpectedAmountOut,
			Discount: discount,
		})
		totalAmountIn.Add(totalAmountIn, route.AmountIn)
	}
	if totalAmountIn.Cmp(order.AmountIn) != 0 {
		return nil, errors.Errorf("fill plan input sum %s does not match order input %s", totalAmountIn, order.AmountIn)
	}
	return routes, nil
}

func executorDiscount(
	route types.FillRoute,
	tokenIn common.Address,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (executor.ILiquidLaneLifiExecutorFillDiscount, error) {
	if route.DiscountID == nil {
		return emptyExecutorDiscount(), nil
	}
	if *route.DiscountID == (common.Hash{}) {
		return executor.ILiquidLaneLifiExecutorFillDiscount{}, errors.New("discount id is zero")
	}
	resolved := resolvedDiscounts[*route.DiscountID]
	if resolved == nil {
		return executor.ILiquidLaneLifiExecutorFillDiscount{}, errors.New("resolved discount is missing")
	}
	if resolved.DiscountID != *route.DiscountID {
		return executor.ILiquidLaneLifiExecutorFillDiscount{}, errors.New("resolved discount id mismatch")
	}
	if resolved.Adapter != route.Adapter {
		return executor.ILiquidLaneLifiExecutorFillDiscount{}, errors.New("resolved discount adapter mismatch")
	}
	if resolved.Terms.TokenToRedeem != tokenIn {
		return executor.ILiquidLaneLifiExecutorFillDiscount{}, errors.New("resolved discount token mismatch")
	}
	return executor.ILiquidLaneLifiExecutorFillDiscount{
		DiscountId: [32]byte(resolved.DiscountID),
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount: executor.ILiquidLaneAdapterDiscount{
				TokenToRedeem: resolved.Terms.TokenToRedeem,
				Discount:      liquidlane.CloneBig(resolved.Terms.Discount),
				Signer:        resolved.Terms.Signer,
				Protocol:      resolved.Terms.Protocol,
				Nonce:         liquidlane.CloneBig(resolved.Terms.Nonce),
				Deadline:      liquidlane.CloneBig(resolved.Terms.Deadline),
			},
			SignerSignature:  append([]byte(nil), resolved.SignerSignature...),
			ProtocolDeadline: liquidlane.CloneBig(resolved.ProtocolDeadline),
		},
		ProtocolSignature: append([]byte(nil), resolved.ProtocolSignature...),
	}, nil
}

func emptyExecutorDiscount() executor.ILiquidLaneLifiExecutorFillDiscount {
	return executor.ILiquidLaneLifiExecutorFillDiscount{
		DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
			Discount: executor.ILiquidLaneAdapterDiscount{
				Discount: new(big.Int), Nonce: new(big.Int), Deadline: new(big.Int),
			},
			ProtocolDeadline: new(big.Int),
		},
	}
}

func buildFillCalldata(
	order submittedOrder,
	orderID common.Hash,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (*fillCalldata, error) {
	routes, err := executorRoutes(order, plan, resolvedDiscounts)
	if err != nil {
		return nil, err
	}
	finaliseCalldata, err := lifiExecutor.TryPackFinaliseWithCurrentTimestamp(
		toExecutorOrder(order.Order),
		routes,
	)
	if err != nil {
		return nil, errors.Errorf("pack finaliseWithCurrentTimestamp: %w", err)
	}
	return &fillCalldata{OrderID: orderID, Finalise: finaliseCalldata}, nil
}

func toExecutorOutput(output inputsettler.MandateOutput) executor.MandateOutput {
	return executor.MandateOutput{
		Oracle:       output.Oracle,
		Settler:      output.Settler,
		ChainId:      output.ChainId,
		Token:        output.Token,
		Amount:       output.Amount,
		Recipient:    output.Recipient,
		CallbackData: output.CallbackData,
		Context:      output.Context,
	}
}

func toExecutorOrder(order inputsettler.StandardOrder) executor.IInputSettlerStandardOrder {
	outputs := make([]executor.MandateOutput, 0, len(order.Outputs))
	for _, out := range order.Outputs {
		outputs = append(outputs, toExecutorOutput(out))
	}
	return executor.IInputSettlerStandardOrder{
		User:          order.User,
		Nonce:         order.Nonce,
		OriginChainId: order.OriginChainId,
		Expires:       order.Expires,
		FillDeadline:  order.FillDeadline,
		InputOracle:   order.InputOracle,
		Inputs:        order.Inputs,
		Outputs:       outputs,
	}
}
