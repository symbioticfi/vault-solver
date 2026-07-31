package lifi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	connected, err := feed.watchOnce(context.Background(), func(context.Context, orderMessage) {})
	if !connected {
		t.Fatal("connection was not reported as established")
	}
	if err == nil {
		t.Fatal("expected read error after server closed the connection")
	}
}

func TestOrderFeedConnectedMetricLifecycle(t *testing.T) {
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
	metrics, err := newLIFIMetrics(prometheus.NewRegistry(), feed)
	if err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.orderFeedConnected); got != 0 {
		t.Fatalf("initial connected = %v, want 0", got)
	}

	result := make(chan error, 1)
	go func() {
		_, watchErr := feed.watchOnce(t.Context(), func(context.Context, orderMessage) {})
		result <- watchErr
	}()
	<-upgraded
	assertGaugeEventually(t, metrics.orderFeedConnected, 1)

	close(release)
	if err := <-result; err == nil {
		t.Fatal("expected read error after server closed the connection")
	}
	if got := testutil.ToFloat64(metrics.orderFeedConnected); got != 0 {
		t.Fatalf("disconnected = %v, want 0", got)
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
