package rfq

import (
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"gopkg.in/yaml.v3"
)

type Config struct {
	filler.Config

	BackendURL string
	// TokenPolicy is the host's input-token scope; the zero value fills every awarded token.
	TokenPolicy tokenpolicy.Policy
}

func parseConfig(node yaml.Node) (Config, error) {
	var raw struct {
		filler.RawConfig `yaml:",inline"`

		BackendURL string `yaml:"backendUrl"`
	}
	if err := solver.DecodeStrict(node, &raw); err != nil {
		return Config{}, err
	}
	shared, err := raw.Parse()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Config: shared, BackendURL: raw.BackendURL}
	return cfg, cfg.Validate()
}

// Validate checks the reusable Merkle execution component's startup settings.
func (c Config) Validate() error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	if err := permissions.ValidateURL(c.BackendURL); err != nil {
		return errors.Errorf("backendUrl: %w", err)
	}
	return nil
}
