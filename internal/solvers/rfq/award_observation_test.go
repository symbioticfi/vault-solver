package rfq

import (
	"encoding/json"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

func rfqObservationLogger(t *testing.T) (logr.Logger, *[]map[string]any) {
	t.Helper()
	events := new([]map[string]any)
	log := funcr.NewJSON(func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		*events = append(*events, event)
	}, funcr.Options{Verbosity: 1})
	return log, events
}

func TestAwardPairVerifiedWithoutQuoteHistoryAndOnlyOnce(t *testing.T) {
	st, backend := fillFixtures(t)
	txm := &fakeTxm{}
	e := newExec(t, st, backend, txm)
	log, events := rfqObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), log)
	if err := e.pollOpenOrders(ctx); err != nil {
		t.Fatal(err)
	}
	exec, err := executableFromBackend(backend.executable)
	if err != nil {
		t.Fatal(err)
	}
	order, err := decodeOrder(exec.encodedOrder)
	if err != nil {
		t.Fatal(err)
	}
	out, required, err := executableOrderTerms(exec, order, e.executor)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := e.buildFillPlan(ctx, "o1", exec, order, out, required, 0); err != nil {
			t.Fatal(err)
		}
	}
	assignments, verified := 0, 0
	for _, event := range *events {
		switch event["msg"] {
		case "quote awarded":
			assignments++
		case "quote award pair verified":
			verified++
			for key, want := range map[string]any{
				"chainId": float64(1), "orderId": "o1", "quoteId": "q1", "assignmentObservedAt": float64(0),
				"tokenIn": tIn.Hex(), "tokenOut": tOut.Hex(), "amountIn": order.Request.AmountIn.String(),
				"requiredAmountOut": required.String(), "tokenInDecimals": float64(18), "tokenOutDecimals": float64(6),
				"pairInScope": true, "pairScope": "directed_route", "pairScopeSource": "validated_candidates",
			} {
				if event[key] != want {
					t.Errorf("%s=%v want=%v", key, event[key], want)
				}
			}
		case "quote result", "quote response returned":
			t.Fatal("test unexpectedly required quote history")
		}
	}
	if assignments != 1 || verified != 1 || txm.calls != 0 || e.metrics != nil || e.tokenMetadata != nil {
		t.Fatalf("assignments=%d verified=%d sends=%d", assignments, verified, txm.calls)
	}
	if record := st.order("o1"); record.Status != statusQueued || !record.AwardPairObserved {
		t.Fatalf("observer changed execution status: %+v", record)
	}
}

func TestAwardPairVerificationRequiresMatchingEvidence(t *testing.T) {
	for _, mode := range []string{"reverse", "wrong chain", "policy", "absent candidates", "absent assignment", "invalid terms", "conflicting decimals"} {
		t.Run(mode, func(t *testing.T) {
			st, backend := fillFixtures(t)
			e := newExec(t, st, backend, &fakeTxm{})
			reg := prometheus.NewRegistry()
			var metricErr error
			e.metrics, metricErr = newRFQMetrics(reg, st, "")
			if metricErr != nil {
				t.Fatal(metricErr)
			}
			defer requirePairSeries(t, reg, 0)
			if mode != "absent assignment" {
				st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
			}
			log, events := rfqObservationLogger(t)
			ctx := observability.WithLogger(t.Context(), log)
			req := strategyRequest{QuoteID: "q1", TokenIn: tIn, TokenOut: tOut, Amount: big.NewInt(100)}
			route := liquidlane.NewRoute(1, vlt, common.Address{}, tIn, tOut, 18, 6)
			candidates := []liquidlane.QuoteCandidate{{Route: route}}
			switch mode {
			case "reverse":
				candidates[0].Route = liquidlane.NewRoute(1, vlt, common.Address{}, tOut, tIn, 6, 18)
			case "wrong chain":
				candidates[0].Route = liquidlane.NewRoute(8453, vlt, common.Address{}, tIn, tOut, 18, 6)
			case "policy":
				e.tokenPolicy = testPermissionedPolicy(t, common.HexToAddress("0xffff"))
			case "absent candidates":
				candidates = nil
			case "invalid terms":
				req.Amount = nil
			case "conflicting decimals":
				other := route
				other.TokenInDecimals = 6
				candidates = append(candidates, liquidlane.QuoteCandidate{Route: other})
			}
			e.observeAwardPair(ctx, "o1", req, big.NewInt(90), candidates)
			if len(*events) != 0 {
				t.Fatalf("unverified award pair=%v", *events)
			}
			if record := st.order("o1"); record != nil && record.AwardPairObserved {
				t.Fatal("missing evidence spent the dedup flag")
			}
		})
	}
}

func TestStoreAwardPairFlagDeduplicatesConcurrentObservers(t *testing.T) {
	st, _ := fillFixtures(t)
	st.upsertQueued(queuedOrder{OrderID: "o1", QuoteID: "q1"})
	before := st.order("o1")
	var admitted atomic.Int64
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			if _, first := st.markAwardPairObserved("o1"); first {
				admitted.Add(1)
			}
		})
	}
	workers.Wait()
	after := st.order("o1")
	if admitted.Load() != 1 || before.Status != after.Status || before.UpdatedAt != after.UpdatedAt ||
		before.CreatedAt != after.CreatedAt || before.NonceRetries != after.NonceRetries {
		t.Fatalf("admitted=%d before=%+v after=%+v", admitted.Load(), before, after)
	}
}
