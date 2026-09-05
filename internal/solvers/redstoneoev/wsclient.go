package redstoneoev

import (
	"context"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

// wsConfig tunes the resilient WS client. Timings default to the RedStone example client's values
// (docs/OEV-PLAN.md §6.1): server pings ~120s, connections forced-closed ~8h (rotate at ~7h).
type wsConfig struct {
	URL    string
	APIKey string
	Topics []string

	HandshakeTimeout time.Duration
	PingInterval     time.Duration
	MsgTimeout       time.Duration // reconnect if no inbound frame/pong within this
	RotateAfter      time.Duration // proactively reconnect before the server's ~8h cutoff
	BackoffInitial   time.Duration
	BackoffMax       time.Duration
}

func (c *wsConfig) withDefaults() {
	setDur(&c.HandshakeTimeout, 10*time.Second)
	setDur(&c.PingInterval, 20*time.Second)
	setDur(&c.MsgTimeout, 30*time.Second)
	setDur(&c.RotateAfter, 7*time.Hour)
	setDur(&c.BackoffInitial, 500*time.Millisecond)
	setDur(&c.BackoffMax, 30*time.Second)
}

// wsClient is a reconnecting WebSocket client: it (re)connects with the x-api-key header, re-sends
// the topic subscriptions after every connect, pings to keep the link alive, rotates before the
// server's cutoff, and delivers inbound frames to onMessage. Outbound solve frames go through Send,
// which is safe for the hot path (non-blocking; drops + returns false if the buffer is full) and
// discards stale buffered solves on every (re)connect.
type wsClient struct {
	cfg    wsConfig
	log    logr.Logger
	onMsg  func(context.Context, []byte)
	dialer *websocket.Dialer
	header http.Header
	send   chan []byte
	// onConnectionState receives true only after every subscription frame has been sent. The
	// callback may be invoked by Run while metrics are concurrently scraped, so it must be safe for
	// concurrent use.
	onConnectionState func(bool)
}

func newWSClient(
	cfg wsConfig,
	log logr.Logger,
	onMsg func(context.Context, []byte),
	onConnectionState func(bool),
) *wsClient {
	cfg.withDefaults()
	h := http.Header{}
	h.Set("x-api-key", cfg.APIKey)
	if onConnectionState == nil {
		onConnectionState = func(bool) {}
	}
	return &wsClient{
		cfg:               cfg,
		log:               log.WithName("ws"),
		onMsg:             onMsg,
		dialer:            &websocket.Dialer{HandshakeTimeout: cfg.HandshakeTimeout},
		header:            h,
		send:              make(chan []byte, 8),
		onConnectionState: onConnectionState,
	}
}

// Send enqueues an outbound frame (e.g. a solve) and reports whether it was accepted. Non-blocking: if
// the buffer is full the frame is dropped (returns false) so the hot path never blocks. A solve only
// lives ~400ms (one auction), so stale frames are dropped on reconnect (flushSendQueue) rather than
// written late to a closed auction — callers must treat false as "not sent" (don't count it as a bid).
func (w *wsClient) Send(frame []byte) bool {
	select {
	case w.send <- frame:
		return true
	default:
		w.log.Info("ws send dropped (buffer full)")
		return false
	}
}

// Run connects and serves until ctx is cancelled, reconnecting with jittered exponential backoff.
func (w *wsClient) Run(ctx context.Context) error {
	backoff := w.cfg.BackoffInitial
	for {
		start := time.Now()
		err := w.serveOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			w.log.Error(err, "ws connection ended; reconnecting")
		}
		// Reset backoff if the last connection was healthy for a while.
		if time.Since(start) > w.cfg.BackoffMax {
			backoff = w.cfg.BackoffInitial
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff + jitter()):
		}
		if backoff *= 2; backoff > w.cfg.BackoffMax {
			backoff = w.cfg.BackoffMax
		}
	}
}

// serveOnce dials, subscribes, and runs the read/write pumps until the connection drops, rotates, or
// ctx is cancelled.
func (w *wsClient) serveOnce(ctx context.Context) error {
	// A failed dial or subscription must leave the client visibly disconnected throughout backoff.
	w.onConnectionState(false)
	conn, resp, err := w.dialer.DialContext(ctx, w.cfg.URL, w.header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close() // handshake response body; not used
	}
	if err != nil {
		return errors.Errorf("dial %s: %w", w.cfg.URL, err)
	}
	w.log.Info("connected", "url", w.cfg.URL)

	// Drop any solves buffered during the downtime: a solve targets one auction (~400ms life), so
	// anything still queued after a reconnect is stale. Start each connection with a clean send queue.
	flushSendQueue(w.send)

	connCtx, cancel := context.WithCancel(ctx)

	// (Re)subscribe to all topics.
	for _, topic := range w.cfg.Topics {
		if werr := conn.WriteMessage(websocket.TextMessage, marshal(SubscribeMessage{Op: "subscribe", Topic: topic})); werr != nil {
			cancel()
			_ = conn.Close()
			return errors.Errorf("subscribe %s: %w", topic, werr)
		}
	}
	w.onConnectionState(true)
	w.log.Info("subscribed", "topics", w.cfg.Topics)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); w.writePump(connCtx, conn, errCh) }()
	go func() { defer wg.Done(); w.readPump(connCtx, conn, errCh) }()

	var retErr error
	select {
	case <-ctx.Done():
		retErr = ctx.Err()
	case e := <-errCh:
		retErr = e
	}
	// Publish teardown before closing/joining the pumps so a blocked pump cannot leave a stale 1.
	w.onConnectionState(false)
	// Tear the connection down and JOIN both pumps before returning, so no pump goroutine — and no
	// second reader competing for w.send — outlives this connection into the next reconnect.
	cancel()
	_ = conn.Close()
	wg.Wait()
	return retErr
}

// readPump reads frames, extends the read deadline on each, dispatches to onMsg, and answers server
// pings (gorilla auto-replies to pings via the default handler; we extend the deadline too).
func (w *wsClient) readPump(ctx context.Context, conn *websocket.Conn, errCh chan<- error) {
	_ = conn.SetReadDeadline(time.Now().Add(w.cfg.MsgTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(w.cfg.MsgTimeout))
	})
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(w.cfg.MsgTimeout))
		// Reply pong via the write pump is simplest, but gorilla allows a direct control write.
		_ = conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
		return nil
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case errCh <- errors.Errorf("read: %w", err):
			default:
			}
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(w.cfg.MsgTimeout))
		if ctx.Err() != nil {
			return
		}
		w.onMsg(ctx, data)
	}
}

// writePump owns all writes (gorilla requires a single writer): outbound frames, periodic pings, and
// a rotation timer that forces a clean reconnect before the server's cutoff.
func (w *wsClient) writePump(ctx context.Context, conn *websocket.Conn, errCh chan<- error) {
	ping := time.NewTicker(w.cfg.PingInterval)
	defer ping.Stop()
	rotate := time.NewTimer(w.cfg.RotateAfter + jitter())
	defer rotate.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-w.send:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				w.nonblockErr(errCh, errors.Errorf("write: %w", err))
				return
			}
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				w.nonblockErr(errCh, errors.Errorf("ping: %w", err))
				return
			}
		case <-rotate.C:
			w.log.Info("rotating connection before server cutoff")
			w.nonblockErr(errCh, errors.New("rotate"))
			return
		}
	}
}

func (w *wsClient) nonblockErr(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}

// flushSendQueue empties the outbound buffer without blocking (used on (re)connect to discard stale solves).
func flushSendQueue(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func setDur(d *time.Duration, def time.Duration) {
	if *d <= 0 {
		*d = def
	}
}

// jitter returns 1–5s of randomness to desynchronize reconnects (matches the example client). The
// reconnect path is not security-sensitive, so math/rand is fine.
func jitter() time.Duration {
	return time.Duration(1000+rand.Intn(4000)) * time.Millisecond //nolint:gosec // non-crypto jitter
}
