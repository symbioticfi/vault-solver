// Package rfq implements the Symbiotic RFQ filler solver: it serves backend quote requests off the
// on-chain per-vault LiquidLane adapters, and fills won orders via the Executor contract. It
// is constructed by the application catalog. See docs/RFQ-PLAN.md.
package rfq

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
)

const (
	// Name is the configuration key that selects this solver from config.
	Name                       = "rfq-filler"
	quoteServerShutdownTimeout = 5 * time.Second
	orderPollOperation         = "order_poll"
)

// Solver is the RFQ filler strategy.
type Solver struct {
	cfg         *Config
	server      *server
	exec        *executionService
	log         logr.Logger
	reportFatal func(error)
}

func New(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	secret := os.Getenv(cfg.BackendSharedSecretEnv)
	if secret == "" {
		return nil, errors.Errorf("%s: backend shared secret env %q is empty", Name, cfg.BackendSharedSecretEnv)
	}
	strategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}
	runtime := &Solver{cfg: cfg, log: deps.Log.WithName(Name), reportFatal: deps.ReportFatal}
	orders := newStore(time.Now)
	reader := newReader(deps.Chain, runtime.log, cfg.LiquidityLens)
	quoting, execution := buildServices(cfg, deps.Chain.ChainID().Int64(), orders, reader,
		deps.TxManager, deps.TxManager.LaneReady, strategy, runtime.log)
	runtime.exec = execution
	runtime.server = &server{sharedSecret: secret, quotes: quoting, log: runtime.log}
	if deps.Metrics != nil {
		metrics, err := newRFQMetrics(deps.Metrics.Registerer(), orders, cfg.Strategy.Name)
		if err != nil {
			return nil, err
		}
		runtime.server.metrics, execution.metrics, execution.orderPollObserver = metrics, metrics, metrics.orderPollObserver
	}
	return runtime, nil
}

// buildServices wires the quote and execution services from the parsed config and shared deps.
// Split from factory so the config → service wiring (notably the adapter whitelist reaching both
// services) is unit-testable without a chain client.
func buildServices(
	cfg *Config,
	chainID int64,
	st *store,
	rdr *reader,
	txm txSender,
	laneReady func() bool,
	quoteStrategy types.Strategy,
	log logr.Logger,
) (*quoteService, *executionService) {
	// The quote and execution paths scope to adapters independently. Quoting uses quoteScopesToAdapters()
	// so an internal-mode filler with configured adapters advertises quotes only for its own adapter
	// universe; execution uses restrictsToAdapters() (external-only) so internal-mode discount recovery can
	// still fill through any advertised adapter. Both predicates imply non-empty Adapters when true, so a
	// wired whitelist is never empty.
	quoteWhitelist := buildAdapterWhitelist(cfg.quoteScopesToAdapters(), cfg.Adapters)
	execWhitelist := buildAdapterWhitelist(cfg.restrictsToAdapters(), cfg.Adapters)

	quotes := &quoteService{
		chainID:      chainID,
		executor:     cfg.Executor,
		laneReady:    laneReady,
		whitelist:    quoteWhitelist,
		tokenPolicy:  cfg.TokenPolicy,
		minAmountsIn: cfg.MinAmountsIn,
		reader:       rdr,
		strategy:     quoteStrategy,
		log:          log,
		now:          time.Now,
	}
	exec := &executionService{
		chainID:          chainID,
		executor:         cfg.Executor,
		orderLimit:       cfg.OrderLimit,
		vaults:           cfg.Adapters,
		whitelist:        execWhitelist,
		tokenPolicy:      cfg.TokenPolicy,
		discountsEnabled: cfg.usesDiscounts(),
		backend:          newBackendClient(cfg.BackendURL),
		store:            st,
		reader:           rdr,
		strategy:         quoteStrategy,
		txm:              txm,
		log:              log,
		now:              time.Now,
	}
	return quotes, exec
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

func (s *Solver) ShutdownPreparationTimeout() time.Duration {
	return quoteServerShutdownTimeout
}

// Run serves the quote HTTP API until ctx is cancelled, then shuts it down gracefully, alongside the
// backend order-poll + fill loop. The filler is poll-only (no push/notify endpoint).
func (s *Solver) Run(ctx context.Context) error {
	// Resolve each recovery adapter's vault + collateral once at startup (config carries only adapter
	// addresses; both are fixed for the adapter's lifetime) and hand the resolved set to recovery. Runs
	// before the poll loop and the quote server, so there's no concurrent reader of exec.vaults. A
	// transport failure aborts startup; a per-adapter revert leaves that entry unresolved (skipped).
	if len(s.cfg.Adapters) > 0 {
		resolved, err := s.exec.reader.resolveVaults(ctx, s.cfg.Adapters)
		if err != nil {
			startupErr := errors.Errorf("rfq: resolve recovery vaults: %w", err)
			s.log.Error(startupErr, "adapter resolution failed",
				"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
			return startupErr
		}
		s.exec.vaults = resolved
		s.exec.reader.setQuoteAdapters(resolved)
		if s.cfg.restrictsToAdapters() {
			if err := s.exec.reader.validateDirectAuthorization(ctx, s.cfg.Executor, resolved); err != nil {
				startupErr := errors.Errorf("rfq: validate direct authorization: %w", err)
				s.log.Error(startupErr, "external adapter authorization failed",
					"solverMode", s.cfg.SolverMode, "executor", s.cfg.Executor.Hex(), "adapters", s.cfg.Adapters)
				return startupErr
			}
		}
	}

	s.log.Info("starting",
		"listenAddr", s.cfg.ListenAddr,
		"executor", s.cfg.Executor.Hex(),
		"solverMode", s.cfg.SolverMode,
		"adapters", len(s.cfg.Adapters),
		"backendUrl", s.cfg.BackendURL,
	)

	httpSrv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           s.server.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.cfg.ListenAddr)
	if err != nil {
		return errors.Errorf("rfq: listen for quotes: %w", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(listener) }()
	s.log.Info("quote server listening", "addr", listener.Addr().String())

	// Stop new polling on shutdown, but join the execution loop before returning. A txmanager Send
	// that reached admission still waits for the manager's terminal or bounded-shutdown result after
	// execCtx is cancelled, so this join preserves RFQ bookkeeping for already-accepted fills.
	execCtx, stopExec := context.WithCancel(ctx)
	execDone := make(chan struct{})
	go func() {
		defer close(execDone)
		s.exec.run(execCtx, s.cfg.PollInterval)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case err := <-errCh:
		errCh = nil
		runErr = errors.Errorf("rfq: quote server failed: %w", err)
		if s.reportFatal != nil && ctx.Err() == nil {
			s.reportFatal(runErr)
		}
	}
	stopExec()

	// Use a fresh bounded context because shutdown may follow parent cancellation.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), quoteServerShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		s.log.Error(err, "quote server graceful shutdown failed")
		if closeErr := httpSrv.Close(); closeErr != nil {
			s.log.Error(closeErr, "quote server forced shutdown failed")
		}
	}
	if errCh != nil {
		<-errCh
	}
	<-execDone
	return runErr
}
