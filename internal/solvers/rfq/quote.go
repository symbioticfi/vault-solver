package rfq

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// quoteService prices backend RFQ requests by handing filtered candidates to the planner. It is safe
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
	planner      Planner
	capacity     *capacity.Book
	metrics      *rfqMetrics
	log          logr.Logger
	now          func() time.Time
}

type quoteCandidateReader interface {
	readQuoteCandidates(
		ctx context.Context,
		inventory []liquidlane.Inventory,
		tokenIn common.Address,
		tokenOut common.Address,
		amountIn *big.Int,
		reservations liquidlane.CapacityReservations,
	) ([]liquidlane.QuoteCandidate, error)
}

// quote returns a priced quote, or nil (→ HTTP 204) when the request is well-formed but this filler
// can't quote it (wrong type/chain, input token out of scope or below its configured minimum, no
// whitelisted adapter, no matching asset, or no viable planner). An error is returned only for
// malformed input or a failed chain read.
func (qs *quoteService) quote(ctx context.Context, q *quoteRequest) (response *quoteResponse, err error) {
	outcome := quoteDecisionError
	defer func() { qs.metrics.quote(outcome, response) }()
	parsed, err := q.toStrategy(qs.chainID)
	if err != nil {
		outcome = quoteDecisionBadRequest
		return nil, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	if !qs.canQuote() {
		outcome = quoteDecisionLaneUnavailable
		qs.log.V(1).Info("declining quote: transaction lane not ready", "quoteId", q.QuoteID)
		return nil, nil
	}
	if parsed == nil {
		outcome = quoteDecisionNotQuotable
		qs.log.V(1).Info("declining quote: not quotable", "quoteId", q.QuoteID, "type", q.Type)
		return nil, nil
	}
	if !qs.allowsInput(parsed.req, q.QuoteID) {
		minimum, configured := qs.minAmountsIn[parsed.req.TokenIn]
		if configured && parsed.req.Amount.Cmp(minimum) < 0 {
			outcome = quoteDecisionBelowMinimum
		} else {
			outcome = quoteDecisionNotQuotable
		}
		return nil, nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if len(inv) == 0 {
		outcome = quoteDecisionNoCandidates
		qs.log.V(1).Info("declining quote: no whitelisted adapters", "quoteId", q.QuoteID)
		return nil, nil
	}

	requireSingleRoute := qs.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	candidates, err := qs.reader.readQuoteCandidates(
		ctx, inv, req.TokenIn, req.TokenOut, req.Amount, qs.capacity.Snapshot(),
	)
	if err != nil {
		return nil, errors.Errorf("quote: read LiquidLane candidates: %w", err)
	}
	if len(candidates) == 0 {
		outcome = quoteDecisionNoCandidates
		qs.log.V(1).Info("declining quote: no viable LiquidLane candidates", "quoteId", q.QuoteID)
		return nil, nil
	}
	input := newQuoteInput(qs.chainID, qs.executor, req, candidates, nil, requireSingleRoute, qs.now())
	out, err := qs.planner.DecideQuote(ctx, input)
	if err != nil {
		return nil, errors.Errorf("quote: planner: %w", err)
	}
	if out.Decision != DecisionQuote {
		outcome = quoteDecisionStrategyDeclined
		qs.log.V(1).Info("declining quote: no viable planner", "quoteId", q.QuoteID, "reason", out.Reason)
		return nil, nil
	}
	if _, err := FillPlanFromQuote(input, out); err != nil {
		return nil, errors.Errorf("quote: planner: %w", err)
	}
	if !qs.canQuote() {
		outcome = quoteDecisionLaneUnavailable
		qs.log.V(1).Info("declining quote: transaction lane no longer ready", "quoteId", q.QuoteID)
		return nil, nil
	}

	qs.log.V(1).Info("quoted",
		"quoteId", q.QuoteID, "amountIn", req.Amount.String(),
		"amountOut", out.QuotedAmountOut.String(), "legs", len(out.Legs))

	outcome = quoteDecisionQuoted
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

func (qs *quoteService) allowsInput(request quoteRequestFacts, quoteID string) bool {
	if !qs.tokenPolicy.Allows(request.TokenIn) {
		qs.log.V(1).Info("declining quote: input token out of scope",
			"quoteId", quoteID, "tokenIn", lowerAddr(request.TokenIn), "scope", qs.tokenPolicy.Scope())
		return false
	}
	minimum, configured := qs.minAmountsIn[request.TokenIn]
	if configured && request.Amount.Cmp(minimum) < 0 {
		qs.log.V(1).Info("declining quote: input amount below configured minimum",
			"quoteId", quoteID, "tokenIn", lowerAddr(request.TokenIn),
			"amount", request.Amount.String(), "min", minimum.String())
		return false
	}
	return true
}

// canQuote fails closed when the lane-state dependency was not wired. Production construction
// always supplies the txmanager predicate; keeping the nil case closed prevents a future alternate
// constructor from silently advertising obligations it cannot fill.
func (qs *quoteService) canQuote() bool {
	return qs.laneReady != nil && qs.laneReady()
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }
