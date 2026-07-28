package defaultstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
)

const Name = "default"

type Config struct{}

type Strategy struct{}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	return New(), nil
}

func New() *Strategy { return &Strategy{} }

func decodeConfig(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	return solver.DecodeStrict(node, out)
}

func (s *Strategy) DecideQuote(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	return s.decideQuote(ctx, input)
}

func (s *Strategy) BuildFillPlan(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	return s.buildFillPlan(ctx, input)
}
