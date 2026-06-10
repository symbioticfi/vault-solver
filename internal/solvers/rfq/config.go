package rfq

import (
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parseConfig.
type rawConfig struct {
	BackendURL              string     `yaml:"backendUrl"`
	BackendSharedSecretEnv  string     `yaml:"backendSharedSecretEnv"`
	ListenAddr              string     `yaml:"listenAddr"`
	Executor                string     `yaml:"executor"`
	Reactor                 string     `yaml:"reactor"`
	PollIntervalMs          int        `yaml:"pollIntervalMs"`
	OrderLimit              int        `yaml:"orderLimit"`
	AdapterWhitelistEnabled bool       `yaml:"adapterWhitelistEnabled"`
	Vaults                  []rawVault `yaml:"vaults"`
}

// rawVault mirrors one entry of the `vaults` list (adapter whitelist + recovery universe): a vault,
// its LiquidLane adapter, and the vault's collateral asset.
type rawVault struct {
	Address string `yaml:"address"`
	Adapter string `yaml:"adapter"`
	Asset   string `yaml:"asset"`
}

// Config is the validated, typed RFQ solver configuration.
type Config struct {
	// BackendURL is the RFQ backend base URL (orders + discounts).
	BackendURL string
	// BackendSharedSecretEnv is the env var NAME holding the shared secret that authenticates the
	// backend peer on /quote (the secret itself is read at startup, never stored here).
	BackendSharedSecretEnv string
	// ListenAddr is the bind address for the quote HTTP server.
	ListenAddr string
	// Executor is the Executor contract (the on-chain filler identity; the bot EOA holds CALLER_ROLE).
	Executor common.Address
	// Reactor is the RFQ Reactor (used at execution time); optional.
	Reactor common.Address
	// PollInterval is how often the backend is polled for open orders.
	PollInterval time.Duration
	// OrderLimit caps how many open orders are fetched per poll.
	OrderLimit int
	// AdapterWhitelistEnabled restricts quoting and filling to the adapters of the configured
	// Vaults. Off by default: every adapter the backend advertises is accepted. While enabled, an
	// empty Vaults list accepts no adapters at all — fail closed.
	AdapterWhitelistEnabled bool
	// Vaults is the configured vault/adapter universe: the whitelist source (see
	// AdapterWhitelistEnabled) and the candidate universe used to rebuild a strategy on-chain when
	// the quote-time strategy isn't cached (e.g. after a restart). Each entry pins a vault, its
	// LiquidLane adapter, and the expected collateral asset. Empty disables recovery.
	Vaults []recoveryVault
}

// Defaults applied when a field is unset.
const (
	defaultListenAddr   = ":42073"
	defaultPollInterval = 3 * time.Second
	defaultOrderLimit   = 20
)

// parseConfig decodes and validates the opaque rfq solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := node.Decode(&raw); err != nil {
		return nil, errors.Errorf("decode solver config: %w", err)
	}
	if raw.BackendURL == "" {
		return nil, errors.New("backendUrl is required")
	}
	if raw.BackendSharedSecretEnv == "" {
		return nil, errors.New("backendSharedSecretEnv is required")
	}
	executor, err := parseAddress(raw.Executor, "executor")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		BackendURL:              raw.BackendURL,
		BackendSharedSecretEnv:  raw.BackendSharedSecretEnv,
		ListenAddr:              orStr(raw.ListenAddr, defaultListenAddr),
		Executor:                executor,
		PollInterval:            defaultPollInterval,
		OrderLimit:              defaultOrderLimit,
		AdapterWhitelistEnabled: raw.AdapterWhitelistEnabled,
	}
	if raw.PollIntervalMs > 0 {
		cfg.PollInterval = time.Duration(raw.PollIntervalMs) * time.Millisecond
	}
	if raw.OrderLimit > 0 {
		cfg.OrderLimit = raw.OrderLimit
	}
	// Reactor is optional (used by execution). Parse when present so a bad address fails fast.
	if raw.Reactor != "" {
		if cfg.Reactor, err = parseAddress(raw.Reactor, "reactor"); err != nil {
			return nil, err
		}
	}
	for i, v := range raw.Vaults {
		rv, verr := v.parse(i)
		if verr != nil {
			return nil, verr
		}
		cfg.Vaults = append(cfg.Vaults, rv)
	}
	return cfg, nil
}

// parse validates one vaults entry into the typed form. The zero address is rejected: these
// addresses feed the adapter whitelist and on-chain recovery reads, so a placeholder left in a
// config must fail at startup rather than weaken the whitelist.
func (v rawVault) parse(i int) (recoveryVault, error) {
	prefix := "vaults[" + strconv.Itoa(i) + "]."
	addr, err := parseNonZeroAddress(v.Address, prefix+"address")
	if err != nil {
		return recoveryVault{}, err
	}
	adapterAddr, err := parseNonZeroAddress(v.Adapter, prefix+"adapter")
	if err != nil {
		return recoveryVault{}, err
	}
	asset, err := parseNonZeroAddress(v.Asset, prefix+"asset")
	if err != nil {
		return recoveryVault{}, err
	}
	return recoveryVault{Adapter: adapterAddr, Vault: addr, AssetHint: asset}, nil
}

func parseAddress(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func parseNonZeroAddress(s, field string) (common.Address, error) {
	addr, err := parseAddress(s, field)
	if err != nil {
		return common.Address{}, err
	}
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address (placeholder not replaced?)", field)
	}
	return addr, nil
}

func orStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
