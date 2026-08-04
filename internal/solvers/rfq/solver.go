// Package rfq implements the Symbiotic RFQ filler solver: it serves backend quote requests off the
// on-chain per-vault LiquidLane adapters, and fills won orders via the Executor contract. It
// self-registers with the solver framework via init(). See docs/RFQ-PLAN.md.
package rfq

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"gopkg.in/yaml.v3"

	frameworksigner "github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/webhook"
)

// Name is the registry key that selects this solver from config.
const Name = "rfq-filler"

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, factory)
}

// Solver is the RFQ filler strategy.
type Solver struct {
	cfg    *Config
	server *server
	exec   *executionService
	swaps  *swapService
	log    logr.Logger
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	secret := os.Getenv(cfg.BackendSharedSecretEnv)
	if secret == "" {
		return nil, errors.Errorf("%s: backend shared secret env %q is empty", Name, cfg.BackendSharedSecretEnv)
	}
	if cfg.SwapEnabled && deps.Signer == nil {
		return nil, errors.Errorf("%s: framework signer is required when swapEnabled is true", Name)
	}

	chainID := deps.Chain.ChainID().Int64()
	log := deps.Log.WithName(Name)
	st := newStore(time.Now)
	rdr := newReader(deps.Chain, log, cfg.LiquidityLens)
	quoteStrategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}

	var metrics *httpMetrics
	if deps.Metrics != nil {
		if metrics, err = newHTTPMetrics(deps.Metrics.Registerer()); err != nil {
			return nil, err
		}
	}

	backend := newBackendClient(cfg.BackendURL)
	quotes, exec, swaps := buildServicesWithSwap(
		cfg, chainID, st, rdr, deps.TxManager, quoteStrategy, backend, deps.Signer, log,
	)
	return &Solver{
		cfg:   cfg,
		exec:  exec,
		swaps: swaps,
		server: &server{
			sharedSecret: secret,
			quotes:       quotes,
			swaps:        swaps,
			metrics:      metrics,
			log:          log,
		},
		log: log,
	}, nil
}

// buildServices wires the quote and execution services from the parsed config and shared deps.
// Split from factory so the config → service wiring (notably the adapter whitelist reaching both
// services) is unit-testable without a chain client.
//
//nolint:unparam // tests intentionally exercise the stage deployment's chain ID while production uses buildServicesWithSwap.
func buildServices(
	cfg *Config, chainID int64, st *store, rdr *reader, txm txSender, quoteStrategy types.Strategy, log logr.Logger,
) (*quoteService, *executionService) {
	quotes, exec, _ := buildServicesWithSwap(
		cfg, chainID, st, rdr, txm, quoteStrategy, newBackendClient(cfg.BackendURL), nil, log,
	)
	return quotes, exec
}

func buildServicesWithSwap(
	cfg *Config,
	chainID int64,
	st *store,
	rdr *reader,
	txm txSender,
	quoteStrategy types.Strategy,
	backend *backendClient,
	signer frameworksigner.Signer,
	log logr.Logger,
) (*quoteService, *executionService, *swapService) {
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
		backend:          backend,
		store:            st,
		reader:           rdr,
		strategy:         quoteStrategy,
		txm:              txm,
		log:              log,
		now:              time.Now,
		inflight:         make(map[string]bool),
	}
	if !cfg.SwapEnabled {
		return quotes, exec, nil
	}
	var state swapStateReader
	if rdr != nil {
		state = rdr.swapState
	}
	swaps := &swapService{
		chainID:          chainID,
		executor:         cfg.Executor,
		router:           cfg.Router,
		quoteTTL:         cfg.SwapQuoteTTL,
		whitelist:        quoteWhitelist,
		tokenPolicy:      cfg.TokenPolicy,
		minAmountsIn:     cfg.MinAmountsIn,
		discountsEnabled: cfg.usesDiscounts(),
		reader:           rdr,
		state:            state,
		strategy:         quoteStrategy,
		store:            newSwapStore(time.Now),
		signer:           signer,
		now:              time.Now,
		newID:            uuid.New,
		log:              log,
	}
	return quotes, exec, swaps
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

// Run serves the quote HTTP API until ctx is cancelled, then shuts it down gracefully, alongside the
// backend order-poll + fill loop. The filler is poll-only (no push/notify endpoint).
func (s *Solver) Run(ctx context.Context) error {
	if s.swaps != nil {
		if s.swaps.state == nil {
			return errors.New("rfq: swap state reader is unavailable")
		}
		if err := s.swaps.state.validateRouter(ctx, s.cfg.Router); err != nil {
			startupErr := errors.Errorf("rfq: validate swap Router: %w", err)
			s.log.Error(startupErr, "swap Router validation failed", "router", s.cfg.Router.Hex())
			return startupErr
		}
		if s.swaps.signer == nil {
			return errors.New("rfq: swap signer is unavailable")
		}
		adapters := make([]common.Address, len(s.cfg.Adapters))
		for i := range s.cfg.Adapters {
			adapters[i] = s.cfg.Adapters[i].Adapter
		}
		if len(adapters) > 0 {
			if _, err := s.swaps.state.validateAdapters(ctx, adapters, s.swaps.signer.Address()); err != nil {
				startupErr := errors.Errorf("rfq: validate swap adapters: %w", err)
				s.log.Error(startupErr, "swap adapter validation failed", "router", s.cfg.Router.Hex(),
					"adapters", adapters)
				return startupErr
			}
		}
	}

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
	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	s.log.Info("quote server listening", "addr", s.cfg.ListenAddr)

	// Backend order poll + fill loop (P2). Stops when ctx is cancelled.
	go s.exec.run(ctx, s.cfg.PollInterval)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return errors.Errorf("rfq: quote server failed: %w", err)
	}

	// Fresh context: the parent is already cancelled, so deriving from it would abort the drain.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx) //nolint:contextcheck // fresh deadline for post-cancellation drain
	return ctx.Err()
}
