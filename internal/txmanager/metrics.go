package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "solver_bot"
	metricsSubsystem = "txmanager"

	replacementKindReplacement  = "replacement"
	replacementKindCancellation = "cancellation"

	admissionOutcomeAdmitted admissionOutcome = "admitted"

	admissionRejectionManagerStopped  admissionRejectionReason = "manager_stopped"
	admissionRejectionNonceConflict   admissionRejectionReason = "nonce_conflict"
	admissionRejectionDeadline        admissionRejectionReason = "deadline_exceeded"
	admissionRejectionCallerCancelled admissionRejectionReason = "caller_cancelled"
	admissionRejectionOther           admissionRejectionReason = "other"
)

const (
	lifecyclePhasePrebroadcast lifecyclePhase = iota
	lifecyclePhasePending
	lifecyclePhaseConfirming
	lifecyclePhaseCount
)

type admissionRejectionReason string
type admissionOutcome string
type lifecyclePhase uint8

type lifecycleObservation struct {
	metrics        *Metrics
	label          string
	started        time.Time
	phase          lifecyclePhase
	phaseStarted   time.Time
	phaseDurations [lifecyclePhaseCount]time.Duration
	phaseObserved  [lifecyclePhaseCount]bool
}

// Metrics records the transaction lifecycle shared by every solver.
type Metrics struct {
	requests            *prometheus.CounterVec
	inflight            *prometheus.GaugeVec
	gasUsed             *prometheus.CounterVec
	feePaidWei          *prometheus.CounterVec
	replacements        *prometheus.CounterVec
	admissionRejections *prometheus.CounterVec
	admissionWait       *prometheus.HistogramVec
	lifecycleDuration   *prometheus.HistogramVec
	phaseDuration       *prometheus.HistogramVec
	account             *accountMetrics
}

// NewMetrics registers transaction lifecycle collectors.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	if reg == nil {
		return nil, errors.New("txmanager: metrics registerer is required")
	}
	m := &Metrics{
		account: newAccountMetrics(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "requests_total",
			Help:      "Logical transaction requests by terminal outcome.",
		}, []string{"label", "outcome"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "inflight",
			Help:      "Accepted transaction requests awaiting a terminal result.",
		}, []string{"label"}),
		gasUsed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "gas_used_total",
			Help:      "Gas used by mined transaction receipts.",
		}, []string{"label", "outcome"}),
		feePaidWei: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "fee_paid_wei_total",
			Help:      "Actual transaction fees paid from mined receipt gas usage and effective gas price.",
		}, []string{"label", "outcome"}),
		replacements: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "replacements_total",
			Help:      "Successfully broadcast transaction replacements and cancellations.",
		}, []string{"label", "kind"}),
		admissionRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "admission_rejections_total",
			Help:      "Transaction requests rejected before the worker lifecycle by a bounded reason.",
		}, []string{"label", "reason"}),
		admissionWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "admission_wait_duration_seconds",
			Help:      "Time from submission until worker lifecycle admission or a terminal pre-admission outcome; busy TrySend probes are excluded.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}, []string{"label", "outcome"}),
		lifecycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "lifecycle_duration_seconds",
			Help:      "Worker lifecycle duration from admission to terminal outcome; nonce-lane wait is excluded.",
			Buckets:   []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"label", "outcome"}),
		phaseDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "phase_duration_seconds",
			Help:      "Cumulative time spent in observed prebroadcast, pending, and confirming phases by terminal outcome.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"label", "phase", "outcome"}),
	}
	for _, collector := range []prometheus.Collector{
		m.requests,
		m.inflight,
		m.gasUsed,
		m.feePaidWei,
		m.replacements,
		m.admissionRejections,
		m.admissionWait,
		m.lifecycleDuration,
		m.phaseDuration,
		m.account,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("txmanager: register metric: %w", err)
		}
	}
	return m, nil
}

func (m *Metrics) beginLifecycle(label string) lifecycleObservation {
	if m == nil {
		return lifecycleObservation{}
	}
	m.inflight.WithLabelValues(label).Inc()
	now := time.Now()
	observation := lifecycleObservation{
		metrics:      m,
		label:        label,
		started:      now,
		phase:        lifecyclePhasePrebroadcast,
		phaseStarted: now,
	}
	observation.phaseObserved[lifecyclePhasePrebroadcast] = true
	return observation
}

func (observation *lifecycleObservation) transitionPhase(next lifecyclePhase) {
	if observation.metrics == nil || observation.phase == next {
		return
	}
	now := time.Now()
	observation.phaseDurations[observation.phase] += now.Sub(observation.phaseStarted)
	observation.phase = next
	observation.phaseStarted = now
	observation.phaseObserved[next] = true
}

func (observation *lifecycleObservation) finish(outcome Outcome, receipt *types.Receipt) {
	if observation.metrics == nil {
		return
	}
	now := time.Now()
	observation.phaseDurations[observation.phase] += now.Sub(observation.phaseStarted)
	outcomeLabel := string(outcome)
	observation.metrics.requests.WithLabelValues(observation.label, outcomeLabel).Inc()
	observation.metrics.inflight.WithLabelValues(observation.label).Dec()
	observation.metrics.lifecycleDuration.WithLabelValues(observation.label, outcomeLabel).
		Observe(now.Sub(observation.started).Seconds())
	for phase := range lifecyclePhaseCount {
		if observation.phaseObserved[phase] {
			observation.metrics.phaseDuration.WithLabelValues(
				observation.label,
				phase.label(),
				outcomeLabel,
			).Observe(observation.phaseDurations[phase].Seconds())
		}
	}
	if receipt != nil {
		observation.metrics.gasUsed.WithLabelValues(observation.label, outcomeLabel).
			Add(float64(receipt.GasUsed))
		if fee, ok := receiptFeePaidWei(receipt); ok {
			observation.metrics.feePaidWei.WithLabelValues(observation.label, outcomeLabel).Add(fee)
		}
	}
}

func receiptFeePaidWei(receipt *types.Receipt) (float64, bool) {
	if receipt == nil || receipt.EffectiveGasPrice == nil || receipt.EffectiveGasPrice.Sign() < 0 {
		return 0, false
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	value, _ := new(big.Float).SetInt(fee).Float64()
	return value, true
}

func (phase lifecyclePhase) label() string {
	switch phase {
	case lifecyclePhasePrebroadcast:
		return "prebroadcast"
	case lifecyclePhasePending:
		return "pending"
	case lifecyclePhaseConfirming:
		return "confirming"
	case lifecyclePhaseCount:
		panic("txmanager: lifecycle phase count has no metrics label")
	default:
		panic("txmanager: invalid lifecycle metrics phase")
	}
}

func (m *Metrics) finishAdmission(label string, started time.Time, err error) {
	if m == nil {
		return
	}
	outcome := admissionOutcomeAdmitted
	if err != nil {
		reason := classifyAdmissionRejection(err)
		m.admissionRejections.WithLabelValues(label, string(reason)).Inc()
		outcome = admissionOutcome(reason)
	}
	m.admissionWait.WithLabelValues(label, string(outcome)).Observe(time.Since(started).Seconds())
}

// classifyAdmissionRejection keeps errors out of labels and bounds future failure modes to "other".
func classifyAdmissionRejection(err error) admissionRejectionReason {
	switch {
	case errors.Is(err, errManagerStopped):
		return admissionRejectionManagerStopped
	case errors.Is(err, errNonceLanePaused):
		return admissionRejectionNonceConflict
	case errors.Is(err, context.DeadlineExceeded):
		return admissionRejectionDeadline
	case errors.Is(err, context.Canceled):
		return admissionRejectionCallerCancelled
	default:
		return admissionRejectionOther
	}
}

func (m *Metrics) replacement(label, kind string) {
	if m != nil {
		m.replacements.WithLabelValues(label, kind).Inc()
	}
}
