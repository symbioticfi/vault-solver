package defaultstrategy

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type decisionStateReader struct {
	Reader

	balance    *big.Int
	balanceErr error
}

func (r decisionStateReader) ReadNativeBalance(context.Context, common.Address) (*big.Int, error) {
	return cloneBig(r.balance), r.balanceErr
}

func TestRefreshStateReadsCallbackBalance(t *testing.T) {
	s := &Strategy{
		reader: decisionStateReader{balance: big.NewInt(20)},
		log:    logr.Discard(),
	}
	s.refreshState(t.Context())
	got, ok := s.state.load()
	if !ok || got.CallbackNative.Cmp(big.NewInt(20)) != 0 || got.CallbackUpdatedAt.IsZero() {
		t.Fatalf("decision state = %+v, want fresh callback balance 20", got)
	}
}

func TestRefreshStatePreservesLastGoodValuesWithoutExtendingFreshness(t *testing.T) {
	previousAt := time.Unix(1000, 0)
	s := &Strategy{
		reader: decisionStateReader{balanceErr: errors.New("rpc unavailable")},
		log:    logr.Discard(),
	}
	s.state.store(decisionState{
		CallbackNative:    big.NewInt(20),
		CallbackUpdatedAt: previousAt,
	})

	s.refreshState(t.Context())
	got, ok := s.state.load()
	if !ok {
		t.Fatal("decision state missing")
	}
	if got.CallbackNative.Int64() != 20 {
		t.Fatalf("last good values not preserved: %+v", got)
	}
	if !got.CallbackUpdatedAt.Equal(previousAt) {
		t.Fatalf("failed refresh extended freshness: %+v", got)
	}
}

func TestNewUsesConfiguredTestMonitor(t *testing.T) {
	market := common.Hash{31: 1}
	position := common.Address{19: 2}
	deps := Deps{
		Log:      logr.Discard(),
		Adapter:  common.Address{19: 1},
		Callback: common.Address{19: 2},
		LoadAdapterSnapshot: func() (types.AdapterSnapshot, bool) {
			return types.AdapterSnapshot{}, true
		},
	}
	strategy, err := New(Config{
		TestMonitor: &TestMonitorConfig{
			Markets:   []common.Hash{market},
			Positions: []common.Address{position},
		},
		MonitorPoll: time.Second,
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	monitor, ok := strategy.mon.(*testMonitor)
	if !ok {
		t.Fatalf("monitor type = %T, want *testMonitor", strategy.mon)
	}
	if len(monitor.markets) != 1 || monitor.markets[0] != market ||
		len(monitor.positions) != 1 || monitor.positions[0] != position {
		t.Fatalf("monitor seeds = %v/%v", monitor.markets, monitor.positions)
	}
}

func TestNewRejectsGasDerivedProfitPoliciesWithoutGasAccounting(t *testing.T) {
	deps := Deps{
		Log:      logr.Discard(),
		Adapter:  common.Address{19: 1},
		Callback: common.Address{19: 2},
		LoadAdapterSnapshot: func() (types.AdapterSnapshot, bool) {
			return types.AdapterSnapshot{}, true
		},
	}
	tests := map[string]Config{
		"total bundle profit share":    {TotalBundleProfitBps: 1},
		"minimum bundle profit margin": {MinBundleProfitBidBps: 1},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg, deps); err == nil || !strings.Contains(err.Error(), "requires gas accounting") {
				t.Fatalf("error = %v, want gas-accounting requirement", err)
			}
		})
	}
}
