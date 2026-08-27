package webhookstrategy

import (
	"context"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"

type Strategy struct {
	client *webhook.Client
}

const decideBidRoute = "/decide-bid"

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, strategies.Registration{
		Factory: NewFromConfig, ValidateConfig: ValidateConfig, RequiresBidCap: true,
	})
}

func ValidateConfig(raw yaml.Node, _ strategies.ValidationDeps) error {
	cfg, err := webhook.ParseConfig(raw)
	if err != nil {
		return err
	}
	_, err = webhook.NewClient(cfg)
	return err
}

func NewFromConfig(raw yaml.Node, _ strategies.Deps) (types.Strategy, error) {
	clientCfg, err := webhook.ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	client, err := webhook.NewClient(clientCfg)
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
	var out types.BidOutput
	if err := s.client.DoJSON(ctx, http.MethodPost, decideBidRoute, input, &out); err != nil {
		return types.BidOutput{}, err
	}
	return out, nil
}

var _ types.Strategy = (*Strategy)(nil)
