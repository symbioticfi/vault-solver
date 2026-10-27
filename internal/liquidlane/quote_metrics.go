package liquidlane

import (
	"math"
	"math/big"
	"slices"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// Diagnostic budget, not a route limit. Request data never adds registry entries.
const maxQuoteMetricTokens = 16

type quoteMetricToken struct {
	label    string
	decimals int
}

// QuoteMetricRequest contains only diagnostic fields, never protocol credentials or inventory.
type QuoteMetricRequest struct {
	TokenIn         string
	TokenOut        string
	TokenInChainID  int64
	TokenOutChainID int64
	Amount          string
	Side            string // input or output; empty means the quote type was not recognized.
}

// QuoteMetrics is shared by HTTP LiquidLane solvers. Initialize once before serving requests;
// afterwards token metadata is immutable. Prometheus collectors synchronize their own scrapes.
// Each Observe call is one final HTTP result; sum results to obtain incoming request count.
type QuoteMetrics struct {
	chainID     int64
	once        sync.Once
	tokens      map[common.Address]quoteMetricToken
	results     *prometheus.CounterVec
	amounts     *prometheus.HistogramVec
	overflow    prometheus.Gauge
	boundReason func(string) string
}

// NewQuoteMetrics registers the common transport counters and trusted-unit histogram.
func NewQuoteMetrics(reg prometheus.Registerer, solver string, boundReason func(string) string) (*QuoteMetrics, error) {
	m := &QuoteMetrics{
		boundReason: boundReason,
		results:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "solver_bot_quote_results_total", Help: "One final HTTP quote result; sum results for incoming requests."}, []string{"token_in", "token_out", "result", "reason"}),
		amounts:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "solver_bot_quote_requested_amount_tokens", Help: "Requested positive uint256 amount in trusted token units; unknown metadata is omitted.", Buckets: []float64{0.001, 0.01, 0.1, 1, 10, 100, 1_000, 10_000, 100_000, 1_000_000}}, []string{"token", "side"}),
		overflow:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "solver_bot_quote_token_overflow", Help: "Trusted startup token identities beyond the diagnostic limit, aggregated as other."}),
	}
	wrapped := prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver}, reg)
	for _, collector := range []prometheus.Collector{m.results, m.amounts, m.overflow} {
		if err := wrapped.Register(collector); err != nil {
			return nil, errors.Errorf("register quote metrics: %w", err)
		}
	}
	return m, nil
}

// Initialize freezes trusted identity/decimal metadata once, before HTTP handlers start.
func (m *QuoteMetrics) Initialize(chainID int64, routes []Route, configured map[common.Address]bool) {
	m.once.Do(func() {
		m.chainID = chainID
		metadata := make(map[common.Address]int)
		add := func(token common.Address, decimals int) {
			if token == (common.Address{}) {
				return
			}
			if decimals < 0 || decimals > 255 {
				decimals = -1
			}
			if previous, exists := metadata[token]; exists && previous != decimals {
				decimals = -1
			}
			metadata[token] = decimals
		}
		for _, route := range routes {
			add(route.TokenIn, route.TokenInDecimals)
			add(route.TokenOut, route.TokenOutDecimals)
		}

		// Configured permissioned inputs remain identifiable even with no initial liquidity.
		// Missing decimal metadata disables only the optional amount histogram.
		for token := range configured {
			if _, exists := metadata[token]; !exists && token != (common.Address{}) {
				metadata[token] = -1
			}
		}
		addresses := make([]common.Address, 0, len(metadata))
		for address := range metadata {
			addresses = append(addresses, address)
		}
		slices.SortFunc(addresses, func(a, b common.Address) int {
			if configured[a] != configured[b] {
				if configured[a] {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Hex(), b.Hex())
		})
		m.tokens = make(map[common.Address]quoteMetricToken)
		overflow := 0
		for i, token := range addresses {
			if i >= maxQuoteMetricTokens {
				overflow++
				continue
			}
			m.tokens[token] = quoteMetricToken{label: strings.ToLower(token.Hex()), decimals: metadata[token]}
		}
		m.overflow.Set(float64(overflow))
	})
}

func (m *QuoteMetrics) token(value string, chainID int64) quoteMetricToken {
	if !common.IsHexAddress(value) {
		return quoteMetricToken{label: "unknown", decimals: -1}
	}
	if chainID == m.chainID {
		if token, known := m.tokens[common.HexToAddress(value)]; known {
			return token
		}
	}
	return quoteMetricToken{label: "other", decimals: -1}
}

// Observe records exactly one terminal transport result. The caller provides a bounded
// integration-owned reason classifier; arbitrary request/error strings never become labels.
func (m *QuoteMetrics) Observe(request QuoteMetricRequest, returned bool, reason string) {
	if m == nil {
		return
	}
	in, out := m.token(request.TokenIn, request.TokenInChainID), m.token(request.TokenOut, request.TokenOutChainID)
	m.observeAmount(request, in, out)
	result := "rejected"
	reason = m.boundReason(reason)
	if returned {
		result, reason = "returned", "none"
	}
	m.results.WithLabelValues(in.label, out.label, result, reason).Inc()
}

func (m *QuoteMetrics) observeAmount(request QuoteMetricRequest, in, out quoteMetricToken) {
	token := in
	switch request.Side {
	case "input":
	case "output":
		token = out
	default:
		return
	}
	if token.decimals < 0 {
		return
	}
	// uint256 has at most 78 decimal digits. Bound work before parsing untrusted input,
	// including requests rejected by auth; amounts with excessive leading zeros are omitted.
	if len(request.Amount) > 78 {
		return
	}
	amount, ok := new(big.Int).SetString(request.Amount, 10)
	if !ok || amount.Sign() <= 0 || amount.BitLen() > 256 {
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	m.amounts.WithLabelValues(token.label, request.Side).Observe(value / math.Pow10(token.decimals))
}
