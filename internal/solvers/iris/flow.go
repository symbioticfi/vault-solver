package iris

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/accessmanager"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/erc20"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipecapability"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"

	"github.com/symbioticfi/vault-solver/api/bindings/iris/blm"
	"github.com/symbioticfi/vault-solver/internal/chain"
)

var (
	erc20Binding = erc20.NewERC20()
	blmBinding   = blm.NewIrisBlm()
	// erc1271Magic is IERC1271.isValidSignature.selector.
	erc1271Magic = [4]byte{0x16, 0x26, 0xba, 0x7e}
	// reverts names the errors a sign can revert with: the recipe contracts and the BLM.
	reverts = chain.RevertDecoderFor(&recipeexecutor.RecipeExecutorMetaData, &merkleaccount.MerkleAccountMetaData,
		&accessmanager.AccessManagerMetaData, &recipecapability.RecipeCapabilityMetaData, &blm.IrisBlmMetaData)
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

// declineWith declines with the failed check and the values it compared.
func declineWith(reason, format string, args ...any) error {
	return &declined{venue: "solver", reason: reason, cause: errors.Errorf(format, args...)}
}

// signBatch is how many markets' signing transactions are simulated at once.
const signBatch = 5

// candidate is one eligible market with the quote it would sign.
type candidate struct {
	market Market
	quote  Quote
}

// quote prices the request and, for a firm quote, has the account sign it on-chain before
// returning. Every failure declines: nothing is returned that the chain has not confirmed.
func (s *Solver) quote(ctx context.Context, r request, indicative bool) (quoteResponse, error) {
	index := s.markets.Load()
	if index == nil {
		return quoteResponse{}, declineWith("markets_unavailable", "market index has not loaded yet")
	}
	eligible, err := s.eligibleMarkets(index.candidates(r.CollateralToken, r.DebtToken, r.VenueBitmap), r)
	if err != nil {
		return quoteResponse{}, err
	}
	// One firm quote at a time, so each one sees the capacity the previous one reserved.
	if !indicative {
		if !s.firm.TryLock() {
			return quoteResponse{}, declineWith("solver_busy", "another firm quote is being signed")
		}
		defer s.firm.Unlock()
	}
	nonce, err := randomNonce()
	if err != nil {
		return quoteResponse{}, err
	}
	if indicative {
		// One batch at latest: the reads and every candidate's sign simulation.
		deadline := new(big.Int).SetInt64(time.Now().Add(s.cfg.Timing.QuoteTTL).Unix())
		for i := range eligible {
			q := &eligible[i].quote
			q.Deadline, q.Nonce = deadline, nonce
			// The recipe only binds the parties; the account stands in when the request has none.
			if q.Borrower == (common.Address{}) {
				q.Borrower = q.Solver
			}
			if q.Receiver == (common.Address{}) {
				q.Receiver = q.Solver
			}
		}
		passing, err := s.preflight(ctx, eligible, nil, true)
		if err != nil {
			return quoteResponse{}, err
		}
		return newQuoteResponse(r, passing[0].market, passing[0].quote, true), nil
	}
	head, err := s.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return quoteResponse{}, errors.Errorf("read head: %w", err)
	}
	passing, err := s.preflight(ctx, eligible, head.Number, false)
	if err != nil {
		return quoteResponse{}, err
	}
	inclusion, deadline := s.cfg.Timing.schedule(time.Now(), head.Time)
	for i := range passing {
		passing[i].quote.Deadline, passing[i].quote.Nonce = new(big.Int).SetUint64(deadline), nonce
	}
	chosen, call, err := s.prepareCheapest(ctx, passing)
	if err != nil {
		return quoteResponse{}, err
	}
	market, q := chosen.market, chosen.quote
	// A signature mined after its slot dates the deadline too early for the coordinator, so
	// nothing is sent once the slot has started.
	slot := time.Unix(int64(inclusion), 0)
	if now := time.Now(); !now.Before(slot) {
		return quoteResponse{}, declineWith("slot_missed", "inclusion slot %d started before send at %d", slot.Unix(), now.Unix())
	}
	if !s.sender.LaneReady() {
		return quoteResponse{}, declineWith("solver_busy", "transaction lane is occupied")
	}
	// Non-strict: past the last moment the response could still be fresh, the manager drops
	// the request and the signer's next transaction reuses the nonce.
	results, accepted := s.sender.SendAsync(ctx, transaction.Request{
		To: call.Executor, Data: call.Data, Deadline: slot.Add(s.cfg.Timing.DeadlineWindow), Label: "iris_sign", Solver: Name,
	})
	if !accepted {
		return quoteResponse{}, declineWith("deadline_missed", "sign request not accepted: %v", context.Cause(ctx))
	}
	var result transaction.Result
	select {
	case result = <-results:
	case <-ctx.Done():
		// The signature may still land: hold its bond until the deadline.
		s.reserve(q)
		return quoteResponse{}, declineWith("signature_pending", "response budget ended before the sign confirmed: %v", context.Cause(ctx))
	}
	block, err := signedBlock(result)
	if result.Reverted() {
		err = &declined{venue: "solver", reason: "signature_reverted", cause: errors.Errorf("sign tx %s reverted on-chain (%s)",
			result.Receipt.TxHash.Hex(), s.minedRevert(ctx, call, result.Receipt.BlockNumber))}
	}
	if result.Included() && !result.Reverted() || result.Uncertain() {
		s.reserve(q)
	}
	if err != nil {
		return quoteResponse{}, err
	}
	if err := s.confirm(ctx, market, q, block); err != nil {
		return quoteResponse{}, err
	}
	if wait := time.Until(s.cfg.Timing.respondAt(deadline)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return quoteResponse{}, declineWith("deadline_missed", "response budget ended while holding the signed quote: %v", context.Cause(ctx))
		}
	}
	if now := time.Now(); !s.cfg.Timing.fresh(now, deadline) {
		return quoteResponse{}, declineWith("deadline_missed", "quote deadline %d is no longer fresh at %d", deadline, now.Unix())
	}
	return newQuoteResponse(r, market, q, false), nil
}

// signedBlock is the block that mined the signature, or why the quote cannot be returned.
func signedBlock(result transaction.Result) (*big.Int, error) {
	switch {
	case result.Reverted():
		return nil, decline("signature_reverted")
	case result.Included():
		return result.Receipt.BlockNumber, nil
	case result.NotAdmitted && errors.Is(result.Err, context.DeadlineExceeded):
		return nil, &declined{venue: "solver", reason: "slot_missed", cause: result.Err}
	case errors.Is(result.Err, context.DeadlineExceeded):
		return nil, &declined{venue: "solver", reason: "deadline_missed", cause: result.Err}
	}
	return nil, errors.Errorf("send sign: %w", reverts.Explain(result.Err))
}

// prepareCheapest simulates the candidates' signing transactions signBatch at a time, in rate
// order, and returns the cheapest that passes, or the cheapest candidate's failure.
func (s *Solver) prepareCheapest(ctx context.Context, candidates []candidate) (candidate, recipes.ExecutorCall, error) {
	var first error
	for start := 0; start < len(candidates); start += signBatch {
		batch := candidates[start:min(start+signBatch, len(candidates))]
		calls, errs := make([]recipes.ExecutorCall, len(batch)), make([]error, len(batch))
		var wg sync.WaitGroup
		for i, c := range batch {
			wg.Go(func() { calls[i], errs[i] = s.prepareSign(ctx, c.market, c.quote) })
		}
		wg.Wait()
		if i := slices.IndexFunc(errs, func(err error) bool { return err == nil }); i >= 0 {
			return batch[i], calls[i], nil
		}
		if first == nil {
			first = errs[0]
		}
	}
	return candidate{}, recipes.ExecutorCall{}, first
}

// eligibleMarkets returns, cheapest first, the markets whose recipe admits the request at or
// below the borrower's ceiling, with their rates. A configured rate below the curator's floor
// is raised to the floor; ties keep executor order.
func (s *Solver) eligibleMarkets(markets []Market, r request) ([]candidate, error) {
	var eligible []candidate
	var lowestAbove *big.Int
	var limited error
	unpriced := 0
	for _, m := range markets {
		rate := s.cfg.Pricing.rate(m)
		switch {
		case rate == nil:
			unpriced++
			continue
		case r.Debt.Cmp(m.MaxDebt) > 0 || r.Duration.Cmp(m.MaxDuration) > 0:
			limited = declineWith("above_market_limits", "market %s: debt %s (max %s) or duration %s (max %s) exceeds the recipe bound",
				m.DataHash.Hex(), r.Debt, m.MaxDebt, r.Duration, m.MaxDuration)
			continue
		case rate.Cmp(m.MinFixedRate) < 0:
			rate = m.MinFixedRate
		}
		if rate.Cmp(r.MaxFixedRate) > 0 {
			if lowestAbove == nil || rate.Cmp(lowestAbove) < 0 {
				lowestAbove = rate
			}
			continue
		}
		eligible = append(eligible, candidate{market: m, quote: newQuote(r, m, rate)})
	}
	switch {
	case len(eligible) > 0:
		slices.SortStableFunc(eligible, func(a, b candidate) int { return a.quote.FixedRate.Cmp(b.quote.FixedRate) })
		return eligible, nil
	case lowestAbove != nil:
		return nil, &declined{venue: "solver", reason: "rate_above_max_fixed_rate", rate: lowestAbove,
			cause: errors.Errorf("lowest rate %s exceeds maxFixedRate %s", lowestAbove, r.MaxFixedRate)}
	case limited != nil:
		return nil, limited
	}
	return nil, declineWith("no_market", "%d markets match the collateral, debt token and venues; %d have no configured rate",
		len(markets), unpriced)
}

func newQuote(r request, m Market, rate *big.Int) Quote {
	return Quote{
		Borrower: r.Borrower, Solver: m.Account, Receiver: r.Receiver, Blm: m.Blm,
		CollateralToken: m.CollateralToken, DebtToken: m.DebtToken, Collateral: r.Collateral, Debt: r.Debt,
		FixedRate: rate, Duration: r.Duration, OverdueRate: m.OverdueRate, OverduePeriod: m.OverduePeriod,
		Bond: new(big.Int), BondLltv: m.BondLltv, VenueBitmap: r.VenueBitmap,
		VenueID: new(big.Int).SetUint64(m.VenueID), Deadline: new(big.Int), Nonce: new(big.Int), Data: m.Data,
	}
}

// preflight reads every candidate's bond requirement, Iris allowance and cash at block in one
// batch and returns, in rate order, the candidates whose bond the account can post, or the
// cheapest candidate's decline. The bond is padded the way the recipe does. The allowance
// must cover this bond and every outstanding one. The recipe tops cash up only to this bond,
// so with quotes outstanding the cash must already cover them all; without any, the recipe's
// own funding check decides, so simulate adds each candidate's sign execution, run from the
// signer, to the same batch.
func (s *Solver) preflight(ctx context.Context, candidates []candidate, block *big.Int, simulate bool) ([]candidate, error) {
	stride := 3
	if simulate {
		stride++
	}
	requests := make([]protocolio.ReadRequest, 0, stride*len(candidates))
	for _, c := range candidates {
		m := c.market
		requests = append(requests,
			protocolio.ContractRead(m.Blm, blmBinding.PackBondRequirement(c.quote.binding()), block, true),
			protocolio.ContractRead(m.DebtToken, erc20Binding.PackAllowance(m.Account, m.Iris), block, true),
			protocolio.ContractRead(m.DebtToken, erc20Binding.PackBalanceOf(m.Account), block, true),
		)
		if simulate {
			call, err := s.signCall(m, c.quote)
			if err != nil {
				return nil, err
			}
			requests = append(requests, protocolio.ReadRequest{Kind: protocolio.ReadCall, Call: ethereum.CallMsg{From: s.signer, To: &call.Executor, Data: call.Data, Gas: cmp.Or(s.maxGas, params.MaxTxGas)}, Block: block})
		}
	}
	group, err := protocolio.ReadGroup(ctx, s.chain, requests...)
	if err != nil {
		return nil, errors.Errorf("read bond capacity: %w", err)
	}
	now := time.Now()
	var passing []candidate
	var first error
	for i, c := range candidates {
		bond, err := s.postable(c.market, group, stride*i, now)
		if err == nil && simulate {
			if result := group.At(stride*i + 3); result.Err != nil {
				err = &declined{venue: "solver", reason: "recipe_rejected", cause: reverts.Explain(result.Err)}
			}
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		c.quote.Bond = bond
		passing = append(passing, c)
	}
	if len(passing) == 0 {
		return nil, first
	}
	return passing, nil
}

// postable is the candidate's padded bond if the account can post it, from the reads at offset.
func (s *Solver) postable(m Market, group *protocolio.ReadResults, offset int, now time.Time) (*big.Int, error) {
	raw, err := group.At(offset).CallData()
	if err != nil {
		return nil, errors.Errorf("read bond requirement: %w", reverts.Explain(err))
	}
	requirement, err := blmBinding.UnpackBondRequirement(raw)
	if err != nil {
		return nil, errors.Errorf("decode bond requirement: %w", err)
	}
	if requirement.Sign() <= 0 {
		return nil, declineWith("zero_bond_requirement", "BLM %s bondRequirement is %s", m.Blm.Hex(), requirement)
	}
	bond := padBond(requirement)
	if limit := s.cfg.Pricing.MaxBond; limit != nil && bond.Cmp(limit) > 0 {
		return nil, declineWith("bond_above_limit", "padded bond %s exceeds pricing.maxBond %s", bond, limit)
	}
	if raw, err = group.At(offset + 1).CallData(); err != nil {
		return nil, errors.Errorf("read Iris allowance: %w", reverts.Explain(err))
	}
	allowance, err := erc20Binding.UnpackAllowance(raw)
	if err != nil {
		return nil, errors.Errorf("decode Iris allowance: %w", err)
	}
	reserved := s.reserved(m.DebtToken, now)
	need := new(big.Int).Add(reserved, bond)
	if allowance.Cmp(need) < 0 {
		return nil, declineWith("insufficient_bond_allowance", "account %s Iris allowance %s is below bond %s plus reserved %s",
			m.Account.Hex(), allowance, bond, reserved)
	}
	if reserved.Sign() == 0 {
		return bond, nil
	}
	if raw, err = group.At(offset + 2).CallData(); err != nil {
		return nil, errors.Errorf("read bond balance: %w", reverts.Explain(err))
	}
	balance, err := erc20Binding.UnpackBalanceOf(raw)
	if err != nil {
		return nil, errors.Errorf("decode bond balance: %w", err)
	}
	if balance.Cmp(need) < 0 {
		return nil, declineWith("insufficient_bond_balance", "account %s balance %s is below bond %s plus reserved %s",
			m.Account.Hex(), balance, bond, reserved)
	}
	return bond, nil
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

// signCall assembles the quote's sign execution and routes it the way discovery admitted it:
// directly when the signer is the executor's caller, else through that AccessManager. It
// reads nothing; a revoked grant surfaces when the call is simulated.
func (s *Solver) signCall(m Market, q Quote) (recipes.ExecutorCall, error) {
	snapshot := m.prepared.Snapshot
	inner, _, err := recipes.Assemble(m.prepared.Recipe, m.tree, q.inputs(), nil, nil)
	if err != nil {
		return recipes.ExecutorCall{}, errors.Errorf("assemble sign: %w", err)
	}
	if s.signer == snapshot.Caller {
		return recipes.ExecutorCall{Executor: snapshot.Executor, Data: inner}, nil
	}
	data, err := recipes.AccessManagerCalldata(snapshot.Executor, inner)
	if err != nil {
		return recipes.ExecutorCall{}, errors.Errorf("route sign: %w", err)
	}
	return recipes.ExecutorCall{Executor: snapshot.Caller, Data: data}, nil
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
		return recipes.ExecutorCall{}, errors.Errorf("route sign: %w", reverts.Explain(err))
	}
	simulate := func(ctx context.Context, request recipes.PreparedBatchSimulationRequest) error {
		_, err := s.chain.CallContract(ctx, ethereum.CallMsg{From: s.signer, To: &call.Executor, Data: bytes.Clone(call.Data), Gas: request.GasLimit}, request.BlockNumber)
		return err
	}
	err = recipes.ValidatePreparedBatchWithSimulation(ctx, s.chain, []recipes.PreparedRecipe{m.prepared},
		[]recipes.ExecutorCall{{Executor: snapshot.Executor, Data: inner}}, [][]common.Hash{leaves}, snapshot.Caller, simulate, s.maxGas)
	if err != nil {
		return recipes.ExecutorCall{}, &declined{venue: "solver", reason: "recipe_rejected", cause: reverts.Explain(err)}
	}
	return call, nil
}

// confirm checks, at the signing block, that the account accepts the quote digest: the
// solver's Quote and the one the recipe signed are the same.
func (s *Solver) confirm(ctx context.Context, m Market, q Quote, block *big.Int) error {
	raw, err := s.call(ctx, m.Account, accountBinding.PackIsValidSignature(q.Digest(m.Domain), nil), block)
	if err != nil {
		return errors.Errorf("read isValidSignature: %w", reverts.Explain(err))
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

// minedRevert replays a reverted sign to name the revert its receipt lacks.
func (s *Solver) minedRevert(ctx context.Context, call recipes.ExecutorCall, block *big.Int) string {
	replayer, ok := s.chain.(chain.MinedCallReplayer)
	if !ok {
		return "revert reason unavailable: the chain reader cannot replay mined calls"
	}
	return reverts.MinedRevert(ctx, replayer, ethereum.CallMsg{From: s.signer, To: &call.Executor, Data: call.Data}, block)
}
