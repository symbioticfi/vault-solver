package redstoneoev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

// wsintegration_test.go drives the REAL wsClient (connect → subscribe → read → reconnect-safe) end to
// end against an in-process httptest websocket server, with no chain (the solver reads only its seeded
// snapshot/state). The end-to-end solve + breaker path through handleMessage is covered by
// TestFullAuctionLifecycle (solver_test.go); here we pin the reconnect hygiene that the in-memory path
// can't exercise.

// TestWSIntegrationDropsStaleSolveAcrossReconnect proves the reconnect hygiene fix (#6): a solve
// buffered while the connection is down is NOT replayed to the next connection (a stale auction has
// closed). One server drops the first connection, then accepts the reconnect and captures any SOLVE the
// client writes (subscribe frames are expected on reconnect and ignored). The URL never changes, so the
// Run goroutine's cfg reads stay race-free.
func TestWSIntegrationDropsStaleSolveAcrossReconnect(t *testing.T) {
	s, _ := seededSolver(t)
	useOnchainTestMonitor(t, s)

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
			if op, _ := opName(data); op == "solve" {
				gotSolve <- string(data)
			}
		}
	}))
	defer srv.Close()

	s.ws = newWSClient(wsConfig{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), APIKey: "k",
		Topics: []string{"t"}, BackoffInitial: 10 * time.Millisecond,
	}, logr.Discard(), s.handleMessage)
	// Pre-load a solve into the send buffer as if a prior auction had queued it during the downtime.
	s.ws.Send([]byte(`{"op":"solve","id":"stale","data":{}}`))

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
