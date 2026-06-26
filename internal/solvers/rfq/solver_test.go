package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

// TestBuildServices_WhitelistWiring pins that solver mode actually reaches both services: reverting the
// factory wiring (leaving the whitelist nil) would silently let an external solver quote/fill through
// adapters it isn't scoped to.
func TestBuildServices_WhitelistWiring(t *testing.T) {
	listed := common.HexToAddress("0x0000000000000000000000000000000000000042")
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cfg := &Config{
		BackendURL: "https://rfq-backend.example",
		Executor:   common.HexToAddress("0x0000000000000000000000000000000000000010"),
		Adapters:   []recoveryVault{{Adapter: listed}},
	}
	st := newStore(func() time.Time { return time.Unix(0, 0) })

	cfg.SolverMode = solverModeExternal // external + configured adapters ⇒ restrictsToAdapters()
	quotes, exec := buildServices(cfg, 1, st, nil, nil, logr.Discard())
	for name, wl := range map[string]adapterWhitelist{"quote": quotes.whitelist, "execution": exec.whitelist} {
		if wl == nil {
			t.Fatalf("%s service: whitelist not wired (nil = fail open)", name)
		}
		if !wl.allows(listed) || wl.allows(rogue) {
			t.Fatalf("%s service: whitelist = %v, want exactly the configured adapters", name, wl)
		}
	}

	cfg.SolverMode = solverModeInternal // internal ⇒ no adapter scoping (whitelist off)
	quotes, exec = buildServices(cfg, 1, st, nil, nil, logr.Discard())
	if quotes.whitelist != nil || exec.whitelist != nil {
		t.Fatal("internal mode should wire the whitelist as nil (filtering off) on both services")
	}
}
