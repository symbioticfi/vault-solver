package merkle

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const (
	runtimePolicy                 = "1"
	recipeCacheBytes        int64 = 16 << 20
	defaultPollInterval           = 15 * time.Second
	defaultOperationTimeout       = 30 * time.Second
	defaultStrategyInterval       = time.Minute
)

type rawConfig struct {
	Mode       string             `yaml:"mode" json:"mode"`
	StateDir   string             `yaml:"stateDir" json:"stateDir"`
	Curator    merklehost.Curator `yaml:"curator" json:"curator"`
	Strategies []string           `yaml:"strategies" json:"strategies"`
}
type config struct {
	mode, stateDir string
	curator        merklehost.Curator
	strategies     []keeper.Strategy
}

func parseConfig(node yaml.Node) (config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return config{}, err
	}
	raw.Mode = parse.OrDefault(raw.Mode, "simulate")
	if raw.Mode != "send" && raw.Mode != "simulate" {
		return config{}, errors.New("merkle mode must be send or simulate")
	}
	if raw.StateDir == "" || len(raw.Strategies) == 0 || len(raw.Strategies) > 256 {
		return config{}, errors.New("merkle stateDir and 1..256 strategies required")
	}
	if err := permissions.ValidateConfig(raw.Curator.HTTPConfig()); err != nil {
		return config{}, err
	}
	cfg := config{mode: raw.Mode, stateDir: raw.StateDir, curator: raw.Curator}
	seen := make(map[string]bool)
	for _, value := range raw.Strategies {
		executor, err := parse.NonZeroAddress(value, "merkle executor")
		if err != nil {
			return config{}, err
		}
		canonical := strings.ToLower(executor.Hex())
		if seen[canonical] {
			return config{}, errors.Errorf("merkle duplicate executor address %s", canonical)
		}
		seen[canonical] = true
		cfg.strategies = append(cfg.strategies, keeper.Strategy{Config: keeper.Config{Name: canonical, Executor: canonical, Interval: defaultStrategyInterval}})
	}
	return cfg, nil
}

// Bind derived identity and the shared source to existing recovery guards.
func strategyRevision(strategy keeper.Config, curator merklehost.Curator, caller common.Address) (string, error) {
	raw, err := json.Marshal(struct {
		Policy   string                `json:"policy"`
		Strategy keeper.Config         `json:"strategy"`
		Curator  protocolio.HTTPConfig `json:"curator"`
		Caller   common.Address        `json:"caller"`
	}{runtimePolicy, strategy, curator.HTTPConfig(), caller})
	if err != nil {
		return "", errors.Errorf("merkle strategy identity: %w", err)
	}
	return crypto.Keccak256Hash(raw).Hex(), nil
}
