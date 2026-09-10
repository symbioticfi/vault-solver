package lifi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestOrderFeedDisconnectLogLevel(t *testing.T) {
	for _, tc := range []struct {
		name      string
		closeCode int
		status    int
		ready     bool
		wantInfo  bool
	}{
		{name: "normal closure", closeCode: websocket.CloseNormalClosure, ready: true, wantInfo: true},
		{name: "going away", closeCode: websocket.CloseGoingAway, ready: true, wantInfo: true},
		{name: "unexpected EOF", ready: true, wantInfo: true},
		{name: "no status", closeCode: websocket.CloseNoStatusReceived, ready: true, wantInfo: true},
		{name: "service restart", closeCode: websocket.CloseServiceRestart, ready: true, wantInfo: true},
		{name: "try again later", closeCode: websocket.CloseTryAgainLater, ready: true, wantInfo: true},
		{name: "early EOF"},
		{name: "early normal closure", closeCode: websocket.CloseNormalClosure},
		{name: "early no status", closeCode: websocket.CloseNoStatusReceived},
		{name: "early restart", closeCode: websocket.CloseServiceRestart},
		{name: "protocol error", closeCode: websocket.CloseProtocolError, ready: true},
		{name: "unauthorized", status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upgrader := websocket.Upgrader{}
			readStarted := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				<-readStarted
				if tc.closeCode != 0 {
					if err := conn.WriteControl(websocket.CloseMessage,
						websocket.FormatCloseMessage(tc.closeCode, ""), time.Now().Add(time.Second)); err != nil {
						t.Error(err)
					}
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var event map[string]any
			log := funcr.NewJSON(func(entry string) {
				var fields map[string]any
				if err := json.Unmarshal([]byte(entry), &fields); err != nil {
					t.Error(err)
				}
				if fields["msg"] == "order feed disconnected; reconnecting" {
					event = fields
					cancel()
				}
			}, funcr.Options{})
			feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", log)
			_ = feed.run(ctx, orderFeedConnectionHooks{beforeRead: func(connectionCtx context.Context) {
				if tc.ready {
					feed.markRecoveryReady(connectionCtx)
				}
				close(readStarted)
			}}, func(context.Context, orderMessage) {})
			_, info := event["level"]
			if event == nil || info != tc.wantInfo || event["error"] == nil || event["backoff"] != "1s" {
				t.Fatalf("disconnect log = %v, want Info=%v with error and backoff", event, tc.wantInfo)
			}
		})
	}
}

func TestOrderFeedBackoffResetsOnlyAfterRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	readStarted := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		select {
		case <-readStarted:
		case <-ctx.Done():
		}
	}))
	defer server.Close()
	var backoffs []string
	log := funcr.NewJSON(func(entry string) {
		var fields map[string]any
		if err := json.Unmarshal([]byte(entry), &fields); err != nil {
			t.Error(err)
		}
		if fields["msg"] != "order feed disconnected; reconnecting" {
			return
		}
		backoff, _ := fields["backoff"].(string)
		backoffs = append(backoffs, backoff)
		_, info := fields["level"]
		if info != (len(backoffs) == 3) {
			t.Errorf("disconnect %d Info=%v", len(backoffs), info)
		}
		if len(backoffs) == 3 {
			cancel()
		}
	}, funcr.Options{})
	feed := newOrderFeed("ws"+strings.TrimPrefix(server.URL, "http"), "", log)
	attempts := 0
	_ = feed.run(ctx, orderFeedConnectionHooks{beforeRead: func(connectionCtx context.Context) {
		attempts++
		if attempts == 3 {
			feed.markRecoveryReady(connectionCtx)
		}
		select {
		case readStarted <- struct{}{}:
		case <-connectionCtx.Done():
		}
	}}, func(context.Context, orderMessage) {})
	if got := strings.Join(backoffs, ","); got != "1s,2s,1s" {
		t.Fatalf("disconnect backoffs = %s", got)
	}
}

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

func TestWatchOnceReportsUnrecoveredConnection(t *testing.T) {
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
	ready, err := feed.watchOnce(context.Background(), orderFeedConnectionHooks{}, func(context.Context, orderMessage) {})
	if ready {
		t.Fatal("connection closed before recovery completed")
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
