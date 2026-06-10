// Package rfq implements the Symbiotic RFQ filler solver: it serves backend quote requests off the
// on-chain per-vault LiquidLane adapters, and fills won orders via the Executor contract. It
// self-registers with the solver framework via init(). See docs/RFQ-PLAN.md.
package rfq

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
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

	chainID := deps.Chain.ChainID().Int64()
	log := deps.Log.WithName(Name)
	st := newStore(time.Now)
	rdr := newReader(deps.Chain, log)

	var metrics *httpMetrics
	if deps.Metrics != nil {
		if metrics, err = newHTTPMetrics(deps.Metrics.Registerer()); err != nil {
			return nil, err
		}
	}

	quotes, exec := buildServices(cfg, chainID, st, rdr, deps.TxManager, log)
	return &Solver{
		cfg:  cfg,
		exec: exec,
		server: &server{
			sharedSecret: secret,
			quotes:       quotes,
			metrics:      metrics,
			log:          log,
		},
		log: log,
	}, nil
}

// buildServices wires the quote and execution services from the parsed config and shared deps.
// Split from factory so the config → service wiring (notably the adapter whitelist reaching both
// services) is unit-testable without a chain client.
func buildServices(
	cfg *Config, chainID int64, st *store, rdr *reader, txm txSender, log logr.Logger,
) (*quoteService, *executionService) {
	// The whitelist is shared by quoting (drop non-whitelisted request adapters) and execution
	// (drop non-whitelisted recovery discounts); recovery's direct inventories come from cfg.Vaults
	// and are therefore whitelisted by construction.
	whitelist := buildAdapterWhitelist(cfg.AdapterWhitelistEnabled, cfg.Vaults)
	if cfg.AdapterWhitelistEnabled && len(cfg.Vaults) == 0 {
		log.Info("adapter whitelist is enabled with no vaults configured; every quote will be declined " +
			"(add vaults entries or set adapterWhitelistEnabled: false)")
	}

	quotes := &quoteService{
		chainID:   chainID,
		executor:  cfg.Executor,
		whitelist: whitelist,
		reader:    rdr,
		store:     st,
		log:       log,
		now:       time.Now,
	}
	exec := &executionService{
		chainID:    chainID,
		executor:   cfg.Executor,
		orderLimit: cfg.OrderLimit,
		vaults:     cfg.Vaults,
		whitelist:  whitelist,
		backend:    newBackendClient(cfg.BackendURL),
		store:      st,
		reader:     rdr,
		txm:        txm,
		log:        log,
		now:        time.Now,
		inflight:   make(map[string]bool),
	}
	return quotes, exec
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

// Run serves the quote HTTP API until ctx is cancelled, then shuts it down gracefully, alongside the
// backend order-poll + fill loop. The filler is poll-only (no push/notify endpoint).
func (s *Solver) Run(ctx context.Context) error {
	s.log.Info("starting",
		"listenAddr", s.cfg.ListenAddr,
		"executor", s.cfg.Executor.Hex(),
		"vaults", len(s.cfg.Vaults),
		"adapterWhitelistEnabled", s.cfg.AdapterWhitelistEnabled,
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
