package iris

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
)

// fakeChain answers the BLM requirement, the debt-token balance and the Iris allowance.
type fakeChain struct {
	requirement, balance, allowance *big.Int
	head                            *types.Header
}

func (c *fakeChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	switch {
	case bytes.HasPrefix(msg.Data, blmBinding.GetABI().Methods["bondRequirement"].ID):
		return common.BigToHash(c.requirement).Bytes(), nil
	case bytes.HasPrefix(msg.Data, erc20Binding.PackBalanceOf(common.Address{})[:4]):
		return common.BigToHash(c.balance).Bytes(), nil
	case bytes.HasPrefix(msg.Data, erc20Binding.PackAllowance(common.Address{}, common.Address{})[:4]):
		return common.BigToHash(c.allowance).Bytes(), nil
	}
	return nil, errors.New("unexpected call")
}

func (c *fakeChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return c.head, nil
}
func (*fakeChain) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func indicativeSolver(t *testing.T, chain *fakeChain) (*Solver, request) {
	t.Helper()
	f := loadFixture(t)
	var markets []Market
	for i := range f.Offers {
		prepared, tree := fixtureMarket(t, f, i)
		m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
		must(t, err)
		markets = append(markets, m)
	}
	s := &Solver{cfg: Config{Pricing: Pricing{Default: big.NewInt(6e16)}}, chain: chain, chainID: big.NewInt(1)}
	index := newMarketIndex(markets)
	s.markets.Store(&index)
	// Aave (venue 0): 20k cap, 30 days, 4% floor. Morpho (venue 1): 5k cap, 7 days, 6.5% floor.
	r := request{
		RequestID: "r", CollateralToken: fixtureWstETH, DebtToken: fixtureUSDC, Collateral: big.NewInt(1e18),
		Debt: big.NewInt(4_000e6), MaxFixedRate: big.NewInt(8e16), Duration: big.NewInt(7 * 86400), VenueBitmap: big.NewInt(0b11),
	}
	return s, r
}

func TestIndicativeQuoteUsesEachMarketsLimits(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*request)
		venue, rate string
	}{
		{"cheapest market", func(*request) {}, "0", "60000000000000000"},
		{"rate raised to the floor", func(r *request) { r.VenueBitmap = big.NewInt(0b10) }, "1", "65000000000000000"},
		{"above the other market's cap", func(r *request) { r.Debt = big.NewInt(6_000e6) }, "0", "60000000000000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1_000e6), head: &types.Header{Number: big.NewInt(1)}}
			s, r := indicativeSolver(t, chain)
			tt.mutate(&r)
			response, err := s.quote(context.Background(), r, true)
			must(t, err)
			equal(t, "37500000", response.Bond)
			equal(t, tt.venue, response.VenueID)
			equal(t, tt.rate, response.FixedRate)
			equal(t, fixtureAccount.Hex(), response.Solver)
			ok(t, response.Indicative)
		})
	}
}

func TestIndicativeQuoteDeclines(t *testing.T) {
	tests := []struct {
		name   string
		chain  fakeChain
		mutate func(*Solver, *request)
		reason string
	}{
		{"allowance short", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(37e6)}, nil, "insufficient_bond_allowance"},
		{"allowance held by an open quote", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(60e6)}, func(s *Solver, _ *request) {
			s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(30e6), Deadline: big.NewInt(time.Now().Add(time.Minute).Unix())})
		}, "insufficient_bond_allowance"},
		{"cash held by an open quote", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1e9), balance: big.NewInt(60e6)}, func(s *Solver, _ *request) {
			s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(30e6), Deadline: big.NewInt(time.Now().Add(time.Minute).Unix())})
		}, "insufficient_bond_balance"},
		{"zero requirement", fakeChain{requirement: new(big.Int), allowance: big.NewInt(1e9)}, nil, "zero_bond_requirement"},
		{"bond cap", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1e9)}, func(s *Solver, _ *request) {
			s.cfg.Pricing.MaxBond = big.NewInt(37e6)
		}, "bond_above_limit"},
		{"venue not allowed", fakeChain{}, func(_ *Solver, r *request) {
			r.VenueBitmap = big.NewInt(0b100)
		}, "no_market"},
		{"rate ceiling", fakeChain{}, func(_ *Solver, r *request) {
			r.MaxFixedRate = big.NewInt(5e16)
		}, "rate_above_max_fixed_rate"},
		{"above every market's limits", fakeChain{}, func(_ *Solver, r *request) {
			r.Duration = big.NewInt(31 * 86400)
		}, "above_market_limits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := tt.chain
			chain.head = &types.Header{Number: big.NewInt(1)}
			s, r := indicativeSolver(t, &chain)
			if tt.mutate != nil {
				tt.mutate(s, &r)
			}
			_, err := s.quote(context.Background(), r, true)
			var refused *declined
			ok(t, errors.As(err, &refused), err)
			equal(t, tt.reason, refused.reason)
		})
	}
}

// fakeSender settles the submitted execution after polls polls, or drops it when stall is set.
type fakeSender struct {
	mu        sync.Mutex
	active    bool
	polls     int
	stall     bool
	submitted []transaction.ExecutionRequest
}

func (s *fakeSender) Submit(_ context.Context, req transaction.ExecutionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.submitted = append(s.submitted, req)
	return nil
}

func (s *fakeSender) Poll(context.Context) (*transaction.ExecutionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stall {
		s.active = false
		return nil, nil
	}
	if s.polls--; s.polls > 0 {
		return nil, nil
	}
	s.active = false
	hash := common.HexToHash("0x1")
	return &transaction.ExecutionResult{Hash: hash, Execution: &provider.Execution{TxHash: hash, BlockNumber: big.NewInt(9)}}, nil
}

func (s *fakeSender) Busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func TestExpiredReservationsAreReleased(t *testing.T) {
	s := &Solver{}
	now := time.Now()
	s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(5), Deadline: big.NewInt(now.Add(-time.Second).Unix())})
	s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(7), Deadline: big.NewInt(now.Add(time.Minute).Unix())})
	s.reserve(Quote{DebtToken: common.HexToAddress("0xD7"), Bond: big.NewInt(11), Deadline: big.NewInt(now.Add(time.Minute).Unix())})
	equal(t, big.NewInt(7), s.reserved(fixtureUSDC, now))
	equal(t, 2, len(s.reservations))
}

func TestLaneOwnsOneExecutionUntilItSettles(t *testing.T) {
	sender := &fakeSender{polls: 3}
	l := &lane{sender: sender, poll: time.Millisecond}
	call := recipes.ExecutorCall{Executor: common.HexToAddress("0xE0"), Data: []byte{1, 2, 3, 4}}
	done, err := l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	must(t, err)
	_, err = l.execute(context.Background(), context.Background(), call, time.Now())
	ok(t, errors.Is(err, errLaneBusy), err)
	outcome := <-done
	must(t, outcome.err)
	equal(t, big.NewInt(9), outcome.result.Execution.BlockNumber)
	equal(t, call.Executor, sender.submitted[0].To)
	ok(t, !l.busy.Load())
}

func TestLaneStallsWhenTheSenderDropsTheExecution(t *testing.T) {
	l := &lane{sender: &fakeSender{stall: true}, poll: time.Millisecond}
	call := recipes.ExecutorCall{Executor: common.HexToAddress("0xE0"), Data: []byte{1, 2, 3, 4}}
	done, err := l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	must(t, err)
	ok(t, errors.Is((<-done).err, errLaneStalled))
	_, err = l.execute(context.Background(), context.Background(), call, time.Now().Add(time.Minute))
	ok(t, errors.Is(err, errLaneStalled), err)
}
