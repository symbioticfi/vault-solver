package uniswapx

import (
	"context"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/trace"

	uxexecutor "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// recipeSettlement fills through connector recipes on the Merkle lane. Quote inventory is read
// from the advertised connector discounts, quote reservations come from the lane's spend ledger,
// and an accepted order is settled by one executor call that draws on one connector or, like
// an adapter fill, on up to types.MaxRoutes of them. The lane owns a single fill: while it is
// busy the next order is deferred and quotes are declined.
type recipeSettlement struct {
	s    *Solver
	lane *filler.Swaps
}

func newRecipeSettlement(s *Solver, cfg filler.Config, deps solver.Deps) (*recipeSettlement, error) {
	provider := timedDiscounts{Provider: s.discounts, timeout: s.cfg.Discounts.HTTPTimeout}
	lane, err := filler.NewSwaps(s.Name(), cfg, provider, deps)
	if err != nil {
		return nil, err
	}
	lane.MinValidity, lane.Decimals = s.cfg.Discounts.MinimumValidity, s.tokenMetadata.Get
	lane.Workflow.Reverts = connector.RevertDecoder(&uxexecutor.LiquidLaneUniswapXExecutorMetaData)
	return &recipeSettlement{s: s, lane: lane}, nil
}

// timedDiscounts bounds each discount request like the native discount client calls.
type timedDiscounts struct {
	liquiddiscounts.Provider

	timeout time.Duration
}

func (d timedDiscounts) ListDiscounts(ctx context.Context) (*liquiddiscounts.List, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	return d.Provider.ListDiscounts(ctx)
}

func (d timedDiscounts) Resolve(ctx context.Context, id string) (*liquiddiscounts.Resolved, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	return d.Provider.Resolve(ctx, id)
}

func (r *recipeSettlement) initialize(ctx context.Context) error { return r.lane.Initialize(ctx) }

func (*recipeSettlement) onMerkleLane() bool { return true }

// The Merkle lane owns one fill at a time, so it is unavailable until that fill settles: an
// order awarded from a quote served meanwhile could miss its exclusivity window. A stalled lane
// keeps its fill, and so stays unavailable until restart.
func (r *recipeSettlement) laneBlockReason() string {
	ready := r.lane.Ready()
	if pending := r.lane.Pending(); pending != nil {
		if r.lane.Stalled() {
			return "recipe lane stalled while filling order " + pending.OrderID
		}
		return "recipe lane is filling order " + pending.OrderID
	}
	if !ready {
		return "recipe transaction manager is not ready"
	}
	return ""
}

func (r *recipeSettlement) reservations(state *quoteState) liquidlane.CapacityReservations {
	return r.lane.Reservations(state.inventory)
}

// Recipe fills hold no native reservation whose release would request the refresh.
func (r *recipeSettlement) fillSettled() { r.s.requestQuoteRefresh() }

// refreshQuoteState publishes connector inventory priced at the connectors' current policy.
func (r *recipeSettlement) refreshQuoteState(ctx context.Context, _ []liquidlane.Route, epoch uint64, now time.Time) (observability.ExternalOperationOutcome, error) {
	s := r.s
	listed, err := s.listDiscounts(ctx)
	if err != nil {
		return observability.ExternalOperationError, err
	}
	// Unpriced offers degrade the refresh; the offers that priced are still published.
	inventory, skipped, err := r.lane.Inventory(ctx, listed.Discounts)
	if err != nil {
		// Keep the last published state, like a failed adapter snapshot.
		return observability.ExternalOperationError, err
	}
	published := observability.ExternalOperationSuccess
	if skipped != nil {
		published = observability.ExternalOperationDegraded
		observability.Log(ctx).V(1).Info("skip unpriced connector discounts", "error", skipped.Error())
	}
	inventory = slices.DeleteFunc(inventory, func(item liquidlane.Inventory) bool { return !s.cfg.TokenPolicy.Allows(item.TokenIn) })
	routes := make([]liquidlane.Route, 0, len(inventory))
	for _, item := range inventory {
		routes = append(routes, item.Route)
	}
	routes = mergeRoutes(routes)
	serverNow := time.Now()
	if !s.publishQuoteState(epoch, &quoteState{
		routes: routes, inventory: inventory, maxFeePerGas: new(big.Int), chainTime: now,
		expiresAt: serverNow.Add(s.cfg.QuoteServer.QuoteTTL), singleRouteFor: s.cfg.TokenPolicy.SingleRouteTokens(),
	}) {
		observability.Log(ctx).V(1).Info("quote state refresh discarded: quote epoch moved during refresh",
			"epoch", epoch, "currentEpoch", s.quoteEpoch.Load())
		return observability.ExternalOperationSkipped, nil
	}
	if s.metrics != nil {
		s.metrics.quoteRefresh.Set(float64(time.Now().Unix()))
	}
	observability.Log(ctx).V(1).Info("quote state refreshed", "epoch", epoch, "routes", len(routes),
		"inventory", len(inventory), "expiresAt", serverNow.Add(s.cfg.QuoteServer.QuoteTTL))
	return published, nil
}

// startFill settles one accepted order through the Merkle lane. Like the adapter fill, the
// uniswapx.fill span outlives this call and ends when the transaction result arrives.
func (r *recipeSettlement) startFill(
	ctx context.Context, _ []liquidlane.Route, order *resolvedOrder, now, _ time.Time,
) (fill *pendingUniswapFill, err error) {
	ctx, end := tracer.Start(ctx, "uniswapx.fill",
		observability.AttrOrderHash.String(order.Hash.Hex()),
		observability.AttrQuoteID.String(order.QuoteID),
	)
	defer func() {
		switch {
		case fill != nil: // the span lives until the transaction result arrives
		case errors.Is(err, errOrderNotFillable):
			end(nil)
		default:
			end(err)
		}
	}()
	if order.TokenOut == (common.Address{}) {
		return nil, declineFill(ctx, "fill_skipped", "order has no output token")
	}
	if order.Deadline == 0 || int64(order.Deadline) <= now.Unix() {
		return nil, declineFill(ctx, "fill_skipped", "order deadline has passed")
	}
	if order.Executor != r.s.cfg.Executor {
		return nil, declineFill(ctx, "fill_skipped", "order executor differs from the recipe filler")
	}
	swap := recipeSwap(order)
	swap.MaxRoutes = r.maxRoutes(order.TokenIn)
	intent, err := r.lane.Fill(ctx, swap, recipeEnvelope(order))
	switch {
	case intent != nil && err != nil:
		// An ambiguous submission keeps the lane; its receipt still decides the order.
		observability.Log(ctx).Error(err, "order fill submission uncertain", "orderHash", order.Hash.Hex())
	case errors.Is(err, filler.ErrBusy), errors.Is(err, filler.ErrNotFillable):
		return nil, declineFill(ctx, "fill_declined", err.Error())
	case errors.Is(err, filler.ErrSimulation), errors.Is(err, filler.ErrSubmission):
		// Like a native preflight or pre-sign failure: back off and count toward the breaker.
		return nil, errors.Errorf("%w: %w", errFillPreflight, err)
	case err != nil:
		return nil, err
	case intent == nil:
		return nil, declineFill(ctx, "fill_skipped", "order was already settled by this lane")
	}
	observability.Log(ctx).V(1).Info("order fill submitted", "source", order.Source, "orderHash", order.Hash.Hex(),
		"quoteId", order.QuoteID, "executor", order.Executor.Hex(), "payment", intent.Spend.Total().String(),
		"submissionDeadline", intent.Deadline.Unix())
	result := make(chan txmanager.Result, 1)
	// The native result shape keeps retry, breaker and exclusivity accounting shared with adapter fills.
	go func() {
		settled := r.lane.AwaitNative(ctx)
		// Adapter fills count their success from the native manager's receipt; this lane has none.
		if settled.Outcome == txmanager.OutcomeConfirmed {
			r.s.observeFillOutcome(liquidlane.FillOutcomeSuccess)
			if r.s.metrics != nil {
				r.s.metrics.pairs.MinedFill(r.s.chainID, order.TokenIn, order.TokenOut)
			}
		}
		result <- settled
	}()
	return &pendingUniswapFill{order: order, result: result, span: trace.SpanFromContext(ctx), end: end}, nil
}

// maxRoutes is how many connectors one fill may draw on, by the same rules as quotes: the single
// strategy and single-route tokens settle through one.
func (r *recipeSettlement) maxRoutes(tokenIn common.Address) int {
	if r.s.cfg.singleSource() || r.s.cfg.TokenPolicy.RequiresSingleRoute(tokenIn) {
		return 1
	}
	return types.MaxRoutes
}

// recipeSwap is the order as the lane sees it: the input the executor receives in the Reactor
// callback and the output it must hold when the callback returns.
func recipeSwap(order *resolvedOrder) filler.Swap {
	return filler.Swap{
		OrderID: order.Hash.Hex(), TokenIn: order.TokenIn, AmountIn: order.AmountIn,
		TokenOut: order.TokenOut, MinAmountOut: order.AmountOut,
		Expiry: time.Unix(int64(min(uint64(order.Deadline), 1<<62)), 0),
	}
}

// recipeEnvelope wraps the callback calls in LiquidLaneUniswapXExecutor.execute. Inside the
// Reactor callback the executor forwards the swapper's input to the connector and calls the
// recipe; the Reactor then settles the order outputs from the executor's balance.
func recipeEnvelope(order *resolvedOrder) filler.Envelope {
	return func(calls []execution.Call) ([]byte, error) {
		callback := make([]uxexecutor.IExecutorCall, len(calls))
		for i, call := range calls {
			callback[i] = uxexecutor.IExecutorCall(call)
		}
		return uniswapXExecutor.TryPackExecute(uxexecutor.UniswapXSignedOrder{Order: order.Encoded, Sig: order.Signature}, callback)
	}
}
