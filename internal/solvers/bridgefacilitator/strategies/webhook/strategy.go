package webhookstrategy

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
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

func (s *Strategy) DecideOffers(ctx context.Context, input types.OfferInput) (types.OfferOutput, error) {
	return webhook.Post[types.OfferOutput](ctx, s.client, "", input)
}

var _ types.Strategy = (*Strategy)(nil)
