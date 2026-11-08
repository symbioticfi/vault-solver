package liquidlane

import (
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// PairMetrics records observations only after the caller proves a directed route from trusted
// configuration/adapter inventory and policy. It is not a support registry. Series grow with the
// supported route set and bounded event/reason vocabulary, never with request/order identities.
// Counters are retained across inventory refreshes; removed routes are not reset.
type PairMetrics struct {
	events      *prometheus.CounterVec
	boundReason func(string) string
}

func NewPairMetrics(reg prometheus.Registerer, solver string, boundReason func(string) string) (*PairMetrics, error) {
	m := &PairMetrics{boundReason: boundReason, events: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "solver_bot_pair_events_total",
		Help: "Observed supported directed-route events. Quotes are final HTTP results; assignments are observed awards/admissions; fills are successful owned mined receipts; standing quotes are acknowledged publication changes or local rejection attempts. Not unique upstream LI.FI client responses or wins.",
	}, []string{"chain_id", "token_in", "token_out", "event", "result", "reason"})}
	if err := prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver}, reg).Register(m.events); err != nil {
		return nil, errors.Errorf("register pair metrics: %w", err)
	}
	return m, nil
}

func (m *PairMetrics) Quote(chainID int64, in, out common.Address, returned bool, reason string) {
	result := "rejected"
	if returned {
		result, reason = "returned", "none"
	} else if m != nil {
		reason = m.boundReason(reason)
	}
	m.observe(chainID, in, out, "quote", result, reason)
}

// Assignment uses integration-specific proven selection boundaries, not transaction submission.
func (m *PairMetrics) Assignment(chainID int64, in, out common.Address, kind string) {
	if kind != "observed" && kind != "admitted" {
		return
	}
	m.observe(chainID, in, out, "assignment", kind, "none")
}

func (m *PairMetrics) Standing(chainID int64, in, out common.Address, result, reason string) {
	switch result {
	case "published", "withdrawn":
		reason = "none"
	case "rejected":
		if m != nil {
			reason = m.boundReason(reason)
		}
	default:
		return
	}
	m.observe(chainID, in, out, "standing_quote", result, reason)
}

// ObserveReceipt counts a successful canonical receipt returned to the worker.
func (m *PairMetrics) ObserveReceipt(chainID int64, in, out common.Address, receipt *types.Receipt) {
	if receipt != nil && receipt.Status == types.ReceiptStatusSuccessful {
		m.observe(chainID, in, out, "fill", "mined_success", "none")
	}
}

func (m *PairMetrics) observe(chainID int64, in, out common.Address, event, result, reason string) {
	if m == nil || chainID <= 0 || in == (common.Address{}) || out == (common.Address{}) || in == out {
		return
	}
	m.events.WithLabelValues(strconv.FormatInt(chainID, 10), strings.ToLower(in.Hex()), strings.ToLower(out.Hex()), event, result, reason).Inc()
}
