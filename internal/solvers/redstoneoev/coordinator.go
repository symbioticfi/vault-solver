// Package redstoneoev implements the RedStone Atom OEV auction coordinator.
package redstoneoev

import (
	"context"
	"math/big"
	"sync"
	"time"

	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
)

const Name = "redstone-oev"

type headReader interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*gethtypes.Header, error)
}

// Solver coordinates WebSocket intake with one ordered bid lane and an atomically refreshed fact cache.
type Solver struct {
	cfg     *Config
	chainID *big.Int
	chain   headReader
	signer  signer.Signer
	reader  stateReader
	planner decision.Planner
	facts   decision.FactSource
	nonces  *nonceStore
	breaker *breaker
	metrics *metrics
	ws      *wsClient
	seen    *seenAuctions
	log     logr.Logger

	state          stateCache
	stateRefreshCh chan struct{}

	exposures exposureBook

	auctionWG sync.WaitGroup
	bidMu     sync.Mutex
}

func (solver *Solver) Name() string { return Name }

func (solver *Solver) updateWinMetrics() {
	if solver.metrics != nil {
		count, oldest := solver.exposures.wonMetrics(time.Now())
		solver.metrics.updateWins(count, oldest)
	}
}
