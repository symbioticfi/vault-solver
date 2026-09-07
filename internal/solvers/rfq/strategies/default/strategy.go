package defaultstrategy

import (
	"github.com/symbioticfi/vault-solver/internal/parse"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"
)

const Name = "default"

type Config struct{}

type Strategy struct{}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return New(), nil
}

func New() *Strategy { return &Strategy{} }
