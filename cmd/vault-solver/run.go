package main

import (
	"context"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/version"
)

func newRunCmd() *cobra.Command {
	var (
		configPath string
		debug      bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the solver against the configured vaults",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The flag overrides observability.debug from config only when explicitly passed.
			return runBot(cmd.Context(), configPath, debug, cmd.Flags().Changed("debug"))
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "path to the YAML config file (required)")
	cmd.Flags().BoolVar(&debug, "debug", false, "enable debug logging (overrides observability.debug)")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

// runBot wires the dependency graph and runs the selected solver until ctx is cancelled. The log
// level is resolved from config, overridden by the --debug flag when it was explicitly set.
func runBot(ctx context.Context, configPath string, debugFlag, debugFlagSet bool) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	debug := cfg.Observability.Debug
	if debugFlagSet {
		debug = debugFlag
	}
	log, sync := observability.NewLogger(debug)
	defer sync()

	solverNames := make([]string, len(cfg.Solvers))
	for i, s := range cfg.Solvers {
		solverNames[i] = s.Name
	}
	log.Info("vault-solver starting",
		"version", version.Version,
		"commit", version.Commit,
		"goVersion", version.GoVersion(),
		"solvers", solverNames,
		"debug", debug,
	)

	// Observability first, so probes/metrics are live during the rest of startup.
	metrics := observability.NewMetrics()
	metrics.SetBuildInfo(version.Version, version.Commit)
	health := &observability.Health{}
	httpSrv := observability.NewHTTPServer(cfg.Observability.Addr, metrics, health)
	go observability.ServeUntil(ctx, httpSrv, log)
	log.Info("observability server listening", "addr", cfg.Observability.Addr)

	// Chain client. rpcUrl is primary; rpcFallbackUrls (if any) are tried in order on failure.
	// writeRpcUrl (if set) is a separate client used only to broadcast transactions.
	rpcURLs := append([]string{cfg.Chain.RPCURL}, cfg.Chain.RPCFallbackURLs...)
	chainClient, err := chain.Dial(ctx, rpcURLs, cfg.Chain.WriteRPCURL, cfg.Chain.MulticallAddress, log)
	if err != nil {
		return err
	}
	defer chainClient.Close()
	if got := chainClient.ChainID().Uint64(); got != cfg.Chain.ChainID {
		return errors.Errorf("chain id mismatch: rpc reports %d, config says %d", got, cfg.Chain.ChainID)
	}

	// Signer.
	sgnr, err := signer.FromConfig(cfg.Signer)
	if err != nil {
		return err
	}
	log.Info("signer ready", "address", sgnr.Address().Hex())

	// Shared, nonce-serialized transaction sender.
	txm := txmanager.New(chainClient, sgnr, chainClient.ChainID(), txmanager.Config{
		Confirmations:       cfg.TxManager.Confirmations,
		MaxFeeGwei:          cfg.TxManager.MaxFeeGwei,
		TipGwei:             cfg.TxManager.TipGwei,
		ReplacementInterval: time.Duration(cfg.TxManager.ReplacementIntervalMs) * time.Millisecond,
		PendingTimeout:      time.Duration(cfg.TxManager.PendingTimeoutMs) * time.Millisecond,
	}, log)
	// Accepted transactions outlive solver intake cancellation: solvers first stop admitting
	// work and drain their pending results, then this deferred stop ends the shared tx manager.
	txCtx, stopTx := context.WithCancel(context.WithoutCancel(ctx))
	txDone := make(chan struct{})
	go func() {
		defer close(txDone)
		txm.Start(txCtx)
	}()
	defer func() {
		stopTx()
		<-txDone
	}()

	// Build every configured solver. They share the chain client, signer, and the single
	// nonce-serialized txManager — running multiple solver types in one process is exactly what the
	// shared txManager exists for, so they never race on nonces.
	deps := solver.Deps{Chain: chainClient, TxManager: txm, Signer: sgnr, Log: log, Metrics: metrics}
	solvers := make([]solver.Solver, 0, len(cfg.Solvers))
	for _, sc := range cfg.Solvers {
		slv, err := solver.New(sc.Name, sc.Config, deps)
		if err != nil {
			return err
		}
		solvers = append(solvers, slv)
	}

	health.SetReady(true)

	// Run all solvers concurrently. The first fatal error cancels the rest; ctx cancellation is a
	// clean shutdown (solver.Run maps context.Canceled to nil).
	var shutdownPreparationTimeout time.Duration
	for _, slv := range solvers {
		if preparer, ok := slv.(solver.ShutdownPreparer); ok {
			shutdownPreparationTimeout = max(
				shutdownPreparationTimeout,
				preparer.ShutdownPreparationTimeout(),
			)
		}
	}
	g, gctx := errgroup.WithContext(ctx)
	for _, slv := range solvers {
		g.Go(func() error { return solver.Run(gctx, slv, log) })
	}
	solversDone := make(chan struct{})
	drainMonitorDone := make(chan struct{})
	// The finite shutdown budget covers solver preparation, one pending-timeout window, and one
	// replacement interval. It bounds how long the process waits; with multiple pending nonces it
	// does not guarantee a cancellation attempt for every nonce.
	shutdownTimeout := shutdownPreparationTimeout + time.Duration(
		cfg.TxManager.PendingTimeoutMs+cfg.TxManager.ReplacementIntervalMs,
	)*time.Millisecond
	go func() {
		defer close(drainMonitorDone)
		monitorTransactionDrain(gctx.Done(), solversDone, shutdownTimeout, func() {
			log.Info("solver shutdown timed out; stopping tx manager", "timeout", shutdownTimeout.String())
			stopTx()
		})
	}()
	err = g.Wait()
	close(solversDone)
	<-drainMonitorDone
	return err
}

func monitorTransactionDrain(
	shutdown <-chan struct{},
	solversDone <-chan struct{},
	timeout time.Duration,
	onTimeout func(),
) {
	select {
	case <-shutdown:
	case <-solversDone:
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		onTimeout()
	case <-solversDone:
	}
}
