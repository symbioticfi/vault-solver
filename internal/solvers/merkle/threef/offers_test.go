package threef

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type fakeClient struct {
	auctions               []threef.AuctionDto
	offers                 []threef.OfferDto
	created                []threef.CreateOfferDto
	cancelled              []uint64
	identityErr, createErr error
	beforeCreate           func(threef.CreateOfferDto)
	createdIDs             map[string]float32
}

func (f *fakeClient) Auctions(context.Context) ([]threef.AuctionDto, error) { return f.auctions, nil }
func (f *fakeClient) Identity(context.Context, common.Address) error        { return f.identityErr }
func (f *fakeClient) Broker() common.Address {
	return common.HexToAddress("0x8000000000000000000000000000000000000008")
}
func (f *fakeClient) Offers(context.Context) ([]threef.OfferDto, error) { return f.offers, nil }
func (f *fakeClient) Create(_ context.Context, offer threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error) {
	if f.beforeCreate != nil {
		f.beforeCreate(offer)
	}
	if f.createdIDs == nil {
		f.createdIDs = make(map[string]float32)
	}
	key := fmt.Sprintf("%v/%s", offer.AuctionId, offer.Nonce)
	id, exists := f.createdIDs[key]
	if !exists {
		id = float32(15 + len(f.createdIDs))
		f.createdIDs[key] = id
	}
	f.created = append(f.created, offer)
	return &threef.CreateOfferResponseDto{Id: id}, f.createErr
}
func (f *fakeClient) Cancel(_ context.Context, id uint64) (*threef.CancelOfferResponseDto, error) {
	f.cancelled = append(f.cancelled, id)
	return &threef.CancelOfferResponseDto{Id: float32(id), Status: "CANCELLED"}, nil
}

func testMarket() offerMarket {
	request := common.HexToAddress("0x1000000000000000000000000000000000000001")
	return offerMarket{ChainID: 1, Policy: common.HexToAddress("0x50"), Account: common.HexToAddress("0x2000000000000000000000000000000000000002"), Executor: common.HexToAddress("0x3000000000000000000000000000000000000003"), RecipeHash: common.HexToHash("0x04"), Asset: common.HexToAddress("0x5000000000000000000000000000000000000005"), UnderlyingAsset: common.HexToAddress("0x6000000000000000000000000000000000000006"), Budget: big.NewInt(1000000), MinimumYield1e8: big.NewInt(19000), MaxPrincipal: big.NewInt(800001), AvailableCash: big.NewInt(900000), EligibleRequests: map[common.Address]bool{request: true}}
}

func testAuction(m offerMarket, id float32, request common.Address) threef.AuctionDto {
	amount := "10000000"
	deposit := threef.AuctionDepositAssetDto{Address: strings.ToLower(m.Asset.Hex())}
	return threef.AuctionDto{Id: id, RequestId: request.Hex(), Status: "OPEN", RateBasisSeconds: 31536000, AmountRequested: *threef.NewNullableString(&amount), DepositAsset: *threef.NewNullableAuctionDepositAssetDto(&deposit), AdditionalProperties: map[string]any{exactMaxRateKey: "2.0"}}
}

func testOffer(m offerMarket, auctionID float32, status string, amount string) threef.OfferDto {
	return threef.OfferDto{Id: 12, AuctionId: auctionID, Maker: strings.ToLower(m.Account.Hex()), RequestId: common.HexToAddress("0x1000000000000000000000000000000000000001").Hex(), ExecutionMode: "PUSH", Status: status, Amount: amount, ExpectedReturn: "1", FilledAmount: "0", FilledExpectedReturn: "0", PendingAmount: "0", PendingExpectedReturn: "0", Nonce: "10", Expiration: "2000000000"}
}

func testBot(t *testing.T) (*offerBot, *fakeClient, offerMarket) {
	t.Helper()
	m := testMarket()
	client := &fakeClient{auctions: []threef.AuctionDto{testAuction(m, 1, common.HexToAddress("0x1000000000000000000000000000000000000001"))}}
	return &offerBot{Client: client, Send: true, Now: func() time.Time { return time.Unix(1900000000, 0) }}, client, m
}

func sameOffer(t *testing.T, first, second threef.CreateOfferDto) bool {
	t.Helper()
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	return string(a) == string(b)
}

func firstRetainedIntent(bot *offerBot) (intent, bool) {
	if len(bot.pending) == 0 {
		return intent{}, false
	}
	return *bot.pending[0], true
}

func TestTickCreatesStorageCompatibleNonce(t *testing.T) {
	bot, client, market := testBot(t)
	if err := bot.Tick(t.Context(), market, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d offers, want one", len(client.created))
	}
	nonce, ok := new(big.Int).SetString(client.created[0].Nonce, 10)
	if !ok || nonce.Sign() <= 0 || !nonce.IsInt64() {
		t.Fatalf("offer nonce must fit positive int64: %q", client.created[0].Nonce)
	}
	saved, found := firstRetainedIntent(bot)
	if !found || saved.Offer.Nonce != client.created[0].Nonce {
		t.Fatalf("submitted nonce not preserved in RAM: found=%v", found)
	}
}

func TestTickRespectsPolicyAndAuctionMinimum(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal int64
		floor1e8  int64
		maxAPR    string
		basis     float32
		want      string
	}{
		{"auction floor above policy", 100_000_000, 10_000, "100", 31536000, "250000"},
		{"eight percent policy floor", 100_000_000, 8_000_000, "1000", 31536000, "8000000"},
		{"policy floor above auction ceiling", 100_000_000, 8_000_000, "700", 31536000, ""},
		{"daily auction basis", 365_000_000, 0, "100", 86400, "2500"},
		{"rounded auction floor", 100_001, 0, "100", 31536000, "251"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot, client, market := testBot(t)
			market.MaxPrincipal = big.NewInt(tc.principal)
			market.Budget, market.AvailableCash = big.NewInt(tc.principal), big.NewInt(tc.principal)
			market.MinimumYield1e8 = big.NewInt(tc.floor1e8)
			auction := &client.auctions[0]
			amount := market.MaxPrincipal.String()
			auction.AmountRequested = *threef.NewNullableString(&amount)
			auction.RateBasisSeconds = tc.basis
			auction.AdditionalProperties[exactMaxRateKey] = tc.maxAPR
			if err := bot.Tick(t.Context(), market, client.auctions); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(client.created) != 0 {
					t.Fatalf("unexpected offer: %+v", client.created)
				}
				return
			}
			if len(client.created) != 1 || client.created[0].ExpectedReturn != tc.want {
				t.Fatalf("offers = %+v, want expectedReturn %s", client.created, tc.want)
			}
		})
	}
}

func TestTickSizesExactUnsignedPushAndRetainsBeforePost(t *testing.T) {
	b, client, m := testBot(t)
	client.beforeCreate = func(offer threef.CreateOfferDto) {
		saved, found := firstRetainedIntent(b)
		if !found || saved.Offer.Nonce != offer.Nonce {
			t.Fatalf("POST without retained exact intent: %v", found)
		}
	}
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 {
		t.Fatalf("creates = %d", len(client.created))
	}
	got := client.created[0]
	if got.Amount != "800001" || got.ExpectedReturn != "153" || got.Maker != strings.ToLower(client.Broker().Hex()) || got.ExecutionMode != nil || got.UseCallback || got.Signature != nil || got.PushAuthorizationSignature != nil || got.ChainId != nil || got.Expiration != "1900000300" {
		t.Fatalf("incorrect unsigned offer: %+v", got)
	}
	if n, ok := new(big.Int).SetString(got.Nonce, 10); !ok || n.Sign() <= 0 {
		t.Fatalf("invalid nonce %q", got.Nonce)
	}
}

func TestTickReservesOtherRequestsAndTargetsBeforeSizing(t *testing.T) {
	for _, status := range []string{"SUBMITTED", "ACCEPTED", "AWAITING_MINT", "PENDING"} {
		t.Run(status, func(t *testing.T) {
			b, client, m := testBot(t)
			other := common.HexToAddress("0x7000000000000000000000000000000000000007")
			client.auctions = append(client.auctions, testAuction(m, 2, other))
			offer := testOffer(m, 2, status, "200000")
			offer.RequestId = other.Hex()
			client.offers = []threef.OfferDto{offer}
			if err := b.Tick(t.Context(), m, client.auctions); err != nil {
				t.Fatal(err)
			}
			if len(client.created) != 1 || client.created[0].Amount != "700000" {
				t.Fatalf("unreserved cash: %+v", client.created)
			}
		})
	}
}

func TestTickAmbiguousPostRetriesExactIntentInProcess(t *testing.T) {
	b, client, m := testBot(t)
	client.createErr = errors.New("timeout after sending")
	if err := b.Tick(t.Context(), m, client.auctions); err == nil {
		t.Fatal("expected uncertain POST error")
	}
	b.Now = func() time.Time { return time.Unix(2000000000, 0) }
	client.createErr = nil
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 2 || !sameOffer(t, client.created[0], client.created[1]) {
		t.Fatalf("retry changed the original request: %+v", client.created)
	}
	created := client.created[0]
	offer := testOffer(m, 1, "AWAITING_MINT", created.Amount)
	offer.Id = 15
	offer.Nonce, offer.ExpectedReturn, offer.Expiration = created.Nonce, created.ExpectedReturn, created.Expiration
	client.offers = []threef.OfferDto{offer}
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 2 || len(client.cancelled) != 0 {
		t.Fatal("accepted offer changed in process")
	}
}

func TestTickRetainsUncertain64BitNonce(t *testing.T) {
	bot, client, market := testBot(t)
	nonce := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1)).String()
	offer := *threef.NewCreateOfferDto(1, strings.ToLower(client.Broker().Hex()), "500000", "95", nonce, "1900000300", false)
	pending := newIntent(market, common.HexToAddress(client.auctions[0].RequestId), offer)
	bot.pending = []*intent{pending}
	client.createErr = errors.New("uncertain submission")
	if err := bot.Tick(t.Context(), market, client.auctions); err == nil {
		t.Fatal("missing submission error")
	}
	saved, found := firstRetainedIntent(bot)
	if !found || saved.Offer.Nonce != nonce {
		t.Fatalf("uncertain 64-bit nonce was replaced: found=%v", found)
	}
}

func TestTickRepeatedFailureKeepsOriginalIntent(t *testing.T) {
	bot, client, market := testBot(t)
	client.createErr = errors.New("HTTP 500")
	for range 3 {
		if err := bot.Tick(t.Context(), market, client.auctions); err == nil {
			t.Fatal("missing POST failure")
		}
	}
	if len(client.created) != 3 {
		t.Fatalf("got %d POSTs, want three identical attempts", len(client.created))
	}
	for _, offer := range client.created[1:] {
		if !sameOffer(t, offer, client.created[0]) {
			t.Fatal("retry changed the offer identity or terms")
		}
	}
}

func TestTickDoesNotRetryConfirmedOfferMissingFromInventory(t *testing.T) {
	bot, client, market := testBot(t)
	if err := bot.Tick(t.Context(), market, client.auctions); err != nil {
		t.Fatal(err)
	}
	if err := bot.Tick(t.Context(), market, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 {
		t.Fatal("confirmed offer was reposted")
	}
}

func TestTickDoesNotRetryOutsideCurrentPolicy(t *testing.T) {
	for _, change := range []string{"minimum", "budget", "cash", "eligibility"} {
		t.Run(change, func(t *testing.T) {
			bot, client, market := testBot(t)
			client.createErr = errors.New("lost response")
			if err := bot.Tick(t.Context(), market, client.auctions); err == nil {
				t.Fatal("missing initial POST failure")
			}
			switch change {
			case "minimum":
				market.MinimumYield1e8 = big.NewInt(99_000_000)
			case "budget":
				market.Budget = new(big.Int)
			case "cash":
				market.AvailableCash = new(big.Int)
			case "eligibility":
				market.EligibleRequests = nil
			}
			_ = bot.Tick(t.Context(), market, client.auctions)
			if len(client.created) != 1 {
				t.Fatal("retry submitted an offer outside the current policy")
			}
		})
	}
}

func TestTickOnlyClearsDefiniteInitialRejections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial int
		retry   bool
	}{
		{"initial bad request", 400, false},
		{"initial forbidden", 403, false},
		{"ambiguous server error", 500, false},
		{"ambiguous timeout", 408, false},
		{"rejected retry after server error", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot, client, market := testBot(t)
			client.createErr = &apiStatusError{status: tc.initial, operation: "create offer"}
			if err := bot.Tick(t.Context(), market, client.auctions); err == nil {
				t.Fatal("missing rejection")
			}
			if tc.retry {
				client.createErr = &apiStatusError{status: 400, operation: "create offer"}
				if err := bot.Tick(t.Context(), market, client.auctions); err == nil {
					t.Fatal("missing retry rejection")
				}
			}
			saved, found := firstRetainedIntent(bot)
			wantRetained := tc.initial == 500 || tc.initial == 408
			if found != wantRetained {
				t.Fatalf("retained=%v, want %v", found, wantRetained)
			}
			if found && saved.Offer.Nonce != client.created[0].Nonce {
				t.Fatal("rejection changed the unresolved offer identity")
			}
			if !found {
				client.createErr = nil
				if err := bot.Tick(t.Context(), market, client.auctions); err != nil {
					t.Fatal(err)
				}
				if len(client.created) != 2 || client.created[1].Nonce == client.created[0].Nonce {
					t.Fatal("definite rejection prevented the next offer")
				}
			}
		})
	}
}

func TestTickSimulationAndWrongMakerCannotMutate(t *testing.T) {
	b, client, m := testBot(t)
	b.Send = false
	client.offers = []threef.OfferDto{testOffer(m, 1, "SUBMITTED", "10000000")}
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created)+len(client.cancelled) != 0 {
		t.Fatal("simulation mutated upstream")
	}
	b, client, m = testBot(t)
	foreign := testOffer(m, 1, "SUBMITTED", "10000000")
	foreign.Maker = m.Executor.Hex()
	client.offers = []threef.OfferDto{foreign}
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 || len(client.cancelled) != 0 {
		t.Fatal("foreign maker affected default maker")
	}
	client.identityErr = errors.New("default maker mismatch")
	if err := b.Tick(t.Context(), m, client.auctions); err == nil {
		t.Fatal("identity failure ignored")
	}
}

func TestTickExactYieldBoundsAndMissingBinding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		change    func(*fakeClient, *offerMarket)
		wantError bool
	}{
		{"below rounded ceil", func(c *fakeClient, m *offerMarket) { c.auctions[0].AdditionalProperties["exactMaxRate"] = "1.9" }, false},
		{"bad decimal", func(c *fakeClient, m *offerMarket) { c.auctions[0].AdditionalProperties["exactMaxRate"] = "NaN" }, true},
		{"unattested request", func(c *fakeClient, m *offerMarket) { m.EligibleRequests = nil }, false},
		{"unknown reservation asset", func(c *fakeClient, m *offerMarket) {
			c.offers = []threef.OfferDto{testOffer(*m, 2, "SUBMITTED", "200000")}
		}, true},
		{"bad amount", func(c *fakeClient, m *offerMarket) { c.offers = []threef.OfferDto{testOffer(*m, 1, "ACCEPTED", "1e6")} }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, c, m := testBot(t)
			tc.change(c, &m)
			err := b.Tick(t.Context(), m, c.auctions)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
			if len(c.created) != 0 {
				t.Fatalf("unsafe create: %+v", c.created)
			}
		})
	}
}

func TestTickCancelsSubmittedOvercommitButNeverAcceptedOrUIReplacement(t *testing.T) {
	for _, status := range []string{"SUBMITTED", "ACCEPTED", "AWAITING_MINT", "CANCELLED"} {
		t.Run(status, func(t *testing.T) {
			b, c, m := testBot(t)
			c.offers = []threef.OfferDto{testOffer(m, 1, status, "950000")}
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			want := 0
			if status == "SUBMITTED" {
				want = 1
			}
			if len(c.cancelled) != want || len(c.created) != 0 {
				t.Fatalf("created %d cancelled %d", len(c.created), len(c.cancelled))
			}
		})
	}
}

func TestTickReservesMultiplePendingFillsWithoutHistoricalDoubleCount(t *testing.T) {
	for _, status := range []string{"ACCEPTED", "AWAITING_MINT"} {
		t.Run(status, func(t *testing.T) {
			b, c, m := testBot(t)
			other := common.HexToAddress("0x7000000000000000000000000000000000000007")
			c.auctions = append(c.auctions, testAuction(m, 2, other))
			first := testOffer(m, 2, status, "500000")
			first.RequestId, first.PendingAmount, first.FilledAmount = other.Hex(), "100000", "400000"
			second := first
			second.Id, second.PendingAmount, second.FilledAmount = 13, "200000", "300000"
			c.offers = []threef.OfferDto{first, second}
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 1 || c.created[0].Amount != "600000" {
				t.Fatalf("pending fills were not reserved exactly once: %+v", c.created)
			}
		})
	}
}

func TestTickRejectsUnexecutableNativeQuantities(t *testing.T) {
	for _, field := range []string{"principal", "yield"} {
		t.Run(field, func(t *testing.T) {
			b, c, m := testBot(t)
			tooLarge := new(big.Int).Lsh(big.NewInt(1), 128)
			if field == "principal" {
				m.Budget, m.AvailableCash, m.MaxPrincipal = tooLarge, tooLarge, tooLarge
				amount := tooLarge.String()
				c.auctions[0].AmountRequested = *threef.NewNullableString(&amount)
			} else {
				m.MinimumYield1e8 = new(big.Int).Lsh(big.NewInt(1), 255)
			}
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 0 {
				t.Fatalf("unexecutable %s was offered", field)
			}
		})
	}
}

func TestFitsTermsNativeAuthorizationBounds(t *testing.T) {
	m := testMarket()
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	m.MaxPrincipal, m.MinimumYield1e8 = maximum, new(big.Int)
	auction := threef.AuctionDto{AdditionalProperties: map[string]any{exactMaxRateKey: "10000"}}
	fits, err := fitsTerms(m, maximum, maximum, auction)
	if err != nil || !fits {
		t.Fatalf("uint128 boundary rejected: %v %v", fits, err)
	}
	tooLarge := new(big.Int).Add(maximum, big.NewInt(1))
	m.MaxPrincipal = tooLarge
	for _, pair := range [][2]*big.Int{{tooLarge, big.NewInt(1)}, {big.NewInt(1), tooLarge}} {
		fits, err = fitsTerms(m, pair[0], pair[1], auction)
		if err != nil || fits {
			t.Fatalf("unexecutable native authorization accepted: %v %v", fits, err)
		}
	}
}

func TestFitsTermsAuctionRateBasis(t *testing.T) {
	for _, tc := range []struct {
		name      string
		basis     float32
		yield     int64
		wantFits  bool
		wantError bool
	}{
		{"annual ceiling", 31536000, 150000000, true, false},
		{"annual above ceiling", 31536000, 150000001, false, false},
		{"daily ceiling", 86400, 410958, true, false},
		{"daily above ceiling", 86400, 410959, false, false},
		{"annual floor", 31536000, 37500000, true, false},
		{"below annual floor", 31536000, 37499999, false, false},
		{"daily rounded floor", 86400, 102740, true, false},
		{"below daily rounded floor", 86400, 102739, false, false},
		{"daily cannot use annual yield", 86400, 150000000, false, false},
		{"absent basis", 0, 150000000, true, false},
		{"negative basis", -1, 1, false, true},
		{"fractional basis", 0.5, 1, false, true},
		{"nonfinite basis", float32(math.Inf(1)), 1, false, true},
		{"NaN basis", float32(math.NaN()), 1, false, true},
		{"overflow basis", float32(math.Exp2(63)), 1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testMarket()
			m.MaxPrincipal, m.MinimumYield1e8 = big.NewInt(1000000000), new(big.Int)
			auction := threef.AuctionDto{RateBasisSeconds: tc.basis, AdditionalProperties: map[string]any{exactMaxRateKey: "1500"}}
			fits, err := fitsTerms(m, m.MaxPrincipal, big.NewInt(tc.yield), auction)
			if fits != tc.wantFits || (err != nil) != tc.wantError {
				t.Fatalf("fits=%v, err=%v", fits, err)
			}
		})
	}
}

func TestTickPreservesRetainedSubmittedOfferForAnotherFundingPair(t *testing.T) {
	b, c, m := testBot(t)
	if err := b.Tick(t.Context(), m, c.auctions); err != nil {
		t.Fatal(err)
	}
	created := c.created[0]
	row := testOffer(m, 1, "SUBMITTED", created.Amount)
	row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
	c.offers = []threef.OfferDto{row}
	m.Asset = common.HexToAddress("0x9000000000000000000000000000000000000009")
	if err := b.Tick(t.Context(), m, c.auctions); err != nil {
		t.Fatal(err)
	}
	if len(c.cancelled) != 0 || len(c.created) != 1 {
		t.Fatal("another funding pair mutated the retained intent")
	}
}

func TestTickIgnoresKnownDifferentFundingAsset(t *testing.T) {
	b, c, m := testBot(t)
	other := common.HexToAddress("0x7000000000000000000000000000000000000007")
	foreign := m
	foreign.Asset = common.HexToAddress("0x9000000000000000000000000000000000000009")
	c.auctions = append(c.auctions, testAuction(foreign, 2, other))
	offer := testOffer(m, 2, "ACCEPTED", "999999999")
	offer.RequestId = other.Hex()
	c.offers = []threef.OfferDto{offer}
	if err := b.Tick(t.Context(), m, c.auctions); err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 1 || c.created[0].Amount != "800001" {
		t.Fatal("different funding asset incorrectly consumed current token cash")
	}
}

func TestTickReservesCapacityAcrossBotAuctions(t *testing.T) {
	for _, status := range []string{"SUBMITTED", "ACCEPTED", "AWAITING_MINT"} {
		t.Run(status, func(t *testing.T) {
			b, c, m := testBot(t)
			m.Budget, m.AvailableCash = big.NewInt(550000), big.NewInt(600000)
			c.auctions = nil
			for i := range 4 {
				request := common.BigToAddress(big.NewInt(int64(i + 100)))
				m.EligibleRequests[request] = true
				auction := testAuction(m, float32(i+1), request)
				amount := "200000"
				auction.AmountRequested = *threef.NewNullableString(&amount)
				c.auctions = append(c.auctions, auction)
			}
			for i, want := range []string{"200000", "200000", "150000"} {
				if err := b.Tick(t.Context(), m, c.auctions); err != nil {
					t.Fatal(err)
				}
				if len(c.created) != i+1 || c.created[i].Amount != want || c.created[i].AuctionId != float32(i+1) {
					t.Fatalf("offer %d must use remaining capacity: %+v", i, c.created)
				}
				created := c.created[i]
				row := testOffer(m, created.AuctionId, status, created.Amount)
				row.Id, row.RequestId = float32(15+i), c.auctions[i].RequestId
				row.Nonce, row.ExpectedReturn, row.Expiration = created.Nonce, created.ExpectedReturn, created.Expiration
				c.offers = append(c.offers, row)
			}
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 3 || len(c.cancelled) != 0 {
				t.Fatalf("fully reserved budget must stop new offers: %+v", c.created)
			}
			if len(b.pending) != 3 {
				t.Fatalf("must retain every live offer: saved=%d", len(b.pending))
			}
		})
	}
}

func TestTickSharesCashAcrossPairsWithoutCancellingOtherPair(t *testing.T) {
	for _, differentToken := range []bool{false, true} {
		t.Run(strconv.FormatBool(differentToken), func(t *testing.T) {
			b, c, first := testBot(t)
			first.MaxPrincipal = big.NewInt(400000)
			if err := b.Tick(t.Context(), first, c.auctions); err != nil {
				t.Fatal(err)
			}
			created := c.created[0]
			row := testOffer(first, 1, "SUBMITTED", created.Amount)
			row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
			c.offers = []threef.OfferDto{row}
			second := first
			second.UnderlyingAsset = common.HexToAddress("0x99")
			second.Budget, second.AvailableCash, second.MaxPrincipal = big.NewInt(800000), big.NewInt(900000), big.NewInt(800000)
			if differentToken {
				second.Asset = common.HexToAddress("0xaa")
			}
			request := common.HexToAddress("0xbb")
			second.EligibleRequests = map[common.Address]bool{request: true}
			c.auctions = append(c.auctions, testAuction(second, 2, request))
			if err := b.Tick(t.Context(), second, c.auctions); err != nil {
				t.Fatal(err)
			}
			want := "500000"
			if differentToken {
				want = "800000"
			}
			if len(c.cancelled) != 0 || len(c.created) != 2 || c.created[1].Amount != want {
				t.Fatalf("incorrect cross-pair reservations: created=%+v cancelled=%v", c.created, c.cancelled)
			}
		})
	}
}

func TestTickRetryReservesOtherOffersAndPreservesTheirIntents(t *testing.T) {
	for _, failure := range []int{400, 500} {
		t.Run(strconv.Itoa(failure), func(t *testing.T) {
			b, c, m := testBot(t)
			m.MaxPrincipal = big.NewInt(400000)
			other := common.HexToAddress("0x99")
			m.EligibleRequests[other] = true
			c.auctions = append(c.auctions, testAuction(m, 2, other))
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			created := c.created[0]
			row := testOffer(m, 1, "ACCEPTED", created.Amount)
			row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
			c.offers = []threef.OfferDto{row}
			c.createErr = &apiStatusError{status: failure, operation: "create offer"}
			if err := b.Tick(t.Context(), m, c.auctions); err == nil {
				t.Fatal("missing submission failure")
			}
			pending := b.pending
			want := 1
			if failure == 500 {
				want = 2
			}
			if len(pending) != want || !sameOffer(t, pending[0].Offer, created) {
				t.Fatalf("other RAM intent lost on rejection: count=%d", len(pending))
			}
			if failure == 400 {
				return
			}
			// Retry must subtract the existing 400000 offer from cash too.
			m.AvailableCash = big.NewInt(700000)
			if err := b.Tick(t.Context(), m, c.auctions); err == nil || len(c.created) != 2 {
				t.Fatal("retried while aggregate commitments exceeded cash")
			}
			m.AvailableCash = big.NewInt(900000)
			c.createErr = nil
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 3 || !sameOffer(t, c.created[1], c.created[2]) {
				t.Fatal("retry changed the second auction's intent")
			}
		})
	}
}

func TestTickReservesOtherRecipeIntentWithoutBlockingNewAuctions(t *testing.T) {
	for _, status := range []string{"SUBMITTED", "ACCEPTED", "AWAITING_MINT", "PENDING", "unresolved", "SUCCEEDED"} {
		t.Run(status, func(t *testing.T) {
			b, c, first := testBot(t)
			if err := b.Tick(t.Context(), first, c.auctions); err != nil {
				t.Fatal(err)
			}
			created := c.created[0]
			row := testOffer(first, 1, status, created.Amount)
			row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
			if status != "unresolved" {
				c.offers = []threef.OfferDto{row}
			}
			second := first
			second.Executor = common.HexToAddress("0x11")
			second.UnderlyingAsset = common.HexToAddress("0x12")
			request := common.HexToAddress("0x13")
			second.EligibleRequests = map[common.Address]bool{request: true}
			c.auctions = append(c.auctions, testAuction(second, 2, request))
			if status == "SUCCEEDED" {
				c.auctions[0].Status = "SOLVED"
			}
			if err := b.Tick(t.Context(), second, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.cancelled) != 0 || len(c.created) != 2 {
				t.Fatal("another recipe blocked new auction or cancelled the original")
			}
			want := "99999" // Shared cash, separate pair budget.
			if status == "SUCCEEDED" {
				want = "800001"
			}
			if c.created[1].Amount != want {
				t.Fatalf("amount = %s, want %s", c.created[1].Amount, want)
			}
			pending := b.pending
			if status != "SUCCEEDED" && (len(pending) != 2 || pending[0].Executor != first.Executor || pending[0].Offer.Nonce != created.Nonce) {
				t.Fatal("another recipe rewrote retained intent")
			}
			if status == "SUCCEEDED" && (len(pending) != 1 || pending[0].Executor != second.Executor) {
				t.Fatal("terminal intent was not released")
			}
		})
	}
}

func TestTickDoesNotCancelAnotherMarketUIOfferAgainstCurrentPairBudget(t *testing.T) {
	b, c, m := testBot(t)
	other := common.HexToAddress("0x13")
	c.auctions = append(c.auctions, testAuction(m, 2, other))
	row := testOffer(m, 2, "SUBMITTED", "700000")
	row.RequestId = other.Hex()
	c.offers = []threef.OfferDto{row}
	m.Budget = big.NewInt(100000)
	if err := b.Tick(t.Context(), m, c.auctions); err != nil {
		t.Fatal(err)
	}
	if len(c.cancelled) != 0 || len(c.created) != 1 || c.created[0].Amount != "100000" {
		t.Fatal("current pair budget incorrectly reserved a different market commitment")
	}
}

func TestPairBudgetDoesNotReserveAnotherUnderlying(t *testing.T) {
	b, c, m := testBot(t)
	m.Budget = big.NewInt(100000)
	m.AvailableCash = big.NewInt(900000)
	other := common.HexToAddress("0x777")
	c.auctions = append(c.auctions, testAuction(m, 2, other))
	offer := testOffer(m, 2, "AWAITING_MINT", "200000")
	offer.RequestId = other.Hex()
	c.offers = []threef.OfferDto{offer}
	if err := b.Tick(t.Context(), m, c.auctions); err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 1 || c.created[0].Amount != "100000" {
		t.Fatalf("foreign underlying consumed this pair budget: %+v", c.created)
	}
}

// A budget-consuming native check cannot attest UI ownership at zero budget.
// Durable ownership still authorizes cancellation; accepted rows are immutable.
func TestZeroBudgetCancellationRequiresDurableOwnership(t *testing.T) {
	for _, owned := range []bool{false, true} {
		for _, status := range []string{"SUBMITTED", "AWAITING_MINT"} {
			t.Run(fmt.Sprintf("owned%v/%s", owned, status), func(t *testing.T) {
				b, c, m := testBot(t)
				row := testOffer(m, 1, status, "100")
				if owned {
					if err := b.Tick(t.Context(), m, c.auctions); err != nil {
						t.Fatal(err)
					}
					created := c.created[0]
					row = testOffer(m, 1, status, created.Amount)
					row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
				}
				c.offers = []threef.OfferDto{row}
				m.Budget = new(big.Int)
				m.EligibleRequests = map[common.Address]bool{}
				createdBefore := len(c.created)
				if err := b.Tick(t.Context(), m, c.auctions); err != nil {
					t.Fatal(err)
				}
				want := 0
				if owned && status == "SUBMITTED" {
					want = 1
				}
				if len(c.cancelled) != want || len(c.created) != createdBefore {
					t.Fatalf("zero-budget ownership policy violated: cancelled=%v created=%d", c.cancelled, len(c.created))
				}
			})
		}
	}
}
