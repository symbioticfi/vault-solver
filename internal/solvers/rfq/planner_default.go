package rfq

import (
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

const defaultPlannerName = "default"

type defaultPlannerConfig struct{}

type defaultPlanner struct{}

func validateDefaultPlannerConfig(raw yaml.Node) error {
	_, err := newDefaultPlannerFromConfig(raw)
	return err
}

func newDefaultPlannerFromConfig(raw yaml.Node) (Planner, error) {
	if err := parse.DecodeStrict(raw, &defaultPlannerConfig{}); err != nil {
		return nil, err
	}
	return newDefaultPlanner(), nil
}

func newDefaultPlanner() *defaultPlanner { return new(defaultPlanner) }
