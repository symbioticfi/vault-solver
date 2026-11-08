package main

import (
	"context"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	txmanager "github.com/symbioticfi/merkle-executor/sdk/transaction"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
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
func runBot(ctx context.Context, configPath string, debugFlag, debugFlagSet bool) (err error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	debug := cfg.Observability.Debug
	if debugFlagSet {
		debug = debugFlag
	}
	solverNames := make([]string, len(cfg.Solvers))
	for i, s := range cfg.Solvers {
		solverNames[i] = s.Name
	}

	log, syncLog := observability.NewLogger(debug)
	defer syncLog()

	shutdownTracing, tracingEnabled := observability.NewTracing(ctx, observability.Tracing{
		Version: version.Version,
		Commit:  version.Commit,
		Solvers: solverNames,
		ChainID: cfg.Chain.ChainID,
	}, log)
	defer func() { //nolint:contextcheck // fresh context on purpose: ctx is cancelled during shutdown and the flush must still run
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(flushCtx); err != nil {
			log.Info("tracing shutdown incomplete", "err", err.Error())
		}
	}()

	// With one integration worker, stamp every line, including common components, with its name.
	// With several workers, stamp each worker logger below and attribute manager work per request.
	if len(solverNames) == 1 {
		log = log.WithValues("solver", solverNames[0])
	}
	// Everything downstream logs through observability.Log(ctx), which stamps the trace ids on this
	// logger. The default covers the few paths that hold no context of ours.
	observability.SetDefaultLogger(log)
	ctx = observability.WithLogger(ctx, log)
	log.Info("vault-solver starting",
		"version", version.Version,
		"commit", version.Commit,
		"goVersion", version.GoVersion(),
		"solvers", solverNames,
		"debug", debug,
		"tracing", tracingEnabled,
	)

	// Observability first, so probes/metrics are live during the rest of startup.
	metrics, health := observability.NewMetrics()
	metrics.SetBuildInfo(version.Version, version.Commit)
	metrics.SetSolvers(solverNames)
	httpSrv := observability.NewHTTPServer(cfg.Observability.Addr, metrics)
	observabilityCtx, stopObservability := context.WithCancel(context.WithoutCancel(ctx))
	observabilityDone := make(chan struct{})
	go func() {
		defer close(observabilityDone)
		observability.ServeUntil(observabilityCtx, httpSrv, log)
	}()
	defer func() {
		stopObservability()
		<-observabilityDone
	}()
	log.Info("observability server listening", "addr", cfg.Observability.Addr)

	// Chain client. rpcUrl is primary; rpcFallbackUrls (if any) are tried in order on failure.
	// writeRpcUrl (if set) broadcasts transactions and supplies both startup nonce reads.
	rpcMetrics, err := chain.NewRPCMetrics(metrics.Registerer())
	if err != nil {
		return err
	}
	rpcURLs := append([]string{cfg.Chain.RPCURL}, cfg.Chain.RPCFallbackURLs...)
	chainClient, err := chain.DialWithMetrics(
		ctx, rpcURLs, cfg.Chain.WriteRPCURL, cfg.Chain.MulticallAddress,
		time.Duration(cfg.Chain.RPCAttemptTimeoutMs)*time.Millisecond, rpcMetrics,
	)
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

	// This solver process owns one signer and SDK transaction manager.
	txm, err := txmanager.New(chainClient, sgnr, chainClient.ChainID(), txmanager.Config{
		Confirmations: cfg.TxManager.Confirmations, MaxFeeGwei: cfg.TxManager.MaxFeeGwei, MaxGas: cfg.TxManager.MaxGas,
		PollInterval:        time.Duration(cfg.TxManager.PollIntervalMs) * time.Millisecond,
		ReplacementInterval: time.Duration(cfg.TxManager.ReplacementIntervalMs) * time.Millisecond,
		ShutdownTimeout:     time.Duration(cfg.TxManager.ShutdownTimeoutMs) * time.Millisecond,
	}, log)
	if err != nil {
		return errors.Errorf("create tx manager: %w", err)
	}
	runCtx, reportFatal := context.WithCancelCause(ctx)
	defer reportFatal(nil)

	// Build the configured integration workers using this process's signer and transaction manager.
	deps := solver.Deps{
		Chain: chainClient, TxManager: txm, MaxGas: cfg.TxManager.MaxGas, Signer: sgnr, Log: log, Metrics: metrics,
		ReportFatal: reportFatal,
	}
	solvers := make([]solver.Solver, 0, len(cfg.Solvers))
	solverLogs := make([]logr.Logger, 0, len(cfg.Solvers))
	// Registered before the sender drain defer: close persistent resources only
	// after accepted transaction lifecycles finish.
	var closers []io.Closer
	defer func() { err = errors.Join(err, closeSolvers(closers)) }()
	requiresTxManager := false
	for _, sc := range cfg.Solvers {
		solverDeps := deps
		if len(cfg.Solvers) > 1 {
			solverDeps.Log = log.WithValues("solver", sc.Name)
		}
		slv, err := solver.New(sc.Name, sc.Config, solverDeps)
		if err != nil {
			return err
		}
		if closer, ok := slv.(io.Closer); ok {
			closers = append(closers, closer)
		}
		solvers = append(solvers, slv)
		solverLogs = append(solverLogs, solverDeps.Log)
		requiresTxManager = requiresTxManager || solver.RequiresTxManager(slv)
	}
	if requiresTxManager {
		if err := cfg.ValidateTxManager(); err != nil {
			return errors.Errorf("invalid config %q: %w", configPath, err)
		}
		if err := txm.Initialize(runCtx); err != nil {
			return errors.Errorf("initialize tx manager: %w", err)
		}
	}

	// Worker cancellation and manager drain start together. Workers waiting for accepted results
	// can finish while the manager applies its bounded shutdown timeout.
	g, gctx := errgroup.WithContext(runCtx)
	if requiresTxManager {
		txDone := make(chan struct{})
		go func() { defer close(txDone); txm.Start(gctx) }()
		defer func() { reportFatal(nil); <-txDone }()
	}
	var background sync.WaitGroup
	laneAvailable := func() bool { return true }
	if requiresTxManager {
		laneAvailable = txm.Available
	}
	background.Go(func() {
		watchReadiness(gctx, time.Duration(cfg.TxManager.PollIntervalMs)*time.Millisecond, laneAvailable, health.SetReady)
	})
	for i, slv := range solvers {
		g.Go(func() error { return solver.Run(gctx, slv, solverLogs[i]) })
	}

	err = g.Wait()
	background.Wait()
	if err == nil {
		if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) {
			return cause
		}
	}
	return err
}

func watchReadiness(ctx context.Context, interval time.Duration, laneAvailable func() bool, setReady func(bool)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	last, known := false, false
	for {
		if ctx.Err() != nil {
			setReady(false)
			return
		}
		ready := laneAvailable()
		if !known || ready != last {
			setReady(ready)
			last, known = ready, true
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
		}
	}
}

func closeSolvers(closers []io.Closer) error {
	var failures []error
	for _, closer := range slices.Backward(closers) {
		if err := closer.Close(); err != nil {
			failures = append(failures, errors.Errorf("close solver resources: %w", err))
		}
	}
	return errors.Join(failures...)
}
