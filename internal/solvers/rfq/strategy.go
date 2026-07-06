package rfq

import (
	"context"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlanemath"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	webhookstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
)

type strategyDeps struct {
	Chain *chain.Client
	Log   logr.Logger
}

func newStrategy(spec StrategyConfig, deps strategyDeps) (strategytypes.Strategy, error) {
	name := spec.Name
	if name == "" {
		name = defaultStrategyName
	}
	switch name {
	case defaultStrategyName:
		return defaultstrategy.NewFromConfig(spec.Config, deps.Chain, deps.Log)
	case "webhook":
		return webhookstrategy.NewFromConfig(spec.Config)
	default:
		return nil, errors.Errorf("unknown RFQ strategy %q", spec.Name)
	}
}

// solverInventory is one candidate adapter leg, taken from the backend quote request's snapshot
// (the filler does not re-read maxAssets/maxRate/decimals on-chain in the quote path). "adapter" is
// the address that fills (placed in the on-chain Swap's vault slot); "asset" is the output token.
type solverInventory struct {
	ID            string
	Adapter       common.Address
	Asset         common.Address
	AssetDecimals int
	MaxAssets     *big.Int
	MaxRate       *big.Int
	DiscountID    *common.Hash // nil for a direct leg; set for a discount leg
}

// strategyLeg is one filled leg of a selected strategy.
type strategyLeg struct {
	Adapter    common.Address
	AmountIn   *big.Int
	AmountOut  *big.Int
	MaxRate    *big.Int
	DiscountID *common.Hash
}

// strategyRecord is the selected execution plan for a quote, persisted by quoteId so execution can
// recover it after the backend awards the order.
type strategyRecord struct {
	QuoteID         string
	RequestID       string
	TokenIn         common.Address
	TokenOut        common.Address
	AmountIn        *big.Int
	QuotedAmountOut *big.Int
	Legs            []strategyLeg
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// strategyRequest is the subset of a quote request the selector needs.
type strategyRequest struct {
	RequestID string
	QuoteID   string
	TokenIn   common.Address
	TokenOut  common.Address
	Amount    *big.Int
}

type amountOutRequest struct {
	Adapter  common.Address
	AmountIn *big.Int
}

type quoteReplayReader interface {
	tokenDecimals(ctx context.Context, token common.Address) (int, error)
	amountsOut(ctx context.Context, tokenIn common.Address, requests []amountOutRequest) ([]*big.Int, error)
}

func newQuoteInput(
	chainID int64,
	executor common.Address,
	req strategyRequest,
	inv []solverInventory,
	required *big.Int,
	now time.Time,
) strategytypes.QuoteInput {
	candidates := make([]strategytypes.QuoteCandidate, 0, len(inv))
	for i, v := range inv {
		id := v.ID
		if id == "" {
			id = "candidate-" + strconv.Itoa(i)
		}
		candidates = append(candidates, strategytypes.QuoteCandidate{
			ID:            id,
			Adapter:       v.Adapter,
			Asset:         v.Asset,
			AssetDecimals: v.AssetDecimals,
			MaxAssets:     cloneBig(v.MaxAssets),
			MaxRate:       cloneBig(v.MaxRate),
			DiscountID:    cloneHash(v.DiscountID),
		})
	}
	return strategytypes.QuoteInput{
		RequestID:         req.RequestID,
		QuoteID:           req.QuoteID,
		ChainID:           chainID,
		Executor:          executor,
		TokenIn:           req.TokenIn,
		TokenOut:          req.TokenOut,
		AmountIn:          cloneBig(req.Amount),
		RequiredAmountOut: cloneBig(required),
		Candidates:        candidates,
		Now:               now,
	}
}

func decideQuote(
	ctx context.Context,
	input strategytypes.QuoteInput,
	configured strategytypes.Strategy,
	reader quoteReplayReader,
) (*strategyRecord, error) {
	if configured == nil {
		return nil, errors.New("strategy is not configured")
	}
	out, err := configured.DecideQuote(ctx, input)
	if err != nil {
		return nil, err
	}
	tokenInDecimals := 0
	if out.Decision == strategytypes.DecisionQuote {
		tokenInDecimals, err = reader.tokenDecimals(ctx, input.TokenIn)
		if err != nil {
			return nil, errors.Errorf("tokenIn decimals: %w", err)
		}
	}
	return validateQuoteOutput(ctx, input, out, tokenInDecimals, reader)
}

func validateQuoteOutput(
	ctx context.Context,
	input strategytypes.QuoteInput,
	out strategytypes.QuoteOutput,
	tokenInDecimals int,
	reader livePriceReader,
) (*strategyRecord, error) {
	switch out.Decision {
	case strategytypes.DecisionDecline:
		if len(out.Legs) != 0 || out.QuotedAmountOut != nil {
			return nil, errors.New("decline output must not include quote data")
		}
		return nil, nil
	case strategytypes.DecisionQuote:
	default:
		return nil, errors.Errorf("invalid decision %q", out.Decision)
	}
	if len(out.Legs) == 0 {
		return nil, errors.New("quote output has no legs")
	}
	if out.QuotedAmountOut == nil || out.QuotedAmountOut.Sign() <= 0 {
		return nil, errors.New("quote output has invalid quotedAmountOut")
	}

	candidates := make(map[string]strategytypes.QuoteCandidate, len(input.Candidates))
	for _, c := range input.Candidates {
		if c.ID == "" {
			return nil, errors.New("candidate id is empty")
		}
		if _, ok := candidates[c.ID]; ok {
			return nil, errors.Errorf("duplicate candidate id %q", c.ID)
		}
		candidates[c.ID] = c
	}

	sumIn := new(big.Int)
	sumOut := new(big.Int)
	seen := make(map[string]bool, len(out.Legs))
	legs := make([]strategyLeg, 0, len(out.Legs))
	var liveRequests []amountOutRequest
	var liveLegs []int
	for i, leg := range out.Legs {
		if seen[leg.CandidateID] {
			return nil, errors.Errorf("duplicate candidate %q", leg.CandidateID)
		}
		seen[leg.CandidateID] = true
		c, ok := candidates[leg.CandidateID]
		if !ok {
			return nil, errors.Errorf("unknown candidate %q", leg.CandidateID)
		}
		if c.Asset != input.TokenOut {
			return nil, errors.Errorf("candidate %q asset does not match tokenOut", leg.CandidateID)
		}
		if leg.AmountIn == nil || leg.AmountIn.Sign() <= 0 {
			return nil, errors.Errorf("leg %d has invalid amountIn", i)
		}
		if leg.AmountOut == nil || leg.AmountOut.Sign() <= 0 {
			return nil, errors.Errorf("leg %d has invalid amountOut", i)
		}
		if c.MaxAssets == nil || c.MaxAssets.Sign() <= 0 {
			return nil, errors.Errorf("candidate %q has invalid maxAssets", leg.CandidateID)
		}
		if leg.AmountOut.Cmp(c.MaxAssets) > 0 {
			return nil, errors.Errorf("leg %d exceeds candidate maxAssets", i)
		}
		if c.MaxRate == nil || c.MaxRate.Sign() <= 0 {
			return nil, errors.Errorf("candidate %q has invalid maxRate", leg.CandidateID)
		}
		maxAmountOut := liquidlanemath.AmountOutForRate(leg.AmountIn, c.MaxRate, tokenInDecimals, c.AssetDecimals)
		if leg.AmountOut.Cmp(maxAmountOut) > 0 {
			return nil, errors.Errorf("leg %d exceeds candidate maxRate", i)
		}
		if c.DiscountID == nil {
			liveRequests = append(liveRequests, amountOutRequest{
				Adapter:  c.Adapter,
				AmountIn: cloneBig(leg.AmountIn),
			})
			liveLegs = append(liveLegs, i)
		}
		sumIn.Add(sumIn, leg.AmountIn)
		sumOut.Add(sumOut, leg.AmountOut)
		legs = append(legs, strategyLeg{
			Adapter:    c.Adapter,
			AmountIn:   cloneBig(leg.AmountIn),
			AmountOut:  cloneBig(leg.AmountOut),
			MaxRate:    cloneBig(c.MaxRate),
			DiscountID: cloneHash(c.DiscountID),
		})
	}
	if sumIn.Cmp(input.AmountIn) != 0 {
		return nil, errors.Errorf("strategy amountIn sum %s does not match request %s", sumIn, input.AmountIn)
	}
	if sumOut.Cmp(out.QuotedAmountOut) != 0 {
		return nil, errors.Errorf("strategy amountOut sum %s does not match quotedAmountOut %s", sumOut, out.QuotedAmountOut)
	}
	if input.RequiredAmountOut != nil && out.QuotedAmountOut.Cmp(input.RequiredAmountOut) < 0 {
		return nil, errors.New("strategy output is below required amount out")
	}
	if err := validateLivePrices(ctx, input.TokenIn, out.Legs, liveLegs, liveRequests, reader); err != nil {
		return nil, err
	}
	return &strategyRecord{
		QuoteID:         input.QuoteID,
		RequestID:       input.RequestID,
		TokenIn:         input.TokenIn,
		TokenOut:        input.TokenOut,
		AmountIn:        cloneBig(input.AmountIn),
		QuotedAmountOut: cloneBig(out.QuotedAmountOut),
		Legs:            legs,
		CreatedAt:       input.Now,
		UpdatedAt:       input.Now,
	}, nil
}

type livePriceReader interface {
	amountsOut(ctx context.Context, tokenIn common.Address, requests []amountOutRequest) ([]*big.Int, error)
}

func validateLivePrices(
	ctx context.Context,
	tokenIn common.Address,
	legs []strategytypes.QuoteLeg,
	liveLegs []int,
	requests []amountOutRequest,
	reader livePriceReader,
) error {
	if len(requests) == 0 {
		return nil
	}
	if reader == nil {
		return errors.New("live pricing reader is required")
	}
	amounts, err := reader.amountsOut(ctx, tokenIn, requests)
	if err != nil {
		return errors.Errorf("live amountOut replay: %w", err)
	}
	if len(amounts) != len(requests) {
		return errors.Errorf("live amountOut replay: got %d results for %d requests", len(amounts), len(requests))
	}
	for i, live := range amounts {
		if live == nil || live.Sign() <= 0 {
			return errors.Errorf("leg %d has no live amountOut", liveLegs[i])
		}
		if legs[liveLegs[i]].AmountOut.Cmp(live) > 0 {
			return errors.Errorf("leg %d exceeds live amountOut", liveLegs[i])
		}
	}
	return nil
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
