package filler

import (
	"cmp"
	"context"
	"iter"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// Source is one advertised connector discount backed by a configured recipe.
type Source struct {
	Venue          common.Address // LiquidLaneConnector
	RecipeExecutor common.Address
	Account        common.Address
	DiscountID     common.Hash
	MaxAssets      *big.Int
	// Listing is the advertised discount the source was read from.
	Listing discounts.ListItem
}

// Request sizes the source's swap; a nil amountOut takes the connector's own payment.
func (s Source) Request(tokenIn common.Address, amountIn *big.Int, tokenOut common.Address, amountOut *big.Int) connector.Request {
	return connector.Request{
		Venue: s.Venue, Executor: s.RecipeExecutor, DiscountID: s.DiscountID, MaxAssets: s.MaxAssets,
		TokenIn: tokenIn, AmountIn: amountIn, TokenOut: tokenOut, AmountOut: amountOut,
	}
}

// Spend is the capacity the legs' exact payments of tokenOut take from their funding accounts.
func (e *Engine) Spend(tokenOut common.Address, deadline int64, legs ...connector.Prepared) connector.Spend {
	amounts := make(liquidlane.CapacityReservations, len(legs))
	for _, leg := range legs {
		amounts.Add(liquidlane.NewCapacityID(e.Workflow.ChainID.Int64(), leg.State.Account, tokenOut), leg.AmountOut)
	}
	return connector.Spend{Amounts: amounts, Deadline: deadline}
}

// Sources yields the advertised connector discounts that can swap tokenIn for tokenOut through
// a configured recipe, in a stable order. A malformed matching listing ends the sequence with
// an error: upstream inventory that cannot be trusted must not be skipped silently.
func (e *Engine) Sources(listed []discounts.ListItem, tokenIn, tokenOut common.Address) iter.Seq2[Source, error] {
	return func(yield func(Source, error) bool) {
		for match, err := range e.matches(listed, func(in, out common.Address) bool { return in == tokenIn && out == tokenOut }) {
			if !yield(match.Source, err) || err != nil {
				return
			}
		}
	}
}

// Decimals reads a token's decimals.
type Decimals func(ctx context.Context, token common.Address) (int, error)

// Inventory prices every advertised connector discount with a configured recipe against its
// connector's current policy. Offers that cannot be priced are dropped and reported in skipped,
// next to the inventory that can; err reports a failure that leaves no inventory to trust, so
// callers keep what they last published instead of an empty inventory.
func (e *Engine) Inventory(ctx context.Context, listed []discounts.ListItem) (inventory []liquidlane.Inventory, skipped, err error) {
	if e.Decimals == nil {
		return nil, nil, errors.New("token decimals reader is not configured")
	}
	var offers []connector.Offer
	var dropped []error
	for match, err := range e.matches(listed, nil) {
		if err != nil {
			return nil, nil, err
		}
		offer, err := match.offer(ctx, e.Decimals)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			dropped = append(dropped, errors.Errorf("connector %s discount %s: %w", match.Venue.Hex(), match.DiscountID.Hex(), err))
			continue
		}
		offers = append(offers, offer)
	}
	inventory, unread, err := connector.ReadInventory(ctx, e.Workflow.Chain, e.Workflow.ChainID.Int64(), offers)
	if err != nil {
		return nil, nil, err
	}
	return inventory, errors.Join(append(dropped, unread)...), nil
}

// Unreserved is the source's advertised funding less other awards' reservations
// and spends its listing has yet to reflect.
func (e *Engine) Unreserved(source Source, tokenOut common.Address, orderID string) *big.Int {
	capacity := liquidlane.NewCapacityID(e.Workflow.ChainID.Int64(), source.Account, tokenOut)
	reserved := e.Spends.ReservationsExcluding([]liquidlane.Inventory{{
		CapacityID: capacity, MaxAssets: source.MaxAssets, BlockNumber: source.Listing.BlockNumber,
	}}, orderID)[capacity]
	if reserved == nil {
		return source.MaxAssets
	}
	return new(big.Int).Sub(source.MaxAssets, reserved)
}

type listedSource struct {
	Source

	tokenIn, tokenOut common.Address
}

func (m listedSource) offer(ctx context.Context, decimals Decimals) (connector.Offer, error) {
	discount, ok := new(big.Int).SetString(m.Listing.Discount, 10)
	if !ok {
		return connector.Offer{}, errors.New("invalid connector discount")
	}
	inDecimals, err := decimals(ctx, m.tokenIn)
	if err != nil {
		return connector.Offer{}, errors.Errorf("tokenIn decimals: %w", err)
	}
	outDecimals, err := decimals(ctx, m.tokenOut)
	if err != nil {
		return connector.Offer{}, errors.Errorf("tokenOut decimals: %w", err)
	}
	if outDecimals != m.Listing.CollateralDecimals {
		return connector.Offer{}, errors.New("advertised output decimals mismatch")
	}
	return connector.Offer{
		Venue: m.Venue, RecipeExecutor: m.RecipeExecutor, Account: m.Account, TokenIn: m.tokenIn, TokenOut: m.tokenOut,
		TokenInDecimals: inDecimals, TokenOutDecimals: outDecimals,
		DiscountID: m.DiscountID, Discount: discount, MaxAssets: m.MaxAssets,
		BlockNumber: m.Listing.BlockNumber, ValidUntil: time.Unix(m.Listing.Deadline, 0),
	}, nil
}

// matches yields the connector listings backed by a configured recipe, in a stable order.
// pair, when set, narrows them before a listing is validated.
func (e *Engine) matches(listed []discounts.ListItem, pair func(tokenIn, tokenOut common.Address) bool) iter.Seq2[listedSource, error] {
	ordered := slices.Clone(listed)
	slices.SortStableFunc(ordered, func(a, b discounts.ListItem) int {
		if byVenue := common.HexToAddress(a.Adapter).Cmp(common.HexToAddress(b.Adapter)); byVenue != 0 {
			return byVenue
		}
		return cmp.Compare(a.DiscountID, b.DiscountID)
	})
	return func(yield func(listedSource, error) bool) {
		for _, item := range ordered {
			if item.DiscountPrecision != connector.PrecisionText {
				continue
			}
			venue, tokenIn, tokenOut := common.HexToAddress(item.Adapter), common.HexToAddress(item.TokenToRedeem), common.HexToAddress(item.Collateral)
			if pair != nil && !pair(tokenIn, tokenOut) {
				continue
			}
			recipeExecutor, account, configured := e.ResolveRoute(venue, tokenIn, tokenOut)
			if !configured {
				continue
			}
			id, err := hexutil.Decode(item.DiscountID)
			if err != nil || len(id) != 32 {
				yield(listedSource{}, errors.New("invalid connector discountId"))
				return
			}
			maximum, ok := new(big.Int).SetString(item.MaxAssets, 10)
			if !ok || maximum.Sign() < 0 || maximum.BitLen() > 256 {
				yield(listedSource{}, errors.New("invalid connector maxAssets"))
				return
			}
			source := Source{Venue: venue, RecipeExecutor: recipeExecutor, Account: account, DiscountID: common.BytesToHash(id), MaxAssets: maximum, Listing: item}
			if !yield(listedSource{Source: source, tokenIn: tokenIn, tokenOut: tokenOut}, nil) {
				return
			}
		}
	}
}

// Observation pins the chain time a fill is planned against.
type Observation struct {
	// Chain is the latest block time; Wall is the local time sampled just before reading it.
	Chain, Wall time.Time
}

// Observe reads the chain clock and retires spends whose signed deadline has passed.
func (e *Engine) Observe(ctx context.Context) (Observation, error) {
	wall := time.Now()
	header, err := e.Workflow.Chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return Observation{}, err
	}
	if header == nil || header.Time > ^uint64(0)>>1 {
		return Observation{}, errors.New("invalid chain timestamp")
	}
	e.Spends.Expire(int64(header.Time))
	return Observation{Chain: time.Unix(int64(header.Time), 0), Wall: wall}, nil
}

// SubmissionDeadline converts the earliest chain-time limit into the lane's wall-clock deadline.
func (o Observation) SubmissionDeadline(limits ...time.Time) (time.Time, bool) {
	if len(limits) == 0 {
		return time.Time{}, false
	}
	earliest := limits[0]
	for _, limit := range limits[1:] {
		if limit.Before(earliest) {
			earliest = limit
		}
	}
	return liquidlane.SubmissionDeadline(earliest, o.Chain, o.Wall, time.Now())
}
