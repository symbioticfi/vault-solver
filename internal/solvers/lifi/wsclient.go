package lifi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

const (
	orderSubmitEvent = "user:vm-order-submit"
	initialWSBackoff = time.Second
	maxWSBackoff     = 30 * time.Second
)

type orderMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type orderFeed struct {
	cfg           OrderServerConfig
	apiKey        string
	log           logr.Logger
	connected     atomic.Bool // watchOnce writes; Prometheus scrapes read concurrently.
	recoveryReady atomic.Bool // connection recovery writes; Prometheus scrapes read concurrently.
}

type orderFeedConnectionHooks struct {
	beforeRead     func(context.Context) // Synchronous: establishes state before the first event.
	whileConnected func(context.Context) // Concurrent with reads and joined on disconnect.
}

func newOrderFeed(cfg OrderServerConfig, apiKey string, log logr.Logger) *orderFeed {
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = defaultMaxMessageBytes
	}
	return &orderFeed{cfg: cfg, apiKey: apiKey, log: log}
}

func (f *orderFeed) run(ctx context.Context, hooks orderFeedConnectionHooks, handle func(context.Context, orderMessage)) error {
	for delay := initialWSBackoff; ctx.Err() == nil; delay = min(delay*2, maxWSBackoff) {
		connected, err := f.watchOnce(ctx, hooks, handle)
		if ctx.Err() != nil {
			break
		}
		if connected {
			delay = initialWSBackoff
		}
		f.log.Error(err, "order feed disconnected; reconnecting", "backoff", delay.String())
		if !waitForRetry(ctx, delay) {
			break
		}
	}
	return ctx.Err()
}

func (f *orderFeed) watchOnce(
	ctx context.Context,
	hooks orderFeedConnectionHooks,
	handle func(context.Context, orderMessage),
) (bool, error) {
	headers := http.Header{}
	if f.apiKey != "" {
		headers.Set("x-api-key", f.apiKey)
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, f.cfg.WSURL, headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return false, errors.Errorf("dial websocket: %w (status %s)", err, resp.Status)
		}
		return false, errors.Errorf("dial websocket: %w", err)
	}
	connectionCtx, cancel := context.WithCancel(ctx)
	stopClose := context.AfterFunc(connectionCtx, func() { _ = conn.Close() })
	defer stopClose()
	conn.SetReadLimit(f.cfg.MaxMessageBytes)
	f.recoveryReady.Store(false)
	f.connected.Store(true)
	var workers sync.WaitGroup
	defer func() {
		f.connected.Store(false)
		cancel()
		_ = conn.Close()
		workers.Wait()
		// A late recovery completion must not re-enable the next connection's readiness.
		f.recoveryReady.Store(false)
	}()
	if hooks.beforeRead != nil {
		hooks.beforeRead(connectionCtx)
	}
	if hooks.whileConnected != nil {
		workers.Go(func() { hooks.whileConnected(connectionCtx) })
	}

	f.log.Info("order feed connected", "url", f.cfg.WSURL)
	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			return true, errors.Errorf("read websocket: %w", err)
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if pong, ok := pongFor(msg); ok {
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return true, errors.Errorf("set pong deadline: %w", err)
			}
			if err := conn.WriteMessage(websocket.TextMessage, pong); err != nil {
				return true, errors.Errorf("write websocket pong: %w", err)
			}
			continue
		}

		var envelope orderMessage
		if err := json.Unmarshal(msg, &envelope); err != nil {
			f.log.V(1).Info("order feed: non-json message ignored")
			continue
		}
		if envelope.Event != orderSubmitEvent {
			f.log.V(1).Info("order feed event ignored", "event", envelope.Event)
			continue
		}
		if connectionCtx.Err() == nil {
			handle(connectionCtx, envelope)
		}
	}
}

// markRecoveryReady publishes readiness only for a still-current established connection. watchOnce
// serializes connection generations and clears the bit both before a reconnect and after joining the
// closing connection's work, so an old recovery goroutine cannot make a newer connection ready.
func (f *orderFeed) markRecoveryReady(connectionCtx context.Context) {
	if connectionCtx.Err() != nil || !f.connected.Load() {
		return
	}
	f.recoveryReady.Store(true)
}

func pongFor(msg []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(msg)
	if strings.EqualFold(string(trimmed), "ping") {
		return []byte("pong"), true
	}
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, false
	}
	if strings.EqualFold(envelope.Event, "ping") {
		return []byte(`{"event":"pong"}`), true
	}
	return nil, false
}
