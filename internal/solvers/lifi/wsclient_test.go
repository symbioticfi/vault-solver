package lifi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	connected, err := feed.watchOnce(context.Background(), func(context.Context, orderMessage) {})
	if !connected {
		t.Fatal("connection was not reported as established")
	}
	if err == nil {
		t.Fatal("expected read error after server closed the connection")
	}
}
