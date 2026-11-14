package rfq

import (
	"context"
	"math/big"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// quoteService prices backend RFQ requests by handing filtered candidates to the strategy. It is safe
// for concurrent use (the HTTP server serves quotes in parallel): its dependencies are individually
// synchronized, and its optional configured route slice is published atomically once.
type quoteService struct {
	recipeExecutors []common.Address
	acceptsRoute    func(executor, connector, account, asset common.Address) bool
	resolveRoute    func(connector, rwa, asset common.Address, assetDecimals int) (executor, account common.Address, ok bool)
	pairRoutes      atomic.Pointer[[]liquidlane.Route] // immutable configured adapter directions, optional background observation only
	chainID         int64
	tokenMetadata   *chain.Decimals
	executor        common.Address
	// laneAvailable is safe for concurrent use and reports sender initialization. A busy lane does not
	// block quoting: pending fills are accounted through reservations instead. It is sampled before
	// and after quote planning so work is declined whenever either check observes a nonce conflict.
	laneAvailable func() bool
	// reservations returns the liquidity held by won, unfinished orders against a resolved
	// inventory; nil means none.
	reservations     func(excludedOrderID string, inventory []solverInventory) liquidlane.CapacityReservations
	acceptsConnector func(common.Address) bool // optional published recipe connectors; may change after start
	whitelist        adapterWhitelist          // nil disables adapter filtering
	tokenPolicy      tokenpolicy.Policy
	// minAmountsIn holds per-input-token minimum request sizes in base units; a token absent from the
	// map (or a nil map) has no minimum.
	discountsEnabled bool
	minAmountsIn     map[common.Address]*big.Int
	strategy         types.Strategy
	strategyName     string // registry key, reported as the strategy.name span attribute
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
		pending pendingReservations,
	) ([]liquidlane.QuoteCandidate, error)
}

// pendingReservations returns the capacity won orders hold against the given resolved inventory.
// It is called once the inventory carries capacity IDs, so a reservation can be compared with the
// block each snapshot was read at. nil means nothing is reserved.
type pendingReservations func(inventory []solverInventory) liquidlane.CapacityReservations

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
	response          *quoteResponse
	outcome           quoteDecisionOutcome
	pairInScope       bool
	metricPairInScope bool
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
func (qs *quoteService) evaluate(ctx context.Context, q *quoteRequest) (decision quoteDecision, err error) {
	knownDirection := false
	configuredDirection := false
	defer func() {
		decision.pairInScope = knownDirection
		decision.metricPairInScope = knownDirection || configuredDirection
	}()
	// A recipe process serves only its committed payment connector. Filter before
	// parsing so unrelated native entries in a shared backend response stay out.
	if len(qs.recipeExecutors) > 0 && qs.acceptsConnector != nil {
		scoped := *q
		scoped.Adapters = nil
		for _, item := range q.Adapters {
			if qs.acceptsConnector(common.HexToAddress(item.Adapter)) {
				scoped.Adapters = append(scoped.Adapters, item)
			}
		}
		q = &scoped
	}
	if qs.resolveRoute != nil {
		q, err = qs.resolveInventory(q)
		if err != nil {
			return quoteDecision{outcome: quoteDecisionError}, &badRequestError{err}
		}
	}
	parsed, err := q.toStrategy(qs.chainID, qs.recipeExecutors)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	configuredDirection = qs.configuredPairInScope(q)
	if !qs.canQuote() {
		observability.Log(ctx).V(1).Info("declining quote: transaction nonce lane unavailable", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionLaneUnavailable}, nil
	}
	if parsed == nil {
		observability.Log(ctx).V(1).Info("declining quote: not quotable", "quoteId", q.QuoteID, "type", q.Type)
		return quoteDecision{outcome: quoteDecisionNotQuotable}, nil
	}
	if !qs.tokenPolicy.Allows(parsed.req.TokenIn) {
		observability.Log(ctx).V(1).Info("declining quote: input token out of scope",
			"quoteId", q.QuoteID, "scope", qs.tokenPolicy.Scope())
		return quoteDecision{outcome: quoteDecisionNotQuotable}, nil
	}
	if minIn, ok := qs.minAmountsIn[parsed.req.TokenIn]; ok && parsed.req.Amount.Cmp(minIn) < 0 {
		observability.Log(ctx).V(1).Info("declining quote: input amount below configured minimum",
			"quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionBelowMinimum}, nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if qs.acceptsRoute != nil {
		inv = slices.DeleteFunc(slices.Clone(inv), func(item solverInventory) bool {
			return !qs.acceptsRoute(item.RecipeExecutor, item.Adapter, item.Vault, item.TokenOut)
		})
	}
	if len(qs.recipeExecutors) > 0 {
		inv = bestRouteInventory(inv)
	}
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
	// Cached route validation and the authenticated snapshot establish a known inventory pair.
	// This is not a promise that the requested amount/strategy can be quoted.
	knownDirection = true
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
		observability.Log(ctx).V(1).Info("declining quote: transaction nonce lane no longer available", "quoteId", q.QuoteID)
		return quoteDecision{outcome: quoteDecisionLaneUnavailable}, nil
	}

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
	}, nil
}

// resolveInventory fills route identities from the immutable local cache without
// changing the caller's request or the backend's capacity bound.
func (qs *quoteService) resolveInventory(q *quoteRequest) (*quoteRequest, error) {
	scoped := *q
	scoped.Adapters = make([]quoteAdapter, 0, len(q.Adapters))
	rwa, err := parse.Address(q.TokenIn, "tokenIn")
	if err != nil {
		return nil, err
	}
	for i, item := range q.Adapters {
		venue, err := parse.Address(item.Adapter, idxField(i, "adapter"))
		if err != nil {
			return nil, err
		}
		asset, err := parse.Address(item.Asset, idxField(i, "asset"))
		if err != nil {
			return nil, err
		}
		executor, account, ok := qs.resolveRoute(venue, rwa, asset, item.AssetDecimals)
		if !ok {
			continue
		}
		item.Account = account.Hex()
		item.RecipeExecutor = ""
		if executor != (common.Address{}) {
			item.RecipeExecutor = executor.Hex()
		}
		scoped.Adapters = append(scoped.Adapters, item)
	}
	return &scoped, nil
}

// snapshotCandidates prices the backend snapshot and subtracts local reservations without I/O.
func (qs *quoteService) snapshotCandidates(
	ctx context.Context, inv []solverInventory, req strategyRequest,
) (candidates []liquidlane.QuoteCandidate, err error) {
	_, end := tracer.Start(ctx, "rfq.quote.snapshot")
	defer func() { end(err) }()
	inv = slices.DeleteFunc(slices.Clone(inv), func(item solverInventory) bool {
		return item.TokenIn != req.TokenIn || item.TokenOut != req.TokenOut || item.Vault == (common.Address{})
	})
	var reservations liquidlane.CapacityReservations
	if qs.reservations != nil {
		reservations = qs.reservations("", inv)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(inv, reservations, 0)
	for _, item := range allocated {
		if candidate := liquidgreedy.NewQuoteCandidate(item, item.MaxAssets); candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}
	return candidates, nil
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
	return qs.laneAvailable != nil && qs.laneAvailable()
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }

// Recipes funding the same swap are alternatives, not additional liquidity.
// Keep the largest eligible snapshot for each physical route and discount.
func bestRouteInventory(inventory []solverInventory) []solverInventory {
	out := make([]solverInventory, 0, len(inventory))
	positions := make(map[liquidlane.CandidateID]int)
	for _, item := range inventory {
		id := liquidlane.NewCandidateID(item.Route, item.DiscountID)
		if index, found := positions[id]; found {
			if item.MaxAssets.Cmp(out[index].MaxAssets) > 0 {
				out[index] = item
			}
			continue
		}
		positions[id] = len(out)
		out = append(out, item)
	}
	return out
}
