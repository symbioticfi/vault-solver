package rfq

import (
	"context"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	frameworksigner "github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

var errSwapNoContent = errors.New("swap confirmation has no current liquidity")

type swapServiceError struct {
	status  int
	message string
	cause   error
}

func (e *swapServiceError) Error() string { return e.message }
func (e *swapServiceError) Unwrap() error { return e.cause }

type swapService struct {
	chainID          int64
	executor         common.Address
	router           common.Address
	quoteTTL         time.Duration
	whitelist        adapterWhitelist
	tokenPolicy      tokenpolicy.Policy
	minAmountsIn     map[common.Address]*big.Int
	discountsEnabled bool
	reader           quoteCandidateReader
	state            swapStateReader
	strategy         strategytypes.Strategy
	store            *swapStore
	signer           frameworksigner.Signer
	now              func() time.Time
	newID            func() uuid.UUID
	log              logr.Logger
}

func (s *swapService) swap(ctx context.Context, request *swapRequest) (*swapResponse, error) {
	parsed, err := request.parse(s.chainID, s.router)
	if err != nil {
		return nil, swapError(http.StatusBadRequest, "invalid swap request", err)
	}
	switch parsed.Phase {
	case swapPhaseDiscovery:
		return s.discover(ctx, parsed)
	case swapPhaseConfirm:
		return s.confirm(ctx, parsed)
	case swapPhaseBuild:
		return s.build(ctx, parsed)
	default:
		return nil, swapError(http.StatusBadRequest, "invalid swap phase", nil)
	}
}

func (s *swapService) discover(ctx context.Context, request *parsedSwapRequest) (*swapResponse, error) {
	if err := s.validatePolicy(request); err != nil {
		return nil, err
	}
	inventory := s.swapInventory(request.Inventory)
	points := make([]swapPointResponse, 0, len(request.SampleAmountsIn))
	pointRecords := make(map[string]discoveryPointRecord, len(request.SampleAmountsIn))
	if len(inventory) > 0 {
		largest := request.SampleAmountsIn[len(request.SampleAmountsIn)-1]
		candidates, err := s.reader.readQuoteCandidates(ctx, inventory, request.TokenIn, request.TokenOut, largest)
		if err != nil {
			return nil, swapError(http.StatusBadGateway, "swap liquidity read failed", err)
		}
		for _, amount := range request.SampleAmountsIn {
			plan, domains, planErr := s.decidePlan(ctx, request, amount, nil, candidates)
			if planErr != nil {
				if errors.Is(planErr, errSwapNoContent) {
					continue
				}
				return nil, planErr
			}
			domainStrings := capacityStrings(domains)
			point := swapPointResponse{
				AmountIn: amount.String(), AmountOut: plan.QuotedAmountOut.String(), LiquidityDomains: domainStrings,
			}
			points = append(points, point)
			pointRecords[amount.String()] = discoveryPointRecord{
				AmountIn: liquidlane.CloneBig(amount), AmountOut: liquidlane.CloneBig(plan.QuotedAmountOut),
				Domains: append([]liquidlane.CapacityID(nil), domains...),
			}
		}
	}
	record := discoveryRecord{
		RequestID: request.RequestID, QuoteID: request.QuoteID, ChainID: request.ChainID, Swapper: request.Swapper,
		TokenIn: request.TokenIn, TokenOut: request.TokenOut, Points: pointRecords, ExpiresAt: s.now().Add(s.quoteTTL),
	}
	if err := s.store.putDiscovery(record); err != nil {
		return nil, swapStoreError(err)
	}
	response := swapBaseResponse(request)
	response.Points = &points
	return response, nil
}

func (s *swapService) confirm(ctx context.Context, request *parsedSwapRequest) (*swapResponse, error) {
	if err := s.validatePolicy(request); err != nil {
		return nil, err
	}
	discovery, err := s.store.discovery(request.DiscoveryRequestID)
	if err != nil {
		return nil, swapStoreError(err)
	}
	if discovery.QuoteID != request.QuoteID || discovery.ChainID != request.ChainID ||
		discovery.Swapper != request.Swapper || discovery.TokenIn != request.TokenIn || discovery.TokenOut != request.TokenOut {
		return nil, swapError(http.StatusBadRequest, "swap confirmation does not match discovery", nil)
	}
	point, exists := discovery.Points[request.AmountIn.String()]
	if !exists || point.AmountOut.Cmp(request.MinAmountOut) != 0 {
		return nil, swapError(http.StatusBadRequest, "swap confirmation must select an exact discovery point", nil)
	}
	if !request.Deadline.After(s.now()) {
		return nil, swapError(http.StatusGone, "swap deadline has expired", nil)
	}

	inventory := s.swapInventory(request.Inventory)
	if len(inventory) == 0 {
		return nil, errSwapNoContent
	}
	candidates, err := s.reader.readQuoteCandidates(ctx, inventory, request.TokenIn, request.TokenOut, request.AmountIn)
	if err != nil {
		return nil, swapError(http.StatusBadGateway, "swap liquidity read failed", err)
	}
	plan, domains, err := s.decidePlan(ctx, request, request.AmountIn, request.MinAmountOut, candidates)
	if err != nil {
		return nil, err
	}
	if !equalCapacityDomains(domains, point.Domains) {
		return nil, swapError(http.StatusConflict, "swap liquidity domains changed", nil)
	}
	adapters := planAdapters(plan)
	if _, err := s.state.validateAdapters(ctx, adapters, s.signer.Address()); err != nil {
		return nil, swapError(http.StatusBadGateway, "swap adapter authorization failed", err)
	}

	validUntil := request.Deadline
	if ttl := s.now().Add(s.quoteTTL); ttl.Before(validUntil) {
		validUntil = ttl
	}
	for _, leg := range plan.Legs {
		if !leg.ValidUntil.IsZero() && leg.ValidUntil.Before(validUntil) {
			validUntil = leg.ValidUntil
		}
	}
	if !validUntil.After(s.now()) {
		return nil, swapError(http.StatusGone, "swap confirmation is already expired", nil)
	}

	solverQuoteID := s.newID()
	record := confirmationRecord{
		SolverQuoteID: solverQuoteID, DiscoveryRequestID: request.DiscoveryRequestID, QuoteID: request.QuoteID,
		ChainID: request.ChainID, Swapper: request.Swapper, TokenIn: request.TokenIn, TokenOut: request.TokenOut,
		AmountIn: liquidlane.CloneBig(request.AmountIn), AmountOut: liquidlane.CloneBig(plan.QuotedAmountOut),
		ValidUntil: validUntil,
		Domains:    append([]liquidlane.CapacityID(nil), domains...), Plan: cloneFillPlan(plan),
	}
	if err := s.store.putConfirmation(record); err != nil {
		return nil, swapStoreError(err)
	}
	response := swapBaseResponse(request)
	response.DiscoveryRequestID = request.DiscoveryRequestID.String()
	response.SolverQuoteID = solverQuoteID.String()
	response.AmountIn = request.AmountIn.String()
	response.AmountOut = plan.QuotedAmountOut.String()
	response.LiquidityDomains = capacityStrings(domains)
	response.ValidUntil = validUntil.Unix()
	return response, nil
}

func (s *swapService) build(ctx context.Context, request *parsedSwapRequest) (*swapResponse, error) {
	confirmation, err := s.store.confirmation(request.SolverQuoteID)
	if err != nil {
		return nil, swapStoreError(err)
	}
	if err := s.validateBuildRequest(request, confirmation); err != nil {
		return nil, err
	}
	lease, err := s.store.acquireBuild(request.SolverQuoteID, request.BuildID, buildFingerprint(request))
	if err != nil {
		return nil, swapStoreError(err)
	}
	defer lease.Release()
	if cached := lease.Cached(); cached != nil {
		if !s.now().Before(request.Deadline) {
			return nil, swapError(http.StatusGone, "swap confirmation has expired", nil)
		}
		return swapBuildResponse(request, cached), nil
	}
	record := lease.Record()
	adapters := planAdapters(record.Plan)
	domains, err := s.state.validateAdapters(ctx, adapters, s.signer.Address())
	if err != nil {
		return nil, swapError(http.StatusBadGateway, "swap adapter authorization failed", err)
	}

	quotes := make([]liquidlane.FillQuote, len(record.Plan.Legs))
	for i, leg := range record.Plan.Legs {
		quote, readErr := s.state.readFillQuote(ctx, leg.Route, leg.AmountIn)
		if readErr != nil {
			return nil, swapError(http.StatusConflict, "confirmed swap route is stale", readErr)
		}
		if quote.MaxAmountOut == nil || quote.MaxAmountOut.Cmp(leg.AmountOut) < 0 || quote.MaxAssets == nil {
			return nil, swapError(http.StatusConflict, "confirmed swap leg is below its floor", nil)
		}
		quotes[i] = quote
	}
	amountsOut := make([]*big.Int, len(record.Plan.Legs))
	capacityUsed := make(map[liquidlane.CapacityID]*big.Int)
	capacityAvailable := make(map[liquidlane.CapacityID]*big.Int)
	for i, leg := range record.Plan.Legs {
		amountsOut[i] = liquidlane.CloneBig(quotes[i].MaxAmountOut)
		domain := liquidlane.RouteCapacityID(leg.Route)
		if capacityUsed[domain] == nil {
			capacityUsed[domain] = new(big.Int)
			capacityAvailable[domain] = liquidlane.CloneBig(quotes[i].MaxAssets)
		} else if quotes[i].MaxAssets.Cmp(capacityAvailable[domain]) < 0 {
			capacityAvailable[domain] = liquidlane.CloneBig(quotes[i].MaxAssets)
		}
		capacityUsed[domain].Add(capacityUsed[domain], amountsOut[i])
	}
	for domain, used := range capacityUsed {
		if used.Cmp(capacityAvailable[domain]) > 0 {
			return nil, swapError(http.StatusConflict, "confirmed swap capacity is stale", nil)
		}
	}

	nonces := make([]*big.Int, len(record.Plan.Legs))
	checks := make([]swapNonceCheck, len(record.Plan.Legs))
	for i, leg := range record.Plan.Legs {
		nonces[i] = signedSwapNonce(request.BuildID, record.ChainID, leg.Adapter, record.TokenIn, i)
		checks[i] = swapNonceCheck{Adapter: leg.Adapter, TokenIn: record.TokenIn, Nonce: nonces[i]}
	}
	usedNonces, nonceErr := s.state.readUsedNonces(ctx, checks)
	if nonceErr != nil || len(usedNonces) != len(checks) {
		return nil, swapError(http.StatusBadGateway, "swap nonce read failed", nonceErr)
	}
	for _, used := range usedNonces {
		if used {
			return nil, swapError(http.StatusConflict, "deterministic swap nonce is already used", nil)
		}
	}

	calls := make([]swapCallResponse, len(record.Plan.Legs))
	amountOut := new(big.Int)
	for i, leg := range record.Plan.Legs {
		domain, exists := domains[leg.Adapter]
		if !exists {
			return nil, swapError(http.StatusConflict, "confirmed swap adapter domain disappeared", nil)
		}
		value := adapter.ILiquidLaneAdapterSignedSwap{
			Recipient: s.router, TokenIn: record.TokenIn, AmountIn: liquidlane.CloneBig(leg.AmountIn),
			AmountOut: liquidlane.CloneBig(amountsOut[i]), Caller: s.router, Signer: s.signer.Address(),
			Nonce: nonces[i], Deadline: big.NewInt(request.Deadline.Unix()),
		}
		data, packErr := packSignedSwapCall(s.signer, domain, value)
		if packErr != nil {
			return nil, swapError(http.StatusBadGateway, "swap signing failed", packErr)
		}
		authSignature, authErr := signRouterSwapAuthorization(s.signer, record.ChainID, s.router, routerSwapAuthorization{
			Swapper: record.Swapper, AuthSigner: s.signer.Address(), TokenIn: record.TokenIn,
			Adapter: leg.Adapter, AmountIn: liquidlane.CloneBig(leg.AmountIn), DataHash: crypto.Keccak256Hash(data),
			ExecutionDeadline:     big.NewInt(request.Deadline.Unix()),
			AuthorizationDeadline: big.NewInt(request.Deadline.Unix()),
		})
		if authErr != nil {
			return nil, swapError(http.StatusBadGateway, "Router swap authorization signing failed", authErr)
		}
		amountOut.Add(amountOut, amountsOut[i])
		calls[i] = swapCallResponse{
			To: lowerAddr(leg.Adapter), Data: strings.ToLower(hexutil.Encode(data)),
			AuthSigner: lowerAddr(s.signer.Address()), AuthDeadline: request.Deadline.Unix(),
			AuthSignature: strings.ToLower(hexutil.Encode(authSignature)),
			AmountIn:      leg.AmountIn.String(),
			AmountOut:     amountsOut[i].String(), TokenOut: lowerAddr(record.TokenOut),
			LiquidityDomain: string(liquidlane.RouteCapacityID(leg.Route)), ValidUntil: request.Deadline.Unix(),
		}
	}
	if amountOut.Cmp(record.AmountOut) < 0 {
		return nil, swapError(http.StatusConflict, "built swap output is below the confirmed floor", nil)
	}
	payload := &swapBuildPayload{
		Router: lowerAddr(s.router), AmountIn: record.AmountIn.String(), AmountOut: amountOut.String(),
		LiquidityDomains: capacityStrings(record.Domains), ValidUntil: request.Deadline.Unix(), Calls: calls,
	}
	if !s.now().Before(request.Deadline) {
		return nil, swapError(http.StatusGone, "swap confirmation has expired", nil)
	}
	lease.Complete(payload)
	return swapBuildResponse(request, payload), nil
}

func (s *swapService) decidePlan(
	ctx context.Context,
	request *parsedSwapRequest,
	amountIn *big.Int,
	requiredAmountOut *big.Int,
	candidates []liquidlane.QuoteCandidate,
) (*fillPlan, []liquidlane.CapacityID, error) {
	if len(candidates) == 0 {
		return nil, nil, errSwapNoContent
	}
	input := newQuoteInput(s.chainID, s.executor, strategyRequest{
		RequestID: request.RequestID.String(), QuoteID: request.QuoteID.String(), TokenIn: request.TokenIn,
		TokenOut: request.TokenOut, Amount: amountIn,
	}, cloneQuoteCandidates(candidates), requiredAmountOut, s.tokenPolicy.RequiresSingleRoute(request.TokenIn), s.now())
	output, err := s.strategy.DecideQuote(ctx, input)
	if err != nil {
		return nil, nil, swapError(http.StatusBadGateway, "swap strategy failed", err)
	}
	if output.Decision != strategytypes.DecisionQuote {
		return nil, nil, errSwapNoContent
	}
	plan, err := strategies.FillPlanFromQuote(input, output)
	if err != nil {
		return nil, nil, swapError(http.StatusBadGateway, "swap strategy returned an invalid allocation", err)
	}
	domains, err := validateSwapPlan(plan, amountIn, candidates)
	if err != nil {
		return nil, nil, swapError(http.StatusBadGateway, "swap strategy returned an invalid allocation", err)
	}
	return plan, domains, nil
}

func (s *swapService) validatePolicy(request *parsedSwapRequest) error {
	if !s.tokenPolicy.Allows(request.TokenIn) {
		return swapError(http.StatusBadRequest, "swap input token is outside solver policy", nil)
	}
	amount := request.AmountIn
	if request.Phase == swapPhaseDiscovery {
		amount = request.SampleAmountsIn[0]
	}
	if minimum := s.minAmountsIn[request.TokenIn]; minimum != nil && amount.Cmp(minimum) < 0 {
		return swapError(http.StatusBadRequest, "swap amount is below the configured minimum", nil)
	}
	return nil
}

func (s *swapService) swapInventory(inventory []solverInventory) []solverInventory {
	inventory = s.whitelist.filter(inventory)
	if s.discountsEnabled {
		return inventory
	}
	out := make([]solverInventory, 0, len(inventory))
	for _, item := range inventory {
		if item.DiscountID == nil {
			out = append(out, item)
		}
	}
	return out
}

func (s *swapService) validateBuildRequest(request *parsedSwapRequest, record *confirmationRecord) error {
	if record.QuoteID != request.QuoteID || record.ChainID != request.ChainID || record.Swapper != request.Swapper ||
		record.TokenIn != request.TokenIn || record.TokenOut != request.TokenOut || record.AmountIn.Cmp(request.AmountIn) != 0 ||
		record.AmountOut.Cmp(request.MinAmountOut) != 0 ||
		request.Router != s.router || !equalCapacityDomains(record.Domains, request.LiquidityDomains) {
		return swapError(http.StatusConflict, "swap build does not match its confirmation", nil)
	}
	if request.Deadline.After(record.ValidUntil) || !s.now().Before(request.Deadline) {
		return swapError(http.StatusGone, "swap confirmation has expired", nil)
	}
	return nil
}

func validateSwapPlan(
	plan *fillPlan,
	amountIn *big.Int,
	candidates []liquidlane.QuoteCandidate,
) ([]liquidlane.CapacityID, error) {
	if plan == nil || plan.AmountIn == nil || plan.AmountIn.Cmp(amountIn) != 0 || len(plan.Legs) == 0 {
		return nil, errors.New("allocation does not fully cover input")
	}
	if len(plan.Legs) > maxSwapCalls {
		return nil, errors.Errorf("allocation contains more than %d calls", maxSwapCalls)
	}
	byID := make(map[liquidlane.CandidateID]liquidlane.QuoteCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ID] = candidate
	}
	seenAdapters := make(map[common.Address]bool, len(plan.Legs))
	domainSet := make(map[liquidlane.CapacityID]bool, len(plan.Legs))
	for i, leg := range plan.Legs {
		candidate, exists := byID[leg.CandidateID]
		if !exists || leg.Route != candidate.Route || leg.Adapter != candidate.Route.Adapter ||
			leg.AmountIn == nil || candidate.MaxAmountIn == nil || leg.AmountIn.Cmp(candidate.MaxAmountIn) > 0 {
			return nil, errors.Errorf("allocation leg %d changed candidate identity or exceeds input capacity", i)
		}
		if seenAdapters[leg.Adapter] {
			return nil, errors.Errorf("allocation repeats adapter %s", leg.Adapter.Hex())
		}
		seenAdapters[leg.Adapter] = true
		domain := liquidlane.RouteCapacityID(leg.Route)
		if domain == "" {
			return nil, errors.Errorf("allocation leg %d has no capacity domain", i)
		}
		domainSet[domain] = true
	}
	domains := make([]liquidlane.CapacityID, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	slices.Sort(domains)
	return domains, nil
}

func planAdapters(plan *fillPlan) []common.Address {
	if plan == nil {
		return nil
	}
	out := make([]common.Address, len(plan.Legs))
	for i, leg := range plan.Legs {
		out[i] = leg.Adapter
	}
	return out
}

func cloneQuoteCandidates(candidates []liquidlane.QuoteCandidate) []liquidlane.QuoteCandidate {
	out := make([]liquidlane.QuoteCandidate, len(candidates))
	for i, candidate := range candidates {
		out[i] = candidate
		out[i].Rate = liquidlane.CloneBig(candidate.Rate)
		out[i].MaxAmountIn = liquidlane.CloneBig(candidate.MaxAmountIn)
		out[i].MaxAmountOut = liquidlane.CloneBig(candidate.MaxAmountOut)
		out[i].DiscountID = liquidlane.CloneHash(candidate.DiscountID)
	}
	return out
}

func swapBaseResponse(request *parsedSwapRequest) *swapResponse {
	return &swapResponse{
		Protocol: swapProtocolV2, Phase: request.Phase, RequestID: request.RequestID.String(),
		QuoteID: request.QuoteID.String(), ChainID: request.ChainID, Swapper: lowerAddr(request.Swapper),
		TokenIn: lowerAddr(request.TokenIn), TokenOut: lowerAddr(request.TokenOut),
	}
}

func swapBuildResponse(request *parsedSwapRequest, payload *swapBuildPayload) *swapResponse {
	response := swapBaseResponse(request)
	response.SolverQuoteID = request.SolverQuoteID.String()
	response.BuildID = request.BuildID.String()
	response.Router = payload.Router
	response.AmountIn = payload.AmountIn
	response.AmountOut = payload.AmountOut
	response.LiquidityDomains = append([]string(nil), payload.LiquidityDomains...)
	response.ValidUntil = payload.ValidUntil
	calls := append([]swapCallResponse(nil), payload.Calls...)
	response.Calls = &calls
	return response
}

func capacityStrings(domains []liquidlane.CapacityID) []string {
	out := make([]string, len(domains))
	for i, domain := range domains {
		out[i] = strings.ToLower(string(domain))
	}
	return out
}

func equalCapacityDomains(left, right []liquidlane.CapacityID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func buildFingerprint(request *parsedSwapRequest) common.Hash {
	fields := make([]string, 0, 13+len(request.LiquidityDomains))
	fields = append(fields,
		request.Protocol, string(request.Phase), request.QuoteID.String(),
		request.SolverQuoteID.String(), request.BuildID.String(), strconv.FormatInt(request.ChainID, 10),
		lowerAddr(request.Swapper), lowerAddr(request.TokenIn), lowerAddr(request.TokenOut), request.AmountIn.String(),
		request.MinAmountOut.String(), strconv.FormatInt(request.Deadline.Unix(), 10), lowerAddr(request.Router),
	)
	fields = append(fields, capacityStrings(request.LiquidityDomains)...)
	return crypto.Keccak256Hash([]byte(strings.Join(fields, "\x00")))
}

func swapError(status int, message string, cause error) error {
	return &swapServiceError{status: status, message: message, cause: cause}
}

func swapStoreError(err error) error {
	switch {
	case errors.Is(err, errSwapRecordNotFound):
		return swapError(http.StatusNotFound, "swap record not found", err)
	case errors.Is(err, errSwapRecordExpired):
		return swapError(http.StatusGone, "swap record expired", err)
	case errors.Is(err, errSwapStoreFull):
		return swapError(http.StatusTooManyRequests, "swap record store is full", err)
	case errors.Is(err, errSwapBuildConflict):
		return swapError(http.StatusConflict, "swap build conflicts with its confirmation", err)
	default:
		return swapError(http.StatusBadGateway, "swap state failed", err)
	}
}
