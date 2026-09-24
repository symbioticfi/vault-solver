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
	"github.com/symbioticfi/vault-solver/internal/observability/metricstest"
	"github.com/symbioticfi/vault-solver/internal/observability/tracetest"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"go.opentelemetry.io/otel/codes"
)

func TestSingleSourceConfiguration(t *testing.T) {
	raw := strings.Replace(validUniswapXConfig, "strategy: {}\n", "strategy:\n  name: single\n", 1)
	for _, test := range []struct {
		config string
		limit  int
		ttl    time.Duration
	}{
		{"", 4096, 10 * time.Minute},
		{"  maxSelections: 8\n  selectionTtl: 1m\n", 8, time.Minute},
		{"  maxSelections: -1\n", 0, 0},
		{"  selectionTtl: -1s\n", 0, 0},
	} {
		cfg, err := parseConfig(uniswapXConfigNode(t, strings.Replace(raw, "quoteServer:\n", "quoteServer:\n"+test.config, 1)))
		if (err != nil) != (test.limit == 0) {
			t.Fatalf("config %q: err=%v", test.config, err)
		}
		if err == nil && (!cfg.singleSource() || cfg.QuoteServer.MaxSelections != test.limit || cfg.QuoteServer.SelectionTTL != test.ttl) {
			t.Fatalf("config %q: single config = %+v", test.config, cfg)
		}
	}
}

func TestFillPreflightFailuresRetainRetryClassification(t *testing.T) {
	for _, strategyName := range []string{"default", "single"} {
		for _, failure := range []error{errors.New("execution reverted"), errors.New("RPC unavailable")} {
			t.Run(strategyName+"/"+failure.Error(), func(t *testing.T) {
				fixture := newDirectExecutionFixture(t)
				useExecutionStrategy(t, fixture, strategyName)
				state := &quoteState{expiresAt: fixture.now.Add(time.Minute)}
				fixture.solver.quoteState.Store(state)
				fixture.solver.chain = contractCallerFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) { return nil, failure })
				_, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
				if fixture.solver.quoteState.Load() != state {
					t.Fatal("unsubmitted failure invalidated unspent quote inventory")
				}
				if !errors.Is(err, errFillPreflight) || !errors.Is(err, failure) || len(fixture.txm.reqs) != 0 || fixture.solver.capacity.Len() != 0 {
					t.Fatalf("preflight failure must retain retry/breaker classification without submitting: %v", err)
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

func TestFillLoopRetriesDeclines(t *testing.T) {
	for _, strategyName := range []string{"default", "single"} {
		for _, source := range []orderSource{orderSourceExclusiveV2, orderSourcePublicV2} {
			t.Run(strategyName+"/"+string(source), func(t *testing.T) {
				rec := tracetest.Install(t)
				fixture := newDirectExecutionFixture(t)
				fixture.order.Source = source
				useExecutionStrategy(t, fixture, strategyName)
				fixture.solver.reader.(*executionTestReader).snapshot.Direct[0].MaxAmountOut = big.NewInt(89)
				metrics, reg := newUniswapXTestMetricsWithRegistry(t, fixture.solver)
				fixture.solver.metrics = metrics
				if !fixture.solver.claim(fixture.order.Hash, fixture.now) {
					t.Fatal("initial claim failed")
				}
				state := &quoteState{expiresAt: fixture.now.Add(time.Minute)}
				fixture.solver.quoteState.Store(state)
				orders := make(chan *resolvedOrder, 1)
				orders <- fixture.order
				close(orders)
				if err := fixture.solver.fillLoop(t.Context(), []liquidlane.Route{fixture.route}, orders); err != nil {
					t.Fatal(err)
				}
				fill := tracetest.Ended(t, rec, "uniswapx.fill")
				if fill.Status().Code == codes.Error || tracetest.HasEvent(fill, "exception") || !tracetest.HasEvent(fill, "declined") {
					t.Fatalf("decline trace: status=%v events=%v", fill.Status(), fill.Events())
				}
				if fixture.solver.quoteState.Load() != state {
					t.Fatal("decline without a reservation invalidated quote inventory")
				}
				if !fixture.solver.claim(fixture.order.Hash, fixture.now.Add(2*time.Second)) {
					t.Fatal("declined order must remain retryable")
				}
				fixture.solver.endFillPlanning()
				for outcome, want := range map[string]float64{fillOutcomeDeclined: 1, liquidlane.FillOutcomeFailure: 0} {
					metricstest.RequireFamilyValue(t, reg, "solver_bot_workflow_events_total",
						map[string]string{"solver": Name, "event": "fill", "outcome": outcome}, want)
				}
				if source == orderSourceExclusiveV2 {
					if _, tracked := fixture.solver.exclusiveUntil[fixture.order.Hash]; !tracked {
						t.Fatal("decline lost exclusive obligation")
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
				reserved := fixture.solver.capacity.Snapshot()
				expected := fixture.route.CapacityID
				if test.expired || calls > 1 {
					expected = second.CapacityID
				}
				if len(reserved) != 1 || reserved[expected] == nil {
					t.Errorf("preflight must hold only selected source %s, got %v", expected, reserved)
				}
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
	for _, reason := range []string{"source disappeared", "payout exceeds reservation"} {
		t.Run(reason, func(t *testing.T) {
			fixture := newDirectExecutionFixture(t)
			useExecutionStrategy(t, fixture, "single")
			fixture.solver.cfg.SolverMode = solverModeInternal
			// No configured direct adapters: this fixture exercises discount-only recovery.
			fixture.solver.cfg.Adapters = nil
			fixture.solver.cfg.Discounts = &DiscountConfig{HTTPTimeout: time.Second, MinimumValidity: time.Second}
			reader := fixture.solver.reader.(*executionTestReader)
			physical := reader.snapshot.Direct[0]
			physical.GrossAmountOut, physical.MinDiscount = big.NewInt(100), new(big.Int)
			physical.MaxAssets = big.NewInt(200)
			physical.AdapterMinDiscount = new(big.Int)
			reader.snapshot.Physical = []liquidlane.FillQuote{physical}
			fixture.solver.capacity.Set("other-fill", liquidlane.CapacityReservations{fixture.route.CapacityID: big.NewInt(30)})
			first := testDiscountOffer(fixture.route, fixture.now.Add(time.Minute), "130", "1000000000000000000")
			second := first
			second.DiscountID = common.HexToHash("0x02").Hex()
			failedID := first.DiscountID
			if reason == "payout exceeds reservation" {
				failedID = ""
				first.MaxRate = "900000000000000000"
			}
			provider := &replacingDiscountProvider{failedID: failedID, fakeDiscountProvider: &fakeDiscountProvider{
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
			for attempt, capacity := range []int64{200, 130, 129} {
				reader.snapshot.Physical[0].MaxAssets = big.NewInt(capacity)
				provider.resolvedIDs = nil
				// Replanning replaces this order's own reservation, excluding it from available-budget deductions.
				fixture.solver.capacity.Set(fixture.order.Hash.Hex(), liquidlane.CapacityReservations{fixture.route.CapacityID: big.NewInt(100)})
				pending, err := fixture.solver.startFill(t.Context(), []liquidlane.Route{fixture.route}, fixture.order, fixture.now, fixture.now)
				if capacity == 129 {
					if err == nil || pending != nil || len(fixture.txm.reqs) != 2 || fixture.solver.capacity.Snapshot()[fixture.route.CapacityID].Int64() != 30 {
						t.Fatalf("retry exceeded fresh capacity or lost another reservation: fill=%v err=%v", pending, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				defer pending.endFill(nil)
				if len(provider.resolvedIDs) != 2 || provider.resolvedIDs[0] != first.DiscountID || provider.resolvedIDs[1] != second.DiscountID {
					t.Fatalf("resolved discounts = %v", provider.resolvedIDs)
				}
				if len(fixture.packed.Routes) != 0 || len(fixture.packed.DiscountRoutes) != 1 || len(fixture.txm.reqs) != attempt+1 {
					t.Fatalf("calldata = %+v, submissions = %d", fixture.packed, len(fixture.txm.reqs))
				}
				if held := fixture.solver.capacity.Snapshot()[fixture.route.CapacityID]; held.Int64() != 130 {
					t.Fatalf("fallback changed another fill's reservation: %s", held)
				}
				if fixture.packed.DiscountRoutes[0].AmountIn.Cmp(fixture.order.AmountIn) != 0 {
					t.Fatal("replacement changed the signed input")
				}
				fixture.solver.completePendingFill(t.Context(), pending, txmanager.Result{NotAdmitted: true, Err: errors.New("sender rejected admission")})
				if held := fixture.solver.capacity.Snapshot()[fixture.route.CapacityID]; held.Int64() != 30 {
					t.Fatalf("unsent retry retained its reservation or released another fill: %s", held)
				}
			}
		})
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

func TestSelectionRetentionAndMatching(t *testing.T) {
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
		fixture.solver.rememberSelection(quoteResponse{QuoteID: step.id, selectedCandidate: "candidate",
			TokenIn: fixture.order.TokenIn.Hex(), TokenOut: fixture.order.TokenOut.Hex(), AmountIn: "100"}, fixture.now.Add(step.at))
		if len(fixture.solver.selections) != len(step.wantIDs) {
			t.Fatalf("after %s: %v", step.id, fixture.solver.selections)
		}
		for _, id := range step.wantIDs {
			if _, ok := fixture.solver.selections[id]; !ok {
				t.Fatalf("missing preference %s", id)
			}
		}
	}
	fixture.order.QuoteID = "last"
	for _, test := range []struct {
		input int64
		at    time.Duration
		want  liquidlane.CandidateID
	}{
		{100, 5 * time.Second, "candidate"}, {101, 5 * time.Second, ""}, {100, 8 * time.Second, ""},
	} {
		fixture.order.AmountIn = big.NewInt(test.input)
		if got := fixture.solver.preferredSource(fixture.order, fixture.now.Add(test.at)); got != test.want {
			t.Fatalf("input=%d age=%s source=%s, want %s", test.input, test.at, got, test.want)
		}
	}
}
