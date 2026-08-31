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

func ValidateConfig(raw yaml.Node) error {
	_, err := webhook.ParseConfig(raw)
	return err
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	clientCfg, err := webhook.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	client, err := webhook.NewClient(clientCfg)
	if err != nil {
		return nil, err
	}
	return &Strategy{client: client}, nil
}

func (s *Strategy) Run(context.Context) {}

func (s *Strategy) DecideBid(ctx context.Context, input types.BidInput) (types.BidOutput, error) {
	var out types.BidOutput
	if err := s.client.DoJSON(ctx, decideBidRoute, input, &out); err != nil {
		return types.BidOutput{}, err
	}
	return out, nil
}

var _ types.Strategy = (*Strategy)(nil)
