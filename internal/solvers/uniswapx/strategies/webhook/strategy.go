package webhookstrategy

import (
	"context"
	"net/http"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies"
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

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
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
	if err := s.client.DoJSON(ctx, http.MethodPost, decideQuoteRoute, input, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	if err := validateQuote(input, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	var out *types.FillPlan
	if err := s.client.DoJSON(ctx, http.MethodPost, decideFillRoute, input, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return out, nil
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
