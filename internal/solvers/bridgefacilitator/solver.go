// Package bridgefacilitator implements the 3F (Grunt) Bridge Facilitator solver: it discovers
// bridge-loan auctions via the 3F API, prices and signs offers sized to vault liquidity and the
// adapter's on-chain policy, and realizes repaid loans back into the vault. It self-registers with
// the solver framework via init().
package bridgefacilitator

import (
	"context"
	"math/big"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

// Name is the registry key that selects this solver from config.
const Name = "3f-bridge-facilitator"

//nolint:gochecknoinits // self-registration with the solver framework is the intended plugin pattern.
func init() {
	solver.Register(Name, factory)
}

// Solver is the 3F Bridge Facilitator strategy.
type Solver struct {
	cfg       *Config
	deps      solver.Deps
	api       *apiClient
	reader    *reader
	log       logr.Logger
	nonceSeq  atomic.Uint64
	onboarded bool          // set once the 3F API key + offer-address are in place (Run goroutine only)
	offers    *offerTracker // dedup: auctions we hold a live offer for (Run goroutine only)
}

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}

	var fallbackKey string
	if cfg.APIKeyEnv != "" {
		fallbackKey = os.Getenv(cfg.APIKeyEnv)
		if fallbackKey == "" {
			return nil, errors.Errorf("%s: api key env %q is empty", Name, cfg.APIKeyEnv)
		}
	}
	// The facilitator (API-key owner) is the signing EOA; offers carry the per-target adapter as
	// maker, registered as the facilitator offer-address at startup.
	api, err := newAPIClient(cfg.APIBaseURL, deps.Signer, deps.Signer.Address(), fallbackKey, deps.Log.WithName(Name))
	if err != nil {
		return nil, err
	}

	s := &Solver{
		cfg:    cfg,
		deps:   deps,
		api:    api,
		reader: newReader(deps.Chain),
		log:    deps.Log.WithName(Name),
		offers: newOfferTracker(),
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
	s.log.Info("starting",
		"vault", s.cfg.Target.Vault.Hex(),
		"adapter", s.cfg.Target.Adapter.Hex(),
		"apiBaseUrl", s.cfg.APIBaseURL,
		"discover", s.cfg.Intervals.Discover.String(),
	)

	// Resolve the target vault's collateral on-chain; auctions are matched to it by deposit
	// asset == collateral. If collateral can't be read it's left zero and matches no auction
	// (logged per-auction), but redemption/reconciliation still run.
	s.resolveCollateral(ctx)

	// Onboard once at startup. On failure the bot runs redeem-only (offers disabled) until restart;
	// redemption and reconciliation are on-chain and need no API auth.
	if err := s.onboard(ctx); err != nil {
		s.log.Error(err, "3F onboarding failed; running redeem-only (offers disabled until restart)")
	}

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

// onboard ensures the 3F API key and offer-address registration are in place. It runs once at
// startup and sets s.onboarded, which gates discovery/offers. Mid-run key expiry is handled
// separately by apiClient.withAuth (regenerate + retry on 401/403), so onboard never needs to rerun.
func (s *Solver) onboard(ctx context.Context) error {
	if err := s.api.ensureKey(ctx); err != nil {
		return errors.Errorf("api key: %w", err)
	}
	if err := s.ensureOfferAddress(ctx); err != nil {
		return err
	}
	// Rebuild the offer-dedup cache from the API so a restart doesn't re-offer on auctions we
	// already hold live offers for. Non-fatal: an empty cache just risks one redundant (bounded-safe)
	// offer per auction.
	if err := s.rebuildOfferCache(ctx); err != nil {
		s.log.Error(err, "could not load existing offers; starting with an empty offer cache")
	}
	s.onboarded = true
	return nil
}

// rebuildOfferCache loads the facilitator's outstanding offers (a single API call covering its
// broker address and its configured offer-address, i.e. our adapter) and records the expiration of
// each still-unexpired one, so discovery skips auctions we already cover.
func (s *Solver) rebuildOfferCache(ctx context.Context) error {
	now := time.Now()
	offers, err := s.api.listOffers(ctx)
	if err != nil {
		return err
	}
	live := 0
	for _, o := range offers {
		exp, perr := parseUnixTime(o.Expiration)
		if perr != nil || !exp.After(now) {
			continue // unparseable or already expired — we may freely re-offer
		}
		s.offers.record(int64(o.AuctionId), exp)
		live++
	}
	s.log.Info("loaded existing offers into dedup cache", "live", live)
	return nil
}

// discoverAndOffer lists auctions and offers per target. It runs only when onboarding succeeded; in
// redeem-only mode (onboarding failed at startup) it is a no-op — redemption and reconciliation run
// independently, since they're on-chain only.
func (s *Solver) discoverAndOffer(ctx context.Context) {
	if !s.onboarded {
		s.log.V(1).Info("not onboarded; skipping discovery/offers (redeem-only mode)")
		return
	}
	auctions, err := s.api.listAuctions(ctx)
	if err != nil {
		s.log.Error(err, "discover: list auctions")
		return
	}

	s.log.V(1).Info("discovered auctions", "count", len(auctions), "auctions", auctions)
	s.offerForTarget(ctx, s.cfg.Target, auctions)
}

// offerForTarget reads the target's vault/adapter liquidity and exposure once, then bids on each
// matching auction. `committed`/`opened` accumulate this pass's offers so successive bids see the
// reduced capacity — preserving the no-over-commit guarantee without re-reading per auction.
func (s *Solver) offerForTarget(ctx context.Context, target Target, auctions []threef.AuctionDto) {
	// One multicall fetches fundable liquidity, live exposure, and the open-loan count together.
	fundable, outstanding, openCount, err := s.reader.liquidityAndExposure(ctx, target.Vault, target.Adapter)
	if err != nil {
		s.log.Error(err, "offer: liquidity/exposure", "adapter", target.Adapter.Hex())
		return
	}
	s.log.V(1).Info("target liquidity",
		"adapter", target.Adapter.Hex(), "vault", target.Vault.Hex(),
		"fundable", fundable.String(), "outstanding", outstanding.String(), "openLoans", openCount)

	committed := new(big.Int)
	opened := 0
	now := time.Now()
	s.offers.pruneExpired(now) // drop expired offer-tracking entries so the map stays bounded
	for i := range auctions {
		av := auctionView{auctions[i]}
		auctionID := int64(av.dto.Id)

		// Asset gate: the auction's deposit asset must equal this target vault's collateral.
		if !av.matchesAsset(target.Collateral) {
			s.log.V(1).Info("skip auction: deposit asset != target collateral", "auctionId", auctionID,
				"depositAsset", av.depositAsset(), "collateral", target.Collateral.Hex())
			continue
		}
		// From here on the auction concerns this target, so decisions are logged at info for visibility.
		if !av.isOpen() {
			s.log.Info("skip auction: status not open/solvable", "auctionId", auctionID, "status", av.dto.Status)
			continue
		}

		request := av.requestAddr()
		if request == (common.Address{}) {
			s.log.Info("skip auction: missing/invalid requestId", "auctionId", auctionID, "requestId", av.dto.RequestId)
			continue
		}

		// Skip auctions we already hold a live (unexpired) offer for. Expired entries fall through so
		// we re-offer.
		if s.offers.hasLive(auctionID, now) {
			s.log.Info("skip auction: live offer already outstanding", "auctionId", auctionID)
			continue
		}

		principal, ok := sizeOffer(sizeInputs{
			perRequestMax:   target.Exposure.PerRequestMax,
			fundable:        new(big.Int).Sub(fundable, committed),
			amountRequested: av.amountRequested(),
			sleeveMax:       target.Exposure.TotalSleeveMax,
			outstanding:     new(big.Int).Add(outstanding, committed),
			openCount:       openCount + opened,
			maxConcurrent:   target.Exposure.MaxConcurrentLoans,
		})
		if !ok {
			s.log.Info("skip auction: not biddable under policy/liquidity", "auctionId", auctionID,
				"request", request.Hex(), "fundableRemaining", new(big.Int).Sub(fundable, committed).String(),
				"openLoans", openCount+opened, "maxConcurrent", target.Exposure.MaxConcurrentLoans)
			continue
		}

		// The adapter configured for this target is the offer maker (validated via EIP-1271).
		dto, bid, buildErr := s.buildSignedOffer(av, request, target.Adapter, principal)
		if buildErr != nil {
			s.log.Error(buildErr, "offer: build", "request", request.Hex())
			continue
		}
		if !bid {
			s.log.Info("skip auction: rate below configured return floor", "auctionId", auctionID,
				"request", request.Hex(), "maxRateBps", av.maxRate(), "minReturnBps", s.cfg.MinReturnBps)
			continue
		}
		if subErr := s.api.createOffer(ctx, dto); subErr != nil {
			s.log.Error(subErr, "offer: submit", "request", request.Hex())
			continue
		}

		committed.Add(committed, principal)
		opened++
		// Record so we don't re-offer until this offer expires.
		if exp, perr := parseUnixTime(dto.Expiration); perr == nil {
			s.offers.record(auctionID, exp)
		}
		s.log.Info("offer submitted",
			"request", request.Hex(), "principal", principal.String(), "expectedReturn", dto.ExpectedReturn)
	}
}

// ensureOfferAddress makes the 3F-registered facilitator offer-address match our maker (the target
// adapter). The offer-address is a facilitator-level singleton — the reason this solver serves a
// single vault+adapter pair. Read/write failures are returned so the caller can degrade to
// redeem-only.
func (s *Solver) ensureOfferAddress(ctx context.Context) error {
	desired := s.cfg.Target.Adapter
	current, err := s.api.offerAddress(ctx)
	if err != nil {
		return errors.Errorf("read offer-address: %w", err)
	}
	if current == desired {
		return nil
	}
	if err := s.api.setOfferAddress(ctx, desired); err != nil {
		return errors.Errorf("set offer-address to %s: %w", desired.Hex(), err)
	}
	s.log.Info("registered facilitator offer-address", "offerAddress", desired.Hex(), "previous", current.Hex())
	return nil
}

// redeemAll runs the redeemer for the configured target.
func (s *Solver) redeemAll(ctx context.Context) {
	s.redeemReady(ctx, s.cfg.Target)
}

// reconcile reports the live open-position set — a stateless health/observability tick.
func (s *Solver) reconcile(ctx context.Context) {
	target := s.cfg.Target
	_, outstanding, openCount, err := s.reader.liquidityAndExposure(ctx, target.Vault, target.Adapter)
	if err != nil {
		s.log.Error(err, "reconcile", "adapter", target.Adapter.Hex())
		return
	}
	s.log.Info("reconcile", "adapter", target.Adapter.Hex(),
		"openLoans", openCount, "outstandingPrincipal", outstanding.String())
}

// nextNonce returns a strictly-increasing offer nonce.
func (s *Solver) nextNonce() uint64 {
	return s.nonceSeq.Add(1)
}

// resolveCollateral reads the target vault's collateral token on-chain and caches it on the Target,
// so discovery can match auctions by deposit asset. A read failure leaves Collateral zero (no auction
// then matches) but doesn't stop redemption/reconciliation.
func (s *Solver) resolveCollateral(ctx context.Context) {
	t := &s.cfg.Target
	collateral, err := s.reader.vaultAsset(ctx, t.Vault)
	if err != nil {
		s.log.Error(err, "resolve collateral; will match no auctions until restart", "vault", t.Vault.Hex())
		return
	}
	t.Collateral = collateral
	s.log.Info("resolved target collateral",
		"vault", t.Vault.Hex(), "adapter", t.Adapter.Hex(), "collateral", collateral.Hex())
}
