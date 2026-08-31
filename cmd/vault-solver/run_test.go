package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-errors/errors"
)

func TestRunBotJoinsObservabilityServerOnStartupFailure(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	observabilityAddr := reserveLoopbackAddr(t)
	observed := make(chan error, 1)

	rpcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			observed <- errors.Errorf("decode JSON-RPC request: %w", err)
			return
		}
		if request.Method != "eth_chainId" {
			observed <- errors.Errorf("JSON-RPC method = %q, want eth_chainId", request.Method)
			return
		}
		observed <- observeStartupProbes(r.Context(), observabilityAddr)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  string          `json:"result"`
		}{JSONRPC: "2.0", ID: request.ID, Result: "0x1"}); err != nil {
			t.Errorf("encode JSON-RPC response: %v", err)
		}
	}))
	defer rpcServer.Close()

	configPath := writeConfigFile(t, `
chain:
  rpcUrl: "`+rpcServer.URL+`"
  chainId: 2
signer:
  keyEnv: TEST_PRIVATE_KEY
observability:
  addr: "`+observabilityAddr+`"
solvers:
  - name: lifecycle-test
    config: {}
`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := runBot(ctx, configPath, false, false)
	assertLoopbackReleased(t, observabilityAddr)
	if probeErr := <-observed; probeErr != nil {
		cancel()
		t.Fatalf("startup probe contract: %v", probeErr)
	}
	if err == nil || !strings.Contains(err.Error(), "chain id mismatch: rpc reports 1, config says 2") {
		cancel()
		t.Fatalf("runBot() error = %v, want chain ID mismatch", err)
	}
}

func observeStartupProbes(ctx context.Context, addr string) error {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for _, probe := range []struct {
		path   string
		status int
		body   string
	}{
		{path: "/healthz", status: http.StatusOK, body: "ok"},
		{path: "/readyz", status: http.StatusServiceUnavailable, body: "not ready"},
	} {
		deadline := time.Now().Add(2 * time.Second)
		var err error
		for time.Now().Before(deadline) {
			err = observeStartupProbe(ctx, client, addr, probe.path, probe.status, probe.body)
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func observeStartupProbe(
	ctx context.Context,
	client *http.Client,
	addr, path string,
	wantStatus int,
	wantBody string,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return errors.Errorf("build %s request: %w", path, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.Errorf("request %s: %w", path, err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		return errors.Errorf("read %s: %w", path, readErr)
	}
	if closeErr != nil {
		return errors.Errorf("close %s response: %w", path, closeErr)
	}
	if response.StatusCode != wantStatus || string(body) != wantBody {
		return errors.Errorf(
			"%s response = (%d, %q), want (%d, %q)",
			path, response.StatusCode, body, wantStatus, wantBody,
		)
	}
	return nil
}

func reserveLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}
	return addr
}

func assertLoopbackReleased(t *testing.T, addr string) {
	t.Helper()
	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("observability listener still bound after runBot returned: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close rebound observability listener: %v", err)
	}
}

func TestWatchReadinessTracksLaneState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	changes := make(chan struct{}, 1)
	states := make(chan bool, 3)
	var ready atomic.Bool
	ready.Store(true)
	go watchReadiness(ctx, changes, ready.Load, func(state bool) { states <- state })

	ready.Store(false)
	changes <- struct{}{}
	expectReadyState(t, states, false)
	ready.Store(true)
	changes <- struct{}{}
	expectReadyState(t, states, true)
	cancel()
	expectReadyState(t, states, false)
}

func expectReadyState(t *testing.T, states <-chan bool, want bool) {
	t.Helper()
	select {
	case got := <-states:
		if got != want {
			t.Fatalf("ready state = %t, want %t", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("ready state did not change to %t", want)
	}
}

func TestMonitorTransactionDrainForcesStopAfterTimeout(t *testing.T) {
	shutdown := make(chan struct{})
	solversDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorTransactionDrain(shutdown, solversDone, time.Millisecond, func() {
			stopped <- struct{}{}
		})
	}()

	close(shutdown)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("transaction drain did not force stop after timeout")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transaction drain monitor did not return after timeout")
	}
}

func TestMonitorTransactionDrainStopsWhenSolversFinish(t *testing.T) {
	shutdown := make(chan struct{})
	solversDone := make(chan struct{})
	stopped := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorTransactionDrain(shutdown, solversDone, time.Hour, func() {
			stopped <- struct{}{}
		})
	}()

	close(shutdown)
	close(solversDone)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("transaction drain monitor did not stop with solvers")
	}
	select {
	case <-stopped:
		t.Fatal("transaction manager was stopped after solvers drained")
	default:
	}
}
