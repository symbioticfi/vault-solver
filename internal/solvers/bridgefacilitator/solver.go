// Package bridgefacilitator implements the 3F (Grunt) Bridge Facilitator solver: it discovers
// bridge-loan auctions via the 3F API, snapshots adapter state for a trusted strategy, signs the
// returned offers, and realizes repaid loans back into the vault. Construction is explicit in internal/app.
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

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

// offerStatusIgnored are 3F offer statuses that are not live coverage when reconciling the cache: a
// FAILED consume, a NOT_ACCEPTED bid, or a CANCELLED offer won't cover the auction, so discovery should
// re-offer.
var errRequiredFieldMissing = errors.New("required field missing from API response")

var offerStatusIgnored = map[string]bool{
	"FAILED":       true,
	"NOT_ACCEPTED": true,
	"CANCELLED":    true,
	"CANCELED":     true,
}

// Name is the configuration key that selects this solver from config.
const Name = "3f-bridge-facilitator"

// Solver owns the 3F Bridge Facilitator lifecycle and delegates offer decisions to strategy.
type Solver struct {
	cfg         *Config
	offerSigner signer.Signer
	api         *apiClient
	reader      *reader
	txManager   transactionSender
	strategy    types.Strategy
	log         logr.Logger
	laneReady   func() bool    // shared txmanager lane state; safe for the single Run goroutine
	signerAddr  common.Address // the solver's own signer address (diagnostics only), set in factory
	probe       signerProbe    // one-time (hash, sig) used to validate offer-signer authorization, set in factory
	nonceSeq    atomic.Uint64
	offers      *offerTracker // dedup: (adapter, auction) pairs we hold a live offer for (Run goroutine only)
	targets     []Target      // current resolved snapshot; owned exclusively by the Run goroutine
	// targetsAuthoritative records whether targets covers the complete configured/discovered source.
	// A partial refresh still installs its safe subset, but derived metric freshness must stay retained.
	targetsAuthoritative bool
	metrics              *threeFMetrics
	operations           threeFOperationObservers
}

func deduplicateAdapters(adapters []common.Address) []common.Address {
	unique := make([]common.Address, 0, len(adapters))
	seen := make(map[common.Address]struct{}, len(adapters))
	for _, adapter := range adapters {
		if _, ok := seen[adapter]; ok {
			continue
		}
		seen[adapter] = struct{}{}
		unique = append(unique, adapter)
	}
	return unique
}

func New(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}

	api := newAPIClient(cfg.APIBaseURL, deps.Signer, deps.Chain.ChainID(), cfg.HTTPTimeout)
	offerStrategy, err := newStrategy(cfg.Strategy)
	if err != nil {
		return nil, err
	}

	probe, err := newSignerProbe(deps.Signer)
	if err != nil {
		return nil, err
	}
	var (
		metrics    *threeFMetrics
		operations threeFOperationObservers
	)
	if deps.Metrics != nil {
		metrics, err = newThreeFMetrics(deps.Metrics.Registerer(), cfg.Strategy.Name)
		if err != nil {
			return nil, err
		}
		operations = metrics.operations
	}

	s := &Solver{
		cfg:         cfg,
		offerSigner: deps.Signer,
		api:         api,
		reader:      newReader(deps.Chain, cfg.LiquidityLens),
		txManager:   deps.TxManager,
		strategy:    offerStrategy,
		log:         deps.Log.WithName(Name),
		laneReady:   deps.TxManager.LaneReady,
		signerAddr:  deps.Signer.Address(),
		probe:       probe,
		offers:      newOfferTracker(),
		metrics:     metrics,
		operations:  operations,
	}
	// Seed the offer nonce sequence from the wall clock so it stays monotonic across restarts.
	s.nonceSeq.Store(uint64(time.Now().UnixNano()))
	return s, nil
}

// Name identifies the solver.
func (s *Solver) Name() string { return Name }

// Run owns targets, offers and all three scheduled activities. One timer
// coalesces missed cadences while slow API calls run; activities never overlap.
func (s *Solver) Run(ctx context.Context) error {
	if err := s.refreshTargetsAndHydrate(ctx); err != nil {
		return err
	}
	if s.cfg.Targets != nil && len(s.targets) == 0 {
		return errors.Errorf("no configured adapter passed startup validation (must resolve and accept this solver %s as an authorized offer signer via ERC-1271); see per-adapter warnings above", s.signerAddr.Hex())
	}
	s.log.Info("starting", "adapters", len(s.targets), "apiBaseUrl", s.cfg.APIBaseURL, "discover", s.cfg.Intervals.Discover)
	s.discoverAndOffer(ctx)
	s.redeemAll(ctx)
	now := time.Now()
	activities := []struct {
		every time.Duration
		next  time.Time
		run   func(context.Context)
	}{
		{s.cfg.Intervals.Discover, now.Add(s.cfg.Intervals.Discover), func(ctx context.Context) {
			if err := s.refreshTargetsAndHydrate(ctx); err != nil {
				s.log.Error(err, "refresh adapters; keeping last-known-good targets")
			}
			s.discoverAndOffer(ctx)
		}},
		{s.cfg.Intervals.RedeemPoll, now.Add(s.cfg.Intervals.RedeemPoll), s.redeemAll},
		{s.cfg.Intervals.Reconcile, now.Add(s.cfg.Intervals.Reconcile), s.reconcile},
	}
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for ctx.Err() == nil {
		soonest := activities[0].next
		for _, activity := range activities[1:] {
			if activity.next.Before(soonest) {
				soonest = activity.next
			}
		}
		timer.Reset(max(time.Until(soonest), 0))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		for i := range activities {
			activity := &activities[i]
			if ctx.Err() != nil {
				return ctx.Err()
			}
			elapsed := time.Since(activity.next)
			if elapsed < 0 {
				continue
			}
			activity.next = activity.next.Add(elapsed - elapsed%activity.every + activity.every)
			activity.run(ctx)
		}
	}
	return ctx.Err()
}

// A successful list replaces that adapter's cache even if individual records
// are malformed. A failed list retains its last observation and freshness.
func (s *Solver) reconcileOffers(ctx context.Context, targets []Target) bool {
	complete, now := true, time.Now()
	for _, target := range targets {
		records, err := s.api.listOffers(ctx, target.Adapter)
		if err != nil {
			complete = false
			s.log.Error(err, "reconcile offers: list offers", "adapter", target.Adapter.Hex())
			continue
		}
		live := make(map[int64]offerState)
		for _, record := range records {
			state, keep, valid := s.readLiveOffer(target.Adapter, record, now)
			complete = complete && valid
			if !keep {
				continue
			}
			id := int64(record.AuctionId)
			if previous, exists := live[id]; !exists || state.expiry.After(previous.expiry) {
				live[id] = state
			}
		}
		s.offers.reconcileAdapter(target.Adapter, live)
	}
	return complete
}

func (s *Solver) readLiveOffer(adapter common.Address, record threef.OfferDto, now time.Time) (state offerState, keep bool, valid bool) {
	status := strings.ToUpper(strings.TrimSpace(record.Status))
	if offerStatusIgnored[status] {
		return offerState{}, false, true
	}
	valid = status != ""
	log := s.log.WithValues("adapter", adapter.Hex(), "offerId", record.Id)
	if !valid {
		log.Info("reconcile offers: empty status; retaining valid subset")
	}
	expiry, err := parseUnixTime(record.Expiration)
	if err != nil {
		log.Error(err, "reconcile offers: malformed expiration; retaining valid subset")
		return offerState{}, false, false
	}
	if !expiry.After(now) {
		return offerState{}, false, valid
	}
	principal, ok := new(big.Int).SetString(record.Amount, 10)
	if !ok || principal.Sign() < 0 {
		principal, valid = new(big.Int), false
		log.Info("reconcile offers: malformed amount; retaining valid subset")
	}
	if record.AuctionId <= 0 {
		log.Error(errRequiredFieldMissing, "reconcile offers: missing auction id; retaining valid subset")
		return offerState{}, false, false
	}
	return offerState{expiry: expiry, principal: principal}, true, valid
}

// adapterOffering tracks one adapter's liquidity/exposure snapshot for one offer pass.
type adapterOffering struct {
	target Target
	st     exposureState
}

// discoverAndOffer lists open auctions, snapshots adapter liquidity/exposure once, delegates offer
// selection to the configured strategy, then signs and submits the returned execution offers.
func (s *Solver) discoverAndOffer(ctx context.Context) {
	timer := observability.StartOperation(s.operations.offerRefresh)
	if !s.canCreateOffer() {
		timer.Finish(ctx, observability.ExternalOperationSkipped)
		return
	}
	if len(s.targets) == 0 {
		timer.Finish(ctx, s.snapshotOutcome(0, 0, true))
		s.observeTargetDerivedState(threeFStateOffers, 0, true)
		return
	}
	rows, err := s.api.listAuctions(ctx)
	if err != nil {
		timer.Finish(ctx, observability.ExternalOperationError)
		s.log.Error(err, "discover: list auctions")
		return
	}
	auctions := s.validAuctions(rows)
	offersComplete := s.reconcileOffers(ctx, s.targets)
	complete := offersComplete
	offerings := make([]adapterOffering, 0, len(s.targets))
	for _, target := range s.targets {
		state, err := s.reader.liquidityAndExposure(ctx, target.Adapter)
		if err != nil {
			complete = false
			s.log.Error(err, "offer: liquidity/exposure", "adapter", target.Adapter.Hex())
			continue
		}
		offerings = append(offerings, adapterOffering{target: target, st: state})
	}
	now := time.Now()
	s.offers.pruneExpired(now)
	liveOffers := s.offers.snapshot(now)
	s.observeTargetDerivedState(threeFStateOffers, len(liveOffers.entries), offersComplete)
	timer.Finish(ctx, s.snapshotOutcome(len(s.targets), len(offerings), complete))
	if len(offerings) == 0 {
		return
	}
	input := buildStrategyInput(auctions, offerings, liveOffers, now)
	if len(input.Auctions) == 0 {
		return
	}
	plan, err := s.strategy.DecideOffers(ctx, input)
	if err != nil {
		s.log.Error(err, "offer: strategy")
		return
	}
	byAuction := auctionsByID(auctions)
	floors := make(map[common.Address]*big.Int, len(offerings))
	for _, offering := range offerings {
		floors[offering.target.Adapter] = offering.st.minYieldPpm
	}
	for _, offer := range plan.Offers {
		if ctx.Err() != nil {
			return
		}
		a, known := byAuction[offer.AuctionID]
		floor, makerKnown := floors[offer.Maker]
		switch {
		case !known || a.maxRate == nil:
			s.log.Error(errors.Errorf("unknown or unpriced auction %d", offer.AuctionID), "offer: build")
			continue
		case !makerKnown:
			s.log.Error(errors.Errorf("unknown adapter %s", offer.Maker.Hex()), "offer: build")
			continue
		}
		if err := types.ValidateYield(offer.ExpectedReturn, offer.Principal, floor, *a.maxRate); err != nil {
			s.log.Error(err, "offer: yield out of bounds; skipping", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
			continue
		}
		dto, err := s.buildSignedOffer(a, offer)
		if err != nil {
			s.log.Error(err, "offer: build", "auctionId", offer.AuctionID)
			continue
		}
		submitted, err := s.submitOfferIfLaneReady(ctx, dto)
		if !submitted {
			return
		}
		if err != nil {
			s.observeOfferSubmission("error")
			s.log.Error(err, "offer: submit", "auctionId", offer.AuctionID)
			continue
		}
		s.observeSubmittedOffer(a.asset, offer.Principal, offer.ExpectedReturn)
		s.log.Info("offer submitted", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex(),
			"principal", offer.Principal.String(), "expectedReturn", dto.ExpectedReturn)
	}
}

func (s *Solver) snapshotOutcome(expected, received int, complete bool) observability.ExternalOperationOutcome {
	if expected > 0 && received == 0 {
		return observability.ExternalOperationError
	}
	if !s.targetsAuthoritative || !complete {
		return observability.ExternalOperationDegraded
	}
	return observability.ExternalOperationSuccess
}

// canCreateOffer fails closed when construction omitted the shared lane dependency. The application
// constructor always wires txmanager.LaneReady; keeping the nil case closed avoids accidental commitments
// from alternate construction paths.
func (s *Solver) canCreateOffer() bool {
	return s.laneReady != nil && s.laneReady()
}

// submitOfferIfLaneReady performs the final lane-state check immediately before the external API
// call. Discovery and strategy work can span RPC/HTTP calls, so the lane may become busy after the pass's
// entry check. A false submitted result tells the caller to abandon the remaining stale plan.
func (s *Solver) submitOfferIfLaneReady(ctx context.Context, dto threef.CreateOfferDto) (bool, error) {
	if !s.canCreateOffer() {
		return false, nil
	}
	return true, s.api.createOffer(ctx, dto)
}

// reconcile reports each adapter's live open-position set — a stateless health/observability tick.
func (s *Solver) reconcile(ctx context.Context) {
	operation := observability.StartOperation(s.operations.activeRequestRefresh)
	open, observed := 0, 0
	for _, target := range s.targets {
		state, err := s.reader.liquidityAndExposure(ctx, target.Adapter)
		log := s.log.WithValues("adapter", target.Adapter.Hex())
		if err != nil {
			log.Error(err, "reconcile")
			continue
		}
		open += state.openCount
		observed++
		log.Info("reconcile", "openRequests", state.openCount, "fundable", state.fundable.String())
	}
	complete := observed == len(s.targets)
	s.observeTargetDerivedState(threeFStateActiveRequests, open, complete)
	operation.Finish(ctx, s.snapshotOutcome(len(s.targets), observed, complete))
}

// nextNonce returns a strictly-increasing offer nonce.
func (s *Solver) nextNonce() uint64 {
	return s.nonceSeq.Add(1)
}

// refreshTargetsAndHydrate discovers adapters and installs the currently safe target subset.
// Whole-batch errors leave the last-known-good targets untouched; an empty discovery is authoritative.
func (s *Solver) refreshTargetsAndHydrate(ctx context.Context) error {
	added, err := s.refreshTargets(ctx)
	if err != nil {
		return err
	}
	s.reconcileOffers(ctx, added) // hydrate only; the next full reconcile publishes metrics
	return nil
}

func (s *Solver) refreshTargets(ctx context.Context) ([]Target, error) {
	timer := observability.StartOperation(s.operations.targetRefresh)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()
	s.targetsAuthoritative = false
	adapters, err := s.targetAdapters(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.reader.resolveAdapters(ctx, adapters, s.probe)
	if err != nil {
		return nil, err
	}
	if len(resolved) != len(adapters) {
		return nil, errors.New("adapter resolution returned incomplete batch")
	}
	previous := make(map[common.Address]bool, len(s.targets))
	for _, target := range s.targets {
		previous[target.Adapter] = true
	}
	var kept, added []Target
	complete := true
	for i, result := range resolved {
		address := adapters[i]
		log := s.log.WithValues("adapter", address.Hex())
		switch {
		case result.err != nil:
			complete = false
			if errors.Is(result.err, errAdapterUnconfigured) {
				log.V(1).Info("skipping adapter: not configured on-chain", "reason", result.err.Error())
			} else {
				log.Error(result.err, "skipping adapter: resolution failed")
			}
		case !result.authorized:
			log.Info("skipping adapter: solver is not an authorized offer signer", "signer", s.signerAddr.Hex(), "offerSigner", result.signer.Hex())
		default:
			target := Target{Adapter: address, Vault: result.vault, Collateral: result.collateral}
			kept = append(kept, target)
			if !previous[address] {
				added = append(added, target)
			}
			log.Info("resolved target", "vault", result.vault.Hex(), "collateral", result.collateral.Hex())
		}
	}
	s.installTargets(kept)
	s.targetsAuthoritative = complete
	outcome = observability.ExternalOperationDegraded
	if complete {
		outcome = observability.ExternalOperationSuccess
		s.observeState(threeFStateTargets, len(kept))
	}
	return added, nil
}

func (s *Solver) targetAdapters(ctx context.Context) ([]common.Address, error) {
	if s.cfg.Targets == nil {
		addresses, err := s.reader.factoryAdapters(ctx, s.cfg.AdapterFactory)
		return deduplicateAdapters(addresses), err
	}
	addresses := make([]common.Address, 0, len(s.cfg.Targets))
	for _, target := range s.cfg.Targets {
		addresses = append(addresses, target.Adapter)
	}
	return deduplicateAdapters(addresses), nil
}

// installTargets applies the currently safe target subset to the offer tracker and runtime snapshot.
// The caller publishes observation metrics separately and only for a complete resolution pass; a
// partial pass still fails closed operationally without making its uncertain count look fresh.
func (s *Solver) installTargets(targets []Target) {
	active := make(map[common.Address]struct{}, len(targets))
	for _, target := range targets {
		active[target.Adapter] = struct{}{}
	}
	s.offers.retainAdapters(active)
	s.targets = targets
}
