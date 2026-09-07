package webhookstrategy

import (
	"context"

	"github.com/go-errors/errors"
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

func (s *Strategy) DecideQuote(ctx context.Context, input types.QuoteInput) (*types.Quote, error) {
	quote, err := webhook.Post[*types.Quote](ctx, s.client, decideQuoteRoute, input)
	if err != nil || quote == nil {
		return nil, err
	}
	if err := validateQuote(input, quote); err != nil {
		return nil, err
	}
	return quote, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	return webhook.Post[*types.FillPlan](ctx, s.client, decideFillRoute, input)
}

func validateQuote(input types.QuoteInput, quote *types.Quote) error {
	if quote.AmountIn == nil || quote.AmountIn.Sign() <= 0 || quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		return errors.New("webhook quote amounts must be positive")
	}
	if input.AmountIn != nil && quote.AmountIn.Cmp(input.AmountIn) != 0 {
		return errors.New("webhook quote changed exact-input amount")
	}
	if input.AmountOut != nil && quote.AmountOut.Cmp(input.AmountOut) != 0 {
		return errors.New("webhook quote changed exact-output amount")
	}
	return nil
}

var _ types.Strategy = (*Strategy)(nil)
