// Package app is the composition root. It owns process resources; integrations own protocol work.
package app

import (
	"context"
	"net"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/version"
)

func newSolver(name string, raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	switch name {
	case bridgefacilitator.Name:
		return bridgefacilitator.New(raw, deps)
	case rfq.Name:
		return rfq.New(raw, deps)
	case lifi.Name:
		return lifi.New(raw, deps)
	case uniswapx.Name:
		return uniswapx.New(raw, deps)
	case redstoneoev.Name:
		return redstoneoev.New(raw, deps)
	default:
		return nil, errors.Errorf("unknown solver %q (available: %v)", name, []string{bridgefacilitator.Name, lifi.Name, redstoneoev.Name, rfq.Name, uniswapx.Name})
	}
}

// Run opens resources in dependency order and closes them in reverse order. A nil debug override
// uses YAML. Transaction ownership lasts through solver shutdown, including withdrawal of quotes.
func Run(ctx context.Context, path string, debug *bool) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if debug != nil {
		cfg.Observability.Debug = *debug
	}
	log, flush, err := observability.NewLogger(cfg.Observability.Debug)
	if err != nil {
		return err
	}
	defer flush()
	names := make([]string, len(cfg.Solvers))
	for i, spec := range cfg.Solvers {
		names[i] = spec.Name
	}
	if len(names) == 1 {
		log = log.WithValues("solver", names[0])
	}
	log.Info("starting", "version", version.String(), "solvers", names)

	runCtx, fail := context.WithCancelCause(ctx)
	defer fail(nil)
	metrics, health := observability.NewMetrics()
	metrics.SetBuildInfo(version.Version, version.Commit)
	metrics.SetSolvers(names)
	// Bind synchronously: startup must not claim success when probes cannot be served.
	listener, err := (&net.ListenConfig{}).Listen(runCtx, "tcp", cfg.Observability.Addr)
	if err != nil {
		return errors.Errorf("listen for observability: %w", err)
	}
	probes := observability.NewHTTPServer(cfg.Observability.Addr, metrics)
	probesCtx, stopProbes := context.WithCancel(context.WithoutCancel(runCtx))
	probesDone := make(chan error, 1)
	go func() {
		err := serve(probesCtx, probes, listener)
		if err != nil {
			fail(err)
		}
		probesDone <- err
	}()
	defer func() { stopProbes(); <-probesDone }()

	rpcMetrics, err := chain.NewRPCMetrics(metrics.Registerer())
	if err != nil {
		return err
	}
	rpc, err := chain.DialWithMetrics(runCtx,
		append([]string{cfg.Chain.RPCURL}, cfg.Chain.RPCFallbackURLs...),
		cfg.Chain.WriteRPCURL, cfg.Chain.MulticallAddress, rpcMetrics, log)
	if err != nil {
		return err
	}
	defer rpc.Close()
	if got := rpc.ChainID(); !got.IsUint64() || got.Uint64() != cfg.Chain.ChainID {
		return errors.Errorf("chain id mismatch: rpc reports %s, config says %d", got, cfg.Chain.ChainID)
	}
	key, err := signer.FromConfig(cfg.Signer)
	if err != nil {
		return err
	}
	txMetrics, err := txmanager.NewMetrics(metrics.Registerer())
	if err != nil {
		return err
	}
	txm := txmanager.New(rpc, key, rpc.ChainID(), transactionConfig(cfg.TxManager), txMetrics, log)
	deps := solver.Deps{Chain: rpc, Signer: key, TxManager: txm, Metrics: metrics, Log: log, ReportFatal: fail}
	services := make([]service, 0, len(cfg.Solvers))
	sends := false
	preparation := time.Duration(0)
	for _, spec := range cfg.Solvers {
		localDeps := deps
		if len(names) > 1 {
			localDeps.Log = log.WithValues("solver", spec.Name)
		}
		instance, err := newSolver(spec.Name, spec.Config, localDeps)
		if err != nil {
			return errors.Errorf("construct %s: %w", spec.Name, err)
		}
		services = append(services, service{instance, localDeps.Log})
		sends = sends || solver.RequiresTxManager(instance)
		if p, ok := instance.(solver.ShutdownPreparer); ok {
			preparation = max(preparation, p.ShutdownPreparationTimeout())
		}
	}
	if !sends {
		return supervise(runCtx, fail, services, nil, health, 0, log)
	}
	if err := cfg.ValidateTxManager(); err != nil {
		return err
	}
	if err := txm.ValidateFeeHeadroom(); err != nil {
		return err
	}
	if err := txm.Initialize(runCtx); err != nil {
		return errors.Errorf("initialize transaction lane: %w", err)
	}
	drain := preparation + time.Duration(cfg.TxManager.PendingTimeoutMs+cfg.TxManager.ReplacementIntervalMs)*time.Millisecond
	return supervise(runCtx, fail, services, txm, health, drain, log)
}

func transactionConfig(c config.TxManagerConfig) txmanager.Config {
	return txmanager.Config{
		Confirmations: c.Confirmations, MaxFeeGwei: c.MaxFeeGwei, TipGwei: c.TipGwei,
		BroadcastTimeout:    time.Duration(c.BroadcastTimeoutMs) * time.Millisecond,
		AccountPollInterval: time.Duration(c.AccountPollIntervalMs) * time.Millisecond,
		ReplacementInterval: time.Duration(c.ReplacementIntervalMs) * time.Millisecond,
		PendingTimeout:      time.Duration(c.PendingTimeoutMs) * time.Millisecond,
		ShutdownTimeout:     time.Duration(c.ShutdownTimeoutMs) * time.Millisecond,
	}
}

// service associates lifecycle errors with their configured integration.
type service struct {
	solver.Solver

	log logr.Logger
}
