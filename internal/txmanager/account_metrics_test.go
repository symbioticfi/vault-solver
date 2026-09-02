package txmanager

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
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
	if err != nil {
		t.Fatal(err)
	}
	sgnr := mustSigner(t)
	manager := NewWithMetrics(
		newMockBackend(), sgnr, big.NewInt(11155111), Config{}, metrics, logr.Discard(),
	)

	assertAccountMetricSeriesCount(t, reg, 0)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	manager.Start(ctx)

	// Identity plus both zero-valued refresh outcomes produce three series; snapshot gauges remain
	// absent because this backend does not expose balance reads.
	assertAccountMetricSeriesCount(t, reg, 3)
	metrics.account.mu.RLock()
	address := metrics.account.address
	hasSnapshot := metrics.account.hasSnapshot
	metrics.account.mu.RUnlock()
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
	if err != nil {
		t.Fatal(err)
	}
	metrics.account.now = func() time.Time { return time.Unix(123, 0) }
	sgnr := mustSigner(t)
	metrics.bindAccount(sgnr.Address())
	manager := NewWithMetrics(backend, sgnr, big.NewInt(11155111), Config{}, metrics, logr.Discard())

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
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatalf("gather account metrics: %v", err)
	}
	if got != want {
		t.Fatalf("account metric series = %d, want %d", got, want)
	}
}

func assertAccountRefreshes(t *testing.T, metrics *accountMetrics, success, failed uint64) {
	t.Helper()
	metrics.mu.RLock()
	defer metrics.mu.RUnlock()
	if metrics.successRefreshes != success || metrics.errorRefreshes != failed {
		t.Fatalf(
			"account refreshes = (%d, %d), want (%d, %d)",
			metrics.successRefreshes,
			metrics.errorRefreshes,
			success,
			failed,
		)
	}
}

func gatherAccountSnapshot(t *testing.T, gatherer prometheus.Gatherer) accountSnapshot {
	t.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather account metrics: %v", err)
	}
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
