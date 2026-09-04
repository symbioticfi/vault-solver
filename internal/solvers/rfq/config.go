package rfq

import (
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parseConfig.
type rawConfig struct {
	BackendURL             string            `yaml:"backendUrl"`
	BackendSharedSecretEnv string            `yaml:"backendSharedSecretEnv"`
	ListenAddr             string            `yaml:"listenAddr"`
	Executor               string            `yaml:"executor"`
	Reactor                string            `yaml:"reactor"`
	LiquidityLens          string            `yaml:"liquidityLens"`
	PollIntervalMs         int               `yaml:"pollIntervalMs"`
	OrderLimit             int               `yaml:"orderLimit"`
	SolverMode             string            `yaml:"solverMode"`
	TokensToQuote          string            `yaml:"tokensToQuote"`
	PermissionedTokens     []string          `yaml:"permissionedTokens"`
	MinAmountsIn           map[string]string `yaml:"minAmountsIn"`
	Adapters               []string          `yaml:"adapters"`
	Strategy               StrategyConfig    `yaml:"strategy"`
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
	// Executor is the Executor contract (the on-chain filler identity; the bot EOA must be an authorized
	// caller — added to the Executor's callers allowlist via setCallers by its owner).
	Executor common.Address
	// Reactor is optional deployment identity retained for runtime diagnostics. Signed order validation
	// uses request.protocol; the solver never trusts this field as an execution target.
	Reactor common.Address
	// LiquidityLens is the optional FrontendLiquidityLens address. When set, LiquidLane swappable headroom
	// is read from the lens's cross-adapter deallocation-cascade estimate instead of each adapter's own
	// getMaxAssets(tokenToRedeem); zero falls back to the adapter getter.
	LiquidityLens common.Address
	// PollInterval is how often the backend is polled for open orders.
	PollInterval time.Duration
	// OrderLimit caps how many open orders are fetched per poll.
	OrderLimit int
	// SolverMode is the deployment profile operators set: "external" (default) or "internal". It drives
	// the discount-API gate and adapter scoping (see usesDiscounts / restrictsToAdapters / quoteScopesToAdapters):
	//   - external: never calls the internal-only discounts API; adapters are REQUIRED and scope quoting AND filling.
	//   - internal: uses public discounts; adapters (optional) scope the QUOTE path only, while filling stays
	//     unrestricted so discount-driven recovery legs through any advertised adapter still execute.
	SolverMode liquidlane.SolverMode
	// TokenPolicy scopes quoted input tokens and enforces single-route fills in permissioned mode.
	TokenPolicy tokenpolicy.Policy
	// MinAmountsIn is the per-input-token minimum request size, keyed by input-token address (config
	// values are decimal strings in the token's BASE UNITS, e.g. "1000000000000000000" for 1e18).
	// A request whose amount is strictly below its token's minimum gets no quote (HTTP 204); an amount
	// equal to the minimum still quotes. A token absent from the map has no minimum. Address keys make
	// the lookup checksum/case-insensitive.
	MinAmountsIn map[common.Address]*big.Int
	// Adapters is the configured LiquidLane adapter universe: in external mode the set quoting/filling is
	// scoped to, and the candidate universe used to build each fresh fill plan. Config carries only adapter addresses;
	// each entry's Vault (adapter.vault()) and Asset (vault.asset()) are resolved on-chain at startup
	// (see reader.resolveVaults) and are fixed for the adapter's lifetime. Empty disables direct fill planning.
	Adapters []recoveryVault
	Strategy StrategyConfig
}

type StrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

// Defaults applied when a field is unset.
const (
	defaultListenAddr   = ":42073"
	defaultPollInterval = 3 * time.Second
	defaultOrderLimit   = 20
	defaultStrategyName = "default"
)

// parseConfig decodes and validates the opaque rfq solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := parse.DecodeStrict(node, &raw); err != nil {
		return nil, err
	}
	if raw.BackendURL == "" {
		return nil, errors.New("backendUrl is required")
	}
	if raw.BackendSharedSecretEnv == "" {
		return nil, errors.New("backendSharedSecretEnv is required")
	}
	executor, err := parse.Address(raw.Executor, "executor")
	if err != nil {
		return nil, err
	}
	mode, err := liquidlane.ParseSolverMode(raw.SolverMode)
	if err != nil {
		return nil, err
	}
	tokenPolicy, err := tokenpolicy.Parse(raw.TokensToQuote, raw.PermissionedTokens)
	if err != nil {
		return nil, err
	}
	if raw.Strategy.Name == "" {
		raw.Strategy.Name = defaultStrategyName
	}

	cfg := &Config{
		BackendURL:             raw.BackendURL,
		BackendSharedSecretEnv: raw.BackendSharedSecretEnv,
		ListenAddr:             parse.OrDefault(raw.ListenAddr, defaultListenAddr),
		Executor:               executor,
		PollInterval:           defaultPollInterval,
		OrderLimit:             defaultOrderLimit,
		SolverMode:             mode,
		TokenPolicy:            tokenPolicy,
		Strategy:               raw.Strategy,
	}
	if raw.PollIntervalMs > 0 {
		cfg.PollInterval = time.Duration(raw.PollIntervalMs) * time.Millisecond
	}
	if raw.OrderLimit > 0 {
		cfg.OrderLimit = raw.OrderLimit
	}
	if cfg.LiquidityLens, err = parse.OptionalNonZeroAddress(raw.LiquidityLens, "liquidityLens"); err != nil {
		return nil, err
	}
	if cfg.Reactor, err = parse.OptionalNonZeroAddress(raw.Reactor, "reactor"); err != nil {
		return nil, err
	}
	for token, amount := range raw.MinAmountsIn {
		field := `minAmountsIn["` + token + `"]`
		addr, aerr := parse.NonZeroAddress(token, field)
		if aerr != nil {
			return nil, aerr
		}
		minIn, berr := parse.Big(amount, field)
		if berr != nil {
			return nil, berr
		}
		if minIn.Sign() <= 0 { // parse.Big accepts negatives; a non-positive floor is a misconfiguration
			return nil, errors.Errorf("%s: must be > 0, got %q", field, amount)
		}
		if cfg.MinAmountsIn == nil {
			cfg.MinAmountsIn = make(map[common.Address]*big.Int, len(raw.MinAmountsIn))
		}
		// Keys differing only in checksum case collide into one address; reject rather than silently
		// letting map order pick a winner.
		if _, dup := cfg.MinAmountsIn[addr]; dup {
			return nil, errors.Errorf("%s: duplicate entry for token %s", field, addr.Hex())
		}
		cfg.MinAmountsIn[addr] = minIn
	}
	for i, a := range raw.Adapters {
		adapter, err := parse.NonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		cfg.Adapters = append(cfg.Adapters, recoveryVault{Adapter: adapter})
	}
	if mode == liquidlane.SolverModeExternal && len(cfg.Adapters) == 0 {
		return nil, errors.New(`solverMode "external" requires at least one adapters entry`)
	}
	return cfg, nil
}

// usesDiscounts reports whether this solver may call the internal-only discounts API (internal mode only).
func (c *Config) usesDiscounts() bool { return c.SolverMode == liquidlane.SolverModeInternal }

// restrictsToAdapters reports whether the EXECUTION path (order filling, incl. discount-leg recovery) is
// scoped to the configured Adapters: external mode with at least one adapter. parseConfig requires external
// to have adapters; the len check guards hand-built Configs.
func (c *Config) restrictsToAdapters() bool {
	return c.SolverMode == liquidlane.SolverModeExternal && len(c.Adapters) > 0
}

// quoteScopesToAdapters reports whether the QUOTE path is scoped to the configured Adapters. It scopes in
// both modes whenever Adapters is non-empty, while execution scoping stays external-only.
func (c *Config) quoteScopesToAdapters() bool {
	return len(c.Adapters) > 0
}
