package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type quoteState struct {
	epoch          uint64
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
			if err := s.refreshQuoteState(ctx, routes); err != nil {
				s.log.Error(err, "requested quote state refresh failed")
			}
		case <-ticker.C:
			if err := s.refreshQuoteState(ctx, routes); err != nil {
				s.log.Error(err, "quote state refresh failed")
			}
		}
	}
}

func (s *Solver) refreshQuoteState(ctx context.Context, routes []liquidlane.Route) (err error) {
	timer := observability.StartOperation(s.metrics.operation(quoteRefreshOperation))
	defer func() { timer.Finish(ctx, observability.OutcomeForError(err)) }()
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	epoch := s.quoteEpoch.Load()
	now, err := s.reader.latestBlockTime(ctx)
	if err != nil {
		return err
	}
	s.chainTime.Store(now.Unix())
	decisionRoutes, listed, discountErr := s.quoteRoutesWithDiscounts(ctx, routes, now)
	if discountErr != nil {
		s.log.Error(discountErr, "refresh advertised discount routes")
	}
	current, err := s.reader.Quote(ctx, decisionRoutes, s.cfg.Executor, now)
	if err != nil {
		return err
	}
	if s.cfg.usesDiscounts() {
		current.Direct = directInventoriesForAdapters(current.Direct, s.cfg.Adapters)
		if listed != nil {
			current.Direct = append(current.Direct, s.discountInventories(listed, current.Physical, now)...)
		}
	}
	maxFee := new(big.Int)
	if s.cfg.Gas != nil {
		maxFee, err = s.txm.MaxFeePerGas(ctx)
		if err != nil {
			return err
		}
	}
	serverNow := time.Now()
	if s.publishQuoteState(epoch, &quoteState{
		inventory:   current.Direct,
		gasSnapshot: current.GasSnapshot, gasPrices: current.GasPrices,
		maxFeePerGas: maxFee, chainTime: now, expiresAt: serverNow.Add(s.cfg.QuoteServer.QuoteTTL),
		singleRouteFor: s.cfg.TokenPolicy.SingleRouteTokens(),
	}) {
		s.metrics.quoteRefreshed(time.Now())
		s.log.V(1).Info(
			"quote state refreshed",
			"epoch", epoch,
			"routes", len(decisionRoutes),
			"inventory", len(current.Direct),
			"physicalInventory", len(current.Physical),
			"gasAccounting", s.cfg.Gas != nil,
			"maxFeePerGas", maxFee.String(),
			"expiresAt", serverNow.Add(s.cfg.QuoteServer.QuoteTTL),
		)
	} else {
		s.log.V(1).Info("quote state refresh discarded", "epoch", epoch)
	}
	return nil
}

func (q *quoteRuntime) publishQuoteState(epoch uint64, state *quoteState) bool {
	if q.planningFills.Load() != 0 || q.quoteEpoch.Load() != epoch {
		return false
	}
	state.epoch = epoch
	q.quoteState.Store(state)
	if q.planningFills.Load() != 0 || q.quoteEpoch.Load() != epoch {
		q.quoteState.CompareAndSwap(state, nil)
		return false
	}
	return true
}
