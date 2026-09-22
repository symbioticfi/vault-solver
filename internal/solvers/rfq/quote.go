package rfq

import (
	"context"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
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
	discountsEnabled bool
	minAmountsIn     map[common.Address]*big.Int
	reader           quoteCandidateReader
	strategy         types.Strategy
	strategyName     string // registry key, reported as the strategy.name span attribute
	reservations     *liquidlane.CapacityLedger
	planningMu       *sync.Mutex
	log              logr.Logger
	now              func() time.Time
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

type quoteDecisionOutcome string

const (
	quoteDecisionQuoted           quoteDecisionOutcome = "quoted"
	quoteDecisionLaneUnavailable  quoteDecisionOutcome = "lane_unavailable"
	quoteDecisionNotQuotable      quoteDecisionOutcome = "not_quotable"
	quoteDecisionBelowMinimum     quoteDecisionOutcome = "below_minimum"
	quoteDecisionNoCandidates     quoteDecisionOutcome = "no_candidates"
	quoteDecisionStrategyDeclined quoteDecisionOutcome = "strategy_declined"
	quoteDecisionBadRequest       quoteDecisionOutcome = "bad_request"
	quoteDecisionError            quoteDecisionOutcome = "error"
)

var quoteDecisionOutcomes = [...]quoteDecisionOutcome{
	quoteDecisionQuoted,
	quoteDecisionLaneUnavailable,
	quoteDecisionNotQuotable,
	quoteDecisionBelowMinimum,
	quoteDecisionNoCandidates,
	quoteDecisionStrategyDeclined,
	quoteDecisionBadRequest,
	quoteDecisionError,
}

// quoteDecision keeps the domain classification next to the response it produced. The HTTP handler
// owns metrics observation, so direct/internal quote evaluation does not masquerade as transport
// traffic and every authenticated handler invocation records exactly one terminal decision.
type quoteDecision struct {
	response    *quoteResponse
	outcome     quoteDecisionOutcome
	observation *quoteObservation
}

type quoteObservation struct {
	tokenIn   common.Address
	tokenOut  common.Address
	amountIn  *big.Int
	amountOut *big.Int
}

// quote returns a terminal domain decision. Its response is nil (→ HTTP 204) when the request is
// well-formed but this filler can't quote it (wrong type/chain, input token out of scope or below its
// configured minimum, no whitelisted adapter, no matching asset, or no viable strategy). An error is
// returned only for malformed input or a failed dependency.
func (qs *quoteService) quote(ctx context.Context, q *quoteRequest) (decision quoteDecision, err error) {
	// The service's own logger, so the pipeline logs through it whatever context the caller brought.
	ctx = observability.WithLogger(ctx, qs.log)
	ctx, end := tracer.Start(ctx, "rfq.quote")
	// Deferred so the span still ends when the pipeline panics; recoverPanics turns that into a 500
	// without unwinding past here, and an unended span is never exported.
	defer func() {
		var bad *badRequestError
		switch {
		case errors.As(err, &bad):
			// A malformed payload is the caller's fault (400), not a solver failure.
			observability.Decline(ctx, "bad_request", bad.Error())
			end(nil)
		case err != nil:
			end(err)
		default:
			// A zero outcome means the pipeline panicked: end the span without recording a decision
			// it never reached.
			if decision.response == nil && decision.outcome != "" {
				observability.Decline(ctx, "no_quote", string(decision.outcome))
			}
			end(nil)
		}
	}()
	return qs.evaluate(ctx, q)
}

// evaluate is the quote pipeline; quote wraps it in the rfq.quote span and classifies its outcome.
func (qs *quoteService) evaluate(ctx context.Context, q *quoteRequest) (quoteDecision, error) {
	parsed, err := q.toStrategy(qs.chainID)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	if !qs.canQuote() {
		observability.Log(ctx).V(1).Info("declining quote: transaction lane not ready", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionLaneUnavailable}, nil
	}
	if parsed == nil {
		observability.Log(ctx).V(1).Info("declining quote: not quotable", "quoteId", q.QuoteID, "type", q.Type)
		return quoteDecision{outcome: quoteDecisionNotQuotable}, nil
	}
	if !qs.tokenPolicy.Allows(parsed.req.TokenIn) {
		observability.Log(ctx).V(1).Info("declining quote: input token out of scope",
			"quoteId", q.QuoteID, "tokenIn", lowerAddr(parsed.req.TokenIn), "scope", qs.tokenPolicy.Scope())
		return quoteDecision{outcome: quoteDecisionNotQuotable}, nil
	}
	if minIn, ok := qs.minAmountsIn[parsed.req.TokenIn]; ok && parsed.req.Amount.Cmp(minIn) < 0 {
		observability.Log(ctx).V(1).Info("declining quote: input amount below configured minimum",
			"quoteId", q.QuoteID, "tokenIn", lowerAddr(parsed.req.TokenIn),
			"amount", parsed.req.Amount.String(), "min", minIn.String())
		return quoteDecision{outcome: quoteDecisionBelowMinimum}, nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if !qs.discountsEnabled {
		inv = slices.DeleteFunc(slices.Clone(inv), func(item solverInventory) bool { return item.DiscountID != nil })
	}
	if len(inv) == 0 {
		observability.Log(ctx).V(1).Info("declining quote: no whitelisted adapters", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionNoCandidates}, nil
	}

	requireSingleRoute := qs.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	candidates, err := qs.snapshotCandidates(ctx, inv, req)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, errors.Errorf("quote: read LiquidLane candidates: %w", err)
	}
	if len(candidates) == 0 {
		observability.Log(ctx).V(1).Info("declining quote: no viable LiquidLane candidates", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionNoCandidates}, nil
	}
	input := newQuoteInput(qs.chainID, qs.executor, req, candidates, nil, requireSingleRoute, qs.now())
	out, err := qs.decideQuote(ctx, input)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, errors.Errorf("quote: strategy: %w", err)
	}
	if out.Decision != types.DecisionQuote {
		observability.Log(ctx).V(1).Info("declining quote: no viable strategy", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionStrategyDeclined}, nil
	}
	plan, err := strategies.FillPlanFromQuote(input, out)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, errors.Errorf("quote: strategy: %w", err)
	}
	traceAdapter(ctx, plan.Legs)
	if !qs.canQuote() {
		observability.Log(ctx).V(1).Info("declining quote: transaction lane no longer ready", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionLaneUnavailable}, nil
	}

	observability.Log(ctx).V(1).Info("quoted",
		"quoteId", q.QuoteID, "amountIn", req.Amount.String(),
		"amountOut", out.QuotedAmountOut.String(), "legs", len(out.Legs))

	response := &quoteResponse{
		ChainID:   qs.chainID,
		AmountIn:  req.Amount.String(),
		AmountOut: out.QuotedAmountOut.String(),
		Filler:    lowerAddr(qs.executor),
		RequestID: q.RequestID,
		Swapper:   lowerAddr(common.HexToAddress(q.Swapper)), // backend payloads use lowercase addresses
		TokenIn:   lowerAddr(req.TokenIn),
		TokenOut:  lowerAddr(req.TokenOut),
		QuoteID:   q.QuoteID,
	}
	return quoteDecision{
		response: response,
		outcome:  quoteDecisionQuoted,
		observation: &quoteObservation{
			tokenIn: req.TokenIn, tokenOut: req.TokenOut,
			amountIn: req.Amount, amountOut: out.QuotedAmountOut,
		},
	}, nil
}

// snapshotCandidates reads current on-chain pricing for the request as the rfq.quote.snapshot stage.
func (qs *quoteService) snapshotCandidates(
	ctx context.Context, inv []solverInventory, req strategyRequest,
) (candidates []liquidlane.QuoteCandidate, err error) {
	ctx, end := tracer.Start(ctx, "rfq.quote.snapshot")
	defer func() { end(err) }()
	// Share the execution snapshot lock so no new fill can reserve while these
	// reads are in progress. Capture existing commitments before the RPC: their
	// completion during it must not combine stale capacity with a released ledger.
	if qs.planningMu != nil {
		qs.planningMu.Lock()
		defer qs.planningMu.Unlock()
	}
	var reserved liquidlane.CapacityReservations
	if qs.reservations != nil {
		reserved = qs.reservations.Snapshot()
	}
	candidates, err = qs.reader.readQuoteCandidates(ctx, inv, req.TokenIn, req.TokenOut, req.Amount)
	if err == nil && qs.reservations != nil {
		candidates = candidatesAfterReservations(candidates, reserved)
	}
	return candidates, err
}

// decideQuote runs the strategy as the rfq.quote.decide stage.
func (qs *quoteService) decideQuote(
	ctx context.Context, input types.QuoteInput,
) (out types.QuoteOutput, err error) {
	ctx, end := tracer.Start(ctx, "rfq.quote.decide", observability.AttrStrategy.String(qs.strategyName))
	defer func() { end(err) }()
	return qs.strategy.DecideQuote(ctx, input)
}

// canQuote fails closed when the lane-state dependency was not wired. Production construction
// always supplies the txmanager predicate; keeping the nil case closed prevents a future alternate
// constructor from silently advertising obligations it cannot fill.
func (qs *quoteService) canQuote() bool {
	return qs.laneReady != nil && qs.laneReady()
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }
