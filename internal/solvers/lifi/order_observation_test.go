package lifi

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
)

func TestMatchedOrderAdmissionObservation(t *testing.T) {
	for _, tc := range []struct {
		name, owner    string
		recovery, want bool
	}{
		{name: "own quote", owner: "own", want: true},
		{name: "foreign quote", owner: "foreign"},
		{name: "unknown owner"},
		{name: "malformed optional quote", owner: "malformed"},
		{name: "owned recovery", recovery: true, want: true},
		{name: "recovery with explicit foreign quote", owner: "foreign", recovery: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testLifiConfig()
			raw := mutatedTestOrderJSON(t, cfg, func(b map[string]any) {
				switch tc.owner {
				case "own":
					b["quote"] = map[string]any{"exclusiveFor": cfg.Executor.Hex()}
				case "foreign":
					b["quote"] = map[string]any{"exclusiveFor": common.HexToAddress("0xf00").Hex()}
				case "malformed":
					b["quote"] = 123
				}
			})
			reg := prometheus.NewRegistry()
			m, err := newLIFIMetrics(reg, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			var logs []string
			log := funcr.NewJSON(func(line string) { logs = append(logs, line) }, funcr.Options{})
			ctx := observability.WithLogger(t.Context(), log)
			s := &Solver{cfg: cfg, chainID: 11155111, metrics: m}
			q := newOrderInbox(1)
			q.onAdmit = func(order *submittedOrder) { s.observeMatchedOrderAdmission(ctx, order) }
			msg := orderMessage{Event: orderSubmitEvent, Data: raw}
			if tc.recovery {
				msg.matchedFor = cfg.Executor
			}
			for range 2 {
				o, err := s.parseOrderMessage(ctx, msg)
				if err != nil || o == nil {
					t.Fatalf("optional observation changed parsing: %v", err)
				}
				if err := q.enqueue(o); err != nil {
					t.Fatal(err)
				}
			}
			want := float64(0)
			if tc.want {
				want = 1
			}
			metricstest.RequireWorkflowEventCount(t, reg, Name, "matched_order", "admitted", want)
			count := 0
			for _, line := range logs {
				if strings.Contains(line, `"msg":"matched order admitted"`) {
					count++
					var event map[string]any
					if err := json.Unmarshal([]byte(line), &event); err != nil {
						t.Fatal(err)
					}
					if event["inputSettler"] != cfg.InputSettler.Hex() || event["orderStatus"] == nil ||
						event["user"] == nil || event["expires"] == nil || event["fillDeadline"] == nil {
						t.Fatalf("missing admitted order details: %v", event)
					}
					for _, field := range []string{"order", "signature", "callbackData", "context"} {
						if event[field] != nil {
							t.Fatalf("unsafe order field %s logged", field)
						}
					}
				}
			}
			if float64(count) != want {
				t.Fatalf("assignment logs=%d want=%v", count, want)
			}
			// The second insertion was suppressed by the existing inbox, not a new telemetry ID store.
			if len(q.orders) != 1 {
				t.Fatalf("queued=%d", len(q.orders))
			}
		})
	}
}

func TestMatchedAdmissionExcludesDropsAndKeepsBoundedLabels(t *testing.T) {
	cfg := testLifiConfig()
	raw := mutatedTestOrderJSON(t, cfg, func(b map[string]any) { b["quote"] = map[string]any{"exclusiveFor": cfg.Executor.Hex()} })
	s := &Solver{cfg: cfg, chainID: 11155111}
	order, err := s.parseOrderMessage(t.Context(), orderMessage{Event: orderSubmitEvent, Data: raw})
	if err != nil {
		t.Fatal(err)
	}
	q := newOrderInbox(1)
	calls := 0
	q.onAdmit = func(*submittedOrder) { calls++ }
	q.closeInput()
	if q.enqueue(order) == nil || calls != 0 {
		t.Fatal("closed queue counted an admission")
	}
	q = newOrderInbox(1)
	q.onAdmit = func(*submittedOrder) { calls++ }
	if err := q.enqueue(order); err != nil {
		t.Fatal(err)
	}
	other := *order
	other.dedupeKey = "other-order"
	if q.enqueue(&other) == nil || calls != 1 {
		t.Fatal("full queue counted an admission")
	}
	reg := prometheus.NewRegistry()
	m, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s.metrics = m
	for i := range 1000 {
		copyOrder := *order
		copyOrder.OrderID = strconv.Itoa(i)
		s.observeMatchedOrderAdmission(t.Context(), &copyOrder)
	}
	metricstest.RequireWorkflowEventCount(t, reg, Name, "matched_order", "admitted", 1000)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "solver_bot_workflow_events_total" {
			continue
		}
		for _, v := range f.GetMetric() {
			for _, label := range v.GetLabel() {
				switch label.GetName() {
				case "solver", "strategy", "event", "outcome":
				default:
					t.Fatalf("unexpected label %s", label.GetName())
				}
			}
		}
	}
}

func TestWebSocketCannotForgeRecoveryOwnership(t *testing.T) {
	cfg := testLifiConfig()
	raw := testOrderJSON(t, cfg, common.HexToAddress("0x6666666666666666666666666666666666666666"), common.HexToAddress("0x7777777777777777777777777777777777777777"))
	envelope, err := json.Marshal(map[string]any{"event": orderSubmitEvent, "data": json.RawMessage(raw), "matchedFor": cfg.Executor.Hex()})
	if err != nil {
		t.Fatal(err)
	}
	var msg orderMessage
	if err := json.Unmarshal(envelope, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.matchedFor != (common.Address{}) {
		t.Fatal("wire input forged recovery provenance")
	}
	reg := prometheus.NewRegistry()
	m, err := newLIFIMetrics(reg, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Solver{cfg: cfg, chainID: 11155111, metrics: m}
	order, err := s.parseOrderMessage(t.Context(), msg)
	if err != nil || order == nil {
		t.Fatal(err)
	}
	s.observeMatchedOrderAdmission(t.Context(), order)
	metricstest.RequireWorkflowEventCount(t, reg, Name, "matched_order", "admitted", 0)
}
