package observability

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServeUntil_OccupiedAddressIsFatal(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}
	err = ServeUntil(context.Background(), srv)
	if err == nil || !strings.Contains(err.Error(), "observability server") {
		t.Fatalf("error = %v, want contextual observability listener failure", err)
	}
}

func TestNewHTTPServer_HealthAndReadinessStatus(t *testing.T) {
	health := &Health{}
	srv := NewHTTPServer("127.0.0.1:0", NewMetrics(), health)

	assertStatus := func(path string, want int) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		srv.Handler.ServeHTTP(resp, req)
		if resp.Code != want {
			t.Fatalf("GET %s status = %d, want %d", path, resp.Code, want)
		}
	}

	assertStatus("/healthz", http.StatusOK)
	assertStatus("/readyz", http.StatusServiceUnavailable)
	health.SetReady(true)
	assertStatus("/readyz", http.StatusOK)
}

func TestServeUntil_CancellationIsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: time.Second,
	}
	if err := ServeUntil(ctx, srv); err != nil {
		t.Fatalf("ServeUntil: %v", err)
	}
}

func TestShutdownTimeoutConstant(t *testing.T) {
	if shutdownTimeout != 5*time.Second {
		t.Fatalf("shutdownTimeout = %s, want 5s", shutdownTimeout)
	}
}
