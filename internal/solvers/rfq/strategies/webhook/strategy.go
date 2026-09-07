package webhookstrategy

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"

type Strategy struct {
	client *webhook.Client
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	client, err := webhook.NewFromConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(client), nil
}

func New(client *webhook.Client) *Strategy {
	return &Strategy{client: client}
}

func (s *Strategy) DecideQuote(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	return webhook.Post[types.QuoteOutput](ctx, s.client, "", input)
}

// BuildFillPlan delegates to the external decider against the current fill snapshot.
func (s *Strategy) BuildFillPlan(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	out, err := s.DecideQuote(ctx, input)
	if err != nil || out.Decision == types.DecisionDecline {
		return nil, err
	}
	return types.FillPlanFromQuote(input, out)
}
