package chain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
)

const testMulticall = "0xcA11bde05977b3631167028862bE2a173976CA11"

// emptyAggregate3Result is the ABI encoding of an empty aggregate3 Result[] return.
const emptyAggregate3Result = `"0x0000000000000000000000000000000000000000000000000000000000000020` +
	`0000000000000000000000000000000000000000000000000000000000000000"`

// wsRPC is a websocket JSON-RPC stub. It records the upgrade request's headers and answers each
// request with result(method).
type wsRPC struct {
	server *httptest.Server

	mu     sync.Mutex
	header http.Header
}

func newWSRPC(result func(method string) string) *wsRPC {
	s := &wsRPC{}
	upgrader := websocket.Upgrader{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.header = r.Header.Clone()
		s.mu.Unlock()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck // test stub teardown
		for {
			_, data, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(data, &req) != nil {
				return
			}
			reply := `{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result(req.Method) + `}`
			if conn.WriteMessage(websocket.TextMessage, []byte(reply)) != nil {
				return
			}
		}
	}))
	return s
}

// url returns the ws:// endpoint of the stub.
func (s *wsRPC) url() string { return "ws" + strings.TrimPrefix(s.server.URL, "http") }

func (s *wsRPC) upgradeHeader() http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.header
}

func (s *wsRPC) close() { s.server.Close() }

// chainRPCResult answers the chain id, a head block number, an empty multicall return, and a null
// receipt — what a node replies while a transaction is still unmined.
func chainRPCResult(method string) string {
	switch method {
	case "eth_blockNumber":
		return `"0x1"`
	case rpcMethodCall:
		return emptyAggregate3Result
	case rpcMethodGetTransactionReceipt:
		return `null`
	default:
		return `"0x7a69"` // 31337
	}
}

// TestDialWebsocket_PropagatesOnHandshakeAndSpansCalls covers the whole websocket path: the
// handshake carries the caller's trace context (the only header a websocket RPC connection has), the
// dial itself is a span, and each call through the client gets its own client span even though no
// instrumented HTTP transport is involved.
func TestDialWebsocket_PropagatesOnHandshakeAndSpansCalls(t *testing.T) {
	rec := tracetest.Install(t)
	srv := newWSRPC(chainRPCResult)
	defer srv.close()

	ctx, parent := otel.Tracer("test").Start(t.Context(), "caller")
	c, err := Dial(ctx, []string{srv.url()}, "", "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	traceparent := srv.upgradeHeader().Get("traceparent")
	if !strings.Contains(traceparent, parent.SpanContext().TraceID().String()) {
		t.Fatalf("handshake traceparent = %q, want the caller trace id %s",
			traceparent, parent.SpanContext().TraceID())
	}

	connect := tracetest.Ended(t, rec, "chain.rpc.connect")
	if connect.SpanKind() != trace.SpanKindClient {
		t.Fatalf("connect span kind = %v, want client", connect.SpanKind())
	}
	if tracetest.Attr(connect, "chain.rpc.transport") != rpcTransportWS ||
		tracetest.Attr(connect, "chain.rpc.role") != rpcRoleShared {
		t.Fatalf("connect attributes = %v", connect.Attributes())
	}
	if connect.Status().Code == codes.Error {
		t.Fatalf("connect status = %v, want unset on a successful dial", connect.Status())
	}

	if _, err = c.BlockNumber(ctx); err != nil {
		t.Fatalf("BlockNumber: %v", err)
	}
	parent.End()

	span := tracetest.Ended(t, rec, "eth_blockNumber")
	if span.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("call span parent = %s, want the caller span %s",
			span.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	if span.SpanKind() != trace.SpanKindClient {
		t.Fatalf("call span kind = %v, want client", span.SpanKind())
	}
	if tracetest.Attr(span, "rpc.system") != "jsonrpc" || tracetest.Attr(span, "rpc.method") != "eth_blockNumber" ||
		tracetest.Attr(span, "chain.rpc.role") != rpcRoleShared || tracetest.Attr(span, "chain.rpc.transport") != rpcTransportWS {
		t.Fatalf("call attributes = %v", span.Attributes())
	}
}

func TestDialWebsocket_ConnectSpanErrorOnDialFailure(t *testing.T) {
	rec := tracetest.Install(t)
	srv := newWSRPC(chainRPCResult)
	endpoint := srv.url()
	srv.close() // nothing is listening any more, so the handshake fails

	if _, err := Dial(t.Context(), []string{endpoint}, "", "", testMulticall, defaultRPCAttemptTimeout); err == nil {
		t.Fatal("expected a dial error against a closed endpoint")
	}

	connect := tracetest.Ended(t, rec, "chain.rpc.connect")
	if connect.Status().Code != codes.Error {
		t.Fatalf("connect status = %v, want Error", connect.Status())
	}
	if tracetest.Attr(connect, "chain.rpc.transport") != rpcTransportWS {
		t.Fatalf("connect attributes = %v", connect.Attributes())
	}
}

// TestTransactionReceiptOverWebsocket_NotFoundIsNotAnError covers the txmanager's steady state: an
// unmined transaction answers null, which ethclient turns into ethereum.NotFound. The HTTP transport
// classifies that response as a success, so the websocket span must not report it as an error either
// or a single pending transaction paints a stream of spans red.
func TestTransactionReceiptOverWebsocket_NotFoundIsNotAnError(t *testing.T) {
	rec := tracetest.Install(t)
	srv := newWSRPC(chainRPCResult)
	defer srv.close()

	c, err := Dial(t.Context(), []string{srv.url()}, "", "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err = c.TransactionReceipt(t.Context(), common.Hash{}); !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("TransactionReceipt error = %v, want ethereum.NotFound", err)
	}

	span := tracetest.Ended(t, rec, rpcMethodGetTransactionReceipt)
	if span.Status().Code == codes.Error {
		t.Fatalf("status = %v, want unset for a not-found receipt", span.Status())
	}
	if !tracetest.HasEvent(span, "not_found") {
		t.Fatalf("events = %v, want a not_found event", span.Events())
	}
}

func TestRPCTransport(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{raw: "ws://node.internal:8546", want: rpcTransportWS},
		{raw: "wss://node.internal:8546", want: rpcTransportWS},
		{raw: "/var/run/geth.ipc", want: rpcTransportIPC},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := rpcTransport(tc.raw); got != tc.want {
				t.Fatalf("rpcTransport(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestMulticallOverWebsocket_SpansOneCall pins the span down to CallContract: Multicall reaches the
// chain through it, so a batched read must not produce a second, nested eth_call span.
func TestMulticallOverWebsocket_SpansOneCall(t *testing.T) {
	rec := tracetest.Install(t)
	srv := newWSRPC(chainRPCResult)
	defer srv.close()

	c, err := Dial(t.Context(), []string{srv.url()}, "", "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err = c.Multicall(t.Context(), nil); err != nil {
		t.Fatalf("Multicall: %v", err)
	}
	if n := len(tracetest.AllEnded(rec, rpcMethodCall)); n != 1 {
		t.Fatalf("eth_call spans = %d, want exactly one", n)
	}
}

// TestDialHTTP_NoMethodLevelSpans guards the HTTP path against a second series: its requests are
// already spanned by fallbackTransport, so the shadowed methods must stay plain passthroughs.
func TestDialHTTP_NoMethodLevelSpans(t *testing.T) {
	rec := tracetest.Install(t)
	srv := rpcRecorder(new([]string), chainRPCResult)
	defer srv.Close()

	c, err := Dial(t.Context(), []string{srv.URL}, "", "", testMulticall, defaultRPCAttemptTimeout)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if _, err = c.Multicall(t.Context(), nil); err != nil {
		t.Fatalf("Multicall: %v", err)
	}
	if _, err = c.BlockNumber(t.Context()); err != nil {
		t.Fatalf("BlockNumber: %v", err)
	}

	if n := len(tracetest.AllEnded(rec, "chain.rpc.connect")); n != 0 {
		t.Fatalf("connect spans = %d, want none on the HTTP path", n)
	}
	for _, method := range []string{rpcMethodCall, "eth_blockNumber", rpcMethodChainID} {
		if n := len(tracetest.AllEnded(rec, method)); n != 1 {
			t.Fatalf("%s spans = %d, want the single transport span", method, n)
		}
	}
	for _, s := range rec.Ended() {
		if tracetest.Attr(s, "chain.rpc.transport") != "" {
			t.Fatalf("span %q carries chain.rpc.transport = %q on the HTTP path",
				s.Name(), tracetest.Attr(s, "chain.rpc.transport"))
		}
	}
}
