package txmanager

import (
	"context"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

var accountMetricFamilyNames = []string{
	"solver_bot_txmanager_account_info",
	"solver_bot_txmanager_account_balance_wei",
	"solver_bot_txmanager_account_latest_nonce",
	"solver_bot_txmanager_account_pending_nonce",
	"solver_bot_txmanager_account_refreshes_total",
	"solver_bot_txmanager_account_last_successful_refresh_timestamp",
}

func TestAccountMetricsActivateOnlyWhenManagerStarts(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	testcheck.NoError(t, err)
	sgnr := mustSigner(t)
	manager := New(
		newMockBackend(), sgnr, big.NewInt(11155111), Config{}, metrics, logr.Discard(),
	)

	assertAccountMetricSeriesCount(t, reg, 0)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager.Start(ctx)

	// Identity plus both zero-valued refresh outcomes produce three series; snapshot gauges remain
	// absent because this backend does not expose balance reads.
	assertAccountMetricSeriesCount(t, reg, 3)
	state := metrics.account.state.Load()
	address := state.address
	hasSnapshot := state.initialized
	if want := strings.ToLower(sgnr.Address().Hex()); address != want {
		t.Fatalf("account address = %q, want %q", address, want)
	}
	if hasSnapshot {
		t.Fatal("account snapshot appeared without a complete refresh")
	}
}

func TestAccountMetricsRetainLastSuccessfulSnapshot(t *testing.T) {
	backend := &accountMetricsBackend{
		mockBackend: newMockBackend(),
		balance:     big.NewInt(2_500_000_000_000_000_000),
	}
	backend.latestNonce = 11
	backend.pendingNonce = 12
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	testcheck.NoError(t, err)
	metrics.account.now = func() time.Time { return time.Unix(123, 0) }
	sgnr := mustSigner(t)
	metrics.bindAccount(sgnr.Address())
	manager := New(backend, sgnr, big.NewInt(11155111), Config{}, metrics, logr.Discard())

	manager.refreshAccount(t.Context())
	want := accountSnapshot{
		balanceWei:  2_500_000_000_000_000_000,
		latestNonce: 11, pendingNonce: 12, refreshedAt: 123,
	}
	if got := gatherAccountSnapshot(t, reg); got != want {
		t.Fatalf("account snapshot = %+v, want %+v", got, want)
	}
	assertAccountRefreshes(t, metrics.account, 1, 0)

	backend.balanceErr = errors.New("rpc unavailable")
	manager.refreshAccount(t.Context())
	if got := gatherAccountSnapshot(t, reg); got != want {
		t.Fatalf("snapshot after failed refresh = %+v, want retained %+v", got, want)
	}
	assertAccountRefreshes(t, metrics.account, 1, 1)
}

func TestAccountMetricsScrapeIsSnapshotConsistent(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	testcheck.NoError(t, err)
	metrics.bindAccount(common.HexToAddress("0x1234"))
	value := int64(1)
	metrics.account.now = func() time.Time { return time.Unix(value, 0) }
	metrics.observeAccount(big.NewInt(value), uint64(value), uint64(value))

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			value = 3 - value
			metrics.observeAccount(big.NewInt(value), uint64(value), uint64(value))
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()

	for range 500 {
		snapshot := gatherAccountSnapshot(t, reg)
		if snapshot.balanceWei != snapshot.latestNonce ||
			snapshot.latestNonce != snapshot.pendingNonce ||
			snapshot.pendingNonce != snapshot.refreshedAt {
			t.Fatalf("scrape mixed account snapshots: %+v", snapshot)
		}
	}
}

type accountMetricsBackend struct {
	*mockBackend

	balance    *big.Int
	balanceErr error
}

func (b *accountMetricsBackend) TransactionSenderBalanceAt(
	context.Context,
	common.Address,
	*big.Int,
) (*big.Int, error) {
	if b.balanceErr != nil {
		return nil, b.balanceErr
	}
	return new(big.Int).Set(b.balance), nil
}

func assertAccountMetricSeriesCount(t *testing.T, gatherer prometheus.Gatherer, want int) {
	t.Helper()
	got, err := testutil.GatherAndCount(gatherer, accountMetricFamilyNames...)
	testcheck.NoError(t, err, "gather account metrics: %v")
	if got != want {
		t.Fatalf("account metric series = %d, want %d", got, want)
	}
}

func assertAccountRefreshes(t *testing.T, metrics *accountMetrics, success, failed uint64) {
	t.Helper()
	state := metrics.state.Load()
	if state.successes != success || state.failures != failed {
		t.Fatalf(
			"account refreshes = (%d, %d), want (%d, %d)",
			state.successes,
			state.failures,
			success,
			failed,
		)
	}
}

func gatherAccountSnapshot(t *testing.T, gatherer prometheus.Gatherer) accountSnapshot {
	t.Helper()
	families, err := gatherer.Gather()
	testcheck.NoError(t, err, "gather account metrics: %v")
	var snapshot accountSnapshot
	found := 0
	for _, family := range families {
		if len(family.GetMetric()) != 1 {
			continue
		}
		value := family.GetMetric()[0].GetGauge().GetValue()
		switch family.GetName() {
		case "solver_bot_txmanager_account_balance_wei":
			snapshot.balanceWei = value
		case "solver_bot_txmanager_account_latest_nonce":
			snapshot.latestNonce = value
		case "solver_bot_txmanager_account_pending_nonce":
			snapshot.pendingNonce = value
		case "solver_bot_txmanager_account_last_successful_refresh_timestamp":
			snapshot.refreshedAt = value
		default:
			continue
		}
		found++
	}
	if found != 4 {
		t.Fatalf("complete account snapshot families = %d, want 4", found)
	}
	return snapshot
}

func TestAccountMetricsConcurrentUpdatesKeepEveryRefresh(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := NewMetrics(reg)
	testcheck.NoError(t, err)
	metrics.bindAccount(common.Address{19: 1})
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				metrics.observeAccount(big.NewInt(10), 2, 3)
				metrics.observeAccountRefreshError()
			}
		})
	}
	workers.Wait()
	assertAccountRefreshes(t, metrics.account, 400, 400)
	got := gatherAccountSnapshot(t, reg)
	if got.balanceWei != 10 || got.latestNonce != 2 || got.pendingNonce != 3 {
		t.Fatalf("incoherent account snapshot: %+v", got)
	}
}
