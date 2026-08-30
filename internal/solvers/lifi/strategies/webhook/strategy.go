package webhookstrategy

import (
	"context"
	"math/big"
	"net/http"
	"sort"

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

func (s *Strategy) DecideQuotes(ctx context.Context, input types.QuoteInput) (types.QuoteOutput, error) {
	var out types.QuoteOutput
	if err := s.client.DoJSON(ctx, http.MethodPost, decideQuotesRoute, input, &out); err != nil {
		return types.QuoteOutput{}, err
	}
	if err := validateQuotes(input, &out); err != nil {
		return types.QuoteOutput{}, err
	}
	return out, nil
}

func (s *Strategy) DecideFill(ctx context.Context, input types.FillInput) (*types.FillPlan, error) {
	var out *types.FillPlan
	if err := s.client.DoJSON(ctx, http.MethodPost, decideFillRoute, input, &out); err != nil {
		if webhook.IsHTTPStatus(err, http.StatusBadRequest, http.StatusUnprocessableEntity) {
			return nil, types.MarkPermanentFillDecisionError(err)
		}
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return out, nil
}

type quotePair struct {
	from, to       common.Address
	fromDec, toDec int
}

func validateQuotes(input types.QuoteInput, out *types.QuoteOutput) error {
	pairs := make(map[quotePair]bool)
	seen := make(map[quotePair]bool)
	for _, candidate := range input.Inventory {
		pairs[quotePair{
			from: candidate.TokenIn, to: candidate.TokenOut,
			fromDec: candidate.TokenInDecimals, toDec: candidate.TokenOutDecimals,
		}] = true
	}
	for i := range out.Quotes {
		quote := &out.Quotes[i]
		pair := quotePair{quote.FromAsset, quote.ToAsset, quote.FromDecimals, quote.ToDecimals}
		if !pairs[pair] {
			return errors.Errorf("webhook quote %d uses unknown token pair", i)
		}
		if seen[pair] {
			return errors.Errorf("webhook quote %d repeats token pair", i)
		}
		seen[pair] = true
		if quote.Expiry <= input.ServerTime.Unix() || quote.Expiry > input.QuoteExpiresAt.Unix() {
			return errors.Errorf("webhook quote %d expiry is outside the solver window", i)
		}
		if len(quote.Ranges) == 0 || len(quote.Ranges) > types.MaxQuoteRanges {
			return errors.Errorf("webhook quote %d has %d ranges, allowed [1,%d]", i, len(quote.Ranges), types.MaxQuoteRanges)
		}
		for j, priceRange := range quote.Ranges {
			rate, rateOK := new(big.Rat).SetString(priceRange.Quote)
			if priceRange.MinAmount == nil || priceRange.MaxAmount == nil || priceRange.MinAmount.Sign() <= 0 ||
				priceRange.MinAmount.Cmp(priceRange.MaxAmount) > 0 || priceRange.Quote == "" {
				return errors.Errorf("webhook quote %d range %d is invalid", i, j)
			}
			if !rateOK || rate.Sign() <= 0 {
				return errors.Errorf("webhook quote %d range %d rate is invalid", i, j)
			}
		}
		sort.Slice(quote.Ranges, func(i, j int) bool { return quote.Ranges[i].MinAmount.Cmp(quote.Ranges[j].MinAmount) < 0 })
		for j, priceRange := range quote.Ranges {
			if j > 0 && quote.Ranges[j-1].MaxAmount.Cmp(priceRange.MinAmount) >= 0 {
				return errors.Errorf("webhook quote %d ranges overlap", i)
			}
		}
		quote.ExclusiveFor = input.Solver
	}
	return nil
}

var _ types.Strategy = (*Strategy)(nil)
