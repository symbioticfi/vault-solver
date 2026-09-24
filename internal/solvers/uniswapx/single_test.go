package uniswapx

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquiddiscounts "github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

func TestSingleSourceConfiguration(t *testing.T) {
	raw := strings.Replace(validUniswapXConfig, "strategy: {}\n", "strategy:\n  name: single\n", 1)
	cfg, err := parseConfig(uniswapXConfigNode(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.singleSource() || cfg.QuoteServer.MaxSelections != 4096 || cfg.QuoteServer.SelectionTTL != 10*time.Minute {
		t.Fatalf("single config = %+v", cfg)
	}
}

type singlePreflightRevertError struct{}

func (singlePreflightRevertError) Error() string  { return "execution reverted" }
func (singlePreflightRevertError) ErrorCode() int { return 3 }

func TestNoFillPlanClassifiesRevertAndRPCFailure(t *testing.T) {
	for _, strategyName := range []string{"default", "single"} {
		for _, failure := range []error{singlePreflightRevertError{}, errors.New("RPC unavailable")} {
			t.Run(strategyName+"/"+failure.Error(), func(t *testing.T) {
				fixture := newDirectExecutionFixture(t)
				useExecutionStrategy(t, fixture, strategyName)
				fixture.solver.chain = contractCallerFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) { return nil, failure })
				_, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
				var revertedError singlePreflightRevertError
				reverted := errors.As(failure, &revertedError)
				if errors.Is(err, errNoFillPlan) != reverted || err == nil || len(fixture.txm.reqs) != 0 {
					t.Fatalf("failure = %v, result = %v", failure, err)
				}
			})
		}
	}
}

func useExecutionStrategy(t *testing.T, fixture *directExecutionFixture, name string) {
	t.Helper()
	fixture.solver.cfg.Strategy = StrategyConfig{Name: name}
	strategy, err := newStrategy(fixture.solver.cfg.Strategy)
	if err != nil {
		t.Fatal(err)
	}
	fixture.solver.strategy = strategy
}

func TestSingleQuoteExcludesExpiredAlternativesBeforeAllocating(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	useExecutionStrategy(t, fixture, "single")
	fixture.solver.chainID = 1
	fixture.solver.exclusiveUntil = nil
	item := fixture.solver.reader.(*executionTestReader).snapshot.Direct[0].Inventory
	item.DiscountID = hashPointer(common.HexToHash("0x01"))
	item.ValidUntil = fixture.now.Add(time.Minute)
	expires := fixture.now.Add(30 * time.Second)
	for _, lifetime := range []time.Duration{time.Second, 35 * time.Second} {
		t.Run(lifetime.String(), func(t *testing.T) {
			alternative := item
			alternative.ID = "another-route"
			alternative.ValidUntil = fixture.now.Add(lifetime)
			fixture.solver.quoteState.Store(&quoteState{
				inventory: []liquidlane.Inventory{item, alternative}, maxFeePerGas: new(big.Int),
				chainTime: fixture.now, expiresAt: expires,
			})
			request := validQuoteRequest(item.TokenIn, item.TokenOut)
			request.Amount = "80"
			quote, err := fixture.solver.quote(t.Context(), request)
			if err != nil || quote.AmountOut != "80" {
				t.Fatalf("valid source can cover 80 after the short-lived alternative is excluded: quote=%+v err=%v", quote, err)
			}
		})
	}
}

func TestFillLoopAbandonsUnavailablePlansAcrossStrategiesAndSources(t *testing.T) {
	for _, strategyName := range []string{"default", "single"} {
		for _, source := range []orderSource{orderSourceExclusiveV2, orderSourcePublicV2} {
			t.Run(strategyName+"/"+string(source), func(t *testing.T) {
				fixture := newDirectExecutionFixture(t)
				fixture.order.Source = source
				useExecutionStrategy(t, fixture, strategyName)
				fixture.solver.reader.(*executionTestReader).snapshot.Direct[0].MaxAmountOut = big.NewInt(89)
				if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
					t.Fatal("initial claim failed")
				}
				orders := make(chan *resolvedOrder, 1)
				orders <- fixture.order
				close(orders)
				if err := fixture.solver.fillLoop(t.Context(), []liquidlane.Route{fixture.route}, orders); err != nil {
					t.Fatal(err)
				}
				if fixture.solver.claim(fixture.order.Hash, fixture.now.Add(2*time.Second)) {
					t.Fatal("a declined order must not be retried")
				}
				if source == orderSourceExclusiveV2 {
					if _, tracked := fixture.solver.exclusiveUntil[fixture.order.Hash]; !tracked {
						t.Fatal("abandonment lost exclusive obligation")
					}
				}
				if len(fixture.txm.reqs) != 0 || len(fixture.solver.failureTimes) != 0 || fixture.solver.localBlockUntil.Load() != 0 {
					t.Fatal("economic decline must neither submit a transaction nor open a public preflight-failure breaker")
				}
			})
		}
	}
}

func TestSingleFillHonorsPreferenceAndReplacesFailedSource(t *testing.T) {
	for _, test := range []struct {
		name                                    string
		failPreferred, expired, wantReplacement bool
	}{
		{"at exclusivity deadline", false, false, false},
		{"preferred preflight failed", true, false, true},
		{"after exclusivity deadline", false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			useExecutionStrategy(t, fixture, "single")
			fixture.order.ExclusiveUntil = uint64(fixture.now.Unix())
			if test.expired {
				fixture.order.ExclusiveUntil--
			}
			reader := fixture.solver.reader.(*executionTestReader)
			second := reader.snapshot.Direct[0]
			second.Route = liquidlane.NewRoute(1, common.HexToAddress("0x99"), common.HexToAddress("0x88"), fixture.order.TokenIn, fixture.order.TokenOut, 0, 0)
			second.MaxAssets, second.MaxAmountOut = big.NewInt(200), big.NewInt(200)
			reader.snapshot.Direct = append(reader.snapshot.Direct, second)
			fixture.solver.rememberSelection(quoteResponse{
				QuoteID: fixture.order.QuoteID, TokenIn: fixture.order.TokenIn.Hex(), TokenOut: fixture.order.TokenOut.Hex(),
				AmountIn: "100", selectedCandidate: liquidlane.NewCandidateID(fixture.route, nil),
			}, fixture.now)
			originalChain := fixture.solver.chain
			calls := 0
			fixture.solver.chain = contractCallerFunc(func(ctx context.Context, call ethereum.CallMsg, block *big.Int) ([]byte, error) {
				calls++
				result, err := originalChain.CallContract(ctx, call, block)
				if test.failPreferred && calls == 1 {
					return nil, errors.New("preferred preflight failed")
				}
				return result, err
			})
			pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route, second.Route}, fixture.order, fixture.now, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			defer pending.endFill(nil)
			want := fixture.route.Adapter
			if test.wantReplacement {
				want = second.Adapter
			}
			if len(fixture.packed.Routes) != 1 || fixture.packed.Routes[0].Adapter != want || len(fixture.txm.reqs) != 1 {
				t.Fatalf("routes = %+v, submissions = %d", fixture.packed.Routes, len(fixture.txm.reqs))
			}
		})
	}
}

type replacingDiscountProvider struct {
	*fakeDiscountProvider

	failedID    string
	resolvedIDs []string
}

func (p *replacingDiscountProvider) Resolve(_ context.Context, id string) (*liquiddiscounts.Resolved, error) {
	p.resolvedIDs = append(p.resolvedIDs, id)
	if id == p.failedID {
		return nil, errors.New("selected discount disappeared")
	}
	resolved := *p.resolved
	resolved.DiscountID = id
	return &resolved, nil
}

func TestSingleInternalReplacesDiscountAndEncodesOnlyReplacement(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	useExecutionStrategy(t, fixture, "single")
	fixture.solver.cfg.SolverMode = solverModeInternal
	// No configured direct adapters: this fixture exercises discount-only recovery.
	fixture.solver.cfg.Adapters = nil
	fixture.solver.cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second, MinimumValidity: time.Second}
	reader := fixture.solver.reader.(*executionTestReader)
	physical := reader.snapshot.Direct[0]
	physical.GrossAmountOut, physical.MinDiscount = big.NewInt(100), new(big.Int)
	physical.AdapterMinDiscount = new(big.Int)
	reader.snapshot.Physical = []liquidlane.FillQuote{physical}
	first := testDiscountOffer(fixture.route, fixture.now.Add(time.Minute), "100", "1000000000000000000")
	second := first
	second.DiscountID = common.HexToHash("0x02").Hex()
	provider := &replacingDiscountProvider{failedID: first.DiscountID, fakeDiscountProvider: &fakeDiscountProvider{
		list: &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{first, second}},
		resolved: &liquiddiscounts.Resolved{DiscountID: second.DiscountID,
			Discount: liquiddiscounts.Terms{Adapter: fixture.route.Adapter.Hex(), TokenToRedeem: fixture.route.TokenIn.Hex(),
				Discount: "0", Signer: common.HexToAddress("0x05").Hex(), Protocol: common.HexToAddress("0x06").Hex(),
				Nonce: "1", Deadline: fixture.now.Add(time.Minute).Unix()},
			SignerSignature: "0x01", ProtocolSignature: "0x02", ProtocolDeadline: fixture.now.Add(time.Minute).Unix(),
		},
	}}
	fixture.solver.discounts = provider
	id := common.HexToHash(first.DiscountID)
	fixture.solver.rememberSelection(quoteResponse{QuoteID: fixture.order.QuoteID, TokenIn: fixture.order.TokenIn.Hex(),
		TokenOut: fixture.order.TokenOut.Hex(), AmountIn: "100", selectedCandidate: liquidlane.NewCandidateID(fixture.route, &id)}, fixture.now)
	pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.endFill(nil)
	if len(provider.resolvedIDs) != 2 || provider.resolvedIDs[0] != first.DiscountID || provider.resolvedIDs[1] != second.DiscountID {
		t.Fatalf("resolved discounts = %v", provider.resolvedIDs)
	}
	if len(fixture.packed.Routes) != 0 || len(fixture.packed.DiscountRoutes) != 1 || len(fixture.txm.reqs) != 1 {
		t.Fatalf("calldata = %+v, submissions = %d", fixture.packed, len(fixture.txm.reqs))
	}
}

func TestSingleInternalCanUseDirectLiquidity(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		fixture := newDirectExecutionFixture(t)
		useExecutionStrategy(t, fixture, "single")
		fixture.solver.cfg.SolverMode = solverModeInternal
		fixture.solver.cfg.Adapters = []common.Address{fixture.route.Adapter}
		fixture.solver.cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second}
		provider := &fakeDiscountProvider{list: &liquiddiscounts.List{}}
		if unavailable {
			provider.listErr = errors.New("backend unavailable")
		}
		fixture.solver.discounts = provider
		fixture.solver.reader.(*executionTestReader).snapshot.Direct[0].MaxAmountOut = new(big.Int).Set(fixture.order.AmountOut)
		fill, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
		if err != nil || fill == nil || len(fixture.txm.reqs) != 1 {
			t.Fatalf("internal single must retain configured direct liquidity: %v", err)
		}
		fill.endFill(nil)
	}
}

func TestSingleInternalQuoteSnapshotUsesNormalSourceSet(t *testing.T) {
	for _, available := range []bool{false, true} {
		fixture := newDirectExecutionFixture(t)
		useExecutionStrategy(t, fixture, "single")
		fixture.solver.cfg.SolverMode = solverModeInternal
		fixture.solver.cfg.Adapters = []common.Address{fixture.route.Adapter}
		fixture.solver.cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second}
		fixture.solver.cfg.QuoteServer.QuoteTTL = time.Minute
		item := fixture.solver.reader.(*executionTestReader).snapshot.Direct[0].Inventory
		item.AdapterMinDiscount = new(big.Int)
		fixture.solver.reader = &quoteModeReader{now: fixture.now, snapshot: snapshot{
			Direct: []liquidlane.Inventory{item}, Physical: []liquidlane.Inventory{item},
		}}
		provider := &fakeDiscountProvider{listErr: errors.New("discount API unavailable")}
		if available {
			provider.listErr = nil
			provider.list = &liquiddiscounts.List{Discounts: []liquiddiscounts.ListItem{
				testDiscountOffer(fixture.route, fixture.now.Add(time.Minute), "100", "1000000000000000000"),
			}}
		}
		fixture.solver.discounts = provider
		if err := fixture.solver.refreshQuoteState(t.Context(), []liquidlane.Route{fixture.route}); err != nil {
			t.Fatal(err)
		}
		state := fixture.solver.quoteState.Load()
		if state == nil {
			t.Fatal("no quote state")
		}
		if len(state.inventory) == 0 || state.inventory[0].DiscountID != nil ||
			available && (len(state.inventory) != 2 || state.inventory[1].DiscountID == nil) || !available && len(state.inventory) != 1 {
			t.Fatalf("available=%t, published inventory=%+v", available, state.inventory)
		}
	}
}

func TestSingleSelectionIsBoundedAndOnlyAPreference(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	fixture.solver.cfg.QuoteServer = QuoteServerConfig{MaxSelections: 1, SelectionTTL: time.Second}
	response := quoteResponse{QuoteID: fixture.order.QuoteID, TokenIn: fixture.order.TokenIn.Hex(), TokenOut: fixture.order.TokenOut.Hex(), AmountIn: "100", selectedCandidate: "candidate"}
	fixture.solver.rememberSelection(response, fixture.now)
	if fixture.solver.preferredSource(fixture.order, fixture.now) != "candidate" {
		t.Fatal("selection not retained")
	}
	fixture.order.AmountIn = big.NewInt(101)
	if fixture.solver.preferredSource(fixture.order, fixture.now) != "" {
		t.Fatal("selection reused for a changed amount")
	}
	fixture.order.AmountIn = big.NewInt(100)
	if fixture.solver.preferredSource(fixture.order, fixture.now.Add(time.Second)) != "" {
		t.Fatal("expired selection reused")
	}
	response.QuoteID = "another"
	fixture.solver.rememberSelection(response, fixture.now.Add(time.Millisecond))
	if len(fixture.solver.selections) != 1 || fixture.solver.preferredSource(fixture.order, fixture.now) != "" {
		t.Fatal("cache bound not enforced")
	}
}

func TestSelectionPrunesExpiredAndEvictsOldestPreference(t *testing.T) {
	fixture := newDirectExecutionFixture(t)
	fixture.solver.cfg.QuoteServer = QuoteServerConfig{MaxSelections: 2, SelectionTTL: 3 * time.Second}
	for _, step := range []struct {
		id      string
		at      time.Duration
		wantIDs []string
	}{
		{"expired", 0, []string{"expired"}},
		{"live", 2 * time.Second, []string{"expired", "live"}},
		{"new", 3 * time.Second, []string{"live", "new"}},
		{"live", 4 * time.Second, []string{"live", "new"}},
		{"last", 5 * time.Second, []string{"live", "last"}},
	} {
		fixture.solver.rememberSelection(quoteResponse{QuoteID: step.id}, fixture.now.Add(step.at))
		if len(fixture.solver.selections) != len(step.wantIDs) {
			t.Fatalf("after %s: preferences = %v", step.id, fixture.solver.selections)
		}
		for _, id := range step.wantIDs {
			if _, exists := fixture.solver.selections[id]; !exists {
				t.Fatalf("after %s: missing preference %s", step.id, id)
			}
		}
	}
}
