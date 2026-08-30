// Package redstoneoev implements the RedStone Atom OEV solver: it subscribes to OEV auctions over
// WebSocket, delegates bid/skip decisions to a configured strategy, signs EXECUTOR_V6 bids, and replies
// with solve payloads that settle through strategy-selected callback operationData. The built-in default
// strategy is the Morpho/LiquidLane liquidation path. The solver registers itself via init(). See
// docs/OEV-PLAN.md.
package redstoneoev

import (
	"math/big"
	"sync"

	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

// Name is the registry key that selects this solver from config.
const Name = "redstone-oev"

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, solver.Registration{
		Factory: factory, ValidateConfig: validateConfig, ExternallySubmitted: true,
	})
}

// Solver is the RedStone OEV solver runtime.
type Solver struct {
	cfg      *Config
	deps     solver.Deps
	chainID  *big.Int
	reader   *reader
	strategy types.Strategy
	nonces   *nonceStore
	breaker  *breaker
	metrics  *metrics
	ws       *wsClient
	seen     *seenAuctions // de-dup of already-processed auction ids, touched before bid dispatch
	log      logr.Logger

	state stateCache // cached executor accounting, refreshed by the ops loop
	// stateRefreshCh coalesces event-driven refresh requests without blocking the WS read loop on RPC.
	stateRefreshCh chan struct{}

	// resMu guards sent-but-unresolved bids. pruneReservations frees a bid once it RESOLVES: its nonce fell
	// below the on-chain nonce (submitted -> settled or reverted; the fresh read reflects it) or it aged past
	// reservationTTL as a last-resort cleanup for missed result frames.
	resMu sync.Mutex
	res   []reservedBid

	// bidMu keeps bid decisions ordered while auction frames are dispatched off the WS read loop. This
	// preserves the pending-auction snapshot semantics strategies use to avoid overlapping bids.
	bidMu sync.Mutex
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }
