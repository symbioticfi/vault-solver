package rfq

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/rfq/strategies/types"
)

func TestSwapStoreDiscoveryIsDeepCopiedAndExpires(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	id := uuid.New()
	record := discoveryRecord{
		RequestID: id, QuoteID: uuid.New(), ChainID: 1,
		Swapper: common.HexToAddress(testSwapper), TokenIn: common.HexToAddress(testTokenIn),
		TokenOut: common.HexToAddress(testTokenOut), ExpiresAt: now.Add(time.Minute),
		Points: map[string]discoveryPointRecord{
			"10": {AmountIn: big.NewInt(10), AmountOut: big.NewInt(19), Domains: []liquidlane.CapacityID{"capacity:a"}},
		},
	}
	if err := store.putDiscovery(record); err != nil {
		t.Fatalf("putDiscovery: %v", err)
	}
	record.Points["10"] = discoveryPointRecord{AmountIn: big.NewInt(99)}

	got, err := store.discovery(id)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if got.Points["10"].AmountOut.String() != "19" {
		t.Fatalf("stored point mutated: %+v", got.Points["10"])
	}
	point := got.Points["10"]
	point.AmountOut.SetInt64(1)
	got.Points["10"] = point
	gotAgain, _ := store.discovery(id)
	if gotAgain.Points["10"].AmountOut.String() != "19" {
		t.Fatal("read returned a mutable alias")
	}

	now = now.Add(time.Minute)
	if _, err := store.discovery(id); err != errSwapRecordExpired {
		t.Fatalf("expired discovery error = %v", err)
	}
}

func TestSwapStoreConfirmationIsDeepCopiedAndExpires(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	record := sampleConfirmation(now)
	if err := store.putConfirmation(record); err != nil {
		t.Fatalf("putConfirmation: %v", err)
	}
	record.AmountOut.SetInt64(1)
	record.Plan.Legs[0].AmountIn.SetInt64(1)

	got, err := store.confirmation(record.SolverQuoteID)
	if err != nil {
		t.Fatalf("confirmation: %v", err)
	}
	if got.AmountOut.String() != "19" || got.Plan.Legs[0].AmountIn.String() != "10" {
		t.Fatalf("stored confirmation mutated: %+v", got)
	}
	got.Plan.Legs[0].AmountIn.SetInt64(2)
	gotAgain, _ := store.confirmation(record.SolverQuoteID)
	if gotAgain.Plan.Legs[0].AmountIn.String() != "10" {
		t.Fatal("confirmation read returned a mutable alias")
	}

	now = record.ValidUntil
	if _, err := store.confirmation(record.SolverQuoteID); err != errSwapRecordExpired {
		t.Fatalf("expired confirmation error = %v", err)
	}
}

func TestSwapStoreRejectsWhenTenThousandLiveRecordsExist(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	for i := 0; i < maxSwapRecords; i++ {
		id := uuid.New()
		if err := store.putDiscovery(discoveryRecord{RequestID: id, Points: map[string]discoveryPointRecord{}, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	if err := store.putDiscovery(discoveryRecord{RequestID: uuid.New(), Points: map[string]discoveryPointRecord{}, ExpiresAt: now.Add(time.Hour)}); err != errSwapStoreFull {
		t.Fatalf("overflow error = %v", err)
	}
	now = now.Add(2 * time.Hour)
	if err := store.putDiscovery(discoveryRecord{RequestID: uuid.New(), Points: map[string]discoveryPointRecord{}, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("expired records were not swept: %v", err)
	}
}

func TestSwapStoreBuildLeaseCachesIdenticalResponse(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	record := sampleConfirmation(now)
	if err := store.putConfirmation(record); err != nil {
		t.Fatal(err)
	}
	buildID := uuid.New()
	fingerprint := common.HexToHash("0x01")
	lease, err := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
	if err != nil || lease.Cached() != nil {
		t.Fatalf("first lease = %+v, err %v", lease, err)
	}
	response := sampleBuildResponse()
	lease.Complete(response)
	lease.Release()

	cachedLease, err := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
	if err != nil {
		t.Fatalf("cached lease: %v", err)
	}
	cached := cachedLease.Cached()
	cachedLease.Release()
	if cached == nil || (*cached.Calls)[0].Data != "0x1234" {
		t.Fatalf("cached response = %+v", cached)
	}
	(*cached.Calls)[0].Data = "0xffff"
	again, _ := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
	if (*again.Cached().Calls)[0].Data != "0x1234" {
		t.Fatal("cached response returned a mutable alias")
	}
	again.Release()
}

func TestSwapStoreBuildLeaseRejectsSecondBuildIDAndFingerprintDrift(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	record := sampleConfirmation(now)
	if err := store.putConfirmation(record); err != nil {
		t.Fatal(err)
	}
	buildID := uuid.New()
	fingerprint := common.HexToHash("0x01")
	lease, err := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if _, err := store.acquireBuild(record.SolverQuoteID, uuid.New(), fingerprint); err != errSwapBuildConflict {
		t.Fatalf("second build ID error = %v", err)
	}
	if _, err := store.acquireBuild(record.SolverQuoteID, buildID, common.HexToHash("0x02")); err != errSwapBuildConflict {
		t.Fatalf("fingerprint drift error = %v", err)
	}
}

func TestSwapStoreBuildLeaseSerializesConcurrentIdenticalBuilds(t *testing.T) {
	now := time.Unix(1_000, 0)
	store := newSwapStore(func() time.Time { return now })
	record := sampleConfirmation(now)
	if err := store.putConfirmation(record); err != nil {
		t.Fatal(err)
	}
	buildID := uuid.New()
	fingerprint := common.HexToHash("0x01")
	first, err := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		lease *buildLease
		err   error
	}
	done := make(chan result, 1)
	go func() {
		lease, acquireErr := store.acquireBuild(record.SolverQuoteID, buildID, fingerprint)
		done <- result{lease: lease, err: acquireErr}
	}()
	select {
	case <-done:
		t.Fatal("second build did not wait for the first")
	case <-time.After(20 * time.Millisecond):
	}
	first.Complete(sampleBuildResponse())
	first.Release()
	second := <-done
	if second.err != nil || second.lease.Cached() == nil {
		t.Fatalf("second lease = %+v, err %v", second.lease, second.err)
	}
	second.lease.Release()
}

func sampleConfirmation(now time.Time) confirmationRecord {
	route := liquidlane.NewRoute(
		1, common.HexToAddress(testAdapter), common.HexToAddress(testVault), common.HexToAddress(testTokenIn),
		common.HexToAddress(testTokenOut), 18, 6,
	)
	candidateID := liquidlane.NewCandidateID(route, nil)
	return confirmationRecord{
		SolverQuoteID: uuid.New(), DiscoveryRequestID: uuid.New(), QuoteID: uuid.New(), ChainID: 1,
		Swapper: common.HexToAddress(testSwapper), TokenIn: route.TokenIn, TokenOut: route.TokenOut,
		AmountIn: big.NewInt(10), AmountOut: big.NewInt(19), PublicDeadline: now.Add(30 * time.Second),
		ValidUntil: now.Add(time.Minute), Domains: []liquidlane.CapacityID{route.CapacityID},
		Plan: &fillPlan{
			QuoteID: "quote", RequestID: "request", TokenIn: route.TokenIn, TokenOut: route.TokenOut,
			AmountIn: big.NewInt(10), QuotedAmountOut: big.NewInt(19),
			Legs: []strategytypes.FillLeg{{
				CandidateID: candidateID, Route: route, Adapter: route.Adapter, AmountIn: big.NewInt(10),
				AmountOut: big.NewInt(19), MaxRate: big.NewInt(1_000_000_000_000_000_000),
			}},
		},
	}
}

func sampleBuildResponse() *swapResponse {
	calls := []swapCallResponse{{
		To: testAdapter, Data: "0x1234", AmountIn: "10", AmountOut: "19", TokenOut: testTokenOut,
		LiquidityDomain: "capacity:1:" + testVault + ":" + testTokenOut, ValidUntil: 1_030,
	}}
	return &swapResponse{Protocol: swapProtocolV2, Phase: swapPhaseBuild, Calls: &calls}
}
