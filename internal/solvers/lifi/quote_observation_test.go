package lifi

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func standingObservationLogger(t *testing.T) (*[]map[string]any, func(string)) {
	t.Helper()
	events := new([]map[string]any)
	return events, func(line string) {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Error(err)
		}
		*events = append(*events, event)
	}
}

func TestStandingQuoteSnapshotUsesCurrentInventoryWithoutRegistry(t *testing.T) {
	r := testQuoteRoute()
	newRoute := r
	newRoute.TokenIn = common.HexToAddress("0x1234")
	newRoute.TokenInDecimals = 18
	forbidden := r
	forbidden.TokenIn = common.HexToAddress("0xffff")
	cfg := testLifiConfig()
	cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, r.TokenIn, newRoute.TokenIn)
	s := &Solver{cfg: cfg, chainID: 11155111}
	s.cfg.QuoteTTL = time.Minute
	s.setPairRoutes([]route{r, newRoute})
	known, newlyTrusted := testStandingQuote(r, 100), testStandingQuote(newRoute, 200)
	newlyTrusted.FromDecimals = 18
	amount, ok := new(big.Int).SetString("900719925474099300000000000000000001", 10)
	if !ok {
		t.Fatal("invalid fixture amount")
	}
	newlyTrusted.Ranges[0].MaxAmount = amount
	newlyTrusted.Ranges[0].Quote = "1.000000000000000001"
	wrongDirection := known
	wrongDirection.FromAsset, wrongDirection.ToAsset = wrongDirection.ToAsset, wrongDirection.FromAsset
	wrongDecimals := known
	wrongDecimals.FromDecimals++
	state := newQuoteState(time.Second)
	state.active = indexQuotePairs([]types.Quote{known, newlyTrusted, testStandingQuote(forbidden, 300), wrongDirection, wrongDecimals})
	before := indexQuotePairs([]types.Quote{known, newlyTrusted, testStandingQuote(forbidden, 300), wrongDirection, wrongDecimals})
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
	inventory := []liquidlane.Inventory{{Route: r}, {Route: newRoute}, {Route: forbidden}}
	s.observeStandingQuoteSnapshot(ctx, state, inventory, nil)
	if !reflect.DeepEqual(state.active, before) {
		t.Fatal("observation changed active quote state")
	}
	if len(*events) != 3 {
		t.Fatalf("events=%v", *events)
	}
	header := (*events)[2]
	if header["msg"] != "standing quote snapshot" || header["snapshotState"] != "reconciled" ||
		header["supportedPairs"] != float64(2) || header["quoteCount"] != float64(2) || header["rangeCount"] != float64(2) {
		t.Fatalf("header=%v", header)
	}
	foundNew := false
	for _, row := range (*events)[:2] {
		scoped, scopeKnown := row["pairInScope"].(bool)
		if !scopeKnown || !scoped || row["snapshotId"] != header["snapshotId"] || row["pairScope"] != "directed_route" || row["pairScopeSource"] != "current_inventory" ||
			row["chainId"] != float64(11155111) || row["earliestExpiry"] != float64(known.Expiry) || row["latestExpiry"] != float64(known.Expiry) {
			t.Fatalf("pair fields=%v", row)
		}
		if row["tokenIn"] == newRoute.TokenIn.Hex() {
			foundNew = true
			terms := row["ranges"].([]any)
			term := terms[0].(map[string]any)
			if len(terms) != 1 || term["minAmount"] != bigString(newlyTrusted.Ranges[0].MinAmount) ||
				term["maxAmount"] != amount.String() || term["rate"] != newlyTrusted.Ranges[0].Quote ||
				term["expiry"] != float64(newlyTrusted.Expiry) || term["exclusiveFor"] != newlyTrusted.ExclusiveFor.Hex() {
				t.Fatalf("wire terms lost precision or identity: %v", terms)
			}
			if row["maxInputAmount"] != amount.String() || row["tokenInDecimals"] != float64(18) || row["tokenOutDecimals"] != float64(6) {
				t.Fatalf("new trusted token fields=%v", row)
			}
		}
	}
	if !foundNew {
		t.Fatal("new trusted inventory token required startup registry membership")
	}
}

func TestStandingQuoteSnapshotAcknowledgementFailureAndClear(t *testing.T) {
	fail := false
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		type quoteRanges struct {
			Ranges []json.RawMessage `json:"ranges"`
		}
		var body struct {
			Quotes []quoteRanges `json:"quotes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		count := 0
		for _, q := range body.Quotes {
			count += len(q.Ranges)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status": "success", "quotesAdded": count}); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	r := testQuoteRoute()
	s := &Solver{cfg: testLifiConfig(), chainID: 11155111, orders: newOrderClient(srv.URL, "test-key", time.Second, 11155111)}
	s.cfg.QuoteTTL = time.Minute
	s.setPairRoutes([]route{r})
	state := newQuoteState(time.Second)
	inventory := []liquidlane.Inventory{{Route: r}}
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
	q := testStandingQuote(r, 100)
	if _, err := s.reconcileQuotes(ctx, state, []types.Quote{q}, inventory, time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(*events) != 2 || (*events)[1]["snapshotState"] != "reconciled" {
		t.Fatalf("publish events=%v calls=%d", *events, calls)
	}
	fail = true
	if _, err := s.reconcileQuotes(ctx, state, nil, nil, time.Now()); err == nil {
		t.Fatal("withdrawal failure was hidden")
	}
	uncertain := (*events)[len(*events)-1]
	if uncertain["snapshotState"] != "uncertain" || uncertain["supportedPairs"] != nil || len(state.active) != 1 {
		t.Fatalf("failed withdrawal invented acknowledgement: %v active=%d", uncertain, len(state.active))
	}
	fail = false
	if _, err := s.reconcileQuotes(ctx, state, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	cleared := (*events)[len(*events)-1]
	if cleared["snapshotState"] != "suspended" || cleared["supportedPairs"] != float64(0) || len(state.active) != 0 || calls != 3 ||
		cleared["snapshotId"] == uncertain["snapshotId"] {
		t.Fatalf("successful empty snapshot did not clear: %v calls=%d", cleared, calls)
	}
}

func TestStandingQuoteSnapshotExpiryAndEmptyRanges(t *testing.T) {
	r := testQuoteRoute()
	s := &Solver{cfg: testLifiConfig()}
	s.cfg.QuoteTTL = time.Minute
	s.setPairRoutes([]route{r})
	q := testStandingQuote(r, 100)
	q.Expiry = time.Now().Add(-time.Minute).Unix()
	state := newQuoteState(time.Second)
	state.active = indexQuotePairs([]types.Quote{q})
	// Renewal scheduling uses expiry=0 internally; logs must retain the actual wire expiry.
	state.forceRenewal()
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{}))
	s.observeStandingQuoteSnapshot(ctx, state, []liquidlane.Inventory{{Route: r}}, nil)
	if len(*events) != 2 || (*events)[0]["earliestExpiry"] != float64(q.Expiry) || (*events)[0]["latestExpiry"] != float64(q.Expiry) {
		t.Fatalf("expired wire terms were invented/reset: %v", *events)
	}
	q.Ranges = nil
	state.active = indexQuotePairs([]types.Quote{q})
	s.observeStandingQuoteSnapshot(ctx, state, []liquidlane.Inventory{{Route: r}}, nil)
	header := (*events)[len(*events)-1]
	if header["supportedPairs"] != float64(0) || len(*events) != 3 {
		t.Fatalf("empty ranges invented a positive pair: %v", *events)
	}
	s.observeStandingQuoteSnapshot(ctx, state, nil, errors.New("publish failed"))
	if (*events)[len(*events)-1]["snapshotState"] != "uncertain" {
		t.Fatal("failure emitted successful state")
	}
}

func TestStandingQuoteRejectionDetailsRequireCurrentDirection(t *testing.T) {
	r := testQuoteRoute()
	newRoute := r
	newRoute.TokenIn = common.HexToAddress("0x1234")
	forbidden := r
	forbidden.TokenIn = common.HexToAddress("0xffff")
	cfg := testLifiConfig()
	cfg.TokenPolicy = testTokenPolicy(t, tokenpolicy.Permissioned, r.TokenIn, newRoute.TokenIn)
	s := &Solver{cfg: cfg}
	s.cfg.QuoteTTL = time.Minute
	s.setPairRoutes([]route{r, newRoute})
	events, write := standingObservationLogger(t)
	ctx := observability.WithLogger(t.Context(), funcr.NewJSON(write, funcr.Options{Verbosity: 1}))
	inventory := []liquidlane.Inventory{{Route: r}, {Route: newRoute}, {Route: forbidden}}
	s.observeQuoteBuildRejections(ctx, inventory, nil, "strategy_error")
	if len(*events) != 2 {
		t.Fatalf("current supported rejection details=%v", *events)
	}
	for _, row := range *events {
		if row["tokenOut"] != r.TokenOut.Hex() || row["tokenIn"] == forbidden.TokenIn.Hex() || row["pairScope"] != "directed_route" || row["pairScopeSource"] != "current_inventory" {
			t.Fatalf("unsupported detail leaked: %v", row)
		}
	}
	// Fresh structural proof remains diagnostic at zero capacity; old inventory cannot renew it.
	s.observeQuoteBuildRejections(ctx, nil, nil, "snapshot_error")
	if len(*events) != 4 {
		t.Fatalf("fresh supported failure omitted: %v", *events)
	}
	s.pairRoutes.Store(nil)
	s.observeQuoteBuildRejections(ctx, inventory, nil, "snapshot_error")
	if len(*events) != 4 {
		t.Fatal("withdrawn support reused old inventory")
	}
}
