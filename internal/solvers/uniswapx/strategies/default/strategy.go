package defaultstrategy

import (
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/parse"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

const Name = "default"

type Config = planning.ExecutionConfig

type Strategy struct {
	policy planning.ExecutionPolicy
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(cfg Config) (*Strategy, error) {
	policy, err := cfg.Parse()
	if err != nil {
		return nil, err
	}
	return &Strategy{policy: policy}, nil
}
