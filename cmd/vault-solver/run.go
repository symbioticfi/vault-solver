package main

import (
	"context"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"

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

	log.Info("vault-solver starting",
		"version", version.Version,
		"commit", version.Commit,
		"goVersion", version.GoVersion(),
		"solver", cfg.Solver.Name,
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
	rpcURLs := append([]string{cfg.Chain.RPCURL}, cfg.Chain.RPCFallbackURLs...)
	chainClient, err := chain.Dial(ctx, rpcURLs, cfg.Chain.MulticallAddress, log)
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
		Confirmations: cfg.TxManager.Confirmations,
		MaxFeeGwei:    cfg.TxManager.MaxFeeGwei,
		TipGwei:       cfg.TxManager.TipGwei,
	}, log)
	go txm.Start(ctx)

	// Select and build the configured solver.
	slv, err := solver.New(cfg.Solver.Name, cfg.Solver.Config, solver.Deps{
		Chain:     chainClient,
		TxManager: txm,
		Signer:    sgnr,
		Log:       log,
		Metrics:   metrics,
	})
	if err != nil {
		return err
	}

	health.SetReady(true)
	return solver.Run(ctx, slv, log)
}
