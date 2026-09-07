package chain

import (
	"strconv"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type rpcOutcome string

const (
	rpcOutcomeSuccess          rpcOutcome = "success"
	rpcOutcomeRPCError         rpcOutcome = "rpc_error"
	rpcOutcomeHTTP3xx          rpcOutcome = "http_3xx"
	rpcOutcomeHTTP4xx          rpcOutcome = "http_4xx"
	rpcOutcomeHTTP5xx          rpcOutcome = "http_5xx"
	rpcOutcomeRateLimited      rpcOutcome = "rate_limited"
	rpcOutcomeNullResult       rpcOutcome = "null_result"
	rpcOutcomeTransportError   rpcOutcome = "transport_error"
	rpcOutcomeDecodeError      rpcOutcome = "decode_error"
	rpcOutcomeContextCanceled  rpcOutcome = "context_canceled"
	rpcOutcomeDeadlineExceeded rpcOutcome = "deadline_exceeded"
)

const (
	rpcRoleRead   = "read"
	rpcRoleWrite  = "write"
	rpcRoleShared = "shared"
)

// RPCMetrics records generic HTTP JSON-RPC traffic. Endpoint labels are role-local ordinal indexes,
// never configured URLs; errors are mapped to bounded outcomes rather than exposed as label text.
type RPCMetrics struct {
	requests              *prometheus.CounterVec
	attempts              *prometheus.CounterVec
	inflight              *prometheus.GaugeVec
	requestDuration       *prometheus.HistogramVec
	lastSuccessfulRequest *prometheus.GaugeVec
	lastSuccessfulAttempt *prometheus.GaugeVec
	now                   func() time.Time
}

// NewRPCMetrics registers generic chain-client collectors.
func NewRPCMetrics(reg prometheus.Registerer) (*RPCMetrics, error) {
	if reg == nil {
		return nil, errors.New("chain: RPC metrics registerer is required")
	}
	group := observability.NewMetricGroup("solver_bot_rpc_")
	m := &RPCMetrics{
		requests:              group.Counter("requests_total", "Logical HTTP JSON-RPC requests by endpoint role, bounded method, and outcome.", "role", "method", "outcome"),
		attempts:              group.Counter("attempts_total", "HTTP JSON-RPC endpoint attempts; endpoint is a role-local ordinal, never a URL.", "role", "endpoint", "method", "outcome"),
		inflight:              group.Gauge("inflight", "Logical HTTP JSON-RPC requests whose response body has not completed.", "role"),
		requestDuration:       group.Histogram("request_duration_seconds", "Logical HTTP JSON-RPC duration through response-body consumption.", []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60}, "role", "method", "outcome"),
		lastSuccessfulRequest: group.Gauge("last_successful_request_timestamp", "Unix timestamp of the last successful logical HTTP JSON-RPC request by role.", "role"),
		lastSuccessfulAttempt: group.Gauge("last_successful_attempt_timestamp", "Unix timestamp of the last successful HTTP JSON-RPC attempt by role and endpoint ordinal.", "role", "endpoint"),
		now:                   time.Now,
	}
	if err := group.Publish(reg); err != nil {
		return nil, err
	}
	return m, nil
}

func endpointLabel(endpoint int) string { return strconv.Itoa(endpoint) }

func (m *RPCMetrics) bindTransport(role string, endpointCount int) {
	if m == nil {
		return
	}
	m.inflight.WithLabelValues(role).Set(0)
	m.lastSuccessfulRequest.WithLabelValues(role).Set(0)
	for endpoint := range endpointCount {
		m.lastSuccessfulAttempt.WithLabelValues(role, endpointLabel(endpoint)).Set(0)
	}
}

// The transport receives only a completion function; metrics retain no request
// object and a body-close/read/error race can finish the sample exactly once.
func (m *RPCMetrics) beginRequest(role, method string) func(rpcOutcome) {
	if m == nil {
		return func(rpcOutcome) {}
	}
	started := m.now()
	inflight := m.inflight.WithLabelValues(role)
	inflight.Inc()
	var once sync.Once
	return func(outcome rpcOutcome) {
		once.Do(func() {
			at := m.now()
			m.requests.WithLabelValues(role, method, string(outcome)).Inc()
			m.requestDuration.WithLabelValues(role, method, string(outcome)).Observe(at.Sub(started).Seconds())
			if outcome == rpcOutcomeSuccess {
				m.lastSuccessfulRequest.WithLabelValues(role).Set(float64(at.Unix()))
			}
			inflight.Dec()
		})
	}
}

func (m *RPCMetrics) observeAttempt(role, endpoint, method string, outcome rpcOutcome) {
	if m == nil {
		return
	}
	m.attempts.WithLabelValues(role, endpoint, method, string(outcome)).Inc()
	if outcome == rpcOutcomeSuccess {
		m.lastSuccessfulAttempt.WithLabelValues(role, endpoint).Set(float64(m.now().Unix()))
	}
}
