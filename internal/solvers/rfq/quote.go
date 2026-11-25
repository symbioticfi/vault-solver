package rfq

import (
	"context"
	"fmt"
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
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// quoteRouteResolver returns only verified local identities; an unresolved or
// ambiguous route declines without I/O.
type quoteRouteResolver func(venue, rwa, asset common.Address, assetDecimals int) (executor, account common.Address, ok bool)

// quoteService prices backend RFQ requests by handing filtered candidates to the strategy. It is safe
// for concurrent use (the HTTP server serves quotes in parallel): its dependencies are individually
// synchronized, and its optional configured route slice is published atomically once.
type quoteService struct {
	resolveRoute  quoteRouteResolver
	pairRoutes    atomic.Pointer[[]liquidlane.Route] // immutable configured adapter directions, optional background observation only
	chainID       int64
	tokenMetadata *chain.Decimals
	executor      common.Address
	// laneAvailable is safe for concurrent use and reports execution readiness before and after
	// quote planning. Reservations account for liquidity held by outstanding fills.
	laneAvailable       func() bool
	requireFullCoverage bool // Merkle recipes settle the exact discounted payout on each leg.
	// reservations returns the liquidity held by won, unfinished orders against a resolved
	// inventory; nil means none.
	reservations func(excludedOrderID string, inventory []solverInventory) liquidlane.CapacityReservations
	whitelist    adapterWhitelist // nil disables adapter filtering
	tokenPolicy  tokenpolicy.Policy
	// minAmountsIn holds per-input-token minimum request sizes in base units; a token absent from the
	// map (or a nil map) has no minimum.
	discountsEnabled bool
	minAmountsIn     map[common.Address]*big.Int
	strategy         types.Strategy
	strategyName     string // registry key, reported as the strategy.name span attribute
	log              logr.Logger
	now              func() time.Time
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
	reason            string // the failed check of a decline, with its values
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
				reason := decision.reason
				if reason == "" {
					reason = string(decision.outcome)
				}
				observability.Decline(ctx, "no_quote", reason)
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
	parsed, err := q.toStrategy(qs.chainID, qs.resolveRoute)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, &badRequestError{errors.Errorf("parse request: %w", err)}
	}
	configuredDirection = qs.configuredPairInScope(q)
	if !qs.canQuote() {
		return declineQuote(ctx, q.QuoteID, quoteDecisionLaneUnavailable, "execution lane unavailable",
			"transaction nonce lane is not ready"), nil
	}
	if parsed.declined != "" {
		return declineQuote(ctx, q.QuoteID, quoteDecisionNotQuotable, "not quotable", parsed.declined), nil
	}
	if !qs.tokenPolicy.Allows(parsed.req.TokenIn) {
		return declineQuote(ctx, q.QuoteID, quoteDecisionNotQuotable, "input token out of scope",
			"tokenIn is outside token scope "+string(qs.tokenPolicy.Scope())), nil
	}
	if minIn, ok := qs.minAmountsIn[parsed.req.TokenIn]; ok && parsed.req.Amount.Cmp(minIn) < 0 {
		return declineQuote(ctx, q.QuoteID, quoteDecisionBelowMinimum, "input amount below configured minimum",
			fmt.Sprintf("amount in %s is below configured minimum %s", parsed.req.Amount, minIn)), nil
	}
	req, inv := parsed.req, qs.whitelist.filter(parsed.inv)
	if len(inv) == 0 {
		return declineQuote(ctx, q.QuoteID, quoteDecisionNoCandidates, "no whitelisted adapters",
			fmt.Sprintf("none of %d resolved inventories uses a whitelisted adapter", len(parsed.inv))), nil
	}
	inv = bestRouteInventory(inv)
	if !qs.discountsEnabled {
		direct := slices.DeleteFunc(slices.Clone(inv), func(item solverInventory) bool { return item.DiscountID != nil })
		if len(direct) == 0 {
			return declineQuote(ctx, q.QuoteID, quoteDecisionNoCandidates, "discounts disabled",
				fmt.Sprintf("all %d whitelisted inventories are discounts and discounts are disabled", len(inv))), nil
		}
		inv = direct
	}

	requireSingleRoute := qs.tokenPolicy.RequiresSingleRoute(req.TokenIn)
	drops := newInventoryDrops(ctx)
	candidates := qs.snapshotCandidates(ctx, inv, req, drops.trace)
	if len(candidates) == 0 {
		return declineQuote(ctx, q.QuoteID, quoteDecisionNoCandidates, "no viable LiquidLane candidates",
			fmt.Sprintf("no candidates from %d inventories, dropped: %s", len(inv), drops)), nil
	}
	// Cached route validation and the authenticated snapshot establish a known inventory pair.
	// This is not a promise that the requested amount/strategy can be quoted.
	knownDirection = true
	input := newQuoteInput(qs.chainID, qs.executor, req, candidates, nil, requireSingleRoute, qs.now())
	input.RequireFullCoverage = qs.requireFullCoverage
	out, err := qs.decideQuote(ctx, input)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, errors.Errorf("quote: strategy: %w", err)
	}
	if out.Decision != types.DecisionQuote {
		observability.Log(ctx).Info("declining quote: strategy declined", "quoteId", q.QuoteID, "reason", out.Reason)
		return quoteDecision{outcome: quoteDecisionStrategyDeclined, reason: out.Reason}, nil
	}
	plan, err := strategies.FillPlanFromQuote(input, out)
	if err != nil {
		return quoteDecision{outcome: quoteDecisionError}, errors.Errorf("quote: strategy: %w", err)
	}
	traceAdapter(ctx, plan.Legs)
	if !qs.canQuote() {
		return declineQuote(ctx, q.QuoteID, quoteDecisionLaneUnavailable, "transaction nonce lane no longer available",
			"transaction nonce lane stopped being ready during planning"), nil
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

// declineQuote logs a quote decline at V(1) with the failed check, which it also keeps for the span.
func declineQuote(ctx context.Context, quoteID string, outcome quoteDecisionOutcome, message, reason string) quoteDecision {
	observability.Log(ctx).V(1).Info("declining quote: "+message, "quoteId", quoteID, "reason", reason)
	return quoteDecision{outcome: outcome, reason: reason}
}

// snapshotCandidates prices the backend snapshot and subtracts local reservations without I/O.
// Each dropped inventory item is declined on trace with the failed check.
func (qs *quoteService) snapshotCandidates(
	ctx context.Context, inv []solverInventory, req strategyRequest, trace liquidstrategies.DecisionTrace,
) (candidates []liquidlane.QuoteCandidate) {
	_, end := tracer.Start(ctx, "rfq.quote.snapshot")
	defer end(nil)
	matching := make([]solverInventory, 0, len(inv))
	for _, item := range inv {
		switch {
		// Only the configured adapter is named: the pair is unproven request data until it matches.
		case item.TokenIn != req.TokenIn || item.TokenOut != req.TokenOut:
			trace.Decline("inventory", "inventory pair is not the order pair", "adapter", item.Adapter)
		case item.Vault == (common.Address{}):
			trace.Decline("inventory", "inventory has no resolved vault", "adapter", item.Adapter)
		default:
			matching = append(matching, item)
		}
	}
	var reservations liquidlane.CapacityReservations
	if qs.reservations != nil {
		reservations = qs.reservations("", matching)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(matching, reservations, 0)
	declineUnallocated(trace, matching, allocated, reservations)
	for _, item := range allocated {
		candidate := liquidgreedy.NewQuoteCandidate(item, item.MaxAssets)
		if candidate == nil {
			trace.Decline("inventory", "candidate prices no output", "route", item.ID,
				"maxAssets", item.MaxAssets, "rate", item.MaxRate, "price", item.Price, "discount", item.Discount)
			continue
		}
		candidates = append(candidates, *candidate)
	}
	return candidates
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
// supplies a readiness predicate; keeping the nil case closed prevents a future alternate
// constructor from silently advertising obligations it cannot fill.
func (qs *quoteService) canQuote() bool {
	return qs.laneAvailable != nil && qs.laneAvailable()
}

// lowerAddr renders an address as lowercase hex; RFQ backend payloads use lowercase addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }

// Repeated snapshots of one swap are alternatives, not additional liquidity.
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
