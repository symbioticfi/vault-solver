package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-errors/errors"
)

func TestEndpointAttemptTimeout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured time.Duration
		remaining  time.Duration
		endpoints  int
		want       time.Duration
	}{
		{name: "default", endpoints: 1, want: 20 * time.Second},
		{name: "shorter", configured: 5 * time.Second, endpoints: 1, want: 5 * time.Second},
		{name: "longer", configured: time.Minute, endpoints: 1, want: time.Minute},
		{name: "cap", configured: 5 * time.Second, remaining: time.Minute, endpoints: 2, want: 5 * time.Second},
		{name: "caller budget split", configured: time.Minute, remaining: 4 * time.Second, endpoints: 2, want: 2 * time.Second},
		{name: "expired", configured: time.Minute, remaining: -time.Second, endpoints: 1},
		{name: "no endpoints", configured: time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				if tc.remaining != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Now().Add(tc.remaining))
					defer cancel()
				}
				got := endpointAttemptTimeout(ctx, tc.endpoints, tc.configured)
				if got != tc.want {
					t.Fatalf("timeout = %s, want %s", got, tc.want)
				}
			})
		})
	}
}

// Exercise the configured budget through real HTTP, including a stalled response body.
func TestDialRPCAttemptTimeout(t *testing.T) {
	for name, stallBody := range map[string]bool{"headers": false, "body": true} {
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				if req.Method == rpcMethodChainID {
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`))
					return
				}
				if stallBody {
					w.WriteHeader(http.StatusOK)
					_ = http.NewResponseController(w).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer func() { close(release); server.Close() }()
			client, err := Dial(t.Context(), []string{server.URL}, server.URL, server.URL,
				testMulticall, 100*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			for role, rpcClient := range map[string]*ethclient.Client{
				"read": client.Client, "write": client.writeClient, "cancel": client.cancelClient,
			} {
				t.Run(role, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					_, callErr := rpcClient.BlockNumber(ctx)
					if !errors.Is(callErr, context.DeadlineExceeded) || ctx.Err() != nil {
						t.Fatalf("call error = %v, parent error = %v; want attempt deadline only", callErr, ctx.Err())
					}
				})
			}
		})
	}
}
