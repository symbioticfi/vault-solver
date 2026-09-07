package webhookstrategy

import (
	"context"
	"math/big"
	"net/http"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	Name              = "webhook"
	decideQuotesRoute = "/decide-quotes"
	decideFillRoute   = "/decide-fill"
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

func (s *Strategy) DecideQuotes(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	output, err := webhook.Post[types.QuoteOutput](ctx, s.client, decideQuotesRoute, input)
	if err == nil {
		err = validateQuotes(input, &output)
	}
	if err != nil {
		return types.QuoteOutput{}, err
	}
	return output, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	plan, err := webhook.Post[*types.FillPlan](ctx, s.client, decideFillRoute, input)
	if webhook.IsHTTPStatus(err, http.StatusBadRequest, http.StatusUnprocessableEntity) {
		return nil, types.MarkPermanentFillDecisionError(err)
	}
	return plan, err
}

type quotePair struct {
	from, to       common.Address
	fromDec, toDec int
}

func validateQuotes(input types.QuoteInput, out *types.QuoteOutput) error {
	pairs := make(map[quotePair]bool)
	for _, item := range input.Inventory {
		pairs[quotePair{item.TokenIn, item.TokenOut, item.TokenInDecimals, item.TokenOutDecimals}] = false
	}
	canonical := make([]types.Quote, len(out.Quotes))
	for index, proposed := range out.Quotes {
		key := quotePair{proposed.FromAsset, proposed.ToAsset, proposed.FromDecimals, proposed.ToDecimals}
		used, known := pairs[key]
		if !known {
			return errors.Errorf("webhook quote %d uses unknown token pair", index)
		}
		if used {
			return errors.Errorf("webhook quote %d repeats token pair", index)
		}
		pairs[key] = true
		if proposed.Expiry <= input.ServerTime.Unix() || proposed.Expiry > input.QuoteExpiresAt.Unix() {
			return errors.Errorf("webhook quote %d expiry is outside the solver window", index)
		}
		if len(proposed.Ranges) < 1 || len(proposed.Ranges) > types.MaxQuoteRanges {
			return errors.Errorf("webhook quote %d has %d ranges, allowed [1,%d]", index, len(proposed.Ranges), types.MaxQuoteRanges)
		}
		ranges := slices.Clone(proposed.Ranges)
		for r, price := range ranges {
			if price.MinAmount == nil || price.MaxAmount == nil || price.MinAmount.Sign() <= 0 || price.MinAmount.Cmp(price.MaxAmount) > 0 || price.Quote == "" {
				return errors.Errorf("webhook quote %d range %d is invalid", index, r)
			}
			rate, valid := new(big.Rat).SetString(price.Quote)
			if !valid || rate.Sign() <= 0 {
				return errors.Errorf("webhook quote %d range %d rate is invalid", index, r)
			}
		}
		slices.SortFunc(ranges, func(a, b types.QuoteRange) int { return a.MinAmount.Cmp(b.MinAmount) })
		for r := 1; r < len(ranges); r++ {
			if ranges[r-1].MaxAmount.Cmp(ranges[r].MinAmount) >= 0 {
				return errors.Errorf("webhook quote %d ranges overlap", index)
			}
		}
		proposed.Ranges, proposed.ExclusiveFor = ranges, input.Solver
		canonical[index] = proposed
	}
	out.Quotes = canonical
	return nil
}

var _ types.Strategy = (*Strategy)(nil)
