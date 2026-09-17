package uniswapx

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const (
	inboundTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	inboundTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	tracingStrategy    = "default"
)

// spanRecorder is the slice of tracetest.SpanRecorder these assertions need.
type spanRecorder interface {
	Ended() []sdktrace.ReadOnlySpan
}

func requireSpans(t *testing.T, rec spanRecorder, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(rec.Ended()))
	for _, name := range tracetest.Names(rec) {
		got[name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("missing span %q; ended spans: %v", name, tracetest.Names(rec))
		}
	}
}

func postQuote(t *testing.T, handler http.Handler, request quoteRequest) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	httpRequest := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/quote", bytes.NewReader(encoded),
	)
	httpRequest.Header.Set("traceparent", inboundTraceparent)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httpRequest)
	return response
}

func newTracingQuoteSolver(t *testing.T, tokenIn common.Address, strategy strategytypes.Strategy) *Solver {
	t.Helper()
	solver := newBlockingQuoteTestSolver(t, tokenIn, strategy)
	solver.cfg.QuoteServer = QuoteServerConfig{HTTPTimeout: time.Second}
	solver.cfg.Strategy = StrategyConfig{Name: tracingStrategy}
	solver.links = observability.NewSpanLinks(0)
	solver.log = logr.Discard()
	return solver
}

// TestQuoteServerTracing pins the inbound quote trace: the server span continues Uniswap's trace,
// every stage span belongs to it, the identifiers are searchable, and the served quote's span
// context is remembered so the fill that wins it can link back (spec §9.4, §12).
func TestQuoteServerTracing(t *testing.T) {
	rec := tracetest.Install(t)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{quote: &strategytypes.Quote{AmountIn: big.NewInt(100), AmountOut: big.NewInt(90)}}
	solver := newTracingQuoteSolver(t, tokenIn, strategy)
	request := validQuoteRequest(tokenIn, tokenOut)

	response := postQuote(t, solver.newQuoteHTTPServer(t.Context()).Handler, request)

	if response.Code != http.StatusOK {
		t.Fatalf("quote = %d, want 200 (body %s)", response.Code, response.Body.String())
	}
	for _, span := range rec.Ended() {
		if got := span.SpanContext().TraceID().String(); got != inboundTraceID {
			t.Fatalf("span %s is in trace %s, want the inbound trace %s", span.Name(), got, inboundTraceID)
		}
	}
	requireSpans(t, rec, "POST /quote", "uniswapx.quote", "uniswapx.quote.decide")

	server := tracetest.Ended(t, rec, "POST /quote")
	if got := tracetest.Attr(server, "quote.id"); got != request.QuoteID {
		t.Fatalf("server span quote.id = %q, want %q", got, request.QuoteID)
	}
	if got := tracetest.Attr(server, "request.id"); got != request.RequestID {
		t.Fatalf("server span request.id = %q, want %q", got, request.RequestID)
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "uniswapx.quote.decide"), "strategy.name"); got != tracingStrategy {
		t.Fatalf("decide span strategy.name = %q, want %q", got, tracingStrategy)
	}
	if _, ok := solver.links.Lookup(request.QuoteID); !ok {
		t.Fatal("quote span context not remembered for linking")
	}
}

// A quote this filler will not serve is not a failure: the span declines instead of erroring, and
// nothing is remembered for a quote that never went out.
func TestQuoteServerTracingDeclines(t *testing.T) {
	rec := tracetest.Install(t)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	strategy := &quoteTestStrategy{}
	solver := newTracingQuoteSolver(t, tokenIn, strategy)
	request := validQuoteRequest(tokenIn, tokenOut)

	if response := postQuote(t, solver.newQuoteHTTPServer(t.Context()).Handler, request); response.Code != http.StatusNoContent {
		t.Fatalf("declined quote = %d, want 204", response.Code)
	}

	span := tracetest.Ended(t, rec, "uniswapx.quote")
	if span.Status().Code == codes.Error {
		t.Fatalf("a declined quote must not be an error span: %v", span.Status())
	}
	events := span.Events()
	if len(events) != 1 || events[0].Name != "declined" {
		t.Fatalf("events = %v, want one declined event", events)
	}
	if _, ok := solver.links.Lookup(request.QuoteID); ok {
		t.Fatal("a quote that was never served must not be remembered")
	}
}

// panicQuoteStrategy stands in for a decider that blows up mid-request — the webhook strategy calls
// out over HTTP, so this is reachable in production and is recovered by the quote server middleware.
type panicQuoteStrategy struct{}

func (panicQuoteStrategy) DecideQuote(context.Context, strategytypes.QuoteInput) (*strategytypes.Quote, error) {
	panic("strategy exploded")
}

func (panicQuoteStrategy) DecideFill(context.Context, strategytypes.FillInput) (*strategytypes.FillPlan, error) {
	return nil, nil
}

// A recovered panic must not leak spans: the quote server completes the request, so any span left
// open by the unwound stack would never be exported.
func TestQuoteServerTracingEndsSpansOnPanic(t *testing.T) {
	rec := tracetest.Install(t)
	tokenIn := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tokenOut := common.HexToAddress("0x2222222222222222222222222222222222222222")
	solver := newTracingQuoteSolver(t, tokenIn, panicQuoteStrategy{})

	response := postQuote(t, solver.newQuoteHTTPServer(t.Context()).Handler, validQuoteRequest(tokenIn, tokenOut))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("panicking quote = %d, want 500 (body %s)", response.Code, response.Body.String())
	}
	requireSpans(t, rec, "POST /quote", "uniswapx.quote", "uniswapx.quote.decide")
}

// A malformed body is the caller's fault, so the server span declines rather than erroring.
func TestQuoteServerTracingDeclinesMalformedBody(t *testing.T) {
	rec := tracetest.Install(t)
	solver := newTracingQuoteSolver(
		t, common.HexToAddress("0x1111111111111111111111111111111111111111"), &quoteTestStrategy{},
	)
	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, "/quote", bytes.NewBufferString(`{`),
	)
	request.Header.Set("traceparent", inboundTraceparent)
	response := httptest.NewRecorder()

	solver.newQuoteHTTPServer(t.Context()).Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed quote = %d, want 400", response.Code)
	}
	server := tracetest.Ended(t, rec, "POST /quote")
	if server.Status().Code == codes.Error {
		t.Fatalf("a malformed request must not be an error span: %v", server.Status())
	}
	if len(server.Events()) != 1 || server.Events()[0].Name != "declined" {
		t.Fatalf("events = %v, want one declined event", server.Events())
	}
}

// Each quote-state refresh roots its own trace: it runs on the solver's own loop, never under a
// request, and its chain reads belong to it.
func TestQuoteRefreshRootsItsOwnTrace(t *testing.T) {
	rec := tracetest.Install(t)
	solver := quoteModeSolver(
		&quoteModeReader{now: time.Unix(1_000, 0)}, &fakeDiscountProvider{list: &liquiddiscounts.List{}},
	)

	if err := solver.refreshQuoteState(t.Context(), nil); err != nil {
		t.Fatalf("refreshQuoteState: %v", err)
	}

	refresh := tracetest.Ended(t, rec, "uniswapx.quote_refresh")
	if refresh.Parent().IsValid() {
		t.Fatalf("quote refresh span has parent %v, want a root", refresh.Parent())
	}
	if got := tracetest.Attr(refresh, "solver"); got != Name {
		t.Fatalf("quote refresh span solver = %q, want %q", got, Name)
	}
}

// tracingFillFixture drives one order from the order poll through the fill loop to completion, the
// way the solver's goroutines do in production.
type tracingFillFixture struct {
	solver *Solver
	route  liquidlane.Route
	txm    *executionTestTxManager
	entry  orderEntry
}

func newTracingFillFixture(t *testing.T) *tracingFillFixture {
	t.Helper()
	direct := newDirectExecutionFixture(t)
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	solver := direct.solver
	solver.chainID = 1
	solver.cfg.Reactor = common.HexToAddress("0x6666666666666666666666666666666666666666")
	solver.cfg.TokenPolicy = policy
	solver.cfg.Strategy = StrategyConfig{Name: tracingStrategy}
	solver.cfg.OrderServer.Sources.ExclusiveV2 = true
	solver.links = observability.NewSpanLinks(0)
	entry := tracingOrderEntry(t, solver.cfg, direct.order.TokenIn, direct.order.TokenOut, direct.now)
	solver.orders = orderPollerFunc(func(context.Context, int64, *common.Address) ([]orderEntry, error) {
		return []orderEntry{entry}, nil
	})
	return &tracingFillFixture{solver: solver, route: direct.route, txm: direct.txm, entry: entry}
}

// run polls the order, hands it to the fill loop, and confirms its transaction.
func (f *tracingFillFixture) run(t *testing.T) {
	t.Helper()
	f.runWithResult(t, txmanager.Result{Hash: tracingFillTxHash, Outcome: txmanager.OutcomeConfirmed})
}

// runWithResult is run with the transaction outcome the manager reports back.
func (f *tracingFillFixture) runWithResult(t *testing.T, result txmanager.Result) {
	t.Helper()
	ctx := f.loopContext(t)
	orders := make(chan *resolvedOrder, 1)
	if _, err := f.solver.pollSource(
		ctx, orderSourceExclusiveV2, &f.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatalf("pollSource: %v", err)
	}
	close(orders)
	accepted := make(chan struct{}, 1)
	f.txm.accepted = accepted
	done := make(chan error, 1)
	go func() { done <- f.solver.fillLoop(ctx, []liquidlane.Route{f.route}, orders) }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("fill was not submitted")
	}
	f.txm.complete(result)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fill loop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fill loop did not finish")
	}
}

// runUnfilled polls the order and lets the fill loop drain it without a transaction, for the paths
// that never reach submission.
func (f *tracingFillFixture) runUnfilled(t *testing.T) {
	t.Helper()
	ctx := f.loopContext(t)
	orders := make(chan *resolvedOrder, 1)
	if _, err := f.solver.pollSource(
		ctx, orderSourceExclusiveV2, &f.solver.cfg.Executor, orders,
	); err != nil {
		t.Fatalf("pollSource: %v", err)
	}
	close(orders)
	if err := f.solver.fillLoop(ctx, []liquidlane.Route{f.route}, orders); err != nil {
		t.Fatalf("fill loop: %v", err)
	}
	if len(f.txm.reqs) != 0 {
		t.Fatalf("transactions sent = %d, want none", len(f.txm.reqs))
	}
}

// loopContext stands in for Run, which stores the solver logger on the context it hands the loops.
func (f *tracingFillFixture) loopContext(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), f.solver.log)
}

var tracingFillTxHash = common.HexToHash("0x2")

// tracingOrderEntry builds an open exclusive V2 order for this filler that resolves to 100 in and
// 90 out, matching the direct execution fixture's snapshot and plan.
func tracingOrderEntry(
	t *testing.T, cfg *Config, tokenIn, tokenOut common.Address, now time.Time,
) orderEntry {
	t.Helper()
	cosignerKey, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	recipient := common.HexToAddress("0x7777777777777777777777777777777777777777")
	order := v2Order{
		Info: v2OrderInfo{
			Reactor: cfg.Reactor, Swapper: recipient, Nonce: big.NewInt(1),
			Deadline: big.NewInt(now.Add(time.Minute).Unix()), AdditionalValidationData: []byte{},
		},
		Cosigner:  crypto.PubkeyToAddress(cosignerKey.PublicKey),
		BaseInput: v2Input{Token: tokenIn, StartAmount: big.NewInt(100), EndAmount: big.NewInt(100)},
		BaseOutputs: []v2Output{{
			Token: tokenOut, StartAmount: big.NewInt(90), EndAmount: big.NewInt(90), Recipient: recipient,
		}},
		CosignerData: v2CosignerData{
			DecayStartTime:  big.NewInt(now.Add(-time.Second).Unix()),
			DecayEndTime:    big.NewInt(now.Add(30 * time.Second).Unix()),
			ExclusiveFiller: cfg.Executor, ExclusivityOverrideBps: new(big.Int),
			InputOverride: new(big.Int), OutputOverrides: []*big.Int{new(big.Int)},
		},
		Cosignature: make([]byte, 65),
	}
	hash, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	cosignerData, err := v2CosignerDataArguments.Pack(order.CosignerData)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature, err = crypto.Sign(crypto.Keccak256(hash.Bytes(), cosignerData), cosignerKey)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature[64] += 27
	encoded, err := v2OrderArguments.Pack(order)
	if err != nil {
		t.Fatal(err)
	}
	return orderEntry{
		Type: orderTypeDutchV2, EncodedOrder: hexutil.Encode(encoded), Signature: "0x01",
		OrderHash: hash.Hex(), OrderStatus: orderStatusOpen, ChainID: 1, QuoteID: "quote-1",
		Input: orderToken{Token: tokenIn.Hex(), StartAmount: "100", EndAmount: "100"},
		Outputs: []orderOutput{{
			Token: tokenOut.Hex(), StartAmount: "90", EndAmount: "90", Recipient: recipient.Hex(),
		}},
	}
}

// TestOrderTraceLinksQuoteToFill pins the fill trace: the accepted order carries its span context
// through the orders channel, the fill continues that trace, and the order links back to the quote
// this process served (spec §9.4, §12).
func TestOrderTraceLinksQuoteToFill(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFillFixture(t)
	var lines []string
	fixture.solver.log = funcr.NewJSON(
		func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1},
	)
	// Stand in for quoteHandler having served quote-1 earlier, in its own trace.
	quoteCtx, endQuote := tracer.Start(t.Context(), "uniswapx.quote")
	fixture.solver.links.Remember(quoteCtx, fixture.entry.QuoteID, time.Minute)
	endQuote(nil)
	quoteTraceID := trace.SpanContextFromContext(quoteCtx).TraceID().String()

	fixture.run(t)

	requireSpans(t, rec,
		"uniswapx.orders.poll", "uniswapx.order.track", "uniswapx.fill",
		"uniswapx.fill.plan", "uniswapx.fill.build", "uniswapx.fill.submit", "uniswapx.fill.complete",
	)
	track := tracetest.Ended(t, rec, "uniswapx.order.track")
	if len(track.Links()) != 1 || track.Links()[0].SpanContext.TraceID().String() != quoteTraceID {
		t.Fatalf("track span links = %v, want one link to trace %s", track.Links(), quoteTraceID)
	}
	if got := tracetest.Attr(track, "order.hash"); got != fixture.entry.OrderHash {
		t.Fatalf("track span order.hash = %q, want %s", got, fixture.entry.OrderHash)
	}
	if got := tracetest.Attr(track, "quote.id"); got != fixture.entry.QuoteID {
		t.Fatalf("track span quote.id = %q, want %s", got, fixture.entry.QuoteID)
	}
	if got := tracetest.Attr(track, "quote.trace_id"); got != quoteTraceID {
		t.Fatalf("track span quote.trace_id = %q, want %s", got, quoteTraceID)
	}
	fill := tracetest.Ended(t, rec, "uniswapx.fill")
	if got, want := fill.SpanContext().TraceID(), track.SpanContext().TraceID(); got != want {
		t.Fatalf("fill trace = %s, want the track span's trace %s", got, want)
	}
	if got := tracetest.Attr(fill, "tx.hash"); got != tracingFillTxHash.Hex() {
		t.Fatalf("fill span tx.hash = %q, want %s", got, tracingFillTxHash.Hex())
	}
	complete := tracetest.Ended(t, rec, "uniswapx.fill.complete")
	if got := tracetest.Attr(complete, "tx.hash"); got != tracingFillTxHash.Hex() {
		t.Fatalf("complete span tx.hash = %q, want %s", got, tracingFillTxHash.Hex())
	}
	if got := tracetest.Attr(complete, "tx.outcome"); got != string(txmanager.OutcomeConfirmed) {
		t.Fatalf("complete span tx.outcome = %q, want confirmed", got)
	}
	if got := tracetest.Attr(tracetest.Ended(t, rec, "uniswapx.fill.plan"), "strategy.name"); got != tracingStrategy {
		t.Fatalf("plan span strategy.name = %q, want %q", got, tracingStrategy)
	}
	// The link's trace id rides on the order, so the fill and its completion log it too — not just
	// the trackOrder logger that resolved it.
	requireLoggedQuoteTrace(t, lines, quoteTraceID, "order fill submitted", "order filled")
}

// requireLoggedQuoteTrace asserts that each named log message carried the quote's trace id.
func requireLoggedQuoteTrace(t *testing.T, lines []string, quoteTraceID string, messages ...string) {
	t.Helper()
	for _, message := range messages {
		var found bool
		for _, line := range lines {
			if strings.Contains(line, message) {
				found = true
				if !strings.Contains(line, `"quoteTraceId":"`+quoteTraceID+`"`) {
					t.Fatalf("%q line carries no quoteTraceId %s: %s", message, quoteTraceID, line)
				}
			}
		}
		if !found {
			t.Fatalf("no %q line was logged: %v", message, lines)
		}
	}
}

// A fill the manager rejected before broadcasting has no transaction: the outcome is recorded,
// tx.hash is left off rather than stamped as the zero hash.
func TestFillCompletionOmitsTxHashWhenNotBroadcast(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFillFixture(t)

	fixture.runWithResult(t, txmanager.Result{
		Outcome: txmanager.OutcomeSubmissionError,
		Err:     errors.New("insufficient funds for gas * price + value"),
	})

	for _, name := range []string{"uniswapx.fill", "uniswapx.fill.complete"} {
		span := tracetest.Ended(t, rec, name)
		if got := tracetest.Attr(span, "tx.hash"); got != "" {
			t.Fatalf("%s tx.hash = %q, want no attribute for a transaction that never went out", name, got)
		}
		if got := tracetest.Attr(span, "tx.outcome"); got != string(txmanager.OutcomeSubmissionError) {
			t.Fatalf("%s tx.outcome = %q, want %s", name, got, txmanager.OutcomeSubmissionError)
		}
	}
}

// A quote nobody remembered (restart, eviction, tracing off) costs the link and nothing else: the
// order still fills, and the miss is named on the span (spec §12).
func TestOrderTraceRecordsLinkMiss(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFillFixture(t)

	fixture.run(t)

	track := tracetest.Ended(t, rec, "uniswapx.order.track")
	if len(track.Links()) != 0 {
		t.Fatalf("track span links = %v, want none", track.Links())
	}
	var misses int
	for _, event := range track.Events() {
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
		if key != fixture.entry.QuoteID {
			t.Fatalf("link_miss key = %q, want %s", key, fixture.entry.QuoteID)
		}
	}
	if misses != 1 {
		t.Fatalf("link_miss events = %d, want 1 (events %v)", misses, track.Events())
	}
	requireSpans(t, rec, "uniswapx.fill", "uniswapx.fill.complete")
	if _, filled := fixture.solver.filled[common.HexToHash(fixture.entry.OrderHash)]; !filled {
		t.Fatal("unlinked order did not fill")
	}
}

// An order this solver will not fill is the ordinary outcome of a poll, not a failure: the fill loop
// logs it at V(1) and retries later, so the span declines instead of erroring.
func TestFillTraceDeclinesUnfillableOrder(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFillFixture(t)
	fixture.solver.strategy = &executionTestStrategy{} // no plan: nothing worth filling

	fixture.runUnfilled(t)

	fill := tracetest.Ended(t, rec, "uniswapx.fill")
	if fill.Status().Code == codes.Error {
		t.Fatalf("an unfillable order must not be an error span: %v", fill.Status())
	}
	if !hasSpanEvent(fill, "declined") {
		t.Fatalf("fill span has no declined event: %v", fill.Events())
	}
	if hasSpanEvent(fill, "exception") {
		t.Fatalf("an unfillable order recorded an exception: %v", fill.Events())
	}
}

// The other half of the classification: a fill we tried and could not send is a real failure and has
// to surface as an error span with the cause recorded.
func TestFillTraceRecordsPreflightFailure(t *testing.T) {
	rec := tracetest.Install(t)
	fixture := newTracingFillFixture(t)
	preflightErr := errors.New("execution reverted")
	fixture.solver.chain = contractCallerFunc(
		func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) { return nil, preflightErr },
	)

	fixture.runUnfilled(t)

	fill := tracetest.Ended(t, rec, "uniswapx.fill")
	if fill.Status().Code != codes.Error {
		t.Fatalf("fill span status = %v, want an error", fill.Status())
	}
	if !hasSpanEvent(fill, "exception") {
		t.Fatalf("failed fill recorded no exception: %v", fill.Events())
	}
	if !strings.Contains(fill.Status().Description, preflightErr.Error()) {
		t.Fatalf("fill span status %q does not name the preflight failure", fill.Status().Description)
	}
}

func hasSpanEvent(s sdktrace.ReadOnlySpan, name string) bool {
	for _, event := range s.Events() {
		if event.Name == name {
			return true
		}
	}
	return false
}

// Trace loggers are derived from the base logger at each span-starting site, never from an
// already-derived one: re-deriving appends a second trace_id/span_id pair to every line.
func TestOrderLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	fixture := newTracingFillFixture(t)
	var lines []string
	fixture.solver.log = funcr.NewJSON(
		func(entry string) { lines = append(lines, entry) }, funcr.Options{Verbosity: 1},
	)

	fixture.run(t)

	if len(lines) == 0 {
		t.Fatal("no log output captured")
	}
	var sawOrderLine, sawAdmissionLine bool
	for _, line := range lines {
		if n := strings.Count(line, `"trace_id"`); n > 1 {
			t.Fatalf("trace_id appears %d times in %s", n, line)
		}
		if n := strings.Count(line, `"span_id"`); n > 1 {
			t.Fatalf("span_id appears %d times in %s", n, line)
		}
		if strings.Contains(line, `"orderHash"`) && strings.Contains(line, `"trace_id"`) {
			sawOrderLine = true
		}
		// The fill loop's admission lines log from the order's track span, so they carry trace ids
		// even though the loop itself runs under no span of its own.
		if strings.Contains(line, "order fill planning started") && strings.Contains(line, `"trace_id"`) {
			sawAdmissionLine = true
		}
	}
	if !sawOrderLine {
		t.Fatalf("no order-path line carried trace_id: %v", lines)
	}
	if !sawAdmissionLine {
		t.Fatalf("the fill-admission line carried no trace_id: %v", lines)
	}
}

// Tracing is off by default: no provider is installed here, so every span is a no-op and the fill
// must behave exactly as it does without tracing.
func TestOrderFillsWithTracingDisabled(t *testing.T) {
	fixture := newTracingFillFixture(t)

	fixture.run(t)

	if _, filled := fixture.solver.filled[common.HexToHash(fixture.entry.OrderHash)]; !filled {
		t.Fatal("order did not fill with tracing disabled")
	}
	if fixture.solver.capacity.Len() != 0 {
		t.Fatal("pending reservation was not released")
	}
}
