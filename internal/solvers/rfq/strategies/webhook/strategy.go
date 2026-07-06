package webhookstrategy

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

type Strategy struct {
	client *webhook.Client
}

func NewFromConfig(raw yaml.Node) (*Strategy, error) {
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

func (s *Strategy) DecideQuote(ctx context.Context, input strategytypes.QuoteInput) (strategytypes.QuoteOutput, error) {
	var out strategytypes.QuoteOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return strategytypes.QuoteOutput{}, err
	}
	return out, nil
}
