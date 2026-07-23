package rfq

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

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
	quotes.strategy = newDefaultTestStrategy(18, map[common.Address]*big.Int{tOut: big.NewInt(1_000000)})

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
