package main

import (
	"context"
	"runtime"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
	"github.com/symbioticfi/vault-solver/internal/version"
)

func newRunCmd() *cobra.Command {
	var path string
	var debug bool
	command := &cobra.Command{
		Use: "run", Short: "Run the solver against the configured vaults", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runBot(command.Context(), path, debug, command.Flags().Changed("debug"))
		},
	}
	command.Flags().StringVar(&path, "config", "", "path to the YAML config file (required)")
	command.Flags().BoolVar(&debug, "debug", false, "enable debug logging (overrides observability.debug)")
	_ = command.MarkFlagRequired("config")
	return command
}

func runBot(ctx context.Context, path string, debugFlag, debugFlagSet bool) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	debug := cfg.Observability.Debug
	if debugFlagSet {
		debug = debugFlag
	}
	log, flushLog := observability.NewLogger(debug)
	defer flushLog()
	logStartup(log, cfg, debug)

	metrics := observability.NewMetrics(version.Version, version.Commit)
	metrics.SetSolvers(configuredSolverNames(cfg))
	stopObservability := startObservability(ctx, cfg.Observability.Addr, metrics, log)
	defer stopObservability()

	readURLs := append([]string{cfg.Chain.RPCURL}, cfg.Chain.RPCFallbackURLs...)
	rpcMetrics, err := chain.NewRPCMetrics(metrics.Registerer())
	if err != nil {
		return err
	}
	client, err := chain.DialWithMetrics(
		ctx, readURLs, cfg.Chain.WriteRPCURL, cfg.Chain.MulticallAddress, rpcMetrics, log,
	)
	if err != nil {
		return err
	}
	defer client.Close()
	if actual := client.ChainID().Uint64(); actual != cfg.Chain.ChainID {
		return errors.Errorf("chain id mismatch: rpc reports %d, config says %d", actual, cfg.Chain.ChainID)
	}

	signer, err := signer.FromConfig(cfg.Signer)
	if err != nil {
		return err
	}
	log.Info("signer ready", "address", signer.Address().Hex())
	txConfig := txmanager.Config{
		Confirmations:       cfg.TxManager.Confirmations,
		MaxFeeGwei:          cfg.TxManager.MaxFeeGwei,
		TipGwei:             cfg.TxManager.TipGwei,
		BroadcastTimeout:    time.Duration(cfg.TxManager.BroadcastTimeoutMs) * time.Millisecond,
		AccountPollInterval: time.Duration(cfg.TxManager.AccountPollIntervalMs) * time.Millisecond,
		ReplacementInterval: time.Duration(cfg.TxManager.ReplacementIntervalMs) * time.Millisecond,
		PendingTimeout:      time.Duration(cfg.TxManager.PendingTimeoutMs) * time.Millisecond,
		ShutdownTimeout:     time.Duration(cfg.TxManager.ShutdownTimeoutMs) * time.Millisecond,
	}
	txMetrics, err := txmanager.NewMetrics(metrics.Registerer())
	if err != nil {
		return err
	}
	manager := txmanager.NewWithMetrics(client, signer, client.ChainID(), txConfig, txMetrics, log)
	integrations, requiresLane, err := buildIntegrations(cfg.Solvers, app.Services{
		Chain: client, TxManager: manager, Signer: signer, Log: log, Metrics: metrics, Capacity: new(capacity.Book),
	})
	if err != nil {
		return err
	}
	return app.Run(ctx, app.RunConfig{
		ConfigSource: path, RequiresTransactionLane: requiresLane, ValidateTxConfig: cfg.ValidateTxManager,
		PendingTimeout: txConfig.PendingTimeout, ReplacementInterval: txConfig.ReplacementInterval,
	}, manager, integrations, metrics.SetReady, log)
}

func configuredSolverNames(cfg *config.Config) []string {
	names := make([]string, len(cfg.Solvers))
	for index, integration := range cfg.Solvers {
		names[index] = integration.Name
	}
	return names
}

func logStartup(log logr.Logger, cfg *config.Config, debug bool) {
	log.Info("vault-solver starting", "version", version.Version, "commit", version.Commit,
		"goVersion", runtime.Version(), "solvers", configuredSolverNames(cfg), "debug", debug)
}

func startObservability(
	parent context.Context,
	address string,
	metrics *observability.Metrics,
	log logr.Logger,
) func() {
	server := observability.NewHTTPServer(address, metrics)
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() { defer close(done); observability.ServeUntil(ctx, server, log) }()
	log.Info("observability server listening", "addr", address)
	return func() { cancel(); <-done }
}
