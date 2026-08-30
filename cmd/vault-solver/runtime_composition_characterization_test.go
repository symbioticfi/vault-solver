package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const compositionPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func TestRuntimeRegistrationCompositionCharacterization(t *testing.T) {
	deps := newCompositionDeps(t)
	for _, env := range []string{
		"COMPOSITION_RFQ_SECRET",
		"COMPOSITION_OEV_API_KEY",
		"COMPOSITION_LIFI_API_KEY",
		"COMPOSITION_UNISWAPX_API_KEY",
	} {
		t.Setenv(env, "local-test-value")
	}

	tests := []struct {
		name                string
		config              string
		selectionDiagnostic string
		externallySubmitted bool
	}{
		{
			name:                "3f-bridge-facilitator",
			selectionDiagnostic: `unknown 3F strategy "command-characterization-missing"`,
			config: `
apiBaseUrl: https://3f.example
adapterFactory: "0x1111111111111111111111111111111111111111"
`,
		},
		{
			name:                "lifi-samechain",
			selectionDiagnostic: `unknown LI.FI strategy "command-characterization-missing"`,
			config: `
orderServer:
  baseUrl: https://order.example
  wsUrl: wss://order.example
  apiKeyEnv: COMPOSITION_LIFI_API_KEY
inputSettler: "0x2222222222222222222222222222222222222222"
outputSettler: "0x3333333333333333333333333333333333333333"
executor: "0x4444444444444444444444444444444444444444"
adapters: ["0x5555555555555555555555555555555555555555"]
`,
		},
		{
			name:                "redstone-oev",
			selectionDiagnostic: `unknown OEV strategy "command-characterization-missing"`,
			config: `
ws: {url: wss://oev.example, apiKeyEnv: COMPOSITION_OEV_API_KEY}
executor: "0x3333333333333333333333333333333333333333"
adapter: "0x4444444444444444444444444444444444444444"
callback: "0x5555555555555555555555555555555555555555"
strategy:
  name: default
  config:
    morphoApiUrl: https://api.morpho.example/graphql
    bid: {bidEth: "0.0001"}
`,
			externallySubmitted: true,
		},
		{
			name:                "rfq-filler",
			selectionDiagnostic: `unknown RFQ strategy "command-characterization-missing"`,
			config: `
backendUrl: https://rfq.example
backendSharedSecretEnv: COMPOSITION_RFQ_SECRET
executor: "0x2222222222222222222222222222222222222222"
solverMode: internal
`,
		},
		{
			name:                "uniswapx-filler",
			selectionDiagnostic: `unknown UniswapX strategy "command-characterization-missing"`,
			config: `
reactor: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
executor: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
adapters: ["0xcccccccccccccccccccccccccccccccccccccccc"]
quoteServer: {}
orderServer:
  baseUrl: https://api.uniswap.org/v2
  apiKeyEnv: COMPOSITION_UNISWAPX_API_KEY
  sources: {exclusiveV2: true}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := compositionYAMLNode(t, test.config)
			if err := solver.ValidateConfig(test.name, raw); err != nil {
				t.Fatalf("ValidateConfig: %v", err)
			}
			constructed, err := solver.New(test.name, raw, deps)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := constructed.Name(); got != test.name {
				t.Fatalf("constructed solver name = %q, want %q", got, test.name)
			}
			requiresTxManager, err := solver.RequiresTxManager(test.name)
			if err != nil {
				t.Fatalf("RequiresTxManager: %v", err)
			}
			if got := !requiresTxManager; got != test.externallySubmitted {
				t.Fatalf("externally submitted = %t, want %t", got, test.externallySubmitted)
			}

			invalid := test.config + "\nstrategy: {name: command-characterization-missing, config: {}}\n"
			if strings.Contains(test.config, "  name: default") {
				invalid = strings.Replace(test.config, "  name: default", "  name: command-characterization-missing", 1)
			}
			invalidRaw := compositionYAMLNode(t, invalid)
			validationErr := solver.ValidateConfig(test.name, invalidRaw)
			_, factoryErr := solver.New(test.name, invalidRaw, deps)
			if validationErr == nil || factoryErr == nil ||
				!strings.Contains(validationErr.Error(), test.selectionDiagnostic) ||
				!strings.Contains(factoryErr.Error(), test.selectionDiagnostic) {
				t.Fatalf("validator/factory selection errors = (%v, %v), want both to contain %q",
					validationErr, factoryErr, test.selectionDiagnostic)
			}
		})
	}
}

func TestRunBotFactoryFailureJoinsObservabilityServer(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("COMPOSITION_RUN_SIGNER", compositionPrivateKey)
	t.Setenv("COMPOSITION_MISSING_RFQ_SECRET", "")
	rpcServer := newCompositionRPCServer(t)
	observabilityAddr := reserveLoopbackAddr(t)
	configPath := writeConfigFile(t, `
chain:
  rpcUrl: "`+rpcServer.URL+`"
  chainId: 1
signer:
  keyEnv: COMPOSITION_RUN_SIGNER
observability:
  addr: "`+observabilityAddr+`"
solvers:
  - name: rfq-filler
    config:
      backendUrl: https://rfq.example
      backendSharedSecretEnv: COMPOSITION_MISSING_RFQ_SECRET
      executor: "0x2222222222222222222222222222222222222222"
      solverMode: internal
`)

	err := runBot(t.Context(), configPath, false, false)
	const want = `rfq-filler: backend shared secret env "COMPOSITION_MISSING_RFQ_SECRET" is empty`
	if err == nil || err.Error() != want {
		t.Fatalf("runBot error = %v, want %q", err, want)
	}
	waitForLoopbackRelease(t, observabilityAddr)
}

func TestUnknownSolverDiagnosticCharacterization(t *testing.T) {
	const want = `solver: unknown solver "future-global-solver" (registered: [3f-bridge-facilitator lifi-samechain redstone-oev rfq-filler uniswapx-filler])`
	raw := compositionYAMLNode(t, "{}")
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "runtime factory", run: func() error {
			_, err := solver.New("future-global-solver", raw, solver.Deps{})
			return err
		}},
		{name: "offline validator", run: func() error {
			return solver.ValidateConfig("future-global-solver", raw)
		}},
		{name: "submission metadata", run: func() error {
			_, err := solver.RequiresTxManager("future-global-solver")
			return err
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func newCompositionDeps(t *testing.T) solver.Deps {
	t.Helper()
	rpcServer := newCompositionRPCServer(t)
	chainClient, err := chain.Dial(
		t.Context(),
		[]string{rpcServer.URL},
		"",
		"0xcA11bde05977b3631167028862bE2a173976CA11",
		logr.Discard(),
	)
	if err != nil {
		t.Fatalf("chain.Dial: %v", err)
	}
	t.Cleanup(chainClient.Close)
	sgnr, err := signer.NewFromHexKey(compositionPrivateKey)
	if err != nil {
		t.Fatalf("signer.NewFromHexKey: %v", err)
	}
	txm := txmanager.New(chainClient, sgnr, chainClient.ChainID(), txmanager.Config{
		Confirmations:       2,
		MaxFeeGwei:          50,
		ReplacementInterval: time.Second,
		PendingTimeout:      2 * time.Second,
		ShutdownTimeout:     time.Second,
	}, logr.Discard())
	return solver.Deps{
		Chain:       chainClient,
		TxManager:   txm,
		Signer:      sgnr,
		Log:         logr.Discard(),
		ReportFatal: func(error) {},
	}
}

func newCompositionRPCServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  string          `json:"result"`
		}{JSONRPC: "2.0", ID: request.ID, Result: "0x1"}); err != nil {
			t.Errorf("encode JSON-RPC response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func compositionYAMLNode(t *testing.T, raw string) yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return *document.Content[0]
}
