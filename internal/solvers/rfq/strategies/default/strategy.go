package defaultstrategy

import (
	"context"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlanemath"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
)

type Config struct{}

type Strategy struct {
	pricing strategytypes.Pricing
}

func NewFromConfig(raw yaml.Node, chainClient *chain.Client, log logr.Logger) (*Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	return New(NewChainReader(chainClient, log)), nil
}

func New(pricing strategytypes.Pricing) *Strategy {
	return &Strategy{pricing: pricing}
}

func decodeConfig(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	return solver.DecodeStrict(node, out)
}

func (s *Strategy) DecideQuote(ctx context.Context, input strategytypes.QuoteInput) (strategytypes.QuoteOutput, error) {
	tokenInDecimals, err := s.pricing.TokenDecimals(ctx, input.TokenIn)
	if err != nil {
		return strategytypes.QuoteOutput{}, errors.Errorf("tokenIn decimals: %w", err)
	}
	candidates := matchingCandidates(input.Candidates, input.TokenOut)
	oracle, err := s.pricing.AmountsOut(ctx, input.TokenIn, candidates, input.AmountIn)
	if err != nil {
		return strategytypes.QuoteOutput{}, err
	}
	out, ok := selectBest(input, candidates, tokenInDecimals, oracle)
	if !ok {
		return strategytypes.QuoteOutput{Decision: strategytypes.DecisionDecline, Reason: "no viable strategy"}, nil
	}
	return out, nil
}

func matchingCandidates(
	candidates []strategytypes.QuoteCandidate,
	tokenOut common.Address,
) []strategytypes.QuoteCandidate {
	out := make([]strategytypes.QuoteCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Asset != tokenOut {
			continue
		}
		out = append(out, c)
	}
	return out
}

// selectBest picks the best single-asset strategy for the request. It is a pure function:
// oracleByAsset supplies adapter.getAmountOut(tokenIn, amount) per candidate asset.
func selectBest(
	input strategytypes.QuoteInput,
	candidates []strategytypes.QuoteCandidate,
	tokenInDecimals int,
	oracleByAsset map[common.Address]*big.Int,
) (strategytypes.QuoteOutput, bool) {
	groups := groupByAsset(candidates)

	keys := make([]common.Address, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Hex() < keys[j].Hex() })

	var best *strategytypes.QuoteOutput
	for _, asset := range keys {
		oracle := oracleByAsset[asset]
		if oracle == nil {
			continue
		}
		cand := evaluateGroup(input, groups[asset], tokenInDecimals, oracle)
		if cand == nil {
			continue
		}
		if best == nil || cand.QuotedAmountOut.Cmp(best.QuotedAmountOut) > 0 {
			candidate := *cand
			best = &candidate
		}
	}
	if best == nil {
		return strategytypes.QuoteOutput{}, false
	}
	return *best, true
}

func groupByAsset(candidates []strategytypes.QuoteCandidate) map[common.Address][]strategytypes.QuoteCandidate {
	groups := make(map[common.Address][]strategytypes.QuoteCandidate)
	for _, c := range candidates {
		groups[c.Asset] = append(groups[c.Asset], c)
	}
	return groups
}

type eligibleLeg struct {
	candidate strategytypes.QuoteCandidate
	rate      *big.Int
}

func evaluateGroup(
	input strategytypes.QuoteInput,
	group []strategytypes.QuoteCandidate,
	tokenInDecimals int,
	oracleAmountOut *big.Int,
) *strategytypes.QuoteOutput {
	asset := group[0].Asset
	assetDecimals := group[0].AssetDecimals
	if asset != input.TokenOut {
		return nil
	}

	privateRate := liquidlanemath.RateForAmountOut(oracleAmountOut, input.AmountIn, tokenInDecimals, assetDecimals)

	eligible := make([]eligibleLeg, 0, len(group))
	for _, c := range group {
		effRate := privateRate
		if c.DiscountID != nil {
			effRate = c.MaxRate
		}
		if effRate.Sign() <= 0 {
			continue
		}
		if c.DiscountID == nil && c.MaxRate.Cmp(effRate) < 0 {
			continue
		}
		eligible = append(eligible, eligibleLeg{candidate: c, rate: effRate})
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		if c := eligible[i].rate.Cmp(eligible[j].rate); c != 0 {
			return c > 0
		}
		if c := eligible[i].candidate.MaxAssets.Cmp(eligible[j].candidate.MaxAssets); c != 0 {
			return c > 0
		}
		return eligible[i].candidate.MaxRate.Cmp(eligible[j].candidate.MaxRate) > 0
	})
	eligible = dedupeByAdapter(eligible)
	if len(eligible) == 0 {
		return nil
	}

	remainingIn := new(big.Int).Set(input.AmountIn)
	quotedAmountOut := new(big.Int)
	var legs []strategytypes.QuoteLeg
	for _, e := range eligible {
		if remainingIn.Sign() == 0 {
			break
		}
		c := e.candidate
		maxAmountIn := liquidlanemath.MaxAmountInForRate(c.MaxAssets, e.rate, tokenInDecimals, c.AssetDecimals)
		if maxAmountIn.Sign() == 0 {
			continue
		}
		saturated := remainingIn.Cmp(maxAmountIn) > 0

		var amountIn, amountOut *big.Int
		if saturated {
			amountIn = liquidlanemath.MinAmountInForAmountOut(c.MaxAssets, e.rate, tokenInDecimals, c.AssetDecimals)
			amountOut = new(big.Int).Set(c.MaxAssets)
		} else {
			amountIn = new(big.Int).Set(remainingIn)
			amountOut = liquidlanemath.AmountOutForRate(amountIn, e.rate, tokenInDecimals, c.AssetDecimals)
		}
		if amountOut.Sign() == 0 {
			continue
		}

		remainingIn.Sub(remainingIn, amountIn)
		quotedAmountOut.Add(quotedAmountOut, amountOut)
		legs = append(legs, strategytypes.QuoteLeg{
			CandidateID: c.ID,
			AmountIn:    amountIn,
			AmountOut:   amountOut,
		})
	}
	if len(legs) == 0 {
		return nil
	}
	if remainingIn.Sign() != 0 {
		return nil
	}

	return &strategytypes.QuoteOutput{
		Decision:        strategytypes.DecisionQuote,
		QuotedAmountOut: quotedAmountOut,
		Legs:            legs,
	}
}

func dedupeByAdapter(legs []eligibleLeg) []eligibleLeg {
	seen := make(map[string]bool, len(legs))
	out := legs[:0]
	for _, e := range legs {
		key := e.candidate.Adapter.Hex() + ":" + e.candidate.Asset.Hex()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}
