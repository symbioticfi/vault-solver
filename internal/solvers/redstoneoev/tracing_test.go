package redstoneoev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/codes"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func noSpan(t *testing.T, rec tracetest.Recorder, name string) {
	t.Helper()
	if got := tracetest.AllEnded(rec, name); len(got) != 0 {
		t.Fatalf("span %q was started, want none; ended spans: %v", name, tracetest.Names(rec))
	}
}

// tracedAuction returns a seeded solver plus a freshly emitted, liquidatable auction frame — the
// fixture every auction-path tracing test starts from.
func tracedAuction(t *testing.T) (*Solver, AuctionMessage) {
	t.Helper()
	s, _ := seededSolver(t)
	a := decodeAuction(t)
	setAuctionPrice(&a, seedLiquidatablePrice)
	a.Timestamp = time.Now().UnixMilli()
	setSnapshotBlockTime(t, s, a.Timestamp)
	return s, a
}

var errStubStrategy = errors.New("stub strategy failure")

// stubBidStrategy fails or blows up the way a misbehaving strategy can, so the auction spans can be
// asserted on the non-happy paths.
type stubBidStrategy struct {
	err    error
	panics bool
}

func (s stubBidStrategy) Run(context.Context) {}

func (s stubBidStrategy) DecideBid(context.Context, types.BidInput) (types.BidOutput, error) {
	if s.panics {
		panic("strategy exploded")
	}
	return types.BidOutput{}, s.err
}

// One auction is one trace: the bid decision, every default-strategy stage and the outbound solve are
// spans beneath it, and a sent solve remembers the auction span for the result frames (spec §9.4/§12).
func TestAuctionTraceSpanTree(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)

	s.handleAuctionWithContext(t.Context(), marshal(a))

	if frame := drainSend(s); frame == nil {
		t.Fatal("expected a solve to be enqueued for a liquidatable auction")
	}
	auction := tracetest.Ended(t, rec, "oev.auction")
	if auction.Parent().IsValid() {
		t.Fatalf("oev.auction parent = %s, want a root span", auction.Parent().SpanID())
	}
	if got := tracetest.Attr(auction, "auction.id"); got != a.ID {
		t.Fatalf("auction.id = %q, want %q", got, a.ID)
	}
	if got := tracetest.Attr(auction, "solver"); got != Name {
		t.Fatalf("solver = %q, want %q", got, Name)
	}

	bid := tracetest.Ended(t, rec, "oev.auction.bid")
	tracetest.RequireChildOf(t, bid, auction)
	if got := tracetest.Attr(bid, "strategy.name"); got != defaultStrategyName {
		t.Fatalf("strategy.name = %q, want %q", got, defaultStrategyName)
	}
	for _, stage := range []string{
		"oev.auction.candidates", "oev.auction.size", "oev.auction.bundle", "oev.auction.economics",
	} {
		span := tracetest.Ended(t, rec, stage)
		tracetest.RequireChildOf(t, span, bid)
		if got := tracetest.Attr(span, "solver"); got != Name {
			t.Fatalf("%s solver = %q, want %q", stage, got, Name)
		}
	}

	send := tracetest.Ended(t, rec, "oev.auction.send")
	tracetest.RequireChildOf(t, send, auction)
	if tracetest.HasEvent(send, "declined") {
		t.Fatal("an accepted solve must not be declined")
	}
	tracetest.RequireNoErrorSpans(t, rec)

	if _, ok := s.links.Lookup(a.ID); !ok {
		t.Fatalf("auction %q was not remembered for result linking", a.ID)
	}
}

// The auction result arrives in its own trace, so it links back to the bid that produced it and
// carries the auction's trace id for log joins (spec §12.2).
func TestAuctionResultLinksToAuctionSpan(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)

	s.handleAuctionWithContext(t.Context(), marshal(a))
	if frame := drainSend(s); frame == nil {
		t.Fatal("expected a solve to be enqueued")
	}
	auction := tracetest.Ended(t, rec, "oev.auction")

	s.handleMessage(t.Context(), marshal(AuctionResult{
		Op: "auction-result", ID: a.ID,
		Data: AuctionResultData{Bid: "0.0005", Liquidator: seedCallback.Hex()},
	}))

	result := tracetest.Ended(t, rec, "oev.auction.result")
	if got := len(result.Links()); got != 1 {
		t.Fatalf("result links = %d, want 1", got)
	}
	if got, want := result.Links()[0].SpanContext.SpanID(), auction.SpanContext().SpanID(); got != want {
		t.Fatalf("result link = %s, want the auction span %s", got, want)
	}
	if got := tracetest.Attr(result, "auction.id"); got != a.ID {
		t.Fatalf("auction.id = %q, want %q", got, a.ID)
	}
	if got := tracetest.Attr(result, "oev.won"); got != "true" {
		t.Fatalf("oev.won = %q, want true", got)
	}
	if got, want := tracetest.Attr(result, "quote.trace_id"), auction.SpanContext().TraceID().String(); got != want {
		t.Fatalf("quote.trace_id = %q, want %q", got, want)
	}
	if tracetest.HasEvent(result, "link_miss") {
		t.Fatal("a resolved link must not record link_miss")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// Linking is best effort: a result for an auction this process never bid on is traced without a link
// and still drives the reservation state exactly as before (spec §12).
func TestAuctionResultLinkMissStaysInert(t *testing.T) {
	rec := tracetest.Install(t)
	s, _ := seededSolver(t)
	s.reserve(8, time.Now(), "unknown-auction", mustBig("500000000000000"))

	s.handleMessage(t.Context(), marshal(AuctionResult{
		Op: "auction-result", ID: "unknown-auction",
		Data: AuctionResultData{Bid: "0.0005", Liquidator: "0x0000000000000000000000000000000000000001"},
	}))

	result := tracetest.Ended(t, rec, "oev.auction.result")
	if got := len(result.Links()); got != 0 {
		t.Fatalf("result links = %d, want 0", got)
	}
	if key, got := tracetest.EventAttr(result, "link_miss", "key"); got != 1 || key != "unknown-auction" {
		t.Fatalf("link_miss events = %d with key %q, want 1 with unknown-auction", got, key)
	}
	tracetest.RequireNoAttr(t, result, "quote.trace_id")
	if pending := s.inFlightSnapshot().pending; len(pending) != 0 {
		t.Fatalf("a lost auction result must release the reservation, pending = %v", pending)
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// Our own liquidation result carries the settlement transaction, and blacklisting is an expected halt
// rather than a span error.
func TestLiquidationAndBlacklistSpans(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)

	s.handleAuctionWithContext(t.Context(), marshal(a))
	if frame := drainSend(s); frame == nil {
		t.Fatal("expected a solve to be enqueued")
	}
	auction := tracetest.Ended(t, rec, "oev.auction")

	const txHash = "0x00000000000000000000000000000000000000000000000000000000000000ab"
	s.handleMessage(t.Context(), marshal(LiquidationResult{
		Op: "liquidation-result", ID: a.ID,
		Data: LiquidationResultData{Success: true, TxHash: txHash, Liquidator: seedCallback.Hex()},
	}))
	liquidation := tracetest.Ended(t, rec, "oev.liquidation.result")
	if got := len(liquidation.Links()); got != 1 {
		t.Fatalf("liquidation links = %d, want 1", got)
	}
	if got, want := liquidation.Links()[0].SpanContext.SpanID(), auction.SpanContext().SpanID(); got != want {
		t.Fatalf("liquidation link = %s, want the auction span %s", got, want)
	}
	if got := tracetest.Attr(liquidation, "tx.hash"); got != txHash {
		t.Fatalf("tx.hash = %q, want %q", got, txHash)
	}

	s.handleMessage(t.Context(), marshal(Blacklisted{
		Op: "blacklisted", ID: a.ID, Data: BlacklistedData{Msg: "key revoked"},
	}))
	blacklisted := tracetest.Ended(t, rec, "oev.blacklisted")
	if got := tracetest.Attr(blacklisted, "auction.id"); got != a.ID {
		t.Fatalf("auction.id = %q, want %q", got, a.ID)
	}
	if !tracetest.HasEvent(blacklisted, "declined") {
		t.Fatal("a blacklist halt is an expected outcome, want a declined event")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// A full outbound queue is an expected outcome of a bounded hot path, not a failure.
func TestDroppedSolveDeclinesSendSpan(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)
	for range cap(s.ws.send) {
		if !s.ws.Send([]byte("occupied")) {
			t.Fatal("failed to fill the send buffer")
		}
	}

	s.handleAuctionWithContext(t.Context(), marshal(a))

	send := tracetest.Ended(t, rec, "oev.auction.send")
	if !tracetest.HasEvent(send, "declined") {
		t.Fatal("a dropped solve must record a declined event")
	}
	tracetest.RequireNoErrorSpans(t, rec)
	if _, ok := s.links.Lookup(a.ID); ok {
		t.Fatal("a dropped solve must not be remembered for result linking")
	}
}

// An auction whose deadline has already passed is declined on its own span and never reaches the bid.
func TestTooLateAuctionDeclinesAuctionSpan(t *testing.T) {
	rec := tracetest.Install(t)
	s, _ := seededSolver(t)

	s.handleAuctionWithContext(t.Context(), marshal(decodeAuction(t)))

	auction := tracetest.Ended(t, rec, "oev.auction")
	if !tracetest.HasEvent(auction, "declined") {
		t.Fatal("a too-late auction must record a declined event")
	}
	noSpan(t, rec, "oev.auction.bid")
	tracetest.RequireNoErrorSpans(t, rec)
}

// A strategy that fails is a real error: the bid span records it so it surfaces in the trace backend.
func TestStrategyFailureRecordsSpanError(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)
	s.strategy = stubBidStrategy{err: errStubStrategy}

	s.handleAuctionWithContext(t.Context(), marshal(a))

	bid := tracetest.Ended(t, rec, "oev.auction.bid")
	if bid.Status().Code != codes.Error {
		t.Fatalf("bid span status = %v, want error", bid.Status().Code)
	}
	if !tracetest.HasEvent(bid, "exception") {
		t.Fatal("a strategy failure must record an exception event")
	}
	if got := tracetest.Ended(t, rec, "oev.auction").Status().Code; got != codes.Error {
		t.Fatalf("auction span status = %v, want error", got)
	}
}

// A panicking strategy must not leak open spans: every end is deferred, so the spans close while the
// panic unwinds.
func TestStrategyPanicStillEndsAuctionSpans(t *testing.T) {
	rec := tracetest.Install(t)
	s, a := tracedAuction(t)
	s.strategy = stubBidStrategy{panics: true}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected the stub strategy to panic")
			}
		}()
		s.handleAuctionWithContext(t.Context(), marshal(a))
	}()

	tracetest.Ended(t, rec, "oev.auction")
	tracetest.Ended(t, rec, "oev.auction.bid")
}

// The handshake carries traceparent so a connection is findable in the trace backend, and the shared
// header the client was built with is never mutated (spec §6.4).
func TestFeedDialInjectsTraceparent(t *testing.T) {
	rec := tracetest.Install(t)
	headers := make(chan http.Header, 1)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Clone()
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	client := newWSClient(wsConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), APIKey: "k", Topics: []string{"t"},
	}, logr.Discard(), func(context.Context, []byte) {}, nil)

	conn, err := client.dial(t.Context())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close() //nolint:errcheck // test teardown

	connect := tracetest.Ended(t, rec, "oev.feed.connect")
	if got := tracetest.Attr(connect, "solver"); got != Name {
		t.Fatalf("solver = %q, want %q", got, Name)
	}
	got := <-headers
	if got.Get("x-api-key") != "k" {
		t.Fatalf("handshake api key = %q, want the configured key", got.Get("x-api-key"))
	}
	if tp := got.Get("traceparent"); !strings.Contains(tp, connect.SpanContext().TraceID().String()) {
		t.Fatalf("handshake traceparent = %q, want the connect trace %s", tp, connect.SpanContext().TraceID())
	}
	if client.header.Get("traceparent") != "" {
		t.Fatal("the shared handshake header must not be mutated by a dial")
	}
	tracetest.RequireNoErrorSpans(t, rec)
}

// Bid-path log lines join the trace exactly once and name the auction with one key everywhere.
func TestBidPathLogsCarryTraceIDOnce(t *testing.T) {
	tracetest.Install(t)
	s, a := tracedAuction(t)
	var mu sync.Mutex
	var lines []string
	s.log = funcr.New(func(_, args string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, args)
	}, funcr.Options{Verbosity: 1})

	s.handleAuctionWithContext(t.Context(), marshal(a))
	if frame := drainSend(s); frame == nil {
		t.Fatal("expected a solve to be enqueued")
	}

	mu.Lock()
	defer mu.Unlock()
	enqueued := ""
	for _, line := range lines {
		if strings.Contains(line, "bid enqueued") {
			enqueued = line
		}
		if strings.Contains(line, `"auction"=`) {
			t.Fatalf("bid-path line still uses the auction log key: %s", line)
		}
	}
	if enqueued == "" {
		t.Fatalf("no bid-enqueued log line; lines: %v", lines)
	}
	if got := strings.Count(enqueued, "trace_id"); got != 1 {
		t.Fatalf("trace_id occurrences = %d, want 1: %s", got, enqueued)
	}
	if !strings.Contains(enqueued, `"auctionId"=`) {
		t.Fatalf("bid-enqueued line does not name auctionId: %s", enqueued)
	}
}

// Tracing is optional: with no provider installed the auction path behaves exactly as before.
func TestAuctionPathUnchangedWithTracingDisabled(t *testing.T) {
	s, a := tracedAuction(t)

	s.handleAuctionWithContext(t.Context(), marshal(a))

	if frame := drainSend(s); frame == nil {
		t.Fatal("expected a solve to be enqueued with tracing disabled")
	}
	if pending := s.inFlightSnapshot().pending; len(pending) != 1 || pending[0].ID != a.ID {
		t.Fatalf("reservation = %v, want the sent auction", pending)
	}
	if _, ok := s.links.Lookup(a.ID); ok {
		t.Fatal("a span-less auction must not be remembered")
	}
	s.handleMessage(t.Context(), marshal(AuctionResult{
		Op: "auction-result", ID: a.ID,
		Data: AuctionResultData{Bid: "0.0005", Liquidator: seedCallback.Hex()},
	}))
	if pending := s.inFlightSnapshot().pending; len(pending) != 1 || !pending[0].Won {
		t.Fatalf("won reservation = %v, want the auction marked won", pending)
	}
}

// A liquidation of ours that failed is a failure the metrics and breaker count, so its result span
// records it; another liquidator's failure is not ours to report.
func TestLiquidationResultSpanReportsOurFailure(t *testing.T) {
	const other = "0x0000000000000000000000000000000000000001"
	cases := []struct {
		name, liquidator, errText string
		success, wantError        bool
	}{
		{name: "ours failed", liquidator: seedCallback.Hex(), errText: "execution reverted", wantError: true},
		{name: "ours failed without a message", liquidator: seedCallback.Hex(), wantError: true},
		{name: "ours succeeded", liquidator: seedCallback.Hex(), success: true},
		{name: "not ours failed", liquidator: other, errText: "execution reverted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := tracetest.Install(t)
			s, _ := seededSolver(t)

			s.handleMessage(t.Context(), marshal(LiquidationResult{
				Op: "liquidation-result", ID: "auction",
				Data: LiquidationResultData{Success: tc.success, Liquidator: tc.liquidator, Error: tc.errText},
			}))

			span := tracetest.Ended(t, rec, "oev.liquidation.result")
			tracetest.RequireAttr(t, span, "oev.liquidation.success", strconv.FormatBool(tc.success))
			if got := span.Status().Code == codes.Error; got != tc.wantError {
				t.Fatalf("error status = %v, want %v (%v)", got, tc.wantError, span.Status())
			}
			if got := tracetest.HasEvent(span, "exception"); got != tc.wantError {
				t.Fatalf("exception recorded = %v, want %v", got, tc.wantError)
			}
			if tc.errText != "" && tc.wantError && !strings.Contains(span.Status().Description, tc.errText) {
				t.Fatalf("status %q does not name the failure %q", span.Status().Description, tc.errText)
			}
		})
	}
}
