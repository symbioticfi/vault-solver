package redstoneoev

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/decision"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const webhookPlannerName = "webhook"

type webhookPlanner struct {
	client *webhook.Client
}

const decideBidRoute = "/decide-bid"

func newWebhookPlannerFromConfig(raw yaml.Node) (decision.Planner, error) {
	client, err := webhook.NewClientFromConfig(raw)
	if err != nil {
		return nil, err
	}
	return &webhookPlanner{client: client}, nil
}

func (s *webhookPlanner) DecideBid(ctx context.Context, input decision.BidInput) (decision.BidOutput, error) {
	var out decision.BidOutput
	if err := s.client.DoJSON(ctx, decideBidRoute, input, &out); err != nil {
		return decision.BidOutput{}, err
	}
	return out, nil
}

var _ decision.Planner = (*webhookPlanner)(nil)
