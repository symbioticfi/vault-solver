package txmanager

import (
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	accountRefreshSuccess = "success"
	accountRefreshError   = "error"
)

const (
	accountInfo = iota
	accountBalance
	accountLatestNonce
	accountPendingNonce
	accountRefreshes
	accountLastRefresh
	accountMetricCount
)

type accountSnapshot struct {
	balanceWei, latestNonce, pendingNonce, refreshedAt float64
}

type accountObservation struct {
	accountSnapshot

	address             string
	initialized         bool
	successes, failures uint64
}

// Writers replace one immutable observation with CAS. A scrape reads once, so
// account identity, counters and the last complete RPC sample always agree and
// collecting metrics cannot delay the account polling worker.
type accountMetrics struct {
	state       atomic.Pointer[accountObservation]
	descriptors [accountMetricCount]*prometheus.Desc
	now         func() time.Time
}

func newAccountMetrics() *accountMetrics {
	m := &accountMetrics{now: time.Now}
	for _, metric := range []struct {
		id         int
		name, help string
		labels     []string
	}{
		{accountInfo, "account_info", "Constant 1 identifying the public transaction-sender address.", []string{"address"}},
		{accountBalance, "account_balance_wei", "Latest native-token balance of the transaction-sending account in wei.", nil},
		{accountLatestNonce, "account_latest_nonce", "Latest mined nonce reported by the transaction write endpoint.", nil},
		{accountPendingNonce, "account_pending_nonce", "Pending nonce reported by the transaction write endpoint.", nil},
		{accountRefreshes, "account_refreshes_total", "Periodic complete signer balance and nonce snapshots by bounded outcome.", []string{"outcome"}},
		{accountLastRefresh, "account_last_successful_refresh_timestamp", "Unix timestamp of the last complete signer balance and nonce snapshot.", nil},
	} {
		m.descriptors[metric.id] = prometheus.NewDesc(prometheus.BuildFQName(metricsNamespace, metricsSubsystem, metric.name), metric.help, metric.labels, nil)
	}
	return m
}

func (m *accountMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, descriptor := range m.descriptors {
		if descriptor != nil {
			ch <- descriptor
		}
	}
}

func (m *accountMetrics) Collect(ch chan<- prometheus.Metric) {
	state := m.state.Load()
	if state == nil || state.address == "" {
		return
	}
	ch <- prometheus.MustNewConstMetric(m.descriptors[accountInfo], prometheus.GaugeValue, 1, state.address)
	ch <- prometheus.MustNewConstMetric(m.descriptors[accountRefreshes], prometheus.CounterValue, float64(state.successes), accountRefreshSuccess)
	ch <- prometheus.MustNewConstMetric(m.descriptors[accountRefreshes], prometheus.CounterValue, float64(state.failures), accountRefreshError)
	if !state.initialized {
		return
	}
	for _, value := range []struct {
		id    int
		value float64
	}{
		{accountBalance, state.balanceWei}, {accountLatestNonce, state.latestNonce},
		{accountPendingNonce, state.pendingNonce}, {accountLastRefresh, state.refreshedAt},
	} {
		ch <- prometheus.MustNewConstMetric(m.descriptors[value.id], prometheus.GaugeValue, value.value)
	}
}

func (m *accountMetrics) update(change func(*accountObservation)) {
	for {
		previous := m.state.Load()
		next := &accountObservation{}
		if previous != nil {
			*next = *previous
		}
		change(next)
		if m.state.CompareAndSwap(previous, next) {
			return
		}
	}
}

func (m *accountMetrics) bind(address common.Address) {
	normalized := strings.ToLower(address.Hex())
	m.update(func(next *accountObservation) {
		if next.address != "" && next.address != normalized {
			panic("txmanager: account metrics cannot be rebound to another signer")
		}
		next.address = normalized
	})
}

func (m *Metrics) bindAccount(address common.Address) {
	if m == nil {
		return
	}
	m.account.bind(address)
}

func (m *Metrics) observeAccount(balance *big.Int, latestNonce, pendingNonce uint64) {
	if m == nil || balance == nil || balance.Sign() < 0 {
		return
	}
	value, _ := new(big.Float).SetInt(balance).Float64()
	at := float64(m.account.now().Unix())
	m.account.update(func(next *accountObservation) {
		next.accountSnapshot = accountSnapshot{value, float64(latestNonce), float64(pendingNonce), at}
		next.initialized = true
		next.successes++
	})
}

func (m *Metrics) observeAccountRefreshError() {
	if m != nil {
		m.account.update(func(next *accountObservation) { next.failures++ })
	}
}
