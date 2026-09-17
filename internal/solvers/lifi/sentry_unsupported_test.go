package lifi

import (
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

// Exercise the production logger and Sentry core, not only an error-string classifier.
func TestUnsupportedOrdersDoNotReachSentry(t *testing.T) {
	hub := sentry.CurrentHub()
	previous := hub.Client()
	t.Cleanup(func() { hub.BindClient(previous) })
	t.Setenv("SENTRY_DSN", "https://public@example.com/1")
	log, flush := observability.NewLogger(true)
	t.Cleanup(flush)
	var events []*sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "https://public@example.com/1", BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		events = append(events, event)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	hub.BindClient(client)
	cfg := testLifiConfig()
	s := &Solver{cfg: cfg, chainID: 11155111, log: log.WithName(Name)}
	for _, tc := range []struct {
		name       string
		mutate     func(map[string]any)
		wantEvents int
	}{
		{"native input", func(b map[string]any) { sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "0" }, 0},
		{"other chain", func(b map[string]any) { mapField(t, b, "order")["originChainId"] = "1" }, 0},
		{"other settler", func(b map[string]any) { b["inputSettler"] = "0x9999999999999999999999999999999999999999" }, 0},
		{"unsupported type", func(b map[string]any) { b["orderType"] = "GaslessCrosschainOrder" }, 0},
		{"callback", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "outputs")[0].(map[string]any)["callbackData"] = "0x1234"
		}, 0},
		{"dirty identifier", func(b map[string]any) {
			sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "1461501637330902918203684832716283019655932542977"
		}, 1},
		{"bad number", func(b map[string]any) { sliceField(t, mapField(t, b, "order"), "inputs")[0].([]any)[0] = "bad" }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(events)
			raw := mutatedTestOrderJSON(t, cfg, tc.mutate)
			if order, _ := s.parseOrderMessage(
				solverContext(t, s), orderMessage{Event: orderSubmitEvent, Data: raw},
			); order != nil {
				t.Fatal("rejected order was accepted")
			}
			if got := len(events) - before; got != tc.wantEvents {
				t.Fatalf("Sentry events=%d, want %d", got, tc.wantEvents)
			}
		})
	}
	for _, tc := range []struct {
		name       string
		err        error
		wantEvents int
	}{
		{"unsupported strategy format", types.MarkPermanentFillDecisionError(types.ErrUnsupportedOutputContext), 0},
		{"malformed known strategy format", types.MarkPermanentFillDecisionError(errors.New("outputContext: limit order length must be 1")), 1},
		{"RPC failure", errors.New("RPC unavailable"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := len(events)
			s.logFillDecisionError(observability.WithLogger(t.Context(), s.log), tc.err, "order fill: strategy", &submittedOrder{OrderID: "order-42"})
			if got := len(events) - before; got != tc.wantEvents {
				t.Fatalf("Sentry events=%d, want %d", got, tc.wantEvents)
			}
		})
	}
}
