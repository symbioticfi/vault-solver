package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const compositionPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type runtimeCompositionCase struct {
	name                string
	config              string
	selectionDiagnostic string
	externallySubmitted bool
}

func runtimeCompositionCases() []runtimeCompositionCase {
	return []runtimeCompositionCase{
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
}

func TestRuntimeCommandCompositionCharacterization(t *testing.T) {
	setCompositionEnv(t)
	deps := newCompositionDeps(t)
	for _, test := range runtimeCompositionCases() {
		t.Run(test.name, func(t *testing.T) {
			testRuntimeCompositionCase(t, deps, test)
		})
	}
}

func testRuntimeCompositionCase(t *testing.T, deps solver.Deps, test runtimeCompositionCase) {
	t.Helper()
	entries := compositionEntries(t, []runtimeCompositionCase{test}, test.name)
	validatedRequiresTxManager, err := validateConfiguredSolvers(entries)
	if err != nil {
		t.Fatalf("validate configured solvers: %v", err)
	}
	constructed, constructedRequiresTxManager, err := constructConfiguredSolvers(entries, deps)
	if err != nil {
		t.Fatalf("construct configured solvers: %v", err)
	}
	if len(constructed) != 1 || constructed[0].Name() != test.name {
		t.Fatalf("constructed solvers = %v, want one named %q", solverNames(constructed), test.name)
	}
	wantRequiresTxManager := !test.externallySubmitted
	if validatedRequiresTxManager != wantRequiresTxManager ||
		constructedRequiresTxManager != wantRequiresTxManager {
		t.Fatalf(
			"requires tx manager from validation/construction = %t/%t, want %t",
			validatedRequiresTxManager, constructedRequiresTxManager, wantRequiresTxManager,
		)
	}

	invalid := test.config + "\nstrategy: {name: command-characterization-missing, config: {}}\n"
	if strings.Contains(test.config, "  name: default") {
		invalid = strings.Replace(test.config, "  name: default", "  name: command-characterization-missing", 1)
	}
	invalidEntries := []config.SolverConfig{{
		Name: test.name, Config: compositionYAMLNode(t, invalid),
	}}
	_, validationErr := validateConfiguredSolvers(invalidEntries)
	_, _, factoryErr := constructConfiguredSolvers(invalidEntries, deps)
	if validationErr == nil || factoryErr == nil ||
		!strings.Contains(validationErr.Error(), test.selectionDiagnostic) ||
		!strings.Contains(factoryErr.Error(), test.selectionDiagnostic) {
		t.Fatalf("validation/construction selection errors = (%v, %v), want both to contain %q",
			validationErr, factoryErr, test.selectionDiagnostic)
	}
}

func TestConfiguredSolverAggregateRequirementCharacterization(t *testing.T) {
	setCompositionEnv(t)
	deps := newCompositionDeps(t)
	cases := runtimeCompositionCases()
	for _, test := range []struct {
		name            string
		solverNames     []string
		requiresManager bool
	}{
		{
			name:            "OEV only",
			solverNames:     []string{"redstone-oev"},
			requiresManager: false,
		},
		{
			name:            "mixed OEV and transaction solver",
			solverNames:     []string{"redstone-oev", "lifi-samechain"},
			requiresManager: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := compositionEntries(t, cases, test.solverNames...)
			validatedRequiresManager, err := validateConfiguredSolvers(entries)
			if err != nil {
				t.Fatalf("validate configured solvers: %v", err)
			}
			constructed, constructedRequiresManager, err := constructConfiguredSolvers(entries, deps)
			if err != nil {
				t.Fatalf("construct configured solvers: %v", err)
			}
			if got := solverNames(constructed); !slices.Equal(got, test.solverNames) {
				t.Fatalf("constructed solver order = %v, want %v", got, test.solverNames)
			}
			if validatedRequiresManager != test.requiresManager ||
				constructedRequiresManager != test.requiresManager {
				t.Fatalf(
					"aggregate requires tx manager from validation/construction = %t/%t, want %t",
					validatedRequiresManager, constructedRequiresManager, test.requiresManager,
				)
			}
		})
	}
}

func compositionEntries(
	t *testing.T,
	cases []runtimeCompositionCase,
	names ...string,
) []config.SolverConfig {
	t.Helper()
	entries := make([]config.SolverConfig, 0, len(names))
	for _, name := range names {
		index := slices.IndexFunc(cases, func(test runtimeCompositionCase) bool {
			return test.name == name
		})
		if index < 0 {
			t.Fatalf("missing characterization config for %q", name)
		}
		entries = append(entries, config.SolverConfig{
			Name: name, Config: compositionYAMLNode(t, cases[index].config),
		})
	}
	return entries
}

func setCompositionEnv(t *testing.T) {
	t.Helper()
	for _, env := range []string{
		"COMPOSITION_RFQ_SECRET",
		"COMPOSITION_OEV_API_KEY",
		"COMPOSITION_LIFI_API_KEY",
		"COMPOSITION_UNISWAPX_API_KEY",
	} {
		t.Setenv(env, "local-test-value")
	}
}

func solverNames(solvers []solver.Solver) []string {
	names := make([]string, len(solvers))
	for i, slv := range solvers {
		names[i] = slv.Name()
	}
	return names
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
	assertLoopbackReleased(t, observabilityAddr)
	const want = `rfq-filler: backend shared secret env "COMPOSITION_MISSING_RFQ_SECRET" is empty`
	if err == nil || err.Error() != want {
		t.Fatalf("runBot error = %v, want %q", err, want)
	}
}

func TestUnknownSolverDiagnosticCharacterization(t *testing.T) {
	const want = `solver: unknown solver "future-global-solver" (registered: [3f-bridge-facilitator lifi-samechain redstone-oev rfq-filler uniswapx-filler])`
	entries := []config.SolverConfig{{
		Name: "future-global-solver", Config: compositionYAMLNode(t, "{}"),
	}}
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "runtime construction", run: func() error {
			_, _, err := constructConfiguredSolvers(entries, solver.Deps{})
			return err
		}},
		{name: "offline validation", run: func() error {
			_, err := validateConfiguredSolvers(entries)
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
