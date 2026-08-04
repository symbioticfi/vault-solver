package rfq

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestRunExternalFailsForUnauthorizedConfiguredAdapter(t *testing.T) {
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	executor := common.HexToAddress("0x0000000000000000000000000000000000000010")
	rdr := &fakeRecoveryReader{authErr: errors.New("adapter is not authorized")}
	var logs []string
	s := &Solver{
		cfg: &Config{
			Executor:   executor,
			SolverMode: solverModeExternal,
			Adapters:   []recoveryVault{{Adapter: adapter}},
		},
		exec: &executionService{reader: rdr},
		log:  funcr.NewJSON(func(entry string) { logs = append(logs, entry) }, funcr.Options{}),
	}

	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "rfq: validate direct authorization: adapter is not authorized") {
		t.Fatalf("Run() error = %v, want direct authorization startup failure", err)
	}
	if rdr.authCalls != 1 {
		t.Fatalf("authorization checks = %d, want 1", rdr.authCalls)
	}
	if rdr.setCalls != 1 {
		t.Fatalf("quote metadata assignments = %d, want 1 before server start", rdr.setCalls)
	}
	logged := strings.Join(logs, "\n")
	if !strings.Contains(logged, "external adapter authorization failed") ||
		!strings.Contains(logged, "adapter is not authorized") ||
		!strings.Contains(logged, executor.Hex()) ||
		!strings.Contains(logged, `"error"`) {
		t.Fatalf("authorization failure was not logged with its reason: %s", logged)
	}
}

// TestBuildServices_WhitelistWiring pins that solver mode actually reaches both services with the correct
// per-path scoping: reverting the factory wiring (leaving a whitelist nil) would silently let a filler
// quote/fill through adapters it isn't scoped to. The quote and execution paths scope independently —
// quoting follows quoteScopesToAdapters() (external + internal-with-adapters), execution follows
// restrictsToAdapters() (external only) — so the two whitelists are asserted separately.
func TestBuildServices_WhitelistWiring(t *testing.T) {
	listed := common.HexToAddress("0x0000000000000000000000000000000000000042")
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cfg := &Config{
		BackendURL:  "https://rfq-backend.example",
		Executor:    common.HexToAddress("0x0000000000000000000000000000000000000010"),
		Adapters:    []recoveryVault{{Adapter: listed}},
		TokenPolicy: testPermissionedPolicy(t, permissionedToken),
	}
	st := newStore(func() time.Time { return time.Unix(0, 0) })

	// scopedToConfigured asserts wl is non-nil and admits exactly the configured adapter.
	scopedToConfigured := func(t *testing.T, name string, wl adapterWhitelist) {
		t.Helper()
		if wl == nil {
			t.Fatalf("%s service: whitelist not wired (nil = fail open)", name)
		}
		if !wl.allows(listed) || wl.allows(rogue) {
			t.Fatalf("%s service: whitelist = %v, want exactly the configured adapters", name, wl)
		}
	}

	// External + configured adapters ⇒ both quote and execution scope to the configured adapters.
	cfg.SolverMode = solverModeExternal
	quotes, exec := buildServices(cfg, 1, st, nil, nil, nil, logr.Discard())
	scopedToConfigured(t, "quote", quotes.whitelist)
	scopedToConfigured(t, "execution", exec.whitelist)
	if !quotes.tokenPolicy.RequiresSingleRoute(permissionedToken) ||
		!exec.tokenPolicy.RequiresSingleRoute(permissionedToken) {
		t.Fatal("token policy was not wired to both quote and execution services")
	}

	// Internal + configured adapters ⇒ the QUOTE path scopes to the configured adapters, but execution
	// stays unrestricted (nil) so discount recovery can fill through any advertised adapter.
	cfg.SolverMode = solverModeInternal
	quotes, exec = buildServices(cfg, 1, st, nil, nil, nil, logr.Discard())
	scopedToConfigured(t, "quote", quotes.whitelist)
	if exec.whitelist != nil {
		t.Fatalf("internal mode: execution whitelist = %v, want nil (filling stays unrestricted)", exec.whitelist)
	}

	// Internal + no adapters ⇒ neither path scopes (both nil): the filler quotes/fills off discounts only.
	cfg.Adapters = nil
	quotes, exec = buildServices(cfg, 1, st, nil, nil, nil, logr.Discard())
	if quotes.whitelist != nil || exec.whitelist != nil {
		t.Fatal("internal mode with no adapters should wire both whitelists nil (filtering off)")
	}
}

// TestBuildServices_InternalModeQuoteScoping is the end-to-end guard for internal-mode quote scoping: a
// filler built (via buildServices, the real wiring path) from an INTERNAL-mode config with one configured
// adapter must drop a request adapter it isn't scoped to at quote time, while still quoting through its
// own configured adapter. This pins the feature through the config→service wiring rather than a hand-set
// whitelist, so reverting quoteScopesToAdapters() (or rewiring the quote whitelist to restrictsToAdapters)
// fails here.
func TestBuildServices_InternalModeQuoteScoping(t *testing.T) {
	clk := func() time.Time { return time.Unix(0, 0) }
	st := newStore(clk)
	cfg := &Config{
		BackendURL: "https://rfq-backend.example",
		Executor:   common.HexToAddress("0x0000000000000000000000000000000000000010"),
		SolverMode: solverModeInternal,
		Adapters:   []recoveryVault{{Adapter: vlt}}, // the only adapter this filler is scoped to
	}

	quotes, _ := buildServices(cfg, 1, st, nil, nil, nil, logr.Discard())
	// buildServices wires real dependencies; swap in test fakes. The default strategy prices the tOut
	// asset-group at 1.000000 USDC.
	quotes.reader = &fakeQuoteCandidateReader{out: map[common.Address]*big.Int{tOut: big.NewInt(1_000000)}}
	quotes.strategy = newDefaultTestStrategy()

	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	rogueAdapter := quoteAdapter{
		Adapter: rogue.Hex(), Asset: tOut.Hex(), AssetDecimals: 6,
		MaxAssets: "10000000", MaxRate: "2000000000000000000", // better rate, but not scoped → must be dropped
	}

	// (1) Request offering ONLY the non-configured adapter ⇒ declined (nil → HTTP 204).
	onlyRogue := validQuoteBody()
	onlyRogue.QuoteID = "33333333-3333-4333-8333-333333333333"
	onlyRogue.Adapters = []quoteAdapter{rogueAdapter}
	resp, err := quotes.quote(t.Context(), &onlyRogue)
	if err != nil {
		t.Fatalf("quote (only non-configured adapter): unexpected error %v", err)
	}
	if resp != nil {
		t.Fatalf("quote (only non-configured adapter): got %+v, want nil (declined: out of adapter scope)", resp)
	}
	// (2) Request offering the configured adapter alongside the rogue one ⇒ quoted through the configured
	// adapter only (the rogue leg, despite a better rate, is filtered out before selection).
	mixed := validQuoteBody() // validQuoteBody's single adapter is vlt (the configured one)
	mixed.Adapters = append(mixed.Adapters, rogueAdapter)
	resp, err = quotes.quote(t.Context(), &mixed)
	if err != nil {
		t.Fatalf("quote (configured + rogue): unexpected error %v", err)
	}
	if resp == nil {
		t.Fatal("quote (configured + rogue): got nil, want a quote through the configured adapter")
	}
	if resp.AmountOut != "1000000" {
		t.Fatalf("amountOut = %s, want quote through the configured adapter", resp.AmountOut)
	}
}

func TestFactoryWiresSwapOnlyWhenEnabledAndSharesBackend(t *testing.T) {
	cfg := &Config{
		BackendURL:   "https://rfq-backend.example",
		Executor:     common.HexToAddress("0x0000000000000000000000000000000000000010"),
		Router:       common.HexToAddress("0x0000000000000000000000000000000000000055"),
		SwapQuoteTTL: 30 * time.Second,
		SolverMode:   solverModeInternal,
		Adapters:     []recoveryVault{{Adapter: vlt}},
	}
	st := newStore(time.Now)
	rdr := &reader{swapState: newFakeSwapState(directSwapCandidate().Route)}
	backend := newBackendClient(cfg.BackendURL)
	signer := newSwapTestSigner(t)

	_, exec, swaps := buildServicesWithSwap(cfg, 1, st, rdr, nil, nil, backend, signer, logr.Discard())
	if swaps != nil {
		t.Fatal("disabled config unexpectedly wired swap service")
	}
	if exec.backend != backend {
		t.Fatal("execution service did not receive the shared backend client")
	}

	cfg.SwapEnabled = true
	quotes, exec, swaps := buildServicesWithSwap(cfg, 1, st, rdr, nil, nil, backend, signer, logr.Discard())
	if swaps == nil {
		t.Fatal("enabled config did not wire swap service")
	}
	if exec.backend != backend || swaps.discountBackend != backend {
		t.Fatal("execution and swap services do not share one backend client")
	}
	if swaps.signer != signer || swaps.reader != rdr || swaps.state != rdr.swapState || swaps.strategy != quotes.strategy {
		t.Fatal("swap service did not receive the framework signer and shared quote dependencies")
	}
	if swaps.router != cfg.Router || swaps.quoteTTL != cfg.SwapQuoteTTL || swaps.store == nil {
		t.Fatalf("swap configuration not wired: %+v", swaps)
	}
	if swaps.whitelist == nil || !swaps.whitelist.allows(vlt) {
		t.Fatal("swap service did not receive quote adapter scope")
	}
}

type startupSwapState struct {
	routerErr    error
	adapterErr   error
	routerCalls  int
	adapterCalls int
	adapters     []common.Address
}

func (s *startupSwapState) validateRouter(context.Context, common.Address) error {
	s.routerCalls++
	return s.routerErr
}

func (s *startupSwapState) validateAdapters(
	_ context.Context, adapters []common.Address, _ common.Address,
) (map[common.Address]swapDomain, error) {
	s.adapterCalls++
	s.adapters = append([]common.Address(nil), adapters...)
	return nil, s.adapterErr
}

func (*startupSwapState) readFillQuote(context.Context, liquidlane.Route, *big.Int) (liquidlane.FillQuote, error) {
	return liquidlane.FillQuote{}, nil
}

func (*startupSwapState) readUsedNonces(context.Context, []swapNonceCheck) ([]bool, error) {
	return nil, nil
}

func TestSolverRunValidatesRouterAndStaticAdaptersBeforeListening(t *testing.T) {
	adapter := common.HexToAddress("0x0000000000000000000000000000000000000042")
	state := &startupSwapState{adapterErr: errors.New("unauthorized signer")}
	signer := newSwapTestSigner(t)
	s := &Solver{
		cfg: &Config{
			Executor:    common.HexToAddress("0x0000000000000000000000000000000000000010"),
			Router:      common.HexToAddress("0x0000000000000000000000000000000000000055"),
			SwapEnabled: true, SolverMode: solverModeExternal,
			Adapters: []recoveryVault{{Adapter: adapter}},
		},
		exec:  &executionService{reader: &fakeRecoveryReader{}},
		swaps: &swapService{state: state, signer: signer},
		log:   logr.Discard(),
	}

	err := s.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "validate swap adapters: unauthorized signer") {
		t.Fatalf("Run() error = %v, want swap adapter startup failure", err)
	}
	if state.routerCalls != 1 || state.adapterCalls != 1 || len(state.adapters) != 1 || state.adapters[0] != adapter {
		t.Fatalf("startup validation calls: router=%d adapters=%d values=%v", state.routerCalls, state.adapterCalls, state.adapters)
	}
}

func TestSolverRunRejectsInvalidRouterBeforeListening(t *testing.T) {
	state := &startupSwapState{routerErr: errors.New("no bytecode")}
	s := &Solver{
		cfg:  &Config{Router: common.HexToAddress("0x0000000000000000000000000000000000000055"), SwapEnabled: true},
		exec: &executionService{reader: &fakeRecoveryReader{}}, swaps: &swapService{state: state}, log: logr.Discard(),
	}
	if err := s.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "validate swap Router: no bytecode") {
		t.Fatalf("Run() error = %v, want Router validation failure", err)
	}
	if state.routerCalls != 1 || state.adapterCalls != 0 {
		t.Fatalf("validation order: router=%d adapters=%d", state.routerCalls, state.adapterCalls)
	}
}

func TestSolverRunAllowsInternalSwapWithoutStaticAdapters(t *testing.T) {
	state := &startupSwapState{}
	signer := newSwapTestSigner(t)
	swaps := &swapService{state: state, signer: signer}
	srv := testServer()
	srv.swaps = swaps
	s := &Solver{
		cfg: &Config{
			ListenAddr: "127.0.0.1:0", Executor: common.HexToAddress("0x0000000000000000000000000000000000000010"),
			Router: common.HexToAddress("0x0000000000000000000000000000000000000055"), SwapEnabled: true,
			SolverMode: solverModeInternal, PollInterval: time.Hour,
		},
		exec: &executionService{
			backend: &fakeBackend{}, store: newStore(time.Now), reader: &fakeRecoveryReader{},
			log: logr.Discard(), now: time.Now, inflight: make(map[string]bool),
		},
		swaps: swaps, server: srv, log: logr.Discard(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want canceled clean shutdown", err)
	}
	if state.routerCalls != 1 || state.adapterCalls != 0 {
		t.Fatalf("dynamic internal validation: router=%d adapters=%d", state.routerCalls, state.adapterCalls)
	}
}
