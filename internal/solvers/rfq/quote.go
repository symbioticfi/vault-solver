package rfq

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// quoteService prices backend RFQ requests by handing filtered candidates to the strategy. It is safe
// for concurrent use (the HTTP server serves quotes in parallel): its dependencies are individually
// synchronized, and it holds no mutable state itself.
type quoteService struct {
	chainID  int64
	executor common.Address
	// laneReady is safe for concurrent use and reflects whether the shared nonce lane can immediately
	// accept work. It is sampled before and after quote planning so work is declined whenever either
	// check observes an occupied or conflicted lane.
	laneReady   func() bool
	whitelist   adapterWhitelist // nil disables adapter filtering
	tokenPolicy tokenpolicy.Policy
	// minAmountsIn holds per-input-token minimum request sizes in base units; a token absent from the
	// map (or a nil map) has no minimum.
	minAmountsIn map[common.Address]*big.Int
	reader       quoteCandidateReader
	strategy     types.Strategy
	log          logr.Logger
	now          func() time.Time
}

type quoteCandidateReader interface {
	readQuoteCandidates(
		ctx context.Context,
		inventory []solverInventory,
		tokenIn common.Address,
		tokenOut common.Address,
		amountIn *big.Int,
	) ([]liquidlane.QuoteCandidate, error)
}

// quote returns a priced quote, or nil (→ HTTP 204) when the request is well-formed but this filler
// can't quote it (wrong type/chain, input token out of scope or below its configured minimum, no
// whitelisted adapter, no matching asset, or no viable strategy). An error is returned only for
// malformed input or a failed chain read.
func (qs *quoteService) quote(ctx context.Context, q *quoteRequest) (*quoteResponse, error) {
	parsed, err := q.toStrategy(qs.chainID)
	if err != nil {
		return nil, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	if !qs.canQuote() {
		qs.log.V(1).Info("declining quote: transaction lane not ready", "quoteId", q.QuoteID)
		return nil, nil
	}
	if parsed == nil {
		qs.log.V(1).Info("declining quote: not quotable", "quoteId", q.QuoteID, "type", q.Type)
		return nil, nil
	}
	if !qs.tokenPolicy.Allows(parsed.req.TokenIn) {
		qs.log.V(1).Info("declining quote: input token out of scope",
			"quoteId", q.QuoteID, "tokenIn", lowerAddr(parsed.req.TokenIn), "scope", qs.tokenPolicy.Scope())
		return nil, nil
	}
	if minIn, ok := qs.minAmountsIn[parsed.req.TokenIn]; ok && parsed.req.Amount.Cmp(minIn) < 0 {
		qs.log.V(1).Info("declining quote: input amount below configured minimum",
			"quoteId", q.QuoteID, "tokenIn", lowerAddr(parsed.req.TokenIn),
			"amount", parsed.req.Amount.String(), "min", minIn.String())
		return nil, nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if len(inv) == 0 {
		qs.log.V(1).Info("declining quote: no whitelisted adapters", "quoteId", q.QuoteID)
		return nil, nil
	}

	requireSingleRoute := qs.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	candidates, err := qs.reader.readQuoteCandidates(ctx, inv, req.TokenIn, req.TokenOut, req.Amount)
	if err != nil {
		return nil, errors.Errorf("quote: read LiquidLane candidates: %w", err)
	}
	if len(candidates) == 0 {
		qs.log.V(1).Info("declining quote: no viable LiquidLane candidates", "quoteId", q.QuoteID)
		return nil, nil
	}
	input := newQuoteInput(qs.chainID, qs.executor, req, candidates, nil, requireSingleRoute, qs.now())
	out, err := qs.strategy.DecideQuote(ctx, input)
	if err != nil {
		return nil, errors.Errorf("quote: strategy: %w", err)
	}
	if out.Decision != types.DecisionQuote {
		qs.log.V(1).Info("declining quote: no viable strategy", "quoteId", q.QuoteID)
		return nil, nil
	}
	if _, err := strategies.FillPlanFromQuote(input, out); err != nil {
		return nil, errors.Errorf("quote: strategy: %w", err)
	}
	if !qs.canQuote() {
		qs.log.V(1).Info("declining quote: transaction lane no longer ready", "quoteId", q.QuoteID)
		return nil, nil
	}

	qs.log.V(1).Info("quoted",
		"quoteId", q.QuoteID, "amountIn", req.Amount.String(),
		"amountOut", out.QuotedAmountOut.String(), "legs", len(out.Legs))

	return &quoteResponse{
		ChainID:   qs.chainID,
		AmountIn:  req.Amount.String(),
		AmountOut: out.QuotedAmountOut.String(),
		Filler:    lowerAddr(qs.executor),
		RequestID: q.RequestID,
		Swapper:   lowerAddr(common.HexToAddress(q.Swapper)), // backend payloads use lowercase addresses
		TokenIn:   lowerAddr(req.TokenIn),
		TokenOut:  lowerAddr(req.TokenOut),
		QuoteID:   q.QuoteID,
	}, nil
}

// canQuote fails closed when the lane-state dependency was not wired. Production construction
// always supplies the txmanager predicate; keeping the nil case closed prevents a future alternate
// constructor from silently advertising obligations it cannot fill.
func (qs *quoteService) canQuote() bool {
	return qs.laneReady != nil && qs.laneReady()
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }
