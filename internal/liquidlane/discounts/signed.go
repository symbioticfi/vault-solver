package discounts

import (
	"context"
	"math/big"
	"time"

	"github.com/symbioticfi/vault-solver/internal/bigmath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Selection identifies the route and economic floor chosen before resolving a fresh discount.
type Selection struct {
	DiscountID common.Hash
	Adapter    common.Address
	TokenIn    common.Address
	TokenOut   common.Address
	AmountIn   *big.Int

	MinAmountOut *big.Int
}

// Provider is the shared signed-discount API surface used by direct LiquidLane clients.
type Provider interface {
	ListDiscounts(ctx context.Context) (*List, error)
	Resolve(ctx context.Context, discountID string) (*Resolved, error)
}

// ValidateSigned verifies fresh signed terms against the selected route and current on-chain quote.
// It returns the currently executable output after the signed discount.
func ValidateSigned(
	signed *Signed,
	selection Selection,
	base liquidlane.FillQuote,
	validAfter time.Time,
) (*big.Int, error) {
	if err := ValidateSelection(signed, selection, validAfter); err != nil {
		return nil, err
	}
	if base.Adapter != selection.Adapter || base.TokenIn != selection.TokenIn ||
		(selection.TokenOut != (common.Address{}) && base.TokenOut != selection.TokenOut) ||
		(selection.AmountIn != nil && (base.AmountIn == nil || base.AmountIn.Cmp(selection.AmountIn) != 0)) {
		return nil, errors.New("resolved discount current quote does not match selected route")
	}
	if base.GrossAmountOut == nil || base.GrossAmountOut.Sign() <= 0 ||
		base.MinDiscount == nil || base.MinDiscount.Sign() < 0 {
		return nil, errors.New("resolved discount has no current on-chain quote")
	}
	if signed.Terms.Discount == nil {
		return nil, errors.New("resolved discount is missing discount terms")
	}
	if signed.Terms.Discount.Cmp(base.MinDiscount) < 0 {
		return nil, errors.New("resolved discount is below the adapter minimum")
	}
	amountOut := liquidlane.AmountOutAfterDiscount(base.GrossAmountOut, signed.Terms.Discount)
	if selection.MinAmountOut != nil && amountOut.Cmp(selection.MinAmountOut) < 0 {
		return nil, errors.New("resolved discount no longer meets the selected minimum output")
	}
	return amountOut, nil
}

// ValidateSelection verifies signed identity, route binding, and execution deadlines.
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

// ParseAndValidate parses a resolved backend payload and validates it against current route facts.
func ParseAndValidate(
	resolved *Resolved,
	selection Selection,
	base liquidlane.FillQuote,
	validAfter time.Time,
) (*Signed, error) {
	signed, err := ParseSigned(resolved)
	if err != nil {
		return nil, err
	}
	if _, err := ValidateSigned(signed, selection, base, validAfter); err != nil {
		return nil, err
	}
	return signed, nil
}

// ResolveSelected fetches and validates the signed discount chosen for one exact fill route.
func ResolveSelected(
	ctx context.Context,
	provider Provider,
	selection Selection,
	physical []liquidlane.FillQuote,
	validAfter time.Time,
) (*Signed, error) {
	if provider == nil {
		return nil, errors.New("discount route cannot be resolved")
	}
	base, ok := FindFillQuote(
		physical,
		selection.Adapter,
		selection.TokenIn,
		selection.TokenOut,
		selection.AmountIn,
	)
	if !ok {
		return nil, errors.New("resolved discount has no current on-chain quote")
	}
	resolved, err := provider.Resolve(ctx, selection.DiscountID.Hex())
	if err != nil {
		return nil, err
	}
	return ParseAndValidate(resolved, selection, base, validAfter)
}

// RefreshFillQuotes rebinds resolved discount candidates to a newer physical adapter snapshot.
func RefreshFillQuotes(candidates []liquidlane.FillQuote, resolved map[common.Hash]*Signed,
	physical []liquidlane.FillQuote, now time.Time) ([]liquidlane.FillQuote, []OfferIssue) {
	current := make(map[liquidlane.RouteID]liquidlane.FillQuote, len(physical))
	for _, quote := range physical {
		current[quote.ID] = quote
	}
	refreshed := make([]liquidlane.FillQuote, 0, len(candidates))
	issues := make([]OfferIssue, 0)
	for _, candidate := range candidates {
		if candidate.DiscountID == nil {
			continue
		}
		signed := resolved[*candidate.DiscountID]
		base, exists := current[candidate.ID]
		if signed == nil || !exists {
			continue
		}
		quote, err := BindFillQuote(candidate.Inventory, signed, base, now)
		if err != nil {
			issues = append(issues, OfferIssue{DiscountID: candidate.DiscountID.Hex(), Err: err})
			continue
		}
		if quote != nil {
			refreshed = append(refreshed, *quote)
		}
	}
	return refreshed, issues
}

// BindFillQuote applies validated signed terms to the current physical route and owns the resulting quote.
// Both initial resolution and later refreshes retain the advertised rate and bounded capacity.
func BindFillQuote(candidate liquidlane.Inventory, signed *Signed, base liquidlane.FillQuote, now time.Time) (*liquidlane.FillQuote, error) {
	if candidate.DiscountID == nil {
		return nil, errors.New("discount candidate has no signed identity")
	}
	if candidate.MaxRate == nil || base.MaxRate == nil || candidate.MaxRate.Cmp(base.MaxRate) > 0 {
		return nil, errors.New("resolved discount rate exceeds refreshed adapter max rate")
	}
	capacity := minPositive(candidate.MaxAssets, base.MaxAssets)
	if capacity.Sign() <= 0 {
		return nil, nil
	}
	amount, err := ValidateSigned(signed, Selection{DiscountID: *candidate.DiscountID,
		Adapter: candidate.Adapter, TokenIn: candidate.TokenIn, TokenOut: candidate.TokenOut}, base, now)
	if err != nil {
		return nil, err
	}
	// All physical observations come from this refresh. Only the offered capacity,
	// selected rate and signed identity survive from the earlier candidate.
	base.MaxAssets, base.MaxRate = capacity, bigmath.Clone(candidate.MaxRate)
	base.AdapterMinDiscount = bigmath.Clone(base.AdapterMinDiscount)
	base.DiscountID = liquidlane.CloneHash(candidate.DiscountID)
	base.ValidUntil, base.MaxAmountOut = ValidUntil(signed), amount
	base.AmountIn, base.GrossAmountOut, base.MinDiscount = bigmath.Clone(base.AmountIn), bigmath.Clone(base.GrossAmountOut), bigmath.Clone(base.MinDiscount)
	return &base, nil
}

// FindFillQuote returns the current physical quote matching a selected route and exact amount.
func FindFillQuote(
	quotes []liquidlane.FillQuote,
	adapter, tokenIn, tokenOut common.Address,
	amountIn *big.Int,
) (liquidlane.FillQuote, bool) {
	for _, quote := range quotes {
		if quote.Adapter == adapter && quote.TokenIn == tokenIn && quote.TokenOut == tokenOut &&
			amountIn != nil && quote.AmountIn != nil && quote.AmountIn.Cmp(amountIn) == 0 {
			return quote, true
		}
	}
	return liquidlane.FillQuote{}, false
}

// ValidUntil returns the earliest signed discount deadline.
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
