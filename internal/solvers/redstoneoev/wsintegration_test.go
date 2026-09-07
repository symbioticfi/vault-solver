package redstoneoev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
)

// wsintegration_test.go drives the REAL wsClient (connect → subscribe → read → reconnect-safe) end to
// end against an in-process httptest websocket server, with no chain (the solver reads only its seeded
// snapshot/state). The end-to-end solve + breaker path through handleMessage is covered by
// TestFullAuctionLifecycle (solver_test.go); here we pin the reconnect hygiene that the in-memory path
// can't exercise.

func TestWSConnectionMetricTracksFullySubscribedLifetime(t *testing.T) {
	topics := []string{"topic-a", "topic-b"}
	allSubscriptionsRead := make(chan struct{})
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck // test teardown
		for range topics {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		close(allSubscriptionsRead)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	m, err := newMetrics(prometheus.NewRegistry(), defaultStrategyName, nil)
	testcheck.NoError(t, err)
	client := newWSClient(wsConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), APIKey: "k", Topics: topics,
		PingInterval: time.Hour, MsgTimeout: time.Hour, RotateAfter: time.Hour,
	}, logr.Discard(), func(context.Context, []byte) {}, m.setFeedConnected)
	metricstest.RequireValue(t, m.feedConnected, 0)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()
	testcheck.ReceiveWithin(t, allSubscriptionsRead, time.Second, "client did not send all subscriptions")
	waitForMetricValue(t, m.feedConnected, 1)

	cancel()
	testcheck.ReceiveWithin(t, done, time.Second, "client did not stop after cancellation")
	metricstest.RequireValue(t, m.feedConnected, 0)
}

func waitForMetricValue(t *testing.T, gauge prometheus.Gauge, want float64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if testutil.ToFloat64(gauge) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("metric value = %v, want %v", testutil.ToFloat64(gauge), want)
}

// TestWSIntegrationDropsStaleSolveAcrossReconnect proves the reconnect hygiene fix (#6): a solve
// buffered while the connection is down is NOT replayed to the next connection (a stale auction has
// closed). One server drops the first connection, then accepts the reconnect and captures any SOLVE the
// client writes (subscribe frames are expected on reconnect and ignored). The URL never changes, so the
// Run goroutine's cfg reads stay race-free.
func TestWSIntegrationDropsStaleSolveAcrossReconnect(t *testing.T) {
	s, _ := seededSolver(t)

	var conns atomic.Int32
	dropped := make(chan struct{}, 1)
	gotSolve := make(chan string, 4)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		if conns.Add(1) == 1 { // first connection: drop immediately so the client must reconnect
			_ = c.Close()
			dropped <- struct{}{}
			return
		}
		defer c.Close() //nolint:errcheck // test teardown
		for {           // reconnect: capture only solve frames (subscribes are expected, ignored)
			_, data, rerr := c.ReadMessage()
			if rerr != nil {
				return
			}
			if frame, _ := decodeFrame(data); frame.Op == "solve" {
				gotSolve <- string(data)
			}
		}
	}))
	defer srv.Close()

	s.ws = newWSClient(wsConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), APIKey: "k",
		Topics: []string{"t"}, BackoffInitial: 10 * time.Millisecond,
	}, logr.Discard(), s.handleMessage, nil)
	// Pre-load a solve into the send buffer as if a prior auction had queued it during the downtime.
	s.ws.Send(t.Context(), []byte(`{"op":"solve","id":"stale","data":{}}`), time.Time{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.ws.Run(ctx) }()
	<-dropped // first connection happened and dropped (the buffered solve survived the drop)

	select {
	case frame := <-gotSolve:
		t.Fatalf("stale solve replayed across reconnect: %s", frame)
	case <-time.After(500 * time.Millisecond):
		// No solve written on the reconnect — flushSendQueue discarded the stale frame. ✓
	}
}

func TestWSWriterSkipsSolveCanceledWhileQueued(t *testing.T) {
	received := make(chan string, 1)
	upgrade := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		_, data, err := conn.ReadMessage()
		if err == nil {
			received <- string(data)
		}
	}))
	defer server.Close()
	var client *wsClient
	client = newWSClient(wsConfig{URL: "ws" + strings.TrimPrefix(server.URL, "http")}, logr.Discard(), func(context.Context, []byte) {}, func(connected bool) {
		if !connected {
			return
		}
		stale, cancel := context.WithCancel(t.Context())
		if !client.Send(stale, []byte("stale"), time.Time{}) {
			t.Error("stale frame was not initially accepted")
		}
		cancel()
		if !client.Send(t.Context(), []byte("fresh"), time.Now().Add(time.Minute)) {
			t.Error("fresh frame was not accepted")
		}
	})
	_ = client.serveOnce(t.Context())
	select {
	case got := <-received:
		if got != "fresh" {
			t.Fatalf("first written frame = %q, want fresh", got)
		}
	default:
		t.Fatal("writer did not deliver fresh frame")
	}
	if client.Send(t.Context(), []byte("expired"), time.Now().Add(-time.Second)) {
		t.Fatal("expired solve was accepted")
	}
}

func TestWSReadLimitRejectsOversizedFrame(t *testing.T) {
	upgrade := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 65)))
	}))
	defer server.Close()
	var dispatched atomic.Int32
	client := newWSClient(wsConfig{URL: "ws" + strings.TrimPrefix(server.URL, "http"), MaxMessageBytes: 64}, logr.Discard(), func(context.Context, []byte) { dispatched.Add(1) }, nil)
	if err := client.serveOnce(t.Context()); err == nil {
		t.Fatal("oversized frame did not terminate connection")
	}
	if dispatched.Load() != 0 {
		t.Fatal("oversized frame reached solver")
	}
}
