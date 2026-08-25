package uniswapx

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
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

func (s *Solver) refreshQuoteState(ctx context.Context, routes []liquidlane.Route) error {
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
		if s.metrics != nil {
			s.metrics.quoteRefresh.Set(float64(time.Now().Unix()))
		}
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

func (s *Solver) publishQuoteState(epoch uint64, state *quoteState) bool {
	if s.planningFills.Load() != 0 || s.quoteEpoch.Load() != epoch {
		return false
	}
	state.epoch = epoch
	s.quoteState.Store(state)
	if s.planningFills.Load() != 0 || s.quoteEpoch.Load() != epoch {
		s.quoteState.CompareAndSwap(state, nil)
		return false
	}
	return true
}
