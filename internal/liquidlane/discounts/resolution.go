package discounts

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type Selection struct {
	DiscountID common.Hash
	Adapter    common.Address
	TokenIn    common.Address
	TokenOut   common.Address
	AmountIn   *big.Int

	MinAmountOut *big.Int
}

type Provider interface {
	ListDiscounts(ctx context.Context) (*List, error)
	Resolve(ctx context.Context, discountID string) (*Resolved, error)
}

func ValidateSelection(signed *Signed, selection Selection, validAfter time.Time) error {
	if signed == nil {
		return errors.New("resolved discount is nil")
	}
	if signed.DiscountID != selection.DiscountID {
		return errors.New("resolved discount id does not match selected route")
	}
	if signed.Adapter != selection.Adapter || signed.Terms.TokenToRedeem != selection.TokenIn {
		return errors.New("resolved discount route does not match selected route")
	}
	if signed.Terms.Deadline == nil || signed.ProtocolDeadline == nil {
		return errors.New("resolved discount is missing deadlines")
	}
	cutoff := big.NewInt(validAfter.Unix())
	if signed.Terms.Deadline.Cmp(cutoff) <= 0 || signed.ProtocolDeadline.Cmp(cutoff) <= 0 {
		return errors.New("resolved discount expires before the execution safety window")
	}
	return nil
}

func ValidateSigned(
	signed *Signed,
	selection Selection,
	physical liquidlane.FillQuote,
	validAfter time.Time,
) (*big.Int, error) {
	if err := ValidateSelection(signed, selection, validAfter); err != nil {
		return nil, err
	}
	if physical.GrossAmountOut == nil || physical.GrossAmountOut.Sign() <= 0 ||
		physical.MinDiscount == nil || physical.MinDiscount.Sign() < 0 {
		return nil, errors.New("resolved discount has no current on-chain quote")
	}
	if signed.Terms.Discount == nil {
		return nil, errors.New("resolved discount is missing discount terms")
	}
	if signed.Terms.Discount.Cmp(physical.MinDiscount) < 0 {
		return nil, errors.New("resolved discount is below the adapter minimum")
	}
	amountOut := liquidlane.AmountOutAfterDiscount(physical.GrossAmountOut, signed.Terms.Discount)
	if selection.MinAmountOut != nil && amountOut.Cmp(selection.MinAmountOut) < 0 {
		return nil, errors.New("resolved discount no longer meets the selected minimum output")
	}
	return amountOut, nil
}

func ParseAndValidate(
	resolved *Resolved,
	selection Selection,
	physical liquidlane.FillQuote,
	validAfter time.Time,
) (*Signed, error) {
	signed, err := ParseSigned(resolved)
	if err != nil {
		return nil, err
	}
	if _, err := ValidateSigned(signed, selection, physical, validAfter); err != nil {
		return nil, err
	}
	return signed, nil
}

func ResolveSelected(
	ctx context.Context,
	provider Provider,
	selection Selection,
	physical []liquidlane.FillQuote,
	validAfter time.Time,
) (*Signed, error) {
	base, exists := FindFillQuote(
		physical, selection.Adapter, selection.TokenIn, selection.TokenOut, selection.AmountIn,
	)
	if !exists {
		return nil, errors.New("resolved discount has no current on-chain quote")
	}
	if provider == nil {
		return nil, errors.New("discount route cannot be resolved")
	}
	resolved, err := provider.Resolve(ctx, selection.DiscountID.Hex())
	if err != nil {
		return nil, err
	}
	return ParseAndValidate(resolved, selection, base, validAfter)
}

func RefreshFillQuotes(
	candidates []liquidlane.FillQuote,
	resolved map[common.Hash]*Signed,
	physical []liquidlane.FillQuote,
	now time.Time,
) ([]liquidlane.FillQuote, []OfferIssue) {
	physicalByRoute := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(physical))
	for _, quote := range physical {
		physicalByRoute[quote.ID] = quote
	}
	refreshed := make([]liquidlane.FillQuote, 0, len(candidates))
	issues := make([]OfferIssue, 0)
	for _, candidate := range candidates {
		if candidate.DiscountID == nil {
			continue
		}
		signed := resolved[*candidate.DiscountID]
		base, exists := physicalByRoute[candidate.ID]
		if signed == nil || !exists {
			continue
		}
		if candidate.MaxRate == nil || base.MaxRate == nil || candidate.MaxRate.Cmp(base.MaxRate) > 0 {
			issues = append(issues, OfferIssue{
				DiscountID: candidate.DiscountID.Hex(),
				Err:        errors.New("resolved discount rate exceeds refreshed adapter max rate"),
			})
			continue
		}
		candidate.MaxAssets = positiveMinimum(candidate.MaxAssets, base.MaxAssets)
		if candidate.MaxAssets.Sign() <= 0 {
			continue
		}
		amountOut, err := ValidateSigned(signed, Selection{
			DiscountID: *candidate.DiscountID, Adapter: candidate.Adapter, TokenIn: candidate.TokenIn,
		}, base, now)
		if err != nil {
			issues = append(issues, OfferIssue{DiscountID: candidate.DiscountID.Hex(), Err: err})
			continue
		}
		candidate.AmountIn = liquidlane.CloneBig(base.AmountIn)
		candidate.GrossAmountOut = liquidlane.CloneBig(base.GrossAmountOut)
		candidate.MaxAmountOut = amountOut
		candidate.MinDiscount = liquidlane.CloneBig(base.MinDiscount)
		candidate.ValidUntil = ValidUntil(signed)
		refreshed = append(refreshed, candidate)
	}
	return refreshed, issues
}

func FindFillQuote(
	quotes []liquidlane.FillQuote,
	adapter, tokenIn, tokenOut common.Address,
	amountIn *big.Int,
) (liquidlane.FillQuote, bool) {
	if amountIn == nil {
		return liquidlane.FillQuote{}, false
	}
	for _, quote := range quotes {
		if quote.Adapter == adapter && quote.TokenIn == tokenIn && quote.TokenOut == tokenOut &&
			quote.AmountIn != nil && quote.AmountIn.Cmp(amountIn) == 0 {
			return quote, true
		}
	}
	return liquidlane.FillQuote{}, false
}

func ValidUntil(signed *Signed) time.Time {
	if signed == nil || signed.Terms.Deadline == nil || signed.ProtocolDeadline == nil {
		return time.Time{}
	}
	deadline := signed.Terms.Deadline
	if signed.ProtocolDeadline.Cmp(deadline) < 0 {
		deadline = signed.ProtocolDeadline
	}
	return time.Unix(deadline.Int64(), 0)
}
