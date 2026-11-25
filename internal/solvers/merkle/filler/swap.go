package filler

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

var (
	// ErrNotFillable reports that no configured connector can settle the order right now.
	ErrNotFillable = errors.New("no connector can settle the order")
	// ErrSimulation reports that a priced fill was rejected by the settlement simulation.
	ErrSimulation = errors.New("settlement simulation failed")
	// ErrSubmission reports that the signer lane refused a validated fill before signing it.
	ErrSubmission = errors.New("settlement submission failed")
	// ErrTermsExpiring reports a fill declined only because its signed discount expires before the
	// fill could land; the order itself lives longer, so the next signed terms may settle it. It is
	// always reported together with ErrNotFillable.
	ErrTermsExpiring = errors.New("signed discount expires before the fill can land")
)

// Swap is a protocol order whose settlement contract holds the input during its callback
// and must end the callback holding at least MinAmountOut of TokenOut.
type Swap struct {
	OrderID      string
	TokenIn      common.Address
	AmountIn     *big.Int
	TokenOut     common.Address
	MinAmountOut *big.Int
	// Expiry is the order's fill deadline in chain time.
	Expiry time.Time
	// MaxRoutes bounds the connectors one fill may draw on; below two, one connector settles the
	// whole order.
	MaxRoutes int
}

// Envelope wraps the callback calls into the protocol's complete settlement call.
type Envelope func(calls []execution.Call) ([]byte, error)

// builtSwap is a validated settlement call and the connector swaps it carries.
type builtSwap struct {
	Data []byte
	Legs []connector.Prepared
}

// validUntil is the earliest signed-terms deadline among the legs.
func (b builtSwap) validUntil() time.Time {
	earliest := b.Legs[0].Terms.ValidUntil
	for _, leg := range b.Legs[1:] {
		if leg.Terms.ValidUntil.Before(earliest) {
			earliest = leg.Terms.ValidUntil
		}
	}
	return earliest
}

// buildSwap prices the swap on its legs' connectors and validates the settlement call. Each
// connector prices its own payment for the input it receives, so together they must cover
// MinAmountOut; any surplus stays with the settlement contract. Inside its callback the
// settlement contract forwards each leg's input to its connector and runs the recipe whose
// connector swap pays the contract back.
func (e *Engine) buildSwap(ctx context.Context, swap Swap, legs []Leg, resolve connector.ResolveDiscount, envelope Envelope) (builtSwap, error) {
	requests := make([]connector.Request, len(legs))
	for i, leg := range legs {
		requests[i] = leg.Request(swap.TokenIn, leg.AmountIn, swap.TokenOut, nil)
	}
	prepared, err := e.Workflow.PrepareLegs(ctx, requests, resolve)
	if err != nil {
		return builtSwap{}, err
	}
	paid := new(big.Int)
	calls := make([]execution.Call, 0, 2*len(prepared))
	for i, leg := range prepared {
		paid.Add(paid, leg.AmountOut)
		funding, err := execution.Transfer(swap.TokenIn, requests[i].Venue, requests[i].AmountIn)
		if err != nil {
			return builtSwap{}, err
		}
		calls = append(calls, funding, execution.Call{Target: leg.Call.Executor, Value: new(big.Int), Data: leg.Call.Data})
	}
	if paid.Cmp(swap.MinAmountOut) < 0 {
		return builtSwap{}, errors.Errorf("%w: connectors pay %s below order output %s", ErrNotFillable, paid, swap.MinAmountOut)
	}
	data, err := envelope(calls)
	if err != nil {
		return builtSwap{}, errors.Errorf("encode settlement: %w", err)
	}
	if err := e.Workflow.Validate(ctx, data, prepared...); err != nil {
		return builtSwap{}, errors.Errorf("%w: %w", ErrSimulation, err)
	}
	return builtSwap{Data: data, Legs: prepared}, nil
}

// prepareSwap returns the fill of the first advertised connector that settles the whole swap,
// or, when none does and the swap allows several routes, a fill split across connectors.
// It returns nil for an order this lane already settled. When nothing settles it, the error
// says why: ErrSimulation when a priced fill was rejected by the settlement simulation, the
// underlying failure when a connector could not be read or authenticated, and ErrNotFillable
// when the connectors simply pay too little or lack capacity.
func (e *Engine) prepareSwap(ctx context.Context, swap Swap, listed []discounts.ListItem, resolve connector.ResolveDiscount, envelope Envelope) (*Intent, error) {
	observed, err := e.Observe(ctx)
	if err != nil {
		return nil, err
	}
	if e.Spends.Finalized(swap.OrderID) {
		return nil, nil
	}
	if !swap.Expiry.After(observed.Chain) {
		return nil, errors.Errorf("%w: order expired: expiry %d is not after chain time %d", ErrNotFillable, swap.Expiry.Unix(), observed.Chain.Unix())
	}
	if swap.MinAmountOut == nil || swap.MinAmountOut.Sign() <= 0 {
		return nil, errors.Errorf("%w: order has no output amount", ErrNotFillable)
	}
	var simulation, failure, expiring, declined error
	// reasons lists why each route was passed over, so the final error names every failed check.
	var reasons []string
	note := func(venue, reason string) {
		reasons = append(reasons, venue+": "+reason)
		observability.Log(ctx).V(1).Info("connector route unavailable", "orderId", swap.OrderID, "connector", venue, "reason", reason)
	}
	// miss records why a candidate fill was not built and keeps the most telling class.
	miss := func(venue string, err error) {
		note(venue, err.Error())
		err = errors.Errorf("%s: %w", venue, err)
		switch {
		case errors.Is(err, ErrSimulation):
			simulation = err
		case errors.Is(err, ErrTermsExpiring):
			expiring = err
		case errors.Is(err, ErrNotFillable):
			declined = err
		case errors.Is(err, connector.ErrInsufficientCapacity):
		case failure == nil:
			failure = err
		}
	}
	intentFor := func(built builtSwap) (*Intent, error) {
		validUntil := built.validUntil()
		// When the signed terms, not the order, bound the fill, fresh terms may still settle it.
		class := ErrNotFillable
		if validUntil.Before(swap.Expiry) {
			class = errors.Errorf("%w: %w", ErrNotFillable, ErrTermsExpiring)
		}
		deadline, ok := observed.SubmissionDeadline(swap.Expiry, validUntil)
		if !ok {
			return nil, errors.Errorf("%w: %s", class, observed.MissedDeadline(swap.Expiry, validUntil))
		}
		if left := time.Until(deadline); left < e.MinValidity {
			return nil, errors.Errorf("%w: order or discount expires within %s: submission deadline in %s", class, e.MinValidity, left.Round(time.Millisecond))
		}
		return &Intent{
			OrderID: swap.OrderID, Data: built.Data, Deadline: deadline,
			Spend: e.Spend(swap.TokenOut, swap.Expiry.Unix(), built.Legs...),
		}, nil
	}
	var routes []Source
	for source, err := range e.Sources(listed, swap.TokenIn, swap.TokenOut) {
		if err != nil {
			return nil, err
		}
		if source.MaxAssets.Sign() > 0 {
			routes = append(routes, source) // Split charges the reservations itself.
		}
		// A connector pays at most its funding not yet spent by this lane: one below the order
		// output can only take part in a split.
		single := source
		if single.MaxAssets = e.Unreserved(source, swap.TokenOut, ""); single.MaxAssets.Cmp(swap.MinAmountOut) < 0 {
			note(source.Venue.Hex(), fmt.Sprintf("unreserved %s below order output %s", single.MaxAssets, swap.MinAmountOut))
			continue
		}
		// Keep an executable single-connector fill when one can settle the order.
		built, err := e.buildSwap(ctx, swap, []Leg{{Source: single, AmountIn: swap.AmountIn}}, resolve, envelope)
		if err == nil {
			// A discount about to expire must not hide a later connector whose terms still hold.
			var intent *Intent
			if intent, err = intentFor(built); err == nil {
				return intent, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		miss(source.Venue.Hex(), err)
	}
	if len(reasons) == 0 {
		note("pair", e.Unmatched(listed, swap.TokenIn, swap.TokenOut))
	}
	if swap.MaxRoutes > 1 {
		legs, err := e.Split(ctx, routes, swap.AmountIn, swap.MinAmountOut, swap.MaxRoutes, observed, "", e.Inventory)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil {
			built, buildErr := e.buildSwap(ctx, swap, legs, resolve, envelope)
			if buildErr == nil {
				return intentFor(built)
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			err = buildErr
		}
		miss("split", err)
	} else {
		note("split", fmt.Sprintf("disabled, order allows %d route(s)", swap.MaxRoutes))
	}
	// also appends the routes passed over for a reason other than the one returned.
	also := func(head error) error {
		others := slices.DeleteFunc(reasons, func(reason string) bool { return reason == head.Error() })
		if len(others) == 0 {
			return head
		}
		return errors.Errorf("%w [also: %s]", head, strings.Join(others, "; "))
	}
	switch {
	case simulation != nil:
		return nil, also(simulation)
	case failure != nil:
		return nil, errors.Errorf("prepare connector settlement: %w", also(failure))
	case expiring != nil:
		return nil, also(expiring)
	case declined != nil:
		return nil, also(declined)
	default:
		return nil, errors.Errorf("%w: %s", ErrNotFillable, strings.Join(reasons, "; "))
	}
}

// Swaps is the fill lane of a protocol whose orders are callback-funded swaps priced from the
// advertised connector discounts.
type Swaps struct {
	*Engine

	Discounts discounts.Provider
}

// NewSwaps builds the lane for the settlement contract in cfg.
func NewSwaps(name string, cfg Config, provider discounts.Provider, deps solver.Deps) (*Swaps, error) {
	if provider == nil {
		return nil, errors.Errorf("%s requires the connector discount service", name)
	}
	engine, err := New(name, cfg, deps)
	if err != nil {
		return nil, err
	}
	return &Swaps{Engine: engine, Discounts: provider}, nil
}

// Fill prepares swap and hands it to the signer lane. It returns the intent that owns the lane,
// together with the submission error when ownership is ambiguous, or nil when the order needs
// no fill.
func (s *Swaps) Fill(ctx context.Context, swap Swap, envelope Envelope) (*Intent, error) {
	if s.Pending() != nil {
		return nil, s.errBusy()
	}
	listed, err := s.Discounts.ListDiscounts(ctx)
	if err != nil {
		return nil, err
	}
	if listed == nil {
		listed = &discounts.List{}
	}
	intent, err := s.prepareSwap(ctx, swap, listed.Discounts, s.Discounts.Resolve, envelope)
	if err != nil || intent == nil {
		return nil, err
	}
	if err := s.Submit(ctx, intent); err != nil {
		if s.Pending() == intent {
			// An ambiguous submission keeps the lane: the caller must still await it.
			return intent, err
		}
		return nil, errors.Errorf("%w: %w", ErrSubmission, err)
	}
	return intent, nil
}
