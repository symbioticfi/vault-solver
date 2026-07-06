// Package bridgefacilitator implements the 3F (Grunt) Bridge Facilitator solver: it discovers
// bridge-loan auctions via the 3F API, snapshots adapter state for a trusted strategy, signs the
// returned offers, and realizes repaid loans back into the vault. It self-registers with the solver
// framework via init().
package bridgefacilitator

import (
	"context"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

// offerStatusIgnored are 3F offer statuses that are not live coverage when rebuilding the cache: a
// FAILED consume or a NOT_ACCEPTED bid won't cover the auction, so discovery should re-offer.
var offerStatusIgnored = map[string]bool{
	"FAILED":       true,
	"NOT_ACCEPTED": true,
}

// Name is the registry key that selects this solver from config.
const Name = "3f-bridge-facilitator"

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, factory)
}

// Solver owns the 3F Bridge Facilitator lifecycle and delegates offer decisions to strategy.
type Solver struct {
	cfg        *Config
	deps       solver.Deps
	api        *apiClient
	reader     *reader
	strategy   types.Strategy
	log        logr.Logger
	signerAddr common.Address // the solver's own EIP-1271 signer address, set in factory
	nonceSeq   atomic.Uint64
	offers     *offerTracker // dedup: (adapter, auction) pairs we hold a live offer for (Run goroutine only)
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}

	api := newAPIClient(cfg.APIBaseURL, deps.Signer, deps.Chain.ChainID(), cfg.HTTPTimeout, deps.Log.WithName(Name))
	offerStrategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}

	s := &Solver{
		cfg:        cfg,
		deps:       deps,
		api:        api,
		reader:     newReader(deps.Chain),
		strategy:   offerStrategy,
		log:        deps.Log.WithName(Name),
		signerAddr: deps.Signer.Address(),
		offers:     newOfferTracker(),
	}
	// Seed the offer nonce sequence from the wall clock so it stays monotonic across restarts.
	s.nonceSeq.Store(uint64(time.Now().UnixNano()))
	return s, nil
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

// Run drives discovery/offer, redemption, and reconciliation on their configured cadences until
// ctx is cancelled.
func (s *Solver) Run(ctx context.Context) error {
	// Resolve every adapter's vault/collateral and drop any for which this solver is not the
	// authorised EIP-1271 signer (see resolveTargets).
	if err := s.resolveTargets(ctx); err != nil {
		return err
	}

	s.log.Info("starting",
		"adapters", len(s.cfg.Targets),
		"apiBaseUrl", s.cfg.APIBaseURL,
		"discover", s.cfg.Intervals.Discover.String(),
	)

	// Best-effort at startup: load existing offers so a restart doesn't re-offer where we already hold
	// a live offer. Per-adapter failures are logged and skipped; a missing entry costs at most one
	// redundant, bounded-safe offer. There is no redeem-only mode — startup either kept ≥1 matching
	// adapter (above) and runs offers + redeems, or resolveTargets already shut the solver down.
	s.rebuildOfferCache(ctx)

	discoverT := time.NewTicker(s.cfg.Intervals.Discover)
	redeemT := time.NewTicker(s.cfg.Intervals.RedeemPoll)
	reconcileT := time.NewTicker(s.cfg.Intervals.Reconcile)
	defer discoverT.Stop()
	defer redeemT.Stop()
	defer reconcileT.Stop()

	// Run one pass immediately rather than waiting a full interval.
	s.discoverAndOffer(ctx)
	s.redeemAll(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-discoverT.C:
			s.discoverAndOffer(ctx)
		case <-redeemT.C:
			s.redeemAll(ctx)
		case <-reconcileT.C:
			s.reconcile(ctx)
		}
	}
}

// rebuildOfferCache records each adapter's still-unexpired offers so discovery skips auctions we
// already cover. Best-effort: a per-adapter list failure is logged and skipped so one bad adapter
// can't blank the others' caches.
func (s *Solver) rebuildOfferCache(ctx context.Context) {
	now := time.Now()
	live := 0
	for _, t := range s.cfg.Targets {
		offers, err := s.api.listOffers(ctx, t.Adapter)
		if err != nil {
			s.log.Error(err, "rebuild offer cache: list offers", "adapter", t.Adapter.Hex())
			continue
		}
		for _, o := range offers {
			if offerStatusIgnored[strings.ToUpper(strings.TrimSpace(o.Status))] {
				continue // failed/not-accepted offers aren't live coverage — let discovery re-offer
			}
			exp, perr := parseUnixTime(o.Expiration)
			if perr != nil || !exp.After(now) {
				continue // unparseable or already expired — we may freely re-offer
			}
			principal, ok := new(big.Int).SetString(o.Amount, 10)
			if !ok {
				s.log.V(1).Info("offer cache: unparseable amount; coverage may undercount",
					"adapter", t.Adapter.Hex(), "amount", o.Amount)
				principal = new(big.Int)
			}
			s.offers.record(t.Adapter, int64(o.AuctionId), exp, principal)
			live++
		}
	}
	s.log.Info("loaded existing offers into dedup cache", "live", live)
}

// adapterOffering tracks one adapter's liquidity/exposure snapshot for one offer pass.
type adapterOffering struct {
	target Target
	st     exposureState
}

// discoverAndOffer lists open auctions, snapshots adapter liquidity/exposure once, delegates offer
// selection to the configured strategy, then signs and submits the returned execution offers.
func (s *Solver) discoverAndOffer(ctx context.Context) {
	auctions, err := s.api.listAuctions(ctx)
	if err != nil {
		s.log.Error(err, "discover: list auctions")
		return
	}
	s.log.V(1).Info("discovered auctions", "count", len(auctions))

	offerings := make([]*adapterOffering, 0, len(s.cfg.Targets))
	for _, t := range s.cfg.Targets {
		st, lerr := s.reader.liquidityAndExposure(ctx, t.Adapter)
		if lerr != nil {
			s.log.Error(lerr, "offer: liquidity/exposure", "adapter", t.Adapter.Hex())
			continue
		}
		s.log.V(1).Info("adapter liquidity",
			"adapter", t.Adapter.Hex(), "fundable", st.fundable.String(), "openRequests", st.openCount,
			"maxAssets", st.maxAssets.String(), "minAssets", st.minAssets.String(),
			"minYieldBps", st.minYieldBps.String())
		offerings = append(offerings, &adapterOffering{target: t, st: st})
	}
	if len(offerings) == 0 {
		return // every adapter's liquidity read failed this pass
	}

	now := time.Now()
	s.offers.pruneExpired(now) // keep the dedup map bounded
	input := buildStrategyInput(auctions, offerings, s.offers, now)
	if len(input.Candidates) == 0 {
		return
	}
	out, err := s.strategy.DecideOffers(ctx, input)
	if err != nil {
		s.log.Error(err, "offer: strategy")
		return
	}
	auctionByID := auctionViewsByID(auctions)
	for _, offer := range out.Offers {
		av, ok := auctionByID[offer.AuctionID]
		if !ok {
			s.log.Error(errors.Errorf("auction %d not found", offer.AuctionID), "offer: build")
			continue
		}
		dto, buildErr := s.buildSignedOffer(av, offer)
		if buildErr != nil {
			s.log.Error(buildErr, "offer: build", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}
		if subErr := s.api.createOffer(ctx, dto); subErr != nil {
			s.log.Error(subErr, "offer: submit", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}

		if exp, perr := parseUnixTime(dto.Expiration); perr == nil {
			s.offers.record(offer.Maker, offer.AuctionID, exp, offer.Principal)
		}
		s.log.Info("offer submitted", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex(),
			"request", offer.Request.Hex(), "principal", offer.Principal.String(), "expectedReturn", dto.ExpectedReturn)
	}
}

// redeemAll runs the redeemer for every matched adapter.
func (s *Solver) redeemAll(ctx context.Context) {
	for _, t := range s.cfg.Targets {
		s.redeemReady(ctx, t)
	}
}

// reconcile reports each adapter's live open-position set — a stateless health/observability tick.
func (s *Solver) reconcile(ctx context.Context) {
	for _, t := range s.cfg.Targets {
		st, err := s.reader.liquidityAndExposure(ctx, t.Adapter)
		if err != nil {
			s.log.Error(err, "reconcile", "adapter", t.Adapter.Hex())
			continue
		}
		s.log.Info("reconcile", "adapter", t.Adapter.Hex(),
			"openRequests", st.openCount, "fundable", st.fundable.String())
	}
}

// nextNonce returns a strictly-increasing offer nonce.
func (s *Solver) nextNonce() uint64 {
	return s.nonceSeq.Add(1)
}

// resolveTargets resolves every adapter's vault, collateral, and EIP-1271 signer at startup (two
// batched Multicalls via reader.resolveAdapters) and keeps only the adapters that resolved and have
// this solver as their on-chain offerSigner — the rest are dropped with a warning. If none remain,
// it returns a startup error.
func (s *Solver) resolveTargets(ctx context.Context) error {
	adapters := make([]common.Address, len(s.cfg.Targets))
	for i := range s.cfg.Targets {
		adapters[i] = s.cfg.Targets[i].Adapter
	}
	resolved, err := s.reader.resolveAdapters(ctx, adapters)
	if err != nil {
		return err // whole-batch transport/RPC failure, not a per-adapter revert
	}

	kept := make([]Target, 0, len(s.cfg.Targets))
	for i, t := range s.cfg.Targets {
		r := resolved[i]
		if r.err != nil {
			s.log.Error(r.err, "skipping adapter: resolution failed", "adapter", t.Adapter.Hex())
			continue
		}
		if r.signer != s.signerAddr {
			s.log.Info("skipping adapter: solver is not its EIP-1271 signer",
				"adapter", t.Adapter.Hex(),
				"want", s.signerAddr.Hex(),
				"got", r.signer.Hex())
			continue
		}
		t.Vault, t.Collateral = r.vault, r.collateral
		s.log.Info("resolved target",
			"adapter", t.Adapter.Hex(), "vault", r.vault.Hex(), "collateral", r.collateral.Hex())
		kept = append(kept, t)
	}

	s.cfg.Targets = kept
	if len(s.cfg.Targets) == 0 {
		return errors.Errorf("no configured adapter passed startup validation (must resolve and have this solver as its EIP-1271 signer, want %s); see per-adapter warnings above", s.signerAddr.Hex())
	}
	return nil
}
