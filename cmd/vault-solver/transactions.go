package main

import (
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/delegation"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// newTransactionSender keeps contract-specific setup at the composition boundary.
// Protocol signers and solver identity remain the primary signer in solver.Deps.
func newTransactionSender(
	backend txmanager.Backend, primary signer.Signer, chainID *big.Int,
	cfg config.TxManagerConfig, reg prometheus.Registerer, log logr.Logger,
) (txmanager.Sender, error) {
	managerConfig := txmanager.Config{
		Confirmations: cfg.Confirmations, MaxFeeGwei: cfg.MaxFeeGwei, TipGwei: cfg.TipGwei,
		BroadcastTimeout:    time.Duration(cfg.BroadcastTimeoutMs) * time.Millisecond,
		AccountPollInterval: time.Duration(cfg.AccountPollIntervalMs) * time.Millisecond,
		ReplacementInterval: time.Duration(cfg.ReplacementIntervalMs) * time.Millisecond,
		PendingTimeout:      time.Duration(cfg.PendingTimeoutMs) * time.Millisecond,
		ShutdownTimeout:     time.Duration(cfg.ShutdownTimeoutMs) * time.Millisecond,
	}
	if cfg.Delegation == nil {
		metrics, err := txmanager.NewMetrics(reg)
		if err != nil {
			return nil, err
		}
		return txmanager.NewWithMetrics(backend, primary, chainID, managerConfig, metrics, log), nil
	}
	codeReader, ok := backend.(delegation.CodeReader)
	if !ok {
		return nil, errors.New("delegation requires an account-code reader")
	}
	signers := []signer.Signer{primary}
	callers := make([]common.Address, 0, len(cfg.Delegation.AuxiliarySigners))
	for i, signerConfig := range cfg.Delegation.AuxiliarySigners {
		auxiliary, err := signer.FromConfig(signerConfig)
		if err != nil {
			return nil, errors.Errorf("auxiliary signer %d: %w", i, err)
		}
		signers = append(signers, auxiliary)
		callers = append(callers, auxiliary.Address())
	}
	forwarder, err := delegation.New(codeReader, primary.Address(), common.HexToAddress(cfg.Delegation.DelegateAddress), callers)
	if err != nil {
		return nil, err
	}
	lanes := make([]*txmanager.Manager, 0, len(signers))
	for i, s := range signers {
		address := strings.ToLower(s.Address().Hex())
		metrics, err := txmanager.NewMetrics(prometheus.WrapRegistererWith(prometheus.Labels{"sender": address}, reg))
		if err != nil {
			return nil, err
		}
		laneConfig := managerConfig
		if i == 0 {
			// A self-transfer would execute the delegated fallback and exceed 21,000
			// gas. A zero-value transfer to zero consumes only this sender's nonce.
			laneConfig.CancellationRecipient = new(common.Address)
		} else {
			laneConfig.Preparer = forwarder
		}
		lanes = append(lanes, txmanager.NewWithMetrics(backend, s, chainID, laneConfig, metrics, log.WithValues("sender", address)))
	}
	return txmanager.NewPool(lanes...)
}
