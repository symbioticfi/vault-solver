package rfq

import (
	"context"
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
)

type discountResolver interface {
	resolveDiscount(ctx context.Context, id string) (*discounts.Resolved, error)
}

type builtFill struct {
	Calldata   []byte
	ValidUntil time.Time
	// Legs are the prepared connector swaps, in the order the Executor runs them.
	Legs []connector.Prepared
}

// buildFill wraps the prepared recipe calls in the RFQ Executor fill. Reactor delivers the
// awarded input to each connector through discountSwapInputs; executorData carries the recipes.
func buildFill(ctx context.Context, w *connector.Workflow, order executor.IReactorOrder, protocolSig []byte, legs []filler.Leg, backend discountResolver) (builtFill, error) {
	token, required, err := orderTerms(order, w.Filler)
	if err != nil {
		return builtFill{}, err
	}
	requests := make([]connector.Request, 0, len(legs))
	totalIn := new(big.Int)
	for _, leg := range legs {
		amountIn, amountOut := leg.AmountIn, (*big.Int)(nil)
		switch {
		case len(legs) == 1 && amountIn == nil:
			amountIn, amountOut = order.Request.AmountIn, required
		case amountIn == nil || amountIn.Sign() <= 0:
			return builtFill{}, errors.Errorf("connector leg %s input %v is not positive", leg.Venue.Hex(), amountIn)
		}
		totalIn.Add(totalIn, amountIn)
		requests = append(requests, leg.Request(order.Request.TokenIn, amountIn, token, amountOut))
	}
	if totalIn.Cmp(order.Request.AmountIn) != 0 {
		return builtFill{}, errors.Errorf("connector leg inputs sum to %s, not the signed order input %s", totalIn, order.Request.AmountIn)
	}
	prepared, err := w.PrepareLegs(ctx, requests, backend.resolveDiscount)
	if err != nil {
		return builtFill{}, err
	}
	calls := make([]executorCall, 0, len(prepared))
	discountInputs := make([]executor.IReactorDiscountSwapInput, 0, len(prepared))
	totalOut := new(big.Int)
	validUntil := time.Unix(order.Request.Deadline.Int64(), 0)
	for i, leg := range prepared {
		terms := leg.Terms
		calls = append(calls, executorCall{Target: leg.Call.Executor, Value: new(big.Int), Data: leg.Call.Data})
		discountInputs = append(discountInputs, executor.IReactorDiscountSwapInput{
			Adapter: requests[i].Venue,
			DiscountSwap: executor.ILiquidLaneAdapterDiscountSwap{
				Discount:        executor.ILiquidLaneAdapterDiscount{TokenToRedeem: terms.Swap.Discount.TokenToRedeem, Discount: terms.Swap.Discount.Discount, Signer: terms.Swap.Discount.Signer, Protocol: terms.Swap.Discount.Protocol, Nonce: terms.Swap.Discount.Nonce, Deadline: terms.Swap.Discount.Deadline},
				SignerSignature: terms.Swap.SignerSignature, ProtocolDeadline: terms.Swap.ProtocolDeadline,
			},
			ProtocolSignature: terms.Signature, Recipient: w.Filler, AmountIn: requests[i].AmountIn,
		})
		totalOut.Add(totalOut, leg.AmountOut)
		if terms.ValidUntil.Before(validUntil) {
			validUntil = terms.ValidUntil
		}
	}
	if totalOut.Cmp(required) < 0 {
		return builtFill{}, errors.Errorf("connector legs pay %s below the signed order output %s", totalOut, required)
	}
	data, err := executorCallsArgs.Pack(calls)
	if err != nil {
		return builtFill{}, errors.Errorf("encode recipe calls: %w", err)
	}
	calldata, err := executorBinding.TryPackFill(order, protocolSig, nil, discountInputs, data)
	if err != nil {
		return builtFill{}, errors.Errorf("encode recipe fill: %w", err)
	}
	if err := w.Validate(ctx, calldata, prepared...); err != nil {
		return builtFill{}, errors.Errorf("validate outer RFQ recipe fill: %w", err)
	}
	return builtFill{Calldata: calldata, ValidUntil: validUntil, Legs: prepared}, nil
}
