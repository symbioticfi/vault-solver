package webhookstrategy

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategyregistry"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategytypes"
	"github.com/symbioticfi/vault-solver/internal/webhook"
)

const Name = "webhook"
const fillPlanTTL = 3 * time.Hour

type Strategy struct {
	client *webhook.Client
	now    func() time.Time

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
	return &Strategy{client: client, now: time.Now, plans: make(map[string]cachedFillPlan)}
}

func (s *Strategy) DecideQuote(ctx context.Context, input strategytypes.QuoteInput) (strategytypes.QuoteOutput, error) {
	var out strategytypes.QuoteOutput
	if err := s.client.PostJSON(ctx, input, &out); err != nil {
		return strategytypes.QuoteOutput{}, err
	}
	if out.Decision == strategytypes.DecisionQuote {
		plan, err := fillPlanFromQuote(input, out)
		if err != nil {
			return strategytypes.QuoteOutput{}, err
		}
		s.remember(input.QuoteID, plan)
	}
	return out, nil
}

func (s *Strategy) BuildFillPlan(ctx context.Context, input strategytypes.FillInput) (*strategytypes.FillPlan, error) {
	if plan := s.cached(input.QuoteID); plan != nil {
		return plan, nil
	}
	quoteInput := strategytypes.QuoteInput(input)
	quoteInput.AmountIn = cloneBig(input.AmountIn)
	quoteInput.RequiredAmountOut = cloneBig(input.RequiredAmountOut)
	out, err := s.DecideQuote(ctx, quoteInput)
	if err != nil || out.Decision == strategytypes.DecisionDecline {
		return nil, err
	}
	plan := s.cached(input.QuoteID)
	if plan == nil {
		return nil, errors.New("rebuilt fill plan was not cached")
	}
	return plan, nil
}

func fillPlanFromQuote(input strategytypes.QuoteInput, out strategytypes.QuoteOutput) (*strategytypes.FillPlan, error) {
	if out.QuotedAmountOut == nil {
		return nil, errors.New("quote output is missing quotedAmountOut")
	}
	candidates := make(map[string]strategytypes.QuoteCandidate, len(input.Candidates))
	for _, c := range input.Candidates {
		candidates[c.ID] = c
	}
	legs := make([]strategytypes.FillLeg, 0, len(out.Legs))
	for _, leg := range out.Legs {
		c, ok := candidates[leg.CandidateID]
		if !ok {
			return nil, errors.Errorf("unknown candidate %q", leg.CandidateID)
		}
		legs = append(legs, strategytypes.FillLeg{
			Adapter:    c.Adapter,
			AmountIn:   cloneBig(leg.AmountIn),
			AmountOut:  cloneBig(leg.AmountOut),
			MaxRate:    cloneBig(c.MaxRate),
			DiscountID: cloneHash(c.DiscountID),
		})
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

func (s *Strategy) cached(quoteID string) *strategytypes.FillPlan {
	s.mu.Lock()
	cached, ok := s.plans[quoteID]
	s.mu.Unlock()
	if !ok || s.now().Sub(cached.createdAt) > fillPlanTTL {
		return nil
	}
	return clonePlan(cached.plan)
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
