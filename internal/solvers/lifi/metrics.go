package lifi

import (
	"math/big"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// Quote reconciliation publishes immutable observations. A slow Prometheus
// consumer cannot hold the quote worker's lock or observe half a refresh.
type lifiQuoteMetrics struct {
	current                                                 atomic.Pointer[quoteObservation]
	activeQuotes, activeRanges, pairMaxInput, lastRefreshAt *prometheus.Desc
}

type quoteObservation struct {
	quotes, ranges int
	updatedAt      float64
	pairs          []quotePairObservation
}

type quotePairObservation struct {
	labels  []string
	maximum float64
}

func newLIFIQuoteMetrics() *lifiQuoteMetrics {
	return &lifiQuoteMetrics{
		activeQuotes:  prometheus.NewDesc("lifi_active_quotes", "Process-local standing quote count from the last successful reconciliation; quotes may expire remotely after their TTL.", nil, nil),
		activeRanges:  prometheus.NewDesc("lifi_active_quote_ranges", "Process-local standing quote range count from the last successful reconciliation.", nil, nil),
		pairMaxInput:  prometheus.NewDesc("lifi_active_quote_max_input_atomic_units", "Largest currently advertised input amount per standing quote pair; alternatives are maxed, not summed.", []string{"token_in", "token_out", "token_in_decimals", "token_out_decimals"}, nil),
		lastRefreshAt: prometheus.NewDesc("lifi_last_successful_refresh_timestamp", "Unix timestamp of the last successful standing-quote reconciliation.", nil, nil),
	}
}

func (m *lifiQuoteMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{m.activeQuotes, m.activeRanges, m.pairMaxInput, m.lastRefreshAt} {
		ch <- desc
	}
}

func (m *lifiQuoteMetrics) Collect(ch chan<- prometheus.Metric) {
	observed := m.current.Load()
	if observed == nil {
		observed = &quoteObservation{}
	}
	ch <- prometheus.MustNewConstMetric(m.activeQuotes, prometheus.GaugeValue, float64(observed.quotes))
	ch <- prometheus.MustNewConstMetric(m.activeRanges, prometheus.GaugeValue, float64(observed.ranges))
	ch <- prometheus.MustNewConstMetric(m.lastRefreshAt, prometheus.GaugeValue, observed.updatedAt)
	for _, pair := range observed.pairs {
		ch <- prometheus.MustNewConstMetric(m.pairMaxInput, prometheus.GaugeValue, pair.maximum, pair.labels...)
	}
}

func (m *lifiQuoteMetrics) observe(state *quoteState) {
	observed := &quoteObservation{updatedAt: float64(time.Now().UnixNano()) / float64(time.Second)}
	if state != nil {
		observed.quotes = state.activeQuoteCount()
		for key, pair := range state.active {
			maximum := new(big.Int)
			for _, quote := range pair.quotes {
				observed.ranges += len(quote.Ranges)
				for _, interval := range quote.Ranges {
					if interval.MaxAmount != nil && interval.MaxAmount.Cmp(maximum) > 0 {
						maximum.Set(interval.MaxAmount)
					}
				}
			}
			value, _ := new(big.Float).SetInt(maximum).Float64()
			observed.pairs = append(observed.pairs, quotePairObservation{maximum: value, labels: []string{
				strings.ToLower(key.fromAsset.Hex()), strings.ToLower(key.toAsset.Hex()),
				strconv.Itoa(key.fromDecimals), strconv.Itoa(key.toDecimals),
			}})
		}
	}
	m.current.Store(observed)
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
	group := observability.NewMetricGroup("")
	workflow, err := observability.NewWorkflowMetrics(group, Name, spec)
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
	group.Add(m.quotes, m.orderFeedConnected, m.orderRecoveryReady, m.orderQueueMetrics)
	if err := group.Publish(reg); err != nil {
		return nil, err
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
