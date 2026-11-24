package iris

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler/fillertest"
)

const validConfig = `
iris: "0x77310475712D7a127D694B3e9D186d9f4a423bd9"
account: "0x0000000000000000000000000000000000000ACC"
recipeExecutors: ["0x00000000000000000000000000000000000000E0"]
curator: {url: "https://curator.example.com"}
server: {authHeader: x-api-key, authValueEnv: IRIS_WEBHOOK_KEY}
pricing:
  defaultFixedRate: "60000000000000000"
  rates:
    - {debtToken: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", venueId: 1, fixedRate: "55000000000000000"}
`

func decodeConfig(t *testing.T, text string) (Config, error) {
	t.Helper()
	var node yaml.Node
	must(t, yaml.Unmarshal([]byte(text), &node))
	return parseConfig(*node.Content[0])
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := decodeConfig(t, validConfig)
	must(t, err)
	equal(t, common.HexToAddress("0xACC"), cfg.Account)
	equal(t, []common.Address{common.HexToAddress("0xE0")}, cfg.RecipeExecutors)
	equal(t, defaultListenAddress, cfg.Server.ListenAddress)
	equal(t, big.NewInt(6e16), cfg.Pricing.Default)
	equal(t, 1, len(cfg.Pricing.Rules))
	equal(t, uint64(1), *cfg.Pricing.Rules[0].VenueID)
	equal(t, Timing{
		BlockTime: defaultBlockTime, SubmitLead: defaultSubmitLead, QuoteTTL: defaultQuoteTTL, DeadlineWindow: defaultDeadlineWindow,
		ResponseBudget: defaultResponseBudget, PollInterval: defaultPollInterval,
	}, cfg.Timing)
	equal(t, time.Minute, cfg.RefreshInterval)
}

func TestParseConfigRejects(t *testing.T) {
	tests := []struct{ name, from, to string }{
		{"no executors", `recipeExecutors: ["0x00000000000000000000000000000000000000E0"]`, `recipeExecutors: []`},
		{"duplicate executors", `["0x00000000000000000000000000000000000000E0"]`, `["0x00000000000000000000000000000000000000E0", "0x00000000000000000000000000000000000000E0"]`},
		{"zero account", `"0x0000000000000000000000000000000000000ACC"`, `"0x0000000000000000000000000000000000000000"`},
		{"rate off the 1bp grid", `"60000000000000000"`, `"60000000000000001"`},
		{"rate above 100%", `"60000000000000000"`, `"2000000000000000000"`},
		{"no auth value", `authValueEnv: IRIS_WEBHOOK_KEY`, `authValueEnv: ""`},
		{"no auth header", `authHeader: x-api-key`, `authHeader: ""`},
		{"bad curator", `https://curator.example.com`, `ftp://curator`},
		{"unknown field", `iris:`, "extra: 1\niris:"},
		{"venue out of range", `venueId: 1`, `venueId: 128`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok(t, strings.Contains(validConfig, tt.from), validConfig)
			_, err := decodeConfig(t, strings.Replace(validConfig, tt.from, tt.to, 1))
			fails(t, err)
		})
	}
}

func TestParseTimingRejects(t *testing.T) {
	for _, timing := range []string{
		"timing: {blockTime: 1500ms}",
		"timing: {deadlineWindow: 3m}",
		"timing: {responseSlack: -1s}",
	} {
		_, err := decodeConfig(t, validConfig+timing+"\n")
		fails(t, err)
	}
}

func TestExampleConfigParses(t *testing.T) {
	cfg, err := parseConfig(fillertest.ExampleConfig(t, "../../../config/iris.example.yaml"))
	must(t, err)
	equal(t, ":42090", cfg.Server.ListenAddress)
	equal(t, big.NewInt(50_000_000_000), cfg.Pricing.MaxBond)
}
