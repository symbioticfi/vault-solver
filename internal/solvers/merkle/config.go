package merkle

import (
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

const (
	recipeCacheBytes        int64 = 16 << 20
	defaultPollInterval           = 15 * time.Second
	defaultOperationTimeout       = 30 * time.Second
	defaultStrategyInterval       = time.Minute
)

type rawConfig struct {
	Mode       string             `yaml:"mode" json:"mode"`
	Curator    merklehost.Curator `yaml:"curator" json:"curator"`
	Strategies []string           `yaml:"strategies" json:"strategies"`
}
type config struct {
	mode       string
	curator    merklehost.Curator
	strategies []keeper.Strategy
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
	if len(raw.Strategies) == 0 || len(raw.Strategies) > 256 {
		return config{}, errors.New("merkle 1..256 strategies required")
	}
	if err := permissions.ValidateURL(raw.Curator.URL); err != nil {
		return config{}, err
	}
	cfg := config{mode: raw.Mode, curator: raw.Curator}
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
