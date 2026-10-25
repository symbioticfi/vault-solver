package threef

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type fakeClient struct {
	auctions               []threef.AuctionDto
	offers                 []threef.OfferDto
	created                []threef.CreateOfferDto
	cancelled              []uint64
	identityErr, createErr error
	beforeCreate           func(threef.CreateOfferDto)
}

func (f *fakeClient) Identity(context.Context, common.Address) error { return f.identityErr }
func (f *fakeClient) Broker() common.Address {
	return common.HexToAddress("0x8000000000000000000000000000000000000008")
}
func (f *fakeClient) Offers(context.Context) ([]threef.OfferDto, error) { return f.offers, nil }
func (f *fakeClient) Create(_ context.Context, offer threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error) {
	if f.beforeCreate != nil {
		f.beforeCreate(offer)
	}
	f.created = append(f.created, offer)
	return &threef.CreateOfferResponseDto{Id: 15}, f.createErr
}
func (f *fakeClient) Cancel(_ context.Context, id uint64) (*threef.CancelOfferResponseDto, error) {
	f.cancelled = append(f.cancelled, id)
	return &threef.CancelOfferResponseDto{Id: float32(id), Status: "CANCELLED"}, nil
}

func testMarket() offerMarket {
	request := common.HexToAddress("0x1000000000000000000000000000000000000001")
	return offerMarket{ChainID: 1, Account: common.HexToAddress("0x2000000000000000000000000000000000000002"), Executor: common.HexToAddress("0x3000000000000000000000000000000000000003"), RecipeHash: common.HexToHash("0x04"), Asset: common.HexToAddress("0x5000000000000000000000000000000000000005"), UnderlyingAsset: common.HexToAddress("0x6000000000000000000000000000000000000006"), Budget: big.NewInt(1000000), MinimumYield1e8: big.NewInt(19000), MaxPrincipal: big.NewInt(800001), AvailableFunding: big.NewInt(900000), EligibleRequests: map[common.Address]bool{request: true}}
}

func testAuction(m offerMarket, id float32, request common.Address) threef.AuctionDto {
	amount := "10000000"
	deposit := threef.AuctionDepositAssetDto{Address: strings.ToLower(m.Asset.Hex())}
	return threef.AuctionDto{Id: id, RequestId: request.Hex(), Status: "OPEN", AmountRequested: *threef.NewNullableString(&amount), DepositAsset: *threef.NewNullableAuctionDepositAssetDto(&deposit), AdditionalProperties: map[string]any{"exactMaxRate": "2.0"}}
}

func testOffer(m offerMarket, auctionID float32, status string, amount string) threef.OfferDto {
	return threef.OfferDto{Id: 12, AuctionId: auctionID, Maker: strings.ToLower(m.Account.Hex()), RequestId: common.HexToAddress("0x1000000000000000000000000000000000000001").Hex(), ExecutionMode: "PUSH", Status: status, Amount: amount, ExpectedReturn: "1", FilledAmount: "0", FilledExpectedReturn: "0", PendingAmount: "0", PendingExpectedReturn: "0", Nonce: "10", Expiration: "2000000000"}
}

func testBot(t *testing.T) (*offerBot, *fakeClient, offerMarket) {
	t.Helper()
	m := testMarket()
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	client := &fakeClient{auctions: []threef.AuctionDto{testAuction(m, 1, common.HexToAddress("0x1000000000000000000000000000000000000001"))}}
	return &offerBot{Client: client, Store: store, Send: true, Now: func() time.Time { return time.Unix(1900000000, 0) }}, client, m
}

func TestTickSizesExactUnsignedPushAndJournalsBeforePost(t *testing.T) {
	b, client, m := testBot(t)
	client.beforeCreate = func(offer threef.CreateOfferDto) {
		var saved intent
		found, err := b.Store.Load(journalKey(m), &saved)
		if err != nil || !found || saved.Offer.Nonce != offer.Nonce {
			t.Fatalf("POST without durable exact intent: %v %v", found, err)
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

func TestTickAmbiguousPostSurvivesRestartAndExpirationWithoutRetry(t *testing.T) {
	b, client, m := testBot(t)
	directory := t.TempDir()
	store, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	b.Store = store
	client.createErr = errors.New("timeout after sending")
	if err := b.Tick(t.Context(), m, client.auctions); err == nil {
		t.Fatal("expected uncertain POST error")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := state.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	b = &offerBot{Client: client, Store: restored, Send: true, Now: func() time.Time { return time.Unix(2000000000, 0) }}
	if err := b.Tick(t.Context(), m, client.auctions); err == nil || !strings.Contains(err.Error(), "unresolved") {
		t.Fatalf("uncertain intent not retained: %v", err)
	}
	if len(client.created) != 1 {
		t.Fatalf("ambiguous offer resubmitted %d times", len(client.created))
	}
	created := client.created[0]
	offer := testOffer(m, 1, "AWAITING_MINT", created.Amount)
	offer.Nonce, offer.ExpectedReturn, offer.Expiration = created.Nonce, created.ExpectedReturn, created.Expiration
	client.offers = []threef.OfferDto{offer}
	if err := b.Tick(t.Context(), m, client.auctions); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 || len(client.cancelled) != 0 {
		t.Fatal("accepted offer changed on restart")
	}
}

func TestTickSimulationAndWrongMakerCannotMutate(t *testing.T) {
	b, client, m := testBot(t)
	b.Send, b.Store = false, nil
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
				m.Budget, m.AvailableFunding, m.MaxPrincipal = tooLarge, tooLarge, tooLarge
				amount := tooLarge.String()
				c.auctions[0].AmountRequested = *threef.NewNullableString(&amount)
			} else {
				m.MinimumYield1e8 = new(big.Int).Lsh(big.NewInt(1), 255)
			}
			c.auctions[0].AdditionalProperties = nil
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
	auction := threef.AuctionDto{}
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

func TestTickPreservesJournaledSubmittedOfferForAnotherFundingPair(t *testing.T) {
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
		t.Fatal("another funding pair mutated the durable intent")
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

func TestTickRetainsOneLiveBotOriginIntentAcrossAuctions(t *testing.T) {
	for _, status := range []string{"SUBMITTED", "ACCEPTED", "AWAITING_MINT"} {
		t.Run(status, func(t *testing.T) {
			b, c, m := testBot(t)
			other := common.HexToAddress("0x7000000000000000000000000000000000000007")
			m.EligibleRequests[other] = true
			c.auctions = append(c.auctions, testAuction(m, 2, other))
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			created := c.created[0]
			row := testOffer(m, 1, status, created.Amount)
			row.Id, row.Nonce, row.ExpectedReturn, row.Expiration = 15, created.Nonce, created.ExpectedReturn, created.Expiration
			c.offers = []threef.OfferDto{row}
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 1 || len(c.cancelled) != 0 {
				t.Fatal("live bot intent was replaced or cancelled")
			}
			c.offers[0].Status, c.auctions[0].Status = "SUCCEEDED", "SOLVED"
			if err := b.Tick(t.Context(), m, c.auctions); err != nil {
				t.Fatal(err)
			}
			if len(c.created) != 2 || c.created[1].AuctionId != 2 {
				t.Fatal("terminal bot intent prevented the next auction")
			}
		})
	}
}

func TestTickPreservesOtherRecipeIntentUntilTerminal(t *testing.T) {
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
			err := b.Tick(t.Context(), second, c.auctions)
			if status == "unresolved" {
				if err == nil || !strings.Contains(err.Error(), "POST outcome unresolved") {
					t.Fatalf("unresolved intent escaped: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(c.cancelled) != 0 {
				t.Fatal("another recipe cancelled live intent")
			}
			want := 1
			if status == "SUCCEEDED" {
				want = 2
			}
			if len(c.created) != want {
				t.Fatal("another recipe replaced unresolved intent or terminal intent blocked progress")
			}
			var saved intent
			found, err := b.Store.Load(journalKey(first), &saved)
			if err != nil || !found {
				t.Fatal("account journal lost", err)
			}
			if status != "SUCCEEDED" && (saved.Executor != first.Executor || saved.Offer.Nonce != created.Nonce) {
				t.Fatal("another recipe rewrote durable intent")
			}
			if status == "SUCCEEDED" && saved.Executor != second.Executor {
				t.Fatal("terminal handoff did not retain new recipe")
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
	m.AvailableFunding = big.NewInt(900000)
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
