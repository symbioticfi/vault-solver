package webhookstrategy

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	Name             = "webhook"
	decideQuoteRoute = "/decide-quote"
	decideFillRoute  = "/decide-fill"
)

type Strategy struct {
	client *webhook.Client
}

func ValidateConfig(raw yaml.Node) error {
	_, err := webhook.ParseConfig(raw)
	return err
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	cfg, err := webhook.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	client, err := webhook.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return New(client), nil
}

func New(client *webhook.Client) *Strategy {
	return &Strategy{client: client}
}

func (s *Strategy) DecideQuote(ctx context.Context, input types.QuoteInput) (*types.Quote, error) {
	var out *types.Quote
	if err := s.client.DoJSON(ctx, decideQuoteRoute, input, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	var out *types.FillPlan
	if err := s.client.DoJSON(ctx, decideFillRoute, input, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return out, nil
}

var _ types.Strategy = (*Strategy)(nil)
