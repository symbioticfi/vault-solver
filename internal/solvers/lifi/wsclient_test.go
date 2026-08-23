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

func expectSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for signal")
	}
}
