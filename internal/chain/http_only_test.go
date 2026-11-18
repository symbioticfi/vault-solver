package chain

import (
	"strings"
	"testing"
)

func TestDialRejectsNonHTTPEndpoints(t *testing.T) {
	server := rpcRecorder(new([]string), chainRPCResult)
	defer server.Close()
	for _, endpoint := range []string{"ws://127.0.0.1:1", "wss://127.0.0.1:1", "/tmp/unused-geth.ipc"} {
		for _, role := range []string{"primary", "fallback", "write"} {
			t.Run(role+"/"+endpoint, func(t *testing.T) {
				urls, write := []string{server.URL}, ""
				switch role {
				case "primary":
					urls = []string{endpoint}
				case "fallback":
					urls = append(urls, endpoint)
				case "write":
					write = endpoint
				}
				client, err := Dial(t.Context(), urls, write, testMulticall, 0)
				if client != nil {
					client.Close()
				}
				if err == nil || !strings.Contains(err.Error(), "http(s) only") {
					t.Fatalf("expected an HTTP(S)-only endpoint error, got %v", err)
				}
			})
		}
	}
}
