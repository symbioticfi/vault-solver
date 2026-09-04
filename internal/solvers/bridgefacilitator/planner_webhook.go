package bridgefacilitator

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const webhookStrategyName = "webhook"

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

func (s *webhookPlanner) DecideOffers(
	ctx context.Context,
	input OfferInput,
) (OfferOutput, error) {
	var out OfferOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return OfferOutput{}, err
	}
	return out, nil
}

var _ Planner = (*webhookPlanner)(nil)
