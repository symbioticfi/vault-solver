package config

import (
	"math"

	"github.com/go-errors/errors"
)

func (c *Config) validate() error {
	if c.Chain.RPCURL == "" {
		return errors.New("chain.rpcUrl is required")
	}
	for index, endpoint := range c.Chain.RPCFallbackURLs {
		if endpoint == "" {
			return errors.Errorf("chain.rpcFallbackUrls[%d] is empty", index)
		}
	}
	if c.Chain.ChainID == 0 {
		return errors.New("chain.chainId is required")
	}
	if err := c.TxManager.validate(false); err != nil {
		return err
	}
	if err := c.Signer.validate(); err != nil {
		return err
	}
	if len(c.Solvers) == 0 {
		return errors.New("at least one solver is required (set `solvers`)")
	}
	seen := make(map[string]struct{}, len(c.Solvers))
	for index, integration := range c.Solvers {
		if integration.Name == "" {
			return errors.Errorf("solvers[%d].name is required", index)
		}
		if _, duplicate := seen[integration.Name]; duplicate {
			return errors.Errorf(
				"duplicate solver %q: only one entry per solver type is allowed",
				integration.Name,
			)
		}
		seen[integration.Name] = struct{}{}
	}
	return nil
}

// ValidateTxManager applies the additional fee requirement used only when at least one selected
// integration submits transactions through the process nonce lane.
func (c *Config) ValidateTxManager() error {
	return c.TxManager.validate(true)
}

func (c TxManagerConfig) validate(required bool) error {
	invalidMaximum := c.MaxFeeGwei < 0 || required && c.MaxFeeGwei == 0 ||
		math.IsNaN(c.MaxFeeGwei) || math.IsInf(c.MaxFeeGwei, 0)
	if invalidMaximum {
		return errors.New("txManager.maxFeeGwei must be finite and positive")
	}
	if c.TipGwei < 0 || math.IsNaN(c.TipGwei) || math.IsInf(c.TipGwei, 0) {
		return errors.New("txManager.tipGwei must be finite and non-negative")
	}
	if c.BroadcastTimeoutMs <= 0 {
		return errors.New("txManager.broadcastTimeoutMs must be positive")
	}
	if c.AccountPollIntervalMs <= 0 {
		return errors.New("txManager.accountPollIntervalMs must be positive")
	}
	if c.ReplacementIntervalMs <= 0 {
		return errors.New("txManager.replacementIntervalMs must be positive")
	}
	if c.PendingTimeoutMs < c.ReplacementIntervalMs {
		return errors.New("txManager.pendingTimeoutMs must be at least replacementIntervalMs")
	}
	if c.ShutdownTimeoutMs <= 0 {
		return errors.New("txManager.shutdownTimeoutMs must be positive")
	}
	return nil
}

func (c SignerConfig) validate() error {
	switch {
	case c.KeyEnv != "" && c.KeystorePath != "":
		return errors.New("signer: set exactly one of keyEnv or keystorePath, not both")
	case c.KeyEnv != "":
		return nil
	case c.KeystorePath == "":
		return errors.New("signer: one of keyEnv or keystorePath is required")
	case c.PassphraseEnv == "":
		return errors.New("signer: keystorePath requires passphraseEnv")
	default:
		return nil
	}
}
