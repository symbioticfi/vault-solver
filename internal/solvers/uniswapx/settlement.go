package uniswapx

import (
	"context"
	"time"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// settlement is the fill path behind UniswapX quoting and order intake. Every solver built on
// this engine has exactly one: native adapters on the host transaction manager
// (uniswapx-filler) or connector recipes on the Merkle lane (uniswapx-merkle).
type settlement interface {
	// initialize prepares the startup state the fill path needs before quotes are served.
	initialize(ctx context.Context) error
	// onMerkleLane reports whether fills are signed by the SDK transaction manager.
	onMerkleLane() bool
	// laneBlockReason names what prevents quoting or filling, or returns empty when work is allowed.
	laneBlockReason() string
	// reservations is the capacity already committed to fills, which quotes must not reuse.
	reservations(state *quoteState) liquidlane.CapacityReservations
	// refreshQuoteState publishes the inventory quotes are priced from, observed at chain time now.
	refreshQuoteState(ctx context.Context, routes []liquidlane.Route, epoch uint64, now time.Time) (observability.ExternalOperationOutcome, error)
	// startFill submits one accepted order; its transaction result arrives on the returned fill.
	startFill(ctx context.Context, routes []liquidlane.Route, order *resolvedOrder, now, chainObservedAt time.Time) (*pendingUniswapFill, error)
	// fillSettled runs once a fill's transaction result has been accounted.
	fillSettled()
}

// fillPath returns the solver's settlement; an engine built without one settles through adapters.
func (s *Solver) fillPath() settlement {
	if s.settlement != nil {
		return s.settlement
	}
	return adapterSettlement{s}
}

// adapterSettlement fills through native LiquidLane adapters on the host transaction manager.
type adapterSettlement struct{ s *Solver }

func (adapterSettlement) initialize(context.Context) error { return nil }
func (adapterSettlement) onMerkleLane() bool               { return false }
func (adapterSettlement) fillSettled()                     {}

func (a adapterSettlement) laneBlockReason() string {
	if a.s.txm != nil && !a.s.txm.Available() {
		return "transaction nonce lane is paused"
	}
	return ""
}

func (a adapterSettlement) reservations(*quoteState) liquidlane.CapacityReservations {
	return a.s.capacity.Snapshot()
}

func (a adapterSettlement) refreshQuoteState(ctx context.Context, routes []liquidlane.Route, epoch uint64, now time.Time) (observability.ExternalOperationOutcome, error) {
	return a.s.refreshAdapterQuoteState(ctx, routes, epoch, now)
}

func (a adapterSettlement) startFill(ctx context.Context, routes []liquidlane.Route, order *resolvedOrder, now, chainObservedAt time.Time) (*pendingUniswapFill, error) {
	return a.s.startFill(ctx, routes, order, now, chainObservedAt)
}
