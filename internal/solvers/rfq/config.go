package rfq

import (
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// rawConfig mirrors the YAML shape; strings are parsed into typed values in parseConfig.
type rawConfig struct {
	BackendURL             string            `yaml:"backendUrl"`
	BackendSharedSecretEnv string            `yaml:"backendSharedSecretEnv"`
	ListenAddr             string            `yaml:"listenAddr"`
	Executor               string            `yaml:"executor"`
	Reactor                string            `yaml:"reactor"`
	PollIntervalMs         int               `yaml:"pollIntervalMs"`
	OrderLimit             int               `yaml:"orderLimit"`
	SolverMode             string            `yaml:"solverMode"`
	TokensToQuote          string            `yaml:"tokensToQuote"`
	PermissionedTokens     []string          `yaml:"permissionedTokens"`
	Adapters               []string          `yaml:"adapters"`
	Strategy               rawStrategyConfig `yaml:"strategy"`
}

type rawStrategyConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
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
	// Reactor is the RFQ Reactor (used at execution time); optional.
	Reactor common.Address
	// PollInterval is how often the backend is polled for open orders.
	PollInterval time.Duration
	// OrderLimit caps how many open orders are fetched per poll.
	OrderLimit int
	// SolverMode is the deployment profile operators set: "external" (default) or "internal". It drives
	// the discount-API gate and adapter scoping (see usesDiscounts / restrictsToAdapters / quoteScopesToAdapters):
	//   - external: never calls the internal-only discounts API; adapters are REQUIRED and scope quoting AND filling.
	//   - internal: uses public discounts; adapters (optional) scope the QUOTE path only, while filling stays
	//     unrestricted so discount-driven recovery legs through any advertised adapter still execute.
	SolverMode string
	// TokensToQuote scopes which input tokens this filler quotes by class: "all" (default) quotes any,
	// "permissioned" quotes only tokens in PermissionedTokens, "permissionless" quotes only tokens NOT in
	// PermissionedTokens. Typically set per instance via env (e.g. tokensToQuote: ${TOKENS_TO_QUOTE}).
	TokensToQuote string
	// PermissionedTokens is the local set of input-token addresses treated as permissioned. The
	// TokensToQuote scope is evaluated against it. When TokensToQuote is "permissioned", admitted
	// inputs must use exactly one route. Empty means no input token is permissioned.
	PermissionedTokens map[common.Address]bool
	// Adapters is the configured LiquidLane adapter universe: in external mode the set quoting/filling is
	// scoped to, and the candidate universe used to rebuild a fill plan when the quote-time plan isn't
	// cached (e.g. after a restart). Config carries only adapter addresses;
	// each entry's Vault (adapter.vault()) and Asset (vault.asset()) are resolved on-chain at startup
	// (see reader.resolveVaults) and are fixed for the adapter's lifetime. Empty disables fill-plan recovery.
	Adapters []recoveryVault
	Strategy StrategyConfig
}

type StrategyConfig struct {
	Name   string
	Config yaml.Node
}

// Solver-mode profiles (see Config.SolverMode).
const (
	solverModeExternal = "external" // permissioned adapters only; no discounts API (default)
	solverModeInternal = "internal" // public discounts API on top of all advertised adapters
)

// Input-token quote scopes (see Config.TokensToQuote): "all" quotes any input token, "permissioned"
// quotes only tokens in PermissionedTokens, "permissionless" quotes only tokens not in it.
const (
	tokensToQuoteAll            = "all"
	tokensToQuotePermissioned   = "permissioned"
	tokensToQuotePermissionless = "permissionless"
)

// Defaults applied when a field is unset.
const (
	defaultListenAddr   = ":42073"
	defaultPollInterval = 3 * time.Second
	defaultOrderLimit   = 20
	defaultSolverMode   = solverModeExternal
	defaultStrategyName = "default"
)

// parseConfig decodes and validates the opaque rfq solver config block.
func parseConfig(node yaml.Node) (*Config, error) {
	var raw rawConfig
	if err := solver.DecodeStrict(node, &raw); err != nil {
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
	mode := parse.OrDefault(raw.SolverMode, defaultSolverMode)
	if mode != solverModeExternal && mode != solverModeInternal {
		return nil, errors.Errorf("solverMode: must be %q or %q, got %q", solverModeExternal, solverModeInternal, mode)
	}
	scope := parse.OrDefault(raw.TokensToQuote, tokensToQuoteAll)
	if scope != tokensToQuoteAll && scope != tokensToQuotePermissioned && scope != tokensToQuotePermissionless {
		return nil, errors.Errorf("tokensToQuote: must be %q, %q or %q, got %q",
			tokensToQuoteAll, tokensToQuotePermissioned, tokensToQuotePermissionless, scope)
	}

	cfg := &Config{
		BackendURL:             raw.BackendURL,
		BackendSharedSecretEnv: raw.BackendSharedSecretEnv,
		ListenAddr:             parse.OrDefault(raw.ListenAddr, defaultListenAddr),
		Executor:               executor,
		PollInterval:           defaultPollInterval,
		OrderLimit:             defaultOrderLimit,
		SolverMode:             mode,
		TokensToQuote:          scope,
		Strategy: StrategyConfig{
			Name:   parse.OrDefault(raw.Strategy.Name, defaultStrategyName),
			Config: raw.Strategy.Config,
		},
	}
	for i, t := range raw.PermissionedTokens {
		addr, terr := parse.NonZeroAddress(t, "permissionedTokens["+strconv.Itoa(i)+"]")
		if terr != nil {
			return nil, terr
		}
		if cfg.PermissionedTokens == nil {
			cfg.PermissionedTokens = make(map[common.Address]bool, len(raw.PermissionedTokens))
		}
		cfg.PermissionedTokens[addr] = true
	}
	if raw.PollIntervalMs > 0 {
		cfg.PollInterval = time.Duration(raw.PollIntervalMs) * time.Millisecond
	}
	if raw.OrderLimit > 0 {
		cfg.OrderLimit = raw.OrderLimit
	}
	if raw.Reactor != "" {
		if cfg.Reactor, err = parse.Address(raw.Reactor, "reactor"); err != nil {
			return nil, err
		}
	}
	for i, a := range raw.Adapters {
		adapter, err := parse.NonZeroAddress(a, "adapters["+strconv.Itoa(i)+"]")
		if err != nil {
			return nil, err
		}
		cfg.Adapters = append(cfg.Adapters, recoveryVault{Adapter: adapter})
	}
	if mode == solverModeExternal && len(cfg.Adapters) == 0 {
		return nil, errors.New(`solverMode "external" requires at least one adapters entry`)
	}
	return cfg, nil
}

// usesDiscounts reports whether this solver may call the internal-only discounts API (internal mode only).
func (c *Config) usesDiscounts() bool { return c.SolverMode == solverModeInternal }

// restrictsToAdapters reports whether the EXECUTION path (order filling, incl. discount-leg recovery) is
// scoped to the configured Adapters: external mode with at least one adapter. parseConfig requires external
// to have adapters; the len check guards hand-built Configs.
func (c *Config) restrictsToAdapters() bool {
	return c.SolverMode == solverModeExternal && len(c.Adapters) > 0
}

// quoteScopesToAdapters reports whether the QUOTE path is scoped to the configured Adapters. It scopes in
// both modes whenever Adapters is non-empty, while execution scoping stays external-only.
func (c *Config) quoteScopesToAdapters() bool {
	return len(c.Adapters) > 0
}
