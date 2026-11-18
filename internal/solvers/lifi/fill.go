package lifi

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

var lifiExecutor = executor.NewLiquidLaneLifiExecutor()

type fillCalldata struct {
	OrderID  common.Hash
	Finalise []byte
	Deadline time.Time
}

func buildExecutorCalls(
	executorAddress common.Address,
	order submittedOrder,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) ([]executor.IExecutorCall, error) {
	if plan == nil || len(plan.Routes) == 0 {
		return nil, errors.New("fill plan has no routes")
	}
	calls := make([]executor.IExecutorCall, 0, 2*len(plan.Routes))
	totalAmountIn := new(big.Int)
	for i, route := range plan.Routes {
		if route.Adapter == (common.Address{}) || route.AmountIn == nil || route.AmountIn.Sign() <= 0 ||
			route.ExpectedAmountOut == nil || route.ExpectedAmountOut.Sign() <= 0 || route.MinAmountOut == nil ||
			route.MinAmountOut.Sign() <= 0 || route.MinAmountOut.Cmp(route.ExpectedAmountOut) > 0 {
			return nil, errors.Errorf("fill plan route %d is invalid", i)
		}
		totalAmountIn.Add(totalAmountIn, route.AmountIn)
		funding, err := execution.Transfer(order.TokenIn, route.Adapter, route.AmountIn)
		if err != nil {
			return nil, err
		}
		var swap execution.Call
		if route.DiscountID == nil {
			swap, err = execution.Swap(route.Adapter, executorAddress, order.TokenIn, route.AmountIn, route.ExpectedAmountOut)
		} else {
			resolved := resolvedDiscounts[*route.DiscountID]
			if *route.DiscountID == (common.Hash{}) || resolved == nil ||
				resolved.DiscountID != *route.DiscountID || resolved.Adapter != route.Adapter ||
				resolved.Terms.TokenToRedeem != order.TokenIn {
				return nil, errors.Errorf("fill plan route %d discount does not match", i)
			}
			swap, err = execution.Discount(resolved, executorAddress, route.AmountIn)
		}
		if err != nil {
			return nil, err
		}
		calls = append(calls, executor.IExecutorCall(funding), executor.IExecutorCall(swap))
	}
	if totalAmountIn.Cmp(order.AmountIn) != 0 {
		return nil, errors.Errorf("fill plan input sum %s does not match order input %s", totalAmountIn, order.AmountIn)
	}
	return calls, nil
}

func buildFillCalldata(
	executorAddress common.Address,
	order submittedOrder,
	orderID common.Hash,
	plan *types.FillPlan,
	resolvedDiscounts map[common.Hash]*discounts.Signed,
) (*fillCalldata, error) {
	calls, err := buildExecutorCalls(executorAddress, order, plan, resolvedDiscounts)
	if err != nil {
		return nil, err
	}
	deadline, err := lifiFillDeadline(order, plan, resolvedDiscounts)
	if err != nil {
		return nil, err
	}
	finaliseCalldata, err := lifiExecutor.TryPackFinaliseWithCurrentTimestamp(
		toExecutorOrder(order.Order),
		calls,
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
