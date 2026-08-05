package main

import (
	"context"
	"sync"
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
	log, syncLog := observability.NewLogger(debug)
	defer syncLog()

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
	// writeRpcUrl (if set) broadcasts transactions and supplies both startup nonce reads.
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
	runCtx, reportFatal := context.WithCancelCause(ctx)
	defer reportFatal(nil)

	// Build every configured solver. Transaction-sending solvers share the single nonce-serialized
	// txManager so they never race on nonces.
	deps := solver.Deps{
		Chain: chainClient, TxManager: txm, Signer: sgnr, Log: log, Metrics: metrics,
		ReportFatal: reportFatal,
	}
	solvers := make([]solver.Solver, 0, len(cfg.Solvers))
	requiresTxManager := false
	for _, sc := range cfg.Solvers {
		slv, err := solver.New(sc.Name, sc.Config, deps)
		if err != nil {
			return err
		}
		solvers = append(solvers, slv)
		requiresTxManager = requiresTxManager || solver.RequiresTxManager(slv)
	}
	if requiresTxManager {
		if err := cfg.ValidateTxManager(); err != nil {
			return errors.Errorf("invalid config %q: %w", configPath, err)
		}
		if err := txm.ValidateFeeHeadroom(); err != nil {
			return errors.Errorf("invalid config %q: txManager: %w", configPath, err)
		}
		if err := txm.Initialize(runCtx); err != nil {
			return errors.Errorf("initialize tx manager: %w", err)
		}
	}

	health.SetReady(true)

	// Run all solvers concurrently. The first fatal error cancels the rest; ctx cancellation is a
	// clean shutdown (solver.Run maps context.Canceled to nil).
	g, gctx := errgroup.WithContext(runCtx)
	var background sync.WaitGroup
	if requiresTxManager {
		background.Go(func() { txm.Start(gctx) })
	}
	background.Go(func() {
		watchReadiness(gctx, txm.AvailabilityChanged(), txm.Available, health.SetReady)
	})
	for _, slv := range solvers {
		g.Go(func() error { return solver.Run(gctx, slv, log) })
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

func watchReadiness(
	ctx context.Context,
	availabilityChanged <-chan struct{},
	available func() bool,
	setReady func(bool),
) {
	for {
		select {
		case <-availabilityChanged:
			setReady(available())
		case <-ctx.Done():
			setReady(false)
			return
		}
	}
}
