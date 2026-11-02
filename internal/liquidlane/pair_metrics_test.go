package liquidlane

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

func TestPairMetricsFixedVocabularyAndReceiptBoundary(t *testing.T) {
	reg := prometheus.NewRegistry()
	bound := func(reason string) string {
		if reason == "capacity" {
			return reason
		}
		return "unknown"
	}
	m, err := NewPairMetrics(reg, "test", bound)
	if err != nil {
		t.Fatal(err)
	}
	in, out := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	m.Quote(1, in, out, true, "ignored")
	m.Quote(1, in, out, false, "capacity")
	m.Quote(1, in, out, false, strings.Repeat("arbitrary", 100))
	m.Assignment(1, in, out, "observed")
	m.Assignment(1, in, out, "admitted")
	m.Assignment(1, in, out, "arbitrary")
	m.Standing(1, in, out, "published", "ignored")
	m.Standing(1, in, out, "withdrawn", "ignored")
	m.Standing(1, in, out, "rejected", "capacity")
	m.Standing(1, in, out, "arbitrary", "arbitrary")
	receipt := m.ReceiptObserver(1, in, out)
	receipt(t.Context(), txmanager.Result{})
	receipt(t.Context(), txmanager.Result{Receipt: &types.Receipt{Status: types.ReceiptStatusFailed}})
	receipt(t.Context(), txmanager.Result{Receipt: &types.Receipt{Status: types.ReceiptStatusSuccessful}})
	for _, item := range []struct{ event, result, reason string }{
		{"quote", "returned", "none"}, {"quote", "rejected", "capacity"}, {"quote", "rejected", "unknown"},
		{"assignment", "observed", "none"}, {"assignment", "admitted", "none"},
		{"standing_quote", "published", "none"}, {"standing_quote", "withdrawn", "none"}, {"standing_quote", "rejected", "capacity"},
		{"fill", "mined_success", "none"},
	} {
		metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"solver": "test", "chain_id": "1", "token_in": strings.ToLower(in.Hex()), "token_out": strings.ToLower(out.Hex()), "event": item.event, "result": item.result, "reason": item.reason}, 1)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || len(families[0].GetMetric()) != 9 {
		t.Fatalf("unexpected series: %v", families)
	}
	// No token symbols, amounts, IDs or arbitrary callback statuses become labels.
	for _, sample := range families[0].GetMetric() {
		if len(sample.GetLabel()) != 7 {
			t.Fatalf("labels=%v", sample.GetLabel())
		}
	}
}

func TestPairMetricsInvalidIdentityDoesNotAllocateAndCountersRetainHistory(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := NewPairMetrics(reg, "test", func(string) string { return "unknown" })
	if err != nil {
		t.Fatal(err)
	}
	in, out := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	for _, pair := range []struct {
		chain   int64
		in, out common.Address
	}{{0, in, out}, {-1, in, out}, {1, common.Address{}, out}, {1, in, common.Address{}}, {1, in, in}} {
		m.Quote(pair.chain, pair.in, pair.out, true, "")
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 0 {
		t.Fatalf("invalid identities allocated: %v", families)
	}
	for range 2 {
		m.Quote(1, in, out, true, "")
	}
	m.Quote(10, in, out, true, "")
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"chain_id": "1"}, 2)
	metricstest.RequireFamilyValue(t, reg, "solver_bot_pair_events_total", map[string]string{"chain_id": "10"}, 1)
}
