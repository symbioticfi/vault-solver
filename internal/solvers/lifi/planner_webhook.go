package lifi

import (
	"context"
	"math/big"
	"net/http"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const (
	webhookPlannerName = "webhook"
	decideQuotesRoute  = "/decide-quotes"
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

func (s *webhookPlanner) DecideQuotes(ctx context.Context, input QuoteInput) (QuoteOutput, error) {
	var out QuoteOutput
	if err := s.client.DoJSON(ctx, decideQuotesRoute, input, &out); err != nil {
		return QuoteOutput{}, err
	}
	if err := validateQuotes(input, &out); err != nil {
		return QuoteOutput{}, err
	}
	return out, nil
}

func (s *webhookPlanner) DecideFill(ctx context.Context, input FillInput) (FillDecision, error) {
	var out *liquidlane.Plan
	if err := s.client.DoJSON(ctx, decideFillRoute, input, &out); err != nil {
		if webhook.IsHTTPStatus(err, http.StatusBadRequest, http.StatusUnprocessableEntity) {
			return FillDecision{}, MarkPermanentFillDecisionError(err)
		}
		return FillDecision{}, err
	}
	return FillDecision{Plan: out}, nil
}

type quotePair struct {
	from, to       common.Address
	fromDec, toDec int
}

func validateQuotes(input QuoteInput, out *QuoteOutput) error {
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
		if len(quote.Ranges) == 0 || len(quote.Ranges) > MaxQuoteRanges {
			return errors.Errorf("webhook quote %d has %d ranges, allowed [1,%d]", i, len(quote.Ranges), MaxQuoteRanges)
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

var _ Planner = (*webhookPlanner)(nil)
