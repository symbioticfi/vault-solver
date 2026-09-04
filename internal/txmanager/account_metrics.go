package txmanager

import (
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	accountRefreshSuccess = "success"
	accountRefreshError   = "error"
)

// accountMetrics uses ordinary Prometheus collectors. Zero-label vectors keep snapshot series
// absent until the first complete refresh, without maintaining a second metrics state machine.
type accountMetrics struct {
	now func() time.Time

	info, balance, latestNonce, pendingNonce, lastRefresh *prometheus.GaugeVec
	refreshes                                             *prometheus.CounterVec
}

func newAccountMetrics() *accountMetrics {
	gauge := func(name, help string, labels ...string) *prometheus.GaugeVec {
		return prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace, Subsystem: metricsSubsystem, Name: name, Help: help,
		}, labels)
	}
	return &accountMetrics{
		now:          time.Now,
		info:         gauge("account_info", "Constant 1 identifying the public transaction-sender address.", "address"),
		balance:      gauge("account_balance_wei", "Latest native-token balance of the transaction-sending account in wei."),
		latestNonce:  gauge("account_latest_nonce", "Latest mined nonce reported by the transaction write endpoint."),
		pendingNonce: gauge("account_pending_nonce", "Pending nonce reported by the transaction write endpoint."),
		lastRefresh:  gauge("account_last_successful_refresh_timestamp", "Unix timestamp of the last complete signer balance and nonce snapshot."),
		refreshes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace, Subsystem: metricsSubsystem, Name: "account_refreshes_total",
			Help: "Periodic complete signer balance and nonce snapshots by bounded outcome.",
		}, []string{"outcome"}),
	}
}

func (m *accountMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.info, m.balance, m.latestNonce, m.pendingNonce, m.refreshes, m.lastRefresh}
}

func (m *accountMetrics) bind(address common.Address) {
	normalized := strings.ToLower(address.Hex())
	m.info.WithLabelValues(normalized).Set(1)
	m.refreshes.WithLabelValues(accountRefreshSuccess).Add(0)
	m.refreshes.WithLabelValues(accountRefreshError).Add(0)
}

func (m *accountMetrics) observe(balance *big.Int, latestNonce, pendingNonce uint64) {
	value, _ := new(big.Float).SetInt(balance).Float64()
	m.balance.WithLabelValues().Set(value)
	m.latestNonce.WithLabelValues().Set(float64(latestNonce))
	m.pendingNonce.WithLabelValues().Set(float64(pendingNonce))
	m.lastRefresh.WithLabelValues().Set(float64(m.now().Unix()))
	m.refreshes.WithLabelValues(accountRefreshSuccess).Inc()
}

func (m *Metrics) bindAccount(address common.Address) {
	if m != nil {
		m.account.bind(address)
	}
}

func (m *Metrics) observeAccount(balance *big.Int, latestNonce, pendingNonce uint64) {
	if m != nil && balance != nil && balance.Sign() >= 0 {
		m.account.observe(balance, latestNonce, pendingNonce)
	}
}

func (m *Metrics) observeAccountRefreshError() {
	if m != nil {
		m.account.refreshes.WithLabelValues(accountRefreshError).Inc()
	}
}
