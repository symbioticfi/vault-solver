package bridgefacilitator

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/threef"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator/strategies/types"
)

// Test3FR1NormalizationCharacterization is the immutable normalization baseline for the 3F-R1
// differential tournament. Candidate worktrees must run these literal expectations unchanged.
func Test3FR1NormalizationCharacterization(t *testing.T) { //nolint:cyclop,gocognit,maintidx // The centralized immutable matrix intentionally enumerates every legacy branch.
	t.Parallel()

	const (
		requestHex    = "0x0000000000000000000000000000000000000010"
		collateralHex = "0x00000000000000000000000000000000000000d0"
	)
	now := time.Unix(1_700_000_000, 0).UTC()
	adapterA := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	adapterB := common.HexToAddress("0x00000000000000000000000000000000000000B2")

	with := func(change func(*threef.AuctionDto)) threef.AuctionDto {
		dto := characterizationAuctionDTO(12.75, requestHex, collateralHex)
		if change != nil {
			change(&dto)
		}
		return dto
	}
	unsetString := threef.NullableString{}
	nullString := *threef.NewNullableString(nil)
	unsetFloat := threef.NullableFloat32{}
	nullFloat := *threef.NewNullableFloat32(nil)
	unsetDeposit := threef.NullableAuctionDepositAssetDto{}
	nullDeposit := *threef.NewNullableAuctionDepositAssetDto(nil)

	tests := []struct {
		name          string
		dto           threef.AuctionDto
		coverage      int64
		wantOK        bool
		wantID        int64
		wantRemaining string
		wantRate      float64
		wantAsset     common.Address
	}{
		{name: "open", dto: with(nil), wantOK: true, wantID: 12, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "solvable case insensitive", dto: with(func(d *threef.AuctionDto) { d.Status = "SoLvAbLe" }), wantOK: true, wantID: 12, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "whitespace status is closed", dto: with(func(d *threef.AuctionDto) { d.Status = " open " })},
		{name: "invalid request", dto: with(func(d *threef.AuctionDto) { d.RequestId = "request" })},
		{name: "zero request", dto: with(func(d *threef.AuctionDto) { d.RequestId = "0x0000000000000000000000000000000000000000" })},
		{name: "amount absent", dto: with(func(d *threef.AuctionDto) { d.AmountRequested = unsetString })},
		{name: "amount explicit null", dto: with(func(d *threef.AuctionDto) { d.AmountRequested = nullString })},
		{name: "amount invalid decimal", dto: with(func(d *threef.AuctionDto) { d.SetAmountRequested("1.25") })},
		{name: "amount zero", dto: with(func(d *threef.AuctionDto) { d.SetAmountRequested("0") })},
		{name: "amount negative", dto: with(func(d *threef.AuctionDto) { d.SetAmountRequested("-1") })},
		{name: "max rate absent", dto: with(func(d *threef.AuctionDto) { d.MaxRate = unsetFloat })},
		{name: "max rate explicit null", dto: with(func(d *threef.AuctionDto) { d.MaxRate = nullFloat })},
		{name: "max rate resolved zero", dto: with(func(d *threef.AuctionDto) { d.SetMaxRate(0) }), wantOK: true, wantID: 12, wantRemaining: "1000", wantRate: 0, wantAsset: common.HexToAddress(collateralHex)},
		{name: "deposit asset absent", dto: with(func(d *threef.AuctionDto) { d.DepositAsset = unsetDeposit })},
		{name: "deposit asset explicit null", dto: with(func(d *threef.AuctionDto) { d.DepositAsset = nullDeposit })},
		{name: "deposit asset zero value missing address", dto: with(func(d *threef.AuctionDto) { d.SetDepositAsset(threef.AuctionDepositAssetDto{}) })},
		{name: "deposit asset invalid address", dto: with(func(d *threef.AuctionDto) { d.SetDepositAsset(*threef.NewAuctionDepositAssetDto("asset", "BAD", 18)) })},
		{name: "deposit zero address is accepted by common IsHexAddress", dto: with(func(d *threef.AuctionDto) {
			d.SetDepositAsset(*threef.NewAuctionDepositAssetDto("0x0000000000000000000000000000000000000000", "ZERO", 18))
		}), wantOK: true, wantID: 12, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.Address{}},
		{name: "fully covered clamps to zero", dto: with(nil), coverage: 1000, wantOK: true, wantID: 12, wantRemaining: "0", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "over covered clamps to zero", dto: with(nil), coverage: 1001, wantOK: true, wantID: 12, wantRemaining: "0", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "fractional positive id truncates", dto: with(func(d *threef.AuctionDto) { d.Id = 9.75 }), wantOK: true, wantID: 9, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "zero id survives", dto: with(func(d *threef.AuctionDto) { d.Id = 0 }), wantOK: true, wantID: 0, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
		{name: "negative fractional id truncates toward zero", dto: with(func(d *threef.AuctionDto) { d.Id = -4.75 }), wantOK: true, wantID: -4, wantRemaining: "1000", wantRate: 250.5, wantAsset: common.HexToAddress(collateralHex)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			offers := newOfferTracker()
			if test.coverage != 0 {
				seed(offers, adapterA, int64(test.dto.Id), now.Add(time.Hour), test.coverage)
			}
			got, ok := buildAuctionSnapshot(auctionView{test.dto}, 17, offers, now)
			if ok != test.wantOK {
				t.Fatalf("eligible = %t, want %t; snapshot=%+v", ok, test.wantOK, got)
			}
			if !test.wantOK {
				return
			}
			if got.ID != big.NewInt(test.wantID).String() || got.AuctionID != test.wantID || got.OriginalIndex != 17 ||
				got.Request != common.HexToAddress(requestHex) || got.Status != test.dto.Status || got.DepositAsset != test.wantAsset ||
				got.AmountRequested.String() != "1000" || got.RemainingAmount.String() != test.wantRemaining || got.MaxRateBps != test.wantRate {
				t.Fatalf("snapshot changed: %+v", got)
			}
		})
	}

	// A nil tracker would panic if normalization reached coverage. These mixed-malformed rows therefore
	// pin the current short-circuit precedence while also characterizing rejection as silent (no error or
	// logger is part of this pure boundary).
	precedence := []struct {
		name string
		dto  threef.AuctionDto
	}{
		{name: "status before request amount rate and deposit", dto: with(func(d *threef.AuctionDto) {
			d.Status, d.RequestId = "closed", "bad"
			d.AmountRequested = unsetString
			d.MaxRate = unsetFloat
			d.DepositAsset = unsetDeposit
		})},
		{name: "request before amount rate and deposit", dto: with(func(d *threef.AuctionDto) {
			d.RequestId = "bad"
			d.AmountRequested = unsetString
			d.MaxRate = unsetFloat
			d.DepositAsset = unsetDeposit
		})},
		{name: "amount before rate and deposit", dto: with(func(d *threef.AuctionDto) {
			d.AmountRequested = unsetString
			d.MaxRate = unsetFloat
			d.DepositAsset = unsetDeposit
		})},
		{name: "rate before deposit", dto: with(func(d *threef.AuctionDto) { d.MaxRate = unsetFloat; d.DepositAsset = unsetDeposit })},
		{name: "deposit before coverage", dto: with(func(d *threef.AuctionDto) { d.DepositAsset = unsetDeposit })},
	}
	for _, test := range precedence {
		t.Run("precedence "+test.name, func(t *testing.T) {
			t.Parallel()
			if got, ok := buildAuctionSnapshot(auctionView{test.dto}, 0, nil, now); ok || got != (strategytypes.AuctionSnapshot{}) {
				t.Fatalf("mixed malformed row = (%+v, %t), want zero,false", got, ok)
			}
		})
	}

	inputAuctions := []threef.AuctionDto{
		characterizationAuctionDTO(31, requestHex, collateralHex),
		with(func(d *threef.AuctionDto) { d.Id, d.Status = 32, "closed" }),
		with(func(d *threef.AuctionDto) { d.Id, d.RequestId = 33, "bad" }),
		with(func(d *threef.AuctionDto) { d.Id, d.Status = 34, "solvable" }),
		with(func(d *threef.AuctionDto) { d.Id = 35; d.AmountRequested = unsetString }),
		with(func(d *threef.AuctionDto) { d.Id = -6.5 }),
	}
	fundableA, maxA, minA, yieldA := big.NewInt(900), big.NewInt(800), big.NewInt(100), big.NewInt(190)
	fundableB, maxB, minB, yieldB := big.NewInt(700), big.NewInt(600), big.NewInt(50), big.NewInt(210)
	offerings := []*adapterOffering{
		{target: Target{Adapter: adapterA, Vault: common.HexToAddress("0x0000000000000000000000000000000000000A01"), Collateral: common.HexToAddress(collateralHex)}, st: exposureState{fundable: fundableA, openCount: 2, maxAssets: maxA, minAssets: minA, minYieldPpm: yieldA}},
		{target: Target{Adapter: adapterB, Vault: common.HexToAddress("0x0000000000000000000000000000000000000B02"), Collateral: common.HexToAddress(collateralHex)}, st: exposureState{fundable: fundableB, openCount: 3, maxAssets: maxB, minAssets: minB, minYieldPpm: yieldB}},
	}
	tracker := newOfferTracker()
	seed(tracker, adapterA, 31, now.Add(time.Hour), 250)
	input, _ := buildStrategyInput(inputAuctions, offerings, tracker, now)

	if !input.Now.Equal(now) || len(input.Adapters) != 2 || len(input.Auctions) != 3 || len(input.LiveOffers) != 1 {
		t.Fatalf("strategy input cardinality/order changed: %+v", input)
	}
	wantAdapters := []struct {
		id, fundable, maxAssets, minAssets, minYield string
		adapter, vault, collateral                   common.Address
		openCount                                    int
	}{
		{id: "0x00000000000000000000000000000000000000a1", adapter: adapterA, vault: common.HexToAddress("0x0000000000000000000000000000000000000A01"), collateral: common.HexToAddress(collateralHex), fundable: "900", openCount: 2, maxAssets: "800", minAssets: "100", minYield: "190"},
		{id: "0x00000000000000000000000000000000000000b2", adapter: adapterB, vault: common.HexToAddress("0x0000000000000000000000000000000000000B02"), collateral: common.HexToAddress(collateralHex), fundable: "700", openCount: 3, maxAssets: "600", minAssets: "50", minYield: "210"},
	}
	for i, want := range wantAdapters {
		got := input.Adapters[i]
		if got.ID != want.id || got.Adapter != want.adapter || got.Vault != want.vault || got.Collateral != want.collateral ||
			got.Fundable.String() != want.fundable || got.OpenCount != want.openCount || got.MaxAssets.String() != want.maxAssets ||
			got.MinAssets.String() != want.minAssets || got.MinYieldPpm.String() != want.minYield || got.MaxConcurrent != 50 {
			t.Fatalf("adapter %d changed: %+v", i, got)
		}
	}
	wantAuctions := []struct {
		id            string
		auctionID     int64
		originalIndex int
		status        string
		remaining     string
	}{
		{id: "31", auctionID: 31, originalIndex: 0, status: "open", remaining: "750"},
		{id: "34", auctionID: 34, originalIndex: 3, status: "solvable", remaining: "1000"},
		{id: "-6", auctionID: -6, originalIndex: 5, status: "open", remaining: "1000"},
	}
	auctionValues := make(map[*big.Int]string, len(wantAuctions)*2)
	for i, want := range wantAuctions {
		got := input.Auctions[i]
		if got.ID != want.id || got.AuctionID != want.auctionID || got.OriginalIndex != want.originalIndex ||
			got.Request != common.HexToAddress(requestHex) || got.Status != want.status ||
			got.DepositAsset != common.HexToAddress(collateralHex) || got.AmountRequested.String() != "1000" ||
			got.RemainingAmount.String() != want.remaining || got.MaxRateBps != 250.5 {
			t.Fatalf("auction %d changed: %+v", i, got)
		}
		for label, value := range map[string]*big.Int{
			"amountRequested": got.AmountRequested,
			"remainingAmount": got.RemainingAmount,
		} {
			if previous, exists := auctionValues[value]; exists {
				t.Fatalf("auction %d %s aliases %s", i, label, previous)
			}
			auctionValues[value] = fmt.Sprintf("auction %d %s", i, label)
		}
	}
	if input.LiveOffers[0] != (strategytypes.LiveOffer{AdapterID: "0x00000000000000000000000000000000000000a1", AuctionID: 31}) {
		t.Fatalf("single live offer changed: %+v", input.LiveOffers)
	}
	if input.Adapters[0].Fundable == fundableA || input.Adapters[0].MaxAssets == maxA || input.Adapters[0].MinAssets == minA || input.Adapters[0].MinYieldPpm == yieldA ||
		input.Auctions[0].AmountRequested == input.Auctions[0].RemainingAmount {
		t.Fatal("strategy input big.Int values alias source or each other; want independent copies")
	}
	fundableA.SetInt64(1)
	maxA.SetInt64(1)
	minA.SetInt64(1)
	yieldA.SetInt64(1)
	if input.Adapters[0].Fundable.String() != "900" || input.Adapters[0].MaxAssets.String() != "800" || input.Adapters[0].MinAssets.String() != "100" || input.Adapters[0].MinYieldPpm.String() != "190" {
		t.Fatalf("source mutation leaked into strategy input: %+v", input.Adapters[0])
	}
}

func characterizationAuctionDTO(id float32, request, collateral string) threef.AuctionDto {
	amount := "1000"
	maxRate := float32(250.5)
	name, version := "characterization-domain", "9.8.7"
	chainID := float32(11155111)
	return threef.AuctionDto{
		Id:              id,
		RequestId:       request,
		AmountRequested: *threef.NewNullableString(&amount),
		MaxRate:         *threef.NewNullableFloat32(&maxRate),
		Status:          "open",
		DepositAsset: *threef.NewNullableAuctionDepositAssetDto(
			threef.NewAuctionDepositAssetDto(collateral, "USDC", 6),
		),
		Eip712Domain: *threef.NewNullableAuctionEip712DomainDto(threef.NewAuctionEip712DomainDto(
			*threef.NewNullableString(&name),
			*threef.NewNullableString(&version),
			*threef.NewNullableFloat32(&chainID),
		)),
	}
}
