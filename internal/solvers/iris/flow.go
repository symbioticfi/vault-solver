package iris

import (
	"bytes"
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/symbioticfi/vault-solver/api/bindings/iris/blm"
)

var (
	erc20Binding = erc20.NewERC20()
	blmBinding   = blm.NewIrisBlm()
	// erc1271Magic is IERC1271.isValidSignature.selector.
	erc1271Magic = [4]byte{0x16, 0x26, 0xba, 0x7e}
)

// declined is a quote the solver chooses not to make. Its reason reaches the borrower
// through the coordinator's own 404; anything else is an internal failure.
type declined struct {
	venue, reason string
	rate          *big.Int
	// cause is the underlying failure, kept for traces and debug logs only.
	cause error
}

func (d *declined) Error() string {
	if d.cause != nil {
		return "declined: " + d.reason + ": " + d.cause.Error()
	}
	return "declined: " + d.reason
}

func (d *declined) Unwrap() error { return d.cause }

func decline(reason string) error { return &declined{venue: "solver", reason: reason} }

// quote prices the request and, for a firm quote, has the account sign it on-chain before
// returning. Every failure declines: nothing is returned that the chain has not confirmed.
func (s *Solver) quote(ctx context.Context, r request, indicative bool) (quoteResponse, error) {
	index := s.markets.Load()
	if index == nil {
		return quoteResponse{}, decline("markets_unavailable")
	}
	market, rate, err := s.selectMarket(index.candidates(r.CollateralToken, r.DebtToken, r.VenueBitmap), r)
	if err != nil {
		return quoteResponse{}, err
	}
	// One firm quote at a time, so each one sees the capacity the previous one reserved.
	if !indicative {
		if !s.firm.TryLock() {
			return quoteResponse{}, decline("solver_busy")
		}
		defer s.firm.Unlock()
	}
	q := Quote{
		Borrower: r.Borrower, Solver: market.Account, Receiver: r.Receiver, Blm: market.Blm,
		CollateralToken: market.CollateralToken, DebtToken: market.DebtToken, Collateral: r.Collateral, Debt: r.Debt,
		FixedRate: rate, Duration: r.Duration, OverdueRate: market.OverdueRate, OverduePeriod: market.OverduePeriod,
		Bond: new(big.Int), BondLltv: market.BondLltv, VenueBitmap: r.VenueBitmap,
		VenueID: new(big.Int).SetUint64(market.VenueID), Deadline: new(big.Int), Nonce: new(big.Int), Data: market.Data,
	}
	head, err := s.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return quoteResponse{}, errors.Errorf("read head: %w", err)
	}
	if q.Bond, err = s.bond(ctx, q, head.Number); err != nil {
		return quoteResponse{}, err
	}
	if err := s.checkCapacity(ctx, market, q.Bond, head.Number); err != nil {
		return quoteResponse{}, err
	}
	if indicative {
		return newQuoteResponse(r, market, q, true), nil
	}
	inclusion, deadline := s.cfg.Timing.schedule(time.Now(), head.Time)
	q.Deadline = new(big.Int).SetUint64(deadline)
	if q.Nonce, err = randomNonce(); err != nil {
		return quoteResponse{}, err
	}
	call, err := s.prepareSign(ctx, market, q)
	if err != nil {
		return quoteResponse{}, err
	}
	// A signature mined after its slot dates the deadline too early for the coordinator, so
	// nothing is sent once the slot has started and the sender stops resubmitting then.
	slot := time.Unix(int64(inclusion), 0)
	if !time.Now().Before(slot) {
		return quoteResponse{}, decline("slot_missed")
	}
	done, err := s.lane.execute(ctx, s.background(), call, slot)
	if errors.Is(err, errLaneBusy) {
		return quoteResponse{}, decline("solver_busy")
	}
	if err != nil {
		return quoteResponse{}, errors.Errorf("submit sign: %w", err)
	}
	var outcome settled
	select {
	case outcome = <-done:
	case <-ctx.Done():
		// The signature may still land: hold its bond until the deadline.
		s.reserve(q)
		return quoteResponse{}, decline("signature_pending")
	}
	if outcome.err != nil {
		s.reserve(q)
		return quoteResponse{}, outcome.err
	}
	if outcome.result.Reverted {
		return quoteResponse{}, decline("signature_reverted")
	}
	s.reserve(q)
	if err := s.confirm(ctx, market, q, outcome.result.Execution.BlockNumber); err != nil {
		return quoteResponse{}, err
	}
	if wait := time.Until(s.cfg.Timing.respondAt(deadline)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return quoteResponse{}, decline("deadline_missed")
		}
	}
	if !s.cfg.Timing.fresh(time.Now(), deadline) {
		return quoteResponse{}, decline("deadline_missed")
	}
	return newQuoteResponse(r, market, q, false), nil
}

// selectMarket picks the lowest-rate market whose recipe admits the request, at or below the
// borrower's ceiling. A configured rate below the curator's floor is raised to the floor.
func (s *Solver) selectMarket(candidates []Market, r request) (Market, *big.Int, error) {
	var best Market
	var bestRate, lowestAbove *big.Int
	limited := false
	for _, m := range candidates {
		rate := s.cfg.Pricing.rate(m)
		switch {
		case rate == nil:
			continue
		case r.Debt.Cmp(m.MaxDebt) > 0 || r.Duration.Cmp(m.MaxDuration) > 0:
			limited = true
			continue
		case rate.Cmp(m.MinFixedRate) < 0:
			rate = m.MinFixedRate
		}
		switch {
		case rate.Cmp(r.MaxFixedRate) > 0:
			if lowestAbove == nil || rate.Cmp(lowestAbove) < 0 {
				lowestAbove = rate
			}
		case bestRate == nil || rate.Cmp(bestRate) < 0:
			best, bestRate = m, rate
		}
	}
	switch {
	case bestRate != nil:
		return best, bestRate, nil
	case lowestAbove != nil:
		return Market{}, nil, &declined{venue: "solver", reason: "rate_above_max_fixed_rate", rate: lowestAbove}
	case limited:
		return Market{}, nil, decline("above_market_limits")
	}
	return Market{}, nil, decline("no_market")
}

// bond reads the BLM requirement for the exact quote and pads it the way the recipe does.
func (s *Solver) bond(ctx context.Context, q Quote, block *big.Int) (*big.Int, error) {
	raw, err := s.call(ctx, q.Blm, blmBinding.PackBondRequirement(q.binding()), block)
	if err != nil {
		return nil, errors.Errorf("read bond requirement: %w", err)
	}
	requirement, err := blmBinding.UnpackBondRequirement(raw)
	if err != nil {
		return nil, errors.Errorf("decode bond requirement: %w", err)
	}
	if requirement.Sign() <= 0 {
		return nil, decline("zero_bond_requirement")
	}
	bond := padBond(requirement)
	if limit := s.cfg.Pricing.MaxBond; limit != nil && bond.Cmp(limit) > 0 {
		return nil, decline("bond_above_limit")
	}
	return bond, nil
}

// checkCapacity requires the Iris allowance to cover this bond and every outstanding one. The
// recipe tops cash up only to this bond, so with offers outstanding the cash must already
// cover them all; without any, the recipe's own funding check decides.
func (s *Solver) checkCapacity(ctx context.Context, m Market, bond, block *big.Int) error {
	reserved := s.reserved(m.DebtToken, time.Now())
	need := new(big.Int).Add(reserved, bond)
	raw, err := s.call(ctx, m.DebtToken, erc20Binding.PackAllowance(m.Account, m.Iris), block)
	if err != nil {
		return errors.Errorf("read Iris allowance: %w", err)
	}
	allowance, err := erc20Binding.UnpackAllowance(raw)
	if err != nil {
		return errors.Errorf("decode Iris allowance: %w", err)
	}
	if allowance.Cmp(need) < 0 {
		return decline("insufficient_bond_allowance")
	}
	if reserved.Sign() == 0 {
		return nil
	}
	balance, err := s.balance(ctx, m.DebtToken, m.Account, block)
	if err != nil {
		return err
	}
	if balance.Cmp(need) < 0 {
		return decline("insufficient_bond_balance")
	}
	return nil
}

// reservation is the bond of a signed quote that may still be taken.
type reservation struct {
	token    common.Address
	bond     *big.Int
	deadline uint64
}

// reserve holds the quote's bond until its deadline.
// shortcut: reservations live in memory and end at the deadline, not at take, so a restart
// forgets them (quotes live quoteTtl) and a taken quote stays counted until it expires.
// Track Take events if this caps capacity in practice.
func (s *Solver) reserve(q Quote) {
	s.reservedMu.Lock()
	defer s.reservedMu.Unlock()
	s.reservations = append(s.reservations, reservation{token: q.DebtToken, bond: q.Bond, deadline: q.Deadline.Uint64()})
}

// reserved drops expired reservations and sums the token's remaining bonds.
func (s *Solver) reserved(token common.Address, now time.Time) *big.Int {
	s.reservedMu.Lock()
	defer s.reservedMu.Unlock()
	total := new(big.Int)
	s.reservations = slices.DeleteFunc(s.reservations, func(r reservation) bool { return int64(r.deadline) < now.Unix() })
	for _, r := range s.reservations {
		if r.token == token {
			total.Add(total, r.bond)
		}
	}
	return total
}

// prepareSign assembles the recipe execution and admits it with the SDK: the recipe and
// root are re-authenticated at a fresh block and the exact transaction is simulated from
// the signer, so the recipe's own limits decide whether the terms may be signed.
func (s *Solver) prepareSign(ctx context.Context, m Market, q Quote) (recipes.ExecutorCall, error) {
	snapshot := m.prepared.Snapshot
	inner, leaves, err := recipes.Assemble(m.prepared.Recipe, m.tree, q.inputs(), nil, nil)
	if err != nil {
		return recipes.ExecutorCall{}, errors.Errorf("assemble sign: %w", err)
	}
	call, err := recipes.RouteExecution(ctx, s.chain, snapshot, s.signer, inner)
	if err != nil {
		return recipes.ExecutorCall{}, err
	}
	simulate := func(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
		_, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.signer, To: &call.Executor, Data: bytes.Clone(call.Data), Gas: request.GasLimit}, request.BlockNumber)
		return err
	}
	err = recipes.ValidatePreparedBatchWithSimulation(ctx, s.chain, []recipes.PreparedRecipe{m.prepared},
		[]recipes.ExecutorCall{{Executor: snapshot.Executor, Data: inner}}, [][]common.Hash{leaves}, snapshot.Caller, simulate, s.maxGas)
	if err != nil {
		return recipes.ExecutorCall{}, &declined{venue: "solver", reason: "recipe_rejected", cause: err}
	}
	return call, nil
}

// confirm checks, at the signing block, that the account accepts the quote digest: the
// solver's Quote and the one the recipe signed are the same.
func (s *Solver) confirm(ctx context.Context, m Market, q Quote, block *big.Int) error {
	raw, err := s.call(ctx, m.Account, accountBinding.PackIsValidSignature(q.Digest(m.Domain), nil), block)
	if err != nil {
		return errors.Errorf("read isValidSignature: %w", err)
	}
	magic, err := accountBinding.UnpackIsValidSignature(raw)
	if err != nil {
		return errors.Errorf("decode isValidSignature: %w", err)
	}
	if magic != erc1271Magic {
		return errors.New("signed digest differs from the quote digest")
	}
	return nil
}

func (s *Solver) balance(ctx context.Context, token, account common.Address, block *big.Int) (*big.Int, error) {
	raw, err := s.call(ctx, token, erc20Binding.PackBalanceOf(account), block)
	if err != nil {
		return nil, errors.Errorf("read bond balance: %w", err)
	}
	balance, err := erc20Binding.UnpackBalanceOf(raw)
	if err != nil {
		return nil, errors.Errorf("decode bond balance: %w", err)
	}
	return balance, nil
}

func (s *Solver) call(ctx context.Context, to common.Address, data []byte, block *big.Int) ([]byte, error) {
	return s.chain.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, block)
}

// randomNonce draws a nonzero 128-bit nonce, so unfilled quotes never collide with later ones.
func randomNonce() (*big.Int, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, errors.Errorf("draw nonce: %w", err)
	}
	return new(big.Int).Add(new(big.Int).SetBytes(b[:]), big.NewInt(1)), nil
}
