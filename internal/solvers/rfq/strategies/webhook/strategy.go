package webhookstrategy

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"

type Strategy struct {
	client *webhook.Client
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, _ strategies.Deps) (types.Strategy, error) {
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

func (s *Strategy) DecideQuote(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	var out types.QuoteOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return types.QuoteOutput{}, err
	}
	return out, nil
}

// BuildFillPlan delegates to the external decider on every call and keeps no local cache. The remote
// implementer owns caching the quote-time decision and validating the fill against the awarded order
// (the fill request carries the order's AmountIn and RequiredAmountOut). We only assemble the returned
// candidate legs into a fill plan against the solver's trusted candidate snapshot.
func (s *Strategy) BuildFillPlan(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	quoteInput := types.QuoteInput(input)
	quoteInput.AmountIn = cloneBig(input.AmountIn)
	quoteInput.RequiredAmountOut = cloneBig(input.RequiredAmountOut)
	out, err := s.DecideQuote(ctx, quoteInput)
	if err != nil || out.Decision == types.DecisionDecline {
		return nil, err
	}
	return fillPlanFromQuote(quoteInput, out)
}

func fillPlanFromQuote(input types.QuoteInput, out types.QuoteOutput) (*types.FillPlan, error) {
	if out.QuotedAmountOut == nil || out.QuotedAmountOut.Sign() <= 0 {
		return nil, errors.New("quote output is missing a positive quotedAmountOut")
	}
	if len(out.Legs) == 0 {
		return nil, errors.New("quote output has no legs")
	}
	candidates := make(map[string]types.QuoteCandidate, len(input.Candidates))
	for _, c := range input.Candidates {
		candidates[c.ID] = c
	}
	legs := make([]types.FillLeg, 0, len(out.Legs))
	for _, leg := range out.Legs {
		c, ok := candidates[leg.CandidateID]
		if !ok {
			return nil, errors.Errorf("unknown candidate %q", leg.CandidateID)
		}
		// Crash-safety only (not economic re-validation): the strategy is trusted for pricing, but a
		// missing/omitted amount would be a nil *big.Int that panics downstream in directSwaps.
		if leg.AmountIn == nil || leg.AmountIn.Sign() <= 0 || leg.AmountOut == nil || leg.AmountOut.Sign() <= 0 {
			return nil, errors.Errorf("leg %q has non-positive amounts", leg.CandidateID)
		}
		legs = append(legs, types.FillLeg{
			Adapter:    c.Adapter,
			AmountIn:   cloneBig(leg.AmountIn),
			AmountOut:  cloneBig(leg.AmountOut),
			MaxRate:    cloneBig(c.MaxRate),
			DiscountID: cloneHash(c.DiscountID),
		})
	}
	return &types.FillPlan{
		QuoteID:         input.QuoteID,
		RequestID:       input.RequestID,
		TokenIn:         input.TokenIn,
		TokenOut:        input.TokenOut,
		AmountIn:        cloneBig(input.AmountIn),
		QuotedAmountOut: cloneBig(out.QuotedAmountOut),
		Legs:            legs,
	}, nil
}

func cloneBig(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}

func cloneHash(h *common.Hash) *common.Hash {
	if h == nil {
		return nil
	}
	out := *h
	return &out
}
