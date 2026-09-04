package lifi

import (
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	quoteRefreshOperation  = "quote_refresh"
	quoteSuspendOperation  = "quote_suspend"
	orderRecoveryOperation = "order_recovery"
)

const (
	queueInbox orderQueueStage = iota
	queueRecoveryRetry
	queueCapacityRetry
	queueDepositRetry
	queueStageCount
)

var queueStageLabels = [queueStageCount]string{"inbox", "recovery_retry", "capacity_retry", "deposit_retry"}

type lifiMetrics struct {
	workflow            *observability.WorkflowMetrics
	quotes              *lifiQuoteMetrics
	connected, recovery prometheus.Gauge
	backlog, deadline   *prometheus.GaugeVec
	fill                *liquidlane.FillMetrics
}

// lifiQuoteMetrics makes a refresh atomic from the collector's point of view: pair.Reset and
// repopulation must not expose a partial quote snapshot to a concurrent scrape.
type lifiQuoteMetrics struct {
	mu                    sync.RWMutex
	active, ranges, fresh prometheus.Gauge
	pair                  *prometheus.GaugeVec
}

func newLIFIMetrics(reg prometheus.Registerer, strategy string) (*lifiMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Strategy = strategy
	spec.Operations = []string{quoteRefreshOperation, quoteSuspendOperation, orderRecoveryOperation}
	spec.Events = append(spec.Events,
		observability.WorkflowEventSpec{Event: "order_processing", Outcomes: []string{"submitted", "deferred", "dropped", "declined", "error"}},
		observability.WorkflowEventSpec{Event: "queue_drop", Outcomes: []string{"inbox", "capacity_retry", "deposit_retry"}},
	)
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	quotes := &lifiQuoteMetrics{
		active: prometheus.NewGauge(prometheus.GaugeOpts{Name: "lifi_active_quotes", Help: "Active standing quotes."}),
		ranges: prometheus.NewGauge(prometheus.GaugeOpts{Name: "lifi_active_quote_ranges", Help: "Active standing quote ranges."}),
		fresh:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "lifi_last_successful_refresh_timestamp", Help: "Last quote reconciliation."}),
		pair: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lifi_active_quote_max_input_atomic_units", Help: "Largest input range per pair."},
			[]string{"token_in", "token_out", "token_in_decimals", "token_out_decimals"}),
	}
	m := &lifiMetrics{
		workflow:  workflow,
		quotes:    quotes,
		connected: prometheus.NewGauge(prometheus.GaugeOpts{Name: "lifi_order_feed_connected", Help: "Whether the order feed is connected."}),
		recovery:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "lifi_order_recovery_ready", Help: "Whether REST recovery completed for this connection."}),
		backlog:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lifi_order_backlog", Help: "Orders waiting by stage."}, []string{"stage"}),
		deadline:  prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "lifi_order_nearest_deadline_timestamp", Help: "Nearest waiting order deadline by stage."}, []string{"stage"}),
		fill:      liquidlane.NewFillMetrics(workflow),
	}
	if err := observability.RegisterCollectors(reg, "lifi",
		m.quotes, m.connected, m.recovery, m.backlog, m.deadline,
	); err != nil {
		return nil, err
	}
	m.queues(orderQueueMetrics{})
	return m, nil
}

func (m *lifiQuoteMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.active.Describe(ch)
	m.ranges.Describe(ch)
	m.fresh.Describe(ch)
	m.pair.Describe(ch)
}

func (m *lifiQuoteMetrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.active.Collect(ch)
	m.ranges.Collect(ch)
	m.fresh.Collect(ch)
	m.pair.Collect(ch)
}

func (m *lifiQuoteMetrics) observe(state *quoteState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pair.Reset()
	quotes, ranges := 0, 0
	for key, value := range state.active {
		quotes += len(value.quotes)
		maximum := new(big.Int)
		for _, quote := range value.quotes {
			for _, span := range quote.Ranges {
				ranges++
				if span.MaxAmount != nil && span.MaxAmount.Cmp(maximum) > 0 {
					maximum.Set(span.MaxAmount)
				}
			}
		}
		asFloat, _ := new(big.Float).SetInt(maximum).Float64()
		m.pair.WithLabelValues(strings.ToLower(key.fromAsset.Hex()), strings.ToLower(key.toAsset.Hex()), strconv.Itoa(key.fromDecimals), strconv.Itoa(key.toDecimals)).Set(asFloat)
	}
	m.active.Set(float64(quotes))
	m.ranges.Set(float64(ranges))
	m.fresh.SetToCurrentTime()
}

func (m *lifiMetrics) observeQuotes(state *quoteState) {
	if m != nil {
		m.quotes.observe(state)
	}
}

func (m *lifiMetrics) operation(name string) *observability.OperationObserver {
	if m == nil {
		return nil
	}
	return m.workflow.Operation(name)
}

func (m *lifiMetrics) feedState(connected, recovered bool) {
	if m == nil {
		return
	}
	m.connected.Set(boolFloat(connected))
	m.recovery.Set(boolFloat(connected && recovered))
}

func (m *lifiMetrics) queues(snapshot orderQueueMetrics) {
	if m == nil {
		return
	}
	for stage, label := range queueStageLabels {
		m.backlog.WithLabelValues(label).Set(float64(snapshot[stage].count))
		m.deadline.WithLabelValues(label).Set(float64(snapshot[stage].deadline))
	}
}

func (m *lifiMetrics) order(outcome string) {
	if m != nil {
		m.workflow.ObserveEvent("order_processing", outcome)
	}
}

func (m *lifiMetrics) queueDrop(stage string) {
	if m != nil {
		m.workflow.ObserveEvent("queue_drop", stage)
	}
}

func boolFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

type orderQueueMetric struct {
	count    int
	deadline int64
}

type orderQueueStage uint8
type orderQueueMetrics [queueStageCount]orderQueueMetric
