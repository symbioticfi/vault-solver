package uniswapx

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/default"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

// The mainnet profile quotes with no price buffer, so the quote must be the adapter payout itself:
// one unit above it cannot be delivered, and a fill that reserves less than the payout aborts.
// Every private quote here must equal the adapter payout for the same oracle state, and the
// awarded order must plan, reserve and resolve at exactly that payout.
func TestPrivateDiscountQuotesAndFillsAtAdapterPayout(t *testing.T) {
	strategy, err := defaultstrategy.New(defaultstrategy.Config{
		Name: types.SingleName, InventoryReserveBps: 500, ExecutionDeadlineBuffer: "12s",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	adapter := common.HexToAddress("0x59cdde0d345c0ee6ed453fe7d3fb365fe0721e85")
	collateral := common.HexToAddress("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")
	maxAssets := big.NewInt(1_237_450_893)
	quoteCapacity := big.NewInt(1_175_578_348) // maxAssets after the 5% inventory reserve
	// Oracle prices (1e10 units) of tokens on the mainnet adapter. The 6-decimal ones make the
	// discounted per-token rate nearly exact, which is where rate pricing over-quoted by one unit.
	prices := []int64{112_837_214, 115_013_646, 102_241_277, 106_177_400, 111_727_500, 173_432_100}
	discounts := []int64{0, 1, 5, 37, 200, 1_000, 2_500, 40_000, 100_000}
	sizes := []int64{1, 7, 64, 123, 777, 1_100} // whole tokens, the last above capacity

	for _, inDec := range []int{18, 6} {
		tokenIn := common.BigToAddress(big.NewInt(int64(inDec)))
		route := liquidlane.NewRoute(1, adapter, common.HexToAddress("0xdb"), tokenIn, collateral, inDec, 6)
		for _, rawPrice := range prices {
			price := new(big.Int).Mul(big.NewInt(rawPrice), big.NewInt(10_000_000_000))
			physical := liquidlane.DirectInventory(route, maxAssets, price)
			physical.AdapterMinDiscount, physical.Price = new(big.Int), price
			for _, ppm := range discounts {
				pair := discountPricingPair{
					t: t, strategy: strategy, now: now, route: route, physical: physical,
					discount: big.NewInt(ppm), id: common.HexToHash("0xd15c"),
				}
				pair.list(maxAssets)
				for index, units := range sizes {
					amountIn := new(big.Int).Mul(big.NewInt(units*1_000+int64(index+1)*123), pow10(inDec-3))
					amountIn.Add(amountIn, big.NewInt(987_654))
					pair.check(amountIn, quoteCapacity)
				}
			}
		}
	}
}

// discountPricingPair quotes and fills one advertised discount on one physical route.
type discountPricingPair struct {
	t        *testing.T
	strategy types.Strategy
	now      time.Time
	route    liquidlane.Route
	physical liquidlane.Inventory
	discount *big.Int
	id       common.Hash
	listed   *liquiddiscounts.List
	quoting  []liquidlane.Inventory
}

func (p *discountPricingPair) list(maxAssets *big.Int) {
	price, inDec := p.physical.Price, p.route.TokenInDecimals
	oneToken := liquidlane.AmountOutForRate(pow10(inDec), price, inDec, 6)
	p.listed = &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{{
		DiscountID: p.id.Hex(), Adapter: p.route.Adapter.Hex(), TokenToRedeem: p.route.TokenIn.Hex(),
		Collateral: p.route.TokenOut.Hex(), CollateralDecimals: 6, Discount: p.discount.String(),
		Deadline: p.now.Add(time.Hour).Unix(), MaxAssets: maxAssets.String(),
		// The backend's advertised rate: the discounted output for one whole token, floored.
		MaxRate: new(big.Int).Mul(liquidlane.AmountOutAfterDiscount(oneToken, p.discount), pow10(12)).String(),
	}}}
	inventory, issues := liquiddiscounts.MatchInventories(p.listed, []liquidlane.Inventory{p.physical},
		liquiddiscounts.MatchOptions{Now: p.now})
	if len(issues) != 0 || len(inventory) != 1 {
		p.t.Fatalf("inventory=%+v issues=%+v", inventory, issues)
	}
	p.quoting = inventory
}

func (p *discountPricingPair) payout(amountIn *big.Int) *big.Int {
	return liquidlane.DiscountedAmountOut(
		amountIn, p.physical.Price, p.discount, p.route.TokenInDecimals, p.route.TokenOutDecimals,
	)
}

func (p *discountPricingPair) quote(amountIn, amountOut *big.Int) *types.Quote {
	quote, err := p.strategy.DecideQuote(context.Background(), types.QuoteInput{
		TokenIn: p.route.TokenIn, TokenOut: p.route.TokenOut, AmountIn: amountIn, AmountOut: amountOut,
		RequireSingleRoute: true, Inventory: p.quoting, MaxFeePerGas: new(big.Int),
		ChainTime: p.now, QuoteExpiresAt: p.now.Add(30 * time.Second),
	})
	if err != nil {
		p.t.Fatal(err)
	}
	return quote
}

func (p *discountPricingPair) check(amountIn, quoteCapacity *big.Int) {
	t := p.t
	t.Helper()
	if exactIn := p.quote(amountIn, nil); exactIn != nil {
		if exactIn.AmountOut.Cmp(p.payout(amountIn)) != 0 {
			t.Fatalf("exact input %s: quoted %s, adapter pays %s", amountIn, exactIn.AmountOut, p.payout(amountIn))
		}
		p.fill(amountIn, exactIn.AmountOut)
	} else if p.payout(amountIn).Cmp(quoteCapacity) <= 0 {
		t.Fatalf("exact input %s within capacity was not quoted", amountIn)
	}

	wanted := new(big.Int).Div(p.payout(amountIn), big.NewInt(7))
	if wanted.Sign() == 0 {
		return
	}
	exactOut := p.quote(nil, wanted)
	if exactOut == nil {
		t.Fatalf("exact output %s was not quoted", wanted)
	}
	minimum := liquidlane.MinAmountInForDiscountedAmountOut(
		wanted, p.physical.Price, p.discount, p.route.TokenInDecimals, p.route.TokenOutDecimals,
	)
	if exactOut.AmountIn.Cmp(minimum) != 0 || p.payout(exactOut.AmountIn).Cmp(wanted) < 0 {
		t.Fatalf("exact output %s: quoted input %s, minimal input %s", wanted, exactOut.AmountIn, minimum)
	}
	p.fill(exactOut.AmountIn, wanted)
}

// fill plans the awarded order against the fill-time quote ReadFillQuotes would return and resolves
// the unchanged signed discount against the plan.
func (p *discountPricingPair) fill(amountIn, amountOut *big.Int) {
	t := p.t
	t.Helper()
	inDec := p.route.TokenInDecimals
	gross := liquidlane.AmountOutForRate(amountIn, p.physical.Price, inDec, 6)
	quoted := liquidlane.DirectInventory(p.route, p.physical.MaxAssets, liquidlane.RateForAmountOut(gross, amountIn, inDec, 6))
	quoted.AdapterMinDiscount = new(big.Int)
	base := liquidlane.FillQuote{
		Inventory: quoted, AmountIn: amountIn, GrossAmountOut: gross, MaxAmountOut: gross, MinDiscount: new(big.Int),
	}
	quotes, issues := liquiddiscounts.AdvertisedFillQuotes(p.listed, []liquidlane.FillQuote{base},
		liquiddiscounts.MatchOptions{Now: p.now})
	if len(issues) != 0 || len(quotes) != 1 {
		t.Fatalf("fill quotes=%+v issues=%+v", quotes, issues)
	}
	input := types.FillInput{
		TokenIn: p.route.TokenIn, TokenOut: p.route.TokenOut, AmountIn: amountIn, OutputAmount: amountOut,
		Deadline: uint32(p.now.Add(2 * time.Minute).Unix()), RequireSingleRoute: true, Quotes: quotes,
		CapacityLimits: fillCapacityLimits(fillSnapshot{Physical: []liquidlane.FillQuote{base}, Direct: quotes}),
		MaxFeePerGas:   new(big.Int), ChainTime: p.now,
	}
	plan, err := p.strategy.DecideFill(context.Background(), input)
	if err != nil || plan == nil {
		t.Fatalf("amountIn %s: no fill plan for the quoted %s (err %v)", amountIn, amountOut, err)
	}
	if err := validatePreparedFill(input, plan); err != nil {
		t.Fatal(err)
	}
	route := plan.Routes[0]
	deadline := big.NewInt(p.now.Add(time.Hour).Unix())
	signed := &liquiddiscounts.Signed{
		DiscountID: p.id, Adapter: p.route.Adapter,
		Terms:            liquiddiscounts.SignedTerms{TokenToRedeem: p.route.TokenIn, Discount: p.discount, Deadline: deadline},
		ProtocolDeadline: deadline,
	}
	resolved, err := liquiddiscounts.ValidateSigned(signed, liquiddiscounts.Selection{
		DiscountID: p.id, Adapter: p.route.Adapter, TokenIn: p.route.TokenIn,
		MinAmountOut: route.MinAmountOut, MaxAmountOut: route.ReservedAmountOut,
	}, base, p.now)
	if err != nil || resolved.Cmp(p.payout(amountIn)) != 0 || route.ReservedAmountOut.Cmp(resolved) != 0 {
		t.Fatalf("amountIn %s: resolved %s, reserved %s, payout %s: %v",
			amountIn, resolved, route.ReservedAmountOut, p.payout(amountIn), err)
	}
}

func pow10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
