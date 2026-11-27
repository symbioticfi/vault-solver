// Package rfq fills awarded RFQ orders through caller-bound Merkle recipes.
package rfq

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/filler"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
	"gopkg.in/yaml.v3"
)

const Name = "rfq-merkle"

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

type orderBackend interface {
	discountResolver
	orders(ctx context.Context, filler string) ([]rfqbackend.OrdersResponseOrdersInner, error)
	listDiscounts(ctx context.Context) (*discounts.List, error)
}

// Solver feeds awarded RFQ orders to the shared Merkle fill lane.
type Solver struct {
	*filler.Engine

	backend     orderBackend
	tokenPolicy tokenpolicy.Policy
	// observeOrderPoll reports the same successful refresh that keeps quotes available.
	observeOrderPoll func(time.Time)
	lastAwardRefresh atomic.Pointer[time.Time]
	// Split pricing is owned by the award loop and scoped to its observed head.
	prices splitPrices
	// skipped is the reasonKey of the last skip logged at Info per award and pass. Owned by the award loop
	// goroutine (pollAwards), so it takes no lock; pruned to the open orders of each complete poll.
	skipped map[awardPass]string
	// lastOpen is the previous complete poll's open orders, so a release names the award. Same owner.
	lastOpen map[string]bool
}

type awardPass struct {
	orderID string
	reserve bool
}

func (*Solver) Name() string                    { return Name }
func (*Solver) RequiresTxManager() bool         { return false }
func (s *Solver) RequiresMerkleTxManager() bool { return s.Sends() }
func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	return New(cfg, deps, nil)
}

// New shares the Merkle fill path with the RFQ quote server.
func New(cfg Config, deps solver.Deps, observeOrderPoll func(time.Time)) (*Solver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	engine, err := filler.New(Name, cfg.Config, deps)
	if err != nil {
		return nil, err
	}
	return &Solver{Engine: engine, backend: newBackendClient(cfg.BackendURL), tokenPolicy: cfg.TokenPolicy, observeOrderPoll: observeOrderPoll}, nil
}

func (s *Solver) Run(ctx context.Context) error { return s.Engine.Run(ctx, s.pollAwards) }

// CanQuote requires a healthy execution lane and a recent successful award refresh.
func (s *Solver) CanQuote(now time.Time) bool {
	lastPoll := s.lastAwardRefresh.Load()
	return lastPoll != nil && now.Sub(*lastPoll) < filler.ConfirmationPatience && s.Ready() && !s.Stalled()
}

// planAward uses the same routes for a queued estimate and a simulated fill. An award it cannot
// plan returns an error naming the failed check; it never returns (nil, nil).
func (s *Solver) planAward(ctx context.Context, row rfqbackend.OrdersResponseOrdersInner, listed []discounts.ListItem, observed filler.Observation, reserve bool, price filler.InventoryReader) (*filler.Intent, error) {
	order, signature, err := decodeOrder(row, s.Workflow.Filler)
	if err != nil {
		return nil, err
	}
	token, required, err := orderTerms(order, s.Workflow.Filler)
	if err != nil {
		return nil, err
	}
	if !s.tokenPolicy.Allows(order.Request.TokenIn) {
		return nil, errors.Errorf("token policy disallows input token %s", order.Request.TokenIn.Hex())
	}
	expiry := time.Unix(order.Request.Deadline.Int64(), 0)
	if !expiry.After(observed.Chain) {
		return nil, errors.Errorf("order expired: deadline %d is not after chain time %d", expiry.Unix(), observed.Chain.Unix())
	}
	var routes []filler.Source
	var reasons []string
	for source, err := range s.Sources(listed, order.Request.TokenIn, token) {
		if err != nil {
			return nil, err
		}
		if source.MaxAssets.Sign() > 0 {
			routes = append(routes, source)
		} else {
			reasons = append(reasons, source.Venue.Hex()+": advertises no capacity")
		}
	}
	if len(routes) == 0 && len(reasons) == 0 {
		reasons = append(reasons, s.Unmatched(listed, order.Request.TokenIn, token))
	}
	plan := func(legs []filler.Leg) (*filler.Intent, error) {
		if reserve {
			amounts := make(liquidlane.CapacityReservations, len(legs))
			for _, leg := range legs {
				amounts.Add(liquidlane.NewCapacityID(s.Workflow.ChainID.Int64(), leg.Account, token), leg.AmountOut)
			}
			return nil, s.Spends.ReserveQueued(row.OrderId, connector.Spend{Amounts: amounts, Deadline: expiry.Unix()})
		}
		built, err := buildFill(ctx, s.Workflow, order, signature, legs, s.backend)
		if err != nil {
			return nil, err
		}
		deadline, ok := observed.SubmissionDeadline(expiry, built.ValidUntil)
		if !ok {
			return nil, errors.New(observed.MissedDeadline(expiry, built.ValidUntil))
		}
		return &filler.Intent{
			OrderID: row.OrderId, Data: built.Calldata, Deadline: deadline,
			Spend: s.Spend(token, order.Request.Deadline.Int64(), built.Legs...),
		}, nil
	}
	for _, route := range routes {
		// Keep an executable single-connector fill when one can settle the award.
		route.MaxAssets = s.Unreserved(route, token, row.OrderId)
		if route.MaxAssets.Cmp(required) < 0 {
			reasons = append(reasons, fmt.Sprintf("%s: unreserved %s below required %s", route.Venue.Hex(), route.MaxAssets, required))
			continue
		}
		intent, err := plan([]filler.Leg{{Source: route, AmountOut: required}})
		if reserve || err == nil {
			return intent, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		observability.Log(ctx).V(1).Info("connector route unavailable", "connector", route.Venue.Hex(), "reason", err.Error())
		reasons = append(reasons, route.Venue.Hex()+": "+err.Error())
	}
	summary := strings.Join(reasons, "; ")
	if s.tokenPolicy.RequiresSingleRoute(order.Request.TokenIn) {
		return nil, errors.Errorf("no single connector settles the award and token policy requires a single route for %s [routes: %s]", order.Request.TokenIn.Hex(), summary)
	}
	legs, err := s.Split(ctx, routes, order.Request.AmountIn, required, len(routes), observed, row.OrderId, price)
	if err != nil {
		return nil, errors.Errorf("%w [routes: %s]", err, summary)
	}
	return plan(legs)
}
