package rfq

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const webhookPlannerName = "webhook"

type webhookPlanner struct {
	client *webhook.Client
}

func newWebhookPlannerFromConfig(raw yaml.Node) (Planner, error) {
	client, err := webhook.NewClientFromConfig(raw)
	if err != nil {
		return nil, err
	}
	return &webhookPlanner{client: client}, nil
}

func (s *webhookPlanner) DecideQuote(ctx context.Context, input QuoteInput) (QuoteOutput, error) {
	var out QuoteOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return QuoteOutput{}, err
	}
	return out, nil
}

// BuildFillPlan delegates to the external decider against the current fill snapshot.
func (s *webhookPlanner) BuildFillPlan(ctx context.Context, input FillInput) (*liquidlane.Plan, error) {
	out, err := s.DecideQuote(ctx, input)
	if err != nil || out.Decision == DecisionDecline {
		return nil, err
	}
	return FillPlanFromQuote(input, out)
}

var _ Planner = (*webhookPlanner)(nil)
