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

	// The balance guard refuses after the worker admitted the request and before anything is signed,
	// so these reasons are recorded by the worker rather than by the admission wait.
	admissionRejectionUnaffordable         admissionRejectionReason = "unaffordable"
	admissionRejectionUnaffordableOneBlock admissionRejectionReason = "unaffordable_one_block"
	admissionRejectionStaleHead            admissionRejectionReason = "stale_head"
	// admissionRejectionFeeCeiling is reserved for the per-request total-fee ceiling (Request.MaxFeeWei),
	// which is not implemented yet; the schema carries it so dashboards need no change when it lands.
	admissionRejectionFeeCeiling admissionRejectionReason = "fee_ceiling"

	attemptKindFill         = "fill"
	attemptKindCancellation = "cancellation"

	firstAttemptFirst     firstAttemptOutcome = "first"
	firstAttemptLate      firstAttemptOutcome = "late"
	firstAttemptReplaced  firstAttemptOutcome = "replaced"
	firstAttemptCancelled firstAttemptOutcome = "cancelled"

	// simulationUnknown is the only simulation class until next-block simulation classifies stalled and
	// cancelled fills (strategy PR5, a TODO in docs/TXMANAGER-PLAN.md §10).
	simulationUnknown = "unknown"

	// Reasons of rebroadcasts_total: an exact rebroadcast of signed bytes, which signs nothing new.
	rebroadcastStall     = "stall"     // a stalled attempt (strategy §2.7 rule 4)
	rebroadcastReorg     = "reorg"     // an attempt whose inclusion a reorg removed
	rebroadcastUncertain = "uncertain" // an attempt whose broadcast was ambiguous
	rebroadcastCapped    = "capped"    // the latest attempt, when no replacement can be funded or fits the cap

	requiredBalanceMin   = "min"
	requiredBalanceQuote = "quote"
	requiredBalanceFull  = "full"
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

// firstAttemptOutcome classifies how a lifecycle that reached the chain landed relative to its first
// signed attempt.
type firstAttemptOutcome string

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
	firstAttempts       *prometheus.CounterVec
	inclusionDelay      *prometheus.HistogramVec
	pendingAge          *prometheus.GaugeVec
	attemptTip          *prometheus.GaugeVec
	attemptMaxFee       *prometheus.GaugeVec
	attemptGasLimit     *prometheus.GaugeVec
	attemptHorizon      *prometheus.HistogramVec
	// nextBaseFee has no labels but is a vector so the series is absent until the first fee reading,
	// rather than exported as 0 (a 0 cannot be told apart from a reading).
	nextBaseFee         *prometheus.GaugeVec
	requiredBalance     *prometheus.GaugeVec
	gasEstimates        *prometheus.CounterVec
	gasEstimateDuration *prometheus.HistogramVec
	repricings          *prometheus.CounterVec
	rebroadcasts        *prometheus.CounterVec
	tipPaidWei          *prometheus.CounterVec
	shadowLifecycles    *prometheus.CounterVec
	feePolicy           *prometheus.GaugeVec
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
			Help:      "Transaction requests rejected before signing by a bounded reason: before the worker lifecycle, or by the balance guard.",
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
		firstAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "first_attempt_total",
			Help:      "Lifecycles that reached the chain, by how they landed: first (first signed attempt within 3 blocks), late, replaced or cancelled; simulation classifies non-first ones.",
		}, []string{"label", "outcome", "simulation"}),
		inclusionDelay: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "inclusion_delay_blocks",
			Help:      "Blocks from the newest block a lifecycle's first attempt was priced knowing of to the block that included its call; 1 is the next block. Cancellations are excluded.",
			Buckets:   []float64{0, 1, 2, 3, 4, 5, 6, 8, 12, 25, 50},
		}, []string{"label"}),
		pendingAge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pending_age_seconds",
			Help:      "Age of the unresolved lifecycle's pending call (kind fill, since its first send) or cancellation (since cancellation began); refreshed on receipt polls, absent when nothing is pending.",
		}, []string{"label", "kind"}),
		attemptTip: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "attempt_tip_wei",
			Help:      "Priority fee per gas of the latest signed attempt, by kind (fill or cancellation).",
		}, []string{"label", "kind"}),
		attemptMaxFee: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "attempt_max_fee_wei",
			Help:      "Fee cap per gas of the latest signed attempt, by kind (fill or cancellation).",
		}, []string{"label", "kind"}),
		attemptGasLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "attempt_gas_limit",
			Help:      "Gas limit of the latest signed attempt, by kind (fill or cancellation).",
		}, []string{"label", "kind"}),
		attemptHorizon: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "attempt_horizon_blocks",
			Help:      "Blocks from the next one an initial attempt's fee cap stays valid at the floor tip, when the horizon policy or the balance guard priced it.",
			Buckets:   []float64{0, 1, 2, 3, 4, 5, 6, 8, 12, 32},
		}, []string{"label"}),
		nextBaseFee: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "fee_next_base_fee_wei",
			Help:      "Base fee of the next block from the latest fee snapshot a send was priced or guarded at, the funding gate's account poll read, or the shadow evaluator took at a new head; absent until the first such reading.",
		}, []string{}),
		requiredBalance: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "account_required_balance_wei",
			Help:      "Balance a reference fill needs at the latest next base fee (fee_next_base_fee_wei, refreshed with it) and the floor tip: min (fees.minHorizonBlocks, can send), quote (fees.pricingHorizonBlocks, funding gate) or full (fees.maxHorizonBlocks).",
		}, []string{"horizon"}),
		gasEstimates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "gas_estimates_total",
			Help:      "Gas estimates of new attempts and horizon re-estimates of pending ones, by mode (latest, next_block, unconfirmed, or fallback when next-block estimates are unavailable) and outcome (ok, revert, unsupported, block_not_found, error).",
		}, []string{"label", "mode", "outcome"}),
		gasEstimateDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "gas_estimate_duration_seconds",
			Help:      "Duration of one gas estimate RPC by mode, bounded by gas.estimateTimeoutMs under the horizon policy.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 5, 10},
		}, []string{"mode"}),
		repricings: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "repricings_total",
			Help:      "Fee-changing same-nonce replacements the horizon policy's pending evaluation signed, by reason: validity, congestion, stall, gas (a stall re-estimate above the gas limit) or fallback (the timed cached bump after evaluation reads failed).",
		}, []string{"label", "reason"}),
		rebroadcasts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "rebroadcasts_total",
			Help:      "Exact rebroadcasts of already signed attempts the endpoint accepted or already knew, by reason: stall, reorg, uncertain (an ambiguous broadcast) or capped (no fundable replacement under the cap).",
		}, []string{"label", "reason"}),
		tipPaidWei: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "tip_paid_wei_total",
			Help:      "Priority fees paid by mined transactions: receipt gas used times the landed attempt's tip cap, an upper bound of the tip actually paid when the fee cap set the price.",
		}, []string{"label", "outcome"}),
		shadowLifecycles: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "shadow_lifecycles_total",
			Help:      "Virtual fills the shadow evaluator scored, one per fee policy per head: first1, first2 or first3 (the first attempt would have been included that many blocks after its head), replaced (included within 3 blocks after a replacement), missed3 (not included within 3 blocks) or refused (the policy would not have signed it).",
		}, []string{"policy", "outcome"}),
		feePolicy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "fee_policy_info",
			Help:      "Constant 1 labelled with the fee policy this process signs with (fees.policy: legacy or horizon), from startup; joined on the scrape target labels it selects the shadow outcomes of the policy in use.",
		}, []string{"policy"}),
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
		m.firstAttempts,
		m.inclusionDelay,
		m.pendingAge,
		m.attemptTip,
		m.attemptMaxFee,
		m.attemptGasLimit,
		m.attemptHorizon,
		m.nextBaseFee,
		m.requiredBalance,
		m.gasEstimates,
		m.gasEstimateDuration,
		m.repricings,
		m.rebroadcasts,
		m.tipPaidWei,
		m.shadowLifecycles,
		m.feePolicy,
		m.account,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("txmanager: register metric: %w", err)
		}
	}
	return m, nil
}

// guardRefusalReasons are the admission rejections recorded after the worker admitted a request. Each
// lifecycle starts them at zero for its label, so once a label has reached the worker its later
// refusals are increases that increase() and rate() see. Labels are not known before their first
// request, so a refusal of a label's very first lifecycle after a restart creates its series at 1 in
// the same worker iteration, which no scrape sees as an increase; its Info refusal log records it.
var guardRefusalReasons = [...]admissionRejectionReason{
	admissionRejectionUnaffordable,
	admissionRejectionUnaffordableOneBlock,
	admissionRejectionStaleHead,
	admissionRejectionFeeCeiling,
}

// repricingReasons and rebroadcastReasons start at zero for every label that reaches the worker, like
// guardRefusalReasons, so the first stall or gas repricing of a label is an increase the alerts see. So do
// the first-attempt outcomes and the confirmed spend series (see beginLifecycle).
var (
	repricingReasons   = [...]string{repriceValidity, repriceCongestion, repriceStall, repriceGas, repriceFallback}
	rebroadcastReasons = [...]string{rebroadcastStall, rebroadcastReorg, rebroadcastUncertain, rebroadcastCapped}
)

// firstAttemptOutcomes are the values of first_attempt_total{outcome}.
var firstAttemptOutcomes = [...]firstAttemptOutcome{
	firstAttemptFirst, firstAttemptLate, firstAttemptReplaced, firstAttemptCancelled,
}

// beginLifecycle starts a lifecycle's observation. It first creates, at zero, the label's series whose first
// increment the dashboards and alerts must see: a series created at its first increment is invisible to
// increase(). That matters most for first_attempt_total, whose non-first outcomes are rare enough to be the
// first of their series on a new pod, and which the 28-day first-attempt ratio sums. The confirmed spend series
// (requests, receipt gas, fee and tip paid) start together, so the spend-per-fill and tip-share ratios count a
// pod's first confirmed fill in numerator and denominator alike. Only a label's very first lifecycle on a pod
// can still go uncounted, when it completes before any scrape saw the zeros.
func (m *Metrics) beginLifecycle(label string) lifecycleObservation {
	if m == nil {
		return lifecycleObservation{}
	}
	for _, reason := range guardRefusalReasons {
		m.admissionRejections.WithLabelValues(label, string(reason))
	}
	for _, reason := range repricingReasons {
		m.repricings.WithLabelValues(label, reason)
	}
	for _, reason := range rebroadcastReasons {
		m.rebroadcasts.WithLabelValues(label, reason)
	}
	for _, outcome := range firstAttemptOutcomes {
		m.firstAttempts.WithLabelValues(label, string(outcome), simulationUnknown)
	}
	confirmed := string(OutcomeConfirmed)
	m.requests.WithLabelValues(label, confirmed)
	m.gasUsed.WithLabelValues(label, confirmed)
	m.feePaidWei.WithLabelValues(label, confirmed)
	m.tipPaidWei.WithLabelValues(label, confirmed)
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

// repricing counts a fee-changing replacement signed for reason.
func (m *Metrics) repricing(label, reason string) {
	if m != nil {
		m.repricings.WithLabelValues(label, reason).Inc()
	}
}

// rebroadcast counts an exact rebroadcast of a signed attempt the endpoint accepted or already knew.
func (m *Metrics) rebroadcast(label, reason string) {
	if m != nil {
		m.rebroadcasts.WithLabelValues(label, reason).Inc()
	}
}

// guardRefusal counts a request the balance guard refused before signing.
func (m *Metrics) guardRefusal(label string, reason admissionRejectionReason) {
	if m != nil {
		m.admissionRejections.WithLabelValues(label, string(reason)).Inc()
	}
}

// observeAttempt records the fees and gas limit of a signed attempt.
func (m *Metrics) observeAttempt(label string, cancellation bool, fees feeQuote, gas uint64) {
	if m == nil {
		return
	}
	kind := attemptKind(cancellation)
	m.attemptTip.WithLabelValues(label, kind).Set(weiFloat(fees.tip))
	m.attemptMaxFee.WithLabelValues(label, kind).Set(weiFloat(fees.maxFee))
	m.attemptGasLimit.WithLabelValues(label, kind).Set(float64(gas))
}

func (m *Metrics) observeHorizon(label string, blocks uint64) {
	if m != nil {
		m.attemptHorizon.WithLabelValues(label).Observe(float64(blocks))
	}
}

// laneRequirements is what a reference fill needs at one next base fee, by horizon.
type laneRequirements struct {
	min, quote, full *big.Int
}

func (m *Metrics) observeFeeSnapshot(nextBase *big.Int, required laneRequirements) {
	if m == nil {
		return
	}
	m.observeNextBaseFee(nextBase)
	m.requiredBalance.WithLabelValues(requiredBalanceMin).Set(weiFloat(required.min))
	m.requiredBalance.WithLabelValues(requiredBalanceQuote).Set(weiFloat(required.quote))
	m.requiredBalance.WithLabelValues(requiredBalanceFull).Set(weiFloat(required.full))
}

// observeNextBaseFee records the next block's base fee from a fee reading.
func (m *Metrics) observeNextBaseFee(nextBase *big.Int) {
	if m != nil {
		m.nextBaseFee.WithLabelValues().Set(weiFloat(nextBase))
	}
}

// observeGasEstimate counts one gas estimate and records how long it took.
func (m *Metrics) observeGasEstimate(label, mode, outcome string, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.gasEstimates.WithLabelValues(label, mode, outcome).Inc()
	m.gasEstimateDuration.WithLabelValues(mode).Observe(elapsed.Seconds())
}

// observePendingAge refreshes the age of the unresolved lifecycle's pending call or cancellation. Only
// one kind is exported per label at a time.
func (m *Metrics) observePendingAge(label string, cancellation bool, age time.Duration) {
	if m == nil {
		return
	}
	m.pendingAge.WithLabelValues(label, attemptKind(cancellation)).Set(age.Seconds())
	m.pendingAge.DeleteLabelValues(label, attemptKind(!cancellation))
}

func (m *Metrics) clearPendingAge(label string) {
	if m != nil {
		m.pendingAge.DeleteLabelValues(label, attemptKindFill)
		m.pendingAge.DeleteLabelValues(label, attemptKindCancellation)
	}
}

// observeInclusion records how a lifecycle that reached the chain landed. delay is the inclusion delay
// in blocks, observed only for a landed call rather than a cancellation.
func (m *Metrics) observeInclusion(label string, outcome firstAttemptOutcome, delay uint64) {
	if m == nil {
		return
	}
	m.firstAttempts.WithLabelValues(label, string(outcome), simulationUnknown).Inc()
	if outcome != firstAttemptCancelled {
		m.inclusionDelay.WithLabelValues(label).Observe(float64(delay))
	}
}

func attemptKind(cancellation bool) string {
	if cancellation {
		return attemptKindCancellation
	}
	return attemptKindFill
}

func weiFloat(value *big.Int) float64 {
	if value == nil {
		return 0
	}
	f, _ := new(big.Float).SetInt(value).Float64()
	return f
}

// startShadow creates every shadow_lifecycles_total series at zero, so the first outcome of each kind after a
// restart is an increase that increase() and rate() see.
func (m *Metrics) startShadow() {
	if m == nil {
		return
	}
	for _, policy := range shadowPolicies {
		for _, outcome := range shadowOutcomes {
			m.shadowLifecycles.WithLabelValues(string(policy), string(outcome))
		}
	}
}

// setFeePolicy exports the fee policy the manager signs with as the one series of fee_policy_info.
func (m *Metrics) setFeePolicy(policy FeePolicy) {
	if m == nil {
		return
	}
	m.feePolicy.Reset()
	m.feePolicy.WithLabelValues(string(policy)).Set(1)
}

// shadowLifecycle counts one scored virtual fill.
func (m *Metrics) shadowLifecycle(policy FeePolicy, outcome shadowOutcome) {
	if m != nil {
		m.shadowLifecycles.WithLabelValues(string(policy), string(outcome)).Inc()
	}
}

// observeTipPaid counts the priority fee a mined lifecycle paid: the receipt's gas used times the landed
// attempt's effective tip (see paidTip).
func (m *Metrics) observeTipPaid(label string, outcome Outcome, receipt *types.Receipt, landed *types.Transaction) {
	if m == nil {
		return
	}
	tip, ok := paidTip(landed, receipt)
	if !ok {
		return
	}
	paid := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), tip)
	m.tipPaidWei.WithLabelValues(label, string(outcome)).Add(weiFloat(paid))
}

// paidTip is the priority fee per gas a mined EIP-1559 transaction paid, from what the receipt carries
// without its block's base fee: effectiveGasPrice = min(feeCap, baseFee + tipCap), so below the fee cap the
// tip cap was paid in full, and at the fee cap the paid tip, feeCap − baseFee, is at most the tip cap, which
// is then reported as an upper bound. It reports false without a transaction or an effective gas price.
func paidTip(tx *types.Transaction, receipt *types.Receipt) (*big.Int, bool) {
	if tx == nil || receipt == nil || receipt.EffectiveGasPrice == nil || receipt.EffectiveGasPrice.Sign() < 0 {
		return nil, false
	}
	tip := new(big.Int).Set(tx.GasTipCap())
	if tip.Cmp(receipt.EffectiveGasPrice) > 0 {
		tip.Set(receipt.EffectiveGasPrice)
	}
	return tip, true
}
