package defaultstrategy

import (
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

const Name = "default"

type Config struct{}

type Strategy struct{}

func ValidateConfig(raw yaml.Node) error {
	_, err := NewFromConfig(raw)
	return err
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return New(), nil
}

func New() *Strategy { return &Strategy{} }
