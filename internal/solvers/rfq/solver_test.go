package rfq

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
)

// TestBuildServices_WhitelistWiring pins that the parsed config flag actually reaches both services:
// reverting the factory wiring (leaving the whitelist nil) would silently turn the default-enabled
// whitelist fail-open.
func TestBuildServices_WhitelistWiring(t *testing.T) {
	listed := common.HexToAddress("0x0000000000000000000000000000000000000042")
	rogue := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	cfg := &Config{
		BackendURL: "https://rfq-backend.example",
		Executor:   common.HexToAddress("0x0000000000000000000000000000000000000010"),
		Vaults:     []recoveryVault{{Adapter: listed}},
	}
	st := newStore(func() time.Time { return time.Unix(0, 0) })

	cfg.AdapterWhitelistEnabled = true
	quotes, exec := buildServices(cfg, 1, st, nil, nil, logr.Discard())
	for name, wl := range map[string]adapterWhitelist{"quote": quotes.whitelist, "execution": exec.whitelist} {
		if wl == nil {
			t.Fatalf("%s service: whitelist not wired (nil = fail open)", name)
		}
		if !wl.allows(listed) || wl.allows(rogue) {
			t.Fatalf("%s service: whitelist = %v, want exactly the configured adapters", name, wl)
		}
	}

	cfg.AdapterWhitelistEnabled = false
	quotes, exec = buildServices(cfg, 1, st, nil, nil, logr.Discard())
	if quotes.whitelist != nil || exec.whitelist != nil {
		t.Fatal("disabled whitelist should be wired as nil (filtering off) on both services")
	}
}
