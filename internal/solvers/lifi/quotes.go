package lifi

import (
	"context"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

const (
	initialQuoteSuspensionBackoff = time.Second
	maximumQuoteSuspensionBackoff = 30 * time.Second
)

type quoteSubmitter interface {
	submitQuotes(ctx context.Context, quotes []types.Quote) error
}

type quotePairKey struct {
	fromAsset    common.Address
	toAsset      common.Address
	fromDecimals int
	toDecimals   int
}

type quotePairState struct {
	fingerprint string
	expiry      int64
	quotes      []types.Quote
}

type quoteState struct {
	active      map[quotePairKey]quotePairState
	renewBefore time.Duration
}

func (s *Solver) quoteLoop(
	ctx context.Context,
	routes []route,
	refresh <-chan struct{},
	feedConnections <-chan context.Context,
) error {
	ticker := time.NewTicker(s.cfg.QuoteInterval)
	defer ticker.Stop()
	availabilityChanges, unsubscribe := s.subscribeTransactionAvailability()
	defer unsubscribe()

	state := newQuoteState(max(s.cfg.QuoteInterval, s.cfg.QuoteTTL/3))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			s.cfg.OrderServer.HTTPTimeout,
		)
		defer cancel()
		s.suspendQuotes(shutdownCtx, state)
		if err := shutdownCtx.Err(); err != nil && len(state.active) > 0 {
			s.log.Error(err, "quote shutdown incomplete", "activePairs", len(state.active))
		}
	}()
	var lastBlock uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case connectionCtx := <-feedConnections:
			state.forceRenewal()
			connectedCtx, stopConnected := context.WithCancel(connectionCtx)
			stopOnShutdown := context.AfterFunc(ctx, stopConnected)
			//nolint:contextcheck // connectedCtx is cancelled by either the feed connection or quote-loop context.
			s.runConnectedQuoteLoop(
				connectedCtx, routes, refresh, ticker.C, availabilityChanges, state, &lastBlock,
			)
			_ = stopOnShutdown()
			stopConnected()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.suspendQuotes(ctx, state)
		case <-refresh:
			s.suspendQuotes(ctx, state)
		case <-ticker.C:
			s.suspendQuotes(ctx, state)
		case <-availabilityChanges:
			// A coalesced signal may represent pause followed by resume. Always retire any curve
			// first so a missed intermediate state cannot leave a pre-pause commitment live.
			s.suspendQuotes(ctx, state)
		}
	}
}

func (s *Solver) runConnectedQuoteLoop(
	ctx context.Context,
	routes []route,
	refresh <-chan struct{},
	ticks <-chan time.Time,
	availabilityChanges <-chan struct{},
	state *quoteState,
	lastBlock *uint64,
) {
	s.refreshQuotes(ctx, routes, state)
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh:
			s.refreshQuotes(ctx, routes, state)
		case <-ticks:
			if s.shouldRefreshQuotes(ctx, state, lastBlock) {
				s.refreshQuotes(ctx, routes, state)
			}
		case <-availabilityChanges:
			// Signals are deliberately coalesced. Retire the current curve even when the latest
			// state is already available, then republish from fresh state below.
			s.suspendQuotes(ctx, state)
			if s.transactionLaneAvailable() {
				state.forceRenewal()
				s.refreshQuotes(ctx, routes, state)
			}
		}
	}
}

func (s *Solver) transactionLaneAvailable() bool {
	return s.txAvailability != nil && s.txAvailability.Available()
}

func (s *Solver) subscribeTransactionAvailability() (<-chan struct{}, func()) {
	if s.txAvailability == nil {
		return nil, func() {}
	}
	return s.txAvailability.SubscribeAvailability()
}

func (s *Solver) suspendQuotes(ctx context.Context, state *quoteState) {
	backoff := initialQuoteSuspensionBackoff
	for {
		removed, err := state.reconcile(ctx, s.orders, nil, s.wallNow())
		if err == nil {
			if removed > 0 {
				s.log.Info("quotes suspended", "removedPairs", removed)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		s.log.Error(err, "quote suspension: expire active quotes; retrying", "backoff", backoff.String())
		if !waitForRetry(ctx, backoff) {
			return
		}
		backoff = min(2*backoff, maximumQuoteSuspensionBackoff)
	}
}

func (s *Solver) shouldRefreshQuotes(ctx context.Context, state *quoteState, lastBlock *uint64) bool {
	if s.cfg.QuoteRefreshMode != quoteRefreshModeBlock {
		return true
	}
	needsRenewal := state.needsRenewal(s.wallNow())
	block, err := s.reader.latestBlockNumber(ctx)
	if err != nil {
		s.log.Error(err, "quote refresh: read latest block")
		return needsRenewal
	}
	if block == *lastBlock && !needsRenewal {
		return false
	}
	*lastBlock = block
	return true
}

func (s *Solver) refreshQuotes(ctx context.Context, routes []route, state *quoteState) {
	if !s.transactionLaneAvailable() {
		s.suspendQuotes(ctx, state)
		return
	}
	chainTime, err := s.now(ctx)
	if err != nil {
		s.log.Error(err, "quote refresh: read latest block time")
		return
	}
	snapshotSet, err := s.reader.quoteSnapshots(ctx, routes, s.cfg.Executor, chainTime)
	if err != nil {
		s.log.Error(err, "quote refresh: read routes")
		return
	}
	maxFeePerGas, err := s.readMaxFeePerGas(ctx)
	if err != nil {
		s.log.Error(err, "quote refresh: read max fee per gas")
		return
	}
	direct := filterQuoteInventory(snapshotSet.Direct, s.cfg.TokenPolicy)
	discountBases := filterQuoteInventory(snapshotSet.Physical, s.cfg.TokenPolicy)
	inventory := append([]liquidlane.Inventory(nil), direct...)
	inventory = append(inventory, s.quoteDiscountInventories(ctx, discountBases, chainTime)...)
	serverTime := s.wallNow()
	out, err := s.strategy.DecideQuotes(ctx, types.QuoteInput{
		Solver:            s.cfg.Executor,
		Inventory:         inventory,
		Reservations:      s.capacity.Snapshot(),
		SingleRouteTokens: s.cfg.TokenPolicy.SingleRouteTokens(),
		GasSnapshot:       snapshotSet.GasSnapshot,
		GasPrices:         snapshotSet.GasPrices,
		MaxFeePerGas:      maxFeePerGas,
		ChainTime:         chainTime,
		ServerTime:        serverTime,
		QuoteExpiresAt:    serverTime.Add(s.cfg.QuoteTTL),
	})
	if err != nil {
		s.log.Error(err, "quote refresh: strategy")
		return
	}
	if len(out.Quotes) == 0 {
		s.log.V(1).Info("quote refresh: strategy produced no quotes", "routes", len(inventory))
	}
	if !s.transactionLaneAvailable() {
		s.suspendQuotes(ctx, state)
		return
	}
	removed, err := state.reconcile(ctx, s.orders, out.Quotes, serverTime)
	if err != nil {
		s.log.Error(err, "quote refresh: submit quotes", "quotes", len(out.Quotes))
		return
	}
	s.log.Info("quotes reconciled", "quotes", len(out.Quotes), "removedPairs", removed, "routes", len(inventory))
}

func filterQuoteInventory(inventory []liquidlane.Inventory, policy tokenpolicy.Policy) []liquidlane.Inventory {
	filtered := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if policy.Allows(item.TokenIn) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func newQuoteState(renewBefore time.Duration) *quoteState {
	return &quoteState{
		active: make(map[quotePairKey]quotePairState), renewBefore: renewBefore,
	}
}

func (s *quoteState) forceRenewal() {
	for key, pair := range s.active {
		pair.expiry = 0
		s.active[key] = pair
	}
}

func (s *quoteState) needsRenewal(now time.Time) bool {
	deadline := now.Add(s.renewBefore).Unix()
	for _, pair := range s.active {
		if pair.expiry <= deadline {
			return true
		}
	}
	return false
}

func (s *quoteState) reconcile(
	ctx context.Context,
	submitter quoteSubmitter,
	quotes []types.Quote,
	now time.Time,
) (int, error) {
	next := indexQuotePairs(quotes)
	expire := make([]quotePairKey, 0)
	publish := make(map[quotePairKey]bool, len(next))
	for key, current := range s.active {
		upcoming, ok := next[key]
		if !ok {
			expire = append(expire, key)
			continue
		}
		if shouldReplaceQuotePair(current, upcoming, now, s.renewBefore) {
			publish[key] = true
		}
	}
	for key := range next {
		if _, ok := s.active[key]; !ok {
			publish[key] = true
		}
	}
	publishKeys := make([]quotePairKey, 0, len(publish))
	for key, enabled := range publish {
		if enabled {
			publishKeys = append(publishKeys, key)
		}
	}
	sort.Slice(publishKeys, func(i, j int) bool {
		return quotePairKeyString(publishKeys[i]) < quotePairKeyString(publishKeys[j])
	})
	sort.Slice(expire, func(i, j int) bool { return quotePairKeyString(expire[i]) < quotePairKeyString(expire[j]) })
	toPublish := make([]types.Quote, 0, len(quotes)+len(expire))
	for _, key := range expire {
		for _, quote := range s.active[key].quotes {
			quote.Expiry = now.Add(-time.Second).Unix()
			toPublish = append(toPublish, quote)
		}
	}
	for _, key := range publishKeys {
		toPublish = append(toPublish, next[key].quotes...)
	}
	if len(toPublish) != 0 {
		if err := submitter.submitQuotes(ctx, toPublish); err != nil {
			// The server may have accepted a request even when the client did not
			// receive its response. Track every attempted pair conservatively so
			// disconnect suspension expires it before quoting resumes.
			for _, key := range publishKeys {
				uncertain := next[key]
				uncertain.expiry = 0
				s.active[key] = uncertain
			}
			return len(expire), err
		}
	}
	for _, key := range expire {
		delete(s.active, key)
	}
	for _, key := range publishKeys {
		s.active[key] = next[key]
	}
	return len(expire), nil
}

func shouldReplaceQuotePair(current, upcoming quotePairState, now time.Time, renewBefore time.Duration) bool {
	if current.fingerprint != upcoming.fingerprint || upcoming.expiry < current.expiry {
		return true
	}
	return current.expiry <= now.Add(renewBefore).Unix()
}

func indexQuotePairs(quotes []types.Quote) map[quotePairKey]quotePairState {
	grouped := make(map[quotePairKey][]types.Quote)
	for _, quote := range quotes {
		key := pairKey(quote)
		grouped[key] = append(grouped[key], quote)
	}

	out := make(map[quotePairKey]quotePairState, len(grouped))
	for key, pairQuotes := range grouped {
		fingerprints := make([]string, 0, len(pairQuotes))
		expiry := int64(0)
		for _, quote := range pairQuotes {
			ranges := make([]string, 0, len(quote.Ranges))
			for _, r := range quote.Ranges {
				ranges = append(ranges, bigString(r.MinAmount)+":"+bigString(r.MaxAmount)+":"+r.Quote)
			}
			fingerprints = append(fingerprints, strings.ToLower(quote.ExclusiveFor.Hex())+":"+strings.Join(ranges, ","))
			if expiry == 0 || quote.Expiry < expiry {
				expiry = quote.Expiry
			}
		}
		sort.Strings(fingerprints)
		out[key] = quotePairState{
			fingerprint: strings.Join(fingerprints, "|"),
			expiry:      expiry,
			quotes:      append([]types.Quote(nil), pairQuotes...),
		}
	}
	return out
}

func pairKey(quote types.Quote) quotePairKey {
	return quotePairKey{
		fromAsset: quote.FromAsset, toAsset: quote.ToAsset,
		fromDecimals: quote.FromDecimals, toDecimals: quote.ToDecimals,
	}
}

func quotePairKeyString(key quotePairKey) string {
	return strings.Join([]string{
		strings.ToLower(key.fromAsset.Hex()), strings.ToLower(key.toAsset.Hex()),
		strconv.Itoa(key.fromDecimals), strconv.Itoa(key.toDecimals),
	}, ":")
}

func bigString(n *big.Int) string {
	if n == nil {
		return "<nil>"
	}
	return n.String()
}
