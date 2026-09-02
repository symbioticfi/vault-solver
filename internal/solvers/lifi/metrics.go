package lifi

import (
	"math/big"
	"strconv"
	"strings"
	"sync"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	quoteRefreshOperation  = "quote_refresh"
	quoteSuspendOperation  = "quote_suspend"
	orderRecoveryOperation = "order_recovery"
)

type lifiOperationObservers struct {
	quoteRefresh  *observability.OperationObserver
	quoteSuspend  *observability.OperationObserver
	orderRecovery *observability.OperationObserver
}

type lifiMetrics struct {
	workflow           *observability.WorkflowMetrics
	operations         lifiOperationObservers
	quotes             *lifiQuoteMetrics
	orderFeedConnected prometheus.GaugeFunc
	orderRecoveryReady prometheus.GaugeFunc
	orderQueueMetrics  *lifiOrderQueueMetrics
	fillAmounts        *liquidlane.FillMetrics
}

// lifiQuoteMetrics registers as one collector and holds the read lock across all child collectors,
// so a scrape cannot interleave with Reset plus repopulation.
type lifiQuoteMetrics struct {
	mu            sync.RWMutex
	activeQuotes  prometheus.Gauge
	activeRanges  prometheus.Gauge
	pairMaxInput  *prometheus.GaugeVec
	lastRefreshAt prometheus.Gauge
}

func newLIFIQuoteMetrics() *lifiQuoteMetrics {
	return &lifiQuoteMetrics{
		activeQuotes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_active_quotes",
			Help: "Process-local standing quote count from the last successful reconciliation; quotes may expire remotely after their TTL.",
		}),
		activeRanges: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_active_quote_ranges",
			Help: "Process-local standing quote range count from the last successful reconciliation.",
		}),
		pairMaxInput: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lifi_active_quote_max_input_atomic_units",
			Help: "Largest currently advertised input amount per standing quote pair; alternatives are maxed, not summed.",
		}, []string{"token_in", "token_out", "token_in_decimals", "token_out_decimals"}),
		lastRefreshAt: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "lifi_last_successful_refresh_timestamp",
			Help: "Unix timestamp of the last successful standing-quote reconciliation.",
		}),
	}
}

func (m *lifiQuoteMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.activeQuotes.Describe(ch)
	m.activeRanges.Describe(ch)
	m.pairMaxInput.Describe(ch)
	m.lastRefreshAt.Describe(ch)
}

func (m *lifiQuoteMetrics) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.activeQuotes.Collect(ch)
	m.activeRanges.Collect(ch)
	m.pairMaxInput.Collect(ch)
	m.lastRefreshAt.Collect(ch)
}

func (m *lifiQuoteMetrics) observe(state *quoteState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pairMaxInput.Reset()
	activeQuotes, activeRanges := 0, 0
	if state != nil {
		activeQuotes = state.activeQuoteCount()
		for key, pair := range state.active {
			maximum := new(big.Int)
			for _, quote := range pair.quotes {
				activeRanges += len(quote.Ranges)
				for _, quoteRange := range quote.Ranges {
					if quoteRange.MaxAmount != nil && quoteRange.MaxAmount.Cmp(maximum) > 0 {
						maximum.Set(quoteRange.MaxAmount)
					}
				}
			}
			value, _ := new(big.Float).SetInt(maximum).Float64()
			m.pairMaxInput.WithLabelValues(
				strings.ToLower(key.fromAsset.Hex()), strings.ToLower(key.toAsset.Hex()),
				strconv.Itoa(key.fromDecimals), strconv.Itoa(key.toDecimals),
			).Set(value)
		}
	}
	m.activeQuotes.Set(float64(activeQuotes))
	m.activeRanges.Set(float64(activeRanges))
	m.lastRefreshAt.SetToCurrentTime()
}

var orderProcessingOutcomes = [...]orderProcessingOutcome{
	orderProcessingSubmitted,
	orderProcessingDepositDeferred,
	orderProcessingCapacityDeferred,
	orderProcessingCapacityDropped,
	orderProcessingNotActionable,
	orderProcessingStrategyDeclined,
	orderProcessingInvalidPlan,
	orderProcessingRetryableError,
	orderProcessingOther,
}

type orderQueue string

const (
	orderQueueInbox         orderQueue = "inbox"
	orderQueueRecoveryRetry orderQueue = "recovery_retry"
	orderQueueCapacityRetry orderQueue = "capacity_retry"
	orderQueueDepositRetry  orderQueue = "deposit_retry"
)

var orderQueues = [...]orderQueue{
	orderQueueInbox,
	orderQueueRecoveryRetry,
	orderQueueCapacityRetry,
	orderQueueDepositRetry,
}

var orderDropQueues = [...]orderQueue{orderQueueInbox, orderQueueCapacityRetry, orderQueueDepositRetry}

type orderQueueSnapshot struct {
	backlog         int
	nearestDeadline int64
}

func earlierOrderDeadlineUnix(current int64, order *submittedOrder) int64 {
	deadline := orderDeadline(order)
	if deadline.IsZero() {
		return current
	}
	if current == 0 || deadline.Unix() < current {
		return deadline.Unix()
	}
	return current
}

type trackedOrderQueue struct {
	generation uint64
	snapshot   func() orderQueueSnapshot
}

// lifiOrderQueueMetrics resolves scrape-time values from the live queue owners.
// Retry queues remain single-worker owned for control flow; their snapshot methods
// add only the synchronization required by concurrent Prometheus collection.
type lifiOrderQueueMetrics struct {
	mu              sync.RWMutex
	nextGeneration  uint64
	tracked         map[orderQueue]trackedOrderQueue
	backlog         *prometheus.Desc
	nearestDeadline *prometheus.Desc
}

func newLIFIOrderQueueMetrics() *lifiOrderQueueMetrics {
	return &lifiOrderQueueMetrics{
		tracked: make(map[orderQueue]trackedOrderQueue, len(orderQueues)),
		backlog: prometheus.NewDesc(
			"lifi_order_backlog",
			"Current process-local LI.FI orders waiting in each execution stage.",
			[]string{"stage"},
			nil,
		),
		nearestDeadline: prometheus.NewDesc(
			"lifi_order_nearest_deadline_timestamp",
			"Unix timestamp of the nearest order deadline in each execution stage; zero when none.",
			[]string{"stage"},
			nil,
		),
	}
}

func (m *lifiOrderQueueMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.backlog
	ch <- m.nearestDeadline
}

func (m *lifiOrderQueueMetrics) Collect(ch chan<- prometheus.Metric) {
	for _, queue := range orderQueues {
		snapshot := m.snapshot(queue)
		ch <- prometheus.MustNewConstMetric(
			m.backlog,
			prometheus.GaugeValue,
			float64(snapshot.backlog),
			string(queue),
		)
		ch <- prometheus.MustNewConstMetric(
			m.nearestDeadline,
			prometheus.GaugeValue,
			float64(snapshot.nearestDeadline),
			string(queue),
		)
	}
}

func (m *lifiOrderQueueMetrics) snapshot(queue orderQueue) orderQueueSnapshot {
	m.mu.RLock()
	snapshot := m.tracked[queue].snapshot
	m.mu.RUnlock()
	if snapshot == nil {
		return orderQueueSnapshot{}
	}
	return snapshot()
}

func (m *lifiMetrics) trackOrderQueue(queue orderQueue, snapshot func() orderQueueSnapshot) func() {
	if m == nil || m.orderQueueMetrics == nil || snapshot == nil {
		return func() {}
	}
	metrics := m.orderQueueMetrics
	metrics.mu.Lock()
	metrics.nextGeneration++
	generation := metrics.nextGeneration
	metrics.tracked[queue] = trackedOrderQueue{generation: generation, snapshot: snapshot}
	metrics.mu.Unlock()
	return func() {
		metrics.mu.Lock()
		if metrics.tracked[queue].generation == generation {
			delete(metrics.tracked, queue)
		}
		metrics.mu.Unlock()
	}
}

func newLIFIMetrics(
	reg prometheus.Registerer,
	feed *orderFeed,
	strategyName string,
) (*lifiMetrics, error) {
	spec := liquidlane.FillWorkflowSpec()
	spec.Strategy = strategyName
	spec.Operations = []string{quoteRefreshOperation, quoteSuspendOperation, orderRecoveryOperation}
	for _, outcome := range orderProcessingOutcomes {
		spec.Events = append(spec.Events, observability.WorkflowEventSpec{
			Event: "order_processing", Outcomes: []string{string(outcome)},
		})
	}
	for _, queue := range orderDropQueues {
		spec.Events = append(spec.Events, observability.WorkflowEventSpec{
			Event: "queue_drop", Outcomes: []string{string(queue)},
		})
	}
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	orderQueueMetrics := newLIFIOrderQueueMetrics()
	m := &lifiMetrics{
		workflow: workflow,
		operations: lifiOperationObservers{
			quoteRefresh:  workflow.Operation(quoteRefreshOperation),
			quoteSuspend:  workflow.Operation(quoteSuspendOperation),
			orderRecovery: workflow.Operation(orderRecoveryOperation),
		},
		quotes: newLIFIQuoteMetrics(),
		orderFeedConnected: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "lifi_order_feed_connected",
			Help: "1 while the LI.FI WebSocket order feed owns an established connection; 0 otherwise.",
		}, func() float64 {
			if feed != nil && feed.connected.Load() {
				return 1
			}
			return 0
		}),
		orderRecoveryReady: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "lifi_order_recovery_ready",
			Help: "1 while the current established LI.FI order-feed connection has completed REST recovery; 0 otherwise.",
		}, func() float64 {
			if feed != nil && feed.connected.Load() && feed.recoveryReady.Load() {
				return 1
			}
			return 0
		}),
		orderQueueMetrics: orderQueueMetrics,
		fillAmounts:       liquidlane.NewFillMetrics(workflow),
	}
	for _, collector := range []prometheus.Collector{
		m.quotes, m.orderFeedConnected, m.orderRecoveryReady, m.orderQueueMetrics,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("lifi: register metric: %w", err)
		}
	}
	return m, nil
}

func (s *Solver) observeQuoteRefresh(state *quoteState) {
	if s.metrics != nil {
		s.metrics.quotes.observe(state)
	}
}

func (s *Solver) operationObservers() lifiOperationObservers {
	if s == nil || s.metrics == nil {
		return lifiOperationObservers{}
	}
	return s.metrics.operations
}

func (m *lifiMetrics) observeOrderProcessing(outcome orderProcessingOutcome) {
	if m != nil {
		m.workflow.ObserveEvent("order_processing", string(boundedOrderProcessingOutcome(outcome)))
	}
}

func boundedOrderProcessingOutcome(outcome orderProcessingOutcome) orderProcessingOutcome {
	for _, declared := range orderProcessingOutcomes {
		if outcome == declared {
			return outcome
		}
	}
	return orderProcessingOther
}

func (m *lifiMetrics) observeOrderQueueDrop(queue orderQueue, err error) {
	if m == nil || !orderQueueWasDropped(queue, err) {
		return
	}
	m.workflow.ObserveEvent("queue_drop", string(queue))
}

func orderQueueWasDropped(queue orderQueue, err error) bool {
	switch queue {
	case orderQueueInbox:
		return errors.Is(err, errOrderInboxFull)
	case orderQueueCapacityRetry:
		return errors.Is(err, errOrderRetryFull)
	case orderQueueDepositRetry:
		return errors.Is(err, errOrderDepositRetryFull) ||
			errors.Is(err, errOrderDepositRetryKey) ||
			errors.Is(err, errOrderDepositRetryExpired) ||
			errors.Is(err, errOrderDepositRetryWindow)
	case orderQueueRecoveryRetry:
		return false
	default:
		return false
	}
}
