// Package bridgefacilitator implements the 3F (Grunt) Bridge Facilitator solver: it discovers
// bridge-loan auctions via the 3F API, snapshots adapter state for a trusted strategy, signs the
// returned offers, and realizes repaid loans back into the vault. It self-registers with the solver
// framework via init().
package bridgefacilitator

import (
	"cmp"
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
	txManager  transactionSender
	strategy   types.Strategy
	log        logr.Logger
	laneReady  func() bool    // shared txmanager lane state; safe for the single Run goroutine
	signerAddr common.Address // the solver's own signer address (diagnostics only), set in factory
	probe      signerProbe    // one-time (hash, sig) used to validate offer-signer authorization, set in factory
	nonceSeq   atomic.Uint64
	offers     *offerTracker // dedup: (adapter, auction) pairs we hold a live offer for (Run goroutine only)
	targets    []Target      // current resolved snapshot; owned exclusively by the Run goroutine
	// targetsAuthoritative records whether targets covers the complete configured/discovered source.
	// A partial refresh still installs its safe subset, but derived metric freshness must stay retained.
	targetsAuthoritative bool
	metrics              *threeFMetrics
	operations           threeFOperationObservers
	// links remembers each submitted offer's span so the later redeem and the API's offer listing can
	// point back at it (spec §12). Process-local and best effort; a miss changes nothing.
	links *observability.SpanLinks
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
		cfg:        cfg,
		deps:       deps,
		api:        api,
		reader:     newReader(deps.Chain, cfg.LiquidityLens),
		txManager:  deps.TxManager,
		strategy:   offerStrategy,
		log:        deps.Log.WithName(Name),
		laneReady:  deps.TxManager.LaneReady,
		signerAddr: deps.Signer.Address(),
		probe:      probe,
		offers:     newOfferTracker(),
		metrics:    metrics,
		operations: operations,
		links:      observability.NewSpanLinks(0),
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
	// The solver logger is narrower than the one solver.Run stored; carry it so every line below,
	// including the discovery, redeem and reconcile ticks, logs through it.
	ctx = observability.WithLogger(ctx, s.log)

	// Build the initial explicit-or-factory snapshot. A successfully empty factory is valid: the
	// daemon stays alive and picks up future entities on a discovery tick.
	if err := s.refreshTargetsAndHydrate(ctx); err != nil {
		return err
	}
	// Preserve the explicit-list fail-closed startup contract. Factory-discovered deployments may
	// start empty because later registry entries are expected.
	if s.cfg.Targets != nil && len(s.targets) == 0 {
		return errors.Errorf("no configured adapter passed startup validation (must resolve and accept this solver %s as an authorized offer signer via ERC-1271); see per-adapter warnings above", s.signerAddr.Hex())
	}

	observability.Log(ctx).Info("starting",
		"adapters", len(s.targets),
		"apiBaseUrl", s.cfg.APIBaseURL,
		"discover", s.cfg.Intervals.Discover.String(),
	)

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
			if err := s.refreshTargetsAndHydrate(ctx); err != nil {
				observability.Log(ctx).Error(err, "refresh adapters; keeping last-known-good targets")
			}
			s.discoverAndOffer(ctx)
		case <-redeemT.C:
			s.redeemAll(ctx)
		case <-reconcileT.C:
			s.reconcile(ctx)
		}
	}
}

// reconcileOffers re-lists each target adapter's live offers from the 3F API and replaces that adapter's
// offer cache with them, so coverage reflects our own offers and any made out of band. The poll is
// authoritative. Best-effort: one adapter's list failure can't block the pass (its cache is left as-is).
func (s *Solver) reconcileOffers(ctx context.Context, targets []Target) bool {
	ctx, end := tracer.Start(ctx, "3f.offers.reconcile")
	// The stage fails when a listing or a field the solver acts on does; the first such error is the
	// one the span reports, the rest are logged per offer.
	var stageErr error
	defer func() { end(stageErr) }()

	now := time.Now()
	complete := true
	for _, t := range targets {
		offers, err := s.api.listOffers(ctx, t.Adapter)
		if err != nil {
			complete = false
			stageErr = cmp.Or[error](stageErr, err)
			observability.Log(ctx).Error(err, "reconcile offers: list offers", "adapter", t.Adapter.Hex())
			continue
		}
		live := make(map[int64]offerState)
		for _, o := range offers {
			offerLog := s.listedOfferLogger(observability.Log(ctx), t.Adapter, int64(o.AuctionId))
			status := strings.ToUpper(strings.TrimSpace(o.Status))
			if offerStatusIgnored[status] {
				observability.Decline(ctx, "offer_not_live", "offer status "+status)
				continue // failed/not-accepted/cancelled offers aren't live coverage
			}
			if status == "" {
				complete = false
				offerLog.Info("reconcile offers: empty status; retaining valid subset",
					"adapter", t.Adapter.Hex(), "offerId", o.Id)
			}
			exp, perr := parseUnixTime(o.Expiration)
			if perr != nil {
				complete = false
				stageErr = cmp.Or[error](stageErr, perr)
				offerLog.Error(perr, "reconcile offers: malformed expiration; retaining valid subset",
					"adapter", t.Adapter.Hex(), "offerId", o.Id)
				continue
			}
			if !exp.After(now) {
				observability.Decline(ctx, "offer_not_live", "offer expired")
				continue // valid, already-expired offer
			}
			principal, ok := new(big.Int).SetString(o.Amount, 10)
			if !ok || principal.Sign() < 0 {
				complete = false
				offerLog.Info("reconcile offers: malformed amount; retaining valid subset",
					"adapter", t.Adapter.Hex(), "offerId", o.Id)
				principal = nil
			}
			if o.AuctionId <= 0 {
				complete = false
				stageErr = cmp.Or[error](stageErr, errRequiredFieldMissing)
				offerLog.Error(errRequiredFieldMissing, "reconcile offers: missing auction id; retaining valid subset",
					"adapter", t.Adapter.Hex(), "offerId", o.Id)
				continue
			}
			// One live offer per (adapter, auction) is assumed; if the API ever lists more, keep the latest.
			auctionID := int64(o.AuctionId)
			if cur, exists := live[auctionID]; !exists || exp.After(cur.expiry) {
				live[auctionID] = offerState{expiry: exp, principal: principal}
			}
		}
		s.offers.reconcileAdapter(t.Adapter, live)
	}
	return complete
}

// adapterOffering tracks one adapter's liquidity/exposure snapshot for one offer pass.
type adapterOffering struct {
	target Target
	st     exposureState
}

// discoverAndOffer lists open auctions, snapshots adapter liquidity/exposure once, delegates offer
// selection to the configured strategy, then signs and submits the returned execution offers.
func (s *Solver) discoverAndOffer(ctx context.Context) {
	ctx, end := tracer.Start(ctx, "3f.sync")
	// The pass fails only on the two calls that abort it outright; every stage records its own error
	// and every expected skip below is a decline.
	var passErr error
	defer func() { end(passErr) }()

	timer := observability.StartOperation(s.operations.offerRefresh)
	observeRefresh := func(outcome observability.ExternalOperationOutcome) {
		timer.Finish(ctx, outcome)
	}
	if !s.canCreateOffer() {
		observeRefresh(observability.ExternalOperationSkipped)
		observability.Decline(ctx, "offer_skipped", "transaction lane not ready")
		observability.Log(ctx).V(1).Info("skipping offer discovery: transaction lane not ready")
		return
	}
	if len(s.targets) == 0 {
		outcome := observability.ExternalOperationSuccess
		if !s.targetsAuthoritative {
			outcome = observability.ExternalOperationDegraded
		}
		observeRefresh(outcome)
		observability.Decline(ctx, "offer_skipped", "no resolved adapters")
		s.observeTargetDerivedState(threeFStateOffers, 0, true)
		return
	}
	auctions, err := s.api.listAuctions(ctx)
	if err != nil {
		passErr = err
		observeRefresh(observability.ExternalOperationError)
		observability.Log(ctx).Error(err, "discover: list auctions")
		return
	}
	auctions = s.validAuctions(observability.Log(ctx), auctions)
	observability.Log(ctx).V(1).Info("discovered auctions", "count", len(auctions))

	// Rebuild coverage from the live API before deciding, so out-of-band offers count and we don't double-offer.
	offersComplete := s.reconcileOffers(ctx, s.targets)

	offerings := make([]*adapterOffering, 0, len(s.targets))
	liquidityReadsFailed := 0
	for _, t := range s.targets {
		st, lerr := s.reader.liquidityAndExposure(ctx, t.Adapter)
		if lerr != nil {
			liquidityReadsFailed++
			observability.Log(ctx).Error(lerr, "offer: liquidity/exposure", "adapter", t.Adapter.Hex())
			continue
		}
		observability.Log(ctx).V(1).Info("adapter liquidity",
			"adapter", t.Adapter.Hex(), "fundable", st.fundable.String(), "openRequests", st.openCount,
			"maxAssets", st.maxAssets.String(), "minAssets", st.minAssets.String(),
			"minYieldPpm", st.minYieldPpm.String())
		offerings = append(offerings, &adapterOffering{target: t, st: st})
	}
	now := time.Now()
	s.offers.pruneExpired(now)
	s.observeTargetDerivedState(threeFStateOffers, len(s.offers.liveEntries(now)), offersComplete)
	refreshOutcome := observability.ExternalOperationSuccess
	switch {
	case len(offerings) == 0 && len(s.targets) != 0:
		refreshOutcome = observability.ExternalOperationError
	case !s.targetsAuthoritative || !offersComplete || liquidityReadsFailed != 0:
		refreshOutcome = observability.ExternalOperationDegraded
	}
	observeRefresh(refreshOutcome)
	if len(offerings) == 0 || !offersComplete {
		observability.Decline(ctx, "offer_skipped", "incomplete adapter or offer coverage")
		return // incomplete live commitments cannot safely authorize another offer
	}
	input := s.buildOfferInput(ctx, auctions, offerings, now)
	if len(input.Auctions) == 0 {
		observability.Decline(ctx, "offer_skipped", "no open offerable auctions")
		return // no open, offerable auctions this pass
	}
	out, err := s.decideOffers(ctx, input)
	if err != nil {
		passErr = err
		observability.Log(ctx).Error(err, "offer: strategy")
		return
	}
	// minYieldByAdapter lets the submission loop validate EVERY strategy's offers (default and webhook),
	// not just the default strategy's pricing, against the adapter's exact on-chain minYieldPerRequest.
	minYieldByAdapter := make(map[common.Address]*big.Int, len(offerings))
	for _, o := range offerings {
		minYieldByAdapter[o.target.Adapter] = o.st.minYieldPpm
	}

	auctionByID := auctionViewsByID(auctions)
	for _, offer := range out.Offers {
		if stop := s.offerOnAuction(ctx, offer, auctionByID, minYieldByAdapter); stop {
			observability.Log(ctx).V(1).Info("stopping offer submission: transaction lane no longer ready")
			return
		}
	}
}

// buildOfferInput is the auction-view stage: it converts this pass's auctions and adapter snapshots
// into the strategy request. 3F lists every auction in one response, so the views are built for the
// whole batch under a single span rather than one per auction.
func (s *Solver) buildOfferInput(
	ctx context.Context, auctions []threef.AuctionDto, offerings []*adapterOffering, now time.Time,
) types.OfferInput {
	ctx, end := tracer.Start(ctx, "3f.auction.view")
	defer end(nil) // a pure conversion; auctions it drops are declines, not failures

	input := buildStrategyInput(auctions, offerings, s.offers, now)
	if len(input.Auctions) < len(auctions) {
		observability.Decline(ctx, "auction_skipped",
			"auctions closed, already covered, or missing a field the offer needs")
	}
	return input
}

// decideOffers is the strategy stage. 3F decides for every auction in one call, so this span is a
// sibling of the per-auction spans rather than a child of one.
func (s *Solver) decideOffers(ctx context.Context, input types.OfferInput) (out types.OfferOutput, err error) {
	ctx, end := tracer.Start(ctx, "3f.offer.decide", observability.AttrStrategy.String(s.strategyName()))
	defer func() { end(err) }()

	out, err = s.strategy.DecideOffers(ctx, input)
	if err == nil && len(out.Offers) == 0 {
		observability.Decline(ctx, "offer_declined", "strategy returned no offers")
	}
	return out, err
}

// offerOnAuction validates, signs and submits one strategy offer under its own auction span. A true
// result means the transaction lane went busy mid-pass, so the rest of the plan is stale.
func (s *Solver) offerOnAuction(
	ctx context.Context,
	offer types.OfferExecution,
	auctionByID map[int64]auctionView,
	minYieldByAdapter map[common.Address]*big.Int,
) (stop bool) {
	var err error
	ctx, end := tracer.Start(ctx, "3f.auction",
		observability.AttrAuctionID.Int64(offer.AuctionID),
		observability.AttrAdapter.String(offer.Maker.Hex()),
		observability.AttrRequestAddress.String(offer.Request.Hex()),
	)
	defer func() { end(err) }()

	av, ok := auctionByID[offer.AuctionID]
	if !ok {
		err = errors.Errorf("auction %d not found", offer.AuctionID)
		observability.Log(ctx).Error(err, "offer: build")
		return false
	}
	floor, known := minYieldByAdapter[offer.Maker]
	if !known {
		err = errors.Errorf("offer for adapter %s absent from this pass's snapshot", offer.Maker.Hex())
		observability.Log(ctx).Error(err, "offer: unknown maker; skipping", "auctionId", offer.AuctionID)
		return false
	}
	maxRate, rateOk := av.maxRateBps()
	if !rateOk {
		err = errors.Errorf("auction %d has no resolved maxRate", offer.AuctionID)
		observability.Log(ctx).Error(err, "offer: unbiddable auction; skipping", "adapter", offer.Maker.Hex())
		return false
	}
	// Backstop for all strategies: the offer must clear the on-chain floor and stay under the auction
	// max rate, or it reverts (FAILED) / is rejected (NOT_ACCEPTED). Also guards nil/invalid amounts.
	if err = types.ValidateYield(offer.ExpectedReturn, offer.Principal, floor, maxRate); err != nil {
		observability.Log(ctx).Error(err, "offer: yield out of bounds; skipping",
			"auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
		return false
	}
	dto, err := s.buildSignedOffer(ctx, av, offer)
	if err != nil {
		observability.Log(ctx).Error(err, "offer: build", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
		return false
	}
	submitted, err := s.submitOfferIfLaneReady(ctx, dto)
	if !submitted {
		err = nil // an expected skip, declined on the submit stage
		return true
	}
	if err != nil {
		s.observeOfferSubmission("error")
		observability.Log(ctx).Error(err, "offer: submit", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex())
		return false
	}
	s.rememberOffer(ctx, offer, dto)
	s.observeSubmittedOffer(common.HexToAddress(av.depositAsset()), offer.Principal, offer.ExpectedReturn)
	// No local record: the next reconcile re-lists this offer from the API (the poll is authoritative).
	observability.Log(ctx).Info("offer submitted", "auctionId", offer.AuctionID, "adapter", offer.Maker.Hex(),
		"request", offer.Request.Hex(), "principal", offer.Principal.String(), "expectedReturn", dto.ExpectedReturn)
	return false
}

// rememberOffer keeps this auction's span reachable from the two places the offer resurfaces: the
// on-chain settlement, which only knows Request addresses, and the API's offer listing, which only
// knows (adapter, auction). Both keys expire with the offer plus slack (spec §12).
func (s *Solver) rememberOffer(ctx context.Context, offer types.OfferExecution, dto threef.CreateOfferDto) {
	if s.links == nil {
		return
	}
	ttl := offerLinkTTLSlack
	if expiration, err := parseUnixTime(dto.Expiration); err == nil {
		ttl = time.Until(expiration) + offerLinkTTLSlack
	}
	s.links.Remember(ctx, requestLinkKey(offer.Request), ttl)
	s.links.Remember(ctx, auctionLinkKey(offer.Maker, offer.AuctionID), ttl)
}

// canCreateOffer fails closed when construction omitted the shared lane dependency. The registered
// factory always wires txmanager.LaneReady; keeping the nil case closed avoids accidental commitments
// from alternate construction paths.
func (s *Solver) canCreateOffer() bool {
	return s.laneReady != nil && s.laneReady()
}

// submitOfferIfLaneReady performs the final lane-state check immediately before the external API
// call. Discovery and strategy work can span RPC/HTTP calls, so the lane may become busy after the pass's
// entry check. A false submitted result tells the caller to abandon the remaining stale plan.
func (s *Solver) submitOfferIfLaneReady(
	ctx context.Context, dto threef.CreateOfferDto,
) (submitted bool, err error) {
	ctx, end := tracer.Start(ctx, "3f.offer.submit")
	defer func() { end(err) }()

	if !s.canCreateOffer() {
		observability.Decline(ctx, "offer_skipped", "transaction lane no longer ready")
		return false, nil
	}
	return true, s.api.createOffer(ctx, dto)
}

// reconcile reports each adapter's live open-position set — a stateless health/observability tick.
func (s *Solver) reconcile(ctx context.Context) {
	timer := observability.StartOperation(s.operations.activeRequestRefresh)
	totalOpen := 0
	complete := true
	successfulReads := 0
	for _, t := range s.targets {
		st, err := s.reader.liquidityAndExposure(ctx, t.Adapter)
		if err != nil {
			complete = false
			observability.Log(ctx).Error(err, "reconcile", "adapter", t.Adapter.Hex())
			continue
		}
		successfulReads++
		totalOpen += st.openCount
		observability.Log(ctx).Info("reconcile", "adapter", t.Adapter.Hex(),
			"openRequests", st.openCount, "fundable", st.fundable.String())
	}
	s.observeTargetDerivedState(threeFStateActiveRequests, totalOpen, complete)
	outcome := observability.ExternalOperationSuccess
	switch {
	case len(s.targets) != 0 && successfulReads == 0:
		outcome = observability.ExternalOperationError
	case !s.targetsAuthoritative || !complete:
		outcome = observability.ExternalOperationDegraded
	}
	timer.Finish(ctx, outcome)
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
	refreshOutcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, refreshOutcome) }()
	// Until this pass proves otherwise, derived observations cannot claim complete target coverage.
	// Whole-batch failures retain the safe runtime snapshot but must also retain metric freshness.
	s.targetsAuthoritative = false

	var adapters []common.Address
	if s.cfg.Targets != nil {
		adapters = make([]common.Address, len(s.cfg.Targets))
		for i := range s.cfg.Targets {
			adapters[i] = s.cfg.Targets[i].Adapter
		}
	} else {
		var err error
		adapters, err = s.reader.factoryAdapters(ctx, s.cfg.AdapterFactory)
		if err != nil {
			return nil, err
		}
	}
	adapters = deduplicateAdapters(adapters)
	if len(adapters) == 0 {
		s.installTargets(nil)
		s.targetsAuthoritative = true
		s.observeState(threeFStateTargets, 0)
		refreshOutcome = observability.ExternalOperationSuccess
		return nil, nil
	}

	resolved, err := s.reader.resolveAdapters(ctx, adapters, s.probe)
	if err != nil {
		return nil, err
	}
	previous := make(map[common.Address]struct{}, len(s.targets))
	for _, target := range s.targets {
		previous[target.Adapter] = struct{}{}
	}

	kept := make([]Target, 0, len(adapters))
	added := make([]Target, 0, len(adapters))
	resolutionComplete := true
	for i, adapterAddr := range adapters {
		r := resolved[i]
		if r.err != nil {
			resolutionComplete = false
			if errors.Is(r.err, errAdapterUnconfigured) {
				observability.Log(ctx).V(1).Info("skipping adapter: not configured on-chain",
					"adapter", adapterAddr.Hex(), "reason", r.err.Error())
			} else {
				observability.Log(ctx).Error(r.err, "skipping adapter: resolution failed", "adapter", adapterAddr.Hex())
			}
			continue
		}
		if !r.authorized {
			observability.Log(ctx).Info("skipping adapter: solver is not an authorized offer signer",
				"adapter", adapterAddr.Hex(),
				"signer", s.signerAddr.Hex(),
				"offerSigner", r.signer.Hex())
			continue
		}
		target := Target{Adapter: adapterAddr, Vault: r.vault, Collateral: r.collateral}
		kept = append(kept, target)
		if _, ok := previous[adapterAddr]; !ok {
			added = append(added, target)
		}
		observability.Log(ctx).Info("resolved target",
			"adapter", adapterAddr.Hex(), "vault", r.vault.Hex(), "collateral", r.collateral.Hex())
	}

	s.installTargets(kept)
	s.targetsAuthoritative = resolutionComplete
	if resolutionComplete {
		refreshOutcome = observability.ExternalOperationSuccess
	} else {
		refreshOutcome = observability.ExternalOperationDegraded
	}
	if s.targetsAuthoritative {
		s.observeState(threeFStateTargets, len(kept))
	}
	return added, nil
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

// validAuctions drops auctions missing a field the solver acts on. The generated client tolerates a
// dropped field by zero-valuing it, so this is where such a schema change becomes visible.
func (s *Solver) validAuctions(log logr.Logger, auctions []threef.AuctionDto) []threef.AuctionDto {
	kept := make([]threef.AuctionDto, 0, len(auctions))
	for _, a := range auctions {
		var missing string
		switch {
		case a.Id <= 0:
			missing = "id"
		case strings.TrimSpace(a.Status) == "":
			missing = "status"
		case !common.IsHexAddress(a.RequestId):
			missing = "requestId"
		default:
			kept = append(kept, a)
			continue
		}
		log.Error(errRequiredFieldMissing, "discover: skipping auction",
			"field", missing, "auctionId", a.Id, "requestId", a.RequestId, "status", a.Status)
	}
	return kept
}
