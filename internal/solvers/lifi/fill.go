package lifi

import (
	"math/big"
	"time"

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
	Deadline time.Time
}

func buildExecutorRoutes(
	order submittedOrder,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (
	[]executor.ILiquidLaneLifiExecutorFillRoute,
	[]executor.ILiquidLaneLifiExecutorDiscountRoute,
	error,
) {
	if plan == nil || len(plan.Routes) == 0 {
		return nil, nil, errors.New("fill plan has no routes")
	}
	directRoutes := make([]executor.ILiquidLaneLifiExecutorFillRoute, 0, len(plan.Routes))
	discountRoutes := make([]executor.ILiquidLaneLifiExecutorDiscountRoute, 0, len(plan.Routes))
	totalAmountIn := new(big.Int)
	for i, route := range plan.Routes {
		if route.Adapter == (common.Address{}) || route.AmountIn == nil || route.AmountIn.Sign() <= 0 ||
			route.ExpectedAmountOut == nil || route.ExpectedAmountOut.Sign() <= 0 ||
			route.MinAmountOut == nil || route.MinAmountOut.Sign() <= 0 ||
			route.MinAmountOut.Cmp(route.ExpectedAmountOut) > 0 {
			return nil, nil, errors.Errorf("fill plan route %d is invalid", i)
		}
		totalAmountIn.Add(totalAmountIn, route.AmountIn)
		if route.DiscountID == nil {
			directRoutes = append(directRoutes, executor.ILiquidLaneLifiExecutorFillRoute{
				Adapter: route.Adapter, AmountIn: route.AmountIn, AmountOut: route.ExpectedAmountOut,
			})
			continue
		}
		discountRoute, err := buildExecutorDiscountRoute(route, *route.DiscountID, order.TokenIn, resolvedDiscounts)
		if err != nil {
			return nil, nil, errors.Errorf("fill plan route %d discount: %w", i, err)
		}
		discountRoutes = append(discountRoutes, discountRoute)
	}
	if totalAmountIn.Cmp(order.AmountIn) != 0 {
		return nil, nil, errors.Errorf("fill plan input sum %s does not match order input %s", totalAmountIn, order.AmountIn)
	}
	return directRoutes, discountRoutes, nil
}

func buildExecutorDiscountRoute(
	route types.FillRoute,
	discountID common.Hash,
	tokenIn common.Address,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (executor.ILiquidLaneLifiExecutorDiscountRoute, error) {
	if discountID == (common.Hash{}) {
		return executor.ILiquidLaneLifiExecutorDiscountRoute{}, errors.New("discount id is zero")
	}
	resolved := resolvedDiscounts[discountID]
	if resolved == nil {
		return executor.ILiquidLaneLifiExecutorDiscountRoute{}, errors.New("resolved discount is missing")
	}
	if resolved.DiscountID != discountID {
		return executor.ILiquidLaneLifiExecutorDiscountRoute{}, errors.New("resolved discount id mismatch")
	}
	if resolved.Adapter != route.Adapter {
		return executor.ILiquidLaneLifiExecutorDiscountRoute{}, errors.New("resolved discount adapter mismatch")
	}
	if resolved.Terms.TokenToRedeem != tokenIn {
		return executor.ILiquidLaneLifiExecutorDiscountRoute{}, errors.New("resolved discount token mismatch")
	}
	return executor.ILiquidLaneLifiExecutorDiscountRoute{
		Adapter:  route.Adapter,
		AmountIn: route.AmountIn,
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

func buildFillCalldata(
	order submittedOrder,
	orderID common.Hash,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (*fillCalldata, error) {
	directRoutes, discountRoutes, err := buildExecutorRoutes(order, plan, resolvedDiscounts)
	if err != nil {
		return nil, err
	}
	deadline, err := lifiFillDeadline(order, plan, resolvedDiscounts)
	if err != nil {
		return nil, err
	}
	finaliseCalldata, err := lifiExecutor.TryPackFinaliseWithCurrentTimestamp(
		toExecutorOrder(order.Order),
		directRoutes,
		discountRoutes,
	)
	if err != nil {
		return nil, errors.Errorf("pack finaliseWithCurrentTimestamp: %w", err)
	}
	return &fillCalldata{OrderID: orderID, Finalise: finaliseCalldata, Deadline: deadline}, nil
}

func lifiFillDeadline(
	order submittedOrder,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (time.Time, error) {
	deadline := earlierDeadline(
		unixDeadline(int64(order.Order.Expires)),
		unixDeadline(int64(order.Order.FillDeadline)),
	)
	for i, route := range plan.Routes {
		if route.DiscountID == nil {
			continue
		}
		resolved := resolvedDiscounts[*route.DiscountID]
		if resolved == nil || resolved.Terms.Deadline == nil || resolved.ProtocolDeadline == nil {
			return time.Time{}, errors.Errorf("fill plan route %d discount is missing deadlines", i)
		}
		if resolved.Terms.Deadline.Sign() <= 0 || !resolved.Terms.Deadline.IsInt64() ||
			resolved.ProtocolDeadline.Sign() <= 0 || !resolved.ProtocolDeadline.IsInt64() {
			return time.Time{}, errors.Errorf("fill plan route %d discount has invalid deadline", i)
		}
		deadline = earlierDeadline(deadline, discounts.ValidUntil(resolved))
	}
	return deadline, nil
}

func orderDeadline(order *submittedOrder) time.Time {
	if order == nil {
		return time.Time{}
	}
	return earlierDeadline(
		unixDeadline(int64(order.Order.Expires)),
		unixDeadline(int64(order.Order.FillDeadline)),
	)
}

func unixDeadline(unix int64) time.Time {
	if unix <= 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

func earlierDeadline(left, right time.Time) time.Time {
	if left.IsZero() || !right.IsZero() && right.Before(left) {
		return right
	}
	return left
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
