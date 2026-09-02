package bridgefacilitator

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

const (
	threeFStateOffers             = "offers"
	threeFStateActiveRequests     = "active_requests"
	threeFStateRedeemable         = "redeemable"
	threeFStateTargets            = "targets"
	threeFEventOffer              = "offer"
	threeFEventRedeem             = "redeem"
	threeFOfferPrincipal          = "principal"
	threeFOfferExpectedYield      = "expected_yield"
	targetRefreshOperation        = "target_refresh"
	offerRefreshOperation         = "offer_refresh"
	activeRequestRefreshOperation = "active_request_refresh"
	redeemableRefreshOperation    = "redeemable_refresh"
)

type threeFOperationObservers struct {
	targetRefresh        *observability.OperationObserver
	offerRefresh         *observability.OperationObserver
	activeRequestRefresh *observability.OperationObserver
	redeemableRefresh    *observability.OperationObserver
}

type threeFMetrics struct {
	workflow             *observability.WorkflowMetrics
	operations           threeFOperationObservers
	backlogNonemptySince *prometheus.GaugeVec
	backlogNonempty      map[string]bool
	now                  func() time.Time
}

func newThreeFMetrics(reg prometheus.Registerer, strategyName string) (*threeFMetrics, error) {
	spec := observability.WorkflowSpec{
		Strategy: strategyName,
		Operations: []string{
			targetRefreshOperation, offerRefreshOperation,
			activeRequestRefreshOperation, redeemableRefreshOperation,
		},
		Events: []observability.WorkflowEventSpec{
			{Event: threeFEventOffer, Outcomes: []string{"success", "error"}},
			{Event: threeFEventRedeem, Outcomes: []string{"success"}},
		},
		Amounts: []observability.WorkflowAmountSpec{{
			Event: threeFEventOffer, Kinds: []string{threeFOfferPrincipal, threeFOfferExpectedYield},
		}},
		States: []string{
			threeFStateOffers, threeFStateActiveRequests, threeFStateRedeemable, threeFStateTargets,
		},
	}
	workflow, err := observability.NewWorkflowMetrics(reg, Name, spec)
	if err != nil {
		return nil, err
	}
	m := &threeFMetrics{
		workflow: workflow,
		operations: threeFOperationObservers{
			targetRefresh:        workflow.Operation(targetRefreshOperation),
			offerRefresh:         workflow.Operation(offerRefreshOperation),
			activeRequestRefresh: workflow.Operation(activeRequestRefreshOperation),
			redeemableRefresh:    workflow.Operation(redeemableRefreshOperation),
		},
		backlogNonemptySince: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "threef_backlog_nonempty_since_timestamp",
			Help: "Unix timestamp when this process first observed a continuous non-empty 3F backlog in complete authoritative snapshots by view; 0 before the first authoritative non-empty observation or after an authoritative empty snapshot. Pair with solver_bot_workflow_last_observation_timestamp; resets on process restart; not an item age.",
		}, []string{"view"}),
		backlogNonempty: make(map[string]bool, 2),
		now:             time.Now,
	}
	if err := reg.Register(m.backlogNonemptySince); err != nil {
		return nil, errors.Errorf("bridgefacilitator: register metric: %w", err)
	}
	for _, view := range []string{threeFStateActiveRequests, threeFStateRedeemable} {
		m.backlogNonemptySince.WithLabelValues(view)
	}
	return m, nil
}

func (s *Solver) observeOfferSubmission(result string) {
	if s.metrics == nil {
		return
	}
	if result != "success" {
		result = "error"
	}
	s.metrics.workflow.ObserveEventAt(threeFEventOffer, result, 1, s.metrics.now())
}

func (s *Solver) observeSubmittedOffer(token common.Address, principal, expectedYield *big.Int) {
	if s.metrics == nil {
		return
	}
	s.observeOfferSubmission("success")
	if token == (common.Address{}) {
		return
	}
	s.metrics.workflow.AddAmount(threeFEventOffer, token.Hex(), threeFOfferPrincipal, principal)
	s.metrics.workflow.AddAmount(threeFEventOffer, token.Hex(), threeFOfferExpectedYield, expectedYield)
}

func (s *Solver) observeRedeemedRequests(count int) {
	if s.metrics != nil && count > 0 {
		s.metrics.workflow.ObserveEventAt(
			threeFEventRedeem, "success", float64(count), s.metrics.now(),
		)
	}
}

func (s *Solver) observeState(view string, count int) {
	if s.metrics != nil {
		s.metrics.workflow.ObserveStateAt(view, count, s.metrics.now())
	}
}

// observeBacklog tracks only the process-local continuous non-empty condition. It deliberately does
// not estimate when any individual Request became active or redeemable; the current on-chain views do
// not expose those transition timestamps.
func (m *threeFMetrics) observeBacklog(view string, count int) {
	if view != threeFStateActiveRequests && view != threeFStateRedeemable {
		return
	}
	gauge := m.backlogNonemptySince.WithLabelValues(view)
	if count <= 0 {
		m.backlogNonempty[view] = false
		gauge.Set(0)
		return
	}
	if m.backlogNonempty[view] {
		return
	}
	m.backlogNonempty[view] = true
	gauge.Set(float64(m.now().Unix()))
}

// observeTargetDerivedState publishes only observations that cover both the complete target source
// and the complete local scan. Runtime work may still use a safe subset from a partial target refresh.
func (s *Solver) observeTargetDerivedState(view string, count int, scanComplete bool) {
	if !s.targetsAuthoritative || !scanComplete {
		return
	}
	s.observeState(view, count)
	if s.metrics != nil {
		s.metrics.observeBacklog(view, count)
	}
}
