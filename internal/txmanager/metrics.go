package txmanager

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/observability"
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

type phaseMeasurement struct {
	elapsed time.Duration
	seen    bool
}

// One transaction worker owns this sample until finish consumes its metrics
// pointer. Revisiting a phase extends that phase's cumulative measurement.
type lifecycleObservation struct {
	metrics          *Metrics
	label            string
	started, entered time.Time
	phase            lifecyclePhase
	phases           [lifecyclePhaseCount]phaseMeasurement
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
	group := observability.NewMetricGroup("solver_bot_txmanager_")
	m := &Metrics{
		account:             newAccountMetrics(),
		requests:            group.Counter("requests_total", "Logical transaction requests by terminal outcome.", "label", "outcome"),
		inflight:            group.Gauge("inflight", "Accepted transaction requests awaiting a terminal result.", "label"),
		gasUsed:             group.Counter("gas_used_total", "Gas used by mined transaction receipts.", "label", "outcome"),
		feePaidWei:          group.Counter("fee_paid_wei_total", "Actual transaction fees paid from mined receipt gas usage and effective gas price.", "label", "outcome"),
		replacements:        group.Counter("replacements_total", "Successfully broadcast transaction replacements and cancellations.", "label", "kind"),
		admissionRejections: group.Counter("admission_rejections_total", "Transaction requests rejected before the worker lifecycle by a bounded reason.", "label", "reason"),
		admissionWait:       group.Histogram("admission_wait_duration_seconds", "Time from submission until worker lifecycle admission or a terminal pre-admission outcome; busy TrySend probes are excluded.", []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}, "label", "outcome"),
		lifecycleDuration:   group.Histogram("lifecycle_duration_seconds", "Worker lifecycle duration from admission to terminal outcome; nonce-lane wait is excluded.", []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}, "label", "outcome"),
		phaseDuration:       group.Histogram("phase_duration_seconds", "Cumulative time spent in observed prebroadcast, pending, and confirming phases by terminal outcome.", []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}, "label", "phase", "outcome"),
	}
	group.Add(m.account)
	if err := group.Publish(reg); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Metrics) beginLifecycle(label string) lifecycleObservation {
	observation := lifecycleObservation{metrics: m, label: label}
	if m != nil {
		m.inflight.WithLabelValues(label).Inc()
		observation.started = time.Now()
		observation.entered = observation.started
		observation.phases[lifecyclePhasePrebroadcast].seen = true
	}
	return observation
}

func (o *lifecycleObservation) transitionPhase(next lifecyclePhase) {
	if o.metrics == nil || next == o.phase {
		return
	}
	if next >= lifecyclePhaseCount {
		panic("txmanager: invalid lifecycle phase")
	}
	at := time.Now()
	o.phases[o.phase].elapsed += at.Sub(o.entered)
	o.phases[next].seen = true
	o.phase, o.entered = next, at
}

func (o *lifecycleObservation) finish(outcome Outcome, receipt *types.Receipt) {
	m := o.metrics
	if m == nil {
		return
	}
	o.metrics = nil
	at := time.Now()
	o.phases[o.phase].elapsed += at.Sub(o.entered)
	result := string(outcome)
	m.requests.WithLabelValues(o.label, result).Inc()
	m.inflight.WithLabelValues(o.label).Dec()
	m.lifecycleDuration.WithLabelValues(o.label, result).Observe(at.Sub(o.started).Seconds())
	for phase, sample := range o.phases {
		if sample.seen {
			m.phaseDuration.WithLabelValues(o.label, lifecyclePhase(phase).label(), result).Observe(sample.elapsed.Seconds())
		}
	}
	if receipt == nil {
		return
	}
	m.gasUsed.WithLabelValues(o.label, result).Add(float64(receipt.GasUsed))
	if fee, valid := receiptFeePaidWei(receipt); valid {
		m.feePaidWei.WithLabelValues(o.label, result).Add(fee)
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
	labels := [...]string{"prebroadcast", "pending", "confirming"}
	if int(phase) >= len(labels) {
		panic("txmanager: invalid lifecycle metrics phase")
	}
	return labels[phase]
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
	for _, match := range []struct {
		cause  error
		reason admissionRejectionReason
	}{
		{errManagerStopped, admissionRejectionManagerStopped},
		{errNonceLanePaused, admissionRejectionNonceConflict},
		{context.DeadlineExceeded, admissionRejectionDeadline},
		{context.Canceled, admissionRejectionCallerCancelled},
	} {
		if errors.Is(err, match.cause) {
			return match.reason
		}
	}
	return admissionRejectionOther
}

func (m *Metrics) replacement(label, kind string) {
	if m != nil {
		m.replacements.WithLabelValues(label, kind).Inc()
	}
}
