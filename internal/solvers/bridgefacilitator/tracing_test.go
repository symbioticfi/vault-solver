package bridgefacilitator

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// stubOfferStrategy stands in for the configured 3F strategy: it returns a fixed plan, an error, or
// blows up mid-decision the way a webhook decoder can.
type stubOfferStrategy struct {
	offers []types.OfferExecution
	err    error
	panics bool
}

func (s stubOfferStrategy) DecideOffers(context.Context, types.OfferInput) (types.OfferOutput, error) {
	if s.panics {
		panic("strategy exploded")
	}
	return types.OfferOutput{Offers: s.offers}, s.err
}

// tracingFixture is one traced 3F solver wired to a fake 3F API and a fake multicall chain.
type tracingFixture struct {
	solver      *Solver
	adapter     common.Address
	request     common.Address
	auctionID   int64
	createCalls *atomic.Int64
}

const (
	tracingAdapterHex   = "0x00000000000000000000000000000000000000a0"
	tracingRequestHex   = "0x00000000000000000000000000000000000000b0"
	tracingPendingHex   = "0x00000000000000000000000000000000000000b1"
	tracingVaultHex     = "0x00000000000000000000000000000000000000c0"
	tracingAssetHex     = "0x00000000000000000000000000000000000000d0"
	tracingAuctionID    = int64(10)
	tracingPrincipal    = 700
	tracingReturn       = 14
	tracingStrategyName = "default"
)

// newTracingFixture builds a solver whose single adapter can fund the single open auction the fake
// API serves. createStatus selects the createOffer response code.
func newTracingFixture(t *testing.T, strategy types.Strategy, createStatus int) *tracingFixture {
	t.Helper()

	adapter := common.HexToAddress(tracingAdapterHex)
	request := common.HexToAddress(tracingRequestHex)
	asset := common.HexToAddress(tracingAssetHex)
	var createCalls atomic.Int64

	auctions, err := json.Marshal([]map[string]any{{
		"id":              tracingAuctionID,
		"requestId":       request.Hex(),
		"status":          "open",
		"amountRequested": strconv.Itoa(tracingPrincipal),
		"maxRate":         200,
		"depositAsset":    map[string]any{"address": asset.Hex(), "symbol": "USDC", "decimals": 6},
		"eip712Domain":    map[string]any{"name": "GruntBF", "version": "1", "chainId": 1},
	}})
	if err != nil {
		t.Fatalf("marshal auctions: %v", err)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/auction":
			_, _ = w.Write(auctions)
		case r.URL.Path == "/v1/offer" && r.Method == http.MethodPost:
			createCalls.Add(1)
			w.WriteHeader(createStatus)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.URL.Path == "/v1/offer":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	chainClient, stopChain := newMulticallFakeClient(t, abiEncodeAggregate3Results(t,
		abiEncodeUint256(t, 1_000), // getMaxAssets
		abiEncodeUint256(t, 0),     // minYieldPerRequest
		abiEncodeUint256(t, 0),     // minAssetsPerRequest
		abiEncodeUint256(t, 1_000), // maxAssetsPerRequest
		abiEncodeUint256(t, 0),     // requestsLength
	))
	t.Cleanup(stopChain)

	s := &Solver{
		cfg: &Config{
			OfferExpiryBuffer: time.Hour,
			RedeemBatchSize:   10,
			Strategy:          StrategyConfig{Name: tracingStrategyName},
		},
		deps:                 solver.Deps{Signer: fakeSigner{}},
		api:                  newAPIClient(api.URL, fakeSigner{}, big.NewInt(1), time.Second, logr.Discard()),
		reader:               newReader(chainClient, common.Address{}),
		strategy:             strategy,
		log:                  logr.Discard(),
		laneReady:            func() bool { return true },
		offers:               newOfferTracker(),
		links:                observability.NewSpanLinks(0),
		targets:              []Target{{Adapter: adapter, Vault: common.HexToAddress(tracingVaultHex), Collateral: asset}},
		targetsAuthoritative: true,
	}
	return &tracingFixture{
		solver:      s,
		adapter:     adapter,
		request:     request,
		auctionID:   tracingAuctionID,
		createCalls: &createCalls,
	}
}

func tracingOffer() []types.OfferExecution {
	return []types.OfferExecution{{
		AuctionID:      tracingAuctionID,
		Request:        common.HexToAddress(tracingRequestHex),
		Maker:          common.HexToAddress(tracingAdapterHex),
		Principal:      big.NewInt(tracingPrincipal),
		ExpectedReturn: big.NewInt(tracingReturn),
	}}
}

// TestDiscoverAndOfferTracesOfferPipeline pins the offer trace: one root per poll, the reconcile,
// view and decide stages under it, one auction span per submitted offer carrying the searchable
// identifiers, and the offer-submission span remembered under both link keys (spec §9.4, §12).
func TestDiscoverAndOfferTracesOfferPipeline(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{offers: tracingOffer()}, http.StatusCreated)

	fixture.solver.discoverAndOffer(t.Context())

	if got := fixture.createCalls.Load(); got != 1 {
		t.Fatalf("createOffer calls = %d, want 1", got)
	}
	sync := tracetest.Ended(t, rec, "3f.sync")
	if sync.Parent().IsValid() {
		t.Fatalf("3f.sync has parent %v, want a root", sync.Parent())
	}
	reconcile := tracetest.Ended(t, rec, "3f.offers.reconcile")
	view := tracetest.Ended(t, rec, "3f.auction.view")
	decide := tracetest.Ended(t, rec, "3f.offer.decide")
	auction := tracetest.Ended(t, rec, "3f.auction")
	build := tracetest.Ended(t, rec, "3f.offer.build")
	submit := tracetest.Ended(t, rec, "3f.offer.submit")
	for _, child := range []sdktrace.ReadOnlySpan{reconcile, view, decide, auction} {
		tracetest.RequireChildOf(t, child, sync)
	}
	tracetest.RequireChildOf(t, build, auction)
	tracetest.RequireChildOf(t, submit, auction)
	tracetest.RequireNoErrorSpans(t, rec)

	if got := tracetest.Attr(decide, "strategy.name"); got != tracingStrategyName {
		t.Fatalf("decide strategy.name = %q, want %q", got, tracingStrategyName)
	}
	if got := tracetest.Attr(auction, "auction.id"); got != strconv.FormatInt(fixture.auctionID, 10) {
		t.Fatalf("auction.id = %q, want %d", got, fixture.auctionID)
	}
	if got := tracetest.Attr(auction, "adapter.address"); got != fixture.adapter.Hex() {
		t.Fatalf("adapter.address = %q, want %s", got, fixture.adapter.Hex())
	}
	if got := tracetest.Attr(auction, "request.address"); got != fixture.request.Hex() {
		t.Fatalf("request.address = %q, want %s", got, fixture.request.Hex())
	}
	if got := tracetest.Attr(sync, "solver"); got != Name {
		t.Fatalf("sync solver = %q, want %q", got, Name)
	}

	link, ok := fixture.solver.links.Lookup(requestLinkKey(fixture.request))
	if !ok {
		t.Fatal("submitted offer was not remembered under its request key")
	}
	if got, want := link.SpanContext.SpanID(), auction.SpanContext().SpanID(); got != want {
		t.Fatalf("remembered span = %s, want the auction span %s", got, want)
	}
	if _, remembered := fixture.solver.links.Lookup(auctionLinkKey(fixture.adapter, fixture.auctionID)); !remembered {
		t.Fatal("submitted offer was not remembered under its (adapter, auction) key")
	}
}

// A strategy that offers nothing is an expected outcome, not a failure: the decide stage records it
// as a decline and no span carries an error.
func TestDiscoverAndOfferDeclinesEmptyStrategyPlan(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{}, http.StatusCreated)

	fixture.solver.discoverAndOffer(t.Context())

	if got := fixture.createCalls.Load(); got != 0 {
		t.Fatalf("createOffer calls = %d, want none", got)
	}
	decide := tracetest.Ended(t, rec, "3f.offer.decide")
	if !tracetest.HasEvent(decide, "declined") {
		t.Fatalf("decide span events = %v, want a declined event", decide.Events())
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// A failing createOffer is a real failure: the submit stage and the auction span it belongs to both
// end with an error status and a recorded exception.
func TestDiscoverAndOfferRecordsSubmitFailure(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{offers: tracingOffer()}, http.StatusInternalServerError)

	fixture.solver.discoverAndOffer(t.Context())

	submit := tracetest.Ended(t, rec, "3f.offer.submit")
	if submit.Status().Code != codes.Error {
		t.Fatalf("submit status = %v, want error", submit.Status())
	}
	if !tracetest.HasEvent(submit, "exception") {
		t.Fatalf("submit events = %v, want a recorded exception", submit.Events())
	}
	auction := tracetest.Ended(t, rec, "3f.auction")
	if auction.Status().Code != codes.Error {
		t.Fatalf("auction status = %v, want error", auction.Status())
	}
	if _, ok := fixture.solver.links.Lookup(requestLinkKey(fixture.request)); ok {
		t.Fatal("a failed submission must not be remembered as a linkable offer")
	}
}

// A lane that is not ready is an expected skip: the sync span records a decline and no error.
func TestDiscoverAndOfferDeclinesWhenLaneNotReady(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{offers: tracingOffer()}, http.StatusCreated)
	fixture.solver.laneReady = func() bool { return false }

	fixture.solver.discoverAndOffer(t.Context())

	sync := tracetest.Ended(t, rec, "3f.sync")
	if !tracetest.HasEvent(sync, "declined") {
		t.Fatalf("sync events = %v, want a declined event", sync.Events())
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// Every span end is deferred, so a strategy that panics mid-decision still exports its stages
// instead of leaving them open forever.
func TestDiscoverAndOfferEndsSpansOnStrategyPanic(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{panics: true}, http.StatusCreated)

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Error("strategy panic did not propagate")
			}
		}()
		fixture.solver.discoverAndOffer(t.Context())
	}()

	tracetest.Ended(t, rec, "3f.offer.decide")
	tracetest.Ended(t, rec, "3f.sync")
}

// newRedeemFixture wires a solver whose single adapter holds two requests, only one of which can
// still be withdrawn, so exactly one finalize is batched.
func newRedeemFixture(t *testing.T, outcome txmanager.Outcome, hash common.Hash) (*Solver, common.Address, *atomic.Int64) {
	t.Helper()

	adapter := common.HexToAddress(tracingAdapterHex)
	request := common.HexToAddress(tracingRequestHex)
	pending := common.HexToAddress(tracingPendingHex)
	chainClient, stopChain := newMulticallFakeClient(t,
		abiEncodeAggregate3Results(t, abiEncodeUint256(t, 2)),
		abiEncodeAggregate3Results(t, abiEncodeAddress(t, request), abiEncodeAddress(t, pending)),
		abiEncodeAggregate3Results(t, abiEncodeBool(t, true), abiEncodeBool(t, false)),
	)
	t.Cleanup(stopChain)

	var sent atomic.Int64
	s := &Solver{
		cfg:                  &Config{RedeemBatchSize: 10},
		reader:               newReader(chainClient, common.Address{}),
		log:                  logr.Discard(),
		links:                observability.NewSpanLinks(0),
		targets:              []Target{{Adapter: adapter}},
		targetsAuthoritative: true,
	}
	s.txManager = transactionSenderFunc(func(context.Context, txmanager.Request) txmanager.Result {
		sent.Add(1)
		return txmanager.Result{Outcome: outcome, Hash: hash}
	})
	return s, request, &sent
}

// TestRedeemAllLinksToRememberedOfferSpans pins the redeem side of §12: the submission span links
// back to the offer span of every request it finalizes and counts the matches.
func TestRedeemAllLinksToRememberedOfferSpans(t *testing.T) {
	rec := tracetest.Install(t)
	hash := common.HexToHash("0xfeed")
	s, request, sent := newRedeemFixture(t, txmanager.OutcomeConfirmed, hash)

	offerCtx, endOffer := tracer.Start(t.Context(), "3f.auction")
	s.links.Remember(offerCtx, requestLinkKey(request), time.Hour)
	offerSpan := trace.SpanContextFromContext(offerCtx)
	endOffer(nil)

	s.redeemAll(t.Context())

	if got := sent.Load(); got != 1 {
		t.Fatalf("sent transactions = %d, want 1", got)
	}
	redeem := tracetest.Ended(t, rec, "3f.redeem")
	read := tracetest.Ended(t, rec, "3f.redeem.read")
	submit := tracetest.Ended(t, rec, "3f.redeem.submit")
	tracetest.RequireChildOf(t, read, redeem)
	tracetest.RequireChildOf(t, submit, redeem)

	if got := len(submit.Links()); got != 1 {
		t.Fatalf("redeem submit links = %d, want 1", got)
	}
	if got, want := submit.Links()[0].SpanContext.SpanID(), offerSpan.SpanID(); got != want {
		t.Fatalf("link target = %s, want the offer span %s", got, want)
	}
	if got := tracetest.Attr(submit, "offer.linked_count"); got != "1" {
		t.Fatalf("offer.linked_count = %q, want 1", got)
	}
	if got := tracetest.Attr(submit, "tx.hash"); got != hash.Hex() {
		t.Fatalf("submit tx.hash = %q, want %s", got, hash.Hex())
	}
	if got := tracetest.Attr(redeem, "tx.hash"); got != hash.Hex() {
		t.Fatalf("redeem tx.hash = %q, want %s", got, hash.Hex())
	}
	if got := tracetest.Attr(submit, "tx.outcome"); got != string(txmanager.OutcomeConfirmed) {
		t.Fatalf("submit tx.outcome = %q, want %s", got, txmanager.OutcomeConfirmed)
	}
	if tracetest.HasEvent(submit, "link_miss") {
		t.Fatal("redeem submit recorded a link miss despite a remembered offer")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// A miss is inert (spec §12): the redeem still sends, with no link and one link_miss naming the key.
func TestRedeemAllSendsWithoutRememberedOfferSpan(t *testing.T) {
	rec := tracetest.Install(t)
	s, request, sent := newRedeemFixture(t, txmanager.OutcomeConfirmed, common.HexToHash("0xbeef"))

	s.redeemAll(t.Context())

	if got := sent.Load(); got != 1 {
		t.Fatalf("sent transactions = %d, want 1", got)
	}
	submit := tracetest.Ended(t, rec, "3f.redeem.submit")
	if got := len(submit.Links()); got != 0 {
		t.Fatalf("redeem submit links = %d, want none", got)
	}
	if got := tracetest.Attr(submit, "offer.linked_count"); got != "0" {
		t.Fatalf("offer.linked_count = %q, want 0", got)
	}
	misses := 0
	for _, event := range submit.Events() {
		if event.Name != "link_miss" {
			continue
		}
		misses++
		var key string
		for _, kv := range event.Attributes {
			if kv.Key == "key" {
				key = kv.Value.AsString()
			}
		}
		if key != requestLinkKey(request) {
			t.Fatalf("link_miss key = %q, want %s", key, requestLinkKey(request))
		}
	}
	if misses != 1 {
		t.Fatalf("link_miss events = %d, want 1 (events %v)", misses, submit.Events())
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// A redeem the manager rejected before broadcasting has no transaction: the outcome is recorded,
// tx.hash is left off rather than stamped as the zero hash.
func TestRedeemAllOmitsTxHashWhenNotBroadcast(t *testing.T) {
	rec := tracetest.Install(t)
	s, _, sent := newRedeemFixture(t, txmanager.OutcomeSubmissionError, common.Hash{})

	s.redeemAll(t.Context())

	if got := sent.Load(); got != 1 {
		t.Fatalf("sent transactions = %d, want 1", got)
	}
	for _, name := range []string{"3f.redeem", "3f.redeem.submit"} {
		span := tracetest.Ended(t, rec, name)
		if got := tracetest.Attr(span, "tx.hash"); got != "" {
			t.Fatalf("%s tx.hash = %q, want no attribute for a transaction that never went out", name, got)
		}
		if got := tracetest.Attr(span, "tx.outcome"); got != string(txmanager.OutcomeSubmissionError) {
			t.Fatalf("%s tx.outcome = %q, want %s", name, got, txmanager.OutcomeSubmissionError)
		}
	}
	if got := tracetest.Ended(t, rec, "3f.redeem.submit").Status().Code; got != codes.Error {
		t.Fatalf("submit span status = %v, want Error", got)
	}
}

// solverContext stands in for Solver.Run, which stores the solver logger on the context it passes
// down to every tick below it.
func solverContext(t *testing.T, s *Solver) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), s.log)
}

// A listed offer whose submission is still remembered carries that trace on its log lines, so the
// API's view of an offer joins back to the pass that created it without the trace backend (spec §12).
func TestReconcileOffersStampsRememberedOfferTrace(t *testing.T) {
	tracetest.Install(t)

	adapter := common.HexToAddress(tracingAdapterHex)
	expiration := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"auctionId":` + strconv.FormatInt(tracingAuctionID, 10) +
			`,"status":"CREATED","maker":"` + strings.ToLower(adapter.Hex()) +
			`","amount":"not-a-uint256","expectedReturn":"1","nonce":"1","expiration":"` + expiration + `"}]`))
	}))
	defer api.Close()

	var lines []string
	s := &Solver{
		api:    newAPIClient(api.URL, fakeSigner{}, big.NewInt(1), time.Second, logr.Discard()),
		log:    funcr.NewJSON(func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1}),
		offers: newOfferTracker(),
		links:  observability.NewSpanLinks(0),
	}
	offerCtx, endOffer := tracer.Start(t.Context(), "3f.auction")
	s.links.Remember(offerCtx, auctionLinkKey(adapter, tracingAuctionID), time.Hour)
	wantTraceID := trace.SpanContextFromContext(offerCtx).TraceID().String()
	endOffer(nil)

	s.reconcileOffers(solverContext(t, s), []Target{{Adapter: adapter}})

	var stamped bool
	for _, line := range lines {
		if strings.Contains(line, `"offerId"`) && strings.Contains(line, `"quoteTraceId":"`+wantTraceID+`"`) {
			stamped = true
		}
	}
	if !stamped {
		t.Fatalf("no listed-offer line carried quoteTraceId %s: %v", wantTraceID, lines)
	}
}

// Trace loggers are derived from the base logger at each span-starting site, never from an
// already-derived one: re-deriving appends a second trace_id/span_id pair to every line.
func TestDiscoverAndOfferLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	fixture := newTracingFixture(t, stubOfferStrategy{offers: tracingOffer()}, http.StatusCreated)
	var lines []string
	fixture.solver.log = funcr.NewJSON(
		func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1},
	)

	fixture.solver.discoverAndOffer(solverContext(t, fixture.solver))

	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	var sawOfferLine bool
	for _, line := range lines {
		if n := strings.Count(line, `"trace_id"`); n > 1 {
			t.Fatalf("trace_id appears %d times in %s", n, line)
		}
		if n := strings.Count(line, `"span_id"`); n > 1 {
			t.Fatalf("span_id appears %d times in %s", n, line)
		}
		if strings.Contains(line, `"auctionId"`) && strings.Contains(line, `"trace_id"`) {
			sawOfferLine = true
		}
	}
	if !sawOfferLine {
		t.Fatalf("no offer-path line carried trace_id: %v", lines)
	}
}

// Tracing is off by default: no provider is installed here, so every span is a no-op and the offer
// pass must behave exactly as it does without tracing.
func TestDiscoverAndOfferWithTracingDisabled(t *testing.T) {
	fixture := newTracingFixture(t, stubOfferStrategy{offers: tracingOffer()}, http.StatusCreated)

	fixture.solver.discoverAndOffer(t.Context())

	if got := fixture.createCalls.Load(); got != 1 {
		t.Fatalf("createOffer calls = %d, want 1", got)
	}
	if _, ok := fixture.solver.links.Lookup(requestLinkKey(fixture.request)); ok {
		t.Fatal("a no-op span context must not be remembered")
	}
}
