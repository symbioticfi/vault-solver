package chain

import (
	"strconv"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type rpcOutcome string

const (
	rpcOutcomeSuccess          rpcOutcome = "success"
	rpcOutcomeRPCError         rpcOutcome = "rpc_error"
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
	m := &RPCMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "requests_total",
			Help:      "Logical HTTP JSON-RPC requests by endpoint role, bounded method, and outcome.",
		}, []string{"role", "method", "outcome"}),
		attempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "attempts_total",
			Help:      "HTTP JSON-RPC endpoint attempts; endpoint is a role-local ordinal, never a URL.",
		}, []string{"role", "endpoint", "method", "outcome"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "inflight",
			Help:      "Logical HTTP JSON-RPC requests whose response body has not completed.",
		}, []string{"role"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "request_duration_seconds",
			Help:      "Logical HTTP JSON-RPC duration through response-body consumption.",
			Buckets:   []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60},
		}, []string{"role", "method", "outcome"}),
		lastSuccessfulRequest: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "last_successful_request_timestamp",
			Help:      "Unix timestamp of the last successful logical HTTP JSON-RPC request by role.",
		}, []string{"role"}),
		lastSuccessfulAttempt: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "solver_bot",
			Subsystem: "rpc",
			Name:      "last_successful_attempt_timestamp",
			Help:      "Unix timestamp of the last successful HTTP JSON-RPC attempt by role and endpoint ordinal.",
		}, []string{"role", "endpoint"}),
		now: time.Now,
	}
	for _, collector := range []prometheus.Collector{
		m.requests, m.attempts, m.inflight, m.requestDuration,
		m.lastSuccessfulRequest, m.lastSuccessfulAttempt,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, errors.Errorf("chain: register RPC metric: %w", err)
		}
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

func (m *RPCMetrics) beginRequest(role, method string) *rpcRequestObservation {
	if m == nil {
		return nil
	}
	m.inflight.WithLabelValues(role).Inc()
	return &rpcRequestObservation{metrics: m, role: role, method: method, started: m.now()}
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

type rpcRequestObservation struct {
	metrics *RPCMetrics
	role    string
	method  string
	started time.Time
	once    sync.Once
}

func (o *rpcRequestObservation) finish(outcome rpcOutcome) {
	if o == nil {
		return
	}
	o.once.Do(func() {
		now := o.metrics.now()
		o.metrics.requests.WithLabelValues(o.role, o.method, string(outcome)).Inc()
		o.metrics.inflight.WithLabelValues(o.role).Dec()
		o.metrics.requestDuration.WithLabelValues(o.role, o.method, string(outcome)).Observe(now.Sub(o.started).Seconds())
		if outcome == rpcOutcomeSuccess {
			o.metrics.lastSuccessfulRequest.WithLabelValues(o.role).Set(float64(now.Unix()))
		}
	})
}
