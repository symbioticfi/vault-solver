package txmanager

import (
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	accountRefreshSuccess = "success"
	accountRefreshError   = "error"
)

type accountSnapshot struct {
	balanceWei   float64
	latestNonce  float64
	pendingNonce float64
	refreshedAt  float64
}

// accountMetrics is one collector so a Prometheus scrape observes account identity, counters, and
// the retained balance/nonce snapshot from one lock-consistent point in time. It emits nothing until
// the transaction manager starts, and it emits snapshot gauges only after the first complete read.
type accountMetrics struct {
	mu sync.RWMutex

	address          string
	snapshot         accountSnapshot
	hasSnapshot      bool
	successRefreshes uint64
	errorRefreshes   uint64
	now              func() time.Time

	infoDesc         *prometheus.Desc
	balanceDesc      *prometheus.Desc
	latestNonceDesc  *prometheus.Desc
	pendingNonceDesc *prometheus.Desc
	refreshesDesc    *prometheus.Desc
	lastRefreshDesc  *prometheus.Desc
}

func newAccountMetrics() *accountMetrics {
	return &accountMetrics{
		now: time.Now,
		infoDesc: newAccountMetricDesc(
			"account_info",
			"Constant 1 identifying the public transaction-sender address.",
			"address",
		),
		balanceDesc: newAccountMetricDesc(
			"account_balance_wei",
			"Latest native-token balance of the transaction-sending account in wei.",
		),
		latestNonceDesc: newAccountMetricDesc(
			"account_latest_nonce",
			"Latest mined nonce reported by the transaction write endpoint.",
		),
		pendingNonceDesc: newAccountMetricDesc(
			"account_pending_nonce",
			"Pending nonce reported by the transaction write endpoint.",
		),
		refreshesDesc: newAccountMetricDesc(
			"account_refreshes_total",
			"Periodic complete signer balance and nonce snapshots by bounded outcome.",
			"outcome",
		),
		lastRefreshDesc: newAccountMetricDesc(
			"account_last_successful_refresh_timestamp",
			"Unix timestamp of the last complete signer balance and nonce snapshot.",
		),
	}
}

func newAccountMetricDesc(name, help string, variableLabels ...string) *prometheus.Desc {
	return prometheus.NewDesc(
		prometheus.BuildFQName(metricsNamespace, metricsSubsystem, name),
		help,
		variableLabels,
		nil,
	)
}

func (m *accountMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.infoDesc
	ch <- m.balanceDesc
	ch <- m.latestNonceDesc
	ch <- m.pendingNonceDesc
	ch <- m.refreshesDesc
	ch <- m.lastRefreshDesc
}

func (m *accountMetrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	address := m.address
	snapshot := m.snapshot
	hasSnapshot := m.hasSnapshot
	successRefreshes := m.successRefreshes
	errorRefreshes := m.errorRefreshes
	m.mu.RUnlock()

	if address == "" {
		return
	}
	ch <- prometheus.MustNewConstMetric(m.infoDesc, prometheus.GaugeValue, 1, address)
	ch <- prometheus.MustNewConstMetric(
		m.refreshesDesc,
		prometheus.CounterValue,
		float64(successRefreshes),
		accountRefreshSuccess,
	)
	ch <- prometheus.MustNewConstMetric(
		m.refreshesDesc,
		prometheus.CounterValue,
		float64(errorRefreshes),
		accountRefreshError,
	)
	if !hasSnapshot {
		return
	}
	ch <- prometheus.MustNewConstMetric(m.balanceDesc, prometheus.GaugeValue, snapshot.balanceWei)
	ch <- prometheus.MustNewConstMetric(m.latestNonceDesc, prometheus.GaugeValue, snapshot.latestNonce)
	ch <- prometheus.MustNewConstMetric(m.pendingNonceDesc, prometheus.GaugeValue, snapshot.pendingNonce)
	ch <- prometheus.MustNewConstMetric(m.lastRefreshDesc, prometheus.GaugeValue, snapshot.refreshedAt)
}

func (m *accountMetrics) bind(address common.Address) {
	normalized := strings.ToLower(address.Hex())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.address != "" && m.address != normalized {
		panic("txmanager: account metrics cannot be rebound to another signer")
	}
	m.address = normalized
}

func (m *accountMetrics) observe(balance *big.Int, latestNonce, pendingNonce uint64) {
	balanceWei, _ := new(big.Float).SetInt(balance).Float64()
	snapshot := accountSnapshot{
		balanceWei:   balanceWei,
		latestNonce:  float64(latestNonce),
		pendingNonce: float64(pendingNonce),
		refreshedAt:  float64(m.now().Unix()),
	}
	m.mu.Lock()
	m.snapshot = snapshot
	m.hasSnapshot = true
	m.successRefreshes++
	m.mu.Unlock()
}

func (m *accountMetrics) observeError() {
	m.mu.Lock()
	m.errorRefreshes++
	m.mu.Unlock()
}

func (m *Metrics) bindAccount(address common.Address) {
	if m != nil {
		m.account.bind(address)
	}
}

func (m *Metrics) observeAccount(balance *big.Int, latestNonce, pendingNonce uint64) {
	if m == nil || balance == nil || balance.Sign() < 0 {
		return
	}
	m.account.observe(balance, latestNonce, pendingNonce)
}

func (m *Metrics) observeAccountRefreshError() {
	if m != nil {
		m.account.observeError()
	}
}
