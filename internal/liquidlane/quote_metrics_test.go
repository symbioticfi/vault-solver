package liquidlane

import (
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func boundTestQuoteReason(reason string) string {
	if reason == "unsupported_protocol" {
		return reason
	}
	return "unknown"
}
func newTestQuoteMetrics(t *testing.T) (*QuoteMetrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m, err := NewQuoteMetrics(reg, "test", boundTestQuoteReason)
	if err != nil {
		t.Fatal(err)
	}
	return m, reg
}
func validQuoteMetricRequest(in, out common.Address) QuoteMetricRequest {
	return QuoteMetricRequest{TokenIn: in.Hex(), TokenOut: out.Hex(), TokenInChainID: 1, TokenOutChainID: 1, Amount: "100", Side: "input"}
}
func assertQuoteResultCount(t *testing.T, reg prometheus.Gatherer, want float64) {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, f := range families {
		if f.GetName() == "solver_bot_quote_results_total" {
			for _, m := range f.GetMetric() {
				total += m.GetCounter().GetValue()
			}
		}
	}
	if total != want {
		t.Fatalf("results=%f want=%f", total, want)
	}
}

func TestQuoteTokenRegistryIsFrozenDeterministicAndBounded(t *testing.T) {
	routes := make([]Route, 0, 24)
	for i := 1; i <= 24; i++ {
		routes = append(routes, Route{TokenIn: common.BigToAddress(big.NewInt(int64(i))), TokenInDecimals: 6})
	}
	a, _ := newTestQuoteMetrics(t)
	b, _ := newTestQuoteMetrics(t)
	a.Initialize(1, routes, nil)
	slices.Reverse(routes)
	b.Initialize(1, routes, nil)
	if len(a.tokens) != maxQuoteMetricTokens || testutil.ToFloat64(a.overflow) != 8 {
		t.Fatal("registry exceeds limit or omitted overflow")
	}
	for address := range a.tokens {
		if _, ok := b.tokens[address]; !ok {
			t.Fatal("order-dependent registry")
		}
	}
	a.Initialize(1, []Route{{TokenIn: common.HexToAddress("0xffff"), TokenInDecimals: 18}}, nil)
	if got := a.token(common.HexToAddress("0xffff").Hex(), 1); got.label != "other" {
		t.Fatal("registry grew after publication")
	}
	for address := range a.tokens {
		if got := a.token(address.Hex(), 2); got.label != "other" {
			t.Fatal("foreign-chain address used local identity")
		}
	}
}

func TestQuoteTrafficAmountsUseTrustedDecimalsAndExactSide(t *testing.T) {
	in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	metrics, reg := newTestQuoteMetrics(t)
	metrics.Initialize(1, []Route{{TokenIn: in, TokenOut: out, TokenInDecimals: 6, TokenOutDecimals: 18}}, nil)
	for _, tc := range []struct {
		mode, amount string
		chain        int64
	}{
		{"input", "2000000", 1}, {"output", "3000000000000000000", 1},
		{"input", "1000000", 2}, {"input", "bad", 1}, {"input", "-1", 1},
		{"input", new(big.Int).Lsh(big.NewInt(1), 257).String(), 1}, {"OTHER", "100", 1},
	} {
		r := validQuoteMetricRequest(in, out)
		r.Side, r.Amount, r.TokenInChainID = tc.mode, tc.amount, tc.chain
		metrics.Observe(r, false, "unsupported_protocol")
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var samples uint64
	var sum float64
	for _, f := range families {
		if f.GetName() != "solver_bot_quote_requested_amount_tokens" {
			continue
		}
		if len(f.GetMetric()) != 2 {
			t.Fatalf("histograms=%d want2", len(f.GetMetric()))
		}
		for _, m := range f.GetMetric() {
			samples += m.GetHistogram().GetSampleCount()
			sum += m.GetHistogram().GetSampleSum()
		}
	}
	if samples != 2 || sum != 5 {
		t.Fatalf("samples=%d sum=%f", samples, sum)
	}
}

func TestQuoteTrafficConflictingDecimalsDisableAmountOnly(t *testing.T) {
	in := common.HexToAddress("0x01")
	m, _ := newTestQuoteMetrics(t)
	m.Initialize(1, []Route{{TokenIn: in, TokenInDecimals: 6}, {TokenIn: in, TokenInDecimals: 18}}, nil)
	if token := m.token(in.Hex(), 1); token.label != strings.ToLower(in.Hex()) || token.decimals != -1 {
		t.Fatalf("token=%+v", token)
	}
}

func TestQuoteTrafficCardinalityUnderForeignRequests(t *testing.T) {
	metrics, reg := newTestQuoteMetrics(t)
	routes := make([]Route, 0, maxQuoteMetricTokens)
	for i := 1; i <= maxQuoteMetricTokens; i++ {
		routes = append(routes, Route{TokenIn: common.BigToAddress(big.NewInt(int64(i))), TokenInDecimals: 6})
	}
	metrics.Initialize(1, routes, nil)
	for i := 1; i <= 10_000; i++ {
		r := validQuoteMetricRequest(common.BigToAddress(big.NewInt(int64(i))), common.BigToAddress(big.NewInt(int64(i+1))))
		r.Amount = big.NewInt(int64(i)).String()
		metrics.Observe(r, false, "arbitrary error "+r.Amount)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "solver_bot_quote_results_total" {
			continue
		}
		if len(f.GetMetric()) > (maxQuoteMetricTokens+2)*(maxQuoteMetricTokens+2) {
			t.Fatal("foreign requests grew pair series beyond fixed budget")
		}
		for _, m := range f.GetMetric() {
			for _, label := range m.GetLabel() {
				switch label.GetName() {
				case "token_in", "token_out":
					v := label.GetValue()
					if v != "other" && v != "unknown" {
						if _, known := metrics.tokens[common.HexToAddress(v)]; !known {
							t.Fatalf("unbounded token=%s", v)
						}
					}
				case "solver":
					if label.GetValue() != "test" {
						t.Fatal("wrong solver")
					}
				case "reason":
					if label.GetValue() != "unknown" {
						t.Fatal("arbitrary reason label")
					}
				case "result":
					if label.GetValue() != "rejected" {
						t.Fatal("wrong result")
					}
				default:
					t.Fatalf("unexpected dimension=%s", label.GetName())
				}
			}
		}
	}
	assertQuoteResultCount(t, reg, 10_000)
}

func FuzzQuoteTrafficRegistry(f *testing.F) {
	f.Add("0x0000000000000000000000000000000000000001", "garbage", int64(1), "unbounded error")
	f.Fuzz(func(t *testing.T, in, out string, chain int64, reason string) {
		m, _ := newTestQuoteMetrics(t)
		m.Initialize(1, []Route{{TokenIn: common.HexToAddress("0x01"), TokenInDecimals: 6}}, nil)
		for _, v := range []string{in, out} {
			token := m.token(v, chain)
			if token.label != "other" && token.label != "unknown" && token.label != "0x0000000000000000000000000000000000000001" {
				t.Fatalf("unbounded label=%s", token.label)
			}
		}
		if bounded := boundTestQuoteReason(reason); len(bounded) > 64 {
			t.Fatal("unbounded reason")
		}
	})
}

func TestQuoteRegistryPreservesConfiguredInputsWithinBudget(t *testing.T) {
	configured := make(map[common.Address]bool)
	for i := int64(1); i <= 14; i++ {
		configured[common.BigToAddress(big.NewInt(i))] = true
	}
	asset := common.HexToAddress("0x1000")
	m, _ := newTestQuoteMetrics(t)
	// No initial input inventory: policy still identifies all 14 input addresses.
	m.Initialize(1, []Route{{TokenOut: asset, TokenOutDecimals: 6}}, configured)
	if len(m.tokens) != 15 || testutil.ToFloat64(m.overflow) != 0 {
		t.Fatalf("registry=%+v", m.tokens)
	}
	for token := range configured {
		if got := m.token(token.Hex(), 1); got.label == "other" || got.decimals != -1 {
			t.Fatalf("configured input lost or invented decimals: %+v", got)
		}
	}
	if m.token(asset.Hex(), 1).decimals != 6 {
		t.Fatal("trusted output decimals lost")
	}
	// One more trusted vault asset fits; a 17th is explicitly surfaced as overflow.
	extra := common.HexToAddress("0x9999")
	second, _ := newTestQuoteMetrics(t)
	second.Initialize(1, []Route{{TokenOut: asset, TokenOutDecimals: 6}, {TokenOut: extra, TokenOutDecimals: 18}}, configured)
	if len(second.tokens) != 16 || testutil.ToFloat64(second.overflow) != 0 || second.token(asset.Hex(), 1).label == "other" || second.token(extra.Hex(), 1).label == "other" {
		t.Fatal("14 configured inputs plus two trusted outputs did not fit")
	}
	overflow, _ := newTestQuoteMetrics(t)
	overflow.Initialize(1, []Route{{TokenOut: asset, TokenOutDecimals: 6}, {TokenOut: extra, TokenOutDecimals: 18}, {TokenOut: common.HexToAddress("0xaaaa"), TokenOutDecimals: 18}}, configured)
	if len(overflow.tokens) != 16 || testutil.ToFloat64(overflow.overflow) != 1 {
		t.Fatal("overflow budget not surfaced")
	}
	for token := range configured {
		if overflow.token(token.Hex(), 1).label == "other" {
			t.Fatal("trusted extra assets displaced configured inputs")
		}
	}
}

func TestQuoteMetricsShareRegistryAcrossSolvers(t *testing.T) {
	reg := prometheus.NewRegistry()
	for _, name := range []string{"uniswapx-filler", "rfq-filler"} {
		m, err := NewQuoteMetrics(reg, name, boundTestQuoteReason)
		if err != nil {
			t.Fatal(err)
		}
		m.Initialize(1, nil, nil)
		m.Observe(QuoteMetricRequest{}, name == "rfq-filler", "unsupported_protocol")
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != "solver_bot_quote_results_total" {
			continue
		}
		if len(family.GetMetric()) != 2 {
			t.Fatalf("solvers merged: %v", family)
		}
		for _, sample := range family.GetMetric() {
			if sample.GetCounter().GetValue() != 1 {
				t.Fatal("cross-solver double count")
			}
		}
	}
}

func TestQuoteTrafficAmountParsingIsBoundedToUint256Digits(t *testing.T) {
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, tc := range []struct {
		name, amount string
		want         uint64
	}{
		{"maximum", maximum.String(), 1},
		{"overflow at 78 digits", new(big.Int).Add(maximum, big.NewInt(1)).String(), 0},
		{"79 digits", strings.Repeat("9", 79), 0},
		{"long zero prefix", strings.Repeat("0", 100_000) + "1", 0},
		{"large unauthenticated shape", strings.Repeat("9", 100_000), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, out := common.HexToAddress("0x01"), common.HexToAddress("0x02")
			m, reg := newTestQuoteMetrics(t)
			m.Initialize(1, []Route{{TokenIn: in, TokenOut: out, TokenInDecimals: 6, TokenOutDecimals: 6}}, nil)
			r := validQuoteMetricRequest(in, out)
			r.Amount = tc.amount
			m.Observe(r, false, "unsupported_protocol")
			families, err := reg.Gather()
			if err != nil {
				t.Fatal(err)
			}
			var count uint64
			for _, f := range families {
				if f.GetName() == "solver_bot_quote_requested_amount_tokens" {
					for _, sample := range f.GetMetric() {
						count += sample.GetHistogram().GetSampleCount()
					}
				}
			}
			if count != tc.want {
				t.Fatalf("samples=%d want=%d", count, tc.want)
			}
			assertQuoteResultCount(t, reg, 1)
		})
	}
}
