package lifi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPongFor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "plain", in: "ping", want: "pong", ok: true},
		{name: "json", in: `{"event":"ping"}`, want: `{"event":"pong"}`, ok: true},
		{name: "other", in: `{"event":"user:vm-order-submit"}`, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pongFor([]byte(tt.in))
			if ok != tt.ok {
				t.Fatalf("ok = %v", ok)
			}
			if string(got) != tt.want {
				t.Fatalf("pong = %q", got)
			}
		})
	}
}

func TestWatchOnceReportsEstablishedConnection(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", logr.Discard())
	connected, err := feed.watchOnce(context.Background(), orderFeedConnectionHooks{}, func(context.Context, orderMessage) {})
	if !connected {
		t.Fatal("connection was not reported as established")
	}
	if err == nil {
		t.Fatal("expected read error after server closed the connection")
	}
}

func TestWatchOnceRunsConnectionWorkAlongsideEventsAndWaitsForIt(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(
			websocket.TextMessage,
			[]byte(`{"event":"`+orderSubmitEvent+`","data":{}}`),
		); err != nil {
			t.Errorf("write event: %v", err)
		}
	}))
	defer server.Close()

	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", logr.Discard())
	workStarted := make(chan struct{})
	workCanceled := make(chan struct{})
	liveHandled := make(chan struct{})
	releaseWork := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = feed.watchOnce(
			t.Context(),
			orderFeedConnectionHooks{
				whileConnected: func(connectionCtx context.Context) {
					close(workStarted)
					<-connectionCtx.Done()
					close(workCanceled)
					<-releaseWork
				},
			},
			func(context.Context, orderMessage) { close(liveHandled) },
		)
		close(done)
	}()

	expectSignal(t, workStarted)
	expectSignal(t, liveHandled)
	expectSignal(t, workCanceled)
	select {
	case <-done:
		t.Fatal("watchOnce returned before connection work stopped")
	default:
	}
	close(releaseWork)
	expectSignal(t, done)
}

func TestWatchOnceRunsConnectionStartHookBeforeFirstEvent(t *testing.T) {
	upgrader := websocket.Upgrader{}
	frameWritten := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(
			websocket.TextMessage,
			[]byte(`{"event":"`+orderSubmitEvent+`","data":{}}`),
		); err != nil {
			t.Errorf("write event: %v", err)
			return
		}
		close(frameWritten)
	}))
	defer server.Close()

	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", logr.Discard())
	hookStarted := make(chan struct{})
	releaseHook := make(chan struct{})
	liveHandled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = feed.watchOnce(
			t.Context(),
			orderFeedConnectionHooks{
				beforeRead: func(connectionCtx context.Context) {
					close(hookStarted)
					select {
					case <-releaseHook:
					case <-connectionCtx.Done():
					}
				},
			},
			func(context.Context, orderMessage) { close(liveHandled) },
		)
		close(done)
	}()

	expectSignal(t, hookStarted)
	expectSignal(t, frameWritten)
	select {
	case <-liveHandled:
		t.Fatal("event was handled before connected hook completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseHook)
	expectSignal(t, liveHandled)
	expectSignal(t, done)
}

func TestOrderFeedRunsConnectionWorkAfterReconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", logr.Discard())
	started := make(chan struct{}, 2)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- feed.run(ctx, orderFeedConnectionHooks{
			whileConnected: func(context.Context) { started <- struct{}{} },
		}, func(context.Context, orderMessage) {})
	}()

	expectSignal(t, started)
	expectSignal(t, started)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("feed.run() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("feed did not stop")
	}
}

func TestOrderFeedMetricsLifecycle(t *testing.T) {
	upgrader := websocket.Upgrader{}
	upgraded := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		close(upgraded)
		<-release
		_ = conn.Close()
	}))
	defer server.Close()

	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", logr.Discard())
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), feed, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.orderFeedConnected); got != 0 {
		t.Fatalf("initial connected = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.orderRecoveryReady); got != 0 {
		t.Fatalf("initial recovery ready = %v, want 0", got)
	}

	result := make(chan error, 1)
	recoveryStarted := make(chan struct{})
	allowRecovery := make(chan struct{})
	go func() {
		_, watchErr := feed.watchOnce(
			t.Context(),
			orderFeedConnectionHooks{whileConnected: func(connectionCtx context.Context) {
				close(recoveryStarted)
				select {
				case <-allowRecovery:
					feed.markRecoveryReady(connectionCtx)
				case <-connectionCtx.Done():
					return
				}
				<-connectionCtx.Done()
			}},
			func(context.Context, orderMessage) {},
		)
		result <- watchErr
	}()
	<-upgraded
	expectSignal(t, recoveryStarted)
	assertGaugeEventually(t, metrics.orderFeedConnected, 1)
	if got := testutil.ToFloat64(metrics.orderRecoveryReady); got != 0 {
		t.Fatalf("recovery ready before convergence = %v, want 0", got)
	}

	close(allowRecovery)
	assertGaugeEventually(t, metrics.orderRecoveryReady, 1)

	close(release)
	if err := <-result; err == nil {
		t.Fatal("expected read error after server closed the connection")
	}
	if got := testutil.ToFloat64(metrics.orderFeedConnected); got != 0 {
		t.Fatalf("disconnected = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.orderRecoveryReady); got != 0 {
		t.Fatalf("recovery ready after disconnect = %v, want 0", got)
	}
}

func TestOrderRecoveryReadyMetricNilFeed(t *testing.T) {
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.orderRecoveryReady); got != 0 {
		t.Fatalf("recovery ready with nil feed = %v, want 0", got)
	}
}

func assertGaugeEventually(t *testing.T, collector prometheus.Collector, want float64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if got := testutil.ToFloat64(collector); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("gauge did not reach %v", want)
		}
		time.Sleep(time.Millisecond)
	}
}

func expectSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}
