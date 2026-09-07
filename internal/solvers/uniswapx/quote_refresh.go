package uniswapx

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type quoteState struct {
	inventory      []liquidlane.Inventory
	gasSnapshot    *liquidlanegas.Snapshot
	gasPrices      *liquidlanegas.PriceSnapshot
	maxFeePerGas   *big.Int
	chainTime      time.Time
	expiresAt      time.Time
	singleRouteFor map[common.Address]bool
}

func (s *Solver) refreshLoop(ctx context.Context, routes []liquidlane.Route) error {
	ticker := time.NewTicker(s.cfg.QuoteServer.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.refreshCh:
		case <-ticker.C:
		}
		if err := s.refreshQuoteState(ctx, routes); err != nil {
			s.log.Error(err, "quote state refresh failed")
		}
	}
}

func (s *Solver) refreshQuoteState(ctx context.Context, routes []liquidlane.Route) error {
	timer := observability.StartOperation(s.operations.quoteRefresh)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	epoch := s.quotes.revision()
	state, complete, err := s.loadQuoteState(ctx, routes)
	if err != nil {
		return err
	}
	if !s.publishQuoteState(epoch, state) {
		outcome = observability.ExternalOperationSkipped
		s.log.V(1).Info("quote state refresh discarded", "epoch", epoch)
		return nil
	}
	outcome = observability.ExternalOperationSuccess
	if !complete {
		outcome = observability.ExternalOperationDegraded
	}
	if s.metrics != nil {
		s.metrics.quoteRefresh.Set(float64(time.Now().Unix()))
	}
	s.log.V(1).Info("quote state refreshed", "epoch", epoch, "inventory", len(state.inventory),
		"gasAccounting", s.cfg.Gas != nil, "maxFeePerGas", state.maxFeePerGas.String(), "expiresAt", state.expiresAt)
	return nil
}

// TTL starts before any read, so slow RPCs cannot make an old snapshot live longer.
func (s *Solver) loadQuoteState(ctx context.Context, routes []liquidlane.Route) (*quoteState, bool, error) {
	expiresAt := time.Now().Add(s.cfg.QuoteServer.QuoteTTL)
	now, err := s.reader.latestBlockTime(ctx)
	if err != nil {
		return nil, false, err
	}
	s.chainTime.Store(now.Unix())
	discountRoutes, discountErr := s.quoteRoutesWithDiscounts(ctx, routes, now)
	if discountErr != nil {
		s.log.Error(discountErr, "refresh advertised discount routes")
	}
	current, err := s.reader.Quote(ctx, discountRoutes.routes, s.cfg.Executor, now)
	if err != nil {
		return nil, false, err
	}
	if s.cfg.usesDiscounts() {
		current.Direct = directInventoriesForAdapters(current.Direct, s.cfg.Adapters)
		if discountRoutes.listed != nil {
			current.Direct = append(current.Direct, s.discountInventories(discountRoutes.listed, current.Physical, now)...)
		}
	}
	maxFee := new(big.Int)
	if s.cfg.Gas != nil {
		maxFee, err = s.txm.MaxFeePerGas(ctx)
		if err != nil {
			return nil, false, err
		}
	}
	return &quoteState{
		inventory: current.Direct, gasSnapshot: current.GasSnapshot, gasPrices: current.GasPrices,
		maxFeePerGas: maxFee, chainTime: now, expiresAt: expiresAt,
		singleRouteFor: s.cfg.TokenPolicy.SingleRouteTokens(),
	}, discountErr == nil && discountRoutes.complete, nil
}

func (s *Solver) publishQuoteState(epoch uint64, state *quoteState) bool {
	q := &s.quotes
	q.mu.Lock()
	defer q.mu.Unlock()
	if !time.Now().Before(state.expiresAt) || q.planning != 0 || q.epoch != epoch {
		return false
	}
	q.snapshot = state
	return true
}

// quotePublication serializes snapshot invalidation, planning and publication.
// RPC and strategy calls run outside the lock; a changed epoch rejects their stale result.
type quotePublication struct {
	mu       sync.Mutex
	snapshot *quoteState
	epoch    uint64
	planning int
}

func (q *quotePublication) current() *quoteState { q.mu.Lock(); defer q.mu.Unlock(); return q.snapshot }
func (q *quotePublication) revision() uint64     { q.mu.Lock(); defer q.mu.Unlock(); return q.epoch }
func (q *quotePublication) planningCount() int   { q.mu.Lock(); defer q.mu.Unlock(); return q.planning }
func (q *quotePublication) changePlanning(delta int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.planning += delta
	if q.planning < 0 {
		panic("uniswapx: negative planning fill count")
	}
	q.epoch++
	q.snapshot = nil
}
