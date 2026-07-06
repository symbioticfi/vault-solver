package defaultstrategy

import (
	"context"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlanemath"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategyregistry"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
)

const Name = "default"
const fillPlanTTL = 3 * time.Hour

type Config struct{}

type Strategy struct {
	pricing strategytypes.Pricing
	now     func() time.Time

	mu    sync.Mutex
	plans map[string]cachedFillPlan
}

type cachedFillPlan struct {
	plan      *strategytypes.FillPlan
	createdAt time.Time
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategyregistry.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, deps strategyregistry.Deps) (strategytypes.Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	return New(NewChainReader(deps.Chain, deps.Log)), nil
}

func New(pricing strategytypes.Pricing) *Strategy {
	return &Strategy{pricing: pricing, now: time.Now, plans: make(map[string]cachedFillPlan)}
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
	plan, err := s.fillPlanFromQuote(input, out, tokenInDecimals)
	if err != nil {
		return strategytypes.QuoteOutput{}, err
	}
	s.remember(input.QuoteID, plan)
	return out, nil
}

func (s *Strategy) BuildFillPlan(ctx context.Context, input strategytypes.FillInput) (*strategytypes.FillPlan, error) {
	if plan := s.cached(input); plan != nil {
		return plan, nil
	}
	quoteInput := strategytypes.QuoteInput(input)
	quoteInput.AmountIn = cloneBig(input.AmountIn)
	quoteInput.RequiredAmountOut = cloneBig(input.RequiredAmountOut)
	out, err := s.DecideQuote(ctx, quoteInput)
	if err != nil || out.Decision == strategytypes.DecisionDecline {
		return nil, err
	}
	plan := s.cached(input)
	if plan == nil {
		return nil, errors.New("rebuilt fill plan was not cached")
	}
	return plan, nil
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

func (s *Strategy) fillPlanFromQuote(
	input strategytypes.QuoteInput,
	out strategytypes.QuoteOutput,
	tokenInDecimals int,
) (*strategytypes.FillPlan, error) {
	if out.Decision != strategytypes.DecisionQuote {
		return nil, errors.Errorf("invalid fill-plan decision %q", out.Decision)
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
	legs := make([]strategytypes.FillLeg, 0, len(out.Legs))
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
		sumIn.Add(sumIn, leg.AmountIn)
		sumOut.Add(sumOut, leg.AmountOut)
		legs = append(legs, strategytypes.FillLeg{
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
	return &strategytypes.FillPlan{
		QuoteID:         input.QuoteID,
		RequestID:       input.RequestID,
		TokenIn:         input.TokenIn,
		TokenOut:        input.TokenOut,
		AmountIn:        cloneBig(input.AmountIn),
		QuotedAmountOut: cloneBig(out.QuotedAmountOut),
		Legs:            legs,
	}, nil
}

func (s *Strategy) remember(quoteID string, plan *strategytypes.FillPlan) {
	if quoteID == "" || plan == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, cached := range s.plans {
		if now.Sub(cached.createdAt) > fillPlanTTL {
			delete(s.plans, id)
		}
	}
	s.plans[quoteID] = cachedFillPlan{plan: clonePlan(plan), createdAt: now}
}

func (s *Strategy) cached(input strategytypes.FillInput) *strategytypes.FillPlan {
	s.mu.Lock()
	cached, ok := s.plans[input.QuoteID]
	s.mu.Unlock()
	if !ok || s.now().Sub(cached.createdAt) > fillPlanTTL {
		return nil
	}
	plan := clonePlan(cached.plan)
	if err := validateCachedPlan(input, plan); err != nil {
		return nil
	}
	return plan
}

func validateCachedPlan(input strategytypes.FillInput, plan *strategytypes.FillPlan) error {
	if plan == nil {
		return errors.New("cached fill plan is nil")
	}
	if plan.TokenIn != input.TokenIn || plan.TokenOut != input.TokenOut {
		return errors.New("cached fill plan token mismatch")
	}
	if plan.AmountIn == nil || input.AmountIn == nil || plan.AmountIn.Cmp(input.AmountIn) != 0 {
		return errors.New("cached fill plan amountIn mismatch")
	}
	if input.RequiredAmountOut != nil &&
		(plan.QuotedAmountOut == nil || plan.QuotedAmountOut.Cmp(input.RequiredAmountOut) < 0) {
		return errors.New("cached fill plan output is below required amount out")
	}
	return nil
}

func clonePlan(in *strategytypes.FillPlan) *strategytypes.FillPlan {
	if in == nil {
		return nil
	}
	out := *in
	out.AmountIn = cloneBig(in.AmountIn)
	out.QuotedAmountOut = cloneBig(in.QuotedAmountOut)
	out.Legs = make([]strategytypes.FillLeg, len(in.Legs))
	for i, leg := range in.Legs {
		out.Legs[i] = strategytypes.FillLeg{
			Adapter:    leg.Adapter,
			AmountIn:   cloneBig(leg.AmountIn),
			AmountOut:  cloneBig(leg.AmountOut),
			MaxRate:    cloneBig(leg.MaxRate),
			DiscountID: cloneHash(leg.DiscountID),
		}
	}
	return &out
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
