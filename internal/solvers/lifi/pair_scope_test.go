package lifi

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

type pairScopeReader struct {
	fakeLifiReader

	routes  []route
	err     error
	started chan struct{}
}

func (r pairScopeReader) resolveRoutes(ctx context.Context, _ []common.Address) ([]route, error) {
	if r.started != nil {
		close(r.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return r.routes, r.err
}

func TestPairScopeRemovalReaddAndZeroInventory(t *testing.T) {
	s, reg := newPairTestSolver(t)
	r := testQuoteRoute()
	trading := []route{r}
	before := slices.Clone(trading)
	s.reader = pairScopeReader{routes: []route{r}}
	s.refreshPairRoutes(t.Context(), trading)
	s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "no_inventory", nil)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "no_inventory"), 1)
	for _, mode := range []string{"removed", "changed output", "changed decimals"} {
		current := []route{r}
		switch mode {
		case "removed":
			current = nil
		case "changed output":
			current[0].TokenOut = common.HexToAddress("0x5678")
		case "changed decimals":
			current[0].TokenInDecimals++
		}
		s.reader = pairScopeReader{routes: current}
		s.refreshPairRoutes(t.Context(), trading)
		if s.pairScopeSource(r.TokenIn, r.TokenOut, []liquidlane.Inventory{{Route: r}}) != "" {
			t.Fatal("old inventory resurrected", mode)
		}
		s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "no_inventory", []liquidlane.Inventory{{Route: r}})
		if !slices.Equal(trading, before) {
			t.Fatal("observability mutated trading routes")
		}
	}
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "no_inventory"), 1)
	s.reader = pairScopeReader{routes: []route{r}}
	s.refreshPairRoutes(t.Context(), trading)
	if s.pairScopeSource(r.TokenOut, r.TokenIn, nil) != "" {
		t.Fatal("reverse route supported")
	}
	s.observeStandingQuoteRejection(t.Context(), r.TokenIn, r.TokenOut, "no_inventory", nil)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", pairEventLabels(s.chainID, r.TokenIn, r.TokenOut, "standing_quote", "rejected", "no_inventory"), 2)
	if pairSeriesCount(t, reg) != 1 {
		t.Fatal("removed routes changed counter inventory")
	}
}

func TestPairScopeFailureDoesNotRenewExpiry(t *testing.T) {
	s, _ := newPairTestSolver(t)
	r := testQuoteRoute()
	s.setPairRoutes([]route{r})
	old := s.pairRoutes.Load()
	s.reader = pairScopeReader{err: errors.New("offline")}
	s.refreshPairRoutes(t.Context(), []route{r})
	if s.pairRoutes.Load() != old {
		t.Fatal("failed read renewed proof")
	}
	s.pairRoutes.Store(&pairRouteScope{routes: old.routes, expiresAt: time.Now().Add(-time.Second)})
	if s.pairScopeSource(r.TokenIn, r.TokenOut, []liquidlane.Inventory{{Route: r}}) != "" {
		t.Fatal("expired proof retained")
	}
}

func TestPairScopeOptionalDiscoveryCancels(t *testing.T) {
	s, _ := newPairTestSolver(t)
	r := testQuoteRoute()
	s.setPairRoutes([]route{r})
	started := make(chan struct{})
	s.reader = pairScopeReader{started: started}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.observePairRoutes(ctx, []route{r}) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("discovery not started")
	}
	// A stalled optional RPC does not block quote/order readers or clear still-fresh proof.
	if s.pairScopeSource(r.TokenIn, r.TokenOut, nil) == "" {
		t.Fatal("optional RPC blocked current proof")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("discovery ignored cancellation")
	}
}
