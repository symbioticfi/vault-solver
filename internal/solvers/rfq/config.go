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
	BackendURL             string   `yaml:"backendUrl"`
	BackendSharedSecretEnv string   `yaml:"backendSharedSecretEnv"`
	ListenAddr             string   `yaml:"listenAddr"`
	Executor               string   `yaml:"executor"`
	Adapter                string   `yaml:"adapter"`
	Reactor                string   `yaml:"reactor"`
	CuratorRegistry        string   `yaml:"curatorRegistry"`
	QuoteDiscountBps       uint64   `yaml:"quoteDiscountBps"`
	PollIntervalMs         int      `yaml:"pollIntervalMs"`
	OrderLimit             int      `yaml:"orderLimit"`
	Vaults                 []string `yaml:"vaults"`
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
	// Adapter is the InstantRedemptionAdapter the filler prices against.
	Adapter common.Address
	// Reactor is the RFQ Reactor (used at execution time; P2).
	Reactor common.Address
	// CuratorRegistry resolves vault curators for permissioned/discount vaults (P3); optional.
	CuratorRegistry common.Address
	// QuoteDiscountBps is the discount applied to oracle pricing before quoting (basis points).
	QuoteDiscountBps uint64
	// PollInterval is how often the backend is polled for open orders (P2).
	PollInterval time.Duration
	// OrderLimit caps how many open orders are fetched per poll (P2).
	OrderLimit int
	// Vaults is an optional candidate vault universe used to rebuild a strategy on-chain when the
	// quote-time strategy isn't cached (e.g. after a restart). Empty disables recovery.
	Vaults []common.Address
}

// Defaults applied when a field is unset.
const (
	defaultListenAddr   = ":42073"
	defaultDiscountBps  = 1000 // 10.00%
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
	adapter, err := parseAddress(raw.Adapter, "adapter")
	if err != nil {
		return nil, err
	}
	if raw.QuoteDiscountBps > 10_000 {
		return nil, errors.Errorf("quoteDiscountBps must be <= 10000, got %d", raw.QuoteDiscountBps)
	}

	cfg := &Config{
		BackendURL:             raw.BackendURL,
		BackendSharedSecretEnv: raw.BackendSharedSecretEnv,
		ListenAddr:             orStr(raw.ListenAddr, defaultListenAddr),
		Executor:               executor,
		Adapter:                adapter,
		QuoteDiscountBps:       raw.QuoteDiscountBps,
		PollInterval:           defaultPollInterval,
		OrderLimit:             defaultOrderLimit,
	}
	if cfg.QuoteDiscountBps == 0 {
		cfg.QuoteDiscountBps = defaultDiscountBps
	}
	if raw.PollIntervalMs > 0 {
		cfg.PollInterval = time.Duration(raw.PollIntervalMs) * time.Millisecond
	}
	if raw.OrderLimit > 0 {
		cfg.OrderLimit = raw.OrderLimit
	}
	// Reactor and CuratorRegistry are optional in P1 (used by execution/discount phases). Parse when
	// present so a bad address fails fast.
	if raw.Reactor != "" {
		if cfg.Reactor, err = parseAddress(raw.Reactor, "reactor"); err != nil {
			return nil, err
		}
	}
	if raw.CuratorRegistry != "" {
		if cfg.CuratorRegistry, err = parseAddress(raw.CuratorRegistry, "curatorRegistry"); err != nil {
			return nil, err
		}
	}
	for i, v := range raw.Vaults {
		addr, verr := parseAddress(v, "vaults["+strconv.Itoa(i)+"]")
		if verr != nil {
			return nil, verr
		}
		cfg.Vaults = append(cfg.Vaults, addr)
	}
	return cfg, nil
}

func parseAddress(s, field string) (common.Address, error) {
	if !common.IsHexAddress(s) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, s)
	}
	return common.HexToAddress(s), nil
}

func orStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
