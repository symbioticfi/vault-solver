package iris

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
)

// fakeChain answers the BLM requirement, the debt-token balance and the Iris allowance, and
// any call from a sender as a sign simulation that fails with simulation, or runs out of gas
// below gas (zero gas is the node's cap). A market whose debt token is short has no cash. It
// counts round trips: batches and head reads.
type fakeChain struct {
	requirement, balance, allowance *big.Int
	short                           common.Address
	simulation                      error
	gas                             uint64
	head                            *types.Header
	trips                           int
}

// batchedChain serves read groups from fakeChain in one round trip, as the RPC batch does.
type batchedChain struct{ *fakeChain }

func (c *batchedChain) BatchRead(_ context.Context, requests []protocolio.ReadRequest) ([]protocolio.ReadResult, error) {
	c.trips++
	results := make([]protocolio.ReadResult, len(requests))
	for i, r := range requests {
		results[i].Data, results[i].Err = c.call(r.Call)
	}
	return results, nil
}

func (c *fakeChain) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.trips++
	return c.call(msg)
}

func (c *fakeChain) call(msg ethereum.CallMsg) ([]byte, error) {
	switch {
	case msg.From != (common.Address{}) && msg.Gas != 0 && msg.Gas < c.gas:
		return nil, errors.New("out of gas")
	case msg.From != (common.Address{}):
		return nil, c.simulation
	case bytes.HasPrefix(msg.Data, blmBinding.GetABI().Methods["bondRequirement"].ID):
		return word(c.requirement), nil
	case *msg.To == c.short && bytes.HasPrefix(msg.Data, erc20Binding.PackBalanceOf(common.Address{})[:4]):
		return common.Hash{}.Bytes(), nil
	case bytes.HasPrefix(msg.Data, erc20Binding.PackBalanceOf(common.Address{})[:4]):
		return word(c.balance), nil
	case bytes.HasPrefix(msg.Data, erc20Binding.PackAllowance(common.Address{}, common.Address{})[:4]):
		return word(c.allowance), nil
	}
	return nil, errors.New("unexpected call")
}

// word encodes n, nil as zero.
func word(n *big.Int) []byte {
	if n == nil {
		return make([]byte, common.HashLength)
	}
	return common.BigToHash(n).Bytes()
}

func (c *fakeChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	c.trips++
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
	s := &Solver{cfg: Config{Pricing: Pricing{Default: big.NewInt(6e16)}, Timing: Timing{QuoteTTL: 2 * time.Minute}}, chain: &batchedChain{chain}, chainID: big.NewInt(1), signer: common.HexToAddress("0x516")}
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
			equal(t, 1, chain.trips, "one round trip")
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
		{"recipe cannot fund the bond", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1e9), simulation: errors.New("execution reverted")}, nil, "recipe_rejected"},
		{"sign above the gas budget", fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1e9), gas: 400_000}, func(s *Solver, _ *request) {
			s.maxGas = 300_000
		}, "recipe_rejected"},
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

func TestExpiredReservationsAreReleased(t *testing.T) {
	s := &Solver{}
	now := time.Now()
	s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(5), Deadline: big.NewInt(now.Add(-time.Second).Unix())})
	s.reserve(Quote{DebtToken: fixtureUSDC, Bond: big.NewInt(7), Deadline: big.NewInt(now.Add(time.Minute).Unix())})
	s.reserve(Quote{DebtToken: common.HexToAddress("0xD7"), Bond: big.NewInt(11), Deadline: big.NewInt(now.Add(time.Minute).Unix())})
	equal(t, big.NewInt(7), s.reserved(fixtureUSDC, now))
	equal(t, 2, len(s.reservations))
}

func TestIndicativeQuoteFallsBackToAPostableMarket(t *testing.T) {
	chain := &fakeChain{requirement: big.NewInt(30e6), allowance: big.NewInt(1e9), balance: big.NewInt(1e9), head: &types.Header{Number: big.NewInt(1)}}
	s, r := indicativeSolver(t, chain)
	markets := (*s.markets.Load())[[2]common.Address{fixtureWstETH, fixtureUSDC}]
	// The cheaper Aave market settles in a token the account holds no cash in.
	short := common.HexToAddress("0xD7")
	markets[0].DebtToken, chain.short = short, short
	index := newMarketIndex(markets)
	s.markets.Store(&index)
	s.reserve(Quote{DebtToken: short, Bond: big.NewInt(1), Deadline: big.NewInt(time.Now().Add(time.Minute).Unix())})
	r.DebtToken = fixtureUSDC
	candidates, err := s.eligibleMarkets(markets, r)
	must(t, err)
	equal(t, 2, len(candidates))
	equal(t, uint64(0), candidates[0].market.VenueID, "aave is cheaper")
	passing, err := s.preflight(context.Background(), candidates, big.NewInt(1), true)
	must(t, err)
	equal(t, 1, len(passing))
	equal(t, uint64(1), passing[0].market.VenueID)

	_, err = s.preflight(context.Background(), candidates[:1], big.NewInt(1), true)
	var refused *declined
	ok(t, errors.As(err, &refused), err)
	equal(t, "insufficient_bond_balance", refused.reason)
}

func TestSignedBlock(t *testing.T) {
	mined := &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}
	tests := []struct {
		name   string
		result transaction.Result
		reason string
	}{
		{"mined", transaction.Result{Receipt: mined}, ""},
		{"reverted", transaction.Result{Receipt: &types.Receipt{Status: types.ReceiptStatusFailed}}, "signature_reverted"},
		{"never admitted", transaction.Result{NotAdmitted: true, Err: context.DeadlineExceeded}, "slot_missed"},
		{"dropped after the window", transaction.Result{Attempts: []common.Hash{{1}}, Err: context.DeadlineExceeded}, "deadline_missed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := signedBlock(tt.result)
			if tt.reason == "" {
				must(t, err)
				equal(t, big.NewInt(9), block)
				return
			}
			var refused *declined
			ok(t, errors.As(err, &refused), err)
			equal(t, tt.reason, refused.reason)
		})
	}
	_, err := signedBlock(transaction.Result{Attempts: []common.Hash{{1}}, Err: transaction.ErrNonceConsumed})
	ok(t, errors.Is(err, transaction.ErrNonceConsumed), err)
}
