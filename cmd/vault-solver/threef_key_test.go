package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestThreeFKeyWritesPrivateFileAndNeverRotatesOverExistingOutput(t *testing.T) {
	t.Setenv("TEST_BF_SIGNER", "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/facilitator/generate-key" || r.Method != http.MethodPost {
			t.Error("unexpected key-generation route")
		}
		_, _ = w.Write([]byte(`{"apiKey":"generated-test-key"}`))
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "credential")
	if err := generateThreeFKey(t.Context(), server.URL, "TEST_BF_SIGNER", path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || string(data) != "generated-test-key\n" {
		t.Fatal("credential output is not private or exact")
	}
	if err := generateThreeFKey(t.Context(), server.URL, "TEST_BF_SIGNER", path); err == nil || calls != 1 {
		t.Fatal("existing output must prevent key rotation")
	}
}
