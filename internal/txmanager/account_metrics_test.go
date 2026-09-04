package txmanager

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestAccountMetricsPublishOnlyCompleteSnapshots(t *testing.T) {
	metrics, err := NewMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	metrics.bindAccount(common.HexToAddress("0x1234"))
	if got := testutil.CollectAndCount(metrics.account.balance); got != 0 {
		t.Fatalf("balance series before refresh = %d, want 0", got)
	}
	metrics.account.now = func() time.Time { return time.Unix(123, 0) }
	metrics.observeAccount(big.NewInt(42), 7, 8)
	metrics.observeAccountRefreshError()

	checks := map[string]struct {
		collector prometheus.Collector
		want      float64
	}{
		"balance": {metrics.account.balance, 42},
		"latest":  {metrics.account.latestNonce, 7},
		"pending": {metrics.account.pendingNonce, 8},
		"fresh":   {metrics.account.lastRefresh, 123},
		"success": {metrics.account.refreshes.WithLabelValues(accountRefreshSuccess), 1},
		"error":   {metrics.account.refreshes.WithLabelValues(accountRefreshError), 1},
	}
	for name, check := range checks {
		if got := testutil.ToFloat64(check.collector); got != check.want {
			t.Errorf("%s = %v, want %v", name, got, check.want)
		}
	}
}
