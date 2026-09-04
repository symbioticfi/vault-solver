package uniswapx

import (
	"gopkg.in/yaml.v3"

	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const defaultPlannerName = "default"

type defaultPlannerConfig = liquidplanning.PolicyConfig

type defaultPlanner struct {
	policy liquidplanning.Policy
}

func validateDefaultPlannerConfig(raw yaml.Node) error {
	_, err := newDefaultPlannerFromConfig(raw)
	return err
}

func newDefaultPlannerFromConfig(raw yaml.Node) (Planner, error) {
	var cfg defaultPlannerConfig
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return newDefaultPlanner(cfg)
}

func newDefaultPlanner(cfg defaultPlannerConfig) (*defaultPlanner, error) {
	policy, err := liquidplanning.NewPolicy(cfg)
	if err != nil {
		return nil, err
	}
	return &defaultPlanner{policy: policy}, nil
}
