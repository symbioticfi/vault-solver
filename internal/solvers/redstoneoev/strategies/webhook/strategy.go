package webhookstrategy

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"

type Strategy struct {
	client *webhook.Client
}

const decideBidRoute = "/decide-bid"

func NewFromConfig(raw yaml.Node, _ types.Dependencies) (types.Strategy, error) {
	client, err := webhook.NewFromConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(client), nil
}

func New(client *webhook.Client) *Strategy {
	return &Strategy{client: client}
}

func (s *Strategy) Run(context.Context) {}

func (s *Strategy) DecideBid(ctx context.Context, input types.BidInput) (types.BidOutput, error) {
	return webhook.Post[types.BidOutput](ctx, s.client, decideBidRoute, input)
}

var _ types.Strategy = (*Strategy)(nil)
