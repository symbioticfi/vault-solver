package webhookstrategy

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategyregistry"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategytypes"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"

type Strategy struct {
	client *webhook.Client
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategyregistry.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, _ strategyregistry.Deps) (strategytypes.Strategy, error) {
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

func (s *Strategy) DecideOffers(
	ctx context.Context,
	input strategytypes.OfferInput,
) (strategytypes.OfferOutput, error) {
	var out strategytypes.OfferOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return strategytypes.OfferOutput{}, err
	}
	return out, nil
}

var _ strategytypes.Strategy = (*Strategy)(nil)
