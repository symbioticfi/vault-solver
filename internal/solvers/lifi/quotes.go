package lifi

import (
	"cmp"
	"context"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/observability"
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

//nolint:contextcheck // session.ctx is a child of ctx, additionally canceled when the feed disconnects.
func (s *Solver) quoteLoop(ctx context.Context, routes []route, refresh <-chan struct{}, feedConnections <-chan context.Context) error {
	ticker := time.NewTicker(s.cfg.QuoteInterval)
	defer ticker.Stop()
	laneChanges, unsubscribe := s.subscribeTransactionLaneState()
	defer unsubscribe()
	state := newQuoteState(max(s.cfg.QuoteInterval, s.cfg.QuoteTTL/3))
	defer func() {
		drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.OrderServer.HTTPTimeout)
		defer cancel()
		s.suspendQuotes(drain, state)
		if err := drain.Err(); err != nil && len(state.active) > 0 {
			s.log.Error(err, "quote shutdown incomplete", "activePairs", len(state.active))
		}
	}()
	type quoteSession struct {
		ctx     context.Context
		release func()
	}
	session := quoteSession{ctx: ctx, release: func() {}}
	var disconnected <-chan struct{}
	defer func() { session.release() }()
	var lastBlock uint64
	for {
		publish, retire := false, false
		select {
		case <-ctx.Done():
			return ctx.Err()
		case connection, ok := <-feedConnections:
			if !ok {
				feedConnections = nil
				continue
			}
			session.release()
			operation, cancel := context.WithCancel(ctx)
			stop := context.AfterFunc(connection, cancel)
			session.release = func() { stop(); cancel() }
			session.ctx = operation //nolint:fatcontext // Each connection replaces a canceled child of the root ctx; contexts never nest.
			disconnected = connection.Done()
			if connection.Err() != nil {
				cancel()
			}
			state.forceRenewal()
			publish = true
		case <-disconnected:
			session.release()
			session.ctx, disconnected = ctx, nil
			retire = true
		case <-refresh:
			publish = true
		case <-ticker.C:
			publish = disconnected == nil || s.shouldRefreshQuotes(session.ctx, state, &lastBlock)
		case <-laneChanges:
			// Notifications coalesce. Retire even if the latest lane state has already
			// returned to ready: the old commitment crossed an unobserved busy period.
			retire, publish = true, true
			state.forceRenewal()
		}
		if retire || disconnected == nil {
			s.suspendQuotes(ctx, state)
		}
		if publish && disconnected != nil && session.ctx.Err() == nil {
			s.refreshQuotes(session.ctx, routes, state)
		}
	}
}

func (s *Solver) transactionLaneReady() bool {
	return s.txLaneState != nil && s.txLaneState.LaneReady()
}

func (s *Solver) subscribeTransactionLaneState() (<-chan struct{}, func()) {
	if s.txLaneState == nil {
		return nil, func() {}
	}
	return s.txLaneState.SubscribeLaneState()
}

func (s *Solver) suspendQuotes(ctx context.Context, state *quoteState) {
	timer := observability.StartOperation(s.operationObservers().quoteSuspend)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()

	backoff := initialQuoteSuspensionBackoff
	for {
		removed, err := state.reconcile(ctx, s.orders, nil, s.wallNow())
		if err == nil {
			outcome = observability.ExternalOperationSuccess
			s.observeQuoteRefresh(state)
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
	timer := observability.StartOperation(s.operationObservers().quoteRefresh)
	outcome := observability.ExternalOperationError
	defer func() { timer.Finish(ctx, outcome) }()

	if !s.transactionLaneReady() {
		outcome = observability.ExternalOperationSkipped
		s.log.V(1).Info(
			"quote refresh skipped: transaction lane unavailable",
			"activePairs", len(state.active),
			"pendingFills", s.capacity.Len(),
		)
		timer.Finish(ctx, outcome)
		s.suspendQuotes(ctx, state)
		return
	}
	chainTime, err := s.now(ctx)
	if err != nil {
		s.log.Error(err, "quote refresh: read latest block time")
		return
	}
	snapshotSet, err := s.reader.Quote(ctx, routes, s.cfg.Executor, chainTime)
	if err != nil {
		s.log.Error(err, "quote refresh: read routes")
		return
	}
	maxFeePerGas := new(big.Int)
	if s.cfg.Gas != nil {
		maxFeePerGas, err = s.readMaxFeePerGas(ctx)
		if err != nil {
			s.log.Error(err, "quote refresh: read max fee per gas")
			return
		}
	}
	direct := filterQuoteInventory(snapshotSet.Direct, s.cfg.TokenPolicy)
	discountBases := filterQuoteInventory(snapshotSet.Physical, s.cfg.TokenPolicy)
	inventory := append([]liquidlane.Inventory(nil), direct...)
	discountInventory, discountDegraded := s.quoteDiscountInventories(ctx, discountBases, chainTime)
	inventory = append(inventory, discountInventory...)
	reservations := s.capacity.Snapshot()
	serverTime := s.wallNow()
	out, err := s.strategy.DecideQuotes(ctx, types.QuoteInput{
		Solver:            s.cfg.Executor,
		Inventory:         inventory,
		Reservations:      reservations,
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
	earliestExpiry, latestExpiry := quoteExpiryBounds(out.Quotes)
	if len(out.Quotes) == 0 {
		s.log.V(1).Info(
			"quote refresh: strategy produced no quotes",
			"inventory", len(inventory),
			"directInventory", len(direct),
			"physicalInventory", len(discountBases),
			"discountInventory", len(inventory)-len(direct),
			"reservationDomains", len(reservations),
			"gasAccounting", s.cfg.Gas != nil,
			"pricingMaxFeePerGas", maxFeePerGas.String(),
		)
	} else {
		s.log.V(1).Info(
			"quote plan selected",
			"quotePairs", len(out.Quotes),
			"quoteRanges", quoteRangeCount(out.Quotes),
			"earliestExpiry", earliestExpiry,
			"latestExpiry", latestExpiry,
		)
	}
	if !s.transactionLaneReady() {
		outcome = observability.ExternalOperationSkipped
		s.log.V(1).Info(
			"quote plan discarded: transaction lane unavailable",
			"quotePairs", len(out.Quotes),
			"quoteRanges", quoteRangeCount(out.Quotes),
		)
		timer.Finish(ctx, outcome)
		s.suspendQuotes(ctx, state)
		return
	}
	removed, err := state.reconcile(ctx, s.orders, out.Quotes, serverTime)
	if err != nil {
		s.log.Error(err, "quote refresh: submit quotes", "quotes", len(out.Quotes))
		return
	}
	s.observeQuoteRefresh(state)
	outcome = observability.ExternalOperationSuccess
	if discountDegraded {
		outcome = observability.ExternalOperationDegraded
	}
	s.log.Info(
		"quotes reconciled",
		"quotes", len(out.Quotes),
		"quoteRanges", quoteRangeCount(out.Quotes),
		"activePairs", len(state.active),
		"removedPairs", removed,
		"inventory", len(inventory),
		"directInventory", len(direct),
		"physicalInventory", len(discountBases),
		"discountInventory", len(inventory)-len(direct),
		"reservationDomains", len(reservations),
		"pendingFills", s.capacity.Len(),
		"gasAccounting", s.cfg.Gas != nil,
		"pricingMaxFeePerGas", maxFeePerGas.String(),
		"earliestExpiry", earliestExpiry,
		"latestExpiry", latestExpiry,
	)
}

func quoteRangeCount(quotes []types.Quote) int {
	ranges := 0
	for _, quote := range quotes {
		ranges += len(quote.Ranges)
	}
	return ranges
}

func quoteExpiryBounds(quotes []types.Quote) (earliest, latest int64) {
	for _, quote := range quotes {
		if earliest == 0 || quote.Expiry < earliest {
			earliest = quote.Expiry
		}
		if quote.Expiry > latest {
			latest = quote.Expiry
		}
	}
	return earliest, latest
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

func (s *quoteState) activeQuoteCount() int {
	count := 0
	for _, pair := range s.active {
		count += len(pair.quotes)
	}
	return count
}

func (s *quoteState) reconcile(
	ctx context.Context,
	submitter quoteSubmitter,
	quotes []types.Quote,
	now time.Time,
) (int, error) {
	next := indexQuotePairs(quotes)
	ordered := make([]quotePairKey, 0, len(s.active)+len(next))
	for key := range s.active {
		ordered = append(ordered, key)
	}
	for key := range next {
		if _, exists := s.active[key]; !exists {
			ordered = append(ordered, key)
		}
	}
	slices.SortFunc(ordered, compareQuotePairs)
	var expire, publish []quotePairKey
	var payload []types.Quote
	for _, key := range ordered {
		current, exists := s.active[key]
		upcoming, wanted := next[key]
		switch {
		case !wanted:
			expire = append(expire, key)
			for _, quote := range current.quotes {
				quote.Expiry = now.Add(-time.Second).Unix()
				payload = append(payload, quote)
			}
		case !exists || shouldReplaceQuotePair(current, upcoming, now, s.renewBefore):
			publish = append(publish, key)
		}
	}
	// Retire vanished pairs before advertising replacements in the same request.
	for _, key := range publish {
		payload = append(payload, next[key].quotes...)
	}
	if len(payload) == 0 {
		return 0, nil
	}
	err := submitter.submitQuotes(ctx, payload)
	for _, key := range publish {
		pair := next[key]
		// A lost response does not prove rejection. Retain every attempted pair so
		// disconnect/shutdown can expire it, and force its renewal on the next attempt.
		if err != nil {
			pair.expiry = 0
		}
		s.active[key] = pair
	}
	if err == nil {
		for _, key := range expire {
			delete(s.active, key)
		}
	}
	return len(expire), err
}

func shouldReplaceQuotePair(current, upcoming quotePairState, now time.Time, renewBefore time.Duration) bool {
	if current.fingerprint != upcoming.fingerprint || upcoming.expiry < current.expiry {
		return true
	}
	return current.expiry <= now.Add(renewBefore).Unix()
}

func indexQuotePairs(quotes []types.Quote) map[quotePairKey]quotePairState {
	pairs := make(map[quotePairKey]quotePairState)
	for _, quote := range quotes {
		key := pairKey(quote)
		pair := pairs[key]
		pair.quotes = append(pair.quotes, quote)
		if pair.expiry == 0 || quote.Expiry < pair.expiry {
			pair.expiry = quote.Expiry
		}
		pairs[key] = pair
	}
	for key, pair := range pairs {
		fingerprints := make([]string, len(pair.quotes))
		for index, quote := range pair.quotes {
			var text strings.Builder
			text.WriteString(strings.ToLower(quote.ExclusiveFor.Hex()))
			text.WriteByte(':')
			for index, segment := range quote.Ranges {
				if index != 0 {
					text.WriteByte(',')
				}
				text.WriteString(bigString(segment.MinAmount))
				text.WriteByte(':')
				text.WriteString(bigString(segment.MaxAmount))
				text.WriteByte(':')
				text.WriteString(segment.Quote)
			}
			fingerprints[index] = text.String()
		}
		sort.Strings(fingerprints)
		pair.fingerprint = strings.Join(fingerprints, "|")
		pairs[key] = pair
	}
	return pairs
}

func pairKey(quote types.Quote) quotePairKey {
	return quotePairKey{
		fromAsset: quote.FromAsset, toAsset: quote.ToAsset,
		fromDecimals: quote.FromDecimals, toDecimals: quote.ToDecimals,
	}
}

func compareQuotePairs(a, b quotePairKey) int {
	// Address bytes have the same order as lower-case hex. Retain the separator
	// after input decimals: in the original key, "10:" sorts before "1:".
	return cmp.Or(a.fromAsset.Cmp(b.fromAsset), a.toAsset.Cmp(b.toAsset),
		strings.Compare(strconv.Itoa(a.fromDecimals)+":", strconv.Itoa(b.fromDecimals)+":"),
		strings.Compare(strconv.Itoa(a.toDecimals), strconv.Itoa(b.toDecimals)))
}

func bigString(n *big.Int) string {
	if n == nil {
		return "<nil>"
	}
	return n.String()
}
