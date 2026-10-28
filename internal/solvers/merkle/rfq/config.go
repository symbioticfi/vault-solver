package rfq

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/internal/merklehost"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"gopkg.in/yaml.v3"
)

type config struct {
	BackendURL string
	StateDir   string
	Executor   common.Address
	Curator    merklehost.Curator
	Connectors map[common.Address]bool
}

func parseConfig(node yaml.Node) (config, error) {
	var raw struct {
		BackendURL string             `yaml:"backendUrl"`
		StateDir   string             `yaml:"stateDir"`
		Executor   string             `yaml:"executor"`
		Curator    merklehost.Curator `yaml:"curator"`
		Connectors []string           `yaml:"connectors"`
	}
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return config{}, err
	}
	if err := permissions.ValidateConfig(protocolio.HTTPConfig{URL: raw.BackendURL, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}); err != nil {
		return config{}, errors.Errorf("backendUrl: %w", err)
	}
	if err := permissions.ValidateConfig(raw.Curator.HTTPConfig()); err != nil {
		return config{}, errors.Errorf("curator: %w", err)
	}
	if raw.StateDir == "" || len(raw.Connectors) == 0 || len(raw.Connectors) > 256 {
		return config{}, errors.New("rfq-merkle requires stateDir and 1..256 connectors")
	}
	executor, err := parse.NonZeroAddress(raw.Executor, "executor")
	if err != nil {
		return config{}, err
	}
	cfg := config{BackendURL: raw.BackendURL, StateDir: raw.StateDir, Executor: executor, Curator: raw.Curator, Connectors: make(map[common.Address]bool)}
	for _, venue := range raw.Connectors {
		connector, err := parse.NonZeroAddress(venue, "connector")
		if err != nil {
			return config{}, err
		}
		if cfg.Connectors[connector] {
			return config{}, errors.New("duplicate connector")
		}
		cfg.Connectors[connector] = true
	}
	return cfg, nil
}
