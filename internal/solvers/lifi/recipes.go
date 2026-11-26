package lifi

import (
	"context"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	lifiexecutor "github.com/symbioticfi/vault-solver/api/bindings/lifi/executor"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	// recipeMinValidity is the lifetime an order and its signed discount must still have when a
	// recipe fill is submitted: one block, like the default executionDeadlineBuffer.
	recipeMinValidity = 12 * time.Second
	// maxRevertRetries bounds how often one order is planned again after its fill reverted, so a
	// fill that keeps passing simulation and reverting on chain cannot spend gas until the deadline.
	maxRevertRetries = 2
)

// recipeSettlement fills through connector recipes on the Merkle lane. Quote inventory is read
// from the advertised connector discounts, quote reservations come from the lane's spend ledger,
// and an opened order is settled by one executor call that draws on one connector or, like an
// adapter fill, on up to types.MaxRoutes of them. The lane owns a single fill, so an order
// arriving while it is busy waits for the pending fill like an order blocked on reserved capacity.
type recipeSettlement struct {
	s    *Solver
	lane *filler.Swaps
}

func newRecipeSettlement(s *Solver, cfg filler.Config, deps solver.Deps) (*recipeSettlement, error) {
	lane, err := filler.NewSwaps(s.Name(), cfg, s.discounts, deps)
	if err != nil {
		return nil, err
	}
	lane.MinValidity, lane.Decimals = recipeMinValidity, s.tokenMetadata.Get
	return &recipeSettlement{s: s, lane: lane}, nil
}

// recipeLane gates quotes on the SDK transaction manager. It has no change feed: the quote
// loop observes readiness on its own refreshes.
type recipeLane struct{ lane *filler.Swaps }

func (l recipeLane) LaneReady() bool                             { return l.lane.Ready() && !l.lane.Stalled() }
func (recipeLane) SubscribeLaneState() (<-chan struct{}, func()) { return nil, func() {} }

func (r *recipeSettlement) initialize(ctx context.Context) error { return r.lane.Initialize(ctx) }

func (*recipeSettlement) onMerkleLane() bool { return true }

// A recipe pays the exact amount priced at its simulation block, so a state change before
// inclusion (an oracle update, a policy change, another fill spending the same funding) reverts
// the fill while the order stays open. Such an order is planned again from fresh state.
func (*recipeSettlement) retryReverted(order *submittedOrder) bool {
	order.reverts++
	return order.reverts <= maxRevertRetries
}

func (r *recipeSettlement) reservations(inventory []liquidlane.Inventory) liquidlane.CapacityReservations {
	return r.lane.Reservations(inventory)
}

// quoteSource prices the advertised connector discounts at their connectors' current policy.
func (r *recipeSettlement) quoteSource(ctx context.Context, _ []route, _ time.Time) (quoteSource, error) {
	s := r.s
	listed, err := s.discounts.ListDiscounts(ctx)
	if err != nil {
		observability.ErrorUnlessCanceled(ctx, err, "connector discounts: list for quote")
		return quoteSource{degraded: true, discountErr: err}, nil
	}
	inventory, skipped, err := r.lane.Inventory(ctx, listed.Discounts)
	if err != nil {
		// Keep the published quotes, like a failed adapter snapshot.
		return quoteSource{}, err
	}
	if skipped != nil && ctx.Err() == nil {
		observability.Log(ctx).V(1).Info("connector discounts: skip unpriced offers", "error", skipped.Error())
	}
	inventory = slices.DeleteFunc(inventory, func(item liquidlane.Inventory) bool { return !s.cfg.TokenPolicy.Allows(item.TokenIn) })
	return quoteSource{inventory: inventory, singleRouteTokens: s.cfg.TokenPolicy.SingleRouteTokens(), degraded: skipped != nil}, nil
}

// recipeSwap is the opened order as the lane sees it: the input the executor receives when it
// finalises the order and the output it must hold to fill the output settler.
func recipeSwap(order *submittedOrder, orderID common.Hash, deadline time.Time) filler.Swap {
	return filler.Swap{
		OrderID: orderID.Hex(), TokenIn: order.TokenIn, AmountIn: order.AmountIn,
		TokenOut: order.TokenOut, MinAmountOut: order.OutputAmount, Expiry: deadline,
	}
}

// recipeEnvelope wraps the callback calls in LiquidLaneLifiExecutor.finaliseWithCurrentTimestamp:
// the executor claims the order input, forwards it to the connector and calls the recipe, then
// fills the output settler from the payment the connector sent back.
func recipeEnvelope(order *submittedOrder) filler.Envelope {
	return func(calls []execution.Call) ([]byte, error) {
		callback := make([]lifiexecutor.IExecutorCall, len(calls))
		for i, call := range calls {
			callback[i] = lifiexecutor.IExecutorCall(call)
		}
		return lifiExecutor.TryPackFinaliseWithCurrentTimestamp(toExecutorOrder(order.Order), callback)
	}
}

// fillOrder settles an opened order through the Merkle lane and reports it in the worker's
// existing result shape.
func (r *recipeSettlement) fillOrder(ctx context.Context, _ []route, order *submittedOrder, orderID common.Hash, _ *liquidlane.CapacityReservations) orderProcessingResult {
	s := r.s
	if pending := r.lane.Pending(); pending != nil {
		reason := "recipe lane owns the fill of order " + pending.OrderID
		observability.Decline(ctx, "fill_deferred", reason)
		blockedOn := make(map[liquidlane.CapacityID]bool, len(pending.Spend.Amounts))
		for capacityID := range pending.Spend.Amounts {
			blockedOn[capacityID] = true
		}
		return orderProcessingResult{blockedOn: blockedOn, capacityReason: reason, outcome: orderProcessingCapacityDeferred}
	}
	chainObservedAt := s.wallNow()
	chainTime, err := s.now(ctx)
	if err != nil {
		return orderProcessingResult{retryable: true, outcome: orderProcessingRetryableError, err: err}
	}
	deadline := orderDeadline(order)
	if deadline.IsZero() {
		observability.Decline(ctx, "fill_skipped", "order has no fill deadline")
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	swap := recipeSwap(order, orderID, deadline)
	if !s.cfg.TokenPolicy.RequiresSingleRoute(order.TokenIn) {
		swap.MaxRoutes = types.MaxRoutes
	}
	intent, err := r.lane.Fill(ctx, swap, recipeEnvelope(order))
	switch {
	case intent != nil && err != nil:
		// An ambiguous submission keeps the lane; its receipt still decides the order.
		observability.Log(ctx).Error(err, "order fill submission uncertain", "orderId", order.OrderID, "onChainOrderId", orderID.Hex())
	case errors.Is(err, filler.ErrTermsExpiring):
		// The order outlives its signed discount: fresh terms may settle it, so retry shortly.
		observability.Log(ctx).V(1).Info("order fill deferred: signed discount expiring", "orderId", order.OrderID,
			"onChainOrderId", orderID.Hex(), "quoteId", order.QuoteID, "reason", err.Error())
		return orderProcessingResult{retryable: true, retrySoon: true, outcome: orderProcessingRetryableError, err: err}
	case errors.Is(err, filler.ErrBusy), errors.Is(err, filler.ErrNotFillable):
		observability.Decline(ctx, "fill_declined", err.Error())
		observability.Log(ctx).V(1).Info("order fill declined", "orderId", order.OrderID, "onChainOrderId", orderID.Hex(),
			"quoteId", order.QuoteID, "reason", err.Error())
		return orderProcessingResult{outcome: orderProcessingStrategyDeclined}
	case err != nil:
		// A failed read, an unreachable curator or discount service, a rejected simulation: retry
		// shortly instead of waiting for the next feed recovery. Only giving up is an error.
		observability.Log(ctx).V(1).Info("order fill: prepare recipe settlement failed; retrying", "orderId", order.OrderID,
			"quoteId", order.QuoteID, "error", err.Error())
		return orderProcessingResult{retryable: true, retrySoon: true, outcome: orderProcessingRetryableError, err: err}
	case intent == nil:
		observability.Decline(ctx, "order_skipped", "order was already settled by this lane")
		return orderProcessingResult{outcome: orderProcessingNotActionable}
	}
	observability.Log(ctx).V(1).Info("order fill submitted", "orderId", order.OrderID, "onChainOrderId", orderID.Hex(),
		"quoteId", order.QuoteID, "payment", intent.Spend.Total().String(), "submissionDeadline", intent.Deadline.Unix())
	retryDeadline, _ := liquidlane.SubmissionDeadline(deadline, chainTime, chainObservedAt, s.wallNow())
	result := make(chan txmanager.Result, 1)
	// The native result shape keeps retry and outcome accounting shared with adapter fills.
	go func() {
		settled := r.lane.AwaitNative(ctx)
		// Adapter fills count their success from the native manager's receipt; this lane has none.
		if settled.Outcome == txmanager.OutcomeConfirmed && s.metrics != nil {
			s.metrics.fillAmounts.ObserveOutcome(liquidlane.FillOutcomeSuccess)
			s.metrics.pairs.MinedFill(s.chainID, order.TokenIn, order.TokenOut)
		}
		result <- settled
		s.requestQuoteRefresh()
	}()
	return orderProcessingResult{
		fill:    &pendingFill{order: order, orderID: orderID, reservationKey: orderID.Hex(), retryDeadline: retryDeadline, result: result},
		outcome: orderProcessingSubmitted,
	}
}
