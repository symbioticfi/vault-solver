package chain

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRPCFallbackAuth(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		primaryAuth, fallbackAuth, primaryOK bool
	}{
		{name: "distinct endpoint credentials", primaryAuth: true, fallbackAuth: true},
		{name: "unauthenticated fallback", primaryAuth: true},
		{name: "authenticated fallback", fallbackAuth: true},
		{name: "primary success", primaryAuth: true, primaryOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fallbackHits int
			checkAuth := func(r *http.Request, enabled bool, user, password string) {
				u, p, ok := r.BasicAuth()
				if ok != enabled || (enabled && (u != user || p != password)) || (!enabled && r.Header.Get("Authorization") != "") {
					t.Error("endpoint received incorrect authentication")
				}
			}
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkAuth(r, tc.primaryAuth, "synthetic-primary", "primary-password")
				if !tc.primaryOK {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			}))
			defer primary.Close()
			fallback := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				fallbackHits++
				checkAuth(r, tc.fallbackAuth, "synthetic-fallback", "fallback-password")
			}))
			defer fallback.Close()
			endpoints := mustEndpoints(t, primary.URL, fallback.URL)
			if tc.primaryAuth {
				endpoints[0].User = url.UserPassword("synthetic-primary", "primary-password")
			}
			if tc.fallbackAuth {
				endpoints[1].User = url.UserPassword("synthetic-fallback", "fallback-password")
			}
			client := &http.Client{Transport: &fallbackTransport{endpoints: endpoints, base: http.DefaultTransport}}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoints[0].String(), strings.NewReader(`{}`))
			if err != nil {
				t.Fatal("construct request")
			}
			req.Header.Set("X-Test", "unchanged")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal("local request failed")
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if req.Header.Get("Authorization") != "" || req.Header.Get("X-Test") != "unchanged" {
				t.Error("caller headers mutated")
			}
			if (fallbackHits == 0) != tc.primaryOK {
				t.Error("unexpected fallback use")
			}
		})
	}
}
