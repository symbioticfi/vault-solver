package redstoneoev

import (
	"context"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

type wsConfig struct {
	URL, APIKey                                                                         string
	Topics                                                                              []string
	HandshakeTimeout, PingInterval, MsgTimeout, RotateAfter, BackoffInitial, BackoffMax time.Duration
	MaxMessageBytes                                                                     int64
}

func (c *wsConfig) withDefaults() {
	setDur(&c.HandshakeTimeout, 10*time.Second)
	setDur(&c.PingInterval, 20*time.Second)
	setDur(&c.MsgTimeout, 30*time.Second)
	setDur(&c.RotateAfter, 7*time.Hour)
	setDur(&c.BackoffInitial, 500*time.Millisecond)
	setDur(&c.BackoffMax, 30*time.Second)
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultMaxMessageBytes
	}
}

type outboundFrame struct {
	data      []byte
	cancelled <-chan struct{}
	deadline  time.Time
}

func (f outboundFrame) expired(now time.Time) bool {
	select {
	case <-f.cancelled:
		return true
	default:
	}
	return !f.deadline.IsZero() && !now.Before(f.deadline)
}

// Each connection owns one reader and one writer. Outbound solves retain their
// originating connection cancellation and auction deadline across queueing.
type wsClient struct {
	cfg               wsConfig
	log               logr.Logger
	onMsg             func(context.Context, []byte)
	dialer            *websocket.Dialer
	header            http.Header
	send              chan outboundFrame
	onConnectionState func(bool)
}

func newWSClient(cfg wsConfig, log logr.Logger, onMsg func(context.Context, []byte), connected func(bool)) *wsClient {
	cfg.withDefaults()
	if connected == nil {
		connected = func(bool) {}
	}
	return &wsClient{
		cfg: cfg, log: log.WithName("ws"), onMsg: onMsg,
		dialer: &websocket.Dialer{HandshakeTimeout: cfg.HandshakeTimeout},
		header: http.Header{"X-Api-Key": []string{cfg.APIKey}},
		send:   make(chan outboundFrame, 8), onConnectionState: connected,
	}
}

func (w *wsClient) Send(ctx context.Context, data []byte, deadline time.Time) bool {
	frame := outboundFrame{data: append([]byte(nil), data...), cancelled: ctx.Done(), deadline: deadline}
	if frame.expired(time.Now()) {
		return false
	}
	select {
	case w.send <- frame:
		return true
	default:
		return false
	}
}

func (w *wsClient) Run(ctx context.Context) error {
	delay := w.cfg.BackoffInitial
	for ctx.Err() == nil {
		started := time.Now()
		err := w.serveOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			w.log.Error(err, "ws connection ended; reconnecting")
		}
		if time.Since(started) > w.cfg.BackoffMax {
			delay = w.cfg.BackoffInitial
		}
		timer := time.NewTimer(delay + jitter())
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay = min(2*delay, w.cfg.BackoffMax)
	}
	return ctx.Err()
}

func (w *wsClient) serveOnce(ctx context.Context) error {
	w.onConnectionState(false)
	conn, response, err := w.dialer.DialContext(ctx, w.cfg.URL, w.header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return errors.Errorf("dial %s: %w", w.cfg.URL, err)
	}
	defer conn.Close()
	connection, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClose := context.AfterFunc(connection, func() { _ = conn.Close() })
	defer stopClose()
	conn.SetReadLimit(w.cfg.MaxMessageBytes)
	flushSendQueue(w.send)
	for _, topic := range w.cfg.Topics {
		if err := conn.SetWriteDeadline(time.Now().Add(w.cfg.HandshakeTimeout)); err != nil {
			return err
		}
		if err := conn.WriteMessage(websocket.TextMessage, marshal(SubscribeMessage{Op: "subscribe", Topic: topic})); err != nil {
			return errors.Errorf("subscribe %s: %w", topic, err)
		}
	}
	w.onConnectionState(true)
	w.log.Info("subscribed", "topics", w.cfg.Topics)
	writerDone := make(chan error, 1)
	go func() {
		err := w.writePump(connection, conn)
		_ = conn.Close() // wake the reader if the writer failed or rotated
		writerDone <- err
	}()
	readErr := w.readPump(connection, conn)
	w.onConnectionState(false)
	cancel()
	_ = conn.Close()
	writeErr := <-writerDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if writeErr != nil && !errors.Is(writeErr, context.Canceled) {
		return writeErr
	}
	return readErr
}

func (w *wsClient) readPump(ctx context.Context, conn *websocket.Conn) error {
	extend := func() error { return conn.SetReadDeadline(time.Now().Add(w.cfg.MsgTimeout)) }
	conn.SetPongHandler(func(string) error { return extend() })
	conn.SetPingHandler(func(data string) error {
		if err := extend(); err != nil {
			return err
		}
		// Gorilla permits control frames alongside the single data writer.
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
	})
	for ctx.Err() == nil {
		if err := extend(); err != nil {
			return err
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return errors.Errorf("read: %w", err)
		}
		if ctx.Err() == nil {
			w.onMsg(ctx, data)
		}
	}
	return ctx.Err()
}

func (w *wsClient) writePump(ctx context.Context, conn *websocket.Conn) error {
	ping := time.NewTicker(w.cfg.PingInterval)
	defer ping.Stop()
	rotate := time.NewTimer(w.cfg.RotateAfter + jitter())
	defer rotate.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rotate.C:
			return errors.New("rotate websocket before server cutoff")
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return errors.Errorf("ping: %w", err)
			}
		case frame := <-w.send:
			if frame.expired(time.Now()) {
				continue
			}
			deadline := time.Now().Add(5 * time.Second)
			if !frame.deadline.IsZero() && frame.deadline.Before(deadline) {
				deadline = frame.deadline
			}
			if err := conn.SetWriteDeadline(deadline); err != nil {
				return err
			}
			if err := conn.WriteMessage(websocket.TextMessage, frame.data); err != nil {
				return errors.Errorf("write: %w", err)
			}
		}
	}
}

func flushSendQueue(ch chan outboundFrame) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func setDur(value *time.Duration, fallback time.Duration) {
	if *value <= 0 {
		*value = fallback
	}
}

// Protocol reconnect jitter is not used for signing or authentication.
func jitter() time.Duration {
	return time.Duration(1000+rand.IntN(4000)) * time.Millisecond //nolint:gosec // reconnect jitter has no security role
}
