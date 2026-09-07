package webhookstrategy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	testcheck "github.com/symbioticfi/vault-solver/internal/testutil"
	"gopkg.in/yaml.v3"
)

func testYAMLNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var node yaml.Node
	testcheck.NoError(t, yaml.Unmarshal([]byte(raw), &node), "unmarshal yaml: %v")
	if len(node.Content) == 0 {
		return yaml.Node{}
	}
	return *node.Content[0]
}

func TestWebhookStrategyRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oev/decide-bid":
			_, _ = w.Write([]byte(`{"decision":"skip","reason":"test"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	strategy, err := NewFromConfig(testYAMLNode(t, "url: "+srv.URL+"/oev\n"), nilDeps())
	testcheck.NoError(t, err, "NewFromConfig: %v")
	out, err := strategy.DecideBid(t.Context(), types.BidInput{})
	testcheck.NoError(t, err, "DecideBid: %v")
	if out.Decision != "skip" || out.Reason != "test" {
		t.Fatalf("output = %+v, want skip/test", out)
	}
}

func TestWebhookStrategyRejectsCallbackListConfig(t *testing.T) {
	_, err := NewFromConfig(testYAMLNode(t, `
url: https://strategy.example/oev
callbacks:
  - "0x7Aa367073B5c2b6Db34cF843d2f1FEbd9dC042B1"
`), nilDeps())
	if err == nil || !strings.Contains(err.Error(), "field callbacks not found") {
		t.Fatalf("NewFromConfig error = %v, want callbacks unknown field", err)
	}
}

func nilDeps() types.Dependencies {
	return types.Dependencies{}
}
