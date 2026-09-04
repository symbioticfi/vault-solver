package lifi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
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
	url    string
	apiKey string
	log    logr.Logger
}

type orderFeedConnectionHooks struct {
	beforeRead     func(context.Context) // Synchronous: establishes state before the first event.
	whileConnected func(context.Context) // Concurrent with reads and joined on disconnect.
}

func newOrderFeed(url, apiKey string, log logr.Logger) *orderFeed {
	return &orderFeed{url: url, apiKey: apiKey, log: log}
}

func (f *orderFeed) run(
	ctx context.Context,
	hooks orderFeedConnectionHooks,
	handle func(context.Context, orderMessage),
) error {
	backoff := initialWSBackoff
	for {
		connected, err := f.watchOnce(ctx, hooks, handle)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if connected {
			backoff = initialWSBackoff
		}
		f.log.Error(err, "order feed disconnected; reconnecting", "backoff", backoff.String())
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff = nextWSBackoff(backoff)
	}
}

func nextWSBackoff(current time.Duration) time.Duration {
	return min(2*current, maxWSBackoff)
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
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, f.url, headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if resp != nil {
			return false, errors.Errorf("dial websocket: %w (status %s)", err, resp.Status)
		}
		return false, errors.Errorf("dial websocket: %w", err)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer conn.Close()

	connectionCtx, cancelConnection := context.WithCancel(ctx)
	var work sync.WaitGroup
	defer func() {
		cancelConnection()
		work.Wait()
	}()
	if hooks.beforeRead != nil {
		hooks.beforeRead(connectionCtx)
	}
	if hooks.whileConnected != nil {
		work.Go(func() { hooks.whileConnected(connectionCtx) })
	}

	f.log.Info("order feed connected", "url", f.url)
	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			return true, errors.Errorf("read websocket: %w", err)
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if pong, ok := pongFor(msg); ok {
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
		handle(connectionCtx, envelope)
	}
}

func pongFor(msg []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(msg)
	if strings.EqualFold(string(trimmed), "ping") {
		return []byte("pong"), true
	}
	event, ok := websocketEvent(trimmed)
	if !ok {
		return nil, false
	}
	if strings.EqualFold(event, "ping") {
		return []byte(`{"event":"pong"}`), true
	}
	return nil, false
}

func websocketEvent(message []byte) (string, bool) {
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(message, &envelope); err != nil {
		return "", false
	}
	return envelope.Event, true
}
