package rfq

import (
	"context"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	defaultstrategy "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/default"
)

func TestSwapDirectLifecycleBuildsRouterBoundSignedCall(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)

	discovery := discoverySwapRequest()
	discoveryResponse, err := service.swap(t.Context(), &discovery)
	if err != nil {
		t.Fatalf("DISCOVERY: %v", err)
	}
	if discoveryResponse.Points == nil || len(*discoveryResponse.Points) != 2 ||
		(*discoveryResponse.Points)[0].AmountIn != "40" || (*discoveryResponse.Points)[0].AmountOut != "80" ||
		(*discoveryResponse.Points)[1].AmountIn != "100" || (*discoveryResponse.Points)[1].AmountOut != "200" {
		t.Fatalf("discovery response = %+v", discoveryResponse)
	}
	if reader.calls != 1 || reader.amounts[0].String() != "100" {
		t.Fatalf("discovery candidate reads = %d, amounts %v", reader.calls, reader.amounts)
	}

	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmResponse, err := service.swap(t.Context(), &confirm)
	if err != nil {
		t.Fatalf("CONFIRM: %v", err)
	}
	if confirmResponse.SolverQuoteID != testSolverQuoteID || confirmResponse.AmountIn != "100" ||
		confirmResponse.AmountOut != "200" || confirmResponse.ValidUntil != 1_020 ||
		len(confirmResponse.LiquidityDomains) != 1 {
		t.Fatalf("confirm response = %+v", confirmResponse)
	}

	build := buildSwapRequest(confirmResponse)
	buildResponse, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatalf("BUILD: %v", err)
	}
	if buildResponse.Calls == nil || len(*buildResponse.Calls) != 1 {
		t.Fatalf("build response = %+v", buildResponse)
	}
	call := (*buildResponse.Calls)[0]
	if call.To != testAdapter || call.AmountIn != "100" || call.AmountOut != "200" ||
		call.TokenOut != testTokenOut || len(call.Data) <= 10 || call.Data[:10] != "0x9a4568b6" {
		t.Fatalf("signed call = %+v", call)
	}
	decoded, _ := unpackSignedCall(t, common.FromHex(call.Data))
	if decoded.Recipient != common.HexToAddress(testRouter) || decoded.Caller != common.HexToAddress(testRouter) ||
		decoded.Signer != signer.Address() || decoded.Deadline.Cmp(big.NewInt(1_020)) != 0 {
		t.Fatalf("decoded signed call = %+v", decoded)
	}
	if signer.calls != 1 || state.nonceReads != 1 || state.fillReads != 1 {
		t.Fatalf("build dependencies: signer=%d nonce=%d fill=%d", signer.calls, state.nonceReads, state.fillReads)
	}

	retry, err := service.swap(t.Context(), &build)
	if err != nil || (*retry.Calls)[0].Data != call.Data {
		t.Fatalf("idempotent BUILD = %+v, err %v", retry, err)
	}
	if signer.calls != 1 || state.nonceReads != 1 || state.fillReads != 1 {
		t.Fatal("cached BUILD repeated reads or signing")
	}
}

func TestSwapBuildRetryWithFreshRequestIDEchoesNewEnvelope(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_020)))
	if err != nil {
		t.Fatal(err)
	}
	build := buildSwapRequest(confirmed)
	first, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatal(err)
	}

	retry := build
	retry.RequestID = uuid.NewString()
	second, err := service.swap(t.Context(), &retry)
	if err != nil {
		t.Fatalf("retry with fresh requestId: %v", err)
	}
	if second.RequestID != retry.RequestID || second.RequestID == first.RequestID {
		t.Fatalf("retry requestId = %s, want %s and not %s", second.RequestID, retry.RequestID, first.RequestID)
	}
	firstCall, secondCall := (*first.Calls)[0], (*second.Calls)[0]
	if secondCall.Data != firstCall.Data {
		t.Fatalf("retry rebuilt immutable payload: first=%+v second=%+v", firstCall, secondCall)
	}
	if signer.calls != 1 || state.fillReads != 1 || state.nonceReads != 1 {
		t.Fatalf("retry repeated dependencies: signer=%d fill=%d nonce=%d", signer.calls, state.fillReads, state.nonceReads)
	}
}

func TestSwapBuildConcurrentFreshRequestIDsShareImmutablePayload(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	state.fillStarted = make(chan struct{})
	fillRelease := make(chan struct{})
	state.fillRelease = fillRelease
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_020)))
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := buildSwapRequest(confirmed)
	secondRequest := firstRequest
	secondRequest.RequestID = uuid.NewString()

	type buildResult struct {
		response *swapResponse
		err      error
	}
	firstDone := make(chan buildResult, 1)
	secondDone := make(chan buildResult, 1)
	go func() {
		response, buildErr := service.swap(t.Context(), &firstRequest)
		firstDone <- buildResult{response: response, err: buildErr}
	}()
	<-state.fillStarted
	go func() {
		response, buildErr := service.swap(t.Context(), &secondRequest)
		secondDone <- buildResult{response: response, err: buildErr}
	}()
	select {
	case result := <-secondDone:
		t.Fatalf("concurrent retry returned before immutable payload existed: %+v", result)
	case <-time.After(20 * time.Millisecond):
	}
	close(fillRelease)
	first, second := <-firstDone, <-secondDone
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent builds failed: first=%v second=%v", first.err, second.err)
	}
	if first.response.RequestID != firstRequest.RequestID || second.response.RequestID != secondRequest.RequestID {
		t.Fatalf("response envelopes = %s/%s, want %s/%s", first.response.RequestID, second.response.RequestID, firstRequest.RequestID, secondRequest.RequestID)
	}
	firstCall, secondCall := (*first.response.Calls)[0], (*second.response.Calls)[0]
	if firstCall.Data != secondCall.Data {
		t.Fatalf("concurrent builds returned different payloads: first=%+v second=%+v", firstCall, secondCall)
	}
	if signer.calls != 1 || state.fillReads != 1 || state.nonceReads != 1 {
		t.Fatalf("concurrent retry repeated dependencies: signer=%d fill=%d nonce=%d", signer.calls, state.fillReads, state.nonceReads)
	}
}

func TestSwapDiscoveryReturnsEmptyPointsWithoutLiquidity(t *testing.T) {
	now := time.Unix(1_000, 0)
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{nil}}
	service := newTestSwapService(now, reader, newFakeSwapState(directSwapCandidate().Route), newSwapTestSigner(t))
	request := discoverySwapRequest()
	response, err := service.swap(t.Context(), &request)
	if err != nil || response.Points == nil || len(*response.Points) != 0 {
		t.Fatalf("empty discovery = %+v, err %v", response, err)
	}
}

func TestSwapConfirmReturnsNoContentWhenExactInputCannotBeCovered(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, nil}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	if response, err := service.swap(t.Context(), &confirm); !errors.Is(err, errSwapNoContent) || response != nil {
		t.Fatalf("confirm response = %+v, err %v", response, err)
	}
}

func TestSwapConfirmRequiresExactDiscoveryTupleAndFloor(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	service := newTestSwapService(
		now, &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}}},
		newFakeSwapState(candidate.Route), newSwapTestSigner(t),
	)
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirm.MinAmountOut = stringPtr("199")
	if _, err := service.swap(t.Context(), &confirm); err == nil {
		t.Fatal("changed discovery floor was accepted")
	}
}

func TestSwapConfirmCapsRequestedDeadlineToQuoteValidity(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}

	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_120)))
	if err != nil {
		t.Fatalf("CONFIRM with requested maximum: %v", err)
	}
	if confirmed.ValidUntil != 1_030 {
		t.Fatalf("validUntil = %d, want solver cap 1030", confirmed.ValidUntil)
	}
}

func TestSwapBuildUsesChosenDeadlineWithinConfirmationValidity(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_120)))
	if err != nil {
		t.Fatal(err)
	}

	build := buildSwapRequest(confirmed)
	build.Deadline = int64Ptr(1_025)
	built, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatalf("BUILD with chosen deadline: %v", err)
	}
	call := (*built.Calls)[0]
	decoded, _ := unpackSignedCall(t, common.FromHex(call.Data))
	if built.ValidUntil != 1_025 || call.ValidUntil != 1_025 || decoded.Deadline.Cmp(big.NewInt(1_025)) != 0 {
		t.Fatalf("chosen deadline not propagated: response=%d call=%+v signed=%s", built.ValidUntil, call, decoded.Deadline)
	}
}

func TestSwapBuildRejectsDeadlineOutsideConfirmationValidity(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_120)))
	if err != nil {
		t.Fatal(err)
	}

	for _, deadline := range []int64{1_000, 1_031} {
		build := buildSwapRequest(confirmed)
		build.RequestID = uuid.NewString()
		build.Deadline = int64Ptr(deadline)
		response, buildErr := service.swap(t.Context(), &build)
		var serviceErr *swapServiceError
		if response != nil || !errors.As(buildErr, &serviceErr) || serviceErr.status != http.StatusGone {
			t.Fatalf("deadline %d response = %+v, err %v", deadline, response, buildErr)
		}
	}
}

func TestSwapBuildRechecksChosenDeadlineBeforeReturning(t *testing.T) {
	current := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	service := newTestSwapService(current, reader, state, newSwapTestSigner(t))
	service.now = func() time.Time { return current }
	service.store.now = service.now
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.swap(t.Context(), ptrBuild(confirmSwapRequest(discovery.RequestID, 1_020)))
	if err != nil {
		t.Fatal(err)
	}
	state.afterFill = func() { current = time.Unix(1_020, 0) }

	response, err := service.swap(t.Context(), ptrBuild(buildSwapRequest(confirmed)))
	var serviceErr *swapServiceError
	if response != nil || !errors.As(err, &serviceErr) || serviceErr.status != http.StatusGone {
		t.Fatalf("late BUILD response = %+v, err %v", response, err)
	}
}

func TestSwapBuildRejectsChangedTupleAndSecondBuildID(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	service := newTestSwapService(now, reader, newFakeSwapState(candidate.Route), newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmed, err := service.swap(t.Context(), &confirm)
	if err != nil {
		t.Fatal(err)
	}
	build := buildSwapRequest(confirmed)
	changed := build
	changed.MinAmountOut = stringPtr("199")
	if _, err := service.swap(t.Context(), &changed); err == nil {
		t.Fatal("changed confirmed floor was accepted")
	}
	if _, err := service.swap(t.Context(), &build); err != nil {
		t.Fatal(err)
	}
	changedDeadline := build
	changedDeadline.RequestID = uuid.NewString()
	changedDeadline.Deadline = int64Ptr(1_019)
	if _, err := service.swap(t.Context(), &changedDeadline); err == nil {
		t.Fatal("same build ID changed its chosen deadline")
	}
	build.BuildID = stringPtr(uuid.NewString())
	if _, err := service.swap(t.Context(), &build); err == nil {
		t.Fatal("second build ID was accepted")
	}
}

func TestSwapBuildRejectsUsedNonceAndStaleLeg(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := directSwapCandidate()
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	service := newTestSwapService(now, reader, state, newSwapTestSigner(t))
	discovery := discoverySwapRequest()
	_, _ = service.swap(t.Context(), &discovery)
	confirm := confirmSwapRequest(discovery.RequestID, 1_020)
	confirmed, _ := service.swap(t.Context(), &confirm)
	build := buildSwapRequest(confirmed)
	state.used = []bool{true}
	if _, err := service.swap(t.Context(), &build); err == nil {
		t.Fatal("used deterministic nonce was accepted")
	}
}

func TestValidateSwapPlanRejectsMoreThan64Legs(t *testing.T) {
	count := maxSwapCalls + 1
	legs := make([]fillLeg, count)
	candidates := make([]liquidlane.QuoteCandidate, count)
	for i := range count {
		route := liquidlane.NewRoute(
			1,
			common.BigToAddress(big.NewInt(int64(100+i))),
			common.BigToAddress(big.NewInt(int64(1_000+i))),
			common.HexToAddress(testTokenIn),
			common.HexToAddress(testTokenOut),
			0,
			0,
		)
		candidateID := liquidlane.NewCandidateID(route, nil)
		candidates[i] = liquidlane.QuoteCandidate{
			ID: candidateID, Route: route, MaxAmountIn: big.NewInt(1), MaxAmountOut: big.NewInt(2),
		}
		legs[i] = fillLeg{
			CandidateID: candidateID, Route: route, Adapter: route.Adapter,
			AmountIn: big.NewInt(1), AmountOut: big.NewInt(2),
		}
	}
	plan := &fillPlan{AmountIn: big.NewInt(int64(count)), Legs: legs}
	if domains, err := validateSwapPlan(plan, plan.AmountIn, candidates); err == nil {
		t.Fatalf("%d-leg plan produced %d domains", count, len(domains))
	}
}

func TestDirectPlanAdaptersExcludesDiscountLegs(t *testing.T) {
	directAdapter := common.HexToAddress(testAdapter)
	discountAdapter := common.HexToAddress("0x8888888888888888888888888888888888888888")
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	got := directPlanAdapters(&fillPlan{Legs: []fillLeg{
		{Adapter: directAdapter},
		{Adapter: discountAdapter, DiscountID: &discountID},
	}})
	if len(got) != 1 || got[0] != directAdapter {
		t.Fatalf("directPlanAdapters = %v, want [%s]", got, directAdapter.Hex())
	}
}

func TestSwapBuildReturnsResolvedDiscountCall(t *testing.T) {
	now := time.Unix(1_000, 0)
	candidate := discountedSwapCandidate(now.Add(30 * time.Second))
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)
	service.discountsEnabled = true
	provider := &fakeSwapDiscountProvider{resolved: resolvedSwapDiscount(candidate, 0, 1_021, 1_022)}
	service.discounts = provider

	discovery := discoverySwapRequestWithDiscount(candidate)
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatal(err)
	}
	confirm := confirmSwapRequestWithAdapters(discovery.RequestID, 1_020, discovery.Adapters)
	confirmed, err := service.swap(t.Context(), &confirm)
	if err != nil {
		t.Fatal(err)
	}
	build := buildSwapRequest(confirmed)
	response, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatalf("BUILD discount: %v", err)
	}
	if response.Calls == nil || len(*response.Calls) != 1 || (*response.Calls)[0].Data[:10] != "0x8fa5c671" {
		t.Fatalf("discount response = %+v", response)
	}
	call := (*response.Calls)[0]
	decoded, protocolSignature, recipient, amountIn := unpackDiscountCall(t, common.FromHex(call.Data))
	if call.To != testAdapter || recipient != common.HexToAddress(testRouter) || amountIn.Cmp(big.NewInt(100)) != 0 ||
		decoded.Discount.TokenToRedeem != common.HexToAddress(testTokenIn) ||
		decoded.Discount.Discount.Sign() != 0 || decoded.Discount.Nonce.Cmp(big.NewInt(7)) != 0 ||
		decoded.Discount.Deadline.Cmp(big.NewInt(1_021)) != 0 ||
		decoded.ProtocolDeadline.Cmp(big.NewInt(1_022)) != 0 || len(protocolSignature) == 0 {
		t.Fatalf("Router-bound adapter discount swap = %+v, decoded %+v", call, decoded)
	}
	if signer.calls != 0 || provider.resolveCalls != 1 || state.nonceReads != 1 {
		t.Fatalf(
			"discount dependencies: signer=%d resolve=%d nonce=%d",
			signer.calls,
			provider.resolveCalls,
			state.nonceReads,
		)
	}
	if len(state.validatedAdapters) != 2 || len(state.validatedAdapters[0]) != 0 ||
		len(state.validatedAdapters[1]) != 0 {
		t.Fatalf("discount adapter entered direct authorization: %v", state.validatedAdapters)
	}
	retry, err := service.swap(t.Context(), &build)
	if err != nil || (*retry.Calls)[0].Data != call.Data {
		t.Fatalf("cached discount BUILD = %+v, err %v", retry, err)
	}
	if provider.resolveCalls != 1 || state.fillReads != 1 || state.nonceReads != 1 {
		t.Fatalf(
			"cached discount BUILD repeated dependencies: resolve=%d fill=%d nonce=%d",
			provider.resolveCalls,
			state.fillReads,
			state.nonceReads,
		)
	}
}

func TestSwapBuildPreservesMixedAuthorizationOrder(t *testing.T) {
	now := time.Unix(1_000, 0)
	tokenIn := common.HexToAddress(testTokenIn)
	tokenOut := common.HexToAddress(testTokenOut)
	discountCandidate := discountedSwapCandidate(now.Add(30 * time.Second))
	directRoute := liquidlane.NewRoute(
		1,
		common.HexToAddress("0x4444444444444444444444444444444444444444"),
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		tokenIn,
		tokenOut,
		0,
		0,
	)
	directID := liquidlane.NewCandidateID(directRoute, nil)
	discountLeg := fillLeg{
		CandidateID: discountCandidate.ID,
		Route:       discountCandidate.Route,
		Adapter:     discountCandidate.Route.Adapter,
		AmountIn:    big.NewInt(40),
		AmountOut:   big.NewInt(80),
		DiscountID:  liquidlane.CloneHash(discountCandidate.DiscountID),
	}
	directLeg := fillLeg{
		CandidateID: directID,
		Route:       directRoute,
		Adapter:     directRoute.Adapter,
		AmountIn:    big.NewInt(60),
		AmountOut:   big.NewInt(120),
	}
	directDomain := sampleSwapDomain()
	directDomain.VerifyingContract = directRoute.Adapter
	state := &fakeSwapState{
		domains: map[common.Address]swapDomain{directRoute.Adapter: directDomain},
		fills: map[liquidlane.RouteID]liquidlane.FillQuote{
			discountLeg.Route.ID: {
				Inventory:      liquidlane.DirectInventory(discountLeg.Route, big.NewInt(1_000), big.NewInt(1)),
				AmountIn:       big.NewInt(40),
				GrossAmountOut: big.NewInt(80),
				MaxAmountOut:   big.NewInt(80),
				MinDiscount:    big.NewInt(0),
			},
			directLeg.Route.ID: {
				Inventory:      liquidlane.DirectInventory(directLeg.Route, big.NewInt(1_000), big.NewInt(1)),
				AmountIn:       big.NewInt(60),
				GrossAmountOut: big.NewInt(120),
				MaxAmountOut:   big.NewInt(120),
				MinDiscount:    big.NewInt(0),
			},
		},
	}
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, &fakeSwapCandidateReader{}, state, signer)
	provider := &fakeSwapDiscountProvider{resolved: resolvedSwapDiscount(discountCandidate, 0, 1_021, 1_022)}
	service.discounts = provider
	domains := []liquidlane.CapacityID{discountLeg.Route.CapacityID, directLeg.Route.CapacityID}
	record := confirmationRecord{
		SolverQuoteID:      uuid.MustParse(testSolverQuoteID),
		DiscoveryRequestID: uuid.MustParse(testDiscoveryRequestID),
		QuoteID:            uuid.MustParse(testQuoteID),
		ChainID:            1,
		Swapper:            common.HexToAddress(testSwapper),
		TokenIn:            tokenIn,
		TokenOut:           tokenOut,
		AmountIn:           big.NewInt(100),
		AmountOut:          big.NewInt(200),
		ValidUntil:         time.Unix(1_030, 0),
		Domains:            domains,
		Plan: &fillPlan{
			TokenIn: tokenIn, TokenOut: tokenOut, AmountIn: big.NewInt(100), QuotedAmountOut: big.NewInt(200),
			Legs: []fillLeg{discountLeg, directLeg},
		},
	}
	if err := service.store.putConfirmation(record); err != nil {
		t.Fatal(err)
	}
	build := buildSwapRequest(&swapResponse{
		SolverQuoteID:    testSolverQuoteID,
		AmountIn:         "100",
		AmountOut:        "200",
		LiquidityDomains: capacityStrings(domains),
	})

	response, err := service.swap(t.Context(), &build)
	if err != nil {
		t.Fatalf("mixed BUILD: %v", err)
	}
	if response.Calls == nil || len(*response.Calls) != 2 || (*response.Calls)[0].Data[:10] != "0x8fa5c671" ||
		(*response.Calls)[1].Data[:10] != "0x9a4568b6" {
		t.Fatalf("mixed call order = %+v", response)
	}
	if len(state.validatedAdapters) != 1 || len(state.validatedAdapters[0]) != 1 ||
		state.validatedAdapters[0][0] != directRoute.Adapter {
		t.Fatalf("direct authorization = %v, want only %s", state.validatedAdapters, directRoute.Adapter.Hex())
	}
	directSwap, _ := unpackSignedCall(t, common.FromHex((*response.Calls)[1].Data))
	wantNonce := signedSwapNonce(uuid.MustParse(testBuildID), 1, directRoute.Adapter, tokenIn, 1)
	if directSwap.Nonce.Cmp(wantNonce) != 0 {
		t.Fatalf("direct nonce = %s, want full-plan index nonce %s", directSwap.Nonce, wantNonce)
	}
	if signer.calls != 1 || provider.resolveCalls != 1 {
		t.Fatalf("mixed authorizations: signer=%d resolve=%d", signer.calls, provider.resolveCalls)
	}
}

func TestSwapBuildRejectsStaleOrMalformedResolvedDiscount(t *testing.T) {
	tests := []struct {
		name   string
		status int
		mutate func(*fakeSwapDiscountProvider, *fakeSwapState)
	}{
		{
			name: "provider error", status: http.StatusBadGateway,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.err = errors.New("backend unavailable")
			},
		},
		{
			name: "malformed payload", status: http.StatusBadGateway,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.Discount.Adapter = "not-an-address"
			},
		},
		{
			name: "mismatched id", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.DiscountID = common.HexToHash("0xbb").Hex()
			},
		},
		{
			name: "mismatched adapter", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.Discount.Adapter = "0x8888888888888888888888888888888888888888"
			},
		},
		{
			name: "mismatched token", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.Discount.TokenToRedeem = testTokenOut
			},
		},
		{
			name: "discount deadline does not cover build", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.Discount.Deadline = 1_020
			},
		},
		{
			name: "protocol deadline does not cover build", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.ProtocolDeadline = 1_020
			},
		},
		{
			name: "below current minimum discount", status: http.StatusConflict,
			mutate: func(_ *fakeSwapDiscountProvider, state *fakeSwapState) {
				state.fill.MinDiscount = big.NewInt(1)
			},
		},
		{
			name: "discounted output below leg floor", status: http.StatusConflict,
			mutate: func(provider *fakeSwapDiscountProvider, _ *fakeSwapState) {
				provider.resolved.Discount.Discount = "1"
			},
		},
		{
			name: "discount nonce invalidated", status: http.StatusConflict,
			mutate: func(_ *fakeSwapDiscountProvider, state *fakeSwapState) {
				state.used = []bool{true}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, build, provider, state, signer, candidate := newDiscountBuildFixture(t)
			test.mutate(provider, state)

			response, err := service.swap(t.Context(), &build)
			var serviceErr *swapServiceError
			if response != nil || !errors.As(err, &serviceErr) || serviceErr.status != test.status {
				t.Fatalf("BUILD response = %+v, err %v, want status %d", response, err, test.status)
			}
			if signer.calls != 0 {
				t.Fatalf("stale discount invoked framework signer %d times", signer.calls)
			}

			provider.err = nil
			provider.resolved = resolvedSwapDiscount(candidate, 0, 1_021, 1_022)
			state.fill.MinDiscount = big.NewInt(0)
			state.used = nil
			retry, retryErr := service.swap(t.Context(), &build)
			if retryErr != nil || retry.Calls == nil || (*retry.Calls)[0].Data[:10] != "0x8fa5c671" {
				t.Fatalf("failed BUILD did not remain retryable: response %+v, err %v", retry, retryErr)
			}
		})
	}
}

func TestSwapBuildConcurrentDiscountRetriesResolveOnce(t *testing.T) {
	service, build, provider, state, _, _ := newDiscountBuildFixture(t)
	provider.resolveStarted = make(chan struct{})
	provider.resolveRelease = make(chan struct{})
	type result struct {
		response *swapResponse
		err      error
	}
	results := make(chan result, 2)
	for i := range 2 {
		request := build
		request.RequestID = uuid.NewString()
		go func() {
			response, err := service.swap(t.Context(), &request)
			results <- result{response: response, err: err}
		}()
		if i == 0 {
			<-provider.resolveStarted
		}
	}
	close(provider.resolveRelease)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent discount BUILD errors: %v / %v", first.err, second.err)
	}
	firstCall := (*first.response.Calls)[0]
	secondCall := (*second.response.Calls)[0]
	if firstCall.Data != secondCall.Data {
		t.Fatalf("concurrent discount calls differ: %+v / %+v", firstCall, secondCall)
	}
	if provider.resolveCalls != 1 || state.fillReads != 1 || state.nonceReads != 1 {
		t.Fatalf(
			"concurrent dependencies: resolve=%d fill=%d nonce=%d",
			provider.resolveCalls,
			state.fillReads,
			state.nonceReads,
		)
	}
}

type fakeSwapCandidateReader struct {
	responses [][]liquidlane.QuoteCandidate
	err       error
	calls     int
	amounts   []*big.Int
}

func (f *fakeSwapCandidateReader) readQuoteCandidates(
	context.Context,
	[]solverInventory,
	common.Address,
	common.Address,
	*big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	panic("amount must be captured by named implementation")
}

func (f *fakeSwapCandidateReader) read(ctx context.Context, inventory []solverInventory, tokenIn, tokenOut common.Address, amount *big.Int) ([]liquidlane.QuoteCandidate, error) {
	_ = ctx
	_ = inventory
	_ = tokenIn
	_ = tokenOut
	f.amounts = append(f.amounts, new(big.Int).Set(amount))
	index := f.calls
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if index >= len(f.responses) {
		index = len(f.responses) - 1
	}
	if index < 0 {
		return nil, nil
	}
	return append([]liquidlane.QuoteCandidate(nil), f.responses[index]...), nil
}

type fakeSwapState struct {
	domains           map[common.Address]swapDomain
	fill              liquidlane.FillQuote
	fills             map[liquidlane.RouteID]liquidlane.FillQuote
	used              []bool
	err               error
	fillReads         int
	nonceReads        int
	validatedAdapters [][]common.Address
	nonceChecks       [][]swapNonceCheck
	afterFill         func()
	fillStarted       chan struct{}
	fillRelease       <-chan struct{}
}

func newFakeSwapState(route liquidlane.Route) *fakeSwapState {
	domain := sampleSwapDomain()
	domain.VerifyingContract = route.Adapter
	return &fakeSwapState{
		domains: map[common.Address]swapDomain{route.Adapter: domain},
		fill: liquidlane.FillQuote{
			Inventory: liquidlane.DirectInventory(route, big.NewInt(1_000), big.NewInt(2_000_000_000_000_000_000)),
			AmountIn:  big.NewInt(100), GrossAmountOut: big.NewInt(200), MaxAmountOut: big.NewInt(200),
			MinDiscount: big.NewInt(0),
		},
	}
}

func (f *fakeSwapState) validateRouter(context.Context, common.Address) error { return f.err }

func (f *fakeSwapState) validateAdapters(
	_ context.Context,
	adapters []common.Address,
	_ common.Address,
) (map[common.Address]swapDomain, error) {
	f.validatedAdapters = append(f.validatedAdapters, append([]common.Address(nil), adapters...))
	if f.err != nil {
		return nil, f.err
	}
	return f.domains, nil
}

func (f *fakeSwapState) readFillQuote(
	_ context.Context,
	route liquidlane.Route,
	_ *big.Int,
) (liquidlane.FillQuote, error) {
	f.fillReads++
	if f.err != nil {
		return liquidlane.FillQuote{}, f.err
	}
	if f.fillStarted != nil {
		close(f.fillStarted)
		<-f.fillRelease
	}
	if f.afterFill != nil {
		f.afterFill()
	}
	if f.fills != nil {
		return cloneFillQuote(f.fills[route.ID]), nil
	}
	return cloneFillQuote(f.fill), nil
}

func (f *fakeSwapState) readUsedNonces(_ context.Context, checks []swapNonceCheck) ([]bool, error) {
	f.nonceReads++
	f.nonceChecks = append(f.nonceChecks, append([]swapNonceCheck(nil), checks...))
	if f.err != nil {
		return nil, f.err
	}
	if f.used != nil {
		return append([]bool(nil), f.used...), nil
	}
	return make([]bool, len(checks)), nil
}

type fakeSwapDiscountProvider struct {
	resolved       *discounts.Resolved
	err            error
	resolveCalls   int
	resolveStarted chan struct{}
	resolveRelease chan struct{}
}

func (*fakeSwapDiscountProvider) ListDiscounts(context.Context) (*discounts.List, error) {
	return nil, nil
}

func (f *fakeSwapDiscountProvider) Resolve(context.Context, string) (*discounts.Resolved, error) {
	f.resolveCalls++
	if f.resolveStarted != nil {
		close(f.resolveStarted)
		<-f.resolveRelease
	}
	return f.resolved, f.err
}

func newDiscountBuildFixture(
	t *testing.T,
) (*swapService, swapRequest, *fakeSwapDiscountProvider, *fakeSwapState, *swapTestSigner, liquidlane.QuoteCandidate) {
	t.Helper()
	now := time.Unix(1_000, 0)
	candidate := discountedSwapCandidate(now.Add(30 * time.Second))
	reader := &fakeSwapCandidateReader{responses: [][]liquidlane.QuoteCandidate{{candidate}, {candidate}}}
	state := newFakeSwapState(candidate.Route)
	signer := newSwapTestSigner(t)
	service := newTestSwapService(now, reader, state, signer)
	service.discountsEnabled = true
	provider := &fakeSwapDiscountProvider{resolved: resolvedSwapDiscount(candidate, 0, 1_021, 1_022)}
	service.discounts = provider
	discovery := discoverySwapRequestWithDiscount(candidate)
	if _, err := service.swap(t.Context(), &discovery); err != nil {
		t.Fatalf("DISCOVERY: %v", err)
	}
	confirmed, err := service.swap(
		t.Context(),
		ptrBuild(confirmSwapRequestWithAdapters(discovery.RequestID, 1_020, discovery.Adapters)),
	)
	if err != nil {
		t.Fatalf("CONFIRM: %v", err)
	}
	return service, buildSwapRequest(confirmed), provider, state, signer, candidate
}

func resolvedSwapDiscount(
	candidate liquidlane.QuoteCandidate,
	discount int64,
	deadline int64,
	protocolDeadline int64,
) *discounts.Resolved {
	return &discounts.Resolved{
		RequestID:  testBuildRequestID,
		DiscountID: candidate.DiscountID.Hex(),
		Discount: discounts.Terms{
			Adapter:       candidate.Route.Adapter.Hex(),
			TokenToRedeem: candidate.Route.TokenIn.Hex(),
			Discount:      big.NewInt(discount).String(),
			Signer:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Protocol:      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Nonce:         "0x7",
			Deadline:      deadline,
		},
		SignerSignature:   "0x0102",
		ProtocolDeadline:  protocolDeadline,
		ProtocolSignature: "0x0304",
	}
}

func newTestSwapService(
	now time.Time,
	reader *fakeSwapCandidateReader,
	state swapStateReader,
	signer *swapTestSigner,
) *swapService {
	return &swapService{
		chainID: 1, executor: common.HexToAddress("0x9999999999999999999999999999999999999999"),
		router: common.HexToAddress(testRouter), quoteTTL: 30 * time.Second, reader: quoteCandidateReaderFunc(reader.read),
		state: state, strategy: defaultstrategy.New(), store: newSwapStore(func() time.Time { return now }),
		signer: signer, now: func() time.Time { return now }, newID: func() uuid.UUID { return uuid.MustParse(testSolverQuoteID) },
		log: logr.Discard(),
	}
}

type quoteCandidateReaderFunc func(context.Context, []solverInventory, common.Address, common.Address, *big.Int) ([]liquidlane.QuoteCandidate, error)

func (f quoteCandidateReaderFunc) readQuoteCandidates(
	ctx context.Context,
	inv []solverInventory,
	tokenIn common.Address,
	tokenOut common.Address,
	amount *big.Int,
) ([]liquidlane.QuoteCandidate, error) {
	return f(ctx, inv, tokenIn, tokenOut, amount)
}

func directSwapCandidate() liquidlane.QuoteCandidate {
	route := liquidlane.NewRoute(
		1, common.HexToAddress(testAdapter), common.HexToAddress(testVault), common.HexToAddress(testTokenIn),
		common.HexToAddress(testTokenOut), 0, 0,
	)
	return liquidlane.QuoteCandidate{
		ID: liquidlane.NewCandidateID(route, nil), Route: route, Rate: big.NewInt(2_000_000_000_000_000_000),
		MaxAmountIn: big.NewInt(100), MaxAmountOut: big.NewInt(200),
	}
}

func discountedSwapCandidate(validUntil time.Time) liquidlane.QuoteCandidate {
	candidate := directSwapCandidate()
	discountID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	candidate.DiscountID = &discountID
	candidate.ID = liquidlane.NewCandidateID(candidate.Route, &discountID)
	candidate.ValidUntil = validUntil
	return candidate
}

func discoverySwapRequest() swapRequest {
	r := baseSwapRequest(swapPhaseDiscovery, testDiscoveryRequestID)
	r.SampleAmountsIn = []string{"40", "100"}
	r.Adapters = []quoteAdapter{{
		Adapter: testAdapter, Asset: testTokenOut, AssetDecimals: 0, MaxAssets: "1000",
		MaxRate: "2000000000000000000",
	}}
	return r
}

func discoverySwapRequestWithDiscount(candidate liquidlane.QuoteCandidate) swapRequest {
	r := discoverySwapRequest()
	id := candidate.DiscountID.Hex()
	r.Adapters[0].DiscountID = &id
	return r
}

func confirmSwapRequest(discoveryID string, deadline int64) swapRequest {
	r := baseSwapRequest(swapPhaseConfirm, testConfirmRequestID)
	r.DiscoveryRequestID, r.AmountIn, r.MinAmountOut = stringPtr(discoveryID), stringPtr("100"), stringPtr("200")
	r.Deadline = int64Ptr(deadline)
	r.Adapters = discoverySwapRequest().Adapters
	return r
}

func confirmSwapRequestWithAdapters(discoveryID string, deadline int64, adapters []quoteAdapter) swapRequest {
	r := confirmSwapRequest(discoveryID, deadline)
	r.Adapters = adapters
	return r
}

func buildSwapRequest(confirmed *swapResponse) swapRequest {
	r := baseSwapRequest(swapPhaseBuild, testBuildRequestID)
	r.SolverQuoteID, r.BuildID = stringPtr(confirmed.SolverQuoteID), stringPtr(testBuildID)
	r.AmountIn, r.MinAmountOut, r.Deadline = stringPtr(confirmed.AmountIn), stringPtr(confirmed.AmountOut), int64Ptr(1_020)
	r.LiquidityDomains = append([]string(nil), confirmed.LiquidityDomains...)
	r.Router = stringPtr(testRouter)
	return r
}

func stringPtr(value string) *string { return &value }

func ptrBuild(value swapRequest) *swapRequest { return &value }
