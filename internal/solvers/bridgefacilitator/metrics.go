package bridgefacilitator

import (
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	targetRefreshOperation     = "target_refresh"
	offerRefreshOperation      = "offer_refresh"
	activeRefreshOperation     = "active_request_refresh"
	redeemableRefreshOperation = "redeemable_refresh"
)

type threeFMetrics struct {
	mu       sync.Mutex
	workflow *observability.WorkflowMetrics
	backlog  *prometheus.GaugeVec
	since    map[string]time.Time
}

func newThreeFMetrics(reg prometheus.Registerer, strategy string) (*threeFMetrics, error) {
	workflow, err := observability.NewWorkflowMetrics(reg, Name, observability.WorkflowSpec{
		Strategy:   strategy,
		Operations: []string{targetRefreshOperation, offerRefreshOperation, activeRefreshOperation, redeemableRefreshOperation},
		Events: []observability.WorkflowEventSpec{
			{Event: "offer", Outcomes: []string{"success", "error"}},
			{Event: "redeem", Outcomes: []string{"success"}},
		},
		Amounts: []observability.WorkflowAmountSpec{{Event: "offer", Kinds: []string{"principal", "expected_yield"}}},
		States:  []string{"targets", "offers", "active_requests", "redeemable"},
	})
	if err != nil {
		return nil, err
	}
	m := &threeFMetrics{
		workflow: workflow,
		backlog: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "threef_backlog_nonempty_since_timestamp", Help: "Start of a continuous non-empty 3F backlog observation.",
		}, []string{"view"}),
		since: make(map[string]time.Time),
	}
	if err := observability.RegisterCollectors(reg, "bridgefacilitator", m.backlog); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *threeFMetrics) operation(name string) *observability.OperationObserver {
	if m == nil {
		return nil
	}
	return m.workflow.Operation(name)
}

func (m *threeFMetrics) state(view string, count int) {
	if m == nil {
		return
	}
	now := time.Now()
	m.workflow.ObserveStateAt(view, count, now)
	if view != "active_requests" && view != "redeemable" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if count == 0 {
		delete(m.since, view)
		m.backlog.WithLabelValues(view).Set(0)
		return
	}
	if m.since[view].IsZero() {
		m.since[view] = now
		m.backlog.WithLabelValues(view).Set(float64(now.Unix()))
	}
}

func (m *threeFMetrics) offer(outcome string, token common.Address, principal, expectedYield *big.Int) {
	if m == nil {
		return
	}
	m.workflow.ObserveEvent("offer", outcome)
	if outcome == "success" {
		m.workflow.AddAmount("offer", token.Hex(), "principal", principal)
		m.workflow.AddAmount("offer", token.Hex(), "expected_yield", expectedYield)
	}
}

func (m *threeFMetrics) redeemed(count int) {
	if m != nil && count > 0 {
		m.workflow.ObserveEventAt("redeem", "success", float64(count), time.Now())
	}
}
