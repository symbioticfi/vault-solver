package uniswapx

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	webhookPlannerName = "webhook"
	decideQuoteRoute   = "/decide-quote"
	decideFillRoute    = "/decide-fill"
)

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

func (s *webhookPlanner) DecideQuote(ctx context.Context, input QuoteInput) (*Quote, error) {
	var out *Quote
	if err := s.client.DoJSON(ctx, decideQuoteRoute, input, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *webhookPlanner) DecideFill(ctx context.Context, input FillInput) (*liquidlane.Plan, error) {
	var out *liquidlane.Plan
	if err := s.client.DoJSON(ctx, decideFillRoute, input, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var _ Planner = (*webhookPlanner)(nil)
